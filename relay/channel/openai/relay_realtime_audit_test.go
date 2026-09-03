package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/promptaudit"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestMain(m *testing.M) {
	service.InitTokenEncoders()
	m.Run()
}

type fakeUpstreamServer struct {
	server        *httptest.Server
	dialCount     int64
	mu            sync.Mutex
	receivedMsgs  [][]byte
	replyOnConn   bool
	replyMsg      []byte
	closeConnChan chan struct{}
}

func newFakeUpstreamServer(t *testing.T) *fakeUpstreamServer {
	fus := &fakeUpstreamServer{
		closeConnChan: make(chan struct{}),
	}
	fus.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fus.dialCount, 1)
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		if fus.replyOnConn && len(fus.replyMsg) > 0 {
			_ = ws.WriteMessage(websocket.TextMessage, fus.replyMsg)
		}

		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			fus.mu.Lock()
			fus.receivedMsgs = append(fus.receivedMsgs, msg)
			fus.mu.Unlock()
		}
	}))
	return fus
}

func (f *fakeUpstreamServer) Close() {
	if f.server != nil {
		f.server.Close()
	}
}

func (f *fakeUpstreamServer) GetReceivedMsgs() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make([][]byte, len(f.receivedMsgs))
	copy(copied, f.receivedMsgs)
	return copied
}

type mockAuditEvaluator struct {
	mu           sync.Mutex
	blockedTexts []string
	errCode      string
	calledCount  int
}

func (m *mockAuditEvaluator) Evaluate(ctx context.Context, cfg promptaudit.ActiveConfig, snapshot promptaudit.PromptSnapshot) (*promptaudit.Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calledCount++

	if m.errCode != "" {
		return nil, &promptaudit.GuardError{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       m.errCode,
		}
	}

	for _, bt := range m.blockedTexts {
		if strings.Contains(snapshot.FullPrompt, bt) {
			return &promptaudit.Decision{
				Kind:           promptaudit.DecisionBlock,
				HTTPStatus:     http.StatusForbidden,
				ErrorCode:      promptaudit.ErrorCodeBlocked,
				AllowNextStage: false,
			}, nil
		}
	}

	return &promptaudit.Decision{
		Kind:           promptaudit.DecisionAllow,
		HTTPStatus:     http.StatusOK,
		AllowNextStage: true,
	}, nil
}

type mockAuditStore struct {
	mu        sync.Mutex
	decisions []*promptaudit.Decision
	err       error
}

func (m *mockAuditStore) Record(ctx context.Context, snapshot promptaudit.PromptSnapshot, decision *promptaudit.Decision, storePassEvents bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.decisions = append(m.decisions, decision)
	return nil
}

// setupRealtimeTestConnections 创建一对相连的客户端连接（ClientWs 和 testClientConn）
func setupRealtimeTestConnections(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	clientWsChan := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		clientWsChan <- ws
	}))

	u := "ws" + strings.TrimPrefix(server.URL, "http")
	testClientConn, _, err := websocket.DefaultDialer.Dial(u, nil)
	require.NoError(t, err)

	clientWs := <-clientWsChan

	cleanup := func() {
		_ = clientWs.Close()
		_ = testClientConn.Close()
		server.Close()
	}
	return clientWs, testClientConn, cleanup
}

func setupMockAudit(t *testing.T, active bool, blockedTexts []string, errCode string) (*mockAuditEvaluator, *mockAuditStore, func()) {
	origMgr := promptaudit.GetManager()
	origEval := promptaudit.GetEvaluator()
	origStore := promptaudit.GetEventStore()

	eval := &mockAuditEvaluator{blockedTexts: blockedTexts, errCode: errCode}
	store := &mockAuditStore{}

	cfg := promptaudit.ActiveConfig{
		Enabled:   active,
		AllGroups: true,
	}
	mgr := promptaudit.NewManager(nil, nil)
	mgr.SetActiveForTest(cfg, false)

	promptaudit.SetGlobalManager(mgr)
	promptaudit.SetGlobalEvaluator(eval)
	promptaudit.SetGlobalEventStore(store)

	cleanup := func() {
		promptaudit.SetGlobalManager(origMgr)
		promptaudit.SetGlobalEvaluator(origEval)
		promptaudit.SetGlobalEventStore(origStore)
	}
	return eval, store, cleanup
}

func TestRealtimeAudit_FirstFrameBlock_ZeroDial_ZeroFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	canary := "SENSITIVE_CANARY_ATTACK_KEYWORD"
	_, _, cleanupAudit := setupMockAudit(t, true, []string{canary}, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	info := &relaycommon.RelayInfo{
		ClientWs:        clientWs,
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}
	info.TargetWsDialer = func() (*websocket.Conn, error) {
		u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		return conn, err
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 1. 发送纯音频控制帧（NoPrompt），应该进入缓存，此时上游仍不应拨号
	audioFrame := `{"type": "input_audio_buffer.append", "audio": "UklGRi4AAABXQVZF"}`
	err := testClientConn.WriteMessage(websocket.TextMessage, []byte(audioFrame))
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(0), atomic.LoadInt64(&fakeUpstream.dialCount), "控制帧不应触发上游拨号")
	assert.Len(t, fakeUpstream.GetReceivedMsgs(), 0, "上游收到的帧数必须为 0")

	// 2. 发送首个文本帧，包含恶意违规 canary
	blockedTextFrame := `{"type": "conversation.item.create", "event_id": "evt_blocked_001", "item": {"role": "user", "content": [{"type": "input_text", "text": "` + canary + `"}]}}`
	err = testClientConn.WriteMessage(websocket.TextMessage, []byte(blockedTextFrame))
	require.NoError(t, err)

	// 3. 客户端应该收到 prompt_guard_blocked 错误事件
	_ = testClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, respData, err := testClientConn.ReadMessage()
	require.NoError(t, err)

	var errEvt dto.RealtimeEvent
	err = common.Unmarshal(respData, &errEvt)
	require.NoError(t, err)
	assert.Equal(t, dto.RealtimeEventTypeError, errEvt.Type)
	assert.Equal(t, "evt_blocked_001", errEvt.EventId)
	require.NotNil(t, errEvt.Error)
	assert.Equal(t, promptaudit.ErrorCodeBlocked, errEvt.Error.Code)
	// 敏感 canary 词绝不进入错误响应
	assert.NotContains(t, errEvt.Error.Message, canary)

	// 验证上游状态：拨号次数依然为 0，帧数依然为 0！
	assert.Equal(t, int64(0), atomic.LoadInt64(&fakeUpstream.dialCount))
	assert.Len(t, fakeUpstream.GetReceivedMsgs(), 0)

	// 4. 随后发送合法安全文本帧
	safeTextFrame := `{"type": "conversation.item.create", "event_id": "evt_safe_002", "item": {"role": "user", "content": [{"type": "input_text", "text": "Hello, this is a safe prompt."}]}}`
	err = testClientConn.WriteMessage(websocket.TextMessage, []byte(safeTextFrame))
	require.NoError(t, err)

	// 等待建连与刷新
	time.Sleep(100 * time.Millisecond)

	// 断言：上游此时必须被拨号，且仅拨号 1 次！
	assert.Equal(t, int64(1), atomic.LoadInt64(&fakeUpstream.dialCount), "安全文本必须触发一次上游拨号")

	// 断言：上游必须按原始顺序收到两帧：第 1 帧为缓存的 audio append，第 2 帧为 safe text
	upstreamMsgs := fakeUpstream.GetReceivedMsgs()
	require.Len(t, upstreamMsgs, 2, "上游必须收到原序刷新的缓存帧与通过的文本帧")
	assert.Contains(t, string(upstreamMsgs[0]), "input_audio_buffer.append")
	assert.Contains(t, string(upstreamMsgs[1]), "Hello, this is a safe prompt.")
	// 被 Block 的危险帧绝对不在上游收到
	assert.NotContains(t, string(upstreamMsgs[0]), canary)
	assert.NotContains(t, string(upstreamMsgs[1]), canary)

	// 正常关闭客户端连接
	_ = testClientConn.Close()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit in time")
	}
}

func TestRealtimeAudit_StreamingPhaseBlock_NotForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	canary := "CANARY_ATTACK_STREAMING"
	_, _, cleanupAudit := setupMockAudit(t, true, []string{canary}, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	info := &relaycommon.RelayInfo{
		ClientWs:        clientWs,
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}
	info.TargetWsDialer = func() (*websocket.Conn, error) {
		u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		return conn, err
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 1. 发送安全文本建立连接
	safeFrame1 := `{"type": "conversation.item.create", "event_id": "evt_1", "item": {"role": "user", "content": [{"type": "text", "text": "initial safe text"}]}}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(safeFrame1)))

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int64(1), atomic.LoadInt64(&fakeUpstream.dialCount))
	require.Len(t, fakeUpstream.GetReceivedMsgs(), 1)

	// 2. streaming 阶段发送危险文本
	dangerFrame := `{"type": "conversation.item.create", "event_id": "evt_danger", "item": {"role": "user", "content": [{"type": "text", "text": "` + canary + `"}]}}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(dangerFrame)))

	// 客户端收到 Block 错误
	_ = testClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, respData, err := testClientConn.ReadMessage()
	require.NoError(t, err)

	var errEvt dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(respData, &errEvt))
	assert.Equal(t, promptaudit.ErrorCodeBlocked, errEvt.Error.Code)
	assert.NotContains(t, errEvt.Error.Message, canary)

	// 上游收到的帧数量依然为 1（危险帧未转发至上游）
	assert.Len(t, fakeUpstream.GetReceivedMsgs(), 1)

	// 3. 随后发送第二个安全文本
	safeFrame2 := `{"type": "conversation.item.create", "event_id": "evt_2", "item": {"role": "user", "content": [{"type": "text", "text": "second safe text"}]}}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(safeFrame2)))

	time.Sleep(100 * time.Millisecond)
	msgs := fakeUpstream.GetReceivedMsgs()
	require.Len(t, msgs, 2)
	assert.Contains(t, string(msgs[1]), "second safe text")

	_ = testClientConn.Close()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit in time")
	}
}

func TestRealtimeAudit_InfrastructureFailures_CloseConnection(t *testing.T) {
	testCases := []struct {
		name        string
		errCode     string
		payload     string
		expErrCode  string
		setDegraded bool
	}{
		{
			name:       "Guard Unavailable",
			errCode:    promptaudit.ErrorCodeUnavailable,
			payload:    `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "trigger guard error"}]}}`,
			expErrCode: promptaudit.ErrorCodeUnavailable,
		},
		{
			name:        "Config Degraded",
			setDegraded: true,
			payload:     `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "test degraded"}]}}`,
			expErrCode:  promptaudit.ErrorCodeConfigDegraded,
		},
		{
			name:       "Unsupported Protocol - malformed JSON",
			payload:    `{"type": "conversation.item.create", broken...`,
			expErrCode: promptaudit.ErrorCodeUnsupportedProtocol,
		},
		{
			name:       "Unsupported Protocol - unknown type",
			payload:    `{"type": "unknown_shadow_event", "text": "hi"}`,
			expErrCode: promptaudit.ErrorCodeUnsupportedProtocol,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

			origMgr := promptaudit.GetManager()
			origEval := promptaudit.GetEvaluator()
			origStore := promptaudit.GetEventStore()
			defer func() {
				promptaudit.SetGlobalManager(origMgr)
				promptaudit.SetGlobalEvaluator(origEval)
				promptaudit.SetGlobalEventStore(origStore)
			}()

			eval := &mockAuditEvaluator{errCode: tc.errCode}
			store := &mockAuditStore{}
			mgr := promptaudit.NewManager(nil, nil)
			mgr.SetActiveForTest(promptaudit.ActiveConfig{Enabled: true, AllGroups: true}, tc.setDegraded)

			promptaudit.SetGlobalManager(mgr)
			promptaudit.SetGlobalEvaluator(eval)
			promptaudit.SetGlobalEventStore(store)

			fakeUpstream := newFakeUpstreamServer(t)
			defer fakeUpstream.Close()

			clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
			defer cleanupConn()

			info := &relaycommon.RelayInfo{
				ClientWs:        clientWs,
				OriginModelName: "gpt-4o-realtime-preview",
				UsingGroup:      "default",
			}
			info.TargetWsDialer = func() (*websocket.Conn, error) {
				u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
				conn, _, err := websocket.DefaultDialer.Dial(u, nil)
				return conn, err
			}

			handlerDone := make(chan struct{})
			go func() {
				defer close(handlerDone)
				_, _ = OpenaiRealtimeHandler(c, info)
			}()

			err := testClientConn.WriteMessage(websocket.TextMessage, []byte(tc.payload))
			require.NoError(t, err)

			_ = testClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, respData, err := testClientConn.ReadMessage()
			require.NoError(t, err)

			var errEvt dto.RealtimeEvent
			require.NoError(t, common.Unmarshal(respData, &errEvt))
			assert.Equal(t, dto.RealtimeEventTypeError, errEvt.Type)
			assert.Equal(t, tc.expErrCode, errEvt.Error.Code)

			// 验证连接已关闭并退出
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not close on error")
			}

			// 上游未被拨号，收到的帧数为 0
			assert.Equal(t, int64(0), atomic.LoadInt64(&fakeUpstream.dialCount))
			assert.Len(t, fakeUpstream.GetReceivedMsgs(), 0)
		})
	}
}

func TestRealtimeAudit_BufferExceeded_ClosesConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	_, _, cleanupAudit := setupMockAudit(t, true, nil, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	info := &relaycommon.RelayInfo{
		ClientWs:        clientWs,
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}
	info.TargetWsDialer = func() (*websocket.Conn, error) {
		u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		return conn, err
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 连续发送超过 maxRealtimeBufferedFrames (128) 帧纯音频帧，不发任何文本帧
	for i := 0; i <= 130; i++ {
		audioMsg := `{"type": "input_audio_buffer.append", "audio": "AAAA"}`
		_ = testClientConn.WriteMessage(websocket.TextMessage, []byte(audioMsg))
	}

	// 客户端应该收到超出缓冲上限的 error
	_ = testClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, respData, err := testClientConn.ReadMessage()
	require.NoError(t, err)

	var errEvt dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(respData, &errEvt))
	assert.Equal(t, dto.RealtimeEventTypeError, errEvt.Type)
	assert.Equal(t, promptaudit.ErrorCodeUnavailable, errEvt.Error.Code)

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not close on buffer limit exceeded")
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&fakeUpstream.dialCount))
}

func TestRealtimeAudit_PreConsumeDeferredUntilSafeText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	canary := "DANGEROUS_INPUT_FOR_BILLING"
	_, _, cleanupAudit := setupMockAudit(t, true, []string{canary}, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	info := &relaycommon.RelayInfo{
		ClientWs:                clientWs,
		OriginModelName:         "gpt-4o-realtime-preview",
		UsingGroup:              "default",
		NeedDeferredPreConsume:  true,
		DeferredPreConsumeQuota: 500,
	}
	info.TargetWsDialer = func() (*websocket.Conn, error) {
		u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		return conn, err
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 1. 发送危险输入被 Block
	dangerFrame := `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "` + canary + `"}]}}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(dangerFrame)))

	// 读取 Block 错误
	_ = testClientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, respData, err := testClientConn.ReadMessage()
	require.NoError(t, err)

	var errEvt dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(respData, &errEvt))
	assert.Equal(t, promptaudit.ErrorCodeBlocked, errEvt.Error.Code)

	// 断言：由于首帧被阻断，NeedDeferredPreConsume 保持为 true，预扣费未执行！上游拨号为 0
	assert.True(t, info.NeedDeferredPreConsume)
	assert.Equal(t, int64(0), atomic.LoadInt64(&fakeUpstream.dialCount))

	_ = testClientConn.Close()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit")
	}
}

func TestRealtimeAudit_DisabledMode_ImmediateConnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	// 审计关闭
	_, _, cleanupAudit := setupMockAudit(t, false, nil, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	// 审计关闭时，上游连接在进入 handler 前已经拨号建立
	u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
	targetWs, _, err := websocket.DefaultDialer.Dial(u, nil)
	require.NoError(t, err)
	defer targetWs.Close()

	info := &relaycommon.RelayInfo{
		ClientWs:        clientWs,
		TargetWs:        targetWs,
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 发送合法 base64 音频消息，直接透传上游
	msg := `{"type": "input_audio_buffer.append", "audio": "UklGRi4AAABXQVZF"}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(msg)))

	time.Sleep(100 * time.Millisecond)
	msgs := fakeUpstream.GetReceivedMsgs()
	require.Len(t, msgs, 1)
	assert.Contains(t, string(msgs[0]), "input_audio_buffer.append")

	_ = testClientConn.Close()
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit")
	}
}

func TestRealtimeAudit_StorePassEventsBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, storePass := range []bool{false, true} {
		t.Run(map[bool]string{false: "store_pass_false", true: "store_pass_true"}[storePass], func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

			origMgr := promptaudit.GetManager()
			origEval := promptaudit.GetEvaluator()
			origStore := promptaudit.GetEventStore()
			defer func() {
				promptaudit.SetGlobalManager(origMgr)
				promptaudit.SetGlobalEvaluator(origEval)
				promptaudit.SetGlobalEventStore(origStore)
			}()

			eval := &mockAuditEvaluator{}
			store := &mockAuditStore{}
			mgr := promptaudit.NewManager(nil, nil)
			mgr.SetActiveForTest(promptaudit.ActiveConfig{
				Enabled:         true,
				AllGroups:       true,
				StorePassEvents: storePass,
			}, false)

			promptaudit.SetGlobalManager(mgr)
			promptaudit.SetGlobalEvaluator(eval)
			promptaudit.SetGlobalEventStore(store)

			fakeUpstream := newFakeUpstreamServer(t)
			defer fakeUpstream.Close()

			clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
			defer cleanupConn()

			info := &relaycommon.RelayInfo{
				ClientWs:        clientWs,
				OriginModelName: "gpt-4o-realtime-preview",
				UsingGroup:      "default",
			}
			info.TargetWsDialer = func() (*websocket.Conn, error) {
				u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
				conn, _, err := websocket.DefaultDialer.Dial(u, nil)
				return conn, err
			}

			handlerDone := make(chan struct{})
			go func() {
				defer close(handlerDone)
				_, _ = OpenaiRealtimeHandler(c, info)
			}()

			safeMsg := `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "pass prompt"}]}}`
			require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(safeMsg)))

			time.Sleep(100 * time.Millisecond)

			store.mu.Lock()
			decisionsLen := len(store.decisions)
			store.mu.Unlock()

			if storePass {
				assert.Equal(t, 1, decisionsLen, "storePassEvents=true 时应落库 Pass 事件")
			} else {
				assert.Equal(t, 0, decisionsLen, "storePassEvents=false 时不落库 Pass 事件")
			}

			_ = testClientConn.Close()
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not exit")
			}
		})
	}
}

func TestRealtimeAudit_ConcurrencyAndSingleWriterRace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)

	canary := "BLOCKED_KEYWORD"
	_, _, cleanupAudit := setupMockAudit(t, true, []string{canary}, "")
	defer cleanupAudit()

	fakeUpstream := newFakeUpstreamServer(t)
	defer fakeUpstream.Close()

	clientWs, testClientConn, cleanupConn := setupRealtimeTestConnections(t)
	defer cleanupConn()

	info := &relaycommon.RelayInfo{
		ClientWs:        clientWs,
		OriginModelName: "gpt-4o-realtime-preview",
		UsingGroup:      "default",
	}
	info.TargetWsDialer = func() (*websocket.Conn, error) {
		u := "ws" + strings.TrimPrefix(fakeUpstream.server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(u, nil)
		return conn, err
	}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 1. 发送第一帧安全文本激活上游建连
	initMsg := `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "start"}]}}`
	require.NoError(t, testClientConn.WriteMessage(websocket.TextMessage, []byte(initMsg)))

	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(1), atomic.LoadInt64(&fakeUpstream.dialCount))

	// 2. 客户端串行发送（通过客户端测试互斥锁保护测试客户端自身连接），交替发送安全文本与阻断文本
	var clientWriteMu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var msg string
			if idx%2 == 0 {
				msg = `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "safe msg"}]}}`
			} else {
				msg = `{"type": "conversation.item.create", "item": {"role": "user", "content": [{"type": "text", "text": "` + canary + `"}]}}`
			}
			clientWriteMu.Lock()
			_ = testClientConn.WriteMessage(websocket.TextMessage, []byte(msg))
			clientWriteMu.Unlock()
		}(i)
	}
	wg.Wait()

	time.Sleep(150 * time.Millisecond)
	_ = testClientConn.Close()

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not finish gracefully under concurrency")
	}
}
