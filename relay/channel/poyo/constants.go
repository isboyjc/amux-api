package poyo

import "strings"

// ChannelName 渠道名(与 constant.ChannelTypeNames[ChannelTypePoyo] 对应)。
const ChannelName = "poyo"

// ModelList 是对外暴露的模型列表。采用火山原生命名风格但去掉日期版本段,
// 保持稳定不随上游版本漂移。一个 id 同时兼「文生图 / 图生图」——图生图(edit)
// 是 adaptor 内部细节(见 adaptor.go 的 pickUpstreamModel),不作为独立模型暴露。
//
// 管理员建渠道时通过 model_mapping 把这些对外 id 映射到 poyo 上游名,例如:
//
//	{
//	  "doubao-seedream-4-5":      "seedream-4.5",
//	  "doubao-seedream-5-0-lite": "seedream-5.0-lite"
//	}
var ModelList = []string{
	"doubao-seedream-4-5",
	"doubao-seedream-5-0-lite",
}

// modelCaps 记录各 seedream 模型的上游约束差异。来自 poyo 文档
// (https://docs.poyo.ai/api-manual/image-series/*):
//
//	模型              n 上限   size 形式
//	seedream-4.5      15      2K/4K + 8 种比例 + WxH + {width,height}
//	seedream-5.0-lite 15      2K/3K + 8 种比例 + WxH + {width,height}
//
// 两者 size 的档位取值不同,但形式一致,adaptor 无需按模型分流 size;
// 差异只体现在对外的 param_schema 里(前端据此渲染下拉项)。
type modelCaps struct {
	maxN int
}

// modelCapsTable 的 key 是 model_mapping 之后的上游基础名(不含 -edit 后缀)。
var modelCapsTable = map[string]modelCaps{
	"seedream-4.5":      {maxN: 15},
	"seedream-5.0-lite": {maxN: 15},
}

// capsFor 按上游模型名查约束。未知模型返回 ok=false,调用方应放行
// (不认识的模型不代表非法——管理员可能映射到我们还没登记的 poyo 模型)。
func capsFor(upstreamModel string) (modelCaps, bool) {
	c, ok := modelCapsTable[strings.TrimSuffix(upstreamModel, "-edit")]
	return c, ok
}

// exposedModels 是 ModelList 的集合形式,用于识别"没配 model_mapping"。
var exposedModels = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ModelList))
	for _, name := range ModelList {
		m[name] = struct{}{}
	}
	return m
}()

// isUnmappedModel 判断模型名是否仍是我们对外暴露的 id。ConvertImageRequest 拿到的
// 应当是 model_mapping 之后的上游名(如 seedream-4.5);若仍是对外 id
// (如 doubao-seedream-4-5),说明管理员没配 model_mapping——poyo 不认识这个名字,
// 提交必然失败,而提交阶段的错误不加 SkipRetry,会静默地换 key、换渠道一路重试,
// 把一个配置错误放大成一串失败。这里提前拦下并给出可操作的报错。
func isUnmappedModel(model string) bool {
	_, ok := exposedModels[strings.TrimSuffix(model, "-edit")]
	return ok
}

// ── Poyo API 端点 ────────────────────────────────────────────────
const (
	// submitPath 提交生成任务(异步)。
	submitPath = "/api/generate/submit"
	// statusPathFmt 查询任务状态/结果。
	statusPathFmt = "/api/generate/status/%s"
)

// ── Poyo 请求 / 响应结构 ─────────────────────────────────────────

// poyoInput 对应 submit 请求体的 input 对象。
//
// 字段严格对齐 poyo 文档,不要凭直觉添加上游没有的参数(如 seed):本结构体同时
// 用于解析调用方的 extra_body 和序列化给上游,多一个字段就等于多一条把未知参数
// 转发给上游的通路,上游若严格校验会整个请求失败。
type poyoInput struct {
	Prompt              string   `json:"prompt"`
	Size                string   `json:"size,omitempty"`
	N                   int      `json:"n,omitempty"`
	ImageURLs           []string `json:"image_urls,omitempty"`
	EnableSafetyChecker *bool    `json:"enable_safety_checker,omitempty"`
}

// poyoSubmitRequest 是打给 poyo /api/generate/submit 的请求体。
type poyoSubmitRequest struct {
	Model       string    `json:"model"`
	CallbackURL string    `json:"callback_url,omitempty"`
	Input       poyoInput `json:"input"`
}

// poyoSubmitResponse 提交返回。poyo 即便是异步也返回 HTTP 200,业务层用 code 区分。
type poyoSubmitResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID      string `json:"task_id"`
		Status      string `json:"status"`
		CreatedTime string `json:"created_time"`
	} `json:"data"`
}

// poyoStatusResponse 查询返回。
type poyoStatusResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID        string     `json:"task_id"`
		Status        string     `json:"status"`
		Progress      float64    `json:"progress"`       // poyo 返回浮点(如 100.0),声明成 int 会导致整条状态解析失败
		CreditsAmount float64    `json:"credits_amount"` // 同为浮点(如 5.0)
		ErrorMessage  string     `json:"error_message"`
		Files         []poyoFile `json:"files"`
	} `json:"data"`
}

type poyoFile struct {
	FileURL  string `json:"file_url"`
	FileType string `json:"file_type"`
}

// ── Poyo 任务状态 ────────────────────────────────────────────────
const (
	statusNotStarted = "not_started"
	statusFinished   = "finished"
	statusFailed     = "failed"
)

// maxInputImages 是 poyo edit 变体的参考图上限(文档:1-10 张)。
const maxInputImages = 10
