package controller

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/promptaudit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type relayMockEvaluator struct {
	decision *promptaudit.Decision
	err      error
	called   int
}

func (m *relayMockEvaluator) Evaluate(ctx context.Context, cfg promptaudit.ActiveConfig, snapshot promptaudit.PromptSnapshot) (*promptaudit.Decision, error) {
	m.called++
	return m.decision, m.err
}

type relayMockEventStore struct {
	recorded  []promptaudit.PromptSnapshot
	decisions []*promptaudit.Decision
	err       error
}

func (m *relayMockEventStore) Record(ctx context.Context, snapshot promptaudit.PromptSnapshot, decision *promptaudit.Decision, storePassEvents bool) error {
	if m.err != nil {
		return m.err
	}
	m.recorded = append(m.recorded, snapshot)
	m.decisions = append(m.decisions, decision)
	return nil
}

func attachCodexCLIHeader(c *gin.Context) {
	if c.Request == nil {
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	}
	c.Request.Header.Set("Originator", "codex_cli_rs")
}

func TestRelay_PromptAudit_Block(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionBlock,
			HTTPStatus:     http.StatusForbidden,
			AllowNextStage: false,
		},
	}
	store := &relayMockEventStore{}

	mgrActive := promptaudit.ActiveConfig{
		Enabled:   true,
		AllGroups: true,
	}
	// 临时注入 active
	cleanup := promptaudit.SetGlobalForTestHelper(mgrActive, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	secretPrompt := "SUPER_SECRET_CANARY_PROMPT_DO_NOT_LEAK"
	bodyBytes, err := common.Marshal(map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": secretPrompt},
		},
	})
	require.NoError(t, err)

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLIHeader(c)
	common.SetContextKey(c, common.RequestIdKey, "req-test-block-001")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")

	Relay(c, relaytypes.RelayFormatOpenAI)

	// 断言 1: HTTP 状态码为 403 Forbidden
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 断言 2: 返回的错误结构包含 prompt_guard_blocked
	var resp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	err = common.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, promptaudit.ErrorCodeBlocked, resp.Error.Code)

	// 断言 3: 零敏感日志与零响应泄露，错误消息绝不含 prompt 原文
	assert.NotContains(t, w.Body.String(), secretPrompt)

	// 断言 4: 上游调用次数为 0，门禁 Evaluator 调用 1 次
	assert.Equal(t, 1, eval.called)

	// 断言 5: 预扣费次数为 0 (未进入 PreConsumeBilling，可通过上下文预扣费标记断言)
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
}

func TestRelay_PromptAudit_FailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		degraded     bool
		evalErr      error
		expectedCode string
	}{
		{
			name:         "unavailable",
			degraded:     false,
			evalErr:      &promptaudit.GuardError{Code: promptaudit.ErrorCodeUnavailable, HTTPStatus: 503},
			expectedCode: promptaudit.ErrorCodeUnavailable,
		},
		{
			name:         "invalid_response",
			degraded:     false,
			evalErr:      &promptaudit.GuardError{Code: promptaudit.ErrorCodeInvalidResponse, HTTPStatus: 503},
			expectedCode: promptaudit.ErrorCodeInvalidResponse,
		},
		{
			name:         "config_degraded",
			degraded:     true,
			evalErr:      nil,
			expectedCode: promptaudit.ErrorCodeConfigDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := &relayMockEvaluator{err: tt.evalErr}
			store := &relayMockEventStore{}
			activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true}

			cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, tt.degraded, eval, store)
			defer cleanup()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			bodyBytes, _ := common.Marshal(map[string]any{
				"model": "gpt-4o",
				"messages": []map[string]string{
					{"role": "user", "content": "普通测试文本"},
				},
			})
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")
			if !tt.degraded {
				attachCodexCLIHeader(c)
			}
			common.SetContextKey(c, common.RequestIdKey, "req-failclosed-"+tt.name)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")

			Relay(c, relaytypes.RelayFormatOpenAI)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedCode)
		})
	}
}

func TestRelayMidjourney_PromptAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionBlock,
			HTTPStatus:     http.StatusForbidden,
			AllowNextStage: false,
		},
	}
	store := &relayMockEventStore{}
	activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	// 1. 提交类动作命中 Block -> 403
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	bodyBytes, _ := common.Marshal(map[string]any{
		"prompt": "画一个违规画面的图像",
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLIHeader(c)
	common.SetContextKey(c, common.RequestIdKey, "req-mj-block-001")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set("relay_mode", relayconstant.RelayModeMidjourneyImagine)

	RelayMidjourney(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var mjResp taskdto.MidjourneyResponse
	err := common.Unmarshal(w.Body.Bytes(), &mjResp)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, mjResp.Code)
	assert.Equal(t, promptaudit.ErrorCodeBlocked, mjResp.Description)
	assert.NotContains(t, w.Body.String(), "画一个违规画面的图像")
	assert.Equal(t, 1, eval.called)
}

func TestRelay_ExplicitZeroValuesPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionAllow,
			HTTPStatus:     http.StatusOK,
			AllowNextStage: true,
		},
	}
	store := &relayMockEventStore{}
	activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true, StorePassEvents: false}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	bodyJSON := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "零值测试"}],
		"temperature": 0.0,
		"stream": false,
		"n": 1
	}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(bodyJSON))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLIHeader(c)

	// 验证 CheckRelayRequest 之后 body 依然保留显式 0, 0.0, false
	info := &relaycommon.RelayInfo{UsingGroup: "default"}
	req := &dto.GeneralOpenAIRequest{}
	err := common.UnmarshalBodyReusable(c, req)
	require.NoError(t, err)
	require.NotNil(t, req.Temperature)
	assert.Equal(t, 0.0, *req.Temperature)
	require.NotNil(t, req.Stream)
	assert.False(t, *req.Stream)

	auditErr := promptaudit.CheckRelayRequest(c, info, req)
	assert.Nil(t, auditErr)

	// 再次从 c 中 UnmarshalBodyReusable，确保 optional scalar 的 0.0 和 false 依然完整保留
	reqAfter := &dto.GeneralOpenAIRequest{}
	err = common.UnmarshalBodyReusable(c, reqAfter)
	require.NoError(t, err)
	require.NotNil(t, reqAfter.Temperature)
	assert.Equal(t, 0.0, *reqAfter.Temperature)
	require.NotNil(t, reqAfter.Stream)
	assert.False(t, *reqAfter.Stream)
}

func TestRelay_PromptAudit_DisabledAndGroupMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}

	// 1. 审计未开启 -> CheckRelayRequest 放行且不调用 Evaluator
	activeCfgDisabled := promptaudit.ActiveConfig{Enabled: false}
	cleanup := promptaudit.SetGlobalForTestHelper(activeCfgDisabled, false, eval, store)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"测试"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{UsingGroup: "default"}
	req := &dto.GeneralOpenAIRequest{}
	_ = common.UnmarshalBodyReusable(c, req)

	err := promptaudit.CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
	cleanup()

	// 2. 分组未命中 + 合法 Codex CLI -> CheckRelayRequest 放行且不调用 Evaluator
	attachCodexCLIHeader(c)
	activeCfgGroup := promptaudit.ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"vip": {}},
	}
	cleanup = promptaudit.SetGlobalForTestHelper(activeCfgGroup, false, eval, store)
	err = promptaudit.CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
	cleanup()
}

func TestRelay_PromptAudit_RecordFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionAllow,
			HTTPStatus:     http.StatusOK,
			AllowNextStage: true,
		},
	}
	store := &relayMockEventStore{
		err: errors.New("database disk full"),
	}
	activeCfg := promptaudit.ActiveConfig{
		Enabled:         true,
		AllGroups:       true,
		StorePassEvents: true,
	}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	bodyBytes, _ := common.Marshal(map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "合规请求内容"},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLIHeader(c)
	common.SetContextKey(c, common.RequestIdKey, "req-record-failed-001")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")

	Relay(c, relaytypes.RelayFormatOpenAI)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeRecordFailed)
	assert.Equal(t, 1, eval.called)
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
}

func TestRelayMidjourney_QueryActionsBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	// 查询类动作，CheckMidjourneyRequest 返回 nil，eval.called == 0
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/mj/task/123/fetch", nil)
	attachCodexCLIHeader(c)

	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyTaskFetch,
		UsingGroup:      "default",
		OriginModelName: "midjourney",
	}

	mjErr := promptaudit.CheckMidjourneyRequest(c, info)
	assert.Nil(t, mjErr)
	assert.Equal(t, 0, eval.called)
}

func TestRelay_RealtimeFormatBypassesHttpGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		RelayFormat: relaytypes.RelayFormatOpenAIRealtime,
		UsingGroup:  "default",
	}

	// Realtime 格式在 HTTP Relay 门禁处应直接返回 nil 放行，保留给 Realtime 帧级门禁处理
	err := promptaudit.CheckRelayRequest(c, info, &dto.BaseRequest{})
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
}

func TestRelayMidjourney_PromptWithImageURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionAllow,
			HTTPStatus:     http.StatusOK,
			AllowNextStage: true,
		},
	}
	store := &relayMockEventStore{}
	activeCfg := promptaudit.ActiveConfig{Enabled: true, AllGroups: true, StorePassEvents: false}

	cleanup := promptaudit.SetGlobalForTestHelper(activeCfg, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	promptWithImgURL := "https://s.mj.run/xyz12345 画一只在雨林中的机械变色龙 --v 6.0"
	bodyBytes, err := common.Marshal(map[string]any{
		"prompt": promptWithImgURL,
	})
	require.NoError(t, err)

	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLIHeader(c)
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyImagine,
		UsingGroup:      "default",
		OriginModelName: "midjourney",
	}

	mjErr := promptaudit.CheckMidjourneyRequest(c, info)
	assert.Nil(t, mjErr)
	assert.Equal(t, 1, eval.called)
}

type hijackCountingWriter struct {
	*httptest.ResponseRecorder
	hijackCount int
}

func (w *hijackCountingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijackCount++
	return nil, nil, errors.New("hijack should not be called before Codex CLI gate")
}

func TestRelay_PromptAudit_CodexCLIRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	cleanup := promptaudit.SetGlobalForTestHelper(promptaudit.ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	secretPrompt := "SUPER_SECRET_CANARY_PROMPT_DO_NOT_LEAK"
	originatorCanary := "CANARY_ORIGINATOR_DO_NOT_ECHO"
	uaCanary := "CANARY_USER_AGENT_DO_NOT_ECHO"

	tests := []struct {
		name          string
		relayFormat   relaytypes.RelayFormat
		path          string
		requestID     string
		setOriginator string
		model         string
	}{
		{name: "openai unknown originator", relayFormat: relaytypes.RelayFormatOpenAI, path: "/v1/chat/completions", requestID: "req-unknown-originator", setOriginator: originatorCanary, model: "gpt-4o"},
		{name: "openai missing originator", relayFormat: relaytypes.RelayFormatOpenAI, path: "/v1/chat/completions", requestID: "req-missing-originator", model: "gpt-4o"},
		{name: "openai curl originator", relayFormat: relaytypes.RelayFormatOpenAI, path: "/v1/chat/completions", requestID: "req-unknown-client", setOriginator: "curl", model: "gpt-4o"},
		{name: "claude wrapper originator", relayFormat: relaytypes.RelayFormatClaude, path: "/v1/messages", requestID: "req-claude-wrapper", setOriginator: "my-codex_cli_rs-wrapper", model: "claude-3-5-sonnet-latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval.called = 0
			store.recorded = nil

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body := map[string]any{
				"model": tt.model,
				"messages": []map[string]string{
					{"role": "user", "content": secretPrompt},
				},
			}
			if tt.relayFormat == relaytypes.RelayFormatClaude {
				body["max_tokens"] = 16
			}
			bodyBytes, err := common.Marshal(body)
			require.NoError(t, err)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("User-Agent", uaCanary)
			if tt.setOriginator != "" {
				c.Request.Header.Set("Originator", tt.setOriginator)
			}
			common.SetContextKey(c, common.RequestIdKey, tt.requestID)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tt.model)

			Relay(c, tt.relayFormat)

			assert.Equal(t, http.StatusServiceUnavailable, w.Code)
			var resp struct {
				Type  string `json:"type"`
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    any    `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, promptaudit.ErrorCodeCodexCLIRequired, resp.Error.Code)
			assert.Contains(t, resp.Error.Message, promptaudit.CodexCLIRequiredMessage)
			if tt.relayFormat == relaytypes.RelayFormatClaude {
				assert.Equal(t, "error", resp.Type)
			}
			assert.Equal(t, 0, eval.called)
			assert.Len(t, store.recorded, 0)
			assert.False(t, c.GetBool("preconsumed_quota_flag"))
			assert.Empty(t, c.GetStringSlice("use_channel"))
			assert.NotContains(t, w.Body.String(), secretPrompt)
			assert.NotContains(t, w.Body.String(), originatorCanary)
			assert.NotContains(t, w.Body.String(), uaCanary)
			if tt.setOriginator != "" && !strings.Contains(promptaudit.CodexCLIRequiredMessage, tt.setOriginator) {
				assert.NotContains(t, w.Body.String(), tt.setOriginator)
			}
		})
	}
}

func TestRelayMidjourney_PromptAudit_CodexCLIRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	cleanup := promptaudit.SetGlobalForTestHelper(promptaudit.ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	canaryPrompt := "CANARY_MJ_PROMPT_DO_NOT_LEAK"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	bodyBytes, _ := common.Marshal(map[string]any{
		"prompt": canaryPrompt,
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Originator", "Codex CLI")
	common.SetContextKey(c, common.RequestIdKey, "req-mj-display-originator")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set("relay_mode", relayconstant.RelayModeMidjourneyImagine)

	RelayMidjourney(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var mjResp struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Result      string `json:"result"`
		Type        string `json:"type"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &mjResp))
	assert.Equal(t, http.StatusServiceUnavailable, mjResp.Code)
	assert.Equal(t, promptaudit.ErrorCodeCodexCLIRequired, mjResp.Result)
	assert.Equal(t, promptaudit.CodexCLIRequiredMessage, mjResp.Description)
	assert.Equal(t, "prompt_audit_error", mjResp.Type)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
	assert.Empty(t, c.GetStringSlice("use_channel"))
	assert.NotContains(t, w.Body.String(), canaryPrompt)
}

func TestRelay_Realtime_PromptAudit_CodexCLIRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	cleanup := promptaudit.SetGlobalForTestHelper(promptaudit.ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	hijack := &hijackCountingWriter{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(hijack)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")
	c.Request.Header.Set("Sec-WebSocket-Version", "13")
	c.Request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	c.Request.Header.Set("Sec-WebSocket-Protocol", "realtime")
	c.Request.Header.Set("Originator", "curl")
	c.Request.Header.Set("User-Agent", "CANARY_UA_REALTIME")
	common.SetContextKey(c, common.RequestIdKey, "req-rt-unknown-client")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	Relay(c, relaytypes.RelayFormatOpenAIRealtime)

	assert.Equal(t, http.StatusServiceUnavailable, hijack.Code)
	assert.NotEqual(t, http.StatusSwitchingProtocols, hijack.Code)
	assert.Equal(t, 0, hijack.hijackCount, "client websocket upgrade must not run")
	var resp struct {
		Error struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(hijack.Body.Bytes(), &resp))
	assert.Equal(t, promptaudit.ErrorCodeCodexCLIRequired, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, promptaudit.CodexCLIRequiredMessage)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
	assert.Empty(t, c.GetStringSlice("use_channel"))
	assert.NotContains(t, hijack.Body.String(), "curl")
	assert.NotContains(t, hijack.Body.String(), "CANARY_UA_REALTIME")
}

func TestRelay_PromptAudit_CodexCLIRequired_GroupMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	eval := &relayMockEvaluator{}
	store := &relayMockEventStore{}
	cleanup := promptaudit.SetGlobalForTestHelper(promptaudit.ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"vip": {}},
	}, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	bodyBytes, _ := common.Marshal(map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "group mismatch canary"},
		},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, common.RequestIdKey, "req-group-mismatch-non-codex")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")

	Relay(c, relaytypes.RelayFormatOpenAI)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeCodexCLIRequired)
	assert.Contains(t, w.Body.String(), promptaudit.CodexCLIRequiredMessage)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
	assert.Empty(t, c.GetStringSlice("use_channel"))
}
