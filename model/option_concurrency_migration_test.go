package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupOptionTestDB(t *testing.T) {
	t.Helper()
	common.UsingSQLite = true
	common.RedisEnabled = false
	prevDB := DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	DB = db
	if err := db.AutoMigrate(&Option{}); err != nil {
		t.Fatalf("迁移 options 表失败: %v", err)
	}
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	// 同包其它测试共享全局 model.DB，这里必须在结束时还原，否则它们会拿到
	// 一个已关闭的连接（表现为 "sql: database is closed"）
	t.Cleanup(func() {
		DB = prevDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// 旧键有值、新键未配置 → 搬到新键并删除旧键
func TestMigrateLegacyConcurrencyOption_MovesValue(t *testing.T) {
	setupOptionTestDB(t)
	ts := operation_setting.GetTokenSetting()
	origNew, origOld := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origNew
		ts.MaxTokenConcurrency = origOld
	})

	ts.MaxTokenConcurrency = 1000
	ts.DefaultUserMaxConcurrency = 0
	DB.Create(&Option{Key: "token_setting.max_token_concurrency", Value: "1000"})

	migrateLegacyConcurrencyOption()

	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 1000 {
		t.Fatalf("新键应为 1000，实际 %d", got)
	}
	var count int64
	DB.Model(&Option{}).Where("key = ?", "token_setting.max_token_concurrency").Count(&count)
	if count != 0 {
		t.Fatal("旧键应被删除")
	}
	var newOpt Option
	if err := DB.Where("key = ?", "token_setting.default_user_max_concurrency").First(&newOpt).Error; err != nil {
		t.Fatalf("新键应已落库: %v", err)
	}
	if newOpt.Value != "1000" {
		t.Fatalf("新键值应为 1000，实际 %q", newOpt.Value)
	}
}

// 新键已被显式配置 → 不覆盖，只清理旧键
func TestMigrateLegacyConcurrencyOption_DoesNotOverwriteNewKey(t *testing.T) {
	setupOptionTestDB(t)
	ts := operation_setting.GetTokenSetting()
	origNew, origOld := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origNew
		ts.MaxTokenConcurrency = origOld
	})

	ts.MaxTokenConcurrency = 1000
	ts.DefaultUserMaxConcurrency = 50 // 管理员已显式配过
	DB.Create(&Option{Key: "token_setting.max_token_concurrency", Value: "1000"})

	migrateLegacyConcurrencyOption()

	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 50 {
		t.Fatalf("已配置的新键不应被旧键覆盖，期望 50，实际 %d", got)
	}
	var count int64
	DB.Model(&Option{}).Where("key = ?", "token_setting.max_token_concurrency").Count(&count)
	if count != 0 {
		t.Fatal("旧键仍应被清理")
	}
}

// 幂等：无旧值时反复调用无副作用（SyncOptions 会周期性触发）
func TestMigrateLegacyConcurrencyOption_Idempotent(t *testing.T) {
	setupOptionTestDB(t)
	ts := operation_setting.GetTokenSetting()
	origNew, origOld := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origNew
		ts.MaxTokenConcurrency = origOld
	})

	ts.MaxTokenConcurrency = 0
	ts.DefaultUserMaxConcurrency = 7
	for i := 0; i < 3; i++ {
		migrateLegacyConcurrencyOption()
	}
	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 7 {
		t.Fatalf("反复调用不应改变新键，期望 7，实际 %d", got)
	}
}

// 回归：json tag 带 ",omitempty" 会让配置系统读不到旧键的值（它用完整 tag
// 字符串匹配键名），迁移就会静默失效 —— 线上表现为管理面板显示 0、旧键一直不消失。
// 这个测试走真实的 updateOptionMap 路径，而不是直接赋值结构体字段。
func TestMigrateLegacyConcurrencyOption_ReadsLegacyKeyThroughOptionMap(t *testing.T) {
	setupOptionTestDB(t)
	ts := operation_setting.GetTokenSetting()
	origNew, origOld := ts.DefaultUserMaxConcurrency, ts.MaxTokenConcurrency
	t.Cleanup(func() {
		ts.DefaultUserMaxConcurrency = origNew
		ts.MaxTokenConcurrency = origOld
	})
	ts.DefaultUserMaxConcurrency = 0
	ts.MaxTokenConcurrency = 0

	// 模拟线上：DB 里只有旧键
	DB.Create(&Option{Key: "token_setting.max_token_concurrency", Value: "1000"})

	// 走真实加载路径（updateOptionMap → handleConfigUpdate → UpdateConfigFromMap）
	loadOptionsFromDatabase()

	if got := operation_setting.GetDefaultUserMaxConcurrency(); got != 1000 {
		t.Fatalf("迁移后新键应为 1000，实际 %d（json tag 带 omitempty 时会是 0）", got)
	}
	var count int64
	DB.Model(&Option{}).Where("key = ?", "token_setting.max_token_concurrency").Count(&count)
	if count != 0 {
		t.Fatal("旧键应已被删除")
	}
}
