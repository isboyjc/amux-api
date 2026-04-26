package operation_setting

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// SidebarCarouselSetting 控制台侧边栏底部「宣传位轮播」配置。
//
// 与 AnnouncementBar 不同点：
//   - 仅在 console 路由下展示（非全站）
//   - 多条 slide（≤5），用户在前端逐条轮播
//   - 每条内容均可配，背景支持图/视频 URL，留空则取一组预置渐变
//   - 整体一个 version；任一可见字段 hash 变化即让已 dismiss 的用户再次看到
//
// Items 字段为 JSON 数组字符串（跟 console_setting.announcements 同款），
// 落库后由前端解析。GetSidebarCarouselItems() 提供已解析的快捷入口。
type SidebarCarouselSetting struct {
	Enabled bool   `json:"enabled"`
	Items   string `json:"items"`
	Version string `json:"version"`
}

// SidebarCarouselItem 单条 slide 的字段。最多 5 条。
//
//   - Title / Description / CTAText：文案；前端已做空值兜底
//   - Link：站内（"/console/xxx"）或外链（"https://..."）。空 → 卡片不可点
//   - OpenInNewTab：仅外链生效
//   - BgURL：背景媒体地址；以 .mp4/.webm/.mov 结尾按 video 渲染，其它按 image
//     渲染（gif 也走 image）。留空则用 BgPresetIndex 选定的渐变
//   - BgPresetIndex：0..4，对应前端的 5 套预置渐变；BgURL 非空时本字段被忽略
//   - Overlay："dark" | "light"，控制蒙层色，给文字保证对比度
type SidebarCarouselItem struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	CTAText       string `json:"cta_text"`
	Link          string `json:"link"`
	OpenInNewTab  bool   `json:"open_in_new_tab"`
	BgURL         string `json:"bg_url"`
	BgPresetIndex int    `json:"bg_preset_index"`
	Overlay       string `json:"overlay"`
}

// MaxSidebarCarouselItems 后台允许配置的 slide 数量上限。
// 数字过大会让侧边栏底部那块小卡片"轮播太久看不完"，5 条体感刚好；
// 前端循环也按这个上限做交互（再多也不至于崩，但拒收）
const MaxSidebarCarouselItems = 5

// 默认配置：默认关闭。开启后管理员需自行填入 items；items 为空时即使
// enabled=true，前端也不会渲染（GetStatus 阶段就会 short-circuit）
var sidebarCarouselSetting = SidebarCarouselSetting{
	Enabled: false,
	Items:   "[]",
	Version: "",
}

func init() {
	config.GlobalConfig.Register("sidebar_carousel", &sidebarCarouselSetting)
}

// GetSidebarCarouselSetting 拿到配置实例（指针，热更新原地修改）
func GetSidebarCarouselSetting() *SidebarCarouselSetting {
	return &sidebarCarouselSetting
}

// GetSidebarCarouselItems 解析 Items 字符串为对象数组；解析失败回空数组而不
// panic——历史脏数据不应该让 /api/status 整个挂掉
func GetSidebarCarouselItems() []SidebarCarouselItem {
	s := strings.TrimSpace(sidebarCarouselSetting.Items)
	if s == "" {
		return nil
	}
	var items []SidebarCarouselItem
	if err := common.UnmarshalJsonStr(s, &items); err != nil {
		return nil
	}
	if len(items) > MaxSidebarCarouselItems {
		items = items[:MaxSidebarCarouselItems]
	}
	return items
}

// ValidateSidebarCarouselItems 校验前端传上来的 items JSON 字符串。在 option
// 写入前调用。错误消息直接返给管理员，所以要可读。
//
// 校验项：
//   - 必须是合法 JSON 数组
//   - 长度 ≤ MaxSidebarCarouselItems
//   - 每条 link/bg_url 非空时必须 http(s):// 开头
//   - bg_preset_index 在 0..4 之间
//   - overlay 仅允许 "" / "dark" / "light"
//   - title 不能整条全为空白（否则卡片啥都没有，体验崩）
func ValidateSidebarCarouselItems(jsonStr string) error {
	s := strings.TrimSpace(jsonStr)
	if s == "" {
		return nil // 等价于 "[]"，由 caller 决定是否 set
	}
	var items []SidebarCarouselItem
	if err := common.UnmarshalJsonStr(s, &items); err != nil {
		return fmt.Errorf("轮播项 JSON 解析失败：%v", err)
	}
	if len(items) > MaxSidebarCarouselItems {
		return fmt.Errorf("最多支持 %d 个轮播项", MaxSidebarCarouselItems)
	}
	for i, it := range items {
		if strings.TrimSpace(it.Title) == "" {
			return fmt.Errorf("第 %d 项标题不能为空", i+1)
		}
		if it.Link != "" {
			lk := strings.TrimSpace(it.Link)
			if !(strings.HasPrefix(lk, "http://") ||
				strings.HasPrefix(lk, "https://") ||
				strings.HasPrefix(lk, "/")) {
				return fmt.Errorf("第 %d 项跳转链接必须以 http:// / https:// 开头，或以 / 开头的站内路径", i+1)
			}
		}
		if it.BgURL != "" {
			bg := strings.TrimSpace(it.BgURL)
			if !(strings.HasPrefix(bg, "http://") ||
				strings.HasPrefix(bg, "https://") ||
				strings.HasPrefix(bg, "/")) {
				return fmt.Errorf("第 %d 项背景 URL 必须以 http:// / https:// 开头，或以 / 开头的站内静态资源路径", i+1)
			}
		}
		if it.BgPresetIndex < 0 || it.BgPresetIndex > 4 {
			return fmt.Errorf("第 %d 项预置渐变下标必须在 0~4 之间", i+1)
		}
		switch it.Overlay {
		case "", "dark", "light":
			// ok
		default:
			return errors.New("overlay 仅允许 \"dark\" 或 \"light\"")
		}
	}
	return nil
}

// ComputeSidebarCarouselVersion 取 (enabled + items 字符串原文) 的 sha1 前 12 位
// 作为版本号；任一可见字段变化即让前端把已 dismiss 的用户再次提示。
//
// 直接 hash items 原始 JSON 字符串而不是先解析再序列化——admin 改动空格 /
// 字段顺序不应该被当成"内容变化"重发，但 admin 本就是从前端 stringify 的
// 标准 JSON 写过来，差异极小，按原文 hash 已经足够
func ComputeSidebarCarouselVersion() string {
	s := sidebarCarouselSetting
	h := sha1.New()
	h.Write([]byte(strconv.FormatBool(s.Enabled)))
	h.Write([]byte{0})
	h.Write([]byte(s.Items))
	return hex.EncodeToString(h.Sum(nil))[:12]
}
