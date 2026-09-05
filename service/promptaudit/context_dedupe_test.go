package promptaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC1, AC2: 环境上下文 + 你是什么模型？，Guard 收到只有这句，预览只有这句，解密只有这句，长度接近 7，条数 1。
func TestCodexContext_StrippingAndSnapshot(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	envContext := `<environment_context>
  <cwd>/Users/admin/project</cwd>
  <shell>zsh</shell>
  <current_date>2026-09-04</current_date>
  <timezone>Asia/Shanghai</timezone>
  <filesystem>
    Directory contents...
  </filesystem>
</environment_context>`

	userPrompt := "你是什么模型？"

	inputJSON, err := common.Marshal([]any{
		map[string]any{"role": "user", "content": envContext},
		map[string]any{"role": "user", "content": userPrompt},
	})
	require.NoError(t, err)

	respReq := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: inputJSON,
	}

	segments, protocol, modelName, err := ExtractRelayRequest(respReq, c)
	require.NoError(t, err)

	// 提取层仍能看到包含环境上下文的原始分段
	require.Len(t, segments, 2)
	assert.Contains(t, segments[0].Content, "<environment_context>")

	snapshot, err := BuildPromptSnapshot(c, nil, protocol, modelName, segments, false)
	require.NoError(t, err)

	// AC1: Guard 送审文本与 FullPrompt 绝不含环境上下文、cwd、filesystem
	assert.Equal(t, "你是什么模型？", snapshot.ScanText)
	assert.Equal(t, "你是什么模型？", snapshot.FullPrompt)
	assert.NotContains(t, snapshot.ScanText, "<environment_context>")
	assert.NotContains(t, snapshot.ScanText, "<cwd>")
	assert.NotContains(t, snapshot.ScanText, "<filesystem>")
	assert.NotContains(t, snapshot.FullPrompt, "<environment_context>")
	assert.NotContains(t, snapshot.FullPrompt, "<cwd>")
	assert.NotContains(t, snapshot.FullPrompt, "<filesystem>")

	// AC2: 预览等于「你是什么模型？」，长度等于 7 rune，条数等于 1，哈希与该句一致
	assert.Equal(t, "你是什么模型？", snapshot.RedactedPreview)
	assert.Equal(t, 7, snapshot.PromptLength)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.Equal(t, CalculatePromptHash("你是什么模型？"), snapshot.PromptHash)

	// 验证落库与解密正文仅有用户真实提问
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
	assert.Equal(t, "你是什么模型？", decrypted)
	assert.NotContains(t, decrypted, "<environment_context>")
}

// 其它成对标记剥离与混合段切除测试
func TestCodexContext_OtherPairedTagsAndMixedSegments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. skills, plugins, user_shell_command, turn_aborted
	tags := []struct {
		open  string
		close string
	}{
		{"<skills_instructions>", "</skills_instructions>"},
		{"<plugins_instructions>", "</plugins_instructions>"},
		{"<user_shell_command>", "</user_shell_command>"},
		{"<turn_aborted>", "</turn_aborted>"},
		{"<ENVIRONMENT_CONTEXT>", "</ENVIRONMENT_CONTEXT>"}, // ASCII 大小写不敏感
	}

	for _, tag := range tags {
		content := fmt.Sprintf("%s\n内部自动指令\n%s", tag.open, tag.close)
		segments := []PromptSegment{
			{Role: "user", Content: content},
			{Role: "user", Content: "真实用户需求"},
		}
		snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
		require.NoError(t, err)
		assert.Equal(t, "真实用户需求", snapshot.FullPrompt, "标签 %s 必须被完整排除", tag.open)
		assert.Equal(t, 1, snapshot.MessageCount)
	}

	// 2. 混合段：同一段中既有标记块又有真实用户正文
	mixedText := "<ENVIRONMENT_CONTEXT>\n<cwd>/tmp</cwd>\n</ENVIRONMENT_CONTEXT>\n\n请帮我写一段冒泡排序代码"
	mixedSegments := []PromptSegment{
		{Role: "user", Content: mixedText},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", mixedSegments, false)
	require.NoError(t, err)
	assert.Equal(t, "请帮我写一段冒泡排序代码", snapshot.FullPrompt)
	assert.Equal(t, "请帮我写一段冒泡排序代码", snapshot.ScanText)
	assert.Equal(t, 1, snapshot.MessageCount)
}

// AC3 前半部分：标题生成模板（含环境上下文）过滤后 ScanText 也是「你是什么模型？」。
func TestCodexContext_TitleGeneration_UnwrapToAuthenticPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	titlePayload := `<environment_context>
  <cwd>/home/user/code</cwd>
  <timezone>UTC</timezone>
</environment_context>

Generate a concise, single-line task title of at most 36 characters for the following conversation.
Do not answer the request.

User prompt:
你是什么模型？`

	segments := []PromptSegment{
		{Role: "user", Content: titlePayload},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Equal(t, "你是什么模型？", snapshot.ScanText)
	assert.Equal(t, "你是什么模型？", snapshot.FullPrompt)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.Equal(t, 7, snapshot.PromptLength)
	assert.Equal(t, "你是什么模型？", snapshot.RedactedPreview)
}

const sampleCodexCatchUp = `Write a brief catch-up for a user returning to this Codex task. In at most 40 words and one or two plain-text sentences, explain the objective, what was completed or learned, and the next step or blocker. Mention changed files, tests, approvals, or requested decisions only when relevant. Never claim changes were made or tests passed unless the conversation confirms it. If the task is complete, say so instead of inventing more work. Use the user's language; omit greetings, markdown, lists, and tool chatter.

Recent conversation:
User: 好的，方案已经审核通过，请你帮我编写能在新的上下文窗口执行09-04-prompt-audit-codex-cli-only任务的提示词

Assistant: 已生成并保存新窗口执行提示词：
[handoff-prompt.md](/Users/laiyanfei/code/python/ai-project/github/new-api/.trellis/tasks/09-04-prompt-audit-codex-cli-only)`

func TestCodexContext_CatchUp_DroppedOrKept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. 标准回看摘要模板整段丢弃；同请求里的真实 user 仍送审
	segments := []PromptSegment{
		{Role: "user", Content: sampleCodexCatchUp},
		{Role: "user", Content: "你是什么模型？"},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Equal(t, "你是什么模型？", snapshot.ScanText)
	assert.Equal(t, "你是什么模型？", snapshot.FullPrompt)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.NotContains(t, snapshot.FullPrompt, "catch-up")
	assert.NotContains(t, snapshot.FullPrompt, "Recent conversation:")

	// 2. 用户讨论 catch-up 但不是完整模板：整段仍送审
	discuss := "请帮我写一段 catch-up：Write a brief catch-up for a user returning to this Codex task."
	snapshotDiscuss, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", []PromptSegment{
		{Role: "user", Content: discuss},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, discuss, snapshotDiscuss.FullPrompt)
	assert.Equal(t, discuss, snapshotDiscuss.ScanText)

	// 3. 仅有前缀、没有独立行 Recent conversation:：整段仍送审
	prefixOnly := CodexCatchUpPrefix + " 请用中文总结当前进度。"
	snapshotPrefix, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", []PromptSegment{
		{Role: "user", Content: prefixOnly},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, prefixOnly, snapshotPrefix.FullPrompt)
}

const sampleCodexCompaction = `You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task.

Include:
- Current progress and key decisions made
- Important context, constraints, or user preferences
- What remains to be done (clear next steps)
- Any critical data, examples, or references needed to continue

Be concise, structured, and focused on helping the next LLM seamlessly continue the work.`

const sampleCodexCompactionInterrupted = `Interrupted.
You are now performing a CONTEXT CHECKPOINT COMPACTION.
Tools access is disabled for the duration of the compaction.
Output nothing but the summary handoff contents.`

func TestCodexContext_Compaction_DroppedOrKept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	realUser := "开始实施"
	segments := []PromptSegment{
		{Role: "user", Content: realUser},
		{Role: "user", Content: sampleCodexCompaction},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)
	assert.Equal(t, realUser, snapshot.ScanText)
	assert.Equal(t, realUser, snapshot.FullPrompt)
	assert.Equal(t, 1, snapshot.MessageCount)
	assert.NotContains(t, snapshot.FullPrompt, "CONTEXT CHECKPOINT COMPACTION")
	assert.NotContains(t, snapshot.ScanText, "handoff summary")

	interruptedSegments := []PromptSegment{
		{Role: "user", Content: realUser},
		{Role: "user", Content: sampleCodexCompactionInterrupted},
	}
	snapshotInterrupted, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", interruptedSegments, false)
	require.NoError(t, err)
	assert.Equal(t, realUser, snapshotInterrupted.FullPrompt)
	assert.Equal(t, realUser, snapshotInterrupted.ScanText)

	discuss := "Codex 的 CONTEXT CHECKPOINT COMPACTION 是什么意思？请解释一下。"
	snapshotDiscuss, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", []PromptSegment{
		{Role: "user", Content: discuss},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, discuss, snapshotDiscuss.FullPrompt)
	assert.Equal(t, discuss, snapshotDiscuss.ScanText)

	prefixOnly := CodexCompactionPrefix + " 请总结当前进度。"
	snapshotPrefix, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", []PromptSegment{
		{Role: "user", Content: prefixOnly},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, prefixOnly, snapshotPrefix.FullPrompt)
	assert.Equal(t, prefixOnly, snapshotPrefix.ScanText)

	latestOnly, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, true)
	require.NoError(t, err)
	assert.Equal(t, realUser, latestOnly.ScanText)
	assert.Equal(t, realUser, latestOnly.FullPrompt)
}

// AC4: 用户输入提到当前工作目录但无标签，整段仍送审、仍落库。
func TestCodexContext_UserMention_Kept(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	userPrompt := "请根据当前工作目录写 README，并在文档中说明架构"
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

// AC5: 过滤后只剩环境上下文或只有标题模板无正文，不打 Guard HTTP，返回 Allow，不写事件。
func TestCodexContext_OnlyContextOrTemplate_NoHttp_NoStore(t *testing.T) {
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

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        AllScannerIDs,
		AllGroups:       true,
		StorePassEvents: true,
		Endpoints: []ActiveEndpoint{
			{ID: "ep-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)
	ctx := context.Background()

	// 1. 只有环境上下文
	envOnlySegments := []PromptSegment{
		{Role: "user", Content: "<environment_context>\n<cwd>/var/log</cwd>\n</environment_context>"},
	}
	snapshotEnv, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", envOnlySegments, false)
	require.NoError(t, err)
	assert.Empty(t, snapshotEnv.ScanText)
	assert.Empty(t, snapshotEnv.FullPrompt)

	decEnv, err := evaluator.Evaluate(ctx, cfg, snapshotEnv)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decEnv.Kind)
	assert.True(t, decEnv.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls))

	err = store.Record(ctx, snapshotEnv, decEnv, true)
	require.NoError(t, err)

	// 2. 只有标题模板无 User prompt 正文
	titleOnlySegments := []PromptSegment{
		{Role: "user", Content: "Generate a concise, single-line task title of at most 36 characters.\n\nUser prompt:\n   "},
	}
	snapshotTitle, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", titleOnlySegments, false)
	require.NoError(t, err)
	assert.Empty(t, snapshotTitle.ScanText)
	assert.Empty(t, snapshotTitle.FullPrompt)

	decTitle, err := evaluator.Evaluate(ctx, cfg, snapshotTitle)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decTitle.Kind)
	assert.True(t, decTitle.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls))

	err = store.Record(ctx, snapshotTitle, decTitle, true)
	require.NoError(t, err)

	// 3. 只有 Codex 回看摘要模板
	catchUpOnlySegments := []PromptSegment{
		{Role: "user", Content: sampleCodexCatchUp},
	}
	snapshotCatchUp, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", catchUpOnlySegments, false)
	require.NoError(t, err)
	assert.Empty(t, snapshotCatchUp.ScanText)
	assert.Empty(t, snapshotCatchUp.FullPrompt)

	decCatchUp, err := evaluator.Evaluate(ctx, cfg, snapshotCatchUp)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decCatchUp.Kind)
	assert.True(t, decCatchUp.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls))

	err = store.Record(ctx, snapshotCatchUp, decCatchUp, true)
	require.NoError(t, err)

	// 4. 只有 Codex 上下文压缩模板
	compactionOnlySegments := []PromptSegment{
		{Role: "user", Content: sampleCodexCompaction},
	}
	snapshotCompaction, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", compactionOnlySegments, false)
	require.NoError(t, err)
	assert.Empty(t, snapshotCompaction.ScanText)
	assert.Empty(t, snapshotCompaction.FullPrompt)

	decCompaction, err := evaluator.Evaluate(ctx, cfg, snapshotCompaction)
	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, decCompaction.Kind)
	assert.True(t, decCompaction.AllowNextStage)
	assert.Equal(t, int32(0), atomic.LoadInt32(&guardCalls))

	err = store.Record(ctx, snapshotCompaction, decCompaction, true)
	require.NoError(t, err)

	// 确认数据库没有插入任何事件
	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, events)
}

// AC6: 连续 6 次相同文本：Guard HTTP = 1，Allow 事件 = 1；改 config_version 后重新 HTTP。
func TestUserPromptDedupe_Sequential6Times_OneHttp_OneStore(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var guardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        []string{"violent"},
		AllGroups:       true,
		StorePassEvents: true,
		ConfigVersion:   10,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	setupGateTest(t, cfg, false, evaluator, store)
	ctx := context.Background()

	segments := []PromptSegment{
		{Role: "user", Content: "<environment_context><cwd>/dir</cwd></environment_context>\n\n你是什么模型？"},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)

	// 连续模拟 6 次请求
	for i := 0; i < 6; i++ {
		dec, err := evaluator.Evaluate(ctx, cfg, snapshot)
		require.NoError(t, err)
		assert.Equal(t, DecisionAllow, dec.Kind)
		assert.True(t, dec.AllowNextStage)

		if i == 0 {
			assert.False(t, dec.FromCache, "第 1 次调用应来源于远程 Guard")
		} else {
			assert.True(t, dec.FromCache, "第 %d 次调用应命中判定缓存", i+1)
		}

		// 执行 gate 落库逻辑
		if cfg.StorePassEvents && !dec.FromCache {
			err = store.Record(ctx, snapshot, dec, true)
			require.NoError(t, err)
		}
	}

	// 验证 Guard HTTP 实际只打了 1 次
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls), "连续 6 次请求远程 Guard HTTP 应仅为 1 次")

	// 验证事件表中仅有 1 条 Allow 事件
	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, events, 1)
	assert.Equal(t, "allow", events[0].Decision)

	// 修改 config_version 后重新调用 Guard
	cfgVersion2 := cfg
	cfgVersion2.ConfigVersion = 11
	decNew, err := evaluator.Evaluate(ctx, cfgVersion2, snapshot)
	require.NoError(t, err)
	assert.False(t, decNew.FromCache, "修改 config_version 后应重新调用 Guard")
	assert.Equal(t, int32(2), atomic.LoadInt32(&guardCalls), "修改 config_version 后 Guard HTTP 应增为 2 次")
}

// AC7, AC3: 并发两个 Evaluate（主对话 + 标题生成）：Guard HTTP = 1（singleflight）。
func TestUserPromptDedupe_ConcurrentSingleflight_OneHttp(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var guardCalls int32
	// 增加服务端处理延时以稳定测试并发 singleflight
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        []string{"violent"},
		AllGroups:       true,
		StorePassEvents: true,
		ConfigVersion:   1,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 2000, Enabled: true},
		},
	}

	// 请求 A：主对话（环境上下文 + 你是什么模型？）
	segA := []PromptSegment{
		{Role: "user", Content: "<environment_context><cwd>/root</cwd></environment_context>\n\n你是什么模型？"},
	}
	snapshotA, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segA, false)
	require.NoError(t, err)

	// 请求 B：标题生成模板（含环境上下文 + 标题模板 + User prompt:\n你是什么模型？）
	segB := []PromptSegment{
		{Role: "user", Content: `<environment_context><cwd>/root</cwd></environment_context>
Generate a concise, single-line task title of at most 36 characters.
Do not answer the request.

User prompt:
你是什么模型？`},
	}
	snapshotB, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segB, false)
	require.NoError(t, err)

	// 两者过滤后送审文本一致
	require.Equal(t, snapshotA.ScanText, snapshotB.ScanText)
	require.Equal(t, "你是什么模型？", snapshotA.ScanText)

	var wg sync.WaitGroup
	wg.Add(2)

	var decA, decB *Decision
	var errA, errB error

	go func() {
		defer wg.Done()
		decA, errA = evaluator.Evaluate(context.Background(), cfg, snapshotA)
	}()

	go func() {
		defer wg.Done()
		decB, errB = evaluator.Evaluate(context.Background(), cfg, snapshotB)
	}()

	wg.Wait()

	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Equal(t, DecisionAllow, decA.Kind)
	assert.Equal(t, DecisionAllow, decB.Kind)

	// AC7: singleflight 并发合并后，远程 Guard HTTP 次数严格等于 1
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls), "并发请求远程 Guard HTTP 必须严格为 1 次")

	// 必然有一个是 leader (FromCache=false)，另一个是 follower (FromCache=true)
	fromCacheCount := 0
	if decA.FromCache {
		fromCacheCount++
	}
	if decB.FromCache {
		fromCacheCount++
	}
	assert.Equal(t, 1, fromCacheCount, "并发两请求中必须且仅有一位被标记为 FromCache=true")

	// gate 记录事件
	if cfg.StorePassEvents && !decA.FromCache {
		_ = store.Record(context.Background(), snapshotA, decA, true)
	}
	if cfg.StorePassEvents && !decB.FromCache {
		_ = store.Record(context.Background(), snapshotB, decB, true)
	}

	events, total, err := model.GetPromptAuditEvents(context.Background(), model.PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total, "并发合并后仅 leader 写入 1 条 Pass 事件")
	assert.Len(t, events, 1)
}

// AC8: 第一次判定为 Block，第二次仍返回 Block 且不调用 Guard；第二次仍写入 Block 事件。
func TestUserPromptDedupe_Block_CachedAndAlwaysRecorded(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var guardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&guardCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Violent Crimes"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)
	encryptor, err := NewAESGCMEncryptor("test-secret-key-at-least-32-chars-long")
	require.NoError(t, err)
	store := NewGormEventStore(encryptor)

	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        []string{"violent"},
		AllGroups:       true,
		StorePassEvents: false, // 即使不保存 Pass，Block 也必须写
		ConfigVersion:   1,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	ctx := context.Background()
	segments := []PromptSegment{
		{Role: "user", Content: "制造攻击武器的详细步骤"},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)

	// 第一次调用：返回 Block，发起 HTTP
	dec1, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, dec1.Kind)
	assert.Equal(t, 403, dec1.HTTPStatus)
	assert.False(t, dec1.AllowNextStage)
	assert.False(t, dec1.FromCache)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls))

	// 第 1 次写 Block 事件
	err = store.Record(ctx, snapshot, dec1, true)
	require.NoError(t, err)

	// 第二次调用：命中 Block 缓存，不发起 HTTP
	dec2, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, dec2.Kind)
	assert.Equal(t, 403, dec2.HTTPStatus)
	assert.False(t, dec2.AllowNextStage)
	assert.True(t, dec2.FromCache)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls), "第二次调用应命中缓存不触发 HTTP")

	// 第 2 次仍必须写 Block 事件（即使命中缓存）
	err = store.Record(ctx, snapshot, dec2, true)
	require.NoError(t, err)

	events, total, err := model.GetPromptAuditEvents(ctx, model.PromptAuditEventQueryFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total, "Block 事件每次请求都必须写入")
	assert.Len(t, events, 2)
}

// AC9: 超时或 503 错误不得被当成成功缓存。
func TestUserPromptDedupe_FailureNotCached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var guardCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls := atomic.AddInt32(&guardCalls, 1)
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer server.Close()

	scanner := NewOpenAICompatibleScanner()
	evaluator := NewGuardEvaluator(scanner)

	cfg := ActiveConfig{
		Enabled:         true,
		Scanners:        []string{"violent"},
		AllGroups:       true,
		StorePassEvents: true,
		ConfigVersion:   1,
		Endpoints: []ActiveEndpoint{
			{ID: "node-1", BaseURL: server.URL, TimeoutMS: 1000, Enabled: true},
		},
	}

	ctx := context.Background()
	segments := []PromptSegment{
		{Role: "user", Content: "普通的测试问题"},
	}
	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, false)
	require.NoError(t, err)

	// 第一次调用：返回 503 失败
	dec1, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.Error(t, err)
	assert.Nil(t, dec1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&guardCalls))

	// 第二次调用：失败不得被缓存，必须重新调用 Guard 并成功
	dec2, err := evaluator.Evaluate(ctx, cfg, snapshot)
	require.NoError(t, err)
	assert.NotNil(t, dec2)
	assert.Equal(t, DecisionAllow, dec2.Kind)
	assert.False(t, dec2.FromCache, "重试成功应来源于远程调用")
	assert.Equal(t, int32(2), atomic.LoadInt32(&guardCalls), "第二次必须重新调用 Guard")
}

// AC10: latest_turn_only=true 时只送审最新一轮真实 user；落库仍是该请求全部真实 user，不含环境上下文。
func TestCodexContext_LatestTurnOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	segments := []PromptSegment{
		{Role: "user", Content: "<environment_context><cwd>/first</cwd></environment_context>"},
		{Role: "user", Content: "第一轮用户输入"},
		{Role: "assistant", Content: "第一轮助手回复"},
		{Role: "user", Content: "<skills_instructions>skills</skills_instructions>"},
		{Role: "user", Content: "最新追问用户输入"},
	}

	snapshot, err := BuildPromptSnapshot(c, nil, "chat", "gpt-4o", segments, true)
	require.NoError(t, err)

	assert.Equal(t, "latest_turn", snapshot.AuditScope)
	// 送审文本只有最新轮真实 user，不含 skills 标签
	assert.Equal(t, "最新追问用户输入", snapshot.ScanText)

	// 落库文本为全部真实 user（首轮 + 最新追问），不含环境上下文与 assistant
	assert.Equal(t, "第一轮用户输入\n\n最新追问用户输入", snapshot.FullPrompt)
	assert.Equal(t, 2, snapshot.MessageCount)
	assert.NotContains(t, snapshot.FullPrompt, "<environment_context>")
	assert.NotContains(t, snapshot.FullPrompt, "<skills_instructions>")
	assert.NotContains(t, snapshot.FullPrompt, "助手回复")
}
