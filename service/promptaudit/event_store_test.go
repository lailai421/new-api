package promptaudit

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFailingEncryptor struct {
	Encryptor
}

func (m *mockFailingEncryptor) EncryptPrompt(plaintext string) (string, error) {
	return "", errors.New("mock encrypt prompt error")
}

func TestGormEventStore_StoreRulesAndSkipPass(t *testing.T) {
	truncateTables(t)
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)

	store := NewGormEventStore(encryptor)
	ctx := context.Background()

	snapshot := PromptSnapshot{
		RequestID:    "req-store-rule-1",
		UserID:       1,
		Group:        "default",
		FullPrompt:   "sensitive prompt text",
		ScanText:     "scan text",
		PromptHash:   "hash-1",
		PromptLength: 21,
	}

	// 1. storePassEvents = false: Allow & Flag must NOT record and return nil
	err = store.Record(ctx, snapshot, &Decision{Kind: DecisionAllow, HTTPStatus: 200}, false)
	require.NoError(t, err)
	events, _, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-store-rule-1"})
	require.NoError(t, err)
	assert.Empty(t, events, "Allow event should not be recorded when storePassEvents=false")

	err = store.Record(ctx, snapshot, &Decision{Kind: DecisionFlag, HTTPStatus: 200}, false)
	require.NoError(t, err)
	events, _, err = model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-store-rule-1"})
	require.NoError(t, err)
	assert.Empty(t, events, "Flag event should not be recorded when storePassEvents=false")

	// 2. storePassEvents = false: Block must record
	snapshotBlock := snapshot
	snapshotBlock.RequestID = "req-store-rule-block"
	err = store.Record(ctx, snapshotBlock, &Decision{
		Kind:       DecisionBlock,
		HTTPStatus: 403,
		ErrorCode:  ErrorCodeBlocked,
		Result: &NormalizedResult{
			RiskLevel:  RiskCritical,
			Action:     ActionBlock,
			Categories: []string{"jailbreak"},
		},
	}, false)
	require.NoError(t, err)

	events, _, err = model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-store-rule-block"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "block", events[0].Decision)
	assert.Equal(t, "critical", events[0].RiskLevel)

	// Verify encryption and no plaintext in DB
	fullEvent, err := model.GetPromptAuditEventByID(ctx, events[0].Id)
	require.NoError(t, err)
	assert.NotEmpty(t, fullEvent.FullPromptCiphertext)
	assert.NotEqual(t, "sensitive prompt text", string(fullEvent.FullPromptCiphertext))

	decrypted, err := encryptor.DecryptPrompt(string(fullEvent.FullPromptCiphertext))
	require.NoError(t, err)
	assert.Equal(t, "sensitive prompt text", decrypted)

	// 3. storePassEvents = true: Allow must record
	snapshotAllow := snapshot
	snapshotAllow.RequestID = "req-store-rule-allow"
	err = store.Record(ctx, snapshotAllow, &Decision{
		Kind:       DecisionAllow,
		HTTPStatus: 200,
		Result: &NormalizedResult{
			RiskLevel: RiskLow,
			Action:    ActionAllow,
		},
	}, true)
	require.NoError(t, err)

	events, _, err = model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-store-rule-allow"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "allow", events[0].Decision)
}

func TestGormEventStore_EncryptionFailureReturnsRecordFailed(t *testing.T) {
	failingStore := NewGormEventStore(&mockFailingEncryptor{})
	ctx := context.Background()

	snapshot := PromptSnapshot{
		RequestID:  "req-enc-fail",
		FullPrompt: "text",
	}

	err := failingStore.Record(ctx, snapshot, &Decision{Kind: DecisionBlock, HTTPStatus: 403}, true)
	require.Error(t, err)

	var guardErr *GuardError
	require.True(t, errors.As(err, &guardErr))
	assert.Equal(t, ErrorCodeRecordFailed, guardErr.Code)
	assert.Equal(t, 503, guardErr.HTTPStatus)
}

func TestGormEventStore_NilDecisionAndNilResultHandledSafely(t *testing.T) {
	truncateTables(t)
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)

	store := NewGormEventStore(encryptor)
	ctx := context.Background()

	snapshot := PromptSnapshot{
		RequestID:  "req-nil-dec",
		FullPrompt: "some text",
	}

	// nil decision
	err = store.Record(ctx, snapshot, nil, false)
	require.NoError(t, err)

	// decision with nil result
	snapshot2 := PromptSnapshot{
		RequestID:  "req-nil-res",
		FullPrompt: "some text 2",
	}
	err = store.Record(ctx, snapshot2, &Decision{Kind: DecisionUnavailable, HTTPStatus: 503, ErrorCode: ErrorCodeUnavailable}, false)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-nil-res"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, ErrorCodeUnavailable, events[0].ErrorCode)
}
