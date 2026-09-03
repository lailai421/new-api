package promptaudit

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConfigStore struct {
	mu  sync.Mutex
	cfg *Config
	err error
}

func (m *mockConfigStore) Load(ctx context.Context) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if m.cfg == nil {
		return nil, nil
	}
	clone := *m.cfg
	return &clone, nil
}

func (m *mockConfigStore) Save(ctx context.Context, cfg *Config, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg != nil && m.cfg.ConfigVersion != expectedVersion {
		return errors.New("version conflict")
	}
	clone := *cfg
	m.cfg = &clone
	return nil
}

func TestManager_InitialState(t *testing.T) {
	store := &mockConfigStore{}
	encryptor, err := NewAESGCMEncryptor("test-secret-123456789012345678")
	require.NoError(t, err)

	mgr := NewManager(store, encryptor)
	require.NotNil(t, mgr)

	assert.False(t, mgr.IsDegraded())
	state := mgr.RuntimeState()
	assert.Equal(t, ModeOff, state.Mode)
	assert.False(t, state.Degraded)
	assert.Empty(t, state.ConfigLoadError)
	assert.Equal(t, int64(1), mgr.Active().ConfigVersion)
}

func TestManager_ReloadEmptyConfigIsSafeDefaultOff(t *testing.T) {
	store := &mockConfigStore{cfg: nil}
	encryptor, err := NewAESGCMEncryptor("test-secret-123456789012345678")
	require.NoError(t, err)

	mgr := NewManager(store, encryptor)
	err = mgr.Reload(context.Background())
	require.NoError(t, err)

	assert.False(t, mgr.IsDegraded())
	assert.Equal(t, ModeOff, mgr.RuntimeState().Mode)
}

func TestManager_ReloadValidConfig(t *testing.T) {
	orig := os.Getenv("CRYPTO_SECRET")
	_ = os.Setenv("CRYPTO_SECRET", "test-stable-secret")
	defer func() { _ = os.Setenv("CRYPTO_SECRET", orig) }()

	encryptor, err := NewAESGCMEncryptor("test-stable-secret")
	require.NoError(t, err)

	validCfg := DefaultConfig()
	validCfg.Enabled = true
	validCfg.ConfigVersion = 42
	validCfg.Endpoints = []Endpoint{
		{
			ID:      "node-1",
			Name:    "Node 1",
			BaseURL: "http://127.0.0.1:8000",
			Enabled: true,
		},
	}

	store := &mockConfigStore{cfg: &validCfg}
	mgr := NewManager(store, encryptor)

	err = mgr.Reload(context.Background())
	require.NoError(t, err)

	assert.False(t, mgr.IsDegraded())
	state := mgr.RuntimeState()
	assert.Equal(t, ModeBlocking, state.Mode)
	assert.Equal(t, int64(42), state.ActiveConfigVersion)
	assert.Equal(t, int64(42), state.ExpectedConfigVersion)
	assert.Equal(t, 1, state.EnabledEndpoints)
	assert.NotNil(t, state.ConfigLoadedAt)
}

func TestManager_DegradedStateOnInvalidConfig(t *testing.T) {
	orig := os.Getenv("CRYPTO_SECRET")
	_ = os.Setenv("CRYPTO_SECRET", "test-stable-secret")
	defer func() { _ = os.Setenv("CRYPTO_SECRET", orig) }()

	encryptor, err := NewAESGCMEncryptor("test-stable-secret")
	require.NoError(t, err)

	// 先激活一个正常配置
	validCfg := DefaultConfig()
	validCfg.Enabled = true
	validCfg.ConfigVersion = 10
	validCfg.Endpoints = []Endpoint{
		{
			ID:      "node-1",
			Name:    "Node 1",
			BaseURL: "http://127.0.0.1:8000",
			Enabled: true,
		},
	}
	store := &mockConfigStore{cfg: &validCfg}
	mgr := NewManager(store, encryptor)
	require.NoError(t, mgr.Reload(context.Background()))
	assert.False(t, mgr.IsDegraded())

	// 更新为损坏配置（开启了审计但没有启用节点）
	badCfg := validCfg
	badCfg.ConfigVersion = 11
	badCfg.Endpoints[0].Enabled = false
	store.cfg = &badCfg

	err = mgr.Reload(context.Background())
	require.Error(t, err)

	// 必须进入 degraded 状态，不得静默退回关闭
	assert.True(t, mgr.IsDegraded())
	state := mgr.RuntimeState()
	assert.True(t, state.Degraded)
	assert.NotEmpty(t, state.ConfigLoadError)
	assert.Equal(t, int64(11), state.ExpectedConfigVersion)
	assert.Equal(t, int64(10), state.ActiveConfigVersion) // 活跃配置保留上一个有效版本

	// 再次更新为正常配置，应自动恢复
	fixedCfg := validCfg
	fixedCfg.ConfigVersion = 12
	fixedCfg.Endpoints[0].Enabled = true
	store.cfg = &fixedCfg

	require.NoError(t, mgr.Reload(context.Background()))
	assert.False(t, mgr.IsDegraded())
	assert.Empty(t, mgr.RuntimeState().ConfigLoadError)
	assert.Equal(t, int64(12), mgr.RuntimeState().ActiveConfigVersion)
}

func TestManager_ConcurrentAccess(t *testing.T) {
	orig := os.Getenv("CRYPTO_SECRET")
	_ = os.Setenv("CRYPTO_SECRET", "test-stable-secret")
	defer func() { _ = os.Setenv("CRYPTO_SECRET", orig) }()

	encryptor, err := NewAESGCMEncryptor("test-stable-secret")
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Endpoints = []Endpoint{
		{ID: "node-1", Name: "Node 1", BaseURL: "http://127.0.0.1:8000", Enabled: true},
	}
	store := &mockConfigStore{cfg: &cfg}
	mgr := NewManager(store, encryptor)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = mgr.Active()
		}()
		go func() {
			defer wg.Done()
			_ = mgr.RuntimeState()
		}()
		go func(version int) {
			defer wg.Done()
			c := cfg
			c.ConfigVersion = int64(version)
			store.cfg = &c
			_ = mgr.Reload(context.Background())
		}(i)
	}
	wg.Wait()
}
