package promptaudit

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// GormConfigStore 实现 core ConfigStore 接口，基于主库 Option 表与 CAS 机制进行持久化。
type GormConfigStore struct{}

// NewGormConfigStore 创建 GormConfigStore 实例。
func NewGormConfigStore() *GormConfigStore {
	return &GormConfigStore{}
}

// Load 从主库 Option 表读取当前提示词审计配置。
// 若尚未配置该 Option，返回 nil, nil，由调用方（如 Manager）按默认安全关闭状态处理。
func (s *GormConfigStore) Load(ctx context.Context) (*Config, error) {
	raw, err := model.GetPromptAuditConfigRaw(ctx)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	return ParseConfig(raw)
}

// Save 在事务内执行基于 expectedVersion 的 CAS 更新，递增 config_version 并写入 Option 表。
// 若版本冲突，返回规范化的 ErrorCodeConfigConflict 错误。
func (s *GormConfigStore) Save(ctx context.Context, cfg *Config, expectedVersion int64) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}

	// 1. 保存前执行确定性规范化与边界校验
	hasStableSecret := HasStableCryptoSecret()
	if err := cfg.NormalizeAndValidate(hasStableSecret); err != nil {
		return err
	}

	// 2. 调用 Model 层事务 CAS 保存
	_, err := model.SavePromptAuditConfigCAS(
		ctx,
		expectedVersion,
		func(currentRaw string, currentVersion int64) (string, int64, error) {
			newVersion := int64(1)
			if currentVersion > 0 {
				newVersion = currentVersion + 1
			}
			cfg.ConfigVersion = newVersion
			cfg.UpdatedAt = time.Now().Unix()

			serialized, err := cfg.Serialize()
			if err != nil {
				return "", 0, err
			}
			return serialized, newVersion, nil
		},
	)
	if err != nil {
		if errors.Is(err, model.ErrPromptAuditConfigConflict) {
			return &GuardError{
				Code:       ErrorCodeConfigConflict,
				HTTPStatus: http.StatusConflict,
				Cause:      err,
			}
		}
		return err
	}

	return nil
}
