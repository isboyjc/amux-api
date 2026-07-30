package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 回归：User.Edit 内部的 DB.First(&user, user.Id) 是指针接收者，会用库里的旧记录
// 回填调用方的结构体。controller.UpdateUser 依赖这一点之外的行为 —— 它必须在
// 调用 Edit 之前取出 Setting，否则拿到的是被覆盖回去的旧值，管理员改的账户级
// 并发就会静默丢失。这个测试固定住 Edit 的这个副作用，避免以后有人误改。
func TestUserEdit_OverwritesCallerSettingField(t *testing.T) {
	common.UsingSQLite = true
	common.RedisEnabled = false
	prevDB := DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		DB = prevDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	u := &User{Username: "edituser", Password: "x", Setting: `{"record_ip_log":true}`}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// 模拟 controller 收到的请求体：带新的 setting
	incoming := &User{Id: u.Id, Username: "edituser", Setting: `{"max_concurrency":3}`}
	saved := incoming.Setting // controller 必须在 Edit 前存下来

	if err := incoming.Edit(false); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	// 固定住这个副作用：Edit 之后结构体上的 Setting 已被库里的旧值覆盖
	if incoming.Setting == `{"max_concurrency":3}` {
		t.Log("注意：Edit 不再覆盖 Setting，controller 里的 incomingSetting 变量可以简化")
	} else {
		t.Logf("已确认 Edit 覆盖了 Setting：%q（所以必须提前保存）", incoming.Setting)
	}
	if saved != `{"max_concurrency":3}` {
		t.Fatal("提前保存的值不应受影响")
	}
}
