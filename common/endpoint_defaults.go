package common

import "github.com/QuantumNous/new-api/constant"

// EndpointInfo 描述单个端点的默认请求信息
// path: 上游路径
// method: HTTP 请求方式，例如 POST/GET
// 目前均为 POST，后续可扩展
//
// json 标签用于直接序列化到 API 输出
// 例如：{"path":"/v1/chat/completions","method":"POST"}

type EndpointInfo struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

// defaultEndpointInfoMap 保存内置端点的默认 Path 与 Method
var defaultEndpointInfoMap = map[constant.EndpointType]EndpointInfo{
	constant.EndpointTypeOpenAI:                {Path: "/v1/chat/completions", Method: "POST"},
	constant.EndpointTypeOpenAIResponse:        {Path: "/v1/responses", Method: "POST"},
	constant.EndpointTypeOpenAIResponseCompact: {Path: "/v1/responses/compact", Method: "POST"},
	constant.EndpointTypeAnthropic:             {Path: "/v1/messages", Method: "POST"},
	constant.EndpointTypeGemini:                {Path: "/v1beta/models/{model}:generateContent", Method: "POST"},
	constant.EndpointTypeJinaRerank:            {Path: "/v1/rerank", Method: "POST"},
	constant.EndpointTypeImageGeneration:       {Path: "/v1/images/generations", Method: "POST"},
	constant.EndpointTypeEmbeddings:            {Path: "/v1/embeddings", Method: "POST"},
	// 视频端点此前缺省，导致模型广场只显示一个没有路径的 "openai-video"
	// —— 前端对空 path 会连方法一起隐藏。这里补上站内统一视频协议的入口，
	// 它对所有视频渠道都适用。
	constant.EndpointTypeOpenAIVideo: {Path: "/v1/video/generations", Method: "POST"},
	// 阿里云百炼 DashScope 官方视频异步任务提交协议。任务查询入口为
	// GET /api/v1/tasks/{task_id}，与官方 API 保持一致。
	constant.EndpointTypeDashScopeVideo: {
		Path: "/api/v1/services/aigc/video-generation/video-synthesis", Method: "POST",
	},
	// 火山方舟 v3 原生协议：路由组 /api/v3/contents/generations + POST /tasks，
	// 见 router/video-router.go。有了默认值，管理员不必在每个模型上手填 path
	// ——手填还有个坑：path 是按端点类型存进全局表的，填错会影响同类型的所有模型。
	constant.EndpointTypeVolcengineVideo: {
		Path: "/api/v3/contents/generations/tasks", Method: "POST",
	},
}

// GetDefaultEndpointInfo 返回指定端点类型的默认信息以及是否存在
func GetDefaultEndpointInfo(et constant.EndpointType) (EndpointInfo, bool) {
	info, ok := defaultEndpointInfoMap[et]
	return info, ok
}
