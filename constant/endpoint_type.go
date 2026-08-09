package constant

type EndpointType string

const (
	EndpointTypeOpenAI                EndpointType = "openai"
	EndpointTypeOpenAIResponse        EndpointType = "openai-response"
	EndpointTypeOpenAIResponseCompact EndpointType = "openai-response-compact"
	EndpointTypeAnthropic             EndpointType = "anthropic"
	EndpointTypeGemini                EndpointType = "gemini"
	EndpointTypeJinaRerank            EndpointType = "jina-rerank"
	EndpointTypeImageGeneration       EndpointType = "image-generation"
	EndpointTypeEmbeddings            EndpointType = "embeddings"
	EndpointTypeOpenAIVideo           EndpointType = "openai-video"
	// EndpointTypeDashScopeVideo 是阿里云百炼 DashScope 视频异步任务的官方兼容端点。
	EndpointTypeDashScopeVideo EndpointType = "dashscope-video"
	// EndpointTypeVolcengineVideo 是火山方舟（Ark）v3 视频协议的原生兼容端点。
	// 键名沿用线上已有配置里手填的 "volcengine"，避免存量模型记录失效。
	EndpointTypeVolcengineVideo EndpointType = "volcengine"
	//EndpointTypeMidjourney     EndpointType = "midjourney-proxy"
	//EndpointTypeSuno           EndpointType = "suno-proxy"
	//EndpointTypeKling          EndpointType = "kling"
	//EndpointTypeJimeng         EndpointType = "jimeng"
)
