package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// StorageSettings 通用对象存储配置。本期只接 Cloudflare R2，但 Provider
// 字段是为后续扩展（aws s3、minio、阿里 OSS 等）预留的开关——届时增加
// 字段集合 + 在 service/storage 里按 Provider 分发即可，前端表单也只需
// 按 provider 显隐字段。
//
// 字段命名规约（必读）：
//   - 项目里 controller.GetOptions 默认过滤 *Token / *Secret / *Key / *secret /
//     *api_key 后缀，把这些值视为机密，不下发到管理面板的 GET 接口。
//     R2 Secret Access Key 是真机密，所以 json tag 起名 r2_secret，
//     让上面的 "secret" 后缀规则吃到，避免回显到前端。
//   - R2 Access Key ID 类似账号名，留在 r2_access_key_id（默认会展示），
//     和 Discord Client ID 处理方式一致。
type StorageSettings struct {
	// Enabled 总开关。即使所有凭证字段都填了，只要这一项是 false，
	// service.IsEnabled() 也返回 false、上传接口直接 503。设计意图：让 admin
	// 在 R2 没准备好时（CORS 没开 / 域名没绑 / 桶没建）能把入口先关掉，避免
	// 不完整配置被生产流量打到。
	//
	// 默认 false——新部署需要 admin 显式打开；env 变量也只填凭证，不影响这
	// 个开关，仍以 admin 面板为准。
	Enabled bool `json:"enabled"`

	// Provider 当前只接受 "r2"；空值或未来非法值在 Validate 里被规整。
	Provider string `json:"provider"`

	// === Cloudflare R2 ===
	R2AccountID       string `json:"r2_account_id"`
	R2AccessKeyID     string `json:"r2_access_key_id"`
	R2SecretAccessKey string `json:"r2_secret"` // 见上方"字段命名规约"
	R2Bucket          string `json:"r2_bucket"`
	// R2Endpoint 留空时由 service 层按 account_id 派生
	// https://${R2AccountID}.r2.cloudflarestorage.com
	R2Endpoint string `json:"r2_endpoint"`
	// R2Region R2 不分 region，统一 "auto"；放出来让用户在对接其它 S3
	// 兼容服务（minio/wasabi）时能改
	R2Region string `json:"r2_region"`
	// R2PublicBaseURL 末尾不带 "/"，前端拼访问 URL 用。R2 桶默认私有，
	// 必须开 r2.dev subdomain 或绑定自定义域名才能直链
	R2PublicBaseURL string `json:"r2_public_base_url"`

	// ImageTransformEnabled 是否启用 Cloudflare Image Resizing 优化显示。
	//
	// 打开后前端在「显示用」的 <img src> 处拼一层 /cdn-cgi/image/...，让 CF
	// 边缘按需生成缩略 / WebP / AVIF 版本——发给上游模型的原图、attachments
	// 持久化都用原 URL，物理上不会被影响。
	//
	// 前置条件：R2PublicBaseURL 必须是 Cloudflare 代理（橙云）的自定义域名 +
	// CF 计划 Pro 起。r2.dev 子域不挂 cdn-cgi 路由，开了也不生效。
	ImageTransformEnabled bool `json:"image_transform_enabled"`
}

// 默认配置：provider 默认 r2，region 默认 auto；其余空。
// 没填关键字段时 service.IsEnabled() 返回 false，上传接口 503。
var defaultStorageSettings = StorageSettings{
	Provider: "r2",
	R2Region: "auto",
}

func init() {
	config.GlobalConfig.Register("storage", &defaultStorageSettings)
}

// GetStorageSettings 获取当前生效配置。返回的是同一个实例的指针——
// 配置由 config.GlobalConfig 的 LoadFromDB 在启动时和保存时刷新。
func GetStorageSettings() *StorageSettings {
	return &defaultStorageSettings
}
