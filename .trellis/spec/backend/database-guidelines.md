# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

- **ORM**: GORM v2
- **Supported Databases**: SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6 (all three must be simultaneously supported)
- **Migrations**: GORM `AutoMigrate(&Model{})` inside `model/main.go:migrateDB()`
- **Transactions & Row Locking**: Use `lockForUpdate(tx)` for standard `SELECT ... FOR UPDATE` row locks; never use legacy `tx.Set("gorm:query_option", "FOR UPDATE")`.

---

## 1. Cross-Database Large Text Types (>64 KiB)

MySQL `TEXT` has a hard 64 KiB limit. When a model needs to store large payloads (such as encrypted prompts, large JSON payloads) that must exceed 64 KiB across all three supported databases:
- Define a custom GORM type (e.g. `PromptCiphertext`) implementing `driver.Valuer` and `sql.Scanner`.
- Implement `GormDataType(db *gorm.DB) string`:
  - Dialect `mysql`: return `"longtext"`
  - Other dialects (`postgres`, `sqlite`): return `"text"`
- Tag sensitive raw ciphertext with `json:"-"` to prevent accidental serialization in generic API responses.

---

## 2. Three-Database Matrix Testing & Global State Isolation

When writing multi-database matrix tests that switch `model.DB`, `model.LOG_DB`, and `common.SetDatabaseTypes`:
- **Mandatory Cleanup**: Always capture previous global state before switching, and restore it via `t.Cleanup()`:
  ```go
  previousDB, previousLogDB := DB, LOG_DB
  previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
  t.Cleanup(func() {
      DB, LOG_DB = previousDB, previousLogDB
      common.SetDatabaseTypes(previousMainType, previousLogType)
      InitCol()
  })
  ```
- **Real Instances**: Matrix tests must exercise real SQLite, MySQL (e.g., port 13306), and PostgreSQL (e.g., port 15432) instances, covering fresh creation, idempotency, data fidelity, and upgrade paths.

---

## 3. High-Security Option CAS & Isolation Pattern

For security-critical or sensitive JSON options (e.g. `PromptAuditConfigSecret`):
1. **Filter from Generic GetOptions**: Explicitly omit the key in `controller/option.go:GetOptions`.
2. **Block from Generic UpdateOption**: Intercept and return 403 in `controller/option.go:UpdateOption` and `model/option.go:updateOptionMap`.
3. **CAS Concurrency Control**: In a transaction, lock the row using `lockForUpdate(tx)`, compare `expected_config_version` with current version, and increment on write. Return 409 Conflict if mismatched.

