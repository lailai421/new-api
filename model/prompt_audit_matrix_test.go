package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type dbMatrixTarget struct {
	name         string
	dbType       common.DatabaseType
	dsn          string
	dialector    gorm.Dialector
	expectedType string
}

func getMatrixTargets(t *testing.T) []dbMatrixTarget {
	return []dbMatrixTarget{
		{
			name:   "SQLite-3",
			dbType: common.DatabaseTypeSQLite,
			dialector: sqlite.Open(fmt.Sprintf("file:prompt_audit_matrix_%d?mode=memory&cache=shared", time.Now().UnixNano())),
			expectedType: "text",
		},
		{
			name:   "MySQL-8.0",
			dbType: common.DatabaseTypeMySQL,
			dsn:    "root:root@tcp(127.0.0.1:13306)/new_api_test?charset=utf8mb4&parseTime=True&loc=Local",
			dialector: mysql.Open("root:root@tcp(127.0.0.1:13306)/new_api_test?charset=utf8mb4&parseTime=True&loc=Local"),
			expectedType: "longtext",
		},
		{
			name:   "PostgreSQL-18",
			dbType: common.DatabaseTypePostgreSQL,
			dsn:    "host=127.0.0.1 port=15432 user=postgres password=postgres dbname=new_api_test sslmode=disable",
			dialector: postgres.Open("host=127.0.0.1 port=15432 user=postgres password=postgres dbname=new_api_test sslmode=disable"),
			expectedType: "text",
		},
	}
}

func TestPromptAudit_ThreeDatabaseMatrix(t *testing.T) {
	targets := getMatrixTargets(t)

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		InitCol()
	})

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			db, err := gorm.Open(target.dialector, &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
			})
			if err != nil {
				t.Skipf("skipping %s: cannot open connection: %v", target.name, err)
				return
			}

			sqlDB, err := db.DB()
			if err != nil {
				t.Skipf("skipping %s: cannot get sql.DB: %v", target.name, err)
				return
			}
			if err := sqlDB.Ping(); err != nil {
				t.Skipf("skipping %s: ping failed: %v", target.name, err)
				return
			}

			// Clean up previous runs
			_ = db.Migrator().DropTable(&PromptAuditEvent{}, &Option{})

			// Switch global context to target database
			DB = db
			LOG_DB = db
			common.SetDatabaseTypes(target.dbType, target.dbType)
			common.OptionMap = make(map[string]string)
			InitCol()

			// 1. Initial AutoMigrate (fresh creation)
			require.NoError(t, db.AutoMigrate(&Option{}, &PromptAuditEvent{}), "%s initial migration failed", target.name)

			// 2. Second AutoMigrate (idempotency verification)
			require.NoError(t, db.AutoMigrate(&Option{}, &PromptAuditEvent{}), "%s second migration (idempotency) failed", target.name)

			// 3. Verify actual column data type in database schema
			var actualColType string
			switch target.dbType {
			case common.DatabaseTypeMySQL:
				row := sqlDB.QueryRow(`
					SELECT DATA_TYPE 
					FROM INFORMATION_SCHEMA.COLUMNS 
					WHERE TABLE_SCHEMA = 'new_api_test' AND TABLE_NAME = 'prompt_audit_events' AND COLUMN_NAME = 'full_prompt_ciphertext'
				`)
				require.NoError(t, row.Scan(&actualColType))
				assert.Equal(t, "longtext", strings.ToLower(actualColType), "MySQL column must be longtext")

			case common.DatabaseTypePostgreSQL:
				row := sqlDB.QueryRow(`
					SELECT data_type 
					FROM information_schema.columns 
					WHERE table_name = 'prompt_audit_events' AND column_name = 'full_prompt_ciphertext'
				`)
				require.NoError(t, row.Scan(&actualColType))
				assert.Equal(t, "text", strings.ToLower(actualColType), "PostgreSQL column must be text")

			case common.DatabaseTypeSQLite:
				var cid int
				var colName, colType string
				var notnull, pk int
				var dfltValue interface{}
				rows, err := sqlDB.Query("PRAGMA table_info(prompt_audit_events)")
				require.NoError(t, err)
				defer rows.Close()
				for rows.Next() {
					require.NoError(t, rows.Scan(&cid, &colName, &colType, &notnull, &dfltValue, &pk))
					if colName == "full_prompt_ciphertext" {
						actualColType = colType
						break
					}
				}
				assert.Equal(t, "text", strings.ToLower(actualColType), "SQLite column must be text")
			}

			ctx := context.Background()

			// 4. >64 KiB Ciphertext Round-trip (~72 KiB)
			largePayload := strings.Repeat("A-Secret-Prompt-Segment-1234567890\n", 2000)
			require.Greater(t, len(largePayload), 65536, "Payload must be >64 KiB")

			largeEvent := &PromptAuditEvent{
				RequestId:            fmt.Sprintf("req-%s-large", target.name),
				UserId:               101,
				UsernameSnapshot:     "alice",
				Group:                "vip",
				Model:                "gpt-4o",
				Decision:             "block",
				RiskLevel:            "high",
				RedactedPreview:      largePayload[:50],
				FullPromptCiphertext: PromptCiphertext(largePayload),
				CategoriesJSON:       `["jailbreak"]`,
				MatchedScannersJSON:  `["jailbreak"]`,
				ScannerScoresJSON:    `{"jailbreak":0.99}`,
				ScannerEvidenceJSON:  `{"jailbreak":"matched pattern"}`,
				CreatedAt:            time.Now().Unix() - 500,
			}
			require.NoError(t, CreatePromptAuditEvent(ctx, largeEvent))
			require.NotZero(t, largeEvent.Id)

			retrieved, err := GetPromptAuditEventByID(ctx, largeEvent.Id)
			require.NoError(t, err)
			require.NotNil(t, retrieved)
			assert.Equal(t, string(largeEvent.FullPromptCiphertext), string(retrieved.FullPromptCiphertext), "%s failed byte fidelity on >64KiB ciphertext", target.name)

			// 5. Option CAS Persistence & Concurrency
			// 5a. Initial CAS create
			_, err = SavePromptAuditConfigCAS(ctx, 0, func(currentRaw string, currentVersion int64) (string, int64, error) {
				assert.Equal(t, int64(0), currentVersion)
				return `{"enabled":false,"config_version":1}`, 1, nil
			})
			require.NoError(t, err, "%s initial CAS create failed", target.name)

			// 5b. Update matching expectedVersion
			_, err = SavePromptAuditConfigCAS(ctx, 1, func(currentRaw string, currentVersion int64) (string, int64, error) {
				assert.Equal(t, int64(1), currentVersion)
				return `{"enabled":true,"config_version":2}`, 2, nil
			})
			require.NoError(t, err, "%s CAS update failed", target.name)

			// 5c. Update mismatching expectedVersion -> Conflict
			_, err = SavePromptAuditConfigCAS(ctx, 1, func(currentRaw string, currentVersion int64) (string, int64, error) {
				return `{"enabled":true,"config_version":3}`, 3, nil
			})
			assert.ErrorIs(t, err, ErrPromptAuditConfigConflict, "%s expected version mismatch must return ErrPromptAuditConfigConflict", target.name)

			// 6. Multi-dimensional filtering and projection
			recentEvent := &PromptAuditEvent{
				RequestId:            fmt.Sprintf("req-%s-recent", target.name),
				UserId:               101,
				UsernameSnapshot:     "alice",
				Group:                "vip",
				Model:                "gpt-4o",
				Decision:             "allow",
				RiskLevel:            "low",
				RedactedPreview:      "Recent safe prompt",
				FullPromptCiphertext: PromptCiphertext("recent-secret"),
				CategoriesJSON:       `[]`,
				MatchedScannersJSON:  `[]`,
				ScannerScoresJSON:    `{}`,
				ScannerEvidenceJSON:  `{}`,
				CreatedAt:            time.Now().Unix(),
			}
			require.NoError(t, CreatePromptAuditEvent(ctx, recentEvent))

			filter := PromptAuditEventQueryFilter{
				UserID:   101,
				Group:    "vip",
				Decision: "block",
				Page:     1,
				PageSize: 10,
			}
			items, total, err := GetPromptAuditEvents(ctx, filter)
			require.NoError(t, err)
			assert.Equal(t, int64(1), total)
			require.Len(t, items, 1)
			assert.Equal(t, largeEvent.RequestId, items[0].RequestId)
			// Ensure full ciphertext was projected out
			assert.Empty(t, items[0].FullPromptCiphertext, "%s list query must project out full_prompt_ciphertext", target.name)

			// 7. Bounded Batch Deletion
			deleted, err := BatchDeletePromptAuditEvents(ctx, []int64{largeEvent.Id})
			require.NoError(t, err)
			assert.Equal(t, int64(1), deleted)

			deletedCheck, err := GetPromptAuditEventByID(ctx, largeEvent.Id)
			assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			assert.Nil(t, deletedCheck)

			// 8. Retention Cleanup
			// recentEvent was created just now, let's insert an old event with CreatedAt = now - 10000
			oldEvent := &PromptAuditEvent{
				RequestId:            fmt.Sprintf("req-%s-old", target.name),
				UserId:               202,
				UsernameSnapshot:     "bob",
				Group:                "default",
				Model:                "claude-3-5-sonnet",
				Decision:             "allow",
				RiskLevel:            "low",
				RedactedPreview:      "Old safe prompt",
				FullPromptCiphertext: PromptCiphertext("old-secret"),
				CreatedAt:            time.Now().Unix() - 10000,
			}
			require.NoError(t, CreatePromptAuditEvent(ctx, oldEvent))

			cutoff := time.Now().Unix() - 5000
			cleaned, err := CleanupPromptAuditEvents(ctx, cutoff, 100, 10)
			require.NoError(t, err)
			assert.Equal(t, int64(1), cleaned, "%s cleanup must delete only events before cutoff", target.name)

			oldCheck, err := GetPromptAuditEventByID(ctx, oldEvent.Id)
			assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			assert.Nil(t, oldCheck, "Old event must be cleaned up")

			recentCheck, err := GetPromptAuditEventByID(ctx, recentEvent.Id)
			require.NoError(t, err)
			assert.NotNil(t, recentCheck, "Recent event must survive cleanup")
		})
	}
}

func TestPromptAudit_UpgradeMatrix(t *testing.T) {
	targets := getMatrixTargets(t)

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		InitCol()
	})

	for _, target := range targets {
		t.Run(target.name+"-Upgrade", func(t *testing.T) {
			db, err := gorm.Open(target.dialector, &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
			})
			if err != nil {
				t.Skipf("skipping %s: cannot open connection: %v", target.name, err)
				return
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Skipf("skipping %s: cannot get sql.DB: %v", target.name, err)
				return
			}
			if err := sqlDB.Ping(); err != nil {
				t.Skipf("skipping %s: ping failed: %v", target.name, err)
				return
			}

			DB = db
			LOG_DB = db
			common.SetDatabaseTypes(target.dbType, target.dbType)
			common.OptionMap = make(map[string]string)
			InitCol()

			// 1. Drop prompt_audit_events table to simulate older version database
			_ = db.Migrator().DropTable(&PromptAuditEvent{})

			// Ensure other tables like options exist and have existing data
			_ = db.AutoMigrate(&Option{})
			testOpt := Option{Key: "test_existing_upgrade_key", Value: "existing_upgrade_val"}
			_ = db.Where(getKeyCol()+" = ?", testOpt.Key).Delete(&Option{}).Error
			require.NoError(t, db.Create(&testOpt).Error)

			// 2. Perform upgrade migration: add PromptAuditEvent table
			require.NoError(t, db.AutoMigrate(&Option{}, &PromptAuditEvent{}))

			// 3. Verify existing data preserved
			var checkOpt Option
			require.NoError(t, db.Where(getKeyCol()+" = ?", testOpt.Key).First(&checkOpt).Error)
			assert.Equal(t, "existing_upgrade_val", checkOpt.Value)

			// 4. Second migration (idempotency verification on upgraded DB)
			require.NoError(t, db.AutoMigrate(&Option{}, &PromptAuditEvent{}))

			// 5. Clean up test option
			db.Where(getKeyCol()+" = ?", testOpt.Key).Delete(&Option{})
		})
	}
}

