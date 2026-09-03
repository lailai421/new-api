package promptaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRelayRequest_GeneralOpenAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. 包含各种角色与排除多媒体 Base64
	base64Payload := "data:image/png;base64," + strings.Repeat("A", 300)
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "system", Content: "系统预设指令"},
			{Role: "developer", Content: "开发者策略"},
			{Role: "user", Content: "第一轮用户问题"},
			{Role: "assistant", Content: "第一轮助手回复"},
			{Role: "tool", Content: "工具执行结果文本"},
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "最新用户问题第一部分"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": base64Payload}},
					map[string]any{"type": "text", "text": "最新用户问题第二部分"},
				},
			},
		},
	}

	segments, protocol, modelName, err := ExtractRelayRequest(req, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_chat_completions", protocol)
	assert.Equal(t, "gpt-4o", modelName)
	assert.Len(t, segments, 7)

	// 验证 Base64 被完全排除
	fullPrompt := JoinSegments(segments)
	assert.NotContains(t, fullPrompt, "image/png;base64")
	assert.NotContains(t, fullPrompt, strings.Repeat("A", 100))

	// 验证 FullPrompt 包含完整上下文
	expectedFullParts := []string{
		"系统预设指令",
		"开发者策略",
		"第一轮用户问题",
		"第一轮助手回复",
		"工具执行结果文本",
		"最新用户问题第一部分",
		"最新用户问题第二部分",
	}
	assert.Equal(t, strings.Join(expectedFullParts, "\n\n"), fullPrompt)

	// 验证最新轮 ScanText 提取
	latestTurn := SelectLatestTurnSegments(segments)
	scanText := BuildScanText(latestTurn)
	assert.Equal(t, "最新用户问题第一部分\n\n最新用户问题第二部分"+PrioritySeparator+"第一轮助手回复", scanText)

	// 2. Completions Prompt
	completionsReq := &dto.GeneralOpenAIRequest{
		Model:  "text-davinci-003",
		Prompt: []any{"completions 第一段", "completions 第二段"},
	}
	compSegments, compProto, _, compErr := ExtractRelayRequest(completionsReq, c)
	require.NoError(t, compErr)
	assert.Equal(t, "openai_chat_completions", compProto)
	assert.Equal(t, "completions 第一段\n\ncompletions 第二段", JoinSegments(compSegments))
}

func TestExtractRelayRequest_Claude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	sysText := "Claude 顶层系统设定"
	userText := "用户输入的提示词"
	assistantText := "助手之前的回复"
	latestUserText := "最新追问内容"

	req := &dto.ClaudeRequest{
		Model:  "claude-3-5-sonnet-20241022",
		System: sysText,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: userText},
			{Role: "assistant", Content: assistantText},
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": latestUserText},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "data": "BASE64IMAGE"}},
				},
			},
		},
	}

	segments, protocol, modelName, err := ExtractRelayRequest(req, c)
	require.NoError(t, err)
	assert.Equal(t, "claude_messages", protocol)
	assert.Equal(t, "claude-3-5-sonnet-20241022", modelName)

	full := JoinSegments(segments)
	assert.Contains(t, full, sysText)
	assert.Contains(t, full, userText)
	assert.Contains(t, full, assistantText)
	assert.Contains(t, full, latestUserText)
	assert.NotContains(t, full, "BASE64IMAGE")

	latestScan := BuildScanText(SelectLatestTurnSegments(segments))
	assert.Equal(t, latestUserText+PrioritySeparator+assistantText, latestScan)
}

func TestExtractRelayRequest_Gemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	chatReq := &dto.GeminiChatRequest{
		SystemInstructions: &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: "Gemini 系统指令"}},
		},
		Contents: []dto.GeminiChatContent{
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: "Gemini 历史用户问题"},
					{InlineData: &dto.GeminiInlineData{MimeType: "image/jpeg", Data: "BINARYDATA"}},
				},
			},
			{
				Role: "model",
				Parts: []dto.GeminiPart{
					{Text: "Gemini 历史模型回答"},
				},
			},
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: "Gemini 最新输入"},
				},
			},
		},
	}

	segments, protocol, _, err := ExtractRelayRequest(chatReq, c)
	require.NoError(t, err)
	assert.Equal(t, "gemini", protocol)
	full := JoinSegments(segments)
	assert.Equal(t, "Gemini 系统指令\n\nGemini 历史用户问题\n\nGemini 历史模型回答\n\nGemini 最新输入", full)
	assert.NotContains(t, full, "BINARYDATA")

	latestScan := BuildScanText(SelectLatestTurnSegments(segments))
	assert.Equal(t, "Gemini 最新输入"+PrioritySeparator+"Gemini 历史模型回答", latestScan)

	// Gemini Embedding
	embReq := &dto.GeminiEmbeddingRequest{
		Model: "text-embedding-004",
		Content: dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: "Gemini 嵌入向量文本"}},
		},
	}
	embSegments, embProto, embModel, embErr := ExtractRelayRequest(embReq, c)
	require.NoError(t, embErr)
	assert.Equal(t, "gemini_embedding", embProto)
	assert.Equal(t, "text-embedding-004", embModel)
	assert.Equal(t, "Gemini 嵌入向量文本", JoinSegments(embSegments))

	// Gemini Batch Embedding
	batchReq := &dto.GeminiBatchEmbeddingRequest{
		Requests: []*dto.GeminiEmbeddingRequest{
			embReq,
			{
				Model: "text-embedding-004",
				Content: dto.GeminiChatContent{
					Parts: []dto.GeminiPart{{Text: "批量嵌入第二条"}},
				},
			},
		},
	}
	batchSegments, batchProto, _, batchErr := ExtractRelayRequest(batchReq, c)
	require.NoError(t, batchErr)
	assert.Equal(t, "gemini_batch_embedding", batchProto)
	assert.Equal(t, "Gemini 嵌入向量文本\n\n批量嵌入第二条", JoinSegments(batchSegments))
}

func TestExtractRelayRequest_ResponsesAndCompaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	instJSON, err := common.Marshal("Responses 指令说明")
	require.NoError(t, err)
	inputJSON, err := common.Marshal([]any{
		map[string]any{"role": "user", "content": "用户首轮问答"},
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "模型先前生成回复"}}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "最新 Responses 请求文本"}}},
	})
	require.NoError(t, err)

	respReq := &dto.OpenAIResponsesRequest{
		Model:        "gpt-4o",
		Instructions: instJSON,
		Input:        inputJSON,
	}

	segments, protocol, _, err := ExtractRelayRequest(respReq, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_responses", protocol)
	full := JoinSegments(segments)
	assert.Equal(t, "Responses 指令说明\n\n用户首轮问答\n\n模型先前生成回复\n\n最新 Responses 请求文本", full)

	latestScan := BuildScanText(SelectLatestTurnSegments(segments))
	assert.Equal(t, "最新 Responses 请求文本"+PrioritySeparator+"模型先前生成回复", latestScan)

	// Compaction Request
	compactReq := &dto.OpenAIResponsesCompactionRequest{
		Model:        "gpt-4o",
		Instructions: instJSON,
		Input:        inputJSON,
	}
	compSegments, compProto, _, compErr := ExtractRelayRequest(compactReq, c)
	require.NoError(t, compErr)
	assert.Equal(t, "openai_responses_compaction", compProto)
	assert.Equal(t, full, JoinSegments(compSegments))
}

func TestExtractRelayRequest_AlphaSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	rawBody, err := common.Marshal(map[string]any{
		"query": "AlphaSearch 测试搜索词",
		"model": "gpt-5.1",
	})
	require.NoError(t, err)

	req := &dto.AlphaSearchRequest{
		Model:   "gpt-5.1",
		RawBody: rawBody,
	}

	segments, protocol, modelName, err := ExtractRelayRequest(req, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_alpha_search", protocol)
	assert.Equal(t, "gpt-5.1", modelName)
	assert.Equal(t, "AlphaSearch 测试搜索词", JoinSegments(segments))
}

func TestExtractRelayRequest_Image_Embedding_Rerank_Audio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. ImageRequest
	negJSON, _ := common.Marshal("不要模糊 不要变形")
	imgReq := &dto.ImageRequest{
		Model:  "dall-e-3",
		Prompt: "画一只在月球上奔跑的机械猫",
		Extra: map[string]json.RawMessage{
			"negative_prompt": negJSON,
		},
	}
	imgSegments, imgProto, _, imgErr := ExtractRelayRequest(imgReq, c)
	require.NoError(t, imgErr)
	assert.Equal(t, "openai_images", imgProto)
	assert.Equal(t, "画一只在月球上奔跑的机械猫\n\n不要模糊 不要变形", JoinSegments(imgSegments))

	// 2. EmbeddingRequest
	embReq := &dto.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []any{"文本嵌入第一句", "文本嵌入第二句"},
	}
	embSegments, embProto, _, embErr := ExtractRelayRequest(embReq, c)
	require.NoError(t, embErr)
	assert.Equal(t, "openai_embeddings", embProto)
	assert.Equal(t, "文本嵌入第一句\n\n文本嵌入第二句", JoinSegments(embSegments))

	// 3. RerankRequest
	rerankReq := &dto.RerankRequest{
		Model:     "bge-reranker-large",
		Query:     "什么是重排模型？",
		Documents: []any{"候选文档一", map[string]any{"text": "候选文档二"}},
	}
	rerankSegments, rerankProto, _, rerankErr := ExtractRelayRequest(rerankReq, c)
	require.NoError(t, rerankErr)
	assert.Equal(t, "rerank", rerankProto)
	assert.Equal(t, "什么是重排模型？\n\n候选文档一\n\n候选文档二", JoinSegments(rerankSegments))

	// 4. AudioRequest
	audioReq := &dto.AudioRequest{
		Model:        "tts-1",
		Input:        "需要合成语音的文本段落",
		Instructions: "用欢快的语气朗读",
	}
	audioSegments, audioProto, _, audioErr := ExtractRelayRequest(audioReq, c)
	require.NoError(t, audioErr)
	assert.Equal(t, "openai_audio", audioProto)
	assert.Equal(t, "需要合成语音的文本段落\n\n用欢快的语气朗读", JoinSegments(audioSegments))
}

func TestExtractRelayRequest_NoPromptAndUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. 空输入返回 ErrNoPrompt
	emptyChat := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "user", Content: "   \n\t  "},
		},
	}
	_, _, _, err := ExtractRelayRequest(emptyChat, c)
	require.ErrorIs(t, err, ErrNoPrompt)

	// 2. 未知 DTO 格式返回 ErrUnsupportedProtocol
	type unknownRequest struct {
		dto.BaseRequest
	}
	_, _, _, err = ExtractRelayRequest(&unknownRequest{}, c)
	require.ErrorIs(t, err, ErrUnsupportedProtocol)

	// nil request
	_, _, _, err = ExtractRelayRequest(nil, c)
	require.ErrorIs(t, err, ErrUnsupportedProtocol)
}

func TestPromptHashAndRedactedPreview(t *testing.T) {
	text := "用户敏感指令 Bearer sk-ant-api03-abcdef123456789012345678 password=SuperSecretPassword123 test@example.com +86 13800138000"
	preview := BuildPromptPreview(text)

	// 脱敏断言
	assert.NotContains(t, preview, "sk-ant-api03")
	assert.NotContains(t, preview, "SuperSecretPassword123")
	assert.NotContains(t, preview, "test@example.com")
	assert.NotContains(t, preview, "13800138000")
	assert.LessOrEqual(t, utf8.RuneCountInString(preview), MaxRedactedPreviewRunes)

	// Hash 验证
	expectedHash := sha256.Sum256([]byte(text))
	assert.Equal(t, hex.EncodeToString(expectedHash[:]), CalculatePromptHash(text))
}

func TestExtractRelayRequest_FIM_Edits_Moderation_Reasoning_Tools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. FIM (Fill-in-the-middle) 请求
	fimReq := &dto.GeneralOpenAIRequest{
		Model:  "deepseek-coder",
		Prefix: "def calculate_area(width, height):\n",
		Suffix: "\n    return area\n",
	}
	segments, proto, model, err := ExtractRelayRequest(fimReq, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_chat_completions", proto)
	assert.Equal(t, "deepseek-coder", model)
	assert.Equal(t, "def calculate_area(width, height):\n\nreturn area", JoinSegments(segments))

	// 2. Edits 请求 (Instruction + Input)
	editReq := &dto.GeneralOpenAIRequest{
		Model:       "text-davinci-edit-001",
		Instruction: "修复所有英文拼写错误",
		Input:       "Ths is an exmple sentnce.",
	}
	segments, proto, _, err = ExtractRelayRequest(editReq, c)
	require.NoError(t, err)
	assert.Equal(t, "openai_chat_completions", proto)
	assert.Contains(t, JoinSegments(segments), "修复所有英文拼写错误")
	assert.Contains(t, JoinSegments(segments), "Ths is an exmple sentnce.")

	// 3. 带有 ReasoningContent 与 ToolCalls 的助手消息
	reasoningText := "深度思考逻辑分析过程"
	toolCallsJSON, _ := common.Marshal([]dto.ToolCallRequest{
		{
			ID:   "call_abc",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "execute_query",
				Arguments: "{\"sql\": \"SELECT * FROM users\"}",
			},
		},
	})
	chatReq := &dto.GeneralOpenAIRequest{
		Model: "deepseek-r1",
		Messages: []dto.Message{
			{Role: "user", Content: "查询用户数据"},
			{
				Role:             "assistant",
				Content:          "好的，正在查询。",
				ReasoningContent: &reasoningText,
				ToolCalls:        toolCallsJSON,
			},
			{Role: "tool", Content: "查询结果：3条记录"},
		},
	}
	segments, _, _, err = ExtractRelayRequest(chatReq, c)
	require.NoError(t, err)
	fullPrompt := JoinSegments(segments)
	assert.Contains(t, fullPrompt, "查询用户数据")
	assert.Contains(t, fullPrompt, "好的，正在查询。")
	assert.Contains(t, fullPrompt, "深度思考逻辑分析过程")
	assert.Contains(t, fullPrompt, "{\"sql\": \"SELECT * FROM users\"}")
	assert.Contains(t, fullPrompt, "查询结果：3条记录")
}

func TestExtractRelayRequest_Claude_Thinking_And_ToolResultArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	thinkingStr := "Claude 内部思考链细节"
	req := &dto.ClaudeRequest{
		Model: "claude-3-7-sonnet-20250219",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "请编写排序算法"},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": "thinking", "thinking": thinkingStr},
					map[string]any{"type": "text", "text": "这是排序算法实现。"},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "tool_result",
						"content": []any{
							map[string]any{"type": "text", "text": "测试用例通过：100%"},
						},
					},
				},
			},
		},
	}

	segments, proto, _, err := ExtractRelayRequest(req, c)
	require.NoError(t, err)
	assert.Equal(t, "claude_messages", proto)
	full := JoinSegments(segments)
	assert.Contains(t, full, "请编写排序算法")
	assert.Contains(t, full, "Claude 内部思考链细节")
	assert.Contains(t, full, "这是排序算法实现。")
	assert.Contains(t, full, "测试用例通过：100%")
}

func TestExtractRelayRequest_Gemini_CodeExecution_And_FunctionResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	chatReq := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: "请计算斐波那契数列第 10 项"},
				},
			},
			{
				Role: "model",
				Parts: []dto.GeminiPart{
					{
						CodeExecutionResult: &dto.GeminiPartCodeExecutionResult{
							Outcome: "OUTCOME_OK",
							Output:  "55",
						},
					},
					{
						FunctionResponse: &dto.GeminiFunctionResponse{
							Name: "fibonacci",
							Response: map[string]any{
								"result": "55",
							},
						},
					},
				},
			},
		},
	}

	segments, proto, _, err := ExtractRelayRequest(chatReq, c)
	require.NoError(t, err)
	assert.Equal(t, "gemini", proto)
	full := JoinSegments(segments)
	assert.Contains(t, full, "请计算斐波那契数列第 10 项")
	assert.Contains(t, full, "55")
}

func TestExtractRelayRequest_URLInPromptNotDropped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 1. 带有 URL 开头的提示词（绝对不能因为以 https:// 开头就被错误当成图片/媒体 URL 丢弃）
	promptWithURL := "https://example.com/api 这是接口文档，请帮我分析其入参格式"
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "user", Content: promptWithURL},
		},
	}
	segments, _, _, err := ExtractRelayRequest(req, c)
	require.NoError(t, err)
	full := JoinSegments(segments)
	assert.Equal(t, promptWithURL, full)

	// 2. 纯独立 URL（不含任何文本，纯链接）应当被识别为纯 URL 并排除
	pureURLReq := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "user", Content: "https://example.com/image.png"},
		},
	}
	_, _, _, err = ExtractRelayRequest(pureURLReq, c)
	require.ErrorIs(t, err, ErrNoPrompt)

	// 3. 纯 Data URL 应当被识别并排除
	dataURLReq := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "user", Content: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="},
		},
	}
	_, _, _, err = ExtractRelayRequest(dataURLReq, c)
	require.ErrorIs(t, err, ErrNoPrompt)
}
