package events

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func countEventLogs(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := getDB().Model(&EventLog{}).Count(&n).Error; err != nil {
		t.Fatalf("count event_log: %v", err)
	}
	return n
}

func countDispatchesForSubscriber(t *testing.T, sub string) int64 {
	t.Helper()
	var n int64
	if err := getDB().Model(&EventDispatch{}).Where("subscriber = ?", sub).Count(&n).Error; err != nil {
		t.Fatalf("count dispatch: %v", err)
	}
	return n
}

func TestPublishWritesEventLogEvenWithoutSubscribers(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := PublishNoTx("user.registered", 42, map[string]any{"hello": "world"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := countEventLogs(t); got != 1 {
		t.Fatalf("expected 1 event_log row, got %d", got)
	}
	var n int64
	getDB().Model(&EventDispatch{}).Count(&n)
	if n != 0 {
		t.Fatalf("expected 0 dispatch rows (no subscribers), got %d", n)
	}
}

func TestPublishFansOutToMatchingSubscribers(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	Register(&testSubscriber{name: "all", topics: []string{"*"}})
	Register(&testSubscriber{name: "userOnly", topics: []string{"user.*"}})
	Register(&testSubscriber{name: "billingOnly", topics: []string{"billing.*"}})

	if err := PublishNoTx("user.registered", 1, "{}"); err != nil {
		t.Fatalf("publish user: %v", err)
	}
	if err := PublishNoTx("billing.topup.succeeded", 1, "{}"); err != nil {
		t.Fatalf("publish billing: %v", err)
	}

	// all 订阅 *，两条事件各得 1 行
	if got := countDispatchesForSubscriber(t, "all"); got != 2 {
		t.Fatalf("subscriber 'all' want 2 rows, got %d", got)
	}
	if got := countDispatchesForSubscriber(t, "userOnly"); got != 1 {
		t.Fatalf("subscriber 'userOnly' want 1 row, got %d", got)
	}
	if got := countDispatchesForSubscriber(t, "billingOnly"); got != 1 {
		t.Fatalf("subscriber 'billingOnly' want 1 row, got %d", got)
	}
}

func TestPublishInTransactionRollsBackEvents(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	Register(&testSubscriber{name: "all", topics: []string{"*"}})

	sentinelErr := errors.New("boom")
	err := getDB().Transaction(func(tx *gorm.DB) error {
		if err := Publish(tx, "user.registered", 1, "{}"); err != nil {
			return err
		}
		// 模拟业务后续失败，整个事务回滚
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if got := countEventLogs(t); got != 0 {
		t.Fatalf("expected 0 event_log after rollback, got %d", got)
	}
	if got := countDispatchesForSubscriber(t, "all"); got != 0 {
		t.Fatalf("expected 0 dispatch after rollback, got %d", got)
	}
}

func TestPublishReturnsErrorWhenDBNotSet(t *testing.T) {
	resetRegistryForTest()
	SetDB(nil)
	if err := PublishNoTx("user.registered", 1, "{}"); !errors.Is(err, errDBNotSet) {
		t.Fatalf("expected errDBNotSet, got %v", err)
	}
}

// testWidget 是 best-effort 隔离测试用的"主业务"表，跟事件子系统完全无关。
type testWidget struct {
	Id   int64 `gorm:"primaryKey;autoIncrement"`
	Name string
}

// TestPublishBestEffortInTxIsolatesFailure 验证 best-effort publish 失败时，
// 外层事务的主业务写入仍能正常 commit（SAVEPOINT 隔离的正确性）。
//
// 这是 Phase 1 "用户无感知" 原则的核心保障：营销事件子系统挂掉绝不能让用户充值失败。
func TestPublishBestEffortInTxIsolatesFailure(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := getDB().AutoMigrate(&testWidget{}); err != nil {
		t.Fatalf("migrate testWidget: %v", err)
	}

	// 故意构造一个 common.Marshal 必然失败的 payload（channel 不能 JSON 序列化），
	// 模拟 publish 在 tx 内出错的场景。
	badPayload := make(chan int)

	err := getDB().Transaction(func(tx *gorm.DB) error {
		// 1) 主业务写入：模拟"用户充值后给 quota 加 100"
		if err := tx.Create(&testWidget{Name: "topup committed"}).Error; err != nil {
			return err
		}
		// 2) best-effort publish 失败：savepoint 应该回滚到这个点之前，但外层事务存活
		PublishBestEffortInTx(tx, "test.event", 1, badPayload)
		// 3) 继续主业务写入：模拟 publish 后还有别的逻辑要跑（验证 tx 没被污染）
		if err := tx.Create(&testWidget{Name: "log written after publish"}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("outer tx unexpectedly failed: %v", err)
	}

	// 主业务两行都应该已 commit
	var count int64
	if err := getDB().Model(&testWidget{}).Count(&count).Error; err != nil {
		t.Fatalf("count widgets: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 widgets committed, got %d", count)
	}
}

// TestPublishBestEffortInTxStillWritesOnSuccess 反向验证：publish 成功的场景，
// 事件行应该跟主业务一起 commit。
func TestPublishBestEffortInTxStillWritesOnSuccess(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	Register(&testSubscriber{name: "all", topics: []string{"*"}})

	err := getDB().Transaction(func(tx *gorm.DB) error {
		PublishBestEffortInTx(tx, "test.event", 42, map[string]string{"k": "v"})
		return nil
	})
	if err != nil {
		t.Fatalf("outer tx failed: %v", err)
	}
	if got := countEventLogs(t); got != 1 {
		t.Fatalf("expected 1 event_log row, got %d", got)
	}
	if got := countDispatchesForSubscriber(t, "all"); got != 1 {
		t.Fatalf("expected 1 dispatch row, got %d", got)
	}
}

// TestPublishBestEffortInTxFallsBackToNoTxWhenTxNil 验证 tx 为 nil 时也能工作。
func TestPublishBestEffortInTxFallsBackToNoTxWhenTxNil(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	Register(&testSubscriber{name: "all", topics: []string{"*"}})

	PublishBestEffortInTx(nil, "test.event", 1, "ok")

	if got := countEventLogs(t); got != 1 {
		t.Fatalf("expected 1 event_log row, got %d", got)
	}
}

// TestPublishBestEffortInTxRollbackOnFailureLeavesNoEventRows 验证 publish 失败时
// 已部分写入的 event_log 行也会被 savepoint 回滚，不会留下垃圾。
func TestPublishBestEffortInTxRollbackOnFailureLeavesNoEventRows(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	if err := getDB().AutoMigrate(&testWidget{}); err != nil {
		t.Fatalf("migrate testWidget: %v", err)
	}
	Register(&testSubscriber{name: "all", topics: []string{"*"}})

	// 删除 event_dispatches 表 → publish 会在写完 event_log、写 dispatch 时失败。
	// savepoint 应把 event_log 的 INSERT 也回滚。
	if err := getDB().Migrator().DropTable(&EventDispatch{}); err != nil {
		t.Fatalf("drop event_dispatches: %v", err)
	}

	err := getDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&testWidget{Name: "main"}).Error; err != nil {
			return err
		}
		PublishBestEffortInTx(tx, "test.event", 1, map[string]string{"ok": "v"})
		return nil
	})
	if err != nil {
		t.Fatalf("outer tx failed: %v", err)
	}

	// 主业务存活
	var widgets int64
	getDB().Model(&testWidget{}).Count(&widgets)
	if widgets != 1 {
		t.Fatalf("expected 1 widget, got %d", widgets)
	}
	// event_log 应该是 0 行（savepoint 回滚把它的 INSERT 也撤掉了）
	var logs int64
	getDB().Model(&EventLog{}).Count(&logs)
	if logs != 0 {
		t.Fatalf("expected 0 event_log rows after savepoint rollback, got %d", logs)
	}
}
