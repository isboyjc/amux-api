package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// Token 的 Redis 缓存走 RedisHSetObj/RedisHGetObj 的反射序列化，只支持
// string/int/bool/gorm.DeletedAt。这里确认新加的 MaxConcurrency(int) 能完整往返，
// 否则每次读令牌缓存都会失败并退化成全量查库。
func TestTokenCache_MaxConcurrencyRoundTrip(t *testing.T) {
	if !common.RedisEnabled {
		t.Skip("需要 Redis，跳过")
	}
	orig := Token{Id: 1, Key: "k", MaxConcurrency: 7}
	if err := cacheSetToken(orig); err != nil {
		t.Fatalf("写缓存失败: %v", err)
	}
	got, err := cacheGetTokenByKey("k")
	if err != nil {
		t.Fatalf("读缓存失败: %v", err)
	}
	if got.MaxConcurrency != 7 {
		t.Fatalf("MaxConcurrency 往返丢失: 期望 7，实际 %d", got.MaxConcurrency)
	}
}
