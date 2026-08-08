package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestAliVideoEndpointTypes(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
		"wan2.7-i2v-2026-04-25",
	} {
		got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, modelName)
		if len(got) != 1 || got[0] != constant.EndpointTypeOpenAIVideo {
			t.Fatalf("endpoint types for %s=%v", modelName, got)
		}
	}
	got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, "qwen-plus")
	if len(got) != 1 || got[0] != constant.EndpointTypeOpenAI {
		t.Fatalf("qwen-plus endpoint types=%v", got)
	}
}

func TestMiniMaxVideoEndpointTypes(t *testing.T) {
	for _, modelName := range []string{"MiniMax-H3", "minimax-h3"} {
		got := GetEndpointTypesByChannelType(constant.ChannelTypeMiniMax, modelName)
		if len(got) != 1 || got[0] != constant.EndpointTypeOpenAIVideo {
			t.Fatalf("endpoint types for %s=%v", modelName, got)
		}
	}
	// 同渠道的文本模型不能被误判成视频
	got := GetEndpointTypesByChannelType(constant.ChannelTypeMiniMax, "abab6.5s-chat")
	if len(got) != 1 || got[0] != constant.EndpointTypeOpenAI {
		t.Fatalf("abab6.5s-chat endpoint types=%v", got)
	}
}

// TestVideoEndpointHasDefaultPath 视频端点必须有默认 path/method，
// 否则模型广场只会显示一个没有路径的 "openai-video"——前端对空 path
// 会把路径和方法一起隐藏。
func TestVideoEndpointHasDefaultPath(t *testing.T) {
	info, ok := GetDefaultEndpointInfo(constant.EndpointTypeOpenAIVideo)
	if !ok {
		t.Fatal("openai-video has no default endpoint info")
	}
	if info.Path == "" || info.Method == "" {
		t.Fatalf("incomplete endpoint info: %+v", info)
	}
}
