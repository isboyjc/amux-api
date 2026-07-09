package poyo

import (
	"math"
	"strconv"
	"strings"
)

// poyo 上游不返回任何 token 计量,而图片生成本身也没有天然的「输出 token」概念。
// 但日志里输入/输出恒为张数(1 张图就是 1/1)可读性极差,所以这里按 Anthropic
// 公开的图片 token 公式估算一个展示值。
//
// 公式:图片按 28x28 像素的 patch 切块,每块算一个 visual token,
// 即 tokens = ⌈width/28⌉ × ⌈height/28⌉。常见的 (w*h)/750 近似即由此而来(28²=784)。
// 出处:https://platform.claude.com/docs/en/build-with-claude/vision 的
// 「Resolution and token cost」一节;poyo_test.go 用该文档的对照表做了回归。
//
// 注意:这只是**展示用估算**,不参与计费。poyo 模型按次计价,金额由
// PriceData.UsePrice 分支算出(见 service/text_quota.go),与 token 无关。
// 也刻意不套用 Anthropic 的 4784 visual token 上限——那是它自己模型的
// 分辨率限制,与 Seedream 无关,套上去会让 4K 图和 2K 图显示成一样大。
const patchSize = 28

// 分辨率档位 → 长边基准像素。覆盖两个模型的并集:
// 4.5 用 2K/4K,5.0-lite 用 2K/3K。
var resolutionTierPixels = map[string]int{
	"2K": 2048,
	"3K": 3072,
	"4K": 4096,
}

// defaultTierPixels 是 size 里没给出档位时的兜底长边(等同 2K)。
const defaultTierPixels = 2048

// estimateImageTokens 按 patch 公式估算单张图的 token 数。
// 宽或高非正时返回 0,由调用方决定兜底。
func estimateImageTokens(width, height int) int {
	if width <= 0 || height <= 0 {
		return 0
	}
	wPatches := int(math.Ceil(float64(width) / patchSize))
	hPatches := int(math.Ceil(float64(height) / patchSize))
	return wPatches * hPatches
}

// parseImageSize 把 size 参数解析成像素宽高。兼容调用方可能传来的几种形态:
//
//	"1728x3072" / "1728*3072"  → 直接取宽高(操练场前端已折算成这种)
//	"2K-16:9"                  → 档位 + 宽高比
//	"2K"                       → 仅档位,按正方形
//	"16:9"                     → 仅宽高比,档位按 2K 兜底
//	""                         → 全部兜底,2048x2048
//
// 解析不出来时返回兜底的 2048x2048,保证日志里始终有个合理的数,不会退化成 0。
func parseImageSize(size string) (int, int) {
	s := strings.TrimSpace(size)
	if s == "" {
		return defaultTierPixels, defaultTierPixels
	}

	// 形态一:显式 WIDTHxHEIGHT
	for _, sep := range []string{"x", "X", "*"} {
		if w, h, ok := parseDimensions(s, sep); ok {
			return w, h
		}
	}

	// 形态二:档位 + 宽高比,如 "2K-16:9"
	if tier, ratio, ok := strings.Cut(s, "-"); ok {
		base := tierPixels(tier)
		if w, h, ok := applyAspectRatio(base, ratio); ok {
			return w, h
		}
	}

	// 形态三:仅宽高比,如 "16:9"
	if strings.Contains(s, ":") {
		if w, h, ok := applyAspectRatio(defaultTierPixels, s); ok {
			return w, h
		}
	}

	// 形态四:仅档位,如 "2K" → 正方形
	if px, ok := resolutionTierPixels[strings.ToUpper(s)]; ok {
		return px, px
	}

	return defaultTierPixels, defaultTierPixels
}

// parseDimensions 尝试按 sep 切出两个正整数。
func parseDimensions(s, sep string) (int, int, bool) {
	left, right, found := strings.Cut(s, sep)
	if !found {
		return 0, 0, false
	}
	w, errW := strconv.Atoi(strings.TrimSpace(left))
	h, errH := strconv.Atoi(strings.TrimSpace(right))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// tierPixels 把 "2K"/"3K"/"4K" 映射到长边像素,未知档位按 2K 兜底。
func tierPixels(tier string) int {
	if px, ok := resolutionTierPixels[strings.ToUpper(strings.TrimSpace(tier))]; ok {
		return px
	}
	return defaultTierPixels
}

// applyAspectRatio 把 "W:H" 宽高比套到 base 长边上,长边取 base,短边按比例缩。
func applyAspectRatio(base int, ratio string) (int, int, bool) {
	left, right, found := strings.Cut(strings.TrimSpace(ratio), ":")
	if !found {
		return 0, 0, false
	}
	wRatio, errW := strconv.ParseFloat(strings.TrimSpace(left), 64)
	hRatio, errH := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if errW != nil || errH != nil || wRatio <= 0 || hRatio <= 0 {
		return 0, 0, false
	}
	if wRatio >= hRatio {
		return base, int(math.Round(float64(base) * hRatio / wRatio)), true
	}
	return int(math.Round(float64(base) * wRatio / hRatio)), base, true
}
