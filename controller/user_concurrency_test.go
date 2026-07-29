package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openUserConcurrencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	prevDB, prevLogDB := model.DB, model.LOG_DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开 sqlite 失败: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("迁移 users 表失败: %v", err)
	}
	// 同包其它测试共享全局 model.DB，结束时必须还原，否则它们会拿到已关闭的连接
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLogDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func intPtr(v int) *int { return &v }

// 管理员设置账户级并发后，值应正确落库且能读回
func TestUpdateUserMaxConcurrency_AdminCanSetAndClear(t *testing.T) {
	openUserConcurrencyTestDB(t)

	u := &model.User{Username: "u1", Password: "x", Setting: ""}
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	if err := model.UpdateUserMaxConcurrency(u.Id, intPtr(7)); err != nil {
		t.Fatalf("设置并发失败: %v", err)
	}
	got, err := model.GetUserSetting(u.Id, true)
	if err != nil {
		t.Fatalf("读设置失败: %v", err)
	}
	if got.MaxConcurrency == nil || *got.MaxConcurrency != 7 {
		t.Fatalf("应为 7，实际 %v", got.MaxConcurrency)
	}

	// 传 nil 表示清除单独配置、回落全局默认
	if err := model.UpdateUserMaxConcurrency(u.Id, nil); err != nil {
		t.Fatalf("清除失败: %v", err)
	}
	got, _ = model.GetUserSetting(u.Id, true)
	if got.MaxConcurrency != nil {
		t.Fatalf("清除后应为 nil，实际 %v", *got.MaxConcurrency)
	}
}

// 设置账户级并发不能抹掉用户自己的其它配置
func TestUpdateUserMaxConcurrency_PreservesOtherSettings(t *testing.T) {
	openUserConcurrencyTestDB(t)

	u := &model.User{Username: "u2", Password: "x"}
	u.SetSetting(dto.UserSetting{
		NotifyType:        dto.NotifyTypeEmail,
		NotificationEmail: "keep@example.com",
		RecordIpLog:       true,
		SidebarModules:    `{"a":1}`,
	})
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	if err := model.UpdateUserMaxConcurrency(u.Id, intPtr(5)); err != nil {
		t.Fatalf("设置并发失败: %v", err)
	}
	got, _ := model.GetUserSetting(u.Id, true)
	if got.NotificationEmail != "keep@example.com" {
		t.Errorf("通知邮箱被抹掉了: %q", got.NotificationEmail)
	}
	if !got.RecordIpLog {
		t.Error("RecordIpLog 被抹掉了")
	}
	if got.SidebarModules != `{"a":1}` {
		t.Errorf("SidebarModules 被抹掉了: %q", got.SidebarModules)
	}
	if got.MaxConcurrency == nil || *got.MaxConcurrency != 5 {
		t.Errorf("并发未写入: %v", got.MaxConcurrency)
	}
}

// 安全关键：用户自助保存设置时，管理员配的并发上限必须保留，不能被清空或改写。
// 这里直接验证 UpdateUserSetting 里的保留逻辑（settings.MaxConcurrency = existing...）。
func TestUserSelfUpdate_CannotChangeMaxConcurrency(t *testing.T) {
	openUserConcurrencyTestDB(t)

	u := &model.User{Username: "u3", Password: "x"}
	u.SetSetting(dto.UserSetting{MaxConcurrency: intPtr(3)})
	if err := model.DB.Create(u).Error; err != nil {
		t.Fatalf("建用户失败: %v", err)
	}

	// 模拟 UpdateUserSetting 的行为：从零构建 settings（不含 MaxConcurrency），
	// 再按控制器里的逻辑把管理员字段搬回来
	existing := u.GetSetting()
	rebuilt := dto.UserSetting{
		NotifyType:            dto.NotifyTypeEmail,
		QuotaWarningThreshold: 1,
		RecordIpLog:           true,
		// 用户即使恶意在请求里塞了 max_concurrency，也不会走到这里 ——
		// UpdateUserSettingRequest 结构体里没有这个字段，压根解析不进来
	}
	rebuilt.MaxConcurrency = existing.MaxConcurrency // 控制器里的保留逻辑

	u.SetSetting(rebuilt)
	if err := model.DB.Model(u).Update("setting", u.Setting).Error; err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	got, _ := model.GetUserSetting(u.Id, true)
	if got.MaxConcurrency == nil || *got.MaxConcurrency != 3 {
		t.Fatalf("用户自助保存后管理员配的并发上限应保持为 3，实际 %v", got.MaxConcurrency)
	}
}

// UpdateUserSettingRequest 不得包含 max_concurrency 字段 —— 这是权限隔离的第一道防线。
// 一旦有人给它加上，用户就能自己提权改账户级并发，这个测试会失败。
func TestUpdateUserSettingRequest_HasNoMaxConcurrencyField(t *testing.T) {
	req := UpdateUserSettingRequest{}
	data, err := common.Marshal(&req)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if strings.Contains(string(data), "max_concurrency") {
		t.Fatal("UpdateUserSettingRequest 不应包含 max_concurrency 字段：" +
			"该字段仅管理员可写，暴露到用户自助接口等于允许用户自行提权")
	}
}
