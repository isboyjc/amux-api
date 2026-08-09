package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestAliVideoEndpointTypes(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
		"happyhorse-1.1-i2v",
		"happyhorse-1.0-i2v",
		"happyhorse-1.1-r2v",
		"happyhorse-1.0-r2v",
		"happyhorse-1.0-video-edit",
		"wan2.7-i2v-2026-04-25",
		"wan2.5-i2v-preview",
		"wan2.2-i2v-flash",
		"wan2.2-i2v-plus",
		"wanx2.1-i2v-plus",
		"wanx2.1-i2v-turbo",
	} {
		got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, modelName)
		want := []constant.EndpointType{
			constant.EndpointTypeOpenAIVideo,
			constant.EndpointTypeDashScopeVideo,
		}
		if len(got) != len(want) {
			t.Fatalf("endpoint types for %s=%v", modelName, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("endpoint types for %s=%v, want=%v", modelName, got, want)
			}
		}
	}
	for _, modelName := range []string{"qwen-plus", "wanx-v1"} {
		got := GetEndpointTypesByChannelType(constant.ChannelTypeAli, modelName)
		if len(got) != 1 || got[0] != constant.EndpointTypeOpenAI {
			t.Fatalf("non-video Ali model %s endpoint types=%v", modelName, got)
		}
	}
}

func TestAliVideoEndpointsHaveDefaultPaths(t *testing.T) {
	want := map[constant.EndpointType]string{
		constant.EndpointTypeOpenAIVideo:    "/v1/video/generations",
		constant.EndpointTypeDashScopeVideo: "/api/v1/services/aigc/video-generation/video-synthesis",
	}
	for endpointType, path := range want {
		info, ok := GetDefaultEndpointInfo(endpointType)
		if !ok {
			t.Fatalf("endpoint %q has no default info", endpointType)
		}
		if info.Path != path || info.Method != "POST" {
			t.Fatalf("endpoint %q info=%+v, want path=%q method=POST", endpointType, info, path)
		}
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

// TestGetEndpointTypes_VolcengineVideo 火山系渠道的视频模型要同时给出站内统一
// 协议与火山 v3 原生协议两个端点，两条路由都真实可用。
func TestGetEndpointTypes_VolcengineVideo(t *testing.T) {
	for _, channelType := range []int{
		constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine,
	} {
		got := GetEndpointTypesByChannelType(channelType, "doubao-seedance-2.5")
		want := []constant.EndpointType{
			constant.EndpointTypeOpenAIVideo,
			constant.EndpointTypeVolcengineVideo,
		}
		if len(got) != len(want) {
			t.Fatalf("channel %d: 端点数 = %d, want %d（%v）", channelType, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("channel %d: 端点[%d] = %q, want %q", channelType, i, got[i], want[i])
			}
		}
		// 优先端点必须是站内统一协议：modality 推断认的是它
		if got[0] != constant.EndpointTypeOpenAIVideo {
			t.Errorf("channel %d: 优先端点应为 openai-video", channelType)
		}

		// 同渠道的文本模型不受影响
		if text := GetEndpointTypesByChannelType(channelType, "doubao-pro-32k"); len(text) != 1 ||
			text[0] != constant.EndpointTypeOpenAI {
			t.Errorf("channel %d: 文本模型端点 = %v, want [openai]", channelType, text)
		}
	}

	// 两个端点都要有默认 path，否则模型广场那一行只显示类型名、没有路径
	for _, et := range []constant.EndpointType{
		constant.EndpointTypeOpenAIVideo, constant.EndpointTypeVolcengineVideo,
	} {
		info, ok := GetDefaultEndpointInfo(et)
		if !ok || info.Path == "" || info.Method == "" {
			t.Errorf("端点 %q 缺默认 path/method: %+v", et, info)
		}
	}
}
