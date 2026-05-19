package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestProcessDispatchSuccessMarksDone 单次成功 → status=done。
func TestProcessDispatchSuccessMarksDone(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	sub := &testSubscriber{name: "ok", topics: []string{"*"}}
	Register(sub)

	if err := PublishNoTx("user.registered", 1, "{}"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	ids, _ := pendingDispatchIDs(time.Now().Unix(), 10)
	if len(ids) != 1 {
		t.Fatalf("want 1 pending, got %d", len(ids))
	}
	processDispatch(context.Background(), ids[0], WorkerOpts{
		WorkerId:      "w1",
		HandleTimeout: time.Second,
	})

	d, _ := getDispatchById(ids[0])
	if d.Status != StatusDone {
		t.Fatalf("want done, got %s (err=%s)", d.Status, d.LastError)
	}
	if d.ProcessedAt == 0 {
		t.Fatalf("processed_at not set")
	}
	if sub.calls.Load() != 1 {
		t.Fatalf("subscriber should be called once, got %d", sub.calls.Load())
	}
}

// TestProcessDispatchPermanentErrorMarksDead 返回 ErrPermanent → dead，不重试。
func TestProcessDispatchPermanentErrorMarksDead(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	sub := &testSubscriber{name: "perm", topics: []string{"*"}, err: ErrPermanent}
	Register(sub)
	_ = PublishNoTx("user.registered", 1, "{}")
	ids, _ := pendingDispatchIDs(time.Now().Unix(), 10)
	processDispatch(context.Background(), ids[0], WorkerOpts{WorkerId: "w1", HandleTimeout: time.Second})
	d, _ := getDispatchById(ids[0])
	if d.Status != StatusDead {
		t.Fatalf("want dead, got %s", d.Status)
	}
}

// TestProcessDispatchTemporaryErrorReschedules 临时错误 → status 回到 pending、retry_count 累计、next_retry_at 推迟。
func TestProcessDispatchTemporaryErrorReschedules(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	sub := &testSubscriber{name: "temp", topics: []string{"*"}, err: errors.New("transient")}
	Register(sub)
	_ = PublishNoTx("user.registered", 1, "{}")
	ids, _ := pendingDispatchIDs(time.Now().Unix(), 10)
	before := time.Now().Unix()
	processDispatch(context.Background(), ids[0], WorkerOpts{WorkerId: "w1", HandleTimeout: time.Second})
	d, _ := getDispatchById(ids[0])
	if d.Status != StatusPending {
		t.Fatalf("want pending, got %s", d.Status)
	}
	if d.RetryCount != 1 {
		t.Fatalf("want retry_count=1, got %d", d.RetryCount)
	}
	if d.NextRetryAt < before+int64(retrySchedule[0].Seconds())-1 {
		t.Fatalf("next_retry_at should be ~30s in future, got %d (before=%d)", d.NextRetryAt, before)
	}
}

// TestProcessDispatchExhaustsRetriesToDead retry_count 达到 maxRetries → dead。
func TestProcessDispatchExhaustsRetriesToDead(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	sub := &testSubscriber{name: "always-fail", topics: []string{"*"}, err: errors.New("nope")}
	Register(sub)
	_ = PublishNoTx("user.registered", 1, "{}")
	ids, _ := pendingDispatchIDs(time.Now().Unix(), 10)
	id := ids[0]

	// 手动把 retry_count 拉到 maxRetries - 1，下一次失败就应进 dead
	if err := getDB().Model(&EventDispatch{}).Where("id = ?", id).
		Update("retry_count", maxRetries-1).Error; err != nil {
		t.Fatalf("update retry_count: %v", err)
	}
	processDispatch(context.Background(), id, WorkerOpts{WorkerId: "w1", HandleTimeout: time.Second})
	d, _ := getDispatchById(id)
	if d.Status != StatusDead {
		t.Fatalf("want dead, got %s (retry=%d)", d.Status, d.RetryCount)
	}
}

// TestUnknownSubscriberMarksDead 订阅者不在 registry → dead，不会无限循环。
func TestUnknownSubscriberMarksDead(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	Register(&testSubscriber{name: "ghost", topics: []string{"*"}})
	_ = PublishNoTx("user.registered", 1, "{}")
	ids, _ := pendingDispatchIDs(time.Now().Unix(), 10)

	// 模拟订阅者被移除：重置 registry
	resetRegistryForTest()

	processDispatch(context.Background(), ids[0], WorkerOpts{WorkerId: "w1", HandleTimeout: time.Second})
	d, _ := getDispatchById(ids[0])
	if d.Status != StatusDead {
		t.Fatalf("want dead, got %s", d.Status)
	}
}

// TestConcurrentClaimsAreMutuallyExclusive 验证乐观 claim：同一行被多个 goroutine 抢，只有一个成功。
func TestConcurrentClaimsAreMutuallyExclusive(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	// 直接写入一行 pending dispatch（不经 publish，因为我们只测 claim 行为）
	d := &EventDispatch{
		EventId:    1,
		Subscriber: "irrelevant",
		Status:     StatusPending,
		CreatedAt:  time.Now().Unix(),
		UpdatedAt:  time.Now().Unix(),
	}
	if err := getDB().Create(d).Error; err != nil {
		t.Fatalf("seed dispatch: %v", err)
	}

	var winners atomic.Int32
	var wg sync.WaitGroup
	const N = 20
	wg.Add(N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			ok, err := claimDispatch(d.Id, fmtWorkerID(i))
			if err != nil {
				t.Errorf("claim err: %v", err)
				return
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("expected exactly 1 claim winner, got %d", winners.Load())
	}
}

func fmtWorkerID(i int) string { return "w-" + string(rune('0'+i%10)) }

// TestReclaimStuckDispatches 验证陈旧 processing 行被回收，新鲜行不受影响。
// 不再限制 worker_id，所以"其他 worker 留下的陈旧行"也能被回收（覆盖另一实例崩溃的场景）。
func TestReclaimStuckDispatches(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	now := time.Now().Unix()
	// 同 worker 的陈旧行：应回收
	staleSelf := &EventDispatch{
		Status: StatusProcessing, WorkerId: "self", Subscriber: "x", EventId: 1,
		CreatedAt: now - 3600, UpdatedAt: now - 600,
	}
	// 同 worker 的新鲜行：不应回收
	fresh := &EventDispatch{
		Status: StatusProcessing, WorkerId: "self", Subscriber: "x", EventId: 2,
		CreatedAt: now, UpdatedAt: now,
	}
	// 其他 worker 的陈旧行：应回收（这一条与原设计不同 —— 见 dao.go 中的注释）
	staleOther := &EventDispatch{
		Status: StatusProcessing, WorkerId: "other-crashed", Subscriber: "x", EventId: 3,
		CreatedAt: now - 3600, UpdatedAt: now - 600,
	}
	// 其他 worker 的新鲜行（其他健康实例正在处理）：不应回收
	freshOther := &EventDispatch{
		Status: StatusProcessing, WorkerId: "other-healthy", Subscriber: "x", EventId: 4,
		CreatedAt: now, UpdatedAt: now,
	}
	getDB().Create(staleSelf)
	getDB().Create(fresh)
	getDB().Create(staleOther)
	getDB().Create(freshOther)

	n, err := reclaimStuckDispatches(now - 300) // 5 分钟阈值
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 reclaimed (both stale rows), got %d", n)
	}
	if d, _ := getDispatchById(staleSelf.Id); d.Status != StatusPending {
		t.Fatalf("staleSelf not pending: %s", d.Status)
	}
	if d, _ := getDispatchById(staleOther.Id); d.Status != StatusPending {
		t.Fatalf("staleOther not pending: %s", d.Status)
	}
	if d, _ := getDispatchById(fresh.Id); d.Status != StatusProcessing {
		t.Fatalf("fresh wrongly reclaimed: %s", d.Status)
	}
	if d, _ := getDispatchById(freshOther.Id); d.Status != StatusProcessing {
		t.Fatalf("healthy other-worker row wrongly reclaimed: %s", d.Status)
	}
}

// TestFinalizeIgnoredAfterReclaim 验证 worker A 处理慢被 reclaim、worker B 处理完之后，
// A 才慢悠悠返回 OK/Dead 时，不会污染已经被 B 改完状态的行。
func TestFinalizeIgnoredAfterReclaim(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	now := time.Now().Unix()
	d := &EventDispatch{
		Status: StatusPending, WorkerId: "", Subscriber: "x", EventId: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	getDB().Create(d)

	// A claim
	ok, _ := claimDispatch(d.Id, "A")
	if !ok {
		t.Fatal("A claim failed")
	}
	// 模拟 A 处理期间 reclaim 跑过：把 A 的 row 强制设回 pending（模拟 reclaim 效果）
	getDB().Model(&EventDispatch{}).Where("id = ?", d.Id).Updates(map[string]any{
		"status": StatusPending, "worker_id": "",
	})
	// B claim
	ok, _ = claimDispatch(d.Id, "B")
	if !ok {
		t.Fatal("B claim failed")
	}
	// 此时 A 才返回，调 finishDispatchOK(id, "A")。应是 no-op。
	if err := finishDispatchOK(d.Id, "A"); err != nil {
		t.Fatalf("A finishOK err: %v", err)
	}
	got, _ := getDispatchById(d.Id)
	if got.Status != StatusProcessing {
		t.Fatalf("expected still processing (owned by B), got %s", got.Status)
	}
	if got.WorkerId != "B" {
		t.Fatalf("expected worker_id=B, got %s", got.WorkerId)
	}
	// B 正常 finishOK 才生效
	if err := finishDispatchOK(d.Id, "B"); err != nil {
		t.Fatalf("B finishOK err: %v", err)
	}
	got, _ = getDispatchById(d.Id)
	if got.Status != StatusDone {
		t.Fatalf("expected done after B finish, got %s", got.Status)
	}
}

// TestDeleteDoneDispatchesBatch 验证 done 行清理：30 天前的 done 被删，新 done 和 dead 保留。
func TestDeleteDoneDispatchesBatch(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	now := time.Now().Unix()
	old := &EventDispatch{Status: StatusDone, Subscriber: "s", EventId: 1, ProcessedAt: now - 86400*40, CreatedAt: now - 86400*40, UpdatedAt: now - 86400*40}
	recent := &EventDispatch{Status: StatusDone, Subscriber: "s", EventId: 2, ProcessedAt: now - 86400*5, CreatedAt: now - 86400*5, UpdatedAt: now - 86400*5}
	dead := &EventDispatch{Status: StatusDead, Subscriber: "s", EventId: 3, CreatedAt: now - 86400*40, UpdatedAt: now - 86400*40}
	getDB().Create(old)
	getDB().Create(recent)
	getDB().Create(dead)

	cutoff := now - 86400*30
	deleted, err := DeleteDoneDispatchesBatch(cutoff, 100)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	var n int64
	getDB().Model(&EventDispatch{}).Count(&n)
	if n != 2 {
		t.Fatalf("expected 2 remaining (recent done + dead), got %d", n)
	}
}

// TestDeleteEventLogsBatchSkipsRowsWithActiveDispatch 验证 event_log 不会删除还在被引用的行。
func TestDeleteEventLogsBatchSkipsRowsWithActiveDispatch(t *testing.T) {
	teardown := setupTestDB(t)
	defer teardown()

	now := time.Now().Unix()
	// event_log A：很老，所有 dispatch 都是 done → 可删
	logA := &EventLog{EventType: "x", Payload: "{}", PublishedAt: now - 86400*400}
	logB := &EventLog{EventType: "x", Payload: "{}", PublishedAt: now - 86400*400} // 老，但还有 dead dispatch → 不可删
	logC := &EventLog{EventType: "x", Payload: "{}", PublishedAt: now - 86400*5}   // 新 → 不可删
	getDB().Create(logA)
	getDB().Create(logB)
	getDB().Create(logC)

	getDB().Create(&EventDispatch{EventId: logA.Id, Subscriber: "s", Status: StatusDone, ProcessedAt: now - 86400*399, CreatedAt: now - 86400*400, UpdatedAt: now - 86400*399})
	getDB().Create(&EventDispatch{EventId: logB.Id, Subscriber: "s", Status: StatusDead, CreatedAt: now - 86400*400, UpdatedAt: now - 86400*399})
	getDB().Create(&EventDispatch{EventId: logC.Id, Subscriber: "s", Status: StatusPending, CreatedAt: now - 86400*5, UpdatedAt: now - 86400*5})

	cutoff := now - 86400*365
	deleted, err := DeleteEventLogsBatch(cutoff, 100)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 event_log deleted (A only), got %d", deleted)
	}
	var n int64
	getDB().Model(&EventLog{}).Count(&n)
	if n != 2 {
		t.Fatalf("expected 2 remaining event_log (B,C), got %d", n)
	}
}
