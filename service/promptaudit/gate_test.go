package promptaudit

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEvaluator struct {
	decision *Decision
	err      error
	called   int
}

func (m *mockEvaluator) Evaluate(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot) (*Decision, error) {
	m.called++
	return m.decision, m.err
}

type mockEventStore struct {
	mu        sync.Mutex
	recorded  []PromptSnapshot
	decisions []*Decision
	err       error
}

func (m *mockEventStore) Record(ctx context.Context, snapshot PromptSnapshot, decision *Decision, storePassEvents bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.recorded = append(m.recorded, snapshot)
	m.decisions = append(m.decisions, decision)
	return nil
}

func setupGateTest(t *testing.T, activeCfg ActiveConfig, degraded bool, eval Evaluator, store EventStore) func() {
	origMgr := GetManager()
	origEval := GetEvaluator()
	origStore := GetEventStore()

	mgr := NewManager(nil, nil)
	mgr.active = activeCfg
	mgr.degraded = degraded

	SetGlobalManager(mgr)
	SetGlobalEvaluator(eval)
	SetGlobalEventStore(store)

	return func() {
		SetGlobalManager(origMgr)
		SetGlobalEvaluator(origEval)
		SetGlobalEventStore(origStore)
	}
}

func TestCheckRelayRequest_DisabledAndGroupMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "测试正常文本"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default"}

	eval := &mockEvaluator{}
	store := &mockEventStore{}

	// 1. 审计未开启 -> 直接放行
	cleanup := setupGateTest(t, ActiveConfig{Enabled: false}, false, eval, store)
	err := CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
	cleanup()

	// 2. 分组未命中 + 合法 Codex CLI -> 直接放行且不送审
	attachCodexCLI(c)
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"vip": {}},
	}, false, eval, store)
	err = CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
	cleanup()

	// 3. 分组未命中 + 非 Codex CLI -> 放行且不送审（生效范围同时约束 Codex 限制）
	attachOriginator(c, "curl")
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"vip": {}},
	}, false, eval, store)
	err = CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	cleanup()

	// 4. 分组命中 + 非 Codex CLI -> 仍因客户端门禁 503
	infoVIP := &relaycommon.RelayInfo{UsingGroup: "vip"}
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"vip": {}},
	}, false, eval, store)
	err = CheckRelayRequest(c, infoVIP, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeCodexCLIRequired), err.GetErrorCode())
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	cleanup()
}

func TestCheckRelayRequest_ConfigDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "测试文本"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default"}

	eval := &mockEvaluator{}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, true, eval, store)
	defer cleanup()

	err := CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeConfigDegraded), err.GetErrorCode())
	assert.Equal(t, 0, eval.called)
}

func TestCheckRelayRequest_UnsupportedProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{UsingGroup: "default"}
	eval := &mockEvaluator{}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	attachCodexCLI(c)
	type unknownReq struct {
		dto.BaseRequest
	}
	err := CheckRelayRequest(c, info, &unknownReq{})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeUnsupportedProtocol), err.GetErrorCode())
	assert.Equal(t, 0, eval.called)
}

func TestCheckRelayRequest_NoPromptPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{UsingGroup: "default"}
	eval := &mockEvaluator{}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	attachCodexCLI(c)
	emptyReq := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "    "}},
	}
	err := CheckRelayRequest(c, info, emptyReq)
	assert.Nil(t, err)
	assert.Equal(t, 0, eval.called)
}

func TestCheckRelayRequest_BlockDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "违规提问内容"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default", RequestId: "req-block-test"}

	eval := &mockEvaluator{
		decision: &Decision{
			Kind:           DecisionBlock,
			HTTPStatus:     http.StatusForbidden,
			AllowNextStage: false,
		},
	}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	attachCodexCLI(c)
	err := CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeBlocked), err.GetErrorCode())
	assert.Equal(t, 1, eval.called)

	// 断言 Block 事件已写库
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.recorded, 1)
	assert.Equal(t, "req-block-test", store.recorded[0].RequestID)
	assert.Equal(t, DecisionBlock, store.decisions[0].Kind)
}

func TestCheckRelayRequest_EvaluatorUnavailableAndInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "普通请求"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default"}

	// 1. Unavailable
	eval := &mockEvaluator{
		err: &GuardError{
			Code:       ErrorCodeUnavailable,
			HTTPStatus: http.StatusServiceUnavailable,
			Cause:      errors.New("connection refused"),
		},
	}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	attachCodexCLI(c)

	err := CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeUnavailable), err.GetErrorCode())
	cleanup()

	// 2. Invalid Response
	evalInvalid := &mockEvaluator{
		err: &GuardError{
			Code:       ErrorCodeInvalidResponse,
			HTTPStatus: http.StatusServiceUnavailable,
			Cause:      errors.New("invalid guard envelope"),
		},
	}
	cleanup = setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, evalInvalid, store)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	attachCodexCLI(c)

	err = CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeInvalidResponse), err.GetErrorCode())
	cleanup()
}

func TestCheckRelayRequest_AllowAndRecordFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "安全的合规请求"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default", RequestId: "req-allow-test"}

	eval := &mockEvaluator{
		decision: &Decision{
			Kind:           DecisionAllow,
			HTTPStatus:     http.StatusOK,
			AllowNextStage: true,
		},
	}

	// 1. StorePassEvents == true 且 Record 成功 -> 放行返回 nil
	storeOK := &mockEventStore{}
	cleanup := setupGateTest(t, ActiveConfig{
		Enabled:         true,
		AllGroups:       true,
		StorePassEvents: true,
	}, false, eval, storeOK)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	attachCodexCLI(c)
	err := CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Len(t, storeOK.recorded, 1)
	cleanup()

	// 2. StorePassEvents == true 但 Record 失败 -> 拦截并返回 ErrorCodeRecordFailed (503)
	storeFail := &mockEventStore{err: errors.New("db insert error")}
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:         true,
		AllGroups:       true,
		StorePassEvents: true,
	}, false, eval, storeFail)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	attachCodexCLI(c)
	err = CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeRecordFailed), err.GetErrorCode())
	cleanup()

	// 3. StorePassEvents == false -> 放行返回 nil，不记录
	storeSkip := &mockEventStore{}
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:         true,
		AllGroups:       true,
		StorePassEvents: false,
	}, false, eval, storeSkip)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	attachCodexCLI(c)
	err = CheckRelayRequest(c, info, req)
	assert.Nil(t, err)
	assert.Len(t, storeSkip.recorded, 0)
	cleanup()
}

func TestCheckMidjourneyRequest_Gate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. 查询类动作直接放行
	infoQuery := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyTaskFetch,
		UsingGroup:      "default",
		OriginModelName: "midjourney",
	}
	eval := &mockEvaluator{}
	store := &mockEventStore{}

	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/mj/task/fetch", nil)
	attachCodexCLI(c)

	mjErr := CheckMidjourneyRequest(c, infoQuery)
	assert.Nil(t, mjErr)
	assert.Equal(t, 0, eval.called)

	// 2. 提交类动作命中 Block -> 拦截并返回 403 prompt_guard_blocked
	bodyBytes, _ := common.Marshal(map[string]any{
		"prompt": "违规绘画内容",
	})
	evalBlock := &mockEvaluator{
		decision: &Decision{
			Kind:           DecisionBlock,
			HTTPStatus:     http.StatusForbidden,
			AllowNextStage: false,
		},
	}
	cleanupBlock := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, evalBlock, store)
	defer cleanupBlock()

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")
	attachCodexCLI(c)

	infoImagine := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeMidjourneyImagine,
		UsingGroup:      "default",
		OriginModelName: "midjourney",
	}

	mjBlockErr := CheckMidjourneyRequest(c, infoImagine)
	require.NotNil(t, mjBlockErr)
	assert.Equal(t, http.StatusForbidden, mjBlockErr.Code)
	assert.Equal(t, ErrorCodeBlocked, mjBlockErr.Description)
	assert.Equal(t, 1, evalBlock.called)
}
