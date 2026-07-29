package limiter

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { rdb.Close(); s.Close() })
	return s, rdb
}

// 只配令牌级：占满后拒绝，释放后可再占
func TestRedis_TokenTierOnly(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	tk := TokenConcurrencyKey(1)

	for i := 0; i < 3; i++ {
		out, err := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 3, fmt.Sprintf("r%d", i), 600)
		if err != nil || out != AcquireOK {
			t.Fatalf("第 %d 个槽位应拿到: out=%v err=%v", i+1, out, err)
		}
	}
	out, _ := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 3, "overflow", 600)
	if out != AcquireTokenExceeded {
		t.Fatalf("超限应返回 AcquireTokenExceeded，实际 %v", out)
	}
	if err := ReleaseConcurrencySlots(ctx, rdb, "", tk, "r0"); err != nil {
		t.Fatalf("release 报错: %v", err)
	}
	out, _ = AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 3, "fresh", 600)
	if out != AcquireOK {
		t.Fatal("释放后应能重新拿到")
	}
}

// 只配账户级：多个不同令牌共享同一账户配额
func TestRedis_UserTierSharedAcrossTokens(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	uk := UserConcurrencyKey(100)

	// 模拟同一用户的 2 个不同令牌，账户级上限 2
	for i := 0; i < 2; i++ {
		out, _ := AcquireConcurrencySlots(ctx, rdb, uk, 2, TokenConcurrencyKey(i), 0, fmt.Sprintf("r%d", i), 600)
		if out != AcquireOK {
			t.Fatalf("第 %d 个应拿到", i+1)
		}
	}
	// 第 3 个（哪怕是全新令牌）应被账户级拦住
	out, _ := AcquireConcurrencySlots(ctx, rdb, uk, 2, TokenConcurrencyKey(99), 0, "r-new", 600)
	if out != AcquireUserExceeded {
		t.Fatalf("账户级超限应返回 AcquireUserExceeded，实际 %v", out)
	}
}

// 关键行为：令牌级失败时，账户级槽位必须被回滚，不能泄漏
func TestRedis_TokenFailureRollsBackUserSlot(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	uk := UserConcurrencyKey(200)
	tk := TokenConcurrencyKey(201)

	// 账户级上限 10（宽松），令牌级上限 1（先占满）
	out, _ := AcquireConcurrencySlots(ctx, rdb, uk, 10, tk, 1, "first", 600)
	if out != AcquireOK {
		t.Fatal("首个请求应通过")
	}
	userCardBefore, _ := rdb.ZCard(ctx, uk).Result()
	if userCardBefore != 1 {
		t.Fatalf("账户级应有 1 个槽位，实际 %d", userCardBefore)
	}

	// 第二个：账户级有余量，但令牌级已满 → 应被令牌级拒绝
	out, _ = AcquireConcurrencySlots(ctx, rdb, uk, 10, tk, 1, "second", 600)
	if out != AcquireTokenExceeded {
		t.Fatalf("应被令牌级拒绝，实际 %v", out)
	}
	// 账户级计数必须没变 —— 说明回滚生效了
	userCardAfter, _ := rdb.ZCard(ctx, uk).Result()
	if userCardAfter != userCardBefore {
		t.Fatalf("令牌级失败后账户级槽位泄漏: 前=%d 后=%d", userCardBefore, userCardAfter)
	}
	// 且被拒的 member 不应残留在账户级集合里
	score := rdb.ZScore(ctx, uk, "second")
	if score.Err() == nil {
		t.Fatal("被拒请求的 member 不应残留在账户级 ZSET 中")
	}
}

// 账户级超限时完全不碰令牌级 key（所以「两级同时超限」不存在）
func TestRedis_UserExceededDoesNotTouchTokenKey(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	uk := UserConcurrencyKey(300)
	tk := TokenConcurrencyKey(301)

	// 账户级上限 1，先占满
	AcquireConcurrencySlots(ctx, rdb, uk, 1, tk, 5, "a", 600)
	tokenCardBefore, _ := rdb.ZCard(ctx, tk).Result()

	out, _ := AcquireConcurrencySlots(ctx, rdb, uk, 1, tk, 5, "b", 600)
	if out != AcquireUserExceeded {
		t.Fatalf("应被账户级拒绝，实际 %v", out)
	}
	tokenCardAfter, _ := rdb.ZCard(ctx, tk).Result()
	if tokenCardAfter != tokenCardBefore {
		t.Fatalf("账户级超限时不应动令牌级: 前=%d 后=%d", tokenCardBefore, tokenCardAfter)
	}
}

// 泄漏自愈：超过 TTL 的僵尸槽位应被回收
func TestRedis_StaleSlotsReclaimed(t *testing.T) {
	s, rdb := newTestRedis(t)
	ctx := context.Background()
	tk := TokenConcurrencyKey(400)

	for i := 0; i < 2; i++ {
		AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 2, fmt.Sprintf("leaked%d", i), 600)
	}
	if out, _ := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 2, "blocked", 600); out != AcquireTokenExceeded {
		t.Fatal("占满时应被拒绝")
	}
	s.FastForward(601 * 1e9)
	if out, _ := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 2, "fresh", 600); out != AcquireOK {
		t.Fatal("僵尸槽位超过 TTL 后应被回收")
	}
}

// 两级 key 都必须有过期时间，否则活跃 key 永久驻留 Redis
func TestRedis_BothKeysHaveTTL(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	uk, tk := UserConcurrencyKey(500), TokenConcurrencyKey(501)

	AcquireConcurrencySlots(ctx, rdb, uk, 5, tk, 5, "x", 600)
	for name, k := range map[string]string{"账户级": uk, "令牌级": tk} {
		ttl, err := rdb.TTL(ctx, k).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("%s key 必须有过期时间，实际 TTL=%v err=%v", name, ttl, err)
		}
	}
}

// SCRIPT FLUSH（等价于 Redis 重启丢脚本缓存）后仍能工作
func TestRedis_SurvivesScriptFlush(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	tk := TokenConcurrencyKey(600)

	if out, err := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 2, "r1", 600); err != nil || out != AcquireOK {
		t.Fatalf("首次应成功: out=%v err=%v", out, err)
	}
	if err := rdb.ScriptFlush(ctx).Err(); err != nil {
		t.Skipf("miniredis 不支持 SCRIPT FLUSH: %v", err)
	}
	if out, err := AcquireConcurrencySlots(ctx, rdb, "", 0, tk, 2, "r2", 600); err != nil || out != AcquireOK {
		t.Fatalf("SCRIPT FLUSH 后应自动回退到 EVAL: out=%v err=%v", out, err)
	}
}

// release 幂等
func TestRedis_ReleaseIsIdempotent(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	if err := ReleaseConcurrencySlots(ctx, rdb, UserConcurrencyKey(700), TokenConcurrencyKey(701), "never"); err != nil {
		t.Fatalf("释放不存在的 member 不应报错: %v", err)
	}
	if err := ReleaseConcurrencySlots(ctx, rdb, "", "", "x"); err != nil {
		t.Fatalf("两个 key 都为空时应直接返回: %v", err)
	}
}
