package promptaudit

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const originatorHeaderName = "Originator"

// allowedCodexCLIOriginators 是审计开启后允许的标准 Codex CLI Originator（规范化后完整值匹配）。
// 取值对齐官方 Codex 0.153.2 实际入站头：默认 HTTP 客户端为 codex_cli_rs，
// 交互 TUI 为 codex-tui，非交互 `codex exec` 为 codex_exec；codex-cli 保留发行标识兼容值。
// 不含 VS Code / Desktop / Monitor 等非 CLI 第一方客户端。
var allowedCodexCLIOriginators = map[string]struct{}{
	"codex_cli_rs": {},
	"codex-cli":    {},
	"codex-tui":    {},
	"codex_exec":   {},
}

// normalizeOriginator 对 Originator 做 TrimSpace 后的 ASCII 小写，供完整值匹配。
func normalizeOriginator(value string) string {
	value = strings.TrimSpace(value)
	b := []byte(value)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// IsCodexCLIRequest 判断入站请求是否声明为标准 Codex CLI。
// Originator 是兼容性识别信号而非鉴权凭据；仅完整匹配允许值，不做子串或 User-Agent 回退。
func IsCodexCLIRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	normalized := normalizeOriginator(r.Header.Get(originatorHeaderName))
	if normalized == "" {
		return false
	}
	_, ok := allowedCodexCLIOriginators[normalized]
	return ok
}

// CheckAuditClientAccess 在确认审计开启后检查 Codex CLI 客户端身份。
// 顺序：Manager 缺失或审计关闭则放行；配置 degraded 优先于身份错误；非允许 Originator 返回 503。
func CheckAuditClientAccess(c *gin.Context) (ActiveConfig, *GuardError) {
	mgr := GetManager()
	if mgr == nil {
		return ActiveConfig{}, nil
	}
	if mgr.IsDegraded() {
		return ActiveConfig{}, &GuardError{
			Code:       ErrorCodeConfigDegraded,
			HTTPStatus: http.StatusServiceUnavailable,
			Retryable:  false,
		}
	}

	cfg := mgr.Active()
	if !cfg.Enabled {
		return cfg, nil
	}

	var req *http.Request
	if c != nil {
		req = c.Request
	}
	if !IsCodexCLIRequest(req) {
		return cfg, &GuardError{
			Code:       ErrorCodeCodexCLIRequired,
			HTTPStatus: http.StatusServiceUnavailable,
			Retryable:  false,
			Cause:      errors.New(CodexCLIRequiredMessage),
		}
	}
	return cfg, nil
}
