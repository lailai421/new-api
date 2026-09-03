package promptaudit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRealtimeEvent_SessionUpdate(t *testing.T) {
	// 包含 instructions 和 tools parameters description
	rawJSON := `{
		"type": "session.update",
		"session": {
			"instructions": "You are a helpful assistant.",
			"voice": "alloy",
			"tools": [
				{
					"type": "function",
					"name": "get_weather",
					"description": "Get current weather in location",
					"parameters": {
						"type": "object",
						"description": "Weather query parameters",
						"properties": {
							"target_city_key": {
								"type": "string",
								"description": "City name"
							}
						}
					}
				}
			]
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientSessionUpdate, eventType)
	require.Len(t, segments, 4)

	assert.Equal(t, RoleSystem, segments[0].Role)
	assert.Equal(t, "You are a helpful assistant.", segments[0].Content)

	assert.Equal(t, RoleSystem, segments[1].Role)
	assert.Equal(t, "Get current weather in location", segments[1].Content)

	// parameters schema descriptions
	assert.Equal(t, RoleSystem, segments[2].Role)
	assert.Equal(t, "Weather query parameters", segments[2].Content)

	assert.Equal(t, RoleSystem, segments[3].Role)
	assert.Equal(t, "City name", segments[3].Content)

	// 验证字段名称（如 "get_weather", "target_city_key", "properties", "alloy"）未被误扫为 PromptSegment
	fullText := JoinSegments(segments)
	assert.NotContains(t, fullText, "get_weather")
	assert.NotContains(t, fullText, "alloy")
	assert.NotContains(t, fullText, "target_city_key")
}

func TestExtractRealtimeEvent_ConversationItemCreate_Text(t *testing.T) {
	rawJSON := `{
		"type": "conversation.item.create",
		"item": {
			"id": "item_123",
			"type": "message",
			"role": "user",
			"content": [
				{
					"type": "input_text",
					"text": "Hello world from realtime user"
				}
			]
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientConversationCreate, eventType)
	require.Len(t, segments, 1)
	assert.Equal(t, RoleUser, segments[0].Role)
	assert.True(t, segments[0].User)
	assert.Equal(t, "Hello world from realtime user", segments[0].Content)
}

func TestExtractRealtimeEvent_ConversationItemCreate_AudioTranscript(t *testing.T) {
	// 音频帧携带 Base64 音频与转写文本，要求排除 Base64 音频并提取转写文本
	rawJSON := `{
		"type": "conversation.item.create",
		"item": {
			"id": "item_audio_123",
			"type": "message",
			"role": "user",
			"content": [
				{
					"type": "input_audio",
					"audio": "UklGRi4AAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA=",
					"transcript": "Audio message transcript text"
				}
			]
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientConversationCreate, eventType)
	require.Len(t, segments, 1)
	assert.Equal(t, "Audio message transcript text", segments[0].Content)

	fullText := JoinSegments(segments)
	assert.NotContains(t, fullText, "UklGRi4AAABXQVZFZm10")
}

func TestExtractRealtimeEvent_ConversationItemCreate_FunctionOutput(t *testing.T) {
	rawJSON := `{
		"type": "conversation.item.create",
		"item": {
			"id": "item_func_out",
			"type": "function_call_output",
			"call_id": "call_abc123",
			"output": "Temperature is 25 degrees Celsius"
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientConversationCreate, eventType)
	require.Len(t, segments, 1)
	assert.Equal(t, RoleTool, segments[0].Role)
	assert.False(t, segments[0].User)
	assert.Equal(t, "Temperature is 25 degrees Celsius", segments[0].Content)
}

func TestExtractRealtimeEvent_ResponseCreate(t *testing.T) {
	rawJSON := `{
		"type": "response.create",
		"response": {
			"instructions": "Be concise.",
			"input": [
				{
					"type": "message",
					"role": "user",
					"content": [
						{
							"type": "text",
							"text": "What is the capital of France?"
						}
					]
				}
			]
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientResponseCreate, eventType)
	require.Len(t, segments, 2)
	assert.Equal(t, RoleSystem, segments[0].Role)
	assert.Equal(t, "Be concise.", segments[0].Content)

	assert.Equal(t, RoleUser, segments[1].Role)
	assert.Equal(t, "What is the capital of France?", segments[1].Content)
}

func TestExtractRealtimeEvent_NoPromptControlAndAudioEvents(t *testing.T) {
	controlCases := []struct {
		name    string
		rawJSON string
		expType string
	}{
		{
			name:    "audio append",
			rawJSON: `{"type": "input_audio_buffer.append", "audio": "base64bytes..."}`,
			expType: RealtimeEventClientInputAudioAppend,
		},
		{
			name:    "audio commit",
			rawJSON: `{"type": "input_audio_buffer.commit"}`,
			expType: RealtimeEventClientInputAudioCommit,
		},
		{
			name:    "audio clear",
			rawJSON: `{"type": "input_audio_buffer.clear"}`,
			expType: RealtimeEventClientInputAudioClear,
		},
		{
			name:    "item truncate",
			rawJSON: `{"type": "conversation.item.truncate", "item_id": "item_1", "content_index": 0, "audio_end_ms": 1500}`,
			expType: RealtimeEventClientConversationTruncate,
		},
		{
			name:    "item delete",
			rawJSON: `{"type": "conversation.item.delete", "item_id": "item_1"}`,
			expType: RealtimeEventClientConversationDelete,
		},
		{
			name:    "response cancel",
			rawJSON: `{"type": "response.cancel"}`,
			expType: RealtimeEventClientResponseCancel,
		},
		{
			name:    "session update with empty instructions",
			rawJSON: `{"type": "session.update", "session": {"instructions": ""}}`,
			expType: RealtimeEventClientSessionUpdate,
		},
		{
			name:    "item create with empty text",
			rawJSON: `{"type": "conversation.item.create", "item": {"type": "message", "role": "user", "content": [{"type": "text", "text": "   "}]}}`,
			expType: RealtimeEventClientConversationCreate,
		},
	}

	for _, tc := range controlCases {
		t.Run(tc.name, func(t *testing.T) {
			segs, eventType, err := ExtractRealtimeEvent([]byte(tc.rawJSON))
			require.ErrorIs(t, err, ErrNoPrompt)
			assert.Equal(t, tc.expType, eventType)
			assert.Nil(t, segs)
		})
	}
}

func TestExtractRealtimeEvent_UnsupportedAndMalformed(t *testing.T) {
	invalidCases := []struct {
		name    string
		rawJSON string
	}{
		{
			name:    "empty json",
			rawJSON: ``,
		},
		{
			name:    "invalid json syntax",
			rawJSON: `{"type": "session.update", broken`,
		},
		{
			name:    "missing type field",
			rawJSON: `{"session": {"instructions": "hello"}}`,
		},
		{
			name:    "unknown event type",
			rawJSON: `{"type": "custom.unknown.event", "payload": "some secret text"}`,
		},
		{
			name:    "type mismatch - session is string",
			rawJSON: `{"type": "session.update", "session": "malicious string"}`,
		},
		{
			name:    "type mismatch - item is int",
			rawJSON: `{"type": "conversation.item.create", "item": 12345}`,
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			segs, _, err := ExtractRealtimeEvent([]byte(tc.rawJSON))
			require.ErrorIs(t, err, ErrUnsupportedProtocol)
			assert.Nil(t, segs)
		})
	}
}

func TestExtractRealtimeEvent_UnicodeAndMultiContent(t *testing.T) {
	rawJSON := `{
		"type": "conversation.item.create",
		"item": {
			"role": "user",
			"content": [
				{"type": "text", "text": "你好，世界！🌍"},
				{"type": "input_audio", "audio": "xyz...", "transcript": "这是语音转写的文本：测试 123"}
			]
		}
	}`

	segments, eventType, err := ExtractRealtimeEvent([]byte(rawJSON))
	require.NoError(t, err)
	assert.Equal(t, RealtimeEventClientConversationCreate, eventType)
	require.Len(t, segments, 2)
	assert.Equal(t, "你好，世界！🌍", segments[0].Content)
	assert.Equal(t, "这是语音转写的文本：测试 123", segments[1].Content)
}
