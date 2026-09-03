package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptAuditOption_GenericUpdateForbidden(t *testing.T) {
	// Direct validateOptionValue check
	err := validateOptionValue(OptionKeyPromptAuditConfigSecret, `{"enabled":true}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")

	// UpdateOption must fail and not create DB record
	err = UpdateOption(OptionKeyPromptAuditConfigSecret, `{"enabled":true}`)
	require.Error(t, err)

	var opt Option
	err = DB.Where(commonKeyCol+" = ?", OptionKeyPromptAuditConfigSecret).First(&opt).Error
	assert.Error(t, err)

	// UpdateOptionsBulk must also reject
	err = UpdateOptionsBulk(map[string]string{
		OptionKeyPromptAuditConfigSecret: `{"enabled":true}`,
	})
	require.Error(t, err)
}

func TestPromptAuditOption_CASInitialCreation(t *testing.T) {
	t.Cleanup(func() {
		DB.Exec("DELETE FROM options WHERE "+commonKeyCol+" = ?", OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Unlock()
	})

	ctx := context.Background()

	// Initial creation with wrong expectedVersion (e.g., 2) must fail
	_, err := SavePromptAuditConfigCAS(ctx, 2, func(currentRaw string, currentVersion int64) (string, int64, error) {
		return `{"enabled":false,"config_version":2}`, 2, nil
	})
	require.ErrorIs(t, err, ErrPromptAuditConfigConflict)

	// Initial creation with expectedVersion = 0 must succeed
	finalVer, err := SavePromptAuditConfigCAS(ctx, 0, func(currentRaw string, currentVersion int64) (string, int64, error) {
		assert.Empty(t, currentRaw)
		assert.Equal(t, int64(0), currentVersion)
		return `{"enabled":false,"config_version":1}`, 1, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), finalVer)

	raw, err := GetPromptAuditConfigRaw(ctx)
	require.NoError(t, err)
	assert.Contains(t, raw, `"config_version":1`)

	// Check in-memory OptionMap
	common.OptionMapRWMutex.RLock()
	mapVal := common.OptionMap[OptionKeyPromptAuditConfigSecret]
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, raw, mapVal)
}

func TestPromptAuditOption_CASUpdateAndConflict(t *testing.T) {
	t.Cleanup(func() {
		DB.Exec("DELETE FROM options WHERE "+commonKeyCol+" = ?", OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Unlock()
	})

	ctx := context.Background()

	// 1. Initial creation
	ver1, err := SavePromptAuditConfigCAS(ctx, 0, func(currentRaw string, currentVersion int64) (string, int64, error) {
		return `{"enabled":false,"config_version":1}`, 1, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), ver1)

	// 2. Conflict: expectedVersion = 99
	_, err = SavePromptAuditConfigCAS(ctx, 99, func(currentRaw string, currentVersion int64) (string, int64, error) {
		return `{"enabled":true,"config_version":100}`, 100, nil
	})
	require.ErrorIs(t, err, ErrPromptAuditConfigConflict)

	// 3. Normal CAS update: expectedVersion = 1 -> 2
	ver2, err := SavePromptAuditConfigCAS(ctx, 1, func(currentRaw string, currentVersion int64) (string, int64, error) {
		assert.Equal(t, int64(1), currentVersion)
		return `{"enabled":true,"config_version":2}`, 2, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), ver2)

	raw, err := GetPromptAuditConfigRaw(ctx)
	require.NoError(t, err)
	assert.Contains(t, raw, `"config_version":2`)
	assert.Contains(t, raw, `"enabled":true`)

	// 4. Stale update with version 1 must now fail
	_, err = SavePromptAuditConfigCAS(ctx, 1, func(currentRaw string, currentVersion int64) (string, int64, error) {
		return `{"enabled":false,"config_version":2}`, 2, nil
	})
	require.ErrorIs(t, err, ErrPromptAuditConfigConflict)
}

func TestPromptAuditOption_ConcurrentCAS(t *testing.T) {
	t.Cleanup(func() {
		DB.Exec("DELETE FROM options WHERE "+commonKeyCol+" = ?", OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, OptionKeyPromptAuditConfigSecret)
		common.OptionMapRWMutex.Unlock()
	})

	ctx := context.Background()

	// Initial version = 1
	_, err := SavePromptAuditConfigCAS(ctx, 0, func(currentRaw string, currentVersion int64) (string, int64, error) {
		return `{"enabled":false,"config_version":1}`, 1, nil
	})
	require.NoError(t, err)

	// Two concurrent workers both trying to update version 1 -> 2
	var wg sync.WaitGroup
	successCount := 0
	conflictCount := 0
	var mu sync.Mutex

	for i := 0; i < 2; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			_, err := SavePromptAuditConfigCAS(ctx, 1, func(currentRaw string, currentVersion int64) (string, int64, error) {
				return fmt.Sprintf(`{"enabled":true,"config_version":2,"worker":%d}`, workerID), 2, nil
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, ErrPromptAuditConfigConflict) {
				conflictCount++
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, successCount, "exactly one worker must succeed")
	assert.Equal(t, 1, conflictCount, "the other worker must get version conflict")
}

func TestPromptAuditOption_SyncHookTriggered(t *testing.T) {
	hookCalled := make(chan struct{}, 1)
	SetPromptAuditConfigSyncHook(func() {
		select {
		case hookCalled <- struct{}{}:
		default:
		}
	})
	t.Cleanup(func() {
		SetPromptAuditConfigSyncHook(nil)
	})

	err := updateOptionMap(OptionKeyPromptAuditConfigSecret, `{"enabled":true}`)
	require.NoError(t, err)

	select {
	case <-hookCalled:
		// Hook successfully called
	case <-time.After(2 * time.Second):
		t.Fatal("prompt audit config sync hook was not triggered")
	}
}
