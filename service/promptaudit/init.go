package promptaudit

import (
	"context"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	globalMu          sync.RWMutex
	globalManager     *Manager
	globalEventStore  EventStore
	globalEncryptor   Encryptor
	globalConfigStore ConfigStore
	globalEvaluator   Evaluator
)

// InitPromptAudit 初始化提示词审计全局单例：派生 Encryptor、组装 ConfigStore、EventStore、
// Evaluator 与 Manager，执行首次配置加载与激活，注册多实例 Option 同步 Hook，并在 Master 节点启动清理循环。
func InitPromptAudit() error {
	masterSecret := common.CryptoSecret
	if masterSecret == "" {
		masterSecret = "default-master-secret"
	}

	encryptor, err := NewAESGCMEncryptor(masterSecret)
	if err != nil {
		return fmt.Errorf("init prompt audit encryptor: %w", err)
	}

	configStore := NewGormConfigStore()
	eventStore := NewGormEventStore(encryptor)
	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	manager := NewManager(configStore, encryptor)

	// 首次加载配置；如果配置不存在保持默认安全关闭，配置损坏则进入 degraded
	if err := manager.Reload(context.Background()); err != nil {
		common.SysError(fmt.Sprintf("[PROMPT_AUDIT] initial reload: %v (manager degraded=%v)", err, manager.IsDegraded()))
	} else {
		common.SysLog(fmt.Sprintf("[PROMPT_AUDIT] initialized: enabled=%v version=%d", manager.Active().Enabled, manager.Active().ConfigVersion))
	}

	// 注册多实例 Option 同步回调
	model.SetPromptAuditConfigSyncHook(func() {
		if err := manager.Reload(context.Background()); err != nil {
			common.SysError(fmt.Sprintf("[PROMPT_AUDIT] option sync reload: %v", err))
		}
	})

	// Master 节点启动保留期清理
	if common.IsMasterNode {
		StartRetentionCleanup(context.Background(), func() int {
			return manager.Active().RetentionDays
		})
	}

	globalMu.Lock()
	globalManager = manager
	globalEventStore = eventStore
	globalEncryptor = encryptor
	globalConfigStore = configStore
	globalEvaluator = evaluator
	globalMu.Unlock()

	return nil
}

// GetManager 获取全局单例 Prompt Audit Manager。
func GetManager() *Manager {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalManager
}

// GetEventStore 获取全局单例 EventStore。
func GetEventStore() EventStore {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalEventStore
}

// GetEncryptor 获取全局单例 Encryptor。
func GetEncryptor() Encryptor {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalEncryptor
}

// GetConfigStore 获取全局单例 ConfigStore。
func GetConfigStore() ConfigStore {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalConfigStore
}

// GetEvaluator 获取全局单例 Evaluator。
func GetEvaluator() Evaluator {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalEvaluator
}

// SetGlobalManager 设置全局 Manager（供测试夹具使用）。
func SetGlobalManager(m *Manager) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalManager = m
}

// SetGlobalEventStore 设置全局 EventStore（供测试夹具使用）。
func SetGlobalEventStore(s EventStore) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalEventStore = s
}

// SetGlobalEncryptor 设置全局 Encryptor（供测试夹具使用）。
func SetGlobalEncryptor(e Encryptor) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalEncryptor = e
}

// SetGlobalEvaluator 设置全局 Evaluator（供测试夹具使用）。
func SetGlobalEvaluator(e Evaluator) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalEvaluator = e
}

// SetGlobalForTestHelper 供测试注入自定义 active 配置和 mock 对象。
func SetGlobalForTestHelper(active ActiveConfig, degraded bool, eval Evaluator, store EventStore) func() {
	globalMu.Lock()
	oldMgr := globalManager
	oldEval := globalEvaluator
	oldStore := globalEventStore

	mgr := NewManager(nil, nil)
	mgr.active = active
	mgr.degraded = degraded

	globalManager = mgr
	globalEvaluator = eval
	globalEventStore = store
	globalMu.Unlock()

	return func() {
		globalMu.Lock()
		globalManager = oldMgr
		globalEvaluator = oldEval
		globalEventStore = oldStore
		globalMu.Unlock()
	}
}
