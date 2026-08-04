package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func setupUserCacheRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	origRDB, origEnabled, origSync := common.RDB, common.RedisEnabled, common.SyncFrequency
	common.RDB = redis.NewClient(&redis.Options{Addr: s.Addr()})
	common.RedisEnabled = true
	// 用户缓存的 TTL 取自 SyncFrequency（RedisKeyCacheSeconds）。生产默认 60，
	// 我们的 docker-compose 也显式设成 60。测试里必须复现这个前置条件：TTL 为 0 时
	// RedisHSetField(s) 按设计整体 no-op，测的就不是真实行为了。
	common.SyncFrequency = 60
	t.Cleanup(func() {
		common.RDB.Close()
		common.RDB, common.RedisEnabled, common.SyncFrequency = origRDB, origEnabled, origSync
		s.Close()
	})
	return s
}

// 核心不变量：刷新资料字段绝不能动缓存里的 Quota。
//
// 缓存侧的余额由计费路径用 HINCRBY 维护（cacheDecrUserQuota）。原来的
// updateUserCache 是整对象 HSET，会把调用方读到的旧 Quota 一起写回去，把并发的
// HINCRBY 覆盖掉——DB 层修好了，这一层没修的话用户照样能白嫖（缓存 TTL 内按
// 复原后的余额放行）。
func TestCacheUpdateUserProfileFields_PreservesQuota(t *testing.T) {
	s := setupUserCacheRedis(t)

	user := User{Id: 7, Group: "default", Email: "a@x.com", Status: 1, Username: "u7", Setting: `{"language":"zh-CN"}`}
	// 先按缓存缺失时的正常路径整体回填一次
	if err := updateUserCache(user); err != nil {
		t.Fatalf("回填缓存失败: %v", err)
	}
	if err := common.RedisHSetField(getUserCacheKey(user.Id), "Quota", 1000); err != nil {
		t.Fatalf("写入初始 Quota 失败: %v", err)
	}

	// 计费结算：原子扣减
	if err := cacheDecrUserQuota(user.Id, 300); err != nil {
		t.Fatalf("扣减失败: %v", err)
	}

	// 用户同时改了设置/语言（拿的是 T0 的快照，Quota 字段还是 1000）
	stale := user
	stale.Setting = `{"language":"en"}`
	stale.Group = "vip"
	if err := cacheUpdateUserProfileFields(stale); err != nil {
		t.Fatalf("刷新资料字段失败: %v", err)
	}

	cached, err := cacheGetUserBase(user.Id)
	if err != nil {
		t.Fatalf("读缓存失败: %v", err)
	}
	if cached.Quota != 700 {
		t.Fatalf("缓存余额被回滚：期望 700，实际 %d", cached.Quota)
	}
	if cached.Setting != `{"language":"en"}` || cached.Group != "vip" {
		t.Fatalf("资料字段没写进去：Setting=%q Group=%q", cached.Setting, cached.Group)
	}
	// 其余字段必须原样保留，不能因为改了部分字段就丢
	if cached.Email != "a@x.com" || cached.Status != 1 || cached.Username != "u7" {
		t.Fatalf("其余字段丢失：%+v", cached)
	}
	_ = s
}

// 缓存缺失时必须整体 no-op：绝不能凭空造出一个只有部分字段的残缺 hash。
// 残缺 hash 会被 cacheGetUserBase 当成有效缓存读走，Status=0 直接等于把用户当禁用，
// Quota=0 等于余额清零。
func TestCacheUpdateUserProfileFields_NoopWhenCacheAbsent(t *testing.T) {
	s := setupUserCacheRedis(t)

	user := User{Id: 9, Group: "vip", Email: "b@x.com", Status: 1, Username: "u9", Setting: "{}"}
	if err := cacheUpdateUserProfileFields(user); err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if s.Exists(getUserCacheKey(user.Id)) {
		t.Fatal("缓存不存在时不应创建任何 key")
	}

	if err := cacheSetUserSetting(user.Id, `{"language":"en"}`); err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if err := cacheSetUserGroup(user.Id, "svip"); err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if s.Exists(getUserCacheKey(user.Id)) {
		t.Fatal("单字段刷新同样不应在缓存缺失时创建 key")
	}
}

// 刷新字段不能顺手把 TTL 抹掉（变成永不过期，缓存就再也拿不到 DB 的真值了）。
func TestCacheUpdateUserProfileFields_KeepsTTL(t *testing.T) {
	s := setupUserCacheRedis(t)

	user := User{Id: 11, Group: "default", Email: "c@x.com", Status: 1, Username: "u11", Setting: "{}"}
	if err := updateUserCache(user); err != nil {
		t.Fatalf("回填缓存失败: %v", err)
	}
	key := getUserCacheKey(user.Id)
	before := s.TTL(key)
	if before <= 0 {
		t.Fatalf("前置条件不成立，回填后应有 TTL，实际 %v", before)
	}

	s.FastForward(2 * time.Second)
	user.Group = "vip"
	if err := cacheUpdateUserProfileFields(user); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}

	after := s.TTL(key)
	if after <= 0 {
		t.Fatalf("刷新后 TTL 丢失：%v", after)
	}
	if after > before {
		t.Fatalf("TTL 被意外续期：刷新前 %v，刷新后 %v", before, after)
	}
}

// cacheSetUserSetting / cacheSetUserGroup 同样不得触碰 Quota。
func TestCacheSetSingleField_PreservesQuota(t *testing.T) {
	setupUserCacheRedis(t)

	user := User{Id: 13, Group: "default", Email: "d@x.com", Status: 1, Username: "u13", Setting: "{}"}
	if err := updateUserCache(user); err != nil {
		t.Fatalf("回填缓存失败: %v", err)
	}
	if err := common.RedisHSetField(getUserCacheKey(user.Id), "Quota", 500); err != nil {
		t.Fatalf("写入初始 Quota 失败: %v", err)
	}

	if err := cacheSetUserSetting(user.Id, `{"max_concurrency":3}`); err != nil {
		t.Fatalf("刷新 setting 失败: %v", err)
	}
	if err := cacheSetUserGroup(user.Id, "svip"); err != nil {
		t.Fatalf("刷新 group 失败: %v", err)
	}

	cached, err := cacheGetUserBase(user.Id)
	if err != nil {
		t.Fatalf("读缓存失败: %v", err)
	}
	if cached.Quota != 500 {
		t.Fatalf("余额被动了：期望 500，实际 %d", cached.Quota)
	}
	if cached.Setting != `{"max_concurrency":3}` || cached.Group != "svip" {
		t.Fatalf("字段没写对：%+v", cached)
	}
}
