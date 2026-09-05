package promptaudit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func attachOriginator(c *gin.Context, value string) {
	if c.Request == nil {
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	}
	if value == "" {
		c.Request.Header.Del("Originator")
		return
	}
	c.Request.Header.Set("Originator", value)
}

func attachCodexCLI(c *gin.Context) {
	attachOriginator(c, "codex_cli_rs")
}

func TestIsCodexCLIRequest(t *testing.T) {
	tests := []struct {
		name    string
		nilReq  bool
		headers map[string]string
		want    bool
	}{
		{name: "codex_cli_rs", headers: map[string]string{"Originator": "codex_cli_rs"}, want: true},
		{name: "CODEX_CLI_RS", headers: map[string]string{"Originator": "CODEX_CLI_RS"}, want: true},
		{name: "codex-cli", headers: map[string]string{"Originator": "codex-cli"}, want: true},
		{name: "CODEX-CLI", headers: map[string]string{"Originator": "CODEX-CLI"}, want: true},
		{name: "codex-tui", headers: map[string]string{"Originator": "codex-tui"}, want: true},
		{name: "CODEX-TUI", headers: map[string]string{"Originator": "CODEX-TUI"}, want: true},
		{name: "codex_exec", headers: map[string]string{"Originator": "codex_exec"}, want: true},
		{name: "CODEX_EXEC", headers: map[string]string{"Originator": "CODEX_EXEC"}, want: true},
		{name: "trimmed allowed value", headers: map[string]string{"Originator": "  codex_cli_rs  "}, want: true},
		{name: "trimmed codex-cli", headers: map[string]string{"Originator": "\tcodex-cli\n"}, want: true},
		{name: "trimmed codex-tui", headers: map[string]string{"Originator": "  codex-tui  "}, want: true},
		{name: "header name case", headers: map[string]string{"ORIGINATOR": "codex-cli"}, want: true},
		{name: "missing header", want: false},
		{name: "empty value", headers: map[string]string{"Originator": ""}, want: false},
		{name: "whitespace only", headers: map[string]string{"Originator": "   "}, want: false},
		{name: "curl", headers: map[string]string{"Originator": "curl"}, want: false},
		{name: "substring wrapper", headers: map[string]string{"Originator": "my-codex_cli_rs-wrapper"}, want: false},
		{name: "tui substring wrapper", headers: map[string]string{"Originator": "my-codex-tui-wrapper"}, want: false},
		{name: "display text Codex CLI", headers: map[string]string{"Originator": "Codex CLI"}, want: false},
		{name: "vscode originator", headers: map[string]string{"Originator": "codex_vscode"}, want: false},
		{name: "desktop originator", headers: map[string]string{"Originator": "Codex Desktop"}, want: false},
		{name: "monitor originator", headers: map[string]string{"Originator": "codex_monitor"}, want: false},
		{
			name: "only Codex User-Agent",
			headers: map[string]string{
				"User-Agent": "codex-cli/0.153.2",
			},
			want: false,
		},
		{
			name: "only session thread and x-codex headers",
			headers: map[string]string{
				"Session_id":            "sess-1",
				"Thread_id":             "thread-1",
				"X-Codex-Beta-Features": "foo",
				"X-Codex-Turn-Metadata": "bar",
			},
			want: false,
		},
		{name: "nil request", nilReq: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if !tt.nilReq {
				req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				for k, v := range tt.headers {
					req.Header.Set(k, v)
				}
			}
			assert.Equal(t, tt.want, IsCodexCLIRequest(req))
		})
	}
}

func TestCheckAuditClientAccess_Order(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("manager nil keeps original behavior", func(t *testing.T) {
		origMgr := GetManager()
		SetGlobalManager(nil)
		defer SetGlobalManager(origMgr)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		cfg, gErr := CheckAuditClientAccess(c)
		assert.Nil(t, gErr)
		assert.False(t, cfg.Enabled)
	})

	t.Run("audit off allows any originator", func(t *testing.T) {
		cleanup := setupGateTest(t, ActiveConfig{Enabled: false}, false, nil, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		cfg, gErr := CheckAuditClientAccess(c)
		require.Nil(t, gErr)
		assert.False(t, cfg.Enabled)

		attachOriginator(c, "curl")
		cfg, gErr = CheckAuditClientAccess(c)
		require.Nil(t, gErr)
		assert.False(t, cfg.Enabled)

		attachCodexCLI(c)
		cfg, gErr = CheckAuditClientAccess(c)
		require.Nil(t, gErr)
		assert.False(t, cfg.Enabled)
	})

	t.Run("degraded precedes client identity", func(t *testing.T) {
		cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, true, nil, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		_, gErr := CheckAuditClientAccess(c)
		require.NotNil(t, gErr)
		assert.Equal(t, ErrorCodeConfigDegraded, gErr.Code)
		assert.Equal(t, http.StatusServiceUnavailable, gErr.HTTPStatus)

		attachCodexCLI(c)
		_, gErr = CheckAuditClientAccess(c)
		require.NotNil(t, gErr)
		assert.Equal(t, ErrorCodeConfigDegraded, gErr.Code)
	})

	t.Run("enabled rejects non Codex CLI", func(t *testing.T) {
		cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, nil, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachOriginator(c, "curl")
		cfg, gErr := CheckAuditClientAccess(c)
		require.NotNil(t, gErr)
		assert.True(t, cfg.Enabled)
		assert.Equal(t, ErrorCodeCodexCLIRequired, gErr.Code)
		assert.Equal(t, http.StatusServiceUnavailable, gErr.HTTPStatus)
		assert.False(t, gErr.Retryable)
		assert.Equal(t, CodexCLIRequiredMessage, gErr.Cause.Error())
		assert.NotContains(t, gErr.Error(), "curl")
	})

	t.Run("enabled allows Codex CLI", func(t *testing.T) {
		cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, nil, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLI(c)
		cfg, gErr := CheckAuditClientAccess(c)
		require.Nil(t, gErr)
		assert.True(t, cfg.Enabled)
	})

	t.Run("enabled allows all canonical originators", func(t *testing.T) {
		cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, nil, nil)
		defer cleanup()

		for _, value := range []string{"codex_cli_rs", "CODEX_CLI_RS", "codex-cli", "  CODEX-CLI\t", "codex-tui", "CODEX-TUI", "codex_exec", "  CODEX_EXEC\t"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			attachOriginator(c, value)
			_, gErr := CheckAuditClientAccess(c)
			require.Nil(t, gErr, value)
		}
	})
}

func TestCheckRelayRequest_CodexCLIRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eval := &mockEvaluator{}
	store := &mockEventStore{}
	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Originator", "curl")
	c.Request.Header.Set("User-Agent", "codex-cli/0.153.2")
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "CANARY_PROMPT_DO_NOT_LEAK"}},
	}
	info := &relaycommon.RelayInfo{UsingGroup: "default"}

	err := CheckRelayRequest(c, info, req)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeCodexCLIRequired), err.GetErrorCode())
	assert.Equal(t, CodexCLIRequiredMessage, err.Error())
	assert.True(t, types.IsSkipRetryError(err))
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	assert.NotContains(t, err.Error(), "CANARY_PROMPT_DO_NOT_LEAK")
	assert.NotContains(t, err.Error(), "curl")
	assert.NotContains(t, err.Error(), "codex-cli/0.153.2")
}

func TestCheckRelayRequest_OfficialTUIAndExecOriginatorsAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, originator := range []string{"codex-tui", "codex_exec"} {
		t.Run(originator, func(t *testing.T) {
			eval := &mockEvaluator{
				decision: &Decision{Kind: DecisionAllow, AllowNextStage: true},
			}
			store := &mockEventStore{}
			cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
			defer cleanup()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("Originator", originator)
			err := CheckRelayRequest(c, &relaycommon.RelayInfo{UsingGroup: "default"}, &dto.GeneralOpenAIRequest{
				Model:    "gpt-4o",
				Messages: []dto.Message{{Role: "user", Content: "你好"}},
			})
			assert.Nil(t, err)
			assert.Equal(t, 1, eval.called)
			assert.Len(t, store.recorded, 0)
		})
	}
}

func TestCheckRelayRequest_AuxiliaryHeadersDoNotAuthorize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eval := &mockEvaluator{}
	store := &mockEventStore{}
	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("User-Agent", "codex-cli/0.153.2")
	c.Request.Header.Set("Session_id", "sess-1")
	c.Request.Header.Set("Thread_id", "thread-1")
	c.Request.Header.Set("X-Codex-Beta-Features", "foo")

	err := CheckRelayRequest(c, &relaycommon.RelayInfo{UsingGroup: "default"}, &dto.GeneralOpenAIRequest{
		Model:    "gpt-4o",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
	})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	assert.Equal(t, types.ErrorCode(ErrorCodeCodexCLIRequired), err.GetErrorCode())
	assert.Equal(t, CodexCLIRequiredMessage, err.Error())
	assert.True(t, types.IsSkipRetryError(err))
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
	assert.NotContains(t, err.Error(), "codex-cli/0.153.2")
	assert.NotContains(t, err.Error(), "sess-1")
}

func TestCheckMidjourneyRequest_CodexCLIRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eval := &mockEvaluator{}
	store := &mockEventStore{}
	cleanup := setupGateTest(t, ActiveConfig{Enabled: true, AllGroups: true}, false, eval, store)
	defer cleanup()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", nil)
	info := &relaycommon.RelayInfo{UsingGroup: "default", OriginModelName: "midjourney"}

	mjErr := CheckMidjourneyRequest(c, info)
	require.NotNil(t, mjErr)
	assert.Equal(t, http.StatusServiceUnavailable, mjErr.Code)
	assert.Equal(t, CodexCLIRequiredMessage, mjErr.Description)
	assert.Equal(t, ErrorCodeCodexCLIRequired, mjErr.Result)
	assert.Equal(t, 0, eval.called)
	assert.Len(t, store.recorded, 0)
}
