package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/promptaudit"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPromptAuditControllerWithAuth(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

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
	))

	db.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
	})

	require.NoError(t, promptaudit.InitPromptAudit())

	r := gin.New()

	// Inject role from header for flexible test scenarios
	r.Use(func(c *gin.Context) {
		roleHeader := c.GetHeader("X-Test-Role")
		if roleHeader == "root" {
			c.Set("role", common.RoleRootUser)
			c.Set("id", 1)
			c.Set("username", "root")
		} else if roleHeader == "admin" {
			c.Set("role", common.RoleAdminUser)
			c.Set("id", 2)
			c.Set("username", "admin")
		} else if roleHeader == "user" {
			c.Set("role", common.RoleCommonUser)
			c.Set("id", 3)
			c.Set("username", "common")
		}
		c.Next()
	})

	api := r.Group("/api")
	promptAuditRoute := api.Group("/prompt-audit")
	promptAuditRoute.Use(func(c *gin.Context) {
		role := c.GetInt("role")
		if role == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
			return
		}
		if role < common.RoleRootUser {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
			return
		}
		c.Next()
	})
	{
		promptAuditRoute.GET("/config", GetPromptAuditConfig)
		promptAuditRoute.PUT("/config", UpdatePromptAuditConfig)
		promptAuditRoute.GET("/runtime", GetPromptAuditRuntime)
		promptAuditRoute.POST("/endpoints/probe", ProbePromptAuditEndpoint)
		promptAuditRoute.GET("/events", GetPromptAuditEvents)
		promptAuditRoute.GET("/events/:id", middleware.DisableCache(), GetPromptAuditEvent)
		promptAuditRoute.DELETE("/events/:id", DeletePromptAuditEvent)
		promptAuditRoute.POST("/events/batch-delete", BatchDeletePromptAuditEvents)
	}

	optionRoute := api.Group("/option")
	optionRoute.Use(func(c *gin.Context) {
		role := c.GetInt("role")
		if role < common.RoleAdminUser {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "forbidden"})
			return
		}
		c.Next()
	})
	{
		optionRoute.GET("/", GetOptions)
		optionRoute.PUT("/", UpdateOption)
	}

	return r
}

func TestPromptAuditController_RootAuthGuard(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)

	// Anonymous request -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/api/prompt-audit/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Common user -> 403 Forbidden
	req = httptest.NewRequest(http.MethodGet, "/api/prompt-audit/config", nil)
	req.Header.Set("X-Test-Role", "user")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Admin user -> 403 Forbidden (Only Root is permitted)
	req = httptest.NewRequest(http.MethodGet, "/api/prompt-audit/config", nil)
	req.Header.Set("X-Test-Role", "admin")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Root user -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/prompt-audit/config", nil)
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPromptAuditController_ConfigLifecycleAndCAS(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)

	// 1. Initial GET returns config_version = 1, default off
	req := httptest.NewRequest(http.MethodGet, "/api/prompt-audit/config", nil)
	req.Header.Set("X-Test-Role", "root")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"config_version":1`)

	// 2. PUT with expected_config_version = 1 -> success, saves initial DB record with version = 1
	enabled := true
	updateReq := dto.PromptAuditConfigUpdateRequest{
		Enabled:               &enabled,
		ExpectedConfigVersion: 1,
		Endpoints: []dto.PromptAuditEndpointUpdateRequest{
			{
				ID:         "ep-1",
				Name:       "Node 1",
				Protocol:   promptaudit.ProtocolOpenAICompatible,
				BaseURL:    "http://127.0.0.1:8000/v1",
				Model:      "Qwen/Qwen3-Guard-0.6B",
				Token:      "secret-token-123",
				TimeoutMS:  2000,
				InputLimit: 4096,
				Enabled:    true,
			},
		},
	}
	body, _ := common.Marshal(updateReq)
	req = httptest.NewRequest(http.MethodPut, "/api/prompt-audit/config", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"config_version":1`)
	// Token must not be echoed in response!
	assert.NotContains(t, rec.Body.String(), "secret-token-123")
	assert.Contains(t, rec.Body.String(), `"has_token":true`)

	// 3. PUT with outdated expected_config_version = 99 -> 409 Conflict
	updateReq2 := updateReq
	updateReq2.ExpectedConfigVersion = 99
	body, _ = common.Marshal(updateReq2)
	req = httptest.NewRequest(http.MethodPut, "/api/prompt-audit/config", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "prompt_audit_config_conflict")

	// 4. PUT with delete_token = true and correct expected_config_version = 1 -> success, version = 2
	updateReq3 := updateReq
	updateReq3.ExpectedConfigVersion = 1
	updateReq3.Endpoints = []dto.PromptAuditEndpointUpdateRequest{
		{
			ID:          "ep-1",
			Name:        "Node 1",
			Protocol:    promptaudit.ProtocolOpenAICompatible,
			BaseURL:     "http://127.0.0.1:8000/v1",
			Model:       "Qwen/Qwen3-Guard-0.6B",
			DeleteToken: true,
			TimeoutMS:   2000,
			InputLimit:  4096,
			Enabled:     true,
		},
	}
	body, _ = common.Marshal(updateReq3)
	req = httptest.NewRequest(http.MethodPut, "/api/prompt-audit/config", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"config_version":2`)
	assert.Contains(t, rec.Body.String(), `"has_token":false`)
}

func TestPromptAuditController_RuntimeEndpoint(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/api/prompt-audit/runtime", nil)
	req.Header.Set("X-Test-Role", "root")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"degraded":false`)
	assert.Contains(t, rec.Body.String(), `"active_config_version":1`)
}

func TestPromptAuditController_EventsQueryDetailAndDelete(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)
	ctx := context.Background()

	// Seed 2 events
	encryptor := promptaudit.GetEncryptor()
	cipher1, err := encryptor.EncryptPrompt("Hello super secret text 1")
	require.NoError(t, err)

	e1 := &model.PromptAuditEvent{
		RequestId:            "req-ctrl-1",
		UserId:               100,
		Group:                "vip",
		Model:                "gpt-4o",
		Decision:             "block",
		RiskLevel:            "critical",
		RedactedPreview:      "Hello...",
		FullPromptCiphertext: model.PromptCiphertext(cipher1),
		CategoriesJSON:       `["jailbreak"]`,
		MatchedScannersJSON:  `["jailbreak"]`,
		ScannerScoresJSON:    `{"jailbreak":0.95}`,
		ScannerEvidenceJSON:  `{"jailbreak":"found harmful pattern"}`,
		CreatedAt:            time.Now().Unix(),
	}
	e2 := &model.PromptAuditEvent{
		RequestId:       "req-ctrl-2",
		UserId:          200,
		Group:           "default",
		Model:           "claude-3-5-sonnet",
		Decision:        "allow",
		RiskLevel:       "low",
		RedactedPreview: "Safe text...",
		CreatedAt:       time.Now().Unix(),
	}
	require.NoError(t, model.CreatePromptAuditEvent(ctx, e1))
	require.NoError(t, model.CreatePromptAuditEvent(ctx, e2))

	// 1. GET /api/prompt-audit/events -> list must contain preview but NOT full prompt
	req := httptest.NewRequest(http.MethodGet, "/api/prompt-audit/events", nil)
	req.Header.Set("X-Test-Role", "root")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "req-ctrl-1")
	assert.Contains(t, rec.Body.String(), "req-ctrl-2")
	assert.NotContains(t, rec.Body.String(), "Hello super secret text 1")

	// 2. GET /api/prompt-audit/events/:id -> detail must have Cache-Control: no-store and decrypted text
	req = httptest.NewRequest(http.MethodGet, "/api/prompt-audit/events/1", nil)
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
	assert.Contains(t, rec.Body.String(), "Hello super secret text 1")
	assert.Contains(t, rec.Body.String(), "found harmful pattern")

	// 3. DELETE /api/prompt-audit/events/:id
	req = httptest.NewRequest(http.MethodDelete, "/api/prompt-audit/events/1", nil)
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"success":true`)

	// 4. POST /api/prompt-audit/events/batch-delete
	batchReq := dto.PromptAuditBatchDeleteRequest{IDs: []int64{e2.Id}}
	body, _ := common.Marshal(batchReq)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/events/batch-delete", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"deleted":1`)

	// 5. Batch delete with empty IDs -> 400
	batchReqEmpty := dto.PromptAuditBatchDeleteRequest{IDs: []int64{}}
	body, _ = common.Marshal(batchReqEmpty)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/events/batch-delete", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 6. Batch delete > 500 IDs -> 400
	tooManyIDs := make([]int64, 501)
	for i := range tooManyIDs {
		tooManyIDs[i] = int64(i + 1)
	}
	batchReqTooMany := dto.PromptAuditBatchDeleteRequest{IDs: tooManyIDs}
	body, _ = common.Marshal(batchReqTooMany)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/events/batch-delete", bytes.NewReader(body))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPromptAuditController_ProbeEndpoint(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)

	// 1. Probe with empty request -> 400 Bad Request
	req := httptest.NewRequest(http.MethodPost, "/api/prompt-audit/endpoints/probe", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Test-Role", "root")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 2. Probe with unsupported protocol -> 400 Bad Request
	badProtoReq := dto.PromptAuditProbeRequest{
		BaseURL:  "http://127.0.0.1:8000",
		Model:    "some-model",
		Protocol: "unsupported_proto",
	}
	bodyBad, _ := common.Marshal(badProtoReq)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/endpoints/probe", bytes.NewReader(bodyBad))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Probe Qwen3Guard with mock server
	qwenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}],"usage":{"total_tokens":10}}`))
	}))
	defer qwenServer.Close()

	probeReqQwen := dto.PromptAuditProbeRequest{
		Protocol: promptaudit.ProtocolOpenAICompatible,
		BaseURL:  qwenServer.URL,
		Model:    "Qwen/Qwen3-Guard-0.6B",
		Token:    "probe-token-secret-qwen",
	}
	bodyQwen, _ := common.Marshal(probeReqQwen)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/endpoints/probe", bytes.NewReader(bodyQwen))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"探测成功"`)
	// Token must never be leaked in probe response!
	assert.NotContains(t, rec.Body.String(), "probe-token-secret-qwen")

	// 4. Probe LLM Classifier with mock server
	var capturedLLMHeaders http.Header
	var capturedLLMBody map[string]any
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLLMHeaders = r.Header
		_ = common.DecodeJson(r.Body, &capturedLLMBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"safety\":\"Safe\",\"categories\":[]}"}}],"usage":{"total_tokens":15}}`))
	}))
	defer llmServer.Close()

	probeReqLLM := dto.PromptAuditProbeRequest{
		Protocol: promptaudit.ProtocolLLMClassifier,
		BaseURL:  llmServer.URL,
		Model:    "deepseek-chat",
		Token:    "probe-token-secret-llm",
	}
	bodyLLM, _ := common.Marshal(probeReqLLM)
	req = httptest.NewRequest(http.MethodPost, "/api/prompt-audit/endpoints/probe", bytes.NewReader(bodyLLM))
	req.Header.Set("X-Test-Role", "root")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"探测成功"`)
	assert.NotContains(t, rec.Body.String(), "probe-token-secret-llm")

	// 确认 LLM 请求中携带了固定分类提示词与带分隔符的 user 消息
	assert.Equal(t, "Bearer probe-token-secret-llm", capturedLLMHeaders.Get("Authorization"))
	msgs, ok := capturedLLMBody["messages"].([]any)
	require.True(t, ok)
	require.Len(t, msgs, 2)
	sysMsg, _ := msgs[0].(map[string]any)
	assert.Equal(t, "system", sysMsg["role"])
	assert.Equal(t, promptaudit.LLMClassifierSystemPrompt, sysMsg["content"])
}

func TestPromptAuditController_OptionSecretIsolation(t *testing.T) {
	r := setupPromptAuditControllerWithAuth(t)

	// Inject PromptAuditConfigSecret into OptionMap
	secretVal := `{"enabled":true,"config_version":1,"endpoints":[{"id":"ep-1","token":"super-secret-guard-token"}]}`
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.OptionKeyPromptAuditConfigSecret] = secretVal
	common.OptionMapRWMutex.Unlock()

	// 1. GET /api/option/ as Root must NEVER return PromptAuditConfigSecret
	req := httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	req.Header.Set("X-Test-Role", "root")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), model.OptionKeyPromptAuditConfigSecret)
	assert.NotContains(t, rec.Body.String(), "super-secret-guard-token")

	// 2. PUT /api/option/ attempting to modify PromptAuditConfigSecret must be rejected with 403 Forbidden
	updateBody, err := common.Marshal(map[string]any{
		"key":   model.OptionKeyPromptAuditConfigSecret,
		"value": `{"enabled":false}`,
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(updateBody))
	req.Header.Set("X-Test-Role", "root")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "更新提示词审计配置请使用专用管理接口")

	// Verify in-memory OptionMap was not modified
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, secretVal, common.OptionMap[model.OptionKeyPromptAuditConfigSecret])
	common.OptionMapRWMutex.RUnlock()
}

