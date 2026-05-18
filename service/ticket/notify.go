package ticket

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/emailtpl"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 通知去重：同一工单 + 通道 N 秒内只发一次。
//
// Redis 优先（SETNX + TTL）—— 多副本部署下能保证集群级唯一。Redis 未启用
// 时退化为 in-memory map，重启即丢；对单机部署"防止抖动刷屏"足够用。
var (
	notifyDedupeMu sync.Mutex
	notifyDedupe   = make(map[string]int64) // key -> last sent unix
)

const notifyDedupeWindow = 60 * time.Second

func shouldSendNotification(key string) bool {
	// 1. Redis 路径：跨副本一致。SETNX 成功即代表本副本是赢家。
	if common.RedisEnabled && common.RDB != nil {
		redisKey := "ticket:notify_dedupe:" + key
		ok, err := common.RDB.SetNX(common.RDB.Context(), redisKey, "1", notifyDedupeWindow).Result()
		if err == nil {
			return ok
		}
		// Redis 异常时退化到本地 map，不阻塞通知。
		common.SysLog("ticket notify dedupe redis error: " + err.Error())
	}

	// 2. In-memory 路径。
	notifyDedupeMu.Lock()
	defer notifyDedupeMu.Unlock()
	now := time.Now().Unix()
	windowSec := int64(notifyDedupeWindow / time.Second)
	if last, ok := notifyDedupe[key]; ok && now-last < windowSec {
		return false
	}
	notifyDedupe[key] = now
	// 顺手清理过期键，避免长尾累积。
	if len(notifyDedupe) > 1024 {
		for k, ts := range notifyDedupe {
			if now-ts > windowSec {
				delete(notifyDedupe, k)
			}
		}
	}
	return true
}

// NotifyTicketCreated 分发新工单通知（仅给管理员）。
// 调用方应在自己的 goroutine 中调用（controller 用 `go NotifyTicketCreated(...)`），
// 本函数同步完成 SMTP / HTTP；同步语义让重试/可观测更直接。
func NotifyTicketCreated(t *model.Ticket) {
	if t == nil {
		return
	}
	st := operation_setting.GetTicketSetting()
	if !st.Enabled {
		return
	}
	subject := fmt.Sprintf("[%s] 新工单 #%d: %s", common.SystemName, t.Id, truncate(t.Title, 60))

	if st.NotifyEmailToAdmin {
		html := buildAdminNotifyEmail(t, "用户提交了新工单", "info")
		sendAdminEmail(st, "ticket_created:"+itoa(t.Id), subject, html)
	}
	if st.NotifyTelegramToAdmin {
		text := buildAdminNotifyPlainText(t, "📩 用户提交了新工单")
		sendTelegram(st, "ticket_created:"+itoa(t.Id), text)
	}
}

// NotifyTicketReplied 分发工单回复通知。同上，调用方负责 goroutine。
// senderRole 决定通知方向：
//   - user 回复 → 通知管理员
//   - admin 回复 → 通知工单所属用户
func NotifyTicketReplied(t *model.Ticket, senderRole int) {
	if t == nil {
		return
	}
	st := operation_setting.GetTicketSetting()
	if !st.Enabled {
		return
	}
	switch senderRole {
	case model.TicketSenderRoleUser:
		subject := fmt.Sprintf("[%s] 工单 #%d 有新用户回复", common.SystemName, t.Id)
		if st.NotifyEmailToAdmin {
			html := buildAdminNotifyEmail(t, "用户追加了回复", "warning")
			sendAdminEmail(st, "ticket_reply_admin:"+itoa(t.Id), subject, html)
		}
		if st.NotifyTelegramToAdmin {
			text := buildAdminNotifyPlainText(t, "💬 用户追加了回复")
			sendTelegram(st, "ticket_reply_admin:"+itoa(t.Id), text)
		}
	case model.TicketSenderRoleAdmin:
		if !st.NotifyEmailToUser {
			return
		}
		user, err := model.GetUserById(t.UserId, false)
		if err != nil || user == nil || strings.TrimSpace(user.Email) == "" {
			return
		}
		subject := fmt.Sprintf("[%s] 您的工单 #%d 已被回复", common.SystemName, t.Id)
		key := "ticket_reply_user:" + itoa(t.Id)
		if !shouldSendNotification(key) {
			return
		}
		html := buildUserNotifyEmail(t)
		if err := common.SendEmail(subject, user.Email, html); err != nil {
			common.SysLog("ticket notify user email error: " + err.Error())
		}
	}
}

// ----- 通道实现 -----

func sendAdminEmail(st *operation_setting.TicketSetting, dedupeKey, subject, body string) {
	if !shouldSendNotification("email:" + dedupeKey) {
		return
	}
	emails := resolveAdminEmails(st)
	for _, addr := range emails {
		if addr == "" {
			continue
		}
		if err := common.SendEmail(subject, addr, body); err != nil {
			common.SysLog("ticket notify admin email error: " + err.Error())
		}
	}
}

func resolveAdminEmails(st *operation_setting.TicketSetting) []string {
	if strings.TrimSpace(st.AdminEmails) != "" {
		parts := strings.Split(st.AdminEmails, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	// 兜底用 root 邮箱
	root := model.GetRootUser()
	if root != nil && strings.TrimSpace(root.Email) != "" {
		return []string{root.Email}
	}
	return nil
}

// sendTelegram 用 Bot API 给 admin 群发文本消息。
// 失败仅记日志，不阻塞业务。
func sendTelegram(st *operation_setting.TicketSetting, dedupeKey, text string) {
	token := strings.TrimSpace(st.TelegramBotToken)
	chatId := strings.TrimSpace(st.TelegramChatId)
	if token == "" || chatId == "" {
		return
	}
	if !shouldSendNotification("tg:" + dedupeKey) {
		return
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	form := url.Values{}
	form.Set("chat_id", chatId)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		common.SysLog("ticket telegram build req error: " + err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		common.SysLog("ticket telegram send error: " + err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// 读取错误体最多 512 字节，足够调试
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		common.SysLog(fmt.Sprintf("ticket telegram non-2xx status=%d body=%s", resp.StatusCode, string(b)))
	}
}

// ----- 文案构造 -----
//
// 邮件 HTML 通过共享的 service/emailtpl 包渲染，所有交易类邮件共用一套品牌
// 外壳（logo / 主色 CTA / Ubuntu / 卡片布局）。Telegram 走纯文本（Markdown
// 解析容易踩坑，转义麻烦，emoji + 文本块就够）。

// adminTicketURL 拼出指向后台详情页的完整链接；ServerAddress 没配时返回空串。
func adminTicketURL(id int) string {
	server := strings.TrimRight(system_setting.ServerAddress, "/")
	if server == "" {
		return ""
	}
	return fmt.Sprintf("%s/console/ticket/admin/%d", server, id)
}

func userTicketURL(id int) string {
	server := strings.TrimRight(system_setting.ServerAddress, "/")
	if server == "" {
		return ""
	}
	return fmt.Sprintf("%s/console/ticket/%d", server, id)
}

// buildAdminNotifyEmail 渲染管理员收件箱里的工单通知。tone 控制 accent 配色。
func buildAdminNotifyEmail(t *model.Ticket, headline, tone string) string {
	rows := []emailtpl.Row{
		{Label: "工单 ID", Value: fmt.Sprintf("#%d", t.Id)},
		{Label: "标题", Value: emailtpl.HtmlEscape(t.Title)},
		{Label: "类型", Value: emailtpl.HtmlEscape(t.Type)},
		{Label: "分类", Value: emailtpl.HtmlEscape(t.Category)},
		{Label: "提交用户", Value: fmt.Sprintf("#%d", t.UserId)},
	}
	if t.ChannelId > 0 {
		rows = append(rows, emailtpl.Row{Label: "渠道 ID", Value: fmt.Sprintf("#%d", t.ChannelId)})
	}
	if t.ModelName != "" {
		rows = append(rows, emailtpl.Row{Label: "模型", Value: emailtpl.HtmlEscape(t.ModelName)})
	}
	if t.Group != "" {
		rows = append(rows, emailtpl.Row{Label: "分组", Value: emailtpl.HtmlEscape(t.Group)})
	}
	rows = append(rows, emailtpl.Row{
		Label: "最近活动", Value: time.Unix(t.LastReplyAt, 0).Format("2006-01-02 15:04:05"),
	})

	return emailtpl.Render(emailtpl.Content{
		Tone:     tone,
		Eyebrow:  "工单通知",
		Headline: headline,
		Intro:    fmt.Sprintf("工单 #%d 需要您的注意，下面是关键信息。", t.Id),
		Rows:     rows,
		CTAHref:  adminTicketURL(t.Id),
		CTALabel: "前往处理",
	})
}

// buildUserNotifyEmail 渲染用户收件箱里的"工单已被回复"通知。比管理员侧字段
// 少，重点是引导用户回到详情页查看。
func buildUserNotifyEmail(t *model.Ticket) string {
	return emailtpl.Render(emailtpl.Content{
		Tone:     emailtpl.ToneInfo,
		Eyebrow:  "工单通知",
		Headline: "您的工单有新回复",
		Intro: fmt.Sprintf(
			"工单 <strong style=\"color:#0f172a;\">#%d</strong> <span style=\"color:#64748b;\">「%s」</span> 已收到管理员的最新回复，点击下方按钮即可查看。",
			t.Id, emailtpl.HtmlEscape(t.Title)),
		CTAHref:  userTicketURL(t.Id),
		CTALabel: "查看回复",
	})
}

// buildAdminNotifyPlainText Telegram 推送用的纯文本格式。emoji + 文本块。
func buildAdminNotifyPlainText(t *model.Ticket, headline string) string {
	var b strings.Builder
	b.WriteString(headline)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "工单 ID: #%d\n", t.Id)
	fmt.Fprintf(&b, "标题: %s\n", t.Title)
	fmt.Fprintf(&b, "类型: %s / %s\n", t.Type, t.Category)
	fmt.Fprintf(&b, "用户 ID: #%d\n", t.UserId)
	if t.ChannelId > 0 {
		fmt.Fprintf(&b, "渠道: #%d\n", t.ChannelId)
	}
	if t.ModelName != "" {
		fmt.Fprintf(&b, "模型: %s\n", t.ModelName)
	}
	if t.Group != "" {
		fmt.Fprintf(&b, "分组: %s\n", t.Group)
	}
	if url := adminTicketURL(t.Id); url != "" {
		fmt.Fprintf(&b, "\n查看：%s\n", url)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func itoa(i int) string {
	// 避免引入 strconv 仅为此用途
	return fmt.Sprintf("%d", i)
}
