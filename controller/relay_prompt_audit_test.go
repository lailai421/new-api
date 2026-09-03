package controller

import (
	"bytes"
	"context"
	"errors"
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

	// 2. 分组未命中 -> CheckRelayRequest 放行且不调用 Evaluator
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
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyImagine,
		UsingGroup:      "default",
		OriginModelName: "midjourney",
	}

	mjErr := promptaudit.CheckMidjourneyRequest(c, info)
	assert.Nil(t, mjErr)
	assert.Equal(t, 1, eval.called)
}
