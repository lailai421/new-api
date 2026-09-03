package promptaudit

import (
	"net/http/httptest"
	"testing"

	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateJSONPointer(t *testing.T) {
	data := map[string]any{
		"prompt": "hello world",
		"nested": map[string]any{
			"message": "nested value",
			"items": []any{
				"first",
				"second",
				map[string]any{"key": "deep"},
			},
		},
		"escaped/key": map[string]any{
			"tilde~name": "escaped value",
		},
	}

	t.Run("valid simple path", func(t *testing.T) {
		val, found, err := EvaluateJSONPointer(data, "/prompt")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "hello world", val)
	})

	t.Run("valid nested map path", func(t *testing.T) {
		val, found, err := EvaluateJSONPointer(data, "/nested/message")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "nested value", val)
	})

	t.Run("valid array indexing", func(t *testing.T) {
		val0, found0, err0 := EvaluateJSONPointer(data, "/nested/items/0")
		require.NoError(t, err0)
		assert.True(t, found0)
		assert.Equal(t, "first", val0)

		val1, found1, err1 := EvaluateJSONPointer(data, "/nested/items/1")
		require.NoError(t, err1)
		assert.True(t, found1)
		assert.Equal(t, "second", val1)

		valDeep, foundDeep, errDeep := EvaluateJSONPointer(data, "/nested/items/2/key")
		require.NoError(t, errDeep)
		assert.True(t, foundDeep)
		assert.Equal(t, "deep", valDeep)
	})

	t.Run("valid RFC 6901 escapes", func(t *testing.T) {
		val, found, err := EvaluateJSONPointer(data, "/escaped~1key/tilde~0name")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, "escaped value", val)
	})

	t.Run("missing keys and out-of-bounds array", func(t *testing.T) {
		_, found, err := EvaluateJSONPointer(data, "/not_exist")
		require.NoError(t, err)
		assert.False(t, found)

		_, found, err = EvaluateJSONPointer(data, "/nested/items/99")
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("invalid pointer syntax", func(t *testing.T) {
		_, _, err := EvaluateJSONPointer(data, "no_leading_slash")
		require.Error(t, err)

		_, _, err = EvaluateJSONPointer(data, "/nested/items/01")
		require.Error(t, err)

		_, _, err = EvaluateJSONPointer(data, "/nested/items/not_a_number")
		require.Error(t, err)

		_, _, err = EvaluateJSONPointer(data, "/prompt/cannot_index_string")
		require.Error(t, err)
	})
}

func TestExtractTaskRequest(t *testing.T) {
	t.Run("extract valid string and array", func(t *testing.T) {
		body := map[string]any{
			"prompt":          "Generate a cat video",
			"negative_prompt": "blurry, dark",
			"tags":            []any{"lofi", "chill"},
		}
		meta := TaskAuditMeta{
			PluginKey:           "test-plugin",
			AuditTextPaths:      []string{"/prompt", "/negative_prompt", "/tags"},
			HasSubmitCapability: true,
		}
		segments, err := ExtractTaskRequest(body, meta)
		require.NoError(t, err)
		require.Len(t, segments, 4)
		assert.Equal(t, "Generate a cat video", segments[0].Content)
		assert.Equal(t, "blurry, dark", segments[1].Content)
		assert.Equal(t, "lofi", segments[2].Content)
		assert.Equal(t, "chill", segments[3].Content)
	})

	t.Run("extract valid text content block", func(t *testing.T) {
		body := map[string]any{
			"message": map[string]any{
				"type": "text",
				"text": "Valid text block",
			},
		}
		meta := TaskAuditMeta{
			PluginKey:           "test-plugin",
			AuditTextPaths:      []string{"/message"},
			HasSubmitCapability: true,
		}
		segments, err := ExtractTaskRequest(body, meta)
		require.NoError(t, err)
		require.Len(t, segments, 1)
		assert.Equal(t, "Valid text block", segments[0].Content)
	})

	t.Run("reject forbidden media container in content block", func(t *testing.T) {
		body := map[string]any{
			"bad_block": map[string]any{
				"type":      "text",
				"text":      "something",
				"image_url": "https://evil.com/img.png",
			},
		}
		meta := TaskAuditMeta{
			PluginKey:           "test-plugin",
			AuditTextPaths:      []string{"/bad_block"},
			HasSubmitCapability: true,
		}
		_, err := ExtractTaskRequest(body, meta)
		require.ErrorIs(t, err, ErrUnsupportedProtocol)
	})

	t.Run("reject non-string non-text object", func(t *testing.T) {
		body := map[string]any{
			"bad_val": map[string]any{
				"random_key": 123,
			},
		}
		meta := TaskAuditMeta{
			PluginKey:           "test-plugin",
			AuditTextPaths:      []string{"/bad_val"},
			HasSubmitCapability: true,
		}
		_, err := ExtractTaskRequest(body, meta)
		require.ErrorIs(t, err, ErrUnsupportedProtocol)
	})

	t.Run("fail-closed when submit plugin has no auditTextPaths", func(t *testing.T) {
		body := map[string]any{
			"prompt": "some prompt",
		}
		meta := TaskAuditMeta{
			PluginKey:           "uncovered-submit-plugin",
			AuditTextPaths:      nil,
			HasSubmitCapability: true,
		}
		_, err := ExtractTaskRequest(body, meta)
		require.ErrorIs(t, err, ErrUnsupportedProtocol)
	})

	t.Run("returns ErrNoPrompt when covered plugin has no text content", func(t *testing.T) {
		body := map[string]any{
			"other_field": "123",
		}
		meta := TaskAuditMeta{
			PluginKey:           "covered-plugin",
			AuditTextPaths:      []string{"/prompt"},
			HasSubmitCapability: true,
		}
		_, err := ExtractTaskRequest(body, meta)
		require.ErrorIs(t, err, ErrNoPrompt)
	})
}

func TestExtractTaskPluginResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("dual source extraction and stable deduplication", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// Set Source A: ProtocolRequestContext with standard Responses input
		rawReq := relaydto.OpenAIResponsesRequest{
			Instructions: []byte(`"System instructions"`),
			Input:        []byte(`[{"role":"user","content":"User input message"}]`),
		}
		protoContext := map[string]any{
			"requestBody": map[string]any{
				"instructions": "System instructions",
				"input": []any{
					map[string]any{
						"role":    "user",
						"content": "User input message",
					},
				},
			},
		}
		// Directly marshaled test context
		c.Set("protocol_request", protoContext)
		_ = rawReq

		// Set Source B: task_request with prompt and new field
		taskBody := map[string]any{
			"prompt":           "User input message", // duplicate with Source A
			"supplement_field": "Extra prompt text",
		}
		meta := TaskAuditMeta{
			PluginKey:           "responses-plugin",
			AuditTextPaths:      []string{"/prompt", "/supplement_field"},
			HasSubmitCapability: true,
		}

		segments, err := ExtractTaskPluginResponsesRequest(c, taskBody, meta)
		require.NoError(t, err)

		// Should contain: System instructions, User input message, Extra prompt text (deduplicated!)
		require.Len(t, segments, 3)
		assert.Equal(t, "System instructions", segments[0].Content)
		assert.Equal(t, "User input message", segments[1].Content)
		assert.Equal(t, "Extra prompt text", segments[2].Content)
	})
}
