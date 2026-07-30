package middleware

import (
	"testing"
	"time"
)

func tokenKey(id int) concurrencyLogKey { return concurrencyLogKey{isUser: false, id: id} }
func userKey(id int) concurrencyLogKey  { return concurrencyLogKey{isUser: true, id: id} }

func resetConcurrencyLogState() {
	concurrencyLogMu.Lock()
	concurrencyLogStats = make(map[concurrencyLogKey]*concurrencyLogState)
	concurrencyLogLastGC = time.Time{}
	concurrencyLogMu.Unlock()
}

// 首次触发必须记录，紧随其后的重复触发必须被抑制
func TestConcurrencyLog_FirstLogsThenSuppresses(t *testing.T) {
	resetConcurrencyLogState()
	now := time.Now()

	should, suppressed := shouldLogConcurrencyReject(tokenKey(1), now)
	if !should {
		t.Fatal("首次超限必须记录")
	}
	if suppressed != 0 {
		t.Fatalf("首次 suppressed 应为 0，实际 %d", suppressed)
	}

	// 窗口内连打 100 次，都应被抑制
	for i := 0; i < 100; i++ {
		if s, _ := shouldLogConcurrencyReject(tokenKey(1), now.Add(time.Duration(i)*time.Millisecond)); s {
			t.Fatalf("第 %d 次应被抑制", i+1)
		}
	}
	// 过了第一级窗口（30s）后应再记录一次，并带上抑制计数
	should, suppressed = shouldLogConcurrencyReject(tokenKey(1), now.Add(31*time.Second))
	if !should {
		t.Fatal("超过窗口后应记录")
	}
	if suppressed != 100 {
		t.Fatalf("应聚合 100 次抑制，实际 %d", suppressed)
	}
}

// 持续触发时窗口必须逐级拉长：30s → 2min → 8min → 30min
func TestConcurrencyLog_WindowEscalates(t *testing.T) {
	resetConcurrencyLogState()
	base := time.Now()

	// 第 1 条
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), base); !s {
		t.Fatal("首条应记录")
	}
	// 29s 时仍在第一级窗口内 → 抑制
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), base.Add(29*time.Second)); s {
		t.Fatal("29s 时应仍被抑制（窗口 30s）")
	}
	// 31s → 记录第 2 条，窗口升到 2min
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), base.Add(31*time.Second)); !s {
		t.Fatal("31s 时应记录")
	}
	// 距第 2 条 1min（<2min）→ 抑制
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), base.Add(31*time.Second+time.Minute)); s {
		t.Fatal("窗口已升到 2min，1min 时应被抑制")
	}
	// 距第 2 条 2min+ → 记录第 3 条，窗口升到 8min
	t3 := base.Add(31*time.Second + 2*time.Minute + time.Second)
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), t3); !s {
		t.Fatal("超过 2min 应记录")
	}
	// 距第 3 条 5min（<8min）→ 抑制
	if s, _ := shouldLogConcurrencyReject(tokenKey(2), t3.Add(5*time.Minute)); s {
		t.Fatal("窗口已升到 8min，5min 时应被抑制")
	}
}

// 安静一段时间后级别必须重置，否则偶发超限会被长窗口静默
func TestConcurrencyLog_LevelResetsAfterIdle(t *testing.T) {
	resetConcurrencyLogState()
	base := time.Now()

	// 连续触发把级别抬上去
	shouldLogConcurrencyReject(tokenKey(3), base)
	shouldLogConcurrencyReject(tokenKey(3), base.Add(31*time.Second))
	t3 := base.Add(31*time.Second + 2*time.Minute + time.Second)
	shouldLogConcurrencyReject(tokenKey(3), t3)

	concurrencyLogMu.Lock()
	lvl := concurrencyLogStats[tokenKey(3)].level
	concurrencyLogMu.Unlock()
	if lvl == 0 {
		t.Fatal("连续触发后级别应已抬高")
	}

	// 安静很久（超过 当前窗口×2）后再次触发 → 应立即记录
	idleLater := t3.Add(2 * time.Hour)
	should, _ := shouldLogConcurrencyReject(tokenKey(3), idleLater)
	if !should {
		t.Fatal("长时间安静后再次超限应立即记录，而不是继承长窗口")
	}
}

// 不同令牌的节流状态互相独立
func TestConcurrencyLog_PerTokenIsolation(t *testing.T) {
	resetConcurrencyLogState()
	now := time.Now()
	if s, _ := shouldLogConcurrencyReject(tokenKey(10), now); !s {
		t.Fatal("令牌 10 首条应记录")
	}
	if s, _ := shouldLogConcurrencyReject(tokenKey(10), now); s {
		t.Fatal("令牌 10 第二条应被抑制")
	}
	// 另一个令牌不受影响
	if s, _ := shouldLogConcurrencyReject(tokenKey(11), now); !s {
		t.Fatal("令牌 11 首条应记录，不该被令牌 10 的状态影响")
	}
}

// GC 必须清掉长期无活动的条目，防止 map 无限增长
func TestConcurrencyLog_GCEvictsIdleEntries(t *testing.T) {
	resetConcurrencyLogState()
	base := time.Now()
	shouldLogConcurrencyReject(tokenKey(20), base)

	concurrencyLogMu.Lock()
	n := len(concurrencyLogStats)
	concurrencyLogMu.Unlock()
	if n != 1 {
		t.Fatalf("应有 1 个条目，实际 %d", n)
	}

	// 很久之后另一个令牌触发，GC 应清掉 20
	shouldLogConcurrencyReject(tokenKey(21), base.Add(3*time.Hour))

	concurrencyLogMu.Lock()
	_, stillThere := concurrencyLogStats[tokenKey(20)]
	concurrencyLogMu.Unlock()
	if stillThere {
		t.Fatal("长期无活动的令牌 20 应被 GC 清掉")
	}
}

// 账户级与令牌级的节流状态必须独立，否则一级的日志会把另一级抑制掉
func TestConcurrencyLog_TiersDoNotSuppressEachOther(t *testing.T) {
	resetConcurrencyLogState()
	now := time.Now()

	// 令牌级触发并记录
	if s, _ := shouldLogConcurrencyReject(tokenKey(7), now); !s {
		t.Fatal("令牌级首条应记录")
	}
	if s, _ := shouldLogConcurrencyReject(tokenKey(7), now); s {
		t.Fatal("令牌级第二条应被抑制")
	}
	// 同一 id 的账户级不应受影响
	if s, _ := shouldLogConcurrencyReject(userKey(7), now); !s {
		t.Fatal("账户级应独立记录，不该被同 id 的令牌级状态抑制")
	}
}
