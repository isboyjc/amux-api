// Package logger 是事件总线内置的参考订阅者：把每个事件写到系统日志。
//
// 用途：
//   - Phase 1 端到端验证：埋点 → publish → worker → handle 整条链路可观测
//   - 后续新增订阅者的参考实现
//
// 订阅了所有事件（"*"）。如果将来日志噪音过大，可以收窄 Topics() 或关闭注册。
package logger

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/events"
)

type Subscriber struct{}

func (Subscriber) Name() string     { return "logger" }
func (Subscriber) Topics() []string { return []string{"*"} }

func (Subscriber) Handle(_ context.Context, e events.Event) error {
	common.SysLog(fmt.Sprintf("[event] id=%d type=%s aggregate=%d payload=%s",
		e.Id, e.Type, e.AggregateId, string(e.Payload)))
	return nil
}

func init() {
	events.Register(Subscriber{})
}
