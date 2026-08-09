package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// GetEndpointTypesByChannelType 获取渠道最优先端点类型（所有的渠道都支持 OpenAI 端点）
func GetEndpointTypesByChannelType(channelType int, modelName string) []constant.EndpointType {
	var endpointTypes []constant.EndpointType
	switch channelType {
	case constant.ChannelTypeJina:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeJinaRerank}
	//case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeMidjourney}
	//case constant.ChannelTypeSunoAPI:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeSuno}
	//case constant.ChannelTypeKling:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeKling}
	//case constant.ChannelTypeJimeng:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeJimeng}
	case constant.ChannelTypeAws:
		fallthrough
	case constant.ChannelTypeAnthropic:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeAnthropic, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeVertexAi:
		fallthrough
	case constant.ChannelTypeGemini:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeGemini, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeOpenRouter: // OpenRouter 只支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
	case constant.ChannelTypeXai:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse}
	case constant.ChannelTypeSora:
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	case constant.ChannelTypeAli:
		modelName = strings.ToLower(strings.TrimSpace(modelName))
		if isAliVideoModel(modelName) {
			endpointTypes = []constant.EndpointType{
				constant.EndpointTypeOpenAIVideo,
				constant.EndpointTypeDashScopeVideo,
			}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	case constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine:
		// 火山渠道下既有视频模型（Seedance）也有文本模型（豆包 LLM），按模型名
		// 分流。走不到这条分支的话视频模型会被当成文本模型，模型广场的端点栏
		// 和操练场的工作区都会错。
		//
		// 视频模型给两个：站内统一协议 + 火山 v3 原生协议，两条路由都真实可用
		// （见 router/video-router.go）。统一协议放第一个——它是优先端点，也是
		// modality 推断认的那个信号。
		if IsVideoGenerationModel(modelName) {
			endpointTypes = []constant.EndpointType{
				constant.EndpointTypeOpenAIVideo,
				constant.EndpointTypeVolcengineVideo,
			}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	case constant.ChannelTypeMiniMax:
		// MiniMax 渠道同时有文本和视频模型，按模型名分流。走不到这条分支的话
		// 视频模型会被当成文本模型，操练场也就不会渲染视频工作区。
		if IsVideoGenerationModel(modelName) {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	default:
		if IsOpenAIResponseOnlyModel(modelName) {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIResponse}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	}
	if IsImageGenerationModel(modelName) {
		// add to first
		endpointTypes = append([]constant.EndpointType{constant.EndpointTypeImageGeneration}, endpointTypes...)
	}
	return endpointTypes
}

// isAliVideoModel 判断阿里渠道中走 DashScope 异步视频任务协议的模型。
// HappyHorse 使用独立前缀；Wan 视频模型按官方 t2v/i2v/kf2v/s2v 命名识别，
// 避免把 wanx-v1 等图片模型误标成视频端点。
func isAliVideoModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if strings.HasPrefix(modelName, "happyhorse-") {
		return true
	}
	if !strings.HasPrefix(modelName, "wan") {
		return false
	}
	for _, marker := range []string{"-t2v", "-i2v", "-kf2v", "-s2v", "-video"} {
		if strings.Contains(modelName, marker) {
			return true
		}
	}
	return false
}
