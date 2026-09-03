package promptaudit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldAuditRealtime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{UsingGroup: "vip"}

	// 1. 无 manager
	origMgr := GetManager()
	SetGlobalManager(nil)
	assert.False(t, ShouldAuditRealtime(c, info))
	SetGlobalManager(origMgr)

	// 2. 正常关闭配置
	cleanup := setupGateTest(t, ActiveConfig{Enabled: false}, false, nil, nil)
	assert.False(t, ShouldAuditRealtime(c, info))
	cleanup()

	// 3. 启用，全部分组
	cleanup = setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, nil, nil)
	assert.True(t, ShouldAuditRealtime(c, info))
	cleanup()

	// 4. 启用，指定分组不匹配
	cleanup = setupGateTest(t, ActiveConfig{
		Enabled:   true,
		AllGroups: false,
		GroupsMap: map[string]struct{}{"svip": {}},
	}, false, nil, nil)
	assert.False(t, ShouldAuditRealtime(c, info))
	cleanup()

	// 5. 处于 degraded 状态必须返回 true 以触发失败关闭
	cleanup = setupGateTest(t, ActiveConfig{Enabled: false}, true, nil, nil)
	assert.True(t, ShouldAuditRealtime(c, info))
	cleanup()
}

func TestCheckRealtimeEvent_Scenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}

	// 1. Degraded 状态
	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, true, nil, nil)
	res := CheckRealtimeEvent(context.Background(), c, info, []byte(`{"type": "input_audio_buffer.commit"}`))
	assert.False(t, res.Allowed)
	assert.True(t, res.IsError)
	assert.Equal(t, ErrorCodeConfigDegraded, res.ErrorCode)
	cleanup()

	// 2. 审计未开启 -> 放行
	cleanup = setupGateTest(t, ActiveConfig{Enabled: false}, false, nil, nil)
	res = CheckRealtimeEvent(context.Background(), c, info, []byte(`{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "hello"}]}}`))
	assert.True(t, res.Allowed)
	assert.False(t, res.IsError)
	cleanup()

	// 3. 纯音频/控制帧 -> NoPrompt 放行
	eval := &mockEvaluator{}
	store := &mockEventStore{}
	cleanup = setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)

	noPromptRes := CheckRealtimeEvent(context.Background(), c, info, []byte(`{"type": "input_audio_buffer.append", "audio": "xyz"}`))
	assert.True(t, noPromptRes.Allowed)
	assert.True(t, noPromptRes.IsNoPrompt)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.decisions, 0)

	// 4. 文本危险帧 -> Block 且写事件
	eval.decision = &Decision{
		Kind:           DecisionBlock,
		HTTPStatus:     http.StatusForbidden,
		ErrorCode:      ErrorCodeBlocked,
		AllowNextStage: false,
	}
	dangerJSON := `{"type": "conversation.item.create", "item": {"type": "message", "role": "user", "content": [{"type": "text", "text": "I want to hurt someone"}]}}`
	blockRes := CheckRealtimeEvent(context.Background(), c, info, []byte(dangerJSON))
	assert.False(t, blockRes.Allowed)
	assert.True(t, blockRes.IsBlock)
	assert.Equal(t, ErrorCodeBlocked, blockRes.ErrorCode)
	assert.Equal(t, http.StatusForbidden, blockRes.StatusCode)
	require.Len(t, store.decisions, 1)
	assert.Equal(t, DecisionBlock, store.decisions[0].Kind)

	// 5. Block 事件写库失败 -> 返回 ErrorCodeRecordFailed
	store.err = errors.New("mock db failure")
	blockResFail := CheckRealtimeEvent(context.Background(), c, info, []byte(dangerJSON))
	assert.False(t, blockResFail.Allowed)
	assert.True(t, blockResFail.IsError)
	assert.Equal(t, ErrorCodeRecordFailed, blockResFail.ErrorCode)
	assert.Equal(t, http.StatusServiceUnavailable, blockResFail.StatusCode)
	store.err = nil

	// 6. Guard 返回 Safe 放行 (storePassEvents = false 时不写库直接放行)
	eval.decision = &Decision{
		Kind:           DecisionAllow,
		HTTPStatus:     http.StatusOK,
		AllowNextStage: true,
	}
	safeJSON := `{"type": "conversation.item.create", "item": {"type": "message", "role": "user", "content": [{"type": "text", "text": "Hello assistant"}]}}`
	safeRes := CheckRealtimeEvent(context.Background(), c, info, []byte(safeJSON))
	assert.True(t, safeRes.Allowed)
	assert.False(t, safeRes.IsBlock)
	assert.False(t, safeRes.IsError)
	assert.Len(t, store.decisions, 1) // 保持为 1，因为 storePassEvents=false 不落库

	// 7. storePassEvents = true 时 Safe 落库
	cleanup()
	cleanup = setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true, StorePassEvents: true}, false, eval, store)
	safeRes2 := CheckRealtimeEvent(context.Background(), c, info, []byte(safeJSON))
	assert.True(t, safeRes2.Allowed)
	assert.Len(t, store.decisions, 2)
	assert.Equal(t, DecisionAllow, store.decisions[1].Kind)

	// 8. 非法格式 -> UnsupportedProtocol
	unsupportedRes := CheckRealtimeEvent(context.Background(), c, info, []byte(`{"type": "unknown_future_event", "text": "attack"}`))
	assert.False(t, unsupportedRes.Allowed)
	assert.True(t, unsupportedRes.IsError)
	assert.Equal(t, ErrorCodeUnsupportedProtocol, unsupportedRes.ErrorCode)

	cleanup()
}
