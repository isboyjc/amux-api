package system_setting

import (
	"net"
	"net/url"
	"strings"
)

var ServerAddress = "http://localhost:3000"
var WorkerUrl = ""
var WorkerValidKey = ""
var WorkerAllowHttpImageRequestEnabled = false

// SeedanceWebhookEnabled 全局开关：开启后 Seedance（官渠 Ark / ZeroCut）提交任务时
// 向客户端提供的 callback_url 对应的任务走「上游回调网关」模式，替代网关轮询。
var SeedanceWebhookEnabled = false

// SeedanceWebhookSecret 是 Seedance 上游回调的 HMAC 签名密钥。构造上游回调地址时
// 把 sig=HMAC(secret, taskID) 拼进 URL，webhook 端点据此校验请求确实来自持有该
// 密钥的上游，防止拿到公开 task_id 的调用者伪造回调（伪造 FAILURE 全额退款 /
// 伪造 SUCCESS 篡改用量）。为空时 webhook 模式不生效（回退轮询）。
var SeedanceWebhookSecret = ""

func EnableWorker() bool {
	return WorkerUrl != ""
}

// IsPublicHTTPAddress 判断地址是否为公网可访问的 http(s) 地址。
// Seedance webhook 依赖上游能回调到网关；localhost / 私网 / 环回地址无法
// 被上游回调，命中时调用方应回退到轮询模式。
func IsPublicHTTPAddress(addr string) bool {
	parsed, err := url.Parse(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()) {
		return false
	}
	return true
}
