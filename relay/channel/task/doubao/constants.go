package doubao

import "strings"

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
//   - 计费档位查表（seedancePricingMap）的查表键恒为官方端点名，别名调用
//     也能吃到同款计费策略。
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

// seedancePricingMap 按 (model, resolution, hasVideoInput) 三维查表的相对
// 倍率。所有比率均相对于 admin 后台配的 ModelRatio——约定管理员按
// 「不含视频 + 720p」基准价配置，其它组合在此表里相对该基准做乘数。
//
// 数据来源：火山方舟官方价目表（"输出视频分辨率" + "输入是否含视频"两列分档）：
//
//	doubao-seedance-2-0:
//	  720p / 480p：不含视频 46 / 含视频 28（元/百万 token）
//	  1080p：    不含视频 51 / 含视频 31
//	doubao-seedance-2-0-fast:
//	  仅支持 720p：不含视频 37 / 含视频 22（无 1080p 档）
//
// 比率表保留分子分母不预先化简，方便排错时一眼对照官方价格。
//
// 背景说明：火山对同一模型存在多档上游单价（同时按"分辨率 × 是否含视频"
// 分档），但我们只能给模型配一个 ModelRatio。这张表是"同模型多档价"的
// 适配——不是对用户的运营优惠，运营层走分组倍率（group_ratio）独立计算。
//
// 想取消自动档位调整、回到单一 ModelRatio + 分组倍率结算，只需让
// GetSeedancePricingRatio 恒返回 (0, false) 或清空本表。
var seedancePricingMap = map[string]map[string]map[bool]float64{
	"doubao-seedance-2-0-260128": {
		// 480p 与 720p 同价；统一归到 "720p" 档查表
		"720p":  {false: 1.0, true: 28.0 / 46.0},
		"1080p": {false: 51.0 / 46.0, true: 31.0 / 46.0},
	},
	"doubao-seedance-2-0-fast-260128": {
		// fast 不支持 1080p；客户端即使传 "1080p" 上游会拒/降级，仍按 720p 档计费
		"720p": {false: 1.0, true: 22.0 / 37.0},
	},
}

// normalizeSeedanceResolution 将客户端传入的分辨率字符串归一为查表键。
// 严格识别 "1080p"（不区分大小写、容忍空白），其它都按 "720p" 档处理：
//   - "" / "auto" / "480p" / "720p" / 未知值 → "720p"
//   - "1080p" / "1080P" → "1080p"
func normalizeSeedanceResolution(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "1080p") {
		return "1080p"
	}
	return "720p"
}

// GetSeedancePricingRatio 按 (modelName, resolution, hasVideoInput) 查档位
// 倍率。查表前先做别名归一，保证 "seedance-2.0-api" 等友好名也能命中。
//
// 返回：
//   - (ratio, true)：命中（即使 ratio == 1.0 也返回 true，调用方据此区分
//     "命中但默认档"与"未配置该模型"）
//   - (0, false)：模型不在 seedance 计费表里——按单一 ModelRatio 结算
//
// fast 系列查 1080p 时落不到表里，按"先按归一后的分辨率查 → 缺则回落到
// 720p 档"的策略兜底，避免运营改 schema / 客户端乱传时崩到 0 倍率。
func GetSeedancePricingRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	canonical, _ := CanonicalSeedanceName(modelName)
	byRes, ok := seedancePricingMap[canonical]
	if !ok {
		return 0, false
	}
	res := normalizeSeedanceResolution(resolution)
	cell, ok := byRes[res]
	if !ok {
		// 该模型没有这档分辨率的价（如 fast 收到 1080p）→ 兜回 720p 档
		cell, ok = byRes["720p"]
		if !ok {
			return 0, false
		}
	}
	return cell[hasVideo], true
}
