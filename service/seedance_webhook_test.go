package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// TestWebhookPollGraceExpired webhook 模式只在宽限期内跳过轮询。宽限期一过必须
// 恢复轮询：网关无法确认上游真的接受了 callback_url（静默忽略、回调在公网被丢、
// 归档 worker 收尾 CAS 失败都没有任何信号），永久跳过意味着这些情况下任务只能
// 卡到超时被判失败退款，已经生成好的视频白白丢掉。
func TestWebhookPollGraceExpired(t *testing.T) {
	prev := constant.TaskWebhookPollGraceMinutes
	t.Cleanup(func() { constant.TaskWebhookPollGraceMinutes = prev })
	constant.TaskWebhookPollGraceMinutes = 5

	now := time.Now().Unix()

	cases := []struct {
		name       string
		submitTime int64
		want       bool
	}{
		{"刚提交，宽限期内不轮询", now, false},
		{"宽限期内（1 分钟前）不轮询", now - 60, false},
		{"刚好到宽限期，恢复轮询", now - 5*60, true},
		{"远超宽限期，恢复轮询", now - 30*60, true},
		{"SubmitTime 缺失的历史数据，按已过期处理", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{SubmitTime: tc.submitTime}
			if got := webhookPollGraceExpired(task); got != tc.want {
				t.Errorf("webhookPollGraceExpired = %v, want %v", got, tc.want)
			}
		})
	}

	// 宽限期配成 0/负数 = 关闭 webhook 跳过，永远轮询（运维逃生开关）。
	constant.TaskWebhookPollGraceMinutes = 0
	if !webhookPollGraceExpired(&model.Task{SubmitTime: now}) {
		t.Error("宽限期为 0 时应立即恢复轮询")
	}
}
