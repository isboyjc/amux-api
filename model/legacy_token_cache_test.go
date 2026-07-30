package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

// 上线瞬间的真实场景：Redis 里全是部署前写入的旧格式令牌缓存，
// 那些 hash 里没有 MaxConcurrency 字段。读它们不能报错，且必须得到 0（不限制），
// 否则存量用户的请求会因为读缓存失败而退化成全量查库，甚至被误限流。
func TestLegacyTokenCache_MissingMaxConcurrencyField(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	defer s.Close()

	origRDB, origEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: s.Addr()})
	common.RedisEnabled = true
	defer func() {
		common.RDB.Close()
		common.RDB, common.RedisEnabled = origRDB, origEnabled
	}()

	// 手工写入「旧版本」缓存：字段集合是部署前的，没有 MaxConcurrency
	key := "token:" + common.GenerateHMAC("legacykey")
	s.HSet(key,
		"Id", "42",
		"UserId", "7",
		"Status", "1",
		"Name", "legacy-token",
		"RemainQuota", "1000",
		"UnlimitedQuota", "false",
		"ModelLimitsEnabled", "false",
		"ModelLimits", "",
		"Group", "default",
		"GroupsJSON", "",
		"CrossGroupRetry", "false",
		"ExpiredTime", "-1",
		"CreatedTime", "0",
		"AccessedTime", "0",
		"UsedQuota", "0",
	)

	tok, err := GetTokenByKey("legacykey", false)
	if err != nil {
		t.Fatalf("读旧格式缓存不应报错，实际: %v", err)
	}
	if tok == nil {
		t.Fatal("应返回令牌")
	}
	if tok.MaxConcurrency != 0 {
		t.Fatalf("缺失字段应为 0（不限制），实际 %d", tok.MaxConcurrency)
	}
	if tok.Name != "legacy-token" {
		t.Fatalf("其它字段应正常解析，Name=%q", tok.Name)
	}
	t.Logf("旧格式缓存读取正常：Name=%s MaxConcurrency=%d", tok.Name, tok.MaxConcurrency)
}
