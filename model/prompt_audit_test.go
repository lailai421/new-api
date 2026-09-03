package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

type testNamedDialector struct {
	tests.DummyDialector
	name string
}

func (d testNamedDialector) Name() string {
	return d.name
}

func TestPromptCiphertext_TypeMappingAndScanValue(t *testing.T) {
	var ct PromptCiphertext

	assert.Equal(t, "text", ct.GormDataType())

	// Test GormDBDataType with named dialectors
	mysqlDB, err := gorm.Open(testNamedDialector{name: "mysql"}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "longtext", ct.GormDBDataType(mysqlDB, nil))

	pgDB, err := gorm.Open(testNamedDialector{name: "postgres"}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "text", ct.GormDBDataType(pgDB, nil))

	sqliteDB, err := gorm.Open(testNamedDialector{name: "sqlite"}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, "text", ct.GormDBDataType(sqliteDB, nil))

	// Scan string
	require.NoError(t, ct.Scan("hello-cipher"))
	assert.Equal(t, PromptCiphertext("hello-cipher"), ct)

	// Scan []byte
	require.NoError(t, ct.Scan([]byte("bytes-cipher")))
	assert.Equal(t, PromptCiphertext("bytes-cipher"), ct)

	// Scan nil
	require.NoError(t, ct.Scan(nil))
	assert.Equal(t, PromptCiphertext(""), ct)

	// Value
	val, err := PromptCiphertext("out-cipher").Value()
	require.NoError(t, err)
	assert.Equal(t, "out-cipher", val)
}

func TestPromptAuditEvent_CRUDAndLargeCiphertext(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	// Construct > 64 KiB ciphertext payload (e.g. 70 KiB)
	largeCiphertext := "v1:" + strings.Repeat("A1b2C3d4E5f6G7h8", 4500) // ~72 KiB
	require.Greater(t, len(largeCiphertext), 65536)

	now := time.Now().Unix()
	event := &PromptAuditEvent{
		RequestId:            "req-crud-test-1",
		UserId:               1001,
		UsernameSnapshot:     "testuser",
		UserEmailSnapshot:    "test@example.com",
		TokenId:              2001,
		TokenNameSnapshot:    "default-token",
		Group:                "vip",
		ChannelId:            1,
		ChannelType:          1,
		RequestPath:          "/v1/chat/completions",
		Protocol:             "openai_chat",
		Model:                "gpt-4o",
		Stage:                "submit",
		AuditScope:           "full",
		PromptHash:           "hash-crud-1",
		RedactedPreview:      "hello world...",
		FullPromptCiphertext: PromptCiphertext(largeCiphertext),
		PromptLength:         70000,
		MessageCount:         2,
		Decision:             "block",
		RiskLevel:            "critical",
		Action:               "Block",
		CategoriesJSON:       `["jailbreak","violence"]`,
		MatchedScannersJSON:  `["jailbreak"]`,
		ScannerScoresJSON:    `{"jailbreak":0.99}`,
		ScannerEvidenceJSON:  `{"jailbreak":"harmful pattern"}`,
		ScannerBackend:       "qwen3guard",
		ScannerVersion:       "0.6b",
		GuardEndpointId:      "node-1",
		PolicyId:             "policy-default",
		PolicyVersion:        1,
		ConfigVersion:        1,
		ChunkTotal:           1,
		LatencyMS:            45,
		ErrorCode:            "prompt_guard_blocked",
		CreatedAt:            now,
	}

	err := CreatePromptAuditEvent(ctx, event)
	require.NoError(t, err)
	require.Greater(t, event.Id, int64(0))

	// Read by ID
	fetched, err := GetPromptAuditEventByID(ctx, event.Id)
	require.NoError(t, err)
	assert.Equal(t, event.RequestId, fetched.RequestId)
	assert.Equal(t, event.UserId, fetched.UserId)
	assert.Equal(t, event.Group, fetched.Group)
	assert.Equal(t, event.Decision, fetched.Decision)
	// Verify ciphertext is preserved character-by-character without truncation
	assert.Equal(t, len(largeCiphertext), len(fetched.FullPromptCiphertext))
	assert.Equal(t, PromptCiphertext(largeCiphertext), fetched.FullPromptCiphertext)

	// Delete by ID
	err = DeletePromptAuditEventByID(ctx, event.Id)
	require.NoError(t, err)

	_, err = GetPromptAuditEventByID(ctx, event.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestPromptAuditEvent_ListAndFilters(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Seed 3 events
	e1 := &PromptAuditEvent{
		RequestId:            "req-list-1",
		UserId:               101,
		TokenId:              201,
		Group:                "default",
		Model:                "gpt-4o",
		Protocol:             "openai_chat",
		Decision:             "allow",
		RiskLevel:            "low",
		CategoriesJSON:       `["safe"]`,
		MatchedScannersJSON:  `[]`,
		GuardEndpointId:      "node-a",
		PromptHash:           "hash-1",
		RedactedPreview:      "preview 1",
		FullPromptCiphertext: "v1:secret-1",
		CreatedAt:            now - 100,
	}
	e2 := &PromptAuditEvent{
		RequestId:            "req-list-2",
		UserId:               102,
		TokenId:              202,
		Group:                "vip",
		Model:                "claude-3-5-sonnet",
		Protocol:             "anthropic_messages",
		Decision:             "block",
		RiskLevel:            "critical",
		CategoriesJSON:       `["jailbreak"]`,
		MatchedScannersJSON:  `["jailbreak"]`,
		GuardEndpointId:      "node-b",
		PromptHash:           "hash-2",
		RedactedPreview:      "preview 2",
		FullPromptCiphertext: "v1:secret-2",
		CreatedAt:            now - 50,
	}
	e3 := &PromptAuditEvent{
		RequestId:            "req-list-3",
		UserId:               101,
		TokenId:              201,
		Group:                "vip",
		Model:                "gpt-4o",
		Protocol:             "openai_chat",
		Decision:             "flag",
		RiskLevel:            "medium",
		CategoriesJSON:       `["controversial"]`,
		MatchedScannersJSON:  `[]`,
		GuardEndpointId:      "node-a",
		PromptHash:           "hash-3",
		RedactedPreview:      "preview 3",
		FullPromptCiphertext: "v1:secret-3",
		CreatedAt:            now,
	}

	require.NoError(t, CreatePromptAuditEvent(ctx, e1))
	require.NoError(t, CreatePromptAuditEvent(ctx, e2))
	require.NoError(t, CreatePromptAuditEvent(ctx, e3))

	// 1. List all - order should be e3, e2, e1 (created_at DESC, id DESC)
	items, total, err := GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	assert.Equal(t, e3.RequestId, items[0].RequestId)
	assert.Equal(t, e2.RequestId, items[1].RequestId)
	assert.Equal(t, e1.RequestId, items[2].RequestId)

	// In list query, full_prompt_ciphertext MUST be empty
	for _, item := range items {
		assert.Empty(t, item.FullPromptCiphertext, "list projection must omit full_prompt_ciphertext")
	}

	// 2. Filter by UserID
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{UserID: 101})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	// 3. Filter by Group
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{Group: "vip"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	// 4. Filter by Decision
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{Decision: "block"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-list-2", items[0].RequestId)

	// 5. Filter by Category keyword
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{Category: "jailbreak"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-list-2", items[0].RequestId)

	// 6. Filter by RequestID
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{RequestID: "req-list-3"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "req-list-3", items[0].RequestId)

	// 7. Filter by PromptHash
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{PromptHash: "hash-1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	// 8. Pagination
	items, total, err = GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, items, 2)
}

func TestPromptAuditEvent_BatchDelete(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	e1 := &PromptAuditEvent{RequestId: "req-del-1", CreatedAt: time.Now().Unix()}
	e2 := &PromptAuditEvent{RequestId: "req-del-2", CreatedAt: time.Now().Unix()}
	e3 := &PromptAuditEvent{RequestId: "req-del-3", CreatedAt: time.Now().Unix()}
	require.NoError(t, CreatePromptAuditEvent(ctx, e1))
	require.NoError(t, CreatePromptAuditEvent(ctx, e2))
	require.NoError(t, CreatePromptAuditEvent(ctx, e3))

	// Empty IDs
	deleted, err := BatchDeletePromptAuditEvents(ctx, []int64{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)

	// > 500 IDs rejected
	tooMany := make([]int64, 501)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	_, err = BatchDeletePromptAuditEvents(ctx, tooMany)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum limit")

	// Delete e1 and e2 with duplicates
	deleted, err = BatchDeletePromptAuditEvents(ctx, []int64{e1.Id, e2.Id, e1.Id, -1, 0})
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify only e3 remains
	remaining, total, err := GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, e3.Id, remaining[0].Id)
}

func TestPromptAuditEvent_Cleanup(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	baseTime := time.Now().Unix()
	cutoff := baseTime - 7*86400 // 7 days ago

	// e1, e2: older than cutoff
	e1 := &PromptAuditEvent{RequestId: "old-1", CreatedAt: cutoff - 100}
	e2 := &PromptAuditEvent{RequestId: "old-2", CreatedAt: cutoff - 10}
	// e3: exactly cutoff (not < cutoff, should be preserved)
	e3 := &PromptAuditEvent{RequestId: "boundary-3", CreatedAt: cutoff}
	// e4: newer than cutoff
	e4 := &PromptAuditEvent{RequestId: "new-4", CreatedAt: cutoff + 100}

	require.NoError(t, CreatePromptAuditEvent(ctx, e1))
	require.NoError(t, CreatePromptAuditEvent(ctx, e2))
	require.NoError(t, CreatePromptAuditEvent(ctx, e3))
	require.NoError(t, CreatePromptAuditEvent(ctx, e4))

	// Cleanup with small batchSize (1) to test loop
	deleted, err := CleanupPromptAuditEvents(ctx, cutoff, 1, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted, "e1 and e2 must be deleted")

	// Verify remaining
	remaining, total, err := GetPromptAuditEvents(ctx, PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, "new-4", remaining[0].RequestId)
	assert.Equal(t, "boundary-3", remaining[1].RequestId)

	// Idempotent second run
	deletedAgain, err := CleanupPromptAuditEvents(ctx, cutoff, 10, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deletedAgain)
}
