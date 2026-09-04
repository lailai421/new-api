package promptaudit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAgentsMdEnvelope(t *testing.T) {
	// 1. 标准信封
	standard := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n工程师工作规范...\n</INSTRUCTIONS>"
	assert.True(t, IsAgentsMdEnvelope(standard))

	// 2. 带首尾空白的标准信封
	standardWithSpaces := "   \n# AGENTS.md instructions\n\n<INSTRUCTIONS>\n规范\n</INSTRUCTIONS>  \n"
	assert.True(t, IsAgentsMdEnvelope(standardWithSpaces))

	// 3. 替换通知
	replaceNotice := "# AGENTS.md instructions\n\nThese AGENTS.md instructions replace all previously provided AGENTS.md instructions."
	assert.True(t, IsAgentsMdEnvelope(replaceNotice))

	// 4. 移除通知
	removeNotice := "# AGENTS.md instructions\n\nThe previously provided AGENTS.md instructions no longer apply."
	assert.True(t, IsAgentsMdEnvelope(removeNotice))

	// 5. 用户普通讨论：不含信封或通知标记 -> false
	userChat := "请根据 AGENTS.md 写个 README"
	assert.False(t, IsAgentsMdEnvelope(userChat))

	// 6. 碰巧有标题但无开闭标签和通知 -> false
	fakeHeader := "# AGENTS.md instructions 是一个好规范吗？"
	assert.False(t, IsAgentsMdEnvelope(fakeHeader))

	// 7. 标签顺序颠倒 -> false
	reversedTags := "# AGENTS.md instructions\n\n</INSTRUCTIONS>\n<INSTRUCTIONS>"
	assert.False(t, IsAgentsMdEnvelope(reversedTags))
}

// AC1, AC2, AC3: 人设 + 信封 + 你好，Guard 收到只有你好，预览以你好开头，哈希与长度只基于真实文本。
func TestAgentsMd_ExcludedFromScanAndStore(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	instJSON, err := common.Marshal("You are Codex, a programming assistant.")
	require.NoError(t, err)

	envelopeText := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n仓库规范：必须遵循严苛的提交与审查流程，禁止随意改动。\n</INSTRUCTIONS>"
	userPrompt := "你好"

	inputJSON, err := common.Marshal([]any{
		map[string]any{"role": "user", "content": envelopeText},
		map[string]any{"role": "user", "content": userPrompt},
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

	// AC1: FullPrompt 和 ScanText 绝不包含 AGENTS.md 信封与人设，仅包含「你好」
	assert.Equal(t, "你好", snapshot.FullPrompt)
	assert.Equal(t, "你好", snapshot.ScanText)
	assert.NotContains(t, snapshot.FullPrompt, "# AGENTS.md instructions")
	assert.NotContains(t, snapshot.FullPrompt, "仓库规范")
	assert.NotContains(t, snapshot.FullPrompt, "You are Codex")
	assert.NotContains(t, snapshot.ScanText, "# AGENTS.md instructions")
	assert.NotContains(t, snapshot.ScanText, "仓库规范")

	// AC2: 预览以「你好」开头，详情解密只有真实文本
	assert.Equal(t, "你好", snapshot.RedactedPreview)
	assert.False(t, strings.HasPrefix(snapshot.RedactedPreview, "# AGENTS.md instructions"))

	// AC3: 长度为 2 (utf8 rune)，条数为 1，哈希与「你好」一致
	assert.Equal(t, 2, snapshot.PromptLength)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.Equal(t, CalculatePromptHash("你好"), snapshot.PromptHash)

	// 验证落库密文解密后仅有真实文本
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	ctx := context.Background()
	allowDecision := &Decision{Kind: DecisionAllow, HTTPStatus: 200, AllowNextStage: true}
	err = store.Record(ctx, snapshot, allowDecision, true)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: snapshot.RequestID})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)

	fullEvent, err := model.GetPromptAuditEventByID(ctx, events[0].Id)
	require.NoError(t, err)
	decrypted, err := encryptor.DecryptPrompt(string(fullEvent.FullPromptCiphertext))
	require.NoError(t, err)
	assert.Equal(t, "你好", decrypted)
	assert.NotContains(t, decrypted, "仓库规范")
	assert.NotContains(t, decrypted, "# AGENTS.md instructions")
}

// 混合段测试：一段里信封和真实提示词拼在一起，去掉信封块，保留剩余真实文本。
func TestAgentsMd_MixedSegment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 情况 1：信封在后，真实文本在前
	mixed1 := "请帮我实现快速排序算法\n\n# AGENTS.md instructions\n\n<INSTRUCTIONS>\n工程师规约全文...\n</INSTRUCTIONS>"
	segments1 := []PromptSegment{
		{Role: "user", Content: mixed1},
	}
	snapshot1, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments1, false)
	require.NoError(t, err)
	assert.Equal(t, "请帮我实现快速排序算法", snapshot1.FullPrompt)
	assert.Equal(t, "请帮我实现快速排序算法", snapshot1.ScanText)
	assert.Equal(t, 1, snapshot1.MessageCount)

	// 情况 2：信封在前，真实文本在后
	mixed2 := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n规约...\n</INSTRUCTIONS>\n\n分析这个代码片段"
	segments2 := []PromptSegment{
		{Role: "user", Content: mixed2},
	}
	snapshot2, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments2, false)
	require.NoError(t, err)
	assert.Equal(t, "分析这个代码片段", snapshot2.FullPrompt)
	assert.Equal(t, "分析这个代码片段", snapshot2.ScanText)
	assert.Equal(t, 1, snapshot2.MessageCount)
}

// 替换通知与移除通知信封整段丢弃
func TestAgentsMd_NoticeVariants_Discarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	segments := []PromptSegment{
		{Role: "user", Content: "# AGENTS.md instructions\n\nThese AGENTS.md instructions replace all previously provided AGENTS.md instructions."},
		{Role: "user", Content: "真实需求第一轮"},
		{Role: "user", Content: "# AGENTS.md instructions\n\nThe previously provided AGENTS.md instructions no longer apply."},
		{Role: "user", Content: "真实需求第二轮"},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Equal(t, "真实需求第一轮\n\n真实需求第二轮", snapshot.FullPrompt)
	assert.Equal(t, 2, snapshot.MessageCount)
	assert.NotContains(t, snapshot.FullPrompt, "replace")
	assert.NotContains(t, snapshot.FullPrompt, "longer apply")
}

// AC4: 用户输入「请根据 AGENTS.md 写个 README」无信封标记，整段仍送审、仍落库。
func TestAgentsMd_UserMention_Kept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	userPrompt := "请根据 AGENTS.md 写个 README，并在文档中说明架构"
	segments := []PromptSegment{
		{Role: "user", Content: userPrompt},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Equal(t, userPrompt, snapshot.FullPrompt)
	assert.Equal(t, userPrompt, snapshot.ScanText)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.Equal(t, userPrompt, snapshot.RedactedPreview)
}

// AC5: 过滤后只剩信封、没有真实 user 时：不发起 Guard HTTP，返回 Allow，不插入 prompt_audit_events。
func TestAgentsMd_OnlyEnvelope_NoHttp_NoStore(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var guardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"safety\":\"Safe\",\"categories\":[]}"}}]}`))
	}))
	defer server.Close()

	// 请求中只有一条 user 消息，且完全是 AGENTS.md 信封
	envelopeText := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n全部是仓库规约内容\n</INSTRUCTIONS>"
	segments := []PromptSegment{
		{Role: "user", Content: envelopeText},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Empty(t, snapshot.FullPrompt, "只有信封时 FullPrompt 必须为空")
	assert.Empty(t, snapshot.ScanText, "只有信封时 ScanText 必须为空")
	assert.Equal(t, 0, snapshot.MessageCount)

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        AllScannerIDs,
		AllGroups:       true,
		StorePassEvents: true, // 即使开启了保存 Pass
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	ctx := context.Background()
	decision, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decision.Kind)
	assert.True(t, decision.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls), "过滤后无真实用户输入时绝不得发起 Guard HTTP")

	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	err = store.Record(ctx, snapshot, decision, true)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{RequestID: snapshot.RequestID})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "过滤后无真实用户输入的 Allow 绝不得落库")
	assert.Empty(t, events)
}

// AC6: latest_turn_only=true 时只送审最新一轮真实 user；落库仍是该请求全部真实 user（不含信封）。
func TestAgentsMd_LatestTurnOnly_Behavior(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	envelopeText := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n规约...\n</INSTRUCTIONS>"
	segments := []PromptSegment{
		{Role: "user", Content: envelopeText},
		{Role: "user", Content: "历史首轮真实需求"},
		{Role: "assistant", Content: "历史助手回复"},
		{Role: "user", Content: "最新追问真实需求"},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, true)
	require.NoError(t, err)

	assert.Equal(t, "latest_turn", snapshot.AuditScope)
	// 送审文本只有最新轮真实 user
	assert.Equal(t, "最新追问真实需求", snapshot.ScanText)
	// 落库文本为全部真实 user（首轮 + 最新追问），不含信封与 assistant
	assert.Equal(t, "历史首轮真实需求\n\n最新追问真实需求", snapshot.FullPrompt)
	assert.Equal(t, 2, snapshot.MessageCount)
	assert.NotContains(t, snapshot.FullPrompt, "# AGENTS.md instructions")
}

// 修正前任务 AC7：标准标题生成请求抽出 User prompt 正文送审与落库；非标准模板用户讨论整段送审。
func TestAgentsMd_TitleGeneration_UnwrappedOrKept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. 标准 Codex 标题生成模板：抽出 User prompt: 正文
	standardTitleTemplate := "Generate a concise, single-line task title of at most 36 characters for the following conversation.\nDo not answer the request.\n\nUser prompt:\n你是什么模型？"
	envelopeText := "# AGENTS.md instructions\n\n<INSTRUCTIONS>\n规约\n</INSTRUCTIONS>"
	segments1 := []PromptSegment{
		{Role: "user", Content: envelopeText},
		{Role: "user", Content: standardTitleTemplate},
	}

	snapshot1, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments1, false)
	require.NoError(t, err)
	assert.Equal(t, "你是什么模型？", snapshot1.FullPrompt)
	assert.Equal(t, "你是什么模型？", snapshot1.ScanText)
	assert.Equal(t, 1, snapshot1.MessageCount)

	// 2. 非标准模板用户讨论：整段仍送审
	nonTemplatePrompt := "请为以下对话生成标题：Generate a concise, single-line task title for the following conversation..."
	segments2 := []PromptSegment{
		{Role: "user", Content: envelopeText},
		{Role: "user", Content: nonTemplatePrompt},
	}

	snapshot2, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments2, false)
	require.NoError(t, err)
	assert.Equal(t, nonTemplatePrompt, snapshot2.FullPrompt)
	assert.Equal(t, nonTemplatePrompt, snapshot2.ScanText)
	assert.Equal(t, 1, snapshot2.MessageCount)
}
