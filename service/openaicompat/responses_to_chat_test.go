package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesFinishReasonFromStatus(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		reason     string
		wantReason string
		wantOK     bool
	}{
		{name: "completed 不覆盖调用方默认值", status: "completed", wantOK: false},
		{name: "空 status 不覆盖", status: "", wantOK: false},
		{name: "撞输出上限", status: "incomplete", reason: "max_output_tokens", wantReason: "length", wantOK: true},
		{name: "内容过滤", status: "incomplete", reason: "content_filter", wantReason: "content_filter", wantOK: true},
		{name: "未知原因归到 length", status: "incomplete", reason: "something_else", wantReason: "length", wantOK: true},
		{name: "没有 incomplete_details 也算 length", status: "incomplete", wantReason: "length", wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &dto.OpenAIResponsesResponse{}
			if tc.status != "" {
				resp.Status = []byte(`"` + tc.status + `"`)
			}
			if tc.reason != "" {
				resp.IncompleteDetails = &dto.IncompleteDetails{Reason: tc.reason}
			}

			got, ok := ResponsesFinishReasonFromStatus(resp)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantReason, got)
		})
	}

	t.Run("nil 安全", func(t *testing.T) {
		got, ok := ResponsesFinishReasonFromStatus(nil)
		assert.False(t, ok)
		assert.Empty(t, got)
	})
}

// incomplete_details 的字段名是 reason，早先误写成 reasoning，导致即使补了分支也解析不出原因。
func TestIncompleteDetailsUsesReasonField(t *testing.T) {
	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.UnmarshalJsonStr(
		`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, &resp))

	require.NotNil(t, resp.IncompleteDetails)
	assert.Equal(t, "max_output_tokens", resp.IncompleteDetails.Reason)
}

// 非流式路径同样不能把被截断的响应报成 stop。
func TestResponsesResponseToChatCompletionsResponse_IncompleteFinishReason(t *testing.T) {
	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.UnmarshalJsonStr(`{
		"id":"resp_1","model":"gpt-5.6-sol","status":"incomplete",
		"incomplete_details":{"reason":"max_output_tokens"},
		"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}],
		"usage":{"input_tokens":10,"output_tokens":1,"total_tokens":11}
	}`, &resp))

	out, _, err := ResponsesResponseToChatCompletionsResponse(&resp, "chatcmpl-1")
	require.NoError(t, err)
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "length", out.Choices[0].FinishReason)
}

func TestResponsesResponseToChatCompletionsResponse_CompletedKeepsStop(t *testing.T) {
	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.UnmarshalJsonStr(`{
		"id":"resp_1","model":"gpt-5.6-sol","status":"completed",
		"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}],
		"usage":{"input_tokens":10,"output_tokens":1,"total_tokens":11}
	}`, &resp))

	out, _, err := ResponsesResponseToChatCompletionsResponse(&resp, "chatcmpl-1")
	require.NoError(t, err)
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "stop", out.Choices[0].FinishReason)
}
