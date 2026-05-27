package dto

// 火山方舟（Doubao）v3 协议的视频任务查询响应格式。
// 对外保持与火山官方 GET /api/v3/contents/generations/tasks/{id} 一致的结构，
// 即使上游实际是 ZeroCut 聚合器（其原始响应结构不同，在转换层做了归一化）。
const (
	DoubaoV3StatusQueued    = "queued"
	DoubaoV3StatusRunning   = "running"
	DoubaoV3StatusSucceeded = "succeeded"
	DoubaoV3StatusFailed    = "failed"
)

type DoubaoV3Video struct {
	ID              string                `json:"id"`
	Model           string                `json:"model,omitempty"`
	Status          string                `json:"status"`
	Content         *DoubaoV3VideoContent `json:"content,omitempty"`
	Seed            int                   `json:"seed,omitempty"`
	Resolution      string                `json:"resolution,omitempty"`
	Duration        int                   `json:"duration,omitempty"`
	Ratio           string                `json:"ratio,omitempty"`
	FramesPerSecond int                   `json:"framespersecond,omitempty"`
	Usage           *DoubaoV3VideoUsage   `json:"usage,omitempty"`
	Error           *DoubaoV3VideoError   `json:"error,omitempty"`
	CreatedAt       int64                 `json:"created_at,omitempty"`
	UpdatedAt       int64                 `json:"updated_at,omitempty"`
}

type DoubaoV3VideoContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type DoubaoV3VideoUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type DoubaoV3VideoError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
