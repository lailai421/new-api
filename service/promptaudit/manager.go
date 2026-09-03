package promptaudit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RuntimeState 提供管理 API 与运行监控所需要的当前运行时状态。
type RuntimeState struct {
	Mode                  Mode       `json:"mode"`
	ExpectedConfigVersion int64      `json:"expected_config_version"`
	ActiveConfigVersion   int64      `json:"active_config_version"`
	ConfigLoadedAt        *time.Time `json:"config_loaded_at,omitempty"`
	ConfigLoadError       string     `json:"config_load_error,omitempty"`
	Degraded              bool       `json:"degraded"`
	EnabledEndpoints      int        `json:"enabled_endpoints"`
}

// Manager 维护提示词审计领域核心的不可变运行时快照与安全降级状态机。
type Manager struct {
	store     ConfigStore
	encryptor Encryptor

	mu              sync.RWMutex
	active          ActiveConfig
	expectedVersion int64
	loadedAt        *time.Time
	loadError       string
	degraded        bool
}

// NewManager 创建领域 Manager 实例。
func NewManager(store ConfigStore, encryptor Encryptor) *Manager {
	defCfg := DefaultConfig()
	activeDef := defCfg.ToActive(encryptor)
	return &Manager{
		store:     store,
		encryptor: encryptor,
		active:    activeDef,
	}
}

// Active 返回当前不可变运行时活跃配置副本。
func (m *Manager) Active() ActiveConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// RuntimeState 返回当前运行状态快照。
func (m *Manager) RuntimeState() RuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mode := ModeOff
	if m.active.Enabled {
		mode = ModeBlocking
	}

	return RuntimeState{
		Mode:                  mode,
		ExpectedConfigVersion: m.expectedVersion,
		ActiveConfigVersion:   m.active.ConfigVersion,
		ConfigLoadedAt:        m.loadedAt,
		ConfigLoadError:       m.loadError,
		Degraded:              m.degraded,
		EnabledEndpoints:      len(m.active.EnabledEndpoints()),
	}
}

// IsDegraded 检查当前运行实例是否处于配置降级（需失败关闭）状态。
func (m *Manager) IsDegraded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.degraded
}

// Reload 从注入的 ConfigStore 加载配置并原子刷新运行时快照。
// 若配置不存在，保持安全的默认关闭；
// 若配置存在但解析/校验失败或启用状态下无可用节点，必须进入 degraded 状态，不得回退为关闭。
func (m *Manager) Reload(ctx context.Context) error {
	if m.store == nil {
		return errors.New("config store not configured")
	}

	cfg, err := m.store.Load(ctx)
	if err != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.loadError = err.Error()
		// 加载失败时如果曾期望开启，则进入 degraded
		if m.active.Enabled {
			m.degraded = true
		}
		return err
	}

	// 配置不存在，安全默认关闭
	if cfg == nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		defCfg := DefaultConfig()
		m.active = defCfg.ToActive(m.encryptor)
		m.expectedVersion = 0
		m.loadError = ""
		m.degraded = false
		now := time.Now()
		m.loadedAt = &now
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectedVersion = cfg.ConfigVersion

	// 校验配置
	hasStableSecret := HasStableCryptoSecret()
	if err := cfg.NormalizeAndValidate(hasStableSecret); err != nil {
		m.loadError = err.Error()
		if cfg.Enabled {
			m.degraded = true
		}
		return err
	}

	active := cfg.ToActive(m.encryptor)
	if cfg.Enabled && len(active.EnabledEndpoints()) == 0 {
		m.loadError = "no usable guard endpoints available"
		m.degraded = true
		return errors.New(m.loadError)
	}

	// 激活成功
	m.active = active
	m.degraded = false
	m.loadError = ""
	now := time.Now()
	m.loadedAt = &now
	return nil
}

// SetActiveForTest 提供测试专用的配置与降级状态注入
func (m *Manager) SetActiveForTest(active ActiveConfig, degraded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = active
	m.degraded = degraded
}
