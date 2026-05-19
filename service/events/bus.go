package events

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Publish 在外层事务内发布事件。事件写入 event_log + 扇出到 event_dispatch，
// 与 caller 的业务变更同事务，原子提交或回滚。
//
// 行为：
//  1. common.Marshal(payload) 序列化为 JSON
//  2. INSERT 1 行 event_log
//  3. 查 registry 找出所有匹配 eventType 的订阅者
//  4. 对每个匹配订阅者 INSERT 1 行 event_dispatch（status=pending, next_retry_at=0）
//
// 无订阅者时仍写入 event_log（保留事实记录），仅 event_dispatch 行数为 0。
// 任意 INSERT 失败立即返回 error，由 caller 决定回滚业务事务。
func Publish(tx *gorm.DB, eventType string, aggregateId int, payload any) error {
	return publish(tx, eventType, aggregateId, payload)
}

// PublishBestEffortInTx 在外层事务里发布事件，但不阻塞主业务。
//
// 工作机制：用 GORM 的嵌套 Transaction（底层 SAVEPOINT，三库原生支持）把 publish
// 的所有写入隔离起来。publish 失败时只回滚到 savepoint，**外层事务可以继续 commit**。
// 失败仅记日志，不返回错误。
//
// 使用场景：营销 / 分析类事件 —— 偶尔丢一两条可以接受，但绝不能因为事件子系统
// 故障导致用户的核心业务（充值、订阅、兑换、订单等）失败。
//
// 注意：caller 不需要也不应该检查返回值；本函数无返回值就是为了让 caller 写法
// 极简，且避免误把 publish 错误当成业务错误回滚。
func PublishBestEffortInTx(tx *gorm.DB, eventType string, aggregateId int, payload any) {
	if tx == nil {
		// 没有外层事务直接走 NoTx
		if err := PublishNoTx(eventType, aggregateId, payload); err != nil {
			common.SysError(fmt.Sprintf("[events] publish %s (best-effort no-tx) failed: %v",
				eventType, err))
		}
		return
	}
	err := tx.Transaction(func(innerTx *gorm.DB) error {
		return publish(innerTx, eventType, aggregateId, payload)
	})
	if err != nil {
		common.SysError(fmt.Sprintf("[events] publish %s (best-effort in-tx) failed: %v",
			eventType, err))
	}
}

// PublishNoTx 用于无外层事务的场景，内部起新事务保证 event_log 与 dispatch
// 一起 commit。
func PublishNoTx(eventType string, aggregateId int, payload any) error {
	d := getDB()
	if d == nil {
		return errDBNotSet
	}
	return d.Transaction(func(tx *gorm.DB) error {
		return publish(tx, eventType, aggregateId, payload)
	})
}

func publish(tx *gorm.DB, eventType string, aggregateId int, payload any) error {
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	eventLog := &EventLog{
		EventType:   eventType,
		AggregateId: aggregateId,
		Payload:     string(payloadBytes),
		PublishedAt: now,
	}
	if err := insertEventLog(tx, eventLog); err != nil {
		return err
	}

	subscribers := SubscribersFor(eventType)
	if len(subscribers) == 0 {
		return nil
	}

	rows := make([]EventDispatch, 0, len(subscribers))
	for _, name := range subscribers {
		rows = append(rows, EventDispatch{
			EventId:     eventLog.Id,
			Subscriber:  name,
			Status:      StatusPending,
			RetryCount:  0,
			NextRetryAt: 0,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return insertEventDispatches(tx, rows)
}
