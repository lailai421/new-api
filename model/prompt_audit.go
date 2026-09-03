package model

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// PromptCiphertext 实现跨数据库的长文本映射。
// 在 MySQL 映射为 LONGTEXT（突破 TEXT 的 64 KiB 上限），在 PostgreSQL 与 SQLite 映射为 TEXT。
// 始终以 string 写入，并在读取时支持 []byte 与 string，确保 pgx simple protocol 与各数据库驱动兼容。
type PromptCiphertext string

func (PromptCiphertext) GormDataType() string {
	return "text"
}

func (PromptCiphertext) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "longtext"
	case "postgres":
		return "text"
	case "sqlite":
		return "text"
	default:
		return "text"
	}
}

func (p PromptCiphertext) Value() (driver.Value, error) {
	return string(p), nil
}

func (p *PromptCiphertext) Scan(value any) error {
	if value == nil {
		*p = ""
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*p = PromptCiphertext(v)
	case string:
		*p = PromptCiphertext(v)
	default:
		return fmt.Errorf("cannot scan %T into PromptCiphertext", value)
	}
	return nil
}

// PromptAuditEvent 是提示词审计事件的 GORM 持久化模型。
// 覆盖父设计中的 34 个审计字段，不建立外键，身份采用快照，且 FullPromptCiphertext 带有 json:"-" 保护。
type PromptAuditEvent struct {
	Id                   int64            `json:"id" gorm:"primaryKey;autoIncrement;index:idx_pae_created_at_id,priority:2"`
	RequestId            string           `json:"request_id" gorm:"column:request_id;size:128;index"`
	UserId               int              `json:"user_id" gorm:"column:user_id;index"`
	UsernameSnapshot     string           `json:"username_snapshot" gorm:"column:username_snapshot;size:255"`
	UserEmailSnapshot    string           `json:"user_email_snapshot" gorm:"column:user_email_snapshot;size:320"`
	TokenId              int              `json:"token_id" gorm:"column:token_id;index"`
	TokenNameSnapshot    string           `json:"token_name_snapshot" gorm:"column:token_name_snapshot;size:255"`
	Group                string           `json:"group" gorm:"column:group;size:64;index"`
	ChannelId            int              `json:"channel_id" gorm:"column:channel_id"`
	ChannelType          int              `json:"channel_type" gorm:"column:channel_type"`
	RequestPath          string           `json:"request_path" gorm:"column:request_path;size:255"`
	Protocol             string           `json:"protocol" gorm:"column:protocol;size:64"`
	Model                string           `json:"model" gorm:"column:model;size:255"`
	Stage                string           `json:"stage" gorm:"column:stage;size:32"`
	AuditScope           string           `json:"audit_scope" gorm:"column:audit_scope;size:32"`
	PromptHash           string           `json:"prompt_hash" gorm:"column:prompt_hash;size:64;index"`
	RedactedPreview      string           `json:"redacted_preview" gorm:"column:redacted_preview;type:text"`
	FullPromptCiphertext PromptCiphertext `json:"-" gorm:"column:full_prompt_ciphertext"`
	PromptLength         int              `json:"prompt_length" gorm:"column:prompt_length"`
	MessageCount         int              `json:"message_count" gorm:"column:message_count"`
	Decision             string           `json:"decision" gorm:"column:decision;size:32;index"`
	RiskLevel            string           `json:"risk_level" gorm:"column:risk_level;size:32;index"`
	Action               string           `json:"action" gorm:"column:action;size:32"`
	CategoriesJSON       string           `json:"categories_json" gorm:"column:categories_json;type:text"`
	MatchedScannersJSON  string           `json:"matched_scanners_json" gorm:"column:matched_scanners_json;type:text"`
	ScannerScoresJSON    string           `json:"scanner_scores_json" gorm:"column:scanner_scores_json;type:text"`
	ScannerEvidenceJSON  string           `json:"scanner_evidence_json" gorm:"column:scanner_evidence_json;type:text"`
	ScannerBackend       string           `json:"scanner_backend" gorm:"column:scanner_backend;size:64"`
	ScannerVersion       string           `json:"scanner_version" gorm:"column:scanner_version;size:128"`
	GuardEndpointId      string           `json:"guard_endpoint_id" gorm:"column:guard_endpoint_id;size:128;index"`
	PolicyId             string           `json:"policy_id" gorm:"column:policy_id;size:128"`
	PolicyVersion        int              `json:"policy_version" gorm:"column:policy_version"`
	ConfigVersion        int64            `json:"config_version" gorm:"column:config_version"`
	ChunkTotal           int              `json:"chunk_total" gorm:"column:chunk_total"`
	LatencyMS            int64            `json:"latency_ms" gorm:"column:latency_ms"`
	ErrorCode            string           `json:"error_code" gorm:"column:error_code;size:64"`
	CreatedAt            int64            `json:"created_at" gorm:"column:created_at;index:idx_pae_created_at_id,priority:1"`
}

func (PromptAuditEvent) TableName() string {
	return "prompt_audit_events"
}

// CreatePromptAuditEvent 创建一条新的审计事件。
func CreatePromptAuditEvent(ctx context.Context, event *PromptAuditEvent) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().Unix()
	}
	return DB.WithContext(ctx).Create(event).Error
}

// GetPromptAuditEventByID 按主键 ID 查询单条完整事件（包含密文）。
func GetPromptAuditEventByID(ctx context.Context, id int64) (*PromptAuditEvent, error) {
	var event PromptAuditEvent
	err := DB.WithContext(ctx).Where("id = ?", id).First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// DeletePromptAuditEventByID 删除单条审计事件。
func DeletePromptAuditEventByID(ctx context.Context, id int64) error {
	return DB.WithContext(ctx).Where("id = ?", id).Delete(&PromptAuditEvent{}).Error
}

// BatchDeletePromptAuditEvents 按明确 ID 列表批量删除事件，最多允许 500 条。
func BatchDeletePromptAuditEvents(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	idSet := make(map[int64]struct{}, len(ids))
	cleanIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := idSet[id]; !exists {
			idSet[id] = struct{}{}
			cleanIDs = append(cleanIDs, id)
		}
	}
	if len(cleanIDs) == 0 {
		return 0, nil
	}
	if len(cleanIDs) > 500 {
		return 0, errors.New("batch delete exceeds maximum limit of 500 ids")
	}
	res := DB.WithContext(ctx).Where("id IN ?", cleanIDs).Delete(&PromptAuditEvent{})
	return res.RowsAffected, res.Error
}

// PromptAuditEventQueryFilter 包含事件列表查询的多维筛选条件。
type PromptAuditEventQueryFilter struct {
	StartTime       int64
	EndTime         int64
	UserID          int
	TokenID         int
	Group           string
	Model           string
	Protocol        string
	Decision        string
	RiskLevel       string
	Category        string
	GuardEndpointID string
	RequestID       string
	PromptHash      string
	Page            int
	PageSize        int
}

// getPromptAuditListSelectCols 动态构建显式投影列表元数据字段，绝不包含 full_prompt_ciphertext 与 scanner_evidence_json。
// 动态调用确保读取到 initCol() 运行时初始化的 commonGroupCol。
func getPromptAuditListSelectCols() []string {
	groupCol := commonGroupCol
	if groupCol == "" {
		groupCol = "`group`"
	}
	return []string{
		"id",
		"request_id",
		"user_id",
		"username_snapshot",
		"user_email_snapshot",
		"token_id",
		"token_name_snapshot",
		groupCol,
		"channel_id",
		"channel_type",
		"request_path",
		"protocol",
		"model",
		"stage",
		"audit_scope",
		"prompt_hash",
		"redacted_preview",
		"prompt_length",
		"message_count",
		"decision",
		"risk_level",
		"action",
		"categories_json",
		"matched_scanners_json",
		"scanner_scores_json",
		"scanner_backend",
		"scanner_version",
		"guard_endpoint_id",
		"policy_id",
		"policy_version",
		"config_version",
		"chunk_total",
		"latency_ms",
		"error_code",
		"created_at",
	}
}

// GetPromptAuditEvents 分页与筛选查询脱敏事件列表。
func GetPromptAuditEvents(ctx context.Context, filter PromptAuditEventQueryFilter) ([]*PromptAuditEvent, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	} else if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	tx := DB.WithContext(ctx).Model(&PromptAuditEvent{})

	if filter.StartTime > 0 {
		tx = tx.Where("created_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		tx = tx.Where("created_at <= ?", filter.EndTime)
	}
	if filter.UserID > 0 {
		tx = tx.Where("user_id = ?", filter.UserID)
	}
	if filter.TokenID > 0 {
		tx = tx.Where("token_id = ?", filter.TokenID)
	}
	if filter.Group != "" {
		groupCol := commonGroupCol
		if groupCol == "" {
			groupCol = "`group`"
		}
		tx = tx.Where(groupCol+" = ?", filter.Group)
	}
	if filter.Model != "" {
		tx = tx.Where("model = ?", filter.Model)
	}
	if filter.Protocol != "" {
		tx = tx.Where("protocol = ?", filter.Protocol)
	}
	if filter.Decision != "" {
		tx = tx.Where("decision = ?", filter.Decision)
	}
	if filter.RiskLevel != "" {
		tx = tx.Where("risk_level = ?", filter.RiskLevel)
	}
	if filter.Category != "" {
		keyword := "%" + filter.Category + "%"
		tx = tx.Where("categories_json LIKE ? OR matched_scanners_json LIKE ?", keyword, keyword)
	}
	if filter.GuardEndpointID != "" {
		tx = tx.Where("guard_endpoint_id = ?", filter.GuardEndpointID)
	}
	if filter.RequestID != "" {
		tx = tx.Where("request_id = ?", filter.RequestID)
	}
	if filter.PromptHash != "" {
		tx = tx.Where("prompt_hash = ?", filter.PromptHash)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []*PromptAuditEvent
	err := tx.Select(getPromptAuditListSelectCols()).
		Order("created_at DESC, id DESC").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// CleanupPromptAuditEvents 按截止时间有界分批清理历史审计事件。
// batchSize 默认 500（最大 500），maxBatches 默认 10，避免长时间占锁。
func CleanupPromptAuditEvents(ctx context.Context, cutoff int64, batchSize int, maxBatches int) (int64, error) {
	if cutoff <= 0 {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 500
	} else if batchSize > 500 {
		batchSize = 500
	}
	if maxBatches <= 0 {
		maxBatches = 10
	}

	var totalDeleted int64
	for i := 0; i < maxBatches; i++ {
		var ids []int64
		err := DB.WithContext(ctx).Model(&PromptAuditEvent{}).
			Where("created_at < ?", cutoff).
			Order("id ASC").
			Limit(batchSize).
			Pluck("id", &ids).Error
		if err != nil {
			return totalDeleted, err
		}
		if len(ids) == 0 {
			break
		}

		res := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&PromptAuditEvent{})
		if res.Error != nil {
			return totalDeleted, res.Error
		}
		totalDeleted += res.RowsAffected

		if len(ids) < batchSize {
			break
		}
	}

	return totalDeleted, nil
}
