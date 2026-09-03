package promptaudit

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup_RetentionZeroDoesNothing(t *testing.T) {
	ctx := context.Background()

	// retentionDays = 0 means permanent, returns (0, nil) without querying DB
	deleted, err := RunCleanupOnce(ctx, 0, time.Now().Unix(), 500, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)

	// negative retentionDays also returns (0, nil)
	deleted, err = RunCleanupOnce(ctx, -5, time.Now().Unix(), 500, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestCleanup_RetentionPositiveCleansExpiredOnly(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()
	nowUnix := time.Now().Unix()
	cutoff := nowUnix - 3*86400 // 3 days

	// Seed 2 expired events and 1 valid event
	eOld1 := &model.PromptAuditEvent{RequestId: "cleanup-old-1", CreatedAt: cutoff - 50}
	eOld2 := &model.PromptAuditEvent{RequestId: "cleanup-old-2", CreatedAt: cutoff - 10}
	eNew := &model.PromptAuditEvent{RequestId: "cleanup-new-1", CreatedAt: cutoff + 10}

	require.NoError(t, model.CreatePromptAuditEvent(ctx, eOld1))
	require.NoError(t, model.CreatePromptAuditEvent(ctx, eOld2))
	require.NoError(t, model.CreatePromptAuditEvent(ctx, eNew))

	deleted, err := RunCleanupOnce(ctx, 3, nowUnix, 500, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify only eNew remains
	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "cleanup-new-1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, eNew.Id, events[0].Id)
}
