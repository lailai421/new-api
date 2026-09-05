package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service/promptaudit"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type mockTaskGateEvaluator struct {
	decision *promptaudit.Decision
	err      error
	calls    int
}

func (m *mockTaskGateEvaluator) Evaluate(ctx context.Context, cfg promptaudit.ActiveConfig, snapshot promptaudit.PromptSnapshot) (*promptaudit.Decision, error) {
	m.calls++
	return m.decision, m.err
}

func setupTaskPluginAuditTest(t *testing.T, activeCfg promptaudit.ActiveConfig, eval promptaudit.Evaluator) func() {
	t.Helper()
	origMgr := promptaudit.GetManager()
	origEval := promptaudit.GetEvaluator()

	mgr := promptaudit.NewManager(nil, nil)
	// bypass private active assignment using reflection or exported helper if available
	// or set via Save/Init. Since NewManager initializes default, let's inject activeCfg:
	*mgr = *promptaudit.NewManager(nil, nil)
	// We can set activeCfg directly through internal helper if same package, or test fixture.
	// In controller package, promptaudit.SetGlobalManager / SetGlobalEvaluator
	promptaudit.SetGlobalManager(mgr)
	promptaudit.SetGlobalEvaluator(eval)

	return func() {
		promptaudit.SetGlobalManager(origMgr)
		promptaudit.SetGlobalEvaluator(origEval)
	}
}

func initTestDatabase(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		model.InitCol()
		_ = sqlDB.Close()
	})

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.OptionMap = make(map[string]string)
	common.CryptoSecret = "test-crypto-secret-key-at-least-32-bytes"
	t.Setenv("CRYPTO_SECRET", "test-crypto-secret-key-at-least-32-bytes")
	model.InitCol()

	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PromptAuditEvent{},
		&model.Log{},
		&model.User{},
		&model.Task{},
	))

	db.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	})
	require.NoError(t, promptaudit.InitPromptAudit())

	mgr := promptaudit.GetManager()
	cfg := promptaudit.DefaultConfig()
	cfg.Enabled = true
	cfg.AllGroups = true
	cfg.Endpoints = []promptaudit.Endpoint{
		{
			ID:         "ep-test",
			Name:       "Test Guard",
			Protocol:   promptaudit.ProtocolOpenAICompatible,
			BaseURL:    "http://127.0.0.1:8000/v1",
			Model:      promptaudit.DefaultGuardModel,
			Enabled:    true,
			TimeoutMS:  3000,
			InputLimit: 4000,
		},
	}
	store := promptaudit.GetConfigStore()
	require.NoError(t, store.Save(context.Background(), &cfg, 0))
	require.NoError(t, mgr.Reload(context.Background()))
}

func TestTaskPluginPromptAudit_TenBuiltinPlugins(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	// Builtin plugin keys and their expected auditTextPaths
	plugins := []struct {
		key   string
		paths []string
		body  map[string]any
	}{
		{"alibaba", []string{"/prompt"}, map[string]any{"prompt": "A scenic mountain"}},
		{"doubao", []string{"/prompt"}, map[string]any{"prompt": "A dancing robot"}},
		{"google", []string{"/prompt"}, map[string]any{"prompt": "A soaring eagle"}},
		{"hailuo", []string{"/prompt"}, map[string]any{"prompt": "A swimming dolphin"}},
		{"jimeng", []string{"/prompt"}, map[string]any{"prompt": "A futuristic city"}},
		{"kling", []string{"/prompt", "/negative_prompt"}, map[string]any{"prompt": "A racing car", "negative_prompt": "blurry"}},
		{"sora", []string{"/prompt"}, map[string]any{"prompt": "An astronaut on mars"}},
		{"sunoapi", []string{"/prompt", "/gpt_description_prompt", "/tags", "/title"}, map[string]any{"prompt": "happy lyrics", "gpt_description_prompt": "acoustic guitar", "tags": "folk", "title": "Sunshine"}},
		{"vertex-ai", []string{"/prompt"}, map[string]any{"prompt": "A blooming flower"}},
		{"vidu", []string{"/prompt"}, map[string]any{"prompt": "A running tiger"}},
	}

	for _, p := range plugins {
		t.Run(p.key+" Pass and Block paths", func(t *testing.T) {
			// 1. Pass path
			evalPass := &mockTaskGateEvaluator{
				decision: &promptaudit.Decision{
					Kind:           promptaudit.DecisionAllow,
					AllowNextStage: true,
				},
			}
			promptaudit.SetGlobalEvaluator(evalPass)

			wPass := httptest.NewRecorder()
			cPass, _ := gin.CreateTestContext(wPass)
			attachCodexCLIHeader(cPass)
			cPass.Set("task_request", p.body)
			cPass.Set("user_id", 1)
			relayInfoPass := &relaycommon.RelayInfo{
				OriginModelName: p.key + "-model",
				RelayFormat:     types.RelayFormatTask,
			}
			auditMeta := promptaudit.TaskAuditMeta{
				PluginKey:           p.key,
				AuditTextPaths:      p.paths,
				HasSubmitCapability: true,
				Found:               true,
			}

			passErr := promptaudit.CheckTaskRequest(cPass, relayInfoPass, auditMeta)
			require.Nil(t, passErr, "Plugin %s should pass", p.key)
			assert.True(t, cPass.GetBool(promptaudit.ContextKeyTaskAuditDone))

			// 2. Block path
			evalBlock := &mockTaskGateEvaluator{
				decision: &promptaudit.Decision{
					Kind:           promptaudit.DecisionBlock,
					ErrorCode:      promptaudit.ErrorCodeBlocked,
					AllowNextStage: false,
				},
			}
			promptaudit.SetGlobalEvaluator(evalBlock)

			wBlock := httptest.NewRecorder()
			cBlock, _ := gin.CreateTestContext(wBlock)
			attachCodexCLIHeader(cBlock)
			cBlock.Set("task_request", p.body)
			cBlock.Set("user_id", 1)
			relayInfoBlock := &relaycommon.RelayInfo{
				OriginModelName: p.key + "-model",
				RelayFormat:     types.RelayFormatTask,
			}

			blockErr := promptaudit.CheckTaskRequest(cBlock, relayInfoBlock, auditMeta)
			require.NotNil(t, blockErr, "Plugin %s should be blocked", p.key)
			assert.Equal(t, http.StatusForbidden, blockErr.StatusCode)
			assert.Equal(t, promptaudit.ErrorCodeBlocked, blockErr.Code)
			assert.True(t, blockErr.LocalError)
		})
	}
}

func TestTaskPluginPromptAudit_ThirdPartyPluginFailClosed(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	metaNoContract := promptaudit.TaskAuditMeta{
		PluginKey:           "third-party-no-contract",
		AuditTextPaths:      nil, // missing contract!
		HasSubmitCapability: true,
		Found:               true,
	}

	t.Run("audit enabled -> fails closed with 503 unsupported protocol", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLIHeader(c)
		c.Set("task_request", map[string]any{"prompt": "test prompt"})
		c.Set("user_id", 1)
		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "custom-video",
			RelayFormat:     types.RelayFormatTask,
		}

		taskErr := promptaudit.CheckTaskRequest(c, relayInfo, metaNoContract)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		assert.Equal(t, promptaudit.ErrorCodeUnsupportedProtocol, taskErr.Code)
		assert.True(t, taskErr.LocalError)
	})

	t.Run("group does not match -> passes untouched", func(t *testing.T) {
		// Manager with group restriction
		mgr := promptaudit.GetManager()
		cfg := promptaudit.DefaultConfig()
		cfg.Enabled = true
		cfg.AllGroups = false
		cfg.Groups = []string{"vip"}
		cfg.Endpoints = []promptaudit.Endpoint{
			{
				ID:         "ep-test",
				Name:       "Test Guard",
				Protocol:   promptaudit.ProtocolOpenAICompatible,
				BaseURL:    "http://127.0.0.1:8000/v1",
				Model:      promptaudit.DefaultGuardModel,
				Enabled:    true,
				TimeoutMS:  3000,
				InputLimit: 4000,
			},
		}
		store := promptaudit.GetConfigStore()
		require.NoError(t, store.Save(context.Background(), &cfg, mgr.Active().ConfigVersion))
		require.NoError(t, mgr.Reload(context.Background()))

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLIHeader(c)
		c.Set("task_request", map[string]any{"prompt": "test prompt"})
		c.Set("user_id", 1)
		c.Set("group", "default") // non-matching group
		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "custom-video",
			RelayFormat:     types.RelayFormatTask,
			UsingGroup:      "default",
		}

		taskErr := promptaudit.CheckTaskRequest(c, relayInfo, metaNoContract)
		require.Nil(t, taskErr, "Non-matching group should pass without audit")
	})
}

func TestTaskPluginPromptAudit_ResponsesBridge(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	protoContext := map[string]any{
		"requestBody": map[string]any{
			"instructions": "Be a helpful video director",
			"input": []any{
				map[string]any{
					"role":    "user",
					"content": "Generate an epic space battle",
				},
			},
		},
	}

	t.Run("Responses Bridge Blocked returns OpenAI formatted 403 and stops before submit", func(t *testing.T) {
		evalBlock := &mockTaskGateEvaluator{
			decision: &promptaudit.Decision{
				Kind:           promptaudit.DecisionBlock,
				ErrorCode:      promptaudit.ErrorCodeBlocked,
				AllowNextStage: false,
			},
		}
		promptaudit.SetGlobalEvaluator(evalBlock)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLIHeader(c)
		c.Set("protocol_request", protoContext)
		c.Set("task_request", map[string]any{
			"prompt": "Generate an epic space battle",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "sora",
			RelayFormat:     types.RelayFormatTask,
			RelayMode:       relayconstant.RelayModeVideoSubmit,
		}
		auditMeta := promptaudit.TaskAuditMeta{
			PluginKey:           "sora",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := promptaudit.CheckTaskPluginProtocolRequest(c, relayInfo, auditMeta)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
		assert.Equal(t, promptaudit.ErrorCodeBlocked, taskErr.Code)

		// Render error through protocol error presenter
		respondPluginProtocolSubmissionError(c, taskErr)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeBlocked)
	})

	t.Run("Responses Bridge Pass marks audit done and deduplicates", func(t *testing.T) {
		evalPass := &mockTaskGateEvaluator{
			decision: &promptaudit.Decision{
				Kind:           promptaudit.DecisionAllow,
				AllowNextStage: true,
			},
		}
		promptaudit.SetGlobalEvaluator(evalPass)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLIHeader(c)
		c.Set("protocol_request", protoContext)
		c.Set("task_request", map[string]any{
			"prompt": "Generate an epic space battle",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "sora",
			RelayFormat:     types.RelayFormatTask,
			RelayMode:       relayconstant.RelayModeVideoSubmit,
		}
		auditMeta := promptaudit.TaskAuditMeta{
			PluginKey:           "sora",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := promptaudit.CheckTaskPluginProtocolRequest(c, relayInfo, auditMeta)
		require.Nil(t, taskErr)
		assert.True(t, c.GetBool(promptaudit.ContextKeyTaskAuditDone))

		// Subsequent CheckTaskRequest skips evaluation
		evalPass.decision = &promptaudit.Decision{Kind: promptaudit.DecisionBlock}
		require.Nil(t, promptaudit.CheckTaskRequest(c, relayInfo, auditMeta))
	})
}

func TestTaskPluginPromptAudit_RuntimeEndpointReportsUncoveredPlugins(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/api/prompt-audit/runtime", GetPromptAuditRuntime)

	req := httptest.NewRequest(http.MethodGet, "/api/prompt-audit/runtime", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool                           `json:"success"`
		Data    dto.PromptAuditRuntimeResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	// All built-in plugins have declared auditTextPaths, so uncovered_plugins should be empty
	assert.Empty(t, resp.Data.UncoveredPlugins)

	// Register an uncovered plugin
	source := `
export const meta = {
	apiVersion: 1,
	key: "test-uncovered-dynamic",
	name: "Uncovered Plugin",
	version: "1.0.0",
	author: {name: "Tester"},
	models: ["uncovered-model"],
	fetchMode: "per_task",
	routes: [{method: "POST", path: "/test/uncovered/dyn", type: "submit", decode: "dec", render: "ren"}],
};
export const native = {
	dec: function(ctx) { return {kind: "submit", model: "uncovered-model"}; },
	ren: function(ctx, t) { return {}; },
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "1"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`
	_, err := pluginruntime.DefaultRegistry.Register(source, pluginruntime.Options{})
	require.NoError(t, err)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, common.Unmarshal(w2.Body.Bytes(), &resp))
	assert.Contains(t, resp.Data.UncoveredPlugins, "test-uncovered-dynamic")

	// Clean up by unregistering
	require.NoError(t, pluginruntime.DefaultRegistry.Unregister("test-uncovered-dynamic"))

	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req)
	require.Equal(t, http.StatusOK, w3.Code)
	require.NoError(t, common.Unmarshal(w3.Body.Bytes(), &resp))
	assert.NotContains(t, resp.Data.UncoveredPlugins, "test-uncovered-dynamic")
}

func TestTaskPluginPromptAudit_RelayTaskZeroSideEffects(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	// Block evaluator
	evalBlock := &mockTaskGateEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionBlock,
			ErrorCode:      promptaudit.ErrorCodeBlocked,
			AllowNextStage: false,
		},
	}
	promptaudit.SetGlobalEvaluator(evalBlock)

	plugin, err := pluginruntime.CompilePlugin(`
export const meta = {apiVersion:1,key:"kling-mock",name:"Kling Mock",version:"1.0.0",author:{name:"Test"},models:["kling-v1"],fetchMode:"per_task",routes:[{method:"POST",path:"/kling/submit",type:"submit",decode:"decode",render:"created"}],auditTextPaths:["/prompt"]};
export const native = {decode:function(ctx){return {kind:"submit",model:"kling-v1",requestBody:ctx.body.value};},created:function(ctx,task){return {};}};
export function buildSubmitRequest(){return {}} export function parseSubmitResponse(){return {taskId:"upstream"}} export function buildQueryRequest(){return {}} export function parseTaskResult(){return {status:"SUCCESS"}}
`, pluginruntime.Options{})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/kling/submit", strings.NewReader(`{"prompt":"forbidden video"}`))
	attachCodexCLIHeader(c)
	c.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{Plugin: plugin, Route: plugin.Meta.Routes[0]})
	c.Set(pluginruntime.ContextKeyRouteRequest, pluginruntime.RouteRequestContext{
		Path:   "/kling/submit",
		Method: http.MethodPost,
		Body:   map[string]any{"kind": "json", "value": map[string]any{"prompt": "forbidden video"}},
	})
	c.Set("task_request", map[string]any{
		"prompt": "forbidden video",
	})
	c.Set("user_id", 1)
	c.Set("token_id", 1)
	c.Set("group", "default")
	c.Set("task_model", "kling-v1")
	c.Set("resolved_task_model", "kling-v1")

	// Call RelayTask
	RelayTask(c)

	// Assert response is 403 Forbidden with prompt_guard_blocked
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeBlocked)

	// Assert 0 tasks inserted in DB
	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Equal(t, int64(0), taskCount, "Blocked task must not persist any task record")
}

func TestTaskPluginPromptAudit_CodexCLIRequired(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	eval := &mockTaskGateEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionAllow,
			AllowNextStage: true,
		},
	}
	promptaudit.SetGlobalEvaluator(eval)

	canaryPrompt := "CANARY_TASK_PROMPT_DO_NOT_LEAK"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/kling/submit", strings.NewReader(`{"prompt":"`+canaryPrompt+`"}`))
	c.Request.Header.Set("Originator", "curl")
	c.Request.Header.Set("User-Agent", "CANARY_UA_TASK")
	common.SetContextKey(c, common.RequestIdKey, "req-task-unknown-client")
	c.Set("task_request", map[string]any{"prompt": canaryPrompt})
	c.Set("user_id", 1)
	c.Set("token_id", 1)
	c.Set("group", "default")
	c.Set("resolved_task_model", "kling-v1")

	RelayTask(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeCodexCLIRequired)
	assert.Contains(t, w.Body.String(), promptaudit.CodexCLIRequiredMessage)
	assert.Equal(t, 0, eval.calls)
	assert.False(t, c.GetBool(promptaudit.ContextKeyTaskAuditDone))
	assert.False(t, c.GetBool("preconsumed_quota_flag"))
	assert.Empty(t, c.GetStringSlice("use_channel"))
	assert.NotContains(t, w.Body.String(), canaryPrompt)
	assert.NotContains(t, w.Body.String(), "curl")
	assert.NotContains(t, w.Body.String(), "CANARY_UA_TASK")

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Equal(t, int64(0), taskCount)
	var eventCount int64
	require.NoError(t, model.DB.Model(&model.PromptAuditEvent{}).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
}

func TestTaskPluginProtocolPromptAudit_CodexCLIRequired(t *testing.T) {
	initTestDatabase(t)
	gin.SetMode(gin.TestMode)

	eval := &mockTaskGateEvaluator{
		decision: &promptaudit.Decision{
			Kind:           promptaudit.DecisionAllow,
			AllowNextStage: true,
		},
	}
	promptaudit.SetGlobalEvaluator(eval)

	canaryPrompt := "CANARY_PLUGIN_PROMPT_DO_NOT_LEAK"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"sora"}`))
	c.Request.Header.Set("Originator", "my-codex_cli_rs-wrapper")
	c.Set("task_request", map[string]any{"prompt": canaryPrompt})
	c.Set("user_id", 1)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "sora",
		RelayFormat:     types.RelayFormatTask,
		RelayMode:       relayconstant.RelayModeVideoSubmit,
	}
	auditMeta := promptaudit.TaskAuditMeta{
		PluginKey:           "sora",
		AuditTextPaths:      []string{"/prompt"},
		HasSubmitCapability: true,
		Found:               true,
	}

	taskErr := promptaudit.CheckTaskPluginProtocolRequest(c, relayInfo, auditMeta)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	assert.Equal(t, promptaudit.ErrorCodeCodexCLIRequired, taskErr.Code)
	assert.Equal(t, promptaudit.CodexCLIRequiredMessage, taskErr.Message)
	assert.True(t, taskErr.LocalError)
	assert.Equal(t, 0, eval.calls)

	respondPluginProtocolSubmissionError(c, taskErr)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), promptaudit.ErrorCodeCodexCLIRequired)
	assert.Contains(t, w.Body.String(), promptaudit.CodexCLIRequiredMessage)
	assert.NotContains(t, w.Body.String(), canaryPrompt)
	assert.NotContains(t, w.Body.String(), "my-codex_cli_rs-wrapper")

	var eventCount int64
	require.NoError(t, model.DB.Model(&model.PromptAuditEvent{}).Count(&eventCount).Error)
	assert.Equal(t, int64(0), eventCount)
}
