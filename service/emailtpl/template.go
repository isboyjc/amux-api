// Package emailtpl 提供站点统一的 HTML 邮件模板，所有交易类邮件（验证码、
// 密码重置、工单通知等）共享同一套 brand 外壳：站点 logo + 主色 CTA +
// Ubuntu 字体 + 现代化卡片布局。
//
// 设计原则：
//   - 全部 inline CSS，邮件客户端剥外部 stylesheet 时也能正常渲染。
//   - 用 <table> 而不是 div+flex，确保 Outlook（Word 引擎）能渲染。
//   - 颜色对齐站点 Semi UI：主色 #18181b（CTA 按钮、Logo 兜底）；accent
//     按 tone 变（warning/success/danger），用作色条 + eyebrow 文字。
//   - Logo 三档兜底：admin 配的 URL → 站点自带 /logo.png → CSS monogram。
//   - 字体栈头部塞 Ubuntu，配合 <head> 里的 Google Fonts 链接，Apple Mail /
//     iOS / Gmail web / Outlook 365 能加载到真 Ubuntu；不支持的回退系统字体。
package emailtpl

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// BrandPrimary 站点主色，对应 web/src/index.css 中 --semi-color-primary。
// CTA 按钮、Logo monogram 背景统一用这个色，识别度最强。
const BrandPrimary = "#18181b"

// Tone 控制顶部 accent 色条 + eyebrow 文字色，给通知一个语义视觉锚。
// CTA 按钮永远用 BrandPrimary，不跟 tone 变。
const (
	ToneInfo    = "info"
	ToneWarning = "warning"
	ToneSuccess = "success"
	ToneDanger  = "danger"
)

// Content 邮件内容入参；调用方按场景填字段后交给 Render 渲染。
type Content struct {
	Tone      string    // accent 色条 + eyebrow 文字色；空 = info
	Eyebrow   string    // 顶部小字标签（如"账户安全"），可空
	Headline  string    // 主标题（如"验证您的邮箱"）
	Intro     string    // 副文案 / 简介；已 HTML 转义
	Rows      []Row     // 信息表格行；Value 已转义。可空
	Highlight Highlight // 大字号高亮（验证码、关键信息等），可空
	CTAHref   string    // 主按钮目标；空则不渲染按钮
	CTALabel  string    // 主按钮文案；空则默认"查看详情"
	Footnote  string    // 正文下方的小字提示（如"如果不是本人操作请忽略"），可空
}

// Row 信息表格里的一行。Value 由调用方按需 HTML 转义或本身由系统生成
// （数字 ID / 时间戳等）。
type Row struct {
	Label string
	Value string
}

// Highlight 大字号高亮块，最典型场景是邮箱验证码。Value 会以等宽字体大字号
// 居中渲染，Hint 是下方的辅助说明文字。
type Highlight struct {
	Value string
	Hint  string
}

// AccentColor 按 tone 返回 accent 色。warning / success / danger 取自 Semi
// 的 semi-orange-5 / green-5 / red-5；info 退到站点主色。
func AccentColor(tone string) string {
	switch tone {
	case ToneWarning:
		return "#fc8800"
	case ToneSuccess:
		return "#3bb346"
	case ToneDanger:
		return "#f93920"
	default:
		return BrandPrimary
	}
}

// HtmlEscape 防止用户内容（标题 / 名字 / 模型 等）破坏 HTML 结构。
// 邮件层面是纯展示，不做复杂 sanitize；转义 5 个保留字符就够。
func HtmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// resolveLogoURL 解析邮件 brand 区要展示的 logo 绝对 URL。三档：
//  1. admin 配的 common.Logo 绝对 URL → 直接用
//  2. admin 配相对路径 + ServerAddress 已设 → 拼绝对 URL
//  3. 没配 + ServerAddress 已设 → 站点自带 /logo.png（web/public/logo.png
//     构建后内嵌进二进制由根路径静态服务）
//  4. 都没 → 返回空，让 Render 用 monogram 兜底
//
// 不用内联 <svg>：Gmail / Outlook 会把 <svg> 元素整个剥掉。
func resolveLogoURL() string {
	server := strings.TrimRight(system_setting.ServerAddress, "/")
	logo := strings.TrimSpace(common.Logo)

	if logo != "" {
		if strings.HasPrefix(logo, "http://") || strings.HasPrefix(logo, "https://") {
			return logo
		}
		if server != "" {
			if strings.HasPrefix(logo, "/") {
				return server + logo
			}
			return server + "/" + logo
		}
	}
	if server != "" {
		return server + "/logo.png"
	}
	return ""
}

// Render 渲染最终 HTML。简约风格：纯白底无卡片，居中大 logo + 左对齐正文
// + 居中 CTA + 居中 footer，灵感来自 Krea / Linear / Vercel 的极简邮件设计。
//
//	┌────────────────────────────────┐
//	│                                 │
//	│            [Logo]                │ ← 居中 64×64
//	│                                 │
//	│ Headline （左对齐 大号粗体）     │
//	│                                 │
//	│ Intro 文案（左对齐）            │
//	│                                 │
//	│        1 2 3 4 5 6              │ ← Highlight（居中大字号）
//	│        5 分钟内有效              │
//	│                                 │
//	│ key:  value                      │ ← Rows（左对齐 plain text）
//	│                                 │
//	│        [ CTA Button ]            │ ← 居中纯黑按钮
//	│                                 │
//	│ Footnote 小字（左对齐 muted）   │
//	│                                 │
//	│ ─────────────────────────────── │ ← 浅灰分割线
//	│        © System Name             │ ← 居中超小字 footer
//	│                                 │
//	└────────────────────────────────┘
//
// Tone 字段当前不再影响视觉（无 accent 色条 / accent 文字），保留是为了
// 未来需要时能加回 subtle 区分；Eyebrow 也保留并以"很小的 muted 文字"渲染，
// 现有调用方不必改。
func Render(c Content) string {
	systemName := common.SystemName

	// ── Logo（居中 64×64）──────────────────────────────────────
	// 没配 Logo + ServerAddress 不可用时退到 monogram：主色方块 + 系统名首字母。
	var logoBlockHTML string
	if logo := resolveLogoURL(); logo != "" {
		logoBlockHTML = fmt.Sprintf(
			`<img src="%s" alt="%s" width="64" height="64" style="display:block;border:0;outline:none;width:64px;height:64px;border-radius:12px;">`,
			logo, HtmlEscape(systemName))
	} else {
		initial := "·"
		if r := []rune(systemName); len(r) > 0 {
			initial = strings.ToUpper(string(r[0]))
		}
		logoBlockHTML = fmt.Sprintf(
			`<table role="presentation" cellspacing="0" cellpadding="0" border="0" width="64" height="64" style="background:%s;border-radius:12px;">`+
				`<tr><td align="center" valign="middle" style="color:#ffffff;font-size:28px;font-weight:700;line-height:64px;height:64px;">%s</td></tr>`+
				`</table>`,
			BrandPrimary, HtmlEscape(initial))
	}

	// ── Eyebrow（极简风格下做成 muted 小字标签，不再用 accent 色）─
	eyebrowHTML := ""
	if c.Eyebrow != "" {
		eyebrowHTML = fmt.Sprintf(
			`<tr><td style="padding:0 0 8px;">`+
				`<div style="font-size:12px;font-weight:600;color:#94a3b8;text-transform:uppercase;letter-spacing:0.08em;">%s</div>`+
				`</td></tr>`,
			HtmlEscape(c.Eyebrow))
	}

	// ── Headline（大号粗体 左对齐）─────────────────────────────
	headlineHTML := fmt.Sprintf(
		`<tr><td style="padding:0 0 16px;">`+
			`<div style="font-size:24px;font-weight:700;color:#18181b;line-height:1.35;letter-spacing:-0.01em;">%s</div>`+
			`</td></tr>`,
		HtmlEscape(c.Headline))

	// ── Intro ───────────────────────────────────────────────────
	introHTML := ""
	if c.Intro != "" {
		introHTML = fmt.Sprintf(
			`<tr><td style="padding:0 0 24px;">`+
				`<div style="font-size:15px;color:#334155;line-height:1.7;">%s</div>`+
				`</td></tr>`,
			c.Intro)
	}

	// ── Highlight（验证码场景：居中大字号等宽，无背景框）──────────
	highlightHTML := ""
	if c.Highlight.Value != "" {
		hintHTML := ""
		if c.Highlight.Hint != "" {
			hintHTML = fmt.Sprintf(
				`<div style="margin-top:12px;font-size:13px;color:#94a3b8;line-height:1.5;">%s</div>`,
				HtmlEscape(c.Highlight.Hint))
		}
		highlightHTML = fmt.Sprintf(
			`<tr><td align="center" style="padding:8px 0 32px;">`+
				`<div style="font-family:'Ubuntu Mono','SF Mono',Menlo,Consolas,monospace;font-size:36px;font-weight:700;color:#18181b;letter-spacing:0.22em;line-height:1.2;">%s</div>`+
				`%s`+
				`</td></tr>`,
			HtmlEscape(c.Highlight.Value), hintHTML)
	}

	// ── Rows（极简的 key-value 列表，无卡片背景）─────────────────
	rowsHTML := ""
	if len(c.Rows) > 0 {
		var b strings.Builder
		b.WriteString(`<tr><td style="padding:0 0 24px;"><table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="border-collapse:collapse;">`)
		for _, r := range c.Rows {
			fmt.Fprintf(&b,
				`<tr>`+
					`<td style="padding:5px 0;color:#64748b;font-size:13px;width:96px;white-space:nowrap;vertical-align:top;">%s</td>`+
					`<td style="padding:5px 0;color:#0f172a;font-size:14px;vertical-align:top;">%s</td>`+
					`</tr>`,
				r.Label, r.Value)
		}
		b.WriteString(`</table></td></tr>`)
		rowsHTML = b.String()
	}

	// ── CTA 按钮（居中纯黑）──────────────────────────────────────
	ctaHTML := ""
	if c.CTAHref != "" {
		label := c.CTALabel
		if label == "" {
			label = "查看详情"
		}
		ctaHTML = fmt.Sprintf(
			`<tr><td align="center" style="padding:8px 0 32px;">`+
				`<a href="%s" style="display:inline-block;padding:13px 28px;background:%s;color:#ffffff;font-size:14px;font-weight:500;text-decoration:none;border-radius:8px;line-height:1;">%s</a>`+
				`</td></tr>`,
			c.CTAHref, BrandPrimary, HtmlEscape(label))
	}

	// ── Footnote ────────────────────────────────────────────────
	footnoteHTML := ""
	if c.Footnote != "" {
		footnoteHTML = fmt.Sprintf(
			`<tr><td style="padding:0 0 8px;">`+
				`<div style="font-size:13px;color:#94a3b8;line-height:1.6;">%s</div>`+
				`</td></tr>`,
			c.Footnote)
	}

	// ── 最终拼装 ────────────────────────────────────────────────
	// 整个邮件：白底 → 居中容器（max-width 560px）→ 上 logo / 下 footer
	// 用一条 1px 灰线隔开。无外层卡片、无阴影、无边框。
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head>
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<link href="https://fonts.googleapis.com/css2?family=Ubuntu:wght@400;500;700&family=Ubuntu+Mono:wght@400;700&display=swap" rel="stylesheet">
<style>@import url('https://fonts.googleapis.com/css2?family=Ubuntu:wght@400;500;700&family=Ubuntu+Mono:wght@400;700&display=swap');</style>
</head>
<body style="margin:0;padding:0;background:#ffffff;font-family:'Ubuntu','Microsoft YaHei',-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC',sans-serif;color:#0f172a;">
<table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%%" style="background:#ffffff;">
  <tr><td align="center" style="padding:48px 24px 56px;">
    <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%%" style="max-width:560px;">
      <!-- 居中 Logo -->
      <tr><td align="center" style="padding:0 0 40px;">%s</td></tr>
      <!-- 正文（左对齐） -->
      %s
      %s
      %s
      %s
      %s
      %s
      %s
      <!-- 分割线 -->
      <tr><td style="padding:24px 0 20px;">
        <div style="border-top:1px solid #e5e7eb;line-height:0;font-size:0;">&nbsp;</div>
      </td></tr>
      <!-- Footer -->
      <tr><td align="center" style="font-size:12px;color:#94a3b8;line-height:1.7;">
        &copy; %s<br>
        自动通知，请勿直接回复本邮件
      </td></tr>
    </table>
  </td></tr>
</table>
</body></html>`,
		logoBlockHTML,
		eyebrowHTML, headlineHTML, introHTML,
		highlightHTML, rowsHTML, ctaHTML, footnoteHTML,
		HtmlEscape(systemName))
}
