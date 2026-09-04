package promptaudit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC1, AC2, AC9, AC12: Codex 人设与工具输出不入库、不送审；预览不以人设开头；哈希与长度只基于用户文本。
func TestUserScan_CodexInstructionsAndToolsExcluded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	instJSON, err := common.Marshal("You are Codex, an agent based on GPT-5. Follow strict engineering guidelines and act as CLI assistant.")
	require.NoError(t, err)

	inputJSON, err := common.Marshal([]any{
		map[string]any{"role": "user", "content": "第一轮用户指令：请写一个斐波那契函数"},
		map[string]any{
			"type": "function_call",
			"name": "read_file",
			"call": map[string]any{"path": "fib.py"},
		},
		map[string]any{
			"type":   "function_call_output",
			"output": "def fib(n): return n if n < 2 else fib(n-1) + fib(n-2)",
		},
		map[string]any{"role": "assistant", "content": "这是我读取的旧实现，可以重构优化。"},
		map[string]any{"role": "user", "content": "第二轮用户指令：请帮我添加单元测试"},
	})
	require.NoError(t, err)

	respReq := &dto.OpenAIResponsesRequest{
		Model:        "gpt-5.6-sol",
		Instructions: instJSON,
		Input:        inputJSON,
	}

	segments, protocol, modelName, err := ExtractRelayRequest(respReq, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_responses", protocol)
	assert.Equal(t, "gpt-5.6-sol", modelName)

	snapshot, err := BuildPromptSnapshot(c, nil, protocol, modelName, segments, false)
	require.NoError(t, err)

	// 1. FullPrompt 仅包含两个用户提示词，且原顺序拼接
	expectedUserFull := "第一轮用户指令：请写一个斐波那契函数\n\n第二轮用户指令：请帮我添加单元测试"
	assert.Equal(t, expectedUserFull, snapshot.FullPrompt)

	// 2. 绝对不能包含 Codex 人设、assistant 回复、工具名称与工具输出正文
	assert.NotContains(t, snapshot.FullPrompt, "You are Codex")
	assert.NotContains(t, snapshot.FullPrompt, "read_file")
	assert.NotContains(t, snapshot.FullPrompt, "这是我读取的旧实现")
	assert.NotContains(t, snapshot.FullPrompt, "def fib(n)")

	// 3. ScanText 包含全部 user，且最新 user 作为优先扫描段
	assert.Contains(t, snapshot.ScanText, "第二轮用户指令：请帮我添加单元测试")
	assert.Contains(t, snapshot.ScanText, "第一轮用户指令：请写一个斐波那契函数")
	assert.NotContains(t, snapshot.ScanText, "You are Codex")
	assert.NotContains(t, snapshot.ScanText, "read_file")

	// 4. RedactedPreview 来自用户送审文本，绝不以人设开头
	assert.False(t, strings.HasPrefix(snapshot.RedactedPreview, "You are Codex"))
	assert.True(t, strings.HasPrefix(snapshot.RedactedPreview, "第二轮用户指令") || strings.HasPrefix(snapshot.RedactedPreview, "第一轮用户指令"))

	// 5. PromptHash / PromptLength / MessageCount 均按用户提示词计算 (AC12)
	assert.Equal(t, CalculatePromptHash(expectedUserFull), snapshot.PromptHash)
	assert.Equal(t, utf8.RuneCountInString(expectedUserFull), snapshot.PromptLength)
	assert.Equal(t, 2, snapshot.MessageCount)
	assert.Equal(t, "user", snapshot.AuditScope)
}

// AC3, AC5: latest_turn_only 行为：仅缩小送审范围（最新连续 user），落库仍为全部 user；无 user 时不退回全文。
func TestUserScan_LatestTurnOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	instJSON, err := common.Marshal("You are a coding agent running in the Codex CLI.")
	require.NoError(t, err)

	inputJSON, err := common.Marshal([]any{
		map[string]any{"role": "user", "content": "首轮历史需求"},
		map[string]any{"role": "assistant", "content": "历史助手回答"},
		map[string]any{"role": "user", "content": "最新追问内容"},
	})
	require.NoError(t, err)

	respReq := &dto.OpenAIResponsesRequest{
		Model:        "gpt-5.6-sol",
		Instructions: instJSON,
		Input:        inputJSON,
	}

	segments, protocol, modelName, err := ExtractRelayRequest(respReq, c)
	require.NoError(t, err)

	// latestTurnOnly = true
	snapshot, err := BuildPromptSnapshot(c, nil, protocol, modelName, segments, true)
	require.NoError(t, err)

	assert.Equal(t, "latest_turn", snapshot.AuditScope)
	// 送审文本仅为最新轮 user，不附带 assistant，不附带历史 user
	assert.Equal(t, "最新追问内容", snapshot.ScanText)
	assert.NotContains(t, snapshot.ScanText, "历史助手回答")
	assert.NotContains(t, snapshot.ScanText, "首轮历史需求")
	assert.NotContains(t, snapshot.ScanText, "You are a coding agent")

	// 落库正文仍为全部 user（首轮 + 最新追问），不含人设与 assistant
	expectedAllUsers := "首轮历史需求\n\n最新追问内容"
	assert.Equal(t, expectedAllUsers, snapshot.FullPrompt)
	assert.Equal(t, 2, snapshot.MessageCount)

	// AC5: 只有人设/工具结果，没有 user 时，不得退回扫描或落库人设全文
	noUserInputJSON, err := common.Marshal([]any{
		map[string]any{"type": "function_call_output", "output": "some tool output"},
	})
	require.NoError(t, err)

	respReqNoUser := &dto.OpenAIResponsesRequest{
		Model:        "gpt-5.6-sol",
		Instructions: instJSON,
		Input:        noUserInputJSON,
	}
	noUserSegments, protoNoUser, modelNoUser, err := ExtractRelayRequest(respReqNoUser, c)
	require.NoError(t, err)

	noUserSnapshot, err := BuildPromptSnapshot(c, nil, protoNoUser, modelNoUser, noUserSegments, true)
	require.NoError(t, err)
	assert.Empty(t, noUserSnapshot.FullPrompt, "无 user 时 FullPrompt 必须为空，不得退回人设全文")
	assert.Empty(t, noUserSnapshot.ScanText, "无 user 时 ScanText 必须为空，不得退回人设全文")
	assert.Empty(t, noUserSnapshot.RedactedPreview)
	assert.Equal(t, 0, noUserSnapshot.MessageCount)
}

// AC4: 只有 system/instructions 没有 user 时，Evaluate 不打 Guard HTTP，返回 Allow，且不写事件表。
func TestUserScan_NoUser_EvaluateAndStore_Skip(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	var guardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"safety\":\"Safe\",\"categories\":[]}"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        AllScannerIDs,
		AllGroups:       true,
		StorePassEvents: true, // 即使开启 StorePass
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	snapshot := PromptSnapshot{
		RequestID:    "req-no-user-skip",
		FullPrompt:   "",
		ScanText:     "",
		PromptLength: 0,
		AuditScope:   "user",
	}

	decision, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decision.Kind)
	assert.True(t, decision.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls), "无用户输入时 Evaluate 绝不得发起 Guard HTTP")

	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	err = store.Record(ctx, snapshot, decision, true)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-no-user-skip"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "无用户输入的 Allow 事件绝不得插入事件表")
	assert.Empty(t, events)
}

// AC6: 进程内短缓存判定（Allow/Block）、配置版本失效、失败不缓存、自定义时钟验证 TTL。
func TestUserScan_DecisionCache_Behavior(t *testing.T) {
	ctx := context.Background()

	var guardCalls int32
	var returnBlocked int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		if atomic.LoadInt32(&returnBlocked) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Violent"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	simulatedTime := time.Now()
	evaluator.SetCacheClock(func() time.Time {
		return simulatedTime
	})

	cfg := ActiveConfig{
		Enabled:       true,
		ConfigVersion: 100,
		Scanners:      []string{"violent"},
		ScannersMap:   map[string]struct{}{"violent": {}},
		AllGroups:     true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	snapshot := PromptSnapshot{
		RequestID: "req-cache-1",
		ScanText:  "用户提问内容",
	}

	// 1. 第一次调用：未命中缓存，发起 HTTP 调用
	dec1, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, dec1.Kind)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls))

	// 2. 第二次相同请求：命中缓存，调用次数保持为 1
	dec2, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, dec2.Kind)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls))

	// 2b. 命中后续期：空闲 50 分钟仍命中，不发起新的 HTTP
	simulatedTime = simulatedTime.Add(50 * time.Minute)
	dec2b, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, dec2b.Kind)
	assert.True(t, dec2b.FromCache)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls))

	// 3. 配置版本变更：缓存失效，发起第 2 次 HTTP 调用
	cfgV2 := cfg
	cfgV2.ConfigVersion = 101
	dec3, err := evaluator.Evaluate(ctx, cfgV2, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, dec3.Kind)
	assert.Equal(t, int32(2), atomic.LoadInt32(&guardCalls))

	// 4. 自定义时钟前进 61 分钟（超过 60 分钟空闲 TTL）：缓存失效，发起第 3 次 HTTP 调用（禁止 Sleep）
	simulatedTime = simulatedTime.Add(61 * time.Minute)
	dec4, err := evaluator.Evaluate(ctx, cfgV2, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, dec4.Kind)
	assert.Equal(t, int32(3), atomic.LoadInt32(&guardCalls))

	// 5. 错误/不可用不被缓存
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	cfgFail := cfg
	cfgFail.ConfigVersion = 200
	cfgFail.Endpoints = []ActiveEndpoint{
		{ID: "node-fail", BaseURL: failingServer.URL, TimeoutMS: 1000, Enabled: true},
	}

	snapshotFail := PromptSnapshot{
		RequestID: "req-fail",
		ScanText:  "会失败的请求文本",
	}

	callsBefore := atomic.LoadInt32(&guardCalls)
	_, err = evaluator.Evaluate(ctx, cfgFail, snapshotFail)
	require.Error(t, err)
	assert.Equal(t, callsBefore+1, atomic.LoadInt32(&guardCalls))

	// 第二次请求依然打到服务端，不被错误缓存
	_, err = evaluator.Evaluate(ctx, cfgFail, snapshotFail)
	require.Error(t, err)
	assert.Equal(t, callsBefore+2, atomic.LoadInt32(&guardCalls))
}

// AC7: 超过 8000 rune 文本远程只按截断分片；未截断 ScanText 可被本地启发式识别；FullPrompt 未截断。
func TestUserScan_MaxRemoteScanRunes_Truncation(t *testing.T) {
	ctx := context.Background()

	var chunksReceived []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = common.DecodeJson(r.Body, &req)
		if messages, ok := req["messages"].([]any); ok && len(messages) > 0 {
			if first, ok := messages[0].(map[string]any); ok {
				if content, ok := first["content"].(string); ok {
					chunksReceived = append(chunksReceived, content)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:     true,
		Scanners:    []string{"violent"},
		ScannersMap: map[string]struct{}{"violent": {}},
		AllGroups:   true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, InputLimit: 4000, Enabled: true},
		},
	}

	// 构造 10000 个中文字符（远超 MaxRemoteScanRunes 8000）
	longUserText := strings.Repeat("字", 10000)
	snapshot := PromptSnapshot{
		RequestID:    "req-trunc",
		ScanText:     longUserText,
		FullPrompt:   longUserText,
		PromptLength: 10000,
	}

	decision, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decision.Kind)

	// 8000 rune 被 input_limit=4000 均分为恰好 2 个分片（若不截断则会产生 3 个分片）
	require.Len(t, chunksReceived, 2)
	totalRunes := utf8.RuneCountInString(chunksReceived[0]) + utf8.RuneCountInString(chunksReceived[1])
	assert.Equal(t, MaxRemoteScanRunes, totalRunes)
}

// AC8: Guard 失败事件记录真实耗时（latency_ms 非 0），且密文仅保存用户提示词。
func TestUserScan_FailureLatencyMS_Recorded(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟后端轻微耗时后返回 500
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:   true,
		Scanners:  AllScannerIDs,
		AllGroups: true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	userPrompt := "用户合法的提问内容"
	snapshot := PromptSnapshot{
		RequestID:    "req-fail-latency",
		FullPrompt:   userPrompt,
		ScanText:     userPrompt,
		PromptLength: utf8.RuneCountInString(userPrompt),
		AuditScope:   "user",
	}

	_, evalErr := evaluator.Evaluate(ctx, cfg, snapshot)
	require.Error(t, evalErr)

	var gErr *GuardError
	require.True(t, errors.As(evalErr, &gErr))
	assert.Equal(t, ErrorCodeUnavailable, gErr.Code)
	assert.Greater(t, gErr.LatencyMS, 0, "GuardError 中的 LatencyMS 必须大于 0")

	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	// 模拟 Gate 传递真实 LatencyMS 落库
	failDecision := &Decision{
		Kind:      DecisionUnavailable,
		ErrorCode: gErr.Code,
		LatencyMS: gErr.LatencyMS,
	}
	err = store.Record(ctx, snapshot, failDecision, true)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: "req-fail-latency"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	assert.Equal(t, "unavailable", events[0].Decision)
	assert.Greater(t, events[0].LatencyMS, int64(0), "数据库中的 LatencyMS 必须反映实际耗时，绝不能为 0")

	// 校验密文仅包含用户提示词（GetPromptAuditEventByID 查出完整密文字段）
	fullEvent, err := model.GetPromptAuditEventByID(ctx, events[0].Id)
	require.NoError(t, err)
	decrypted, err := encryptor.DecryptPrompt(string(fullEvent.FullPromptCiphertext))
	require.NoError(t, err)
	assert.Equal(t, userPrompt, decrypted)
}
