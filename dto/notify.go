package dto

type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`

	// EmailHTML 可选：调用方为邮件渠道预渲染好的完整 HTML（一般用
	// service/emailtpl 渲染，自带 brand 外壳 + 站点统一样式）。
	//
	// 若不为空，sendEmailNotify 会直接把它当邮件正文发出，跳过 {{value}}
	// 占位符替换——因为预渲染发生在调用方，那时已经有了具体值。
	// 其它通道（Bark / Webhook / Gotify）始终走 Content + Values 路径，
	// 各自该是什么格式还是什么格式。
	EmailHTML string `json:"-"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed   = "quota_exceed"
	NotifyTypeChannelUpdate = "channel_update"
	NotifyTypeChannelTest   = "channel_test"
)

func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
