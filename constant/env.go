package constant

var StreamingTimeout int
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// TaskWebhookPollGraceMinutes 是 webhook 模式任务被轮询跳过的宽限期（分钟）。
// 期间信任上游回调；超过后仍未终态的任务重新纳入轮询自愈。见 service 包的
// webhookPollGraceExpired。
var TaskWebhookPollGraceMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// TrustedRedirectDomains is a list of trusted domains for redirect URL validation.
// Domains support subdomain matching (e.g., "example.com" matches "sub.example.com").
var TrustedRedirectDomains []string
