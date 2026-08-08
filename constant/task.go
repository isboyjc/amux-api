package constant

type TaskPlatform string

const (
	TaskPlatformSuno       TaskPlatform = "suno"
	TaskPlatformMidjourney              = "mj"
	TaskPlatformAmuxSTT                 = "amux_stt"
)

const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = "generate"
	TaskActionTextGenerate      = "textGenerate"
	TaskActionFirstTailGenerate = "firstTailGenerate"
	TaskActionReferenceGenerate = "referenceGenerate"
	TaskActionRemix             = "remixGenerate"
)

// 上游原生协议入口标记。路由上的转换中间件负责置位，任务适配器据此切换
// 请求解析与响应构造。放在 constant 这个叶子包，避免 middleware 与各
// adaptor 之间靠字面量约定而出现拼写漂移。
const (
	CtxKeyMinimaxV2Format = "minimax_v2_official_format"
)

// CtxKeyVideoBillingDetail 由视频适配器在 EstimateBilling 里写入计费明细，
// LogTaskConsumption 读出来落进消费日志，供前端渲染「计费过程」。
// 视频模型的 ModelPrice 只是哨兵基准价，不带明细的话日志里只会显示
// 「按次 $1 × 倍率」这种毫无意义的东西。
const CtxKeyVideoBillingDetail = "video_billing_detail"

// CtxKeyVideoUsageSnapshot 是提交时的视频计费口径，落进任务的
// BillingContext，终态结算时用来判断输入素材的归属。
const CtxKeyVideoUsageSnapshot = "video_usage_snapshot"

var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
