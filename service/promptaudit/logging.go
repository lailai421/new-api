package promptaudit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/logger"
)

const (
	EventGuardStarted   = "prompt_guard.started"
	EventGuardAllowed   = "prompt_guard.allowed"
	EventGuardBlocked   = "prompt_guard.blocked"
	EventGuardFailed    = "prompt_guard.failed"
	EventConfigDegraded = "prompt_guard.config_degraded"
)

// allowedLogFields 严格白名单，任何不在白名单内的字段均自动丢弃。
// 绝不包含 Token、ScanText、FullPrompt、请求体、完整响应或密文。
var allowedLogFields = map[string]struct{}{
	"request_id":          {},
	"user_id":             {},
	"token_id":            {},
	"group":               {},
	"protocol":            {},
	"model":               {},
	"stage":               {},
	"audit_scope":         {},
	"config_version":      {},
	"guard_endpoint_id":   {},
	"decision":            {},
	"risk_level":          {},
	"action":              {},
	"chunk_index":         {},
	"chunk_total":         {},
	"chunk_chars":         {},
	"input_chars":         {},
	"input_limit":         {},
	"latency_ms":          {},
	"status":              {},
	"error_code":          {},
	"upstream_dispatched": {},
	"billing_preconsumed": {},
}

var (
	logHookMu sync.RWMutex
	logHook   func(level, event string, fields map[string]any)
)

// SetLogHook 设置测试或监控专用的日志捕获钩子。
func SetLogHook(hook func(level, event string, fields map[string]any)) {
	logHookMu.Lock()
	defer logHookMu.Unlock()
	logHook = hook
}

// SanitizeLogFields 仅保留白名单内的非敏感元数据。
func SanitizeLogFields(fields map[string]any) map[string]any {
	clean := make(map[string]any, len(fields))
	for k, v := range fields {
		if _, allowed := allowedLogFields[k]; allowed {
			clean[k] = v
		}
	}
	return clean
}

func logEmit(ctx context.Context, level, event string, fields map[string]any) {
	sanitized := SanitizeLogFields(fields)

	logHookMu.RLock()
	hook := logHook
	logHookMu.RUnlock()
	if hook != nil {
		hook(level, event, sanitized)
	}

	// 格式化输出为安全的结构化日志字符串
	keys := make([]string, 0, len(sanitized))
	for k := range sanitized {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[PROMPT_AUDIT] event=%s", event))
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf(" %s=%v", k, sanitized[k]))
	}

	msg := sb.String()
	switch level {
	case "WARN":
		logger.LogWarn(ctx, msg)
	case "ERR":
		logger.LogError(ctx, msg)
	default:
		logger.LogInfo(ctx, msg)
	}
}

func LogGuardInfo(ctx context.Context, event string, fields map[string]any) {
	logEmit(ctx, "INFO", event, fields)
}

func LogGuardWarn(ctx context.Context, event string, fields map[string]any) {
	logEmit(ctx, "WARN", event, fields)
}

func LogGuardError(ctx context.Context, event string, fields map[string]any) {
	logEmit(ctx, "ERR", event, fields)
}
