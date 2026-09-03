package promptaudit

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.OptionMap = make(map[string]string)
	common.CryptoSecret = "test-crypto-secret-key-at-least-32-bytes"
	model.InitCol()

	if err := db.AutoMigrate(
		&model.Option{},
		&model.PromptAuditEvent{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

func truncateTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM options")
		model.DB.Exec("DELETE FROM prompt_audit_events")
		common.OptionMapRWMutex.Lock()
		common.OptionMap = make(map[string]string)
		common.OptionMapRWMutex.Unlock()
	})
}
