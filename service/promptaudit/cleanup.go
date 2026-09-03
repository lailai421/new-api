package promptaudit

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// RunCleanupOnce 执行单次过期事件清理。
// retentionDays <= 0 表示永久保留，直接返回 0, nil。
// 否则计算 cutoff = nowUnix - retentionDays * 86400，并调用 Model 层有界小批次删除。
// 参数可注入，方便进行确定性测试，避免依赖真实 sleep。
func RunCleanupOnce(
	ctx context.Context,
	retentionDays int,
	nowUnix int64,
	batchSize int,
	maxBatches int,
) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := nowUnix - int64(retentionDays)*86400
	return model.CleanupPromptAuditEvents(ctx, cutoff, batchSize, maxBatches)
}

// StartRetentionCleanup 仅在 Master 节点启动小时级自动清理后台任务。
// getRetentionDays 函数在每个周期动态读取当前活跃配置的保留期，使配置修改后自动按新策略清理既有数据。
func StartRetentionCleanup(ctx context.Context, getRetentionDays func() int) {
	if !common.IsMasterNode {
		return
	}

	common.SysLog("[PROMPT_AUDIT] master node starting hourly retention cleanup background task")

	go func() {
		runOnce := func() {
			retentionDays := 0
			if getRetentionDays != nil {
				retentionDays = getRetentionDays()
			}
			if retentionDays <= 0 {
				return
			}
			deleted, err := RunCleanupOnce(ctx, retentionDays, time.Now().Unix(), 500, 10)
			if err != nil {
				common.SysError(fmt.Sprintf("[PROMPT_AUDIT] retention cleanup failed: %v", err))
			} else if deleted > 0 {
				common.SysLog(fmt.Sprintf("[PROMPT_AUDIT] retention cleanup completed: deleted=%d retention_days=%d", deleted, retentionDays))
			}
		}

		// 启动后首次执行
		runOnce()

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runOnce()
			}
		}
	}()
}
