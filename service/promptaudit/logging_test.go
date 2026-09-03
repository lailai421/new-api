package promptaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeLogFields_WhitelistEnforcement(t *testing.T) {
	rawFields := map[string]any{
		// 允许的元数据
		"request_id":          "req-test-123",
		"decision":            "block",
		"risk_level":          "critical",
		"action":              "Block",
		"config_version":      int64(42),
		"latency_ms":          int64(150),
		"status":              "blocked",
		"error_code":          "prompt_guard_blocked",
		"upstream_dispatched": false,

		// 禁止泄漏的敏感字段
		"token":            "sensitive-guard-token-abc",
		"scan_text":        "user sensitive prompt text",
		"full_prompt":      "system prompt and full conversations",
		"token_ciphertext": "v1:ciphertext...",
		"request_body":     `{"messages":[{"content":"secret"}]}`,
		"response_body":    `{"choices":[{"message":{"content":"unsafe"}]}`,
		"password":         "pass123",
	}

	sanitized := SanitizeLogFields(rawFields)

	// 验证白名单字段保留
	assert.Equal(t, "req-test-123", sanitized["request_id"])
	assert.Equal(t, "block", sanitized["decision"])
	assert.Equal(t, "critical", sanitized["risk_level"])
	assert.Equal(t, "Block", sanitized["action"])
	assert.Equal(t, int64(42), sanitized["config_version"])
	assert.Equal(t, int64(150), sanitized["latency_ms"])
	assert.Equal(t, "blocked", sanitized["status"])
	assert.Equal(t, "prompt_guard_blocked", sanitized["error_code"])
	assert.Equal(t, false, sanitized["upstream_dispatched"])

	// 验证敏感字段全部被坚决丢弃
	assert.NotContains(t, sanitized, "token")
	assert.NotContains(t, sanitized, "scan_text")
	assert.NotContains(t, sanitized, "full_prompt")
	assert.NotContains(t, sanitized, "token_ciphertext")
	assert.NotContains(t, sanitized, "request_body")
	assert.NotContains(t, sanitized, "response_body")
	assert.NotContains(t, sanitized, "password")
}

func TestGuardEvaluator_NoSensitiveLeakageInLogs(t *testing.T) {
	secretToken := "SUPER-SECRET-GUARD-TOKEN-999"
	secretPrompt := "CONFIDENTIAL-USER-PROMPT-CONTENT-888"
	fullPrompt := "SYSTEM-FULL-PROMPT-777"

	var capturedLogs []struct {
		Level  string
		Event  string
		Fields map[string]any
	}
	var mu sync.Mutex

	SetLogHook(func(level, event string, fields map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		capturedLogs = append(capturedLogs, struct {
			Level  string
			Event  string
			Fields map[string]any
		}{Level: level, Event: event, Fields: fields})
	})
	defer SetLogHook(nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Violent"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:       true,
		Scanners:      AllScannerIDs,
		AllGroups:     true,
		ConfigVersion: 101,
		Endpoints: []ActiveEndpoint{
			{
				ID:        "node-leak-test",
				BaseURL:   server.URL,
				Token:     secretToken,
				TimeoutMS: 1000,
				Enabled:   true,
			},
		},
	}

	snapshot := PromptSnapshot{
		RequestID:    "req-leak-test",
		UserID:       12345,
		TokenID:      67890,
		Group:        "default",
		Protocol:     "openai_chat",
		Model:        "gpt-4o",
		FullPrompt:   fullPrompt,
		ScanText:     secretPrompt,
		PromptLength: len([]rune(secretPrompt)),
	}

	decision, err := evaluator.Evaluate(context.Background(), cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, decision.Kind)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, capturedLogs)

	for _, entry := range capturedLogs {
		// 检查每个字段的 key 和 value 均不包含敏感原文或 Token
		for k, v := range entry.Fields {
			strVal := fmt.Sprintf("%v", v)
			assert.NotEqual(t, "token", k, "Guard Token key must never be logged")
			assert.NotEqual(t, "scan_text", k, "ScanText key must never be logged")
			assert.NotEqual(t, "full_prompt", k, "FullPrompt key must never be logged")
			assert.NotContains(t, strVal, secretToken, "Guard Token value must not leak in log values")
			assert.NotContains(t, strVal, secretPrompt, "ScanText value must not leak in log values")
			assert.NotContains(t, strVal, fullPrompt, "FullPrompt value must not leak in log values")
		}
	}
}
