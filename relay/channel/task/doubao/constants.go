package doubao

// ModelList 是渠道新建时在前端"可选模型"中出现的默认清单。除了火山引擎
// 内部的 `xxx-260128` 端点名外，管理员更常见的命名是 `seedance-2.0` /
// `seedance-2.0-api` / `seedance-2.0-fast` 等别名，这里把它们一起列出来，
// 方便一键勾选。实际调用上游时，adapter 会在 BuildRequestBody 里把别名
// 规范化为官方端点名（见 canonicalSeedanceName）。
var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	// seedance 2.0 对外/常用别名
	"seedance-2.0",
	"seedance-2.0-api",
	"seedance-2.0-fast",
	"seedance-2.0-fast-api",
}

var ChannelName = "doubao-video"

// seedanceAliasMap 把所有常见写法归一到火山引擎官方端点名。adapter 在构造
// 上游请求时会把 body.Model 套一次这个映射，保证：
//   - 管理员即便没配 channel 的 model_mapping，用 "seedance-2.0-api" 这类
//     友好名也能直达上游；
//   - videoInputRatioMap 的查表键恒为官方端点名，别名调用也能吃到同款
//     计费策略。
var seedanceAliasMap = map[string]string{
	// pro
	"doubao-seedance-2-0-260128": "doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-pro":    "doubao-seedance-2-0-260128",
	"seedance-2.0":               "doubao-seedance-2-0-260128",
	"seedance-2.0-api":           "doubao-seedance-2-0-260128",
	// fast
	"doubao-seedance-2-0-fast-260128": "doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-fast":        "doubao-seedance-2-0-fast-260128",
	"seedance-2.0-fast":               "doubao-seedance-2-0-fast-260128",
	"seedance-2.0-fast-api":           "doubao-seedance-2-0-fast-260128",
}

// CanonicalSeedanceName 把别名归一到官方端点名；非 seedance 名字原样返回，
// ok=false 方便调用方判断"要不要重写 body.Model"。
func CanonicalSeedanceName(modelName string) (string, bool) {
	canonical, ok := seedanceAliasMap[modelName]
	if !ok {
		return modelName, false
	}
	return canonical, true
}

// videoInputRatioMap 视频输入折扣比率（含视频单价 / 不含视频单价）。
//
// 背景：火山引擎对同一模型存在两档上游单价——纯文本/图片输入（贵）和
// 含视频输入（便宜）。我们只能在后台给模型配一个 ModelRatio，因此约定
// 管理员按"不含视频"的较高费率配置；当请求里检测到视频输入时，自动乘以
// 此比值把结算拉回"便宜那档"，对齐上游真实定价。
//
// 这不是给用户的运营优惠，而是"同模型两档价"的适配；想取消自动折扣、
// 改为完全由单一 ModelRatio + 分组倍率结算，只需让 GetVideoInputRatio
// 恒返回 (0, false)（或把本表置空）。
var videoInputRatioMap = map[string]float64{
	"doubao-seedance-2-0-260128":      28.0 / 46.0, // ~0.6087
	"doubao-seedance-2-0-fast-260128": 22.0 / 37.0, // ~0.5946
}

// GetVideoInputRatio 查表前先做别名归一，保证 "seedance-2.0-api" /
// "doubao-seedance-2-0-pro" 等常见写法都能命中官方端点的折扣系数。
func GetVideoInputRatio(modelName string) (float64, bool) {
	canonical, _ := CanonicalSeedanceName(modelName)
	r, ok := videoInputRatioMap[canonical]
	return r, ok
}
