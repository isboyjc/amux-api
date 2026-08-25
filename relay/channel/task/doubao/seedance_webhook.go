package doubao

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// seedanceWebhookSetup 是 Seedance webhook 模式下需要设置给上游的配置项。
type seedanceWebhookSetup struct {
	// CallbackURL 网关构造的上游回调地址（填给上游 callback_url 字段）。
	// 上游任务状态变化时回调到这里，由网关的 webhook 端点接收。
	CallbackURL string
	// TraceID 网关公开任务 ID，透传给上游（ZeroCut 的 trace_id 字段）。
	// ZeroCut 回调时会把 trace_id 原样带回来（方案 A），webhook 端点据此
	// 兜底定位任务。仅 ZeroCut 有此字段：官方 Ark 不认识 trace_id，多传会被
	// 判成 InvalidParameter 把提交打挂，所以对 Ark 这里恒为空（omitempty 丢弃）。
	TraceID string
}

// SetupSeedanceWebhook 判断当前提交是否应走 Seedance webhook 模式，返回需要
// 设置给上游的回调配置。返回 ok=false 表示走轮询模式。
//
// apiType 决定填哪些字段，必须由调用方传入 adaptor 探测到的真实上游类型
// （见 TaskAdaptor.Init）——不能按模型名推断：走哪条 BuildRequestBody 分支
// （2.5 / raw / 通用）是模型决定的，而回调字段的要求是上游厂商决定的，两者
// 不是一回事。
//
// 触发条件（缺一不可）：
//   - 全局开关 system_setting.SeedanceWebhookEnabled 开启；
//   - HMAC 签名密钥 system_setting.SeedanceWebhookSecret 已配置（否则回调地址
//     无法带上可校验的签名，伪造风险未消除，宁可不开启）；
//   - 客户端请求携带 callback_url（middleware 已存入 context）；
//   - 网关公网地址（ServerAddress）可用——私网/localhost 地址上游回调不到，
//     命中时回退轮询，避免任务卡死；
//   - 任务公开 ID（PublicTaskID）已生成（RelayTaskSubmit 在 BuildRequestBody
//     之前已生成），作为回调路由的寻址 key。
//
// 安全：回调地址额外带 sig=HMAC(SeedanceWebhookSecret, taskID) 查询参数。
// 只有拿到回调地址的上游能原样回带这个签名；仅知道公开 task_id 的任务调用者
// 无法伪造（不知道密钥），webhook 端点据此校验请求确实来自上游。
func SetupSeedanceWebhook(info *relaycommon.RelayInfo, clientCallbackURL string, apiType APIType) (setup seedanceWebhookSetup, ok bool) {
	if !system_setting.SeedanceWebhookEnabled {
		return
	}
	if system_setting.SeedanceWebhookSecret == "" {
		return
	}
	if clientCallbackURL == "" {
		return
	}
	if !system_setting.IsPublicHTTPAddress(system_setting.ServerAddress) {
		return
	}
	taskID := ""
	if info != nil && info.TaskRelayInfo != nil {
		taskID = info.TaskRelayInfo.PublicTaskID
	}
	if taskID == "" {
		return
	}
	sig := common.GenerateHMACWithKey([]byte(system_setting.SeedanceWebhookSecret), taskID)
	setup.CallbackURL = strings.TrimRight(system_setting.ServerAddress, "/") + "/api/v1/webhook/seedance/" + taskID + "?sig=" + sig
	if apiType == APITypeZeroCut {
		// ZeroCut 的 webhook 触发条件是 callback_url 与 trace_id 同时传；
		// 回调体里再把 trace_id 原样带回，作为 URL 寻址失败时的兜底。
		setup.TraceID = taskID
	}
	return setup, true
}
