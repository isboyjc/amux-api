package limiter

import (
	"sync"
	"testing"
)

func TestInflight_IncDecRoundTrip(t *testing.T) {
	const tokenId = 90001
	defer resetInflight()

	if got := SnapshotInflight()[tokenId]; got != 0 {
		t.Fatalf("初始应为 0，实际 %d", got)
	}

	IncInflight(tokenId)
	IncInflight(tokenId)
	if got := SnapshotInflight()[tokenId]; got != 2 {
		t.Fatalf("两次 Inc 后应为 2，实际 %d", got)
	}

	DecInflight(tokenId)
	if got := SnapshotInflight()[tokenId]; got != 1 {
		t.Fatalf("一次 Dec 后应为 1，实际 %d", got)
	}

	DecInflight(tokenId)
	// 归零后必须从 map 里消失，否则 map 会随历史令牌数无限增长
	if _, exists := SnapshotInflight()[tokenId]; exists {
		t.Fatal("归零后 key 应被删除")
	}
}

// 非法 id 不应写入任何计数（tokenId<=0 表示上下文里没有令牌身份）。
func TestInflight_IgnoresNonPositiveId(t *testing.T) {
	defer resetInflight()
	IncInflight(0)
	IncInflight(-1)
	if n := len(SnapshotInflight()); n != 0 {
		t.Fatalf("非法 id 不应产生计数，实际有 %d 条", n)
	}
}

// Dec 多于 Inc 时不能出现负数或残留 key —— 中间件里 defer 配对，但防御性检查。
func TestInflight_UnderflowDoesNotGoNegative(t *testing.T) {
	const tokenId = 90002
	defer resetInflight()
	DecInflight(tokenId)
	if _, exists := SnapshotInflight()[tokenId]; exists {
		t.Fatal("未 Inc 就 Dec 不应留下 key")
	}
}

// 快照要覆盖分布在不同分片上的多个令牌。
func TestInflight_SnapshotAcrossShards(t *testing.T) {
	defer resetInflight()
	ids := []int{1, 2, 3, 33, 64, 65, 1000, 99999}
	for _, id := range ids {
		IncInflight(id)
	}
	snapshot := SnapshotInflight()
	if len(snapshot) != len(ids) {
		t.Fatalf("应有 %d 条，实际 %d 条", len(ids), len(snapshot))
	}
	for _, id := range ids {
		if snapshot[id] != 1 {
			t.Errorf("令牌 %d 应为 1，实际 %d", id, snapshot[id])
		}
	}
}

// 并发 Inc/Dec 必须配平回零，且不能 panic（map 并发写会直接崩）。
func TestInflight_ConcurrentIncDecBalances(t *testing.T) {
	defer resetInflight()
	const (
		goroutines = 64
		perG       = 200
		tokenId    = 90003
	)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				IncInflight(tokenId)
				DecInflight(tokenId)
			}
		}()
	}
	wg.Wait()
	if _, exists := SnapshotInflight()[tokenId]; exists {
		t.Fatal("Inc/Dec 完全配对后不应有残留")
	}
}

// 并发只 Inc 时总数必须精确等于调用次数（验证分片锁没有丢更新）。
func TestInflight_ConcurrentIncCountsExactly(t *testing.T) {
	defer resetInflight()
	const (
		goroutines = 32
		perG       = 100
		tokenId    = 90004
	)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				IncInflight(tokenId)
			}
		}()
	}
	wg.Wait()
	if got := SnapshotInflight()[tokenId]; got != goroutines*perG {
		t.Fatalf("应为 %d，实际 %d", goroutines*perG, got)
	}
}

// resetInflight 清空全局分片，避免用例间互相污染。
func resetInflight() {
	for _, sh := range inflightShards {
		sh.mu.Lock()
		sh.tokens = make(map[int]int)
		sh.mu.Unlock()
	}
}
