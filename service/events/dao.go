package events

import (
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"
)

// EventLog 记录所有通过事件总线发布的事件（不可变事实流）。
// 此表只追加，不更新。配套表 event_dispatch 记录每个订阅者的投递状态。
type EventLog struct {
	Id          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType   string `gorm:"type:varchar(64);not null;index:idx_event_log_type_time,priority:1" json:"event_type"`
	AggregateId int    `gorm:"index" json:"aggregate_id"`
	Payload     string `gorm:"type:text;not null" json:"payload"`
	PublishedAt int64  `gorm:"not null;index:idx_event_log_type_time,priority:2;index:idx_event_log_pub" json:"published_at"`
}

// EventDispatch 记录每个订阅者对每个事件的投递状态。
// 一个 EventLog 行可以对应多行 EventDispatch（每个匹配的订阅者一行）。
type EventDispatch struct {
	Id          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	EventId     int64  `gorm:"not null;index:idx_dispatch_event" json:"event_id"`
	Subscriber  string `gorm:"type:varchar(64);not null;index:idx_dispatch_poll,priority:2" json:"subscriber"`
	Status      string `gorm:"type:varchar(16);not null;default:'pending';index:idx_dispatch_poll,priority:1" json:"status"`
	RetryCount  int    `gorm:"not null;default:0" json:"retry_count"`
	NextRetryAt int64  `gorm:"not null;default:0;index:idx_dispatch_poll,priority:3" json:"next_retry_at"`
	LastError   string `gorm:"type:text" json:"last_error"`
	WorkerId    string `gorm:"type:varchar(64)" json:"worker_id"`
	CreatedAt   int64  `gorm:"not null;index:idx_dispatch_created" json:"created_at"`
	UpdatedAt   int64  `gorm:"not null" json:"updated_at"`
	ProcessedAt int64  `json:"processed_at"`
}

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusDead       = "dead"
)

// db 是事件子系统使用的 GORM 实例。由 main.go 在启动时通过 SetDB 注入，
// 避免 service/events ↔ model 的导入循环。
var (
	dbMu sync.RWMutex
	db   *gorm.DB
)

// SetDB 注入 GORM 实例。必须在 AutoMigrate / Publish / StartWorker 之前调用。
func SetDB(d *gorm.DB) {
	dbMu.Lock()
	defer dbMu.Unlock()
	db = d
}

func getDB() *gorm.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return db
}

// errDBNotSet 是 events.SetDB 未被调用时的错误。
var errDBNotSet = errors.New("events: DB not set; call events.SetDB before using the bus")

// AutoMigrate 创建 / 升级 event_log 和 event_dispatch 表。
// 由 main.go 在 model.InitDB 后调用。
func AutoMigrate() error {
	d := getDB()
	if d == nil {
		return errDBNotSet
	}
	return d.AutoMigrate(&EventLog{}, &EventDispatch{})
}

// ---------- EventLog DAO ----------

func insertEventLog(tx *gorm.DB, e *EventLog) error {
	if tx == nil {
		tx = getDB()
	}
	if tx == nil {
		return errDBNotSet
	}
	return tx.Create(e).Error
}

// getEventLogPayload 按 id 取事件 type / aggregate_id / payload / published_at（worker 用）。
func getEventLogPayload(id int64) (eventType string, aggregateId int, payload string, publishedAt int64, err error) {
	d := getDB()
	if d == nil {
		return "", 0, "", 0, errDBNotSet
	}
	var e EventLog
	if err = d.Select("event_type", "aggregate_id", "payload", "published_at").
		Where("id = ?", id).First(&e).Error; err != nil {
		return "", 0, "", 0, err
	}
	return e.EventType, e.AggregateId, e.Payload, e.PublishedAt, nil
}

// DeleteEventLogsBatch 批量删除超过 cutoff 且无活跃 dispatch 引用的 event_log 行。
// 三库兼容：先 Pluck id 再按 id IN 删除，避免 SQLite 默认不支持 DELETE ... LIMIT。
func DeleteEventLogsBatch(cutoff int64, batchSize int) (int64, error) {
	d := getDB()
	if d == nil {
		return 0, errDBNotSet
	}
	var ids []int64
	err := d.Model(&EventLog{}).
		Where("published_at < ?", cutoff).
		Where("NOT EXISTS (SELECT 1 FROM event_dispatches WHERE event_dispatches.event_id = event_logs.id AND event_dispatches.status IN (?, ?, ?))",
			StatusPending, StatusProcessing, StatusDead).
		Order("id").
		Limit(batchSize).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := d.Where("id IN ?", ids).Delete(&EventLog{})
	return res.RowsAffected, res.Error
}

// ---------- EventDispatch DAO ----------

func insertEventDispatches(tx *gorm.DB, rows []EventDispatch) error {
	if len(rows) == 0 {
		return nil
	}
	if tx == nil {
		tx = getDB()
	}
	if tx == nil {
		return errDBNotSet
	}
	return tx.Create(&rows).Error
}

func pendingDispatchIDs(now int64, batchSize int) ([]int64, error) {
	d := getDB()
	if d == nil {
		return nil, errDBNotSet
	}
	var ids []int64
	err := d.Model(&EventDispatch{}).
		Where("status = ? AND next_retry_at <= ?", StatusPending, now).
		Order("id").
		Limit(batchSize).
		Pluck("id", &ids).Error
	return ids, err
}

// claimDispatch 用乐观锁 claim 一行待处理记录。返回 true 表示 claim 成功。
func claimDispatch(id int64, workerId string) (bool, error) {
	d := getDB()
	if d == nil {
		return false, errDBNotSet
	}
	now := time.Now().Unix()
	res := d.Model(&EventDispatch{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":     StatusProcessing,
			"worker_id":  workerId,
			"updated_at": now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func getDispatchById(id int64) (*EventDispatch, error) {
	d := getDB()
	if d == nil {
		return nil, errDBNotSet
	}
	var row EventDispatch
	if err := d.Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// finishDispatchOK 标记成功。仅在仍由 workerId 持有时生效，避免被 reclaim 后
// 老实例慢悠悠返回时再次写库。
func finishDispatchOK(id int64, workerId string) error {
	d := getDB()
	if d == nil {
		return errDBNotSet
	}
	now := time.Now().Unix()
	return d.Model(&EventDispatch{}).
		Where("id = ? AND status = ? AND worker_id = ?", id, StatusProcessing, workerId).
		Updates(map[string]any{
			"status":       StatusDone,
			"updated_at":   now,
			"processed_at": now,
			"last_error":   "",
		}).Error
}

func rescheduleDispatch(id int64, workerId string, retryCount int, nextRetryAt int64, lastError string) error {
	d := getDB()
	if d == nil {
		return errDBNotSet
	}
	now := time.Now().Unix()
	return d.Model(&EventDispatch{}).
		Where("id = ? AND status = ? AND worker_id = ?", id, StatusProcessing, workerId).
		Updates(map[string]any{
			"status":        StatusPending,
			"retry_count":   retryCount,
			"next_retry_at": nextRetryAt,
			"last_error":    truncateError(lastError),
			"worker_id":     "",
			"updated_at":    now,
		}).Error
}

func markDispatchDead(id int64, workerId string, retryCount int, lastError string) error {
	d := getDB()
	if d == nil {
		return errDBNotSet
	}
	now := time.Now().Unix()
	return d.Model(&EventDispatch{}).
		Where("id = ? AND status = ? AND worker_id = ?", id, StatusProcessing, workerId).
		Updates(map[string]any{
			"status":      StatusDead,
			"retry_count": retryCount,
			"last_error":  truncateError(lastError),
			"worker_id":   "",
			"updated_at":  now,
		}).Error
}

// reclaimStuckDispatches 回收任意 worker 残留的 processing 行（不再限 worker_id）。
// 单凭 updated_at < staleBefore 已能区分"健康 worker（新鲜）"和"崩溃 worker（陈旧）"：
// 健康 worker 的 handler 在 HandleTimeout（默认 30s）内一定完成，updated_at 不会陈旧。
// staleBefore 默认 5 分钟，对 30s timeout 有 10x 余量。
//
// 不限 worker_id 之后，**任何**新启动的实例都能回收**任何**崩溃实例的孤儿行；
// 配合 finalize 系列的 worker_id 校验，避免被回收的行被原 worker 慢慢醒来时误改。
func reclaimStuckDispatches(staleBefore int64) (int64, error) {
	d := getDB()
	if d == nil {
		return 0, errDBNotSet
	}
	now := time.Now().Unix()
	res := d.Model(&EventDispatch{}).
		Where("status = ? AND updated_at < ?", StatusProcessing, staleBefore).
		Updates(map[string]any{
			"status":     StatusPending,
			"worker_id":  "",
			"updated_at": now,
		})
	return res.RowsAffected, res.Error
}

// DeleteDoneDispatchesBatch 批量删除超过 cutoff 的 done 记录。
func DeleteDoneDispatchesBatch(cutoff int64, batchSize int) (int64, error) {
	d := getDB()
	if d == nil {
		return 0, errDBNotSet
	}
	var ids []int64
	err := d.Model(&EventDispatch{}).
		Where("status = ? AND processed_at > 0 AND processed_at < ?", StatusDone, cutoff).
		Order("id").
		Limit(batchSize).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := d.Where("id IN ?", ids).Delete(&EventDispatch{})
	return res.RowsAffected, res.Error
}

func truncateError(s string) string {
	const max = 4000
	if len(s) > max {
		return s[:max]
	}
	return s
}
