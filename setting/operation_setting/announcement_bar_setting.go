package operation_setting

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

// AnnouncementBarSetting 全站「公告横幅」配置。
//
// 与 ConsoleSetting.Announcements（用户控制台里的公告面板）不同，本横幅是
// 整站顶部一行文案，登录前 / 登录后都看得到；用户点击 X 关闭后该 version
// 不再提示，只有内容真正变化（version 变更）才会再次浮现。
//
// 字段语义：
//   - Enabled         总开关；关闭则前端完全不渲染
//   - Content         一行文案（限 500 字），渲染为纯文本，不接受 HTML
//   - Link            点击跳转 URL，留空则不可点击；非空必须 http(s)://
//   - OpenInNewTab    true 时 target=_blank（带 noopener,noreferrer）
//   - BgColor         背景主色（hex，#RRGGBB）；前端用 color-mix 派生深浅
//                     stops，省得管理员调一堆梯度
//   - AccentColor     高光 / 光泽色，对应丝绒上掠过的暖光带
//   - TextColor       文字 + 关闭图标颜色；要保证与 BgColor 对比 ≥4.5:1
//   - Version         由后端按 (内容 + 颜色) hash 自动派生，**前端只读**；
//                     用户的 dismissed_announcement_bar_version 与此值不
//                     一致即触发重新提示。admin 改任一可见字段都会自动 bump
type AnnouncementBarSetting struct {
	Enabled      bool   `json:"enabled"`
	Content      string `json:"content"`
	Link         string `json:"link"`
	OpenInNewTab bool   `json:"open_in_new_tab"`
	BgColor      string `json:"bg_color"`
	AccentColor  string `json:"accent_color"`
	TextColor    string `json:"text_color"`
	Version      string `json:"version"`
}

// 默认配置：暗金丝绒套装；新部署直接启用就是这套。所有色值都是
// CSS 合法 hex；前端没拿到（老部署 / 字段为空）会自动 fallback 到
// 这同一组（默认值在 CSS 变量层面定义）。
var announcementBarSetting = AnnouncementBarSetting{
	Enabled:      false,
	Content:      "",
	Link:         "",
	OpenInNewTab: false,
	BgColor:      "#5a3f1f", // 古铜暗金
	AccentColor:  "#d4a13e", // 亮金高光
	TextColor:    "#f4e4c1", // 羊皮纸米
	Version:      "",
}

func init() {
	config.GlobalConfig.Register("announcement_bar", &announcementBarSetting)
}

// GetAnnouncementBarSetting 获取横幅配置实例。返回的是同一指针——
// LoadFromDB / handleConfigUpdate 会原地刷新字段值。
func GetAnnouncementBarSetting() *AnnouncementBarSetting {
	return &announcementBarSetting
}

// ComputeAnnouncementBarVersion 把可见字段拼起来取 sha1 前 12 位作为版本号。
//
// 设计要点：
//   - 只有任一可见字段变化才让 hash 变化；admin 误点保存（没改任何字段）
//     不会 spam 全站用户
//   - 颜色也算可见字段——admin 改色后所有用户都需重新看到（视觉变化也是
//     一种"通知"）；这是有意为之
//   - admin 想"完全相同的内容再 broadcast 一次"得稍微改个标点；这种场景
//     罕见，可接受。后续如有强需求可以加"强制重发"按钮
//   - 字段间用 \x00 分隔，避免 "ab" + "c" 与 "a" + "bc" 撞 hash
func ComputeAnnouncementBarVersion() string {
	s := announcementBarSetting
	h := sha1.New()
	h.Write([]byte(strconv.FormatBool(s.Enabled)))
	h.Write([]byte{0})
	h.Write([]byte(s.Content))
	h.Write([]byte{0})
	h.Write([]byte(s.Link))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatBool(s.OpenInNewTab)))
	h.Write([]byte{0})
	h.Write([]byte(s.BgColor))
	h.Write([]byte{0})
	h.Write([]byte(s.AccentColor))
	h.Write([]byte{0})
	h.Write([]byte(s.TextColor))
	return hex.EncodeToString(h.Sum(nil))[:12]
}
