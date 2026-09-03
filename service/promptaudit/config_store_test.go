package promptaudit

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGormConfigStore_LoadAndSaveCAS(t *testing.T) {
	truncateTables(t)
	store := NewGormConfigStore()
	ctx := context.Background()

	// 1. Initial Load when Option table is empty -> returns nil, nil
	cfg, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Nil(t, cfg)

	// 2. Initial Save with expectedVersion = 0
	initialCfg := DefaultConfig()
	initialCfg.Enabled = false
	err = store.Save(ctx, &initialCfg, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), initialCfg.ConfigVersion)

	// 3. Load saved config
	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, int64(1), loaded.ConfigVersion)
	assert.False(t, loaded.Enabled)

	// 4. Save with conflict version (e.g. expectedVersion = 99)
	err = store.Save(ctx, loaded, 99)
	require.Error(t, err)
	var guardErr *GuardError
	require.True(t, errors.As(err, &guardErr))
	assert.Equal(t, ErrorCodeConfigConflict, guardErr.Code)
	assert.Equal(t, http.StatusConflict, guardErr.HTTPStatus)

	// 5. Successful CAS update with version 1 -> 2
	loaded.Enabled = false
	err = store.Save(ctx, loaded, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), loaded.ConfigVersion)

	loaded2, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), loaded2.ConfigVersion)
}
