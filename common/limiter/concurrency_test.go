package limiter

import (
	"sync"
	"testing"
)

// 只配令牌级：占满上限后拒绝
func TestMemory_TokenTierLimit(t *testing.T) {
	tk := TokenConcurrencyKey(9001)
	defer func() {
		for i := 0; i < 3; i++ {
			ReleaseMemoryConcurrencySlots("", tk)
		}
	}()

	for i := 0; i < 3; i++ {
		if out := AcquireMemoryConcurrencySlots("", 0, tk, 3); out != AcquireOK {
			t.Fatalf("第 %d 个槽位应拿到，实际 %v", i+1, out)
		}
	}
	if out := AcquireMemoryConcurrencySlots("", 0, tk, 3); out != AcquireTokenExceeded {
		t.Fatalf("超限应返回 AcquireTokenExceeded，实际 %v", out)
	}
}

// 只配账户级：同一账户下不同令牌共享配额
func TestMemory_UserTierSharedAcrossTokens(t *testing.T) {
	uk := UserConcurrencyKey(9100)
	defer func() {
		for i := 0; i < 2; i++ {
			ReleaseMemoryConcurrencySlots(uk, "")
		}
	}()

	for i := 0; i < 2; i++ {
		if out := AcquireMemoryConcurrencySlots(uk, 2, TokenConcurrencyKey(i), 0); out != AcquireOK {
			t.Fatalf("第 %d 个应拿到", i+1)
		}
	}
	// 全新令牌也应被账户级拦住
	if out := AcquireMemoryConcurrencySlots(uk, 2, TokenConcurrencyKey(999), 0); out != AcquireUserExceeded {
		t.Fatalf("账户级超限应返回 AcquireUserExceeded，实际 %v", out)
	}
}

// 关键行为：令牌级失败时账户级槽位必须回滚
func TestMemory_TokenFailureRollsBackUserSlot(t *testing.T) {
	uk := UserConcurrencyKey(9200)
	tk := TokenConcurrencyKey(9201)
	defer ReleaseMemoryConcurrencySlots(uk, tk)

	// 账户级 10（宽松），令牌级 1（占满）
	if out := AcquireMemoryConcurrencySlots(uk, 10, tk, 1); out != AcquireOK {
		t.Fatal("首个请求应通过")
	}
	before := memoryInUse(uk)

	if out := AcquireMemoryConcurrencySlots(uk, 10, tk, 1); out != AcquireTokenExceeded {
		t.Fatal("应被令牌级拒绝")
	}
	if after := memoryInUse(uk); after != before {
		t.Fatalf("令牌级失败后账户级槽位泄漏: 前=%d 后=%d", before, after)
	}
}

// 账户级超限时不应占用令牌级槽位
func TestMemory_UserExceededDoesNotTouchTokenTier(t *testing.T) {
	uk := UserConcurrencyKey(9300)
	tk := TokenConcurrencyKey(9301)
	defer ReleaseMemoryConcurrencySlots(uk, tk)

	AcquireMemoryConcurrencySlots(uk, 1, tk, 5)
	before := memoryInUse(tk)
	if out := AcquireMemoryConcurrencySlots(uk, 1, tk, 5); out != AcquireUserExceeded {
		t.Fatal("应被账户级拒绝")
	}
	if after := memoryInUse(tk); after != before {
		t.Fatalf("账户级超限时不应动令牌级: 前=%d 后=%d", before, after)
	}
}

// 释放后槽位可复用；计数归零时 key 从 map 删除，避免无限增长
func TestMemory_ReleaseAndCleanup(t *testing.T) {
	tk := TokenConcurrencyKey(9400)
	AcquireMemoryConcurrencySlots("", 0, tk, 1)
	if out := AcquireMemoryConcurrencySlots("", 0, tk, 1); out != AcquireTokenExceeded {
		t.Fatal("上限 1 时第二个应被拒绝")
	}
	ReleaseMemoryConcurrencySlots("", tk)
	if out := AcquireMemoryConcurrencySlots("", 0, tk, 1); out != AcquireOK {
		t.Fatal("释放后应能重新拿到")
	}
	ReleaseMemoryConcurrencySlots("", tk)
	if n := memoryInUse(tk); n != 0 {
		t.Fatalf("计数归零后 key 应被删除，实际残留 %d", n)
	}
}

// 并发下放行数必须恰好等于上限，不能超发
func TestMemory_NoOversubscribeUnderRace(t *testing.T) {
	tk := TokenConcurrencyKey(9500)
	const limit = 10
	const goroutines = 200

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if AcquireMemoryConcurrencySlots("", 0, tk, limit) == AcquireOK {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if granted != limit {
		t.Fatalf("并发下应恰好放行 %d 个，实际 %d", limit, granted)
	}
	for i := 0; i < granted; i++ {
		ReleaseMemoryConcurrencySlots("", tk)
	}
}

// 两级并发下都不能超发
func TestMemory_BothTiersNoOversubscribe(t *testing.T) {
	uk := UserConcurrencyKey(9600)
	const userLimit = 6
	const tokenLimit = 4
	const goroutines = 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	// 两个令牌共享账户配额，各自令牌上限 4，账户上限 6
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		tk := TokenConcurrencyKey(9601 + i%2)
		go func() {
			defer wg.Done()
			if AcquireMemoryConcurrencySlots(uk, userLimit, tk, tokenLimit) == AcquireOK {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// 账户级 6 是更紧的约束（两个令牌各 4 合计 8 > 6）
	if granted != userLimit {
		t.Fatalf("应恰好放行账户级上限 %d 个，实际 %d", userLimit, granted)
	}
	if n := memoryInUse(uk); n != userLimit {
		t.Fatalf("账户级计数应为 %d，实际 %d", userLimit, n)
	}
}

// 释放不存在的 key 不应 panic
func TestMemory_ReleaseUnknownIsSafe(t *testing.T) {
	ReleaseMemoryConcurrencySlots(UserConcurrencyKey(9700), TokenConcurrencyKey(9701))
	ReleaseMemoryConcurrencySlots("", "")
}

// memoryInUse 读当前进程内计数，仅测试用
func memoryInUse(key string) int {
	sh := shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.inUse[key]
}
