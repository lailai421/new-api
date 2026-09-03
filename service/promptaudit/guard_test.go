package promptaudit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitRunes_UnicodeRunePreservation(t *testing.T) {
	// 4 个中文字符，每个 3 字节，若按 limit=2 分片，应分为两片，每片两个汉字
	text := "一二三四"
	chunks := SplitRunes(text, 2)
	require.Len(t, chunks, 2)
	assert.Equal(t, "一二", chunks[0])
	assert.Equal(t, "三四", chunks[1])

	// Emoji 测试
	emojiText := "😀😁😂🤣"
	emojiChunks := SplitRunes(emojiText, 2)
	require.Len(t, emojiChunks, 2)
	assert.Equal(t, "😀😁", emojiChunks[0])
	assert.Equal(t, "😂🤣", emojiChunks[1])

	// 优先分段测试
	priorityText := "最新用户输入" + PrioritySeparator + "历史对话内容"
	pChunks := SplitRunes(priorityText, 4)
	require.Len(t, pChunks, 4)
	assert.Equal(t, "最新用户", pChunks[0])
	assert.Equal(t, "输入", pChunks[1])
	assert.Equal(t, "历史对话", pChunks[2])
	assert.Equal(t, "内容", pChunks[3])
}

func TestAggregateResults_HighestSeverityWins(t *testing.T) {
	c1 := &NormalizedResult{
		Decision:        EventPass,
		RiskLevel:       RiskLow,
		Action:          ActionAllow,
		Safety:          "Safe",
		Categories:      []string{},
		MatchedScanners: []string{},
	}
	c2 := &NormalizedResult{
		Decision:        EventFlag,
		RiskLevel:       RiskMedium,
		Action:          ActionWarn,
		Safety:          "Controversial",
		Categories:      []string{"politically_sensitive_topics"},
		MatchedScanners: []string{"politically_sensitive_topics"},
		ScannerScores:   map[string]float64{"politically_sensitive_topics": 0.5},
	}
	c3 := &NormalizedResult{
		Decision:        EventCritical,
		RiskLevel:       RiskCritical,
		Action:          ActionBlock,
		Safety:          "Unsafe",
		Categories:      []string{"violent"},
		MatchedScanners: []string{"violent"},
		ScannerScores:   map[string]float64{"violent": 1.0},
		GuardEndpointID: "ep-risk",
	}

	agg, err := AggregateResults([]*NormalizedResult{c1, c2, c3}, 120*time.Millisecond)
	require.NoError(t, err)

	// 最高风险结论胜出
	assert.Equal(t, EventCritical, agg.Decision)
	assert.Equal(t, RiskCritical, agg.RiskLevel)
	assert.Equal(t, ActionBlock, agg.Action)
	assert.Equal(t, "ep-risk", agg.GuardEndpointID)
	assert.Equal(t, 3, agg.ChunkTotal)
	assert.Equal(t, 120, agg.LatencyMS)

	// 合并所有的 Categories 与 MatchedScanners
	assert.Contains(t, agg.MatchedScanners, "violent")
	assert.Contains(t, agg.MatchedScanners, "politically_sensitive_topics")
	assert.Equal(t, 1.0, agg.ScannerScores["violent"])
	assert.Equal(t, 0.5, agg.ScannerScores["politically_sensitive_topics"])
}

func TestOpenAICompatibleScanner_RequestContract(t *testing.T) {
	var receivedToken string
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		receivedToken = r.Header.Get("Authorization")

		err := common.DecodeJson(r.Body, &receivedPayload)
		assert.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"content": "Safety: Safe\nCategories: None"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	endpoint := ActiveEndpoint{
		ID:         "test-node",
		BaseURL:    server.URL,
		Model:      DefaultGuardModel,
		Token:      "my-guard-token",
		TimeoutMS:  1000,
		InputLimit: 4000,
		Enabled:    true,
	}

	res, err := scanner.Scan(context.Background(), endpoint, "测试输入文本", AllScannerIDs)
	require.NoError(t, err)
	assert.Equal(t, EventPass, res.Decision)
	assert.Equal(t, ActionAllow, res.Action)
	assert.Equal(t, "Bearer my-guard-token", receivedToken)
	assert.Equal(t, DefaultGuardModel, receivedPayload["model"])
	assert.Equal(t, float64(42), receivedPayload["seed"])
	assert.Equal(t, float64(0), receivedPayload["temperature"])
}

func TestOpenAICompatibleScanner_RejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回超过 256 KiB 的响应
		oversize := strings.Repeat("A", int(MaxGuardResponseBytes)+10)
		_, _ = w.Write([]byte(oversize))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	endpoint := ActiveEndpoint{
		ID:        "oversize-node",
		BaseURL:   server.URL,
		TimeoutMS: 1000,
	}

	_, err := scanner.Scan(context.Background(), endpoint, "input", AllScannerIDs)
	require.Error(t, err)
	var gErr *GuardError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, ErrorCodeInvalidResponse, gErr.Code)
}

func TestGuardEvaluator_OrderedFailover(t *testing.T) {
	var node1Calls int32
	var node2Calls int32

	serverNode1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&node1Calls, 1)
		// 节点 1 返回 500 可重试错误
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverNode1.Close()

	serverNode2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&node2Calls, 1)
		// 节点 2 正常响应 Safe
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer serverNode2.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:   true,
		Scanners:  AllScannerIDs,
		AllGroups: true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: serverNode1.URL, TimeoutMS: 1000, Enabled: true},
			{ID: "node-2", BaseURL: serverNode2.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	snapshot := PromptSnapshot{
		RequestID:    "req-failover-1",
		ScanText:     "test failover",
		PromptLength: 13,
	}

	decision, err := evaluator.Evaluate(context.Background(), cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decision.Kind)
	assert.True(t, decision.AllowNextStage)

	// 验证按顺序先调用 Node 1，失败后再调用 Node 2
	assert.Equal(t, int32(1), atomic.LoadInt32(&node1Calls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&node2Calls))
}

func TestGuardEvaluator_NonRetryableErrorDoesNotFailover(t *testing.T) {
	var node1Calls int32
	var node2Calls int32

	serverNode1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&node1Calls, 1)
		// 节点 1 返回 400 Bad Request（不可重试配置错误）
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer serverNode1.Close()

	serverNode2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&node2Calls, 1)
	}))
	defer serverNode2.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:   true,
		Scanners:  AllScannerIDs,
		AllGroups: true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: serverNode1.URL, TimeoutMS: 1000, Enabled: true},
			{ID: "node-2", BaseURL: serverNode2.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	snapshot := PromptSnapshot{
		RequestID: "req-non-retryable",
		ScanText:  "test non retryable",
	}

	_, err := evaluator.Evaluate(context.Background(), cfg, snapshot)
	require.Error(t, err)

	// 节点 1 调用了，但节点 2 绝不能被调用
	assert.Equal(t, int32(1), atomic.LoadInt32(&node1Calls))
	assert.Equal(t, int32(0), atomic.LoadInt32(&node2Calls))
}

func TestGuardEvaluator_EarlyTerminationOnBlock(t *testing.T) {
	var scanCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddInt32(&scanCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if idx == 1 {
			// 第一块直接返回 Unsafe Block
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Violent"}}]}`))
			return
		}
		// 第二块正常不应被调用
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:   true,
		Scanners:  AllScannerIDs,
		AllGroups: true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, InputLimit: 10, Enabled: true},
		},
	}

	// 长度为 30 的文本，input_limit=10，共切为 3 块
	longText := strings.Repeat("一二三四五六七八九十", 3)
	snapshot := PromptSnapshot{
		RequestID: "req-early-term",
		ScanText:  longText,
	}

	decision, err := evaluator.Evaluate(context.Background(), cfg, snapshot)
	require.NoError(t, err)

	assert.Equal(t, DecisionBlock, decision.Kind)
	assert.False(t, decision.AllowNextStage)
	assert.Equal(t, 403, decision.HTTPStatus)

	// 验证第一块阻断后立即终止，总调用次数仅为 1
	assert.Equal(t, int32(1), atomic.LoadInt32(&scanCount))
}

func TestGuardEvaluator_BulkheadSaturation(t *testing.T) {
	scanner := NewOpenAICompatibleScanner()
	// 设置全局并发上限为 1
	evaluator := NewGuardEvaluatorWithLimits(scanner, 1, 1)

	// 手动占用全局信号量
	evaluator.globalSem <- struct{}{}

	cfg := ActiveConfig{
		Enabled:   true,
		Scanners:  AllScannerIDs,
		AllGroups: true,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: "http://127.0.0.1:8000", TimeoutMS: 1000, Enabled: true},
		},
	}

	snapshot := PromptSnapshot{
		RequestID: "req-bulkhead-full",
		ScanText:  "text",
	}

	_, err := evaluator.Evaluate(context.Background(), cfg, snapshot)
	require.Error(t, err)

	var gErr *GuardError
	require.ErrorAs(t, err, &gErr)
	assert.Equal(t, ErrorCodeUnavailable, gErr.Code)
	assert.Equal(t, 503, gErr.HTTPStatus)
}
