package events

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建一个临时 SQLite 文件作为测试 DB，迁移事件表，注入到 events 包。
// 返回清理函数，会重置 registry 并断开 DB。
func setupTestDB(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "events_test.db")
	d, err := gorm.Open(sqlite.Open(dbFile+"?cache=shared&_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	SetDB(d)
	if err := AutoMigrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	resetRegistryForTest()
	return func() {
		sqlDB, _ := d.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		SetDB(nil)
		resetRegistryForTest()
		_ = os.Remove(dbFile)
	}
}
