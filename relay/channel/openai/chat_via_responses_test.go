package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// runResponsesToChatStream 用一段 Responses API 的 SSE 驱动 chat->responses 转换器，
// 返回写给客户端的原始响应体和流状态。
func runResponsesToChatStream(t *testing.T, sse string) (string, *relaycommon.RelayInfo) {
	t.Helper()

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-sol"},
		RelayFormat: types.RelayFormatOpenAI,
	}
	info.StreamStatus = relaycommon.NewStreamStatus()

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(sse))}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	return recorder.Body.String(), info
}

const responsesStreamPrelude = `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}

data: {"type":"response.output_text.delta","delta":"hello"}

`

func TestResponsesToChatStream_CompletedKeepsStop(t *testing.T) {
	body, info := runResponsesToChatStream(t, responsesStreamPrelude+
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}}`+"\n\n")

	assert.Contains(t, body, `"finish_reason":"stop"`)
	assert.NotContains(t, body, `"error"`)
	assert.Contains(t, body, "[DONE]")
	// 必须留下终结事件标记，否则日志里和被截断的流无法区分。
	assert.Equal(t, "response.completed", info.StreamStatus.TerminalEvent())
	assert.False(t, info.StreamStatus.HasErrors())
}

// response.incomplete 是 Responses API 表达"没写完"的终结事件。老实现没有这个分支，
// 一律伪造 finish_reason=stop，agent 会把截断当成正常结束而停止对话。
func TestResponsesToChatStream_IncompleteMapsToLength(t *testing.T) {
	body, info := runResponsesToChatStream(t, responsesStreamPrelude+
		`data: {"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}}`+"\n\n")

	assert.Contains(t, body, `"finish_reason":"length"`)
	assert.NotContains(t, body, `"finish_reason":"stop"`)
	assert.Equal(t, "response.incomplete", info.StreamStatus.TerminalEvent())
}

func TestResponsesToChatStream_IncompleteContentFilter(t *testing.T) {
	body, _ := runResponsesToChatStream(t, responsesStreamPrelude+
		`data: {"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`+"\n\n")

	assert.Contains(t, body, `"finish_reason":"content_filter"`)
	assert.NotContains(t, body, `"finish_reason":"stop"`)
}

// 上游没回填 status 时也要按事件类型判定，否则又会退回 stop。
func TestResponsesToChatStream_IncompleteWithoutStatus(t *testing.T) {
	body, _ := runResponsesToChatStream(t, responsesStreamPrelude+
		`data: {"type":"response.incomplete","response":{"id":"resp_1"}}`+"\n\n")

	assert.Contains(t, body, `"finish_reason":"length"`)
	assert.NotContains(t, body, `"finish_reason":"stop"`)
}

// response.done 是部分上游对 response.completed 的别名。
func TestResponsesToChatStream_DoneAliasTreatedAsTerminal(t *testing.T) {
	body, info := runResponsesToChatStream(t, responsesStreamPrelude+
		`data: {"type":"response.done","response":{"id":"resp_1","status":"completed"}}`+"\n\n")

	assert.Contains(t, body, `"finish_reason":"stop"`)
	assert.Equal(t, "response.done", info.StreamStatus.TerminalEvent())
}

// 核心回归：上游一个终结事件都没发就断了。以前这里会伪造 finish_reason=stop + [DONE]，
// 客户端据此认为本轮正常结束；现在必须给出可感知的错误，并在流状态里留下痕迹。
func TestResponsesToChatStream_TruncatedDoesNotFabricateStop(t *testing.T) {
	body, info := runResponsesToChatStream(t, responsesStreamPrelude)

	assert.NotContains(t, body, `"finish_reason":"stop"`)
	assert.Contains(t, body, `"code":"stream_truncated"`)
	// 没有终结事件 + eof 就是被截断的判定组合。
	assert.Empty(t, info.StreamStatus.TerminalEvent())
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
}

// 连一个分片都没收到的空流，同样不能报成正常结束。
func TestResponsesToChatStream_EmptyStreamIsTruncated(t *testing.T) {
	body, info := runResponsesToChatStream(t, "")

	assert.NotContains(t, body, `"finish_reason":"stop"`)
	assert.Contains(t, body, `"code":"stream_truncated"`)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Empty(t, info.StreamStatus.TerminalEvent())
	assert.Equal(t, 0, info.ReceivedResponseCount)
}

// ---- 工具调用与文本共存 ----
//
// Responses API 的事件顺序固定为先 output_text.delta 后 function_call。老实现里
// sendToolCallDelta 见到 outputText 非空就直接返回，等于把带前言的工具调用整个丢掉，
// 客户端只收到一段文字和 finish_reason=stop，agent 无事可做就停止执行。
// gpt-5.x 几乎每次调工具前都会先说一句话，所以这是必现的。

const responsesToolCallSSE = `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}

data: {"type":"response.output_text.delta","delta":"我来看一下这个文件。"}

data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}

data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":\"a.go\"}"}

data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}

`

func TestResponsesToChatStream_ToolCallSurvivesLeadingText(t *testing.T) {
	body, _ := runResponsesToChatStream(t, responsesToolCallSSE)

	// 文本要保留
	assert.Contains(t, body, "我来看一下这个文件。")
	// 工具调用不能被丢掉
	assert.Contains(t, body, `"tool_calls"`)
	assert.Contains(t, body, `"read_file"`)
	assert.Contains(t, body, `call_1`)
	assert.Contains(t, body, `a.go`)
	// 有工具调用就必须是 tool_calls，否则 agent 认为本轮结束
	assert.Contains(t, body, `"finish_reason":"tool_calls"`)
	assert.NotContains(t, body, `"finish_reason":"stop"`)
}

// 没有前言文本的纯工具调用，行为不能被上面的改动带坏。
func TestResponsesToChatStream_ToolCallWithoutText(t *testing.T) {
	body, _ := runResponsesToChatStream(t, `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}

data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}

data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{}"}

data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}

`)

	assert.Contains(t, body, `"read_file"`)
	assert.Contains(t, body, `"finish_reason":"tool_calls"`)
}

// 并行工具调用：每个 call 要拿到自己的 index，不能挤在一起。
func TestResponsesToChatStream_ParallelToolCallsGetDistinctIndexes(t *testing.T) {
	body, _ := runResponsesToChatStream(t, `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}

data: {"type":"response.output_text.delta","delta":"并行查两个文件。"}

data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_a"}}

data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"read_b"}}

data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}

`)

	assert.Contains(t, body, `"read_a"`)
	assert.Contains(t, body, `"read_b"`)
	assert.Contains(t, body, `"index":0`)
	assert.Contains(t, body, `"index":1`)
	assert.Contains(t, body, `"finish_reason":"tool_calls"`)
}

// 被截断时仍然不能伪造 stop —— 上一轮的修复不能被这次改动破坏。
func TestResponsesToChatStream_ToolCallThenTruncated(t *testing.T) {
	body, info := runResponsesToChatStream(t, `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.6-sol"}}

data: {"type":"response.output_text.delta","delta":"我来看一下。"}

data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}

`)

	assert.Contains(t, body, `"read_file"`)
	assert.Contains(t, body, `"code":"stream_truncated"`)
	assert.NotContains(t, body, `"finish_reason":"stop"`)
	assert.Empty(t, info.StreamStatus.TerminalEvent())
}
