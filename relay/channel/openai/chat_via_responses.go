package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Responses API 的三个终结事件。response.done 是部分上游对 response.completed 的别名。
const (
	responsesEventCompleted  = "response.completed"
	responsesEventDone       = "response.done"
	responsesEventIncomplete = "response.incomplete"
)

// responsesTerminalFinishReason 由终结事件推导 Chat Completions 的 finish_reason。
// 第二个返回值为 false 时表示这不是"未跑完"的响应，调用方沿用自己的默认值。
//
// response.incomplete 代表模型没写完就停了（撞 max_output_tokens、被内容过滤等）。
// 部分上游不在该事件里回填 status，所以这里按事件类型判定，再用 incomplete_details.reason 细分。
func responsesTerminalFinishReason(event *dto.ResponsesStreamResponse) (string, bool) {
	if event == nil {
		return "", false
	}
	if event.Type != responsesEventIncomplete {
		// completed/done 也过一遍 status，防止上游用 completed 事件携带 incomplete 状态。
		return service.ResponsesFinishReasonFromStatus(event.Response)
	}
	if event.Response != nil && event.Response.IncompleteDetails != nil &&
		strings.TrimSpace(event.Response.IncompleteDetails.Reason) == "content_filter" {
		return "content_filter", true
	}
	return "length", true
}

func responsesStreamIndexKey(itemID string, idx *int) string {
	if itemID == "" {
		return ""
	}
	if idx == nil {
		return itemID
	}
	return fmt.Sprintf("%s:%d", itemID, *idx)
}

func stringDeltaFromPrefix(prev string, next string) string {
	if next == "" {
		return ""
	}
	if prev != "" && strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	return next
}

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(&responsesResp, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		responseBody, err = common.Marshal(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

	var (
		usage       = &dto.Usage{}
		outputText  strings.Builder
		usageText   strings.Builder
		sentStart   bool
		sentStop    bool
		sawToolCall bool
		sawTerminal bool // 是否收到过 completed / done / incomplete / failed / error
		streamErr   *types.NewAPIError
	)

	toolCallIndexByID := make(map[string]int)
	toolCallNameByID := make(map[string]string)
	toolCallArgsByID := make(map[string]string)
	toolCallNameSent := make(map[string]bool)
	toolCallCanonicalIDByItemID := make(map[string]string)
	hasSentReasoningSummary := false
	needsReasoningSummarySeparator := false
	//reasoningSummaryTextByKey := make(map[string]string)

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendChatChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		if chunk == nil {
			return true
		}
		if info.RelayFormat == types.RelayFormatOpenAI {
			if err := helper.ObjectData(c, chunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		}

		chunkData, err := common.Marshal(chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := HandleStreamFormat(c, info, string(chunkData), false, false); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	sendStartIfNeeded := func() bool {
		if sentStart {
			return true
		}
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return false
		}
		sentStart = true
		return true
	}

	// sendTruncatedError 在上游流被截断时给客户端一个明确的错误分片，取代伪造的
	// finish_reason=stop —— OpenAI 官方 SDK 见到 data 里带 error 会抛异常，agent 就不会
	// 把截断当成本轮正常结束。写失败（多半是客户端已经走了）只记日志，不改变本次结算。
	sendTruncatedError := func() {
		payload := map[string]any{
			"error": map[string]any{
				"message": "upstream stream ended without a terminal event (truncated)",
				"type":    "upstream_error",
				"code":    "stream_truncated",
			},
		}
		data, err := common.Marshal(payload)
		if err != nil {
			logger.LogError(c, "marshal truncated error failed: "+err.Error())
			return
		}
		if err := helper.StringData(c, string(data)); err != nil {
			logger.LogError(c, "send truncated error failed: "+err.Error())
		}
	}

	//sendReasoningDelta := func(delta string) bool {
	//	if delta == "" {
	//		return true
	//	}
	//	if !sendStartIfNeeded() {
	//		return false
	//	}
	//
	//	usageText.WriteString(delta)
	//	chunk := &dto.ChatCompletionsStreamResponse{
	//		Id:      responseId,
	//		Object:  "chat.completion.chunk",
	//		Created: createAt,
	//		Model:   model,
	//		Choices: []dto.ChatCompletionsStreamResponseChoice{
	//			{
	//				Index: 0,
	//				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
	//					ReasoningContent: &delta,
	//				},
	//			},
	//		},
	//	}
	//	if err := helper.ObjectData(c, chunk); err != nil {
	//		streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	//		return false
	//	}
	//	return true
	//}

	sendReasoningSummaryDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		if needsReasoningSummarySeparator {
			if strings.HasPrefix(delta, "\n\n") {
				needsReasoningSummarySeparator = false
			} else if strings.HasPrefix(delta, "\n") {
				delta = "\n" + delta
				needsReasoningSummarySeparator = false
			} else {
				delta = "\n\n" + delta
				needsReasoningSummarySeparator = false
			}
		}
		if !sendStartIfNeeded() {
			return false
		}

		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ReasoningContent: &delta,
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		hasSentReasoningSummary = true
		return true
	}

	sendToolCallDelta := func(callID string, name string, argsDelta string) bool {
		if callID == "" {
			return true
		}
		if outputText.Len() > 0 {
			// Prefer streaming assistant text over tool calls to match non-stream behavior.
			return true
		}
		if !sendStartIfNeeded() {
			return false
		}

		idx, ok := toolCallIndexByID[callID]
		if !ok {
			idx = len(toolCallIndexByID)
			toolCallIndexByID[callID] = idx
		}
		if name != "" {
			toolCallNameByID[callID] = name
		}
		if toolCallNameByID[callID] != "" {
			name = toolCallNameByID[callID]
		}

		tool := dto.ToolCallResponse{
			ID:   callID,
			Type: "function",
			Function: dto.FunctionResponse{
				Arguments: argsDelta,
			},
		}
		tool.SetIndex(idx)
		if name != "" && !toolCallNameSent[callID] {
			tool.Function.Name = name
			toolCallNameSent[callID] = true
		}

		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tool},
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		sawToolCall = true

		// Include tool call data in the local builder for fallback token estimation.
		if tool.Function.Name != "" {
			usageText.WriteString(tool.Function.Name)
		}
		if argsDelta != "" {
			usageText.WriteString(argsDelta)
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}

		switch streamResp.Type {
		case "response.created":
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
			}

		//case "response.reasoning_text.delta":
		//if !sendReasoningDelta(streamResp.Delta) {
		//	sr.Stop(streamErr)
		//	return
		//}

		//case "response.reasoning_text.done":

		case "response.reasoning_summary_text.delta":
			if !sendReasoningSummaryDelta(streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.reasoning_summary_text.done":
			if hasSentReasoningSummary {
				needsReasoningSummarySeparator = true
			}

		//case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		//	key := responsesStreamIndexKey(strings.TrimSpace(streamResp.ItemID), streamResp.SummaryIndex)
		//	if key == "" || streamResp.Part == nil {
		//		break
		//	}
		//	// Only handle summary text parts, ignore other part types.
		//	if streamResp.Part.Type != "" && streamResp.Part.Type != "summary_text" {
		//		break
		//	}
		//	prev := reasoningSummaryTextByKey[key]
		//	next := streamResp.Part.Text
		//	delta := stringDeltaFromPrefix(prev, next)
		//	reasoningSummaryTextByKey[key] = next
		//	if !sendReasoningSummaryDelta(delta) {
		//		sr.Stop(streamErr)
		//		return
		//	}

		case "response.output_text.delta":
			if !sendStartIfNeeded() {
				sr.Stop(streamErr)
				return
			}

			if streamResp.Delta != "" {
				outputText.WriteString(streamResp.Delta)
				usageText.WriteString(streamResp.Delta)
				delta := streamResp.Delta
				chunk := &dto.ChatCompletionsStreamResponse{
					Id:      responseId,
					Object:  "chat.completion.chunk",
					Created: createAt,
					Model:   model,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
								Content: &delta,
							},
						},
					},
				}
				if !sendChatChunk(chunk) {
					sr.Stop(streamErr)
					return
				}
			}

		case "response.output_item.added", "response.output_item.done":
			if streamResp.Item == nil {
				break
			}
			if streamResp.Item.Type != "function_call" {
				break
			}

			itemID := strings.TrimSpace(streamResp.Item.ID)
			callID := strings.TrimSpace(streamResp.Item.CallId)
			if callID == "" {
				callID = itemID
			}
			if itemID != "" && callID != "" {
				toolCallCanonicalIDByItemID[itemID] = callID
			}
			name := strings.TrimSpace(streamResp.Item.Name)
			if name != "" {
				toolCallNameByID[callID] = name
			}

			newArgs := streamResp.Item.ArgumentsString()
			prevArgs := toolCallArgsByID[callID]
			argsDelta := ""
			if newArgs != "" {
				if strings.HasPrefix(newArgs, prevArgs) {
					argsDelta = newArgs[len(prevArgs):]
				} else {
					argsDelta = newArgs
				}
				toolCallArgsByID[callID] = newArgs
			}

			if !sendToolCallDelta(callID, name, argsDelta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.delta":
			itemID := strings.TrimSpace(streamResp.ItemID)
			callID := toolCallCanonicalIDByItemID[itemID]
			if callID == "" {
				callID = itemID
			}
			if callID == "" {
				break
			}
			toolCallArgsByID[callID] += streamResp.Delta
			if !sendToolCallDelta(callID, "", streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.done":

		case responsesEventCompleted, responsesEventDone, responsesEventIncomplete:
			sawTerminal = true
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				if streamResp.Response.Usage != nil {
					if streamResp.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResp.Response.Usage.InputTokens
						usage.InputTokens = streamResp.Response.Usage.InputTokens
					}
					if streamResp.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResp.Response.Usage.OutputTokens
						usage.OutputTokens = streamResp.Response.Usage.OutputTokens
					}
					if streamResp.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResp.Response.Usage.TotalTokens
					} else {
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					}
					if streamResp.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResp.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.ImageTokens = streamResp.Response.Usage.InputTokensDetails.ImageTokens
						usage.PromptTokensDetails.AudioTokens = streamResp.Response.Usage.InputTokensDetails.AudioTokens
					}
					if streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
						usage.CompletionTokenDetails.ReasoningTokens = streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens
					}
				}
			}

			if !sendStartIfNeeded() {
				sr.Stop(streamErr)
				return
			}
			if !sentStop {
				if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
					info.ClaudeConvertInfo.Usage = usage
				}
				finishReason := "stop"
				if sawToolCall && outputText.Len() == 0 {
					finishReason = "tool_calls"
				}
				if mapped, ok := responsesTerminalFinishReason(&streamResp); ok {
					finishReason = mapped
				}
				stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
				if !sendChatChunk(stop) {
					sr.Stop(streamErr)
					return
				}
				sentStop = true
			}
			// Responses API 不发 data: [DONE]，不显式标记的话流结束后只会记成 eof，
			// 日志里分不出"正常跑完"和"被截断"。
			sr.MarkTerminal(streamResp.Type)

		case "response.error", "response.failed":
			sawTerminal = true
			sr.MarkTerminal(streamResp.Type)
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return

		default:
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, usageText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	if !sentStart {
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return nil, streamErr
		}
	}
	if !sentStop {
		if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
			info.ClaudeConvertInfo.Usage = usage
		}
		// 客户端自己断开（agent 取消、用户中止）时上游当然不会有终结事件，那不是截断。
		// 线上 client_gone 占比不低，不排除掉会把正常的取消刷成一堆假的截断告警。
		clientGone := c.Request != nil && c.Request.Context().Err() != nil
		truncated := !sawTerminal && !clientGone
		if truncated {
			// 上游一个终结事件都没发就把连接关了：这条流是被截断的。
			// 以前这里会伪造 finish_reason=stop，客户端（尤其是 agent）据此认为本轮
			// 正常结束、直接停止对话，问题完全不可见。
			logger.LogError(c, fmt.Sprintf(
				"responses stream truncated: no terminal event, sent_start=%v saw_tool_call=%v output_len=%d",
				sentStart, sawToolCall, outputText.Len()))
			info.StreamStatus.RecordError("responses stream truncated: no terminal event")
		}
		// 非 OpenAI 格式（Claude / Gemini）的错误事件是另一套结构，这里先只记录不改其行为，
		// 等日志确认这些格式上也有量再单独处理。
		if truncated && info.RelayFormat == types.RelayFormatOpenAI {
			sendTruncatedError()
		} else {
			finishReason := "stop"
			if sawToolCall && outputText.Len() == 0 {
				finishReason = "tool_calls"
			}
			stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
			if !sendChatChunk(stop) {
				return nil, streamErr
			}
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
