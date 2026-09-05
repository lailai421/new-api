package promptaudit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckTaskRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	baseCfg := ActiveConfig{
		Enabled:        true,
		AllGroups:      true,
		LatestTurnOnly: false,
	}

	t.Run("blocked prompt returns 403 LocalError", func(t *testing.T) {
		eval := &mockEvaluator{
			decision: &Decision{
				Kind:           DecisionBlock,
				ErrorCode:      ErrorCodeBlocked,
				AllowNextStage: false,
			},
		}
		cleanup := setupGateTest(t, baseCfg, false, eval, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLI(c)
		c.Set("task_request", map[string]any{
			"prompt": "harmful content",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "kling-v1",
		}
		auditMeta := TaskAuditMeta{
			PluginKey:           "kling",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := CheckTaskRequest(c, relayInfo, auditMeta)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
		assert.Equal(t, ErrorCodeBlocked, taskErr.Code)
		assert.True(t, taskErr.LocalError)
	})

	t.Run("pass prompt returns nil and marks done", func(t *testing.T) {
		eval := &mockEvaluator{
			decision: &Decision{
				Kind:           DecisionAllow,
				AllowNextStage: true,
			},
		}
		cleanup := setupGateTest(t, baseCfg, false, eval, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLI(c)
		c.Set("task_request", map[string]any{
			"prompt": "friendly video generation",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "kling-v1",
		}
		auditMeta := TaskAuditMeta{
			PluginKey:           "kling",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := CheckTaskRequest(c, relayInfo, auditMeta)
		require.Nil(t, taskErr)
		assert.True(t, c.GetBool(ContextKeyTaskAuditDone))

		// Second call returns nil immediately without calling evaluator again
		eval.decision = &Decision{Kind: DecisionBlock}
		require.Nil(t, CheckTaskRequest(c, relayInfo, auditMeta))
	})

	t.Run("uncovered submit plugin returns 503 unsupported protocol", func(t *testing.T) {
		cleanup := setupGateTest(t, baseCfg, false, &mockEvaluator{}, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		attachCodexCLI(c)
		c.Set("task_request", map[string]any{
			"prompt": "some text",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "custom-model",
		}
		auditMeta := TaskAuditMeta{
			PluginKey:           "third-party-uncovered",
			AuditTextPaths:      nil, // no contract
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := CheckTaskRequest(c, relayInfo, auditMeta)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		assert.Equal(t, ErrorCodeUnsupportedProtocol, taskErr.Code)
		assert.True(t, taskErr.LocalError)
	})

	t.Run("degraded mode returns 503 config degraded", func(t *testing.T) {
		cleanup := setupGateTest(t, baseCfg, true, &mockEvaluator{}, nil)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		relayInfo := &relaycommon.RelayInfo{}
		auditMeta := TaskAuditMeta{PluginKey: "kling", AuditTextPaths: []string{"/prompt"}, HasSubmitCapability: true}

		taskErr := CheckTaskRequest(c, relayInfo, auditMeta)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		assert.Equal(t, ErrorCodeConfigDegraded, taskErr.Code)
		assert.True(t, taskErr.LocalError)
	})

	t.Run("non Codex CLI returns 503 before extract", func(t *testing.T) {
		eval := &mockEvaluator{}
		store := &mockEventStore{}
		cleanup := setupGateTest(t, baseCfg, false, eval, store)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("task_request", map[string]any{
			"prompt": "CANARY_TASK_PROMPT",
		})
		c.Set("user_id", 1)

		relayInfo := &relaycommon.RelayInfo{
			OriginModelName: "kling-v1",
		}
		auditMeta := TaskAuditMeta{
			PluginKey:           "kling",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		}

		taskErr := CheckTaskRequest(c, relayInfo, auditMeta)
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		assert.Equal(t, ErrorCodeCodexCLIRequired, taskErr.Code)
		assert.Equal(t, CodexCLIRequiredMessage, taskErr.Message)
		assert.True(t, taskErr.LocalError)
		assert.Equal(t, 0, eval.called)
		assert.Len(t, store.recorded, 0)
		assert.False(t, c.GetBool(ContextKeyTaskAuditDone))
	})

	t.Run("plugin protocol non Codex CLI returns 503", func(t *testing.T) {
		eval := &mockEvaluator{}
		store := &mockEventStore{}
		cleanup := setupGateTest(t, baseCfg, false, eval, store)
		defer cleanup()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("task_request", map[string]any{
			"prompt": "CANARY_PLUGIN_PROMPT",
		})

		taskErr := CheckTaskPluginProtocolRequest(c, &relaycommon.RelayInfo{OriginModelName: "sora"}, TaskAuditMeta{
			PluginKey:           "sora",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
			Found:               true,
		})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
		assert.Equal(t, ErrorCodeCodexCLIRequired, taskErr.Code)
		assert.Equal(t, CodexCLIRequiredMessage, taskErr.Message)
		assert.True(t, taskErr.LocalError)
		assert.Equal(t, 0, eval.called)
		assert.Len(t, store.recorded, 0)
	})
}
