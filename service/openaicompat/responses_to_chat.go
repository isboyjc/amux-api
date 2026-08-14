package openaicompat

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	ResponsesStatusIncomplete              = "incomplete"
	responsesIncompleteReasonContentFilter = "content_filter"
)

// ResponsesStatusString 取出 Responses 响应的 status（它是 json.RawMessage，形如 "completed"）。
func ResponsesStatusString(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Status) == 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(resp.Status)), `"`)
}

// ResponsesFinishReasonFromStatus 把 status=incomplete 的响应映射成 Chat Completions 的
// finish_reason。第二个返回值表示这是不是一个"没跑完"的响应；false 时调用方应沿用自己的
// 默认值（stop / tool_calls），不要当成 length。
//
// 上游 Responses API 用 response.incomplete + incomplete_details.reason 表达"被截断"，
// 老实现从不看这两个字段，一律报 stop，导致 agent 把截断当成正常结束而停止对话。
func ResponsesFinishReasonFromStatus(resp *dto.OpenAIResponsesResponse) (string, bool) {
	if ResponsesStatusString(resp) != ResponsesStatusIncomplete {
		return "", false
	}
	reason := ""
	if resp.IncompleteDetails != nil {
		reason = strings.TrimSpace(resp.IncompleteDetails.Reason)
	}
	if reason == responsesIncompleteReasonContentFilter {
		return "content_filter", true
	}
	// max_output_tokens 以及未知原因都归到 length，与上游 relaykit 的映射保持一致。
	return "length", true
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	text := ExtractOutputTextFromResponses(resp)

	usage := &dto.Usage{}
	if resp.Usage != nil {
		if resp.Usage.InputTokens != 0 {
			usage.PromptTokens = resp.Usage.InputTokens
			usage.InputTokens = resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != 0 {
			usage.CompletionTokens = resp.Usage.OutputTokens
			usage.OutputTokens = resp.Usage.OutputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if resp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
		}
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
	}

	created := resp.CreatedAt

	var toolCalls []dto.ToolCallResponse
	if text == "" && len(resp.Output) > 0 {
		for _, out := range resp.Output {
			if out.Type != "function_call" {
				continue
			}
			name := strings.TrimSpace(out.Name)
			if name == "" {
				continue
			}
			callId := strings.TrimSpace(out.CallId)
			if callId == "" {
				callId = strings.TrimSpace(out.ID)
			}
			toolCalls = append(toolCalls, dto.ToolCallResponse{
				ID:   callId,
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      name,
					Arguments: out.ArgumentsString(),
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	// status=incomplete 时优先用 incomplete_details 映射出的 length / content_filter，
	// 否则会把被截断的响应报成正常结束。
	if mapped, ok := ResponsesFinishReasonFromStatus(resp); ok {
		finishReason = mapped
	}

	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
		msg.Content = ""
	}

	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	// Prefer assistant message outputs.
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
