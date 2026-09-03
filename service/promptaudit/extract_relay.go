package promptaudit

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// ExtractRelayRequest 从 relaykit 的具体 DTO 中确定性提取提示词文本分段。
// 严格排除 URL、Data URL、Base64 载荷、模型名、回调地址和元数据。
// 确定无文本时返回 ErrNoPrompt；未覆盖/未知协议格式返回 ErrUnsupportedProtocol。
func ExtractRelayRequest(request dto.Request, c *gin.Context) (segments []PromptSegment, protocol string, modelName string, err error) {
	if request == nil {
		return nil, "", "", ErrUnsupportedProtocol
	}

	switch req := request.(type) {
	case *dto.GeneralOpenAIRequest:
		protocol = "openai_chat_completions"
		modelName = req.Model
		segments = extractOpenAIRequest(req)

	case *dto.ClaudeRequest:
		protocol = "claude_messages"
		modelName = req.Model
		segments = extractClaudeRequest(req)

	case *dto.GeminiChatRequest:
		protocol = "gemini"
		segments = extractGeminiChatRequest(req)

	case *dto.GeminiEmbeddingRequest:
		protocol = "gemini_embedding"
		modelName = req.Model
		segments = extractGeminiEmbeddingRequest(req)

	case *dto.GeminiBatchEmbeddingRequest:
		protocol = "gemini_batch_embedding"
		segments, modelName = extractGeminiBatchEmbeddingRequest(req)

	case *dto.OpenAIResponsesRequest:
		protocol = "openai_responses"
		modelName = req.Model
		segments = extractResponsesRequest(req)

	case *dto.OpenAIResponsesCompactionRequest:
		protocol = "openai_responses_compaction"
		modelName = req.Model
		segments = extractResponsesCompactionRequest(req)

	case *dto.AlphaSearchRequest:
		protocol = "openai_alpha_search"
		modelName = req.Model
		segments = extractAlphaSearchRequest(req)

	case *dto.ImageRequest:
		protocol = "openai_images"
		modelName = req.Model
		segments = extractImageRequest(req)

	case *dto.EmbeddingRequest:
		protocol = "openai_embeddings"
		modelName = req.Model
		segments = extractEmbeddingRequest(req)

	case *dto.RerankRequest:
		protocol = "rerank"
		modelName = req.Model
		segments = extractRerankRequest(req)

	case *dto.AudioRequest:
		protocol = "openai_audio"
		modelName = req.Model
		segments = extractAudioRequest(req, c)

	default:
		return nil, "", "", ErrUnsupportedProtocol
	}

	normalized := NormalizeSegments(segments)
	if len(normalized) == 0 {
		return nil, protocol, modelName, ErrNoPrompt
	}

	return normalized, protocol, modelName, nil
}

func extractOpenAIRequest(req *dto.GeneralOpenAIRequest) []PromptSegment {
	var segments []PromptSegment

	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = RoleUser
		}

		if msg.Content != nil {
			switch v := msg.Content.(type) {
			case string:
				if text := strings.TrimSpace(v); text != "" && !looksLikeMediaPayload(text) {
					segments = append(segments, PromptSegment{
						Role:    role,
						Content: text,
						User:    role == RoleUser,
					})
				}
			case []any:
				for _, part := range msg.ParseContent() {
					if part.Type == dto.ContentTypeText {
						if text := strings.TrimSpace(part.Text); text != "" && !looksLikeMediaPayload(text) {
							segments = append(segments, PromptSegment{
								Role:    role,
								Content: text,
								User:    role == RoleUser,
							})
						}
					}
				}
			}
		}

		if rText := strings.TrimSpace(msg.GetReasoningContent()); rText != "" && !looksLikeMediaPayload(rText) {
			segments = append(segments, PromptSegment{
				Role:    RoleAssistant,
				Content: rText,
				User:    false,
			})
		}

		for _, tc := range msg.ParseToolCalls() {
			if tc.Function.Arguments != "" && !looksLikeMediaPayload(tc.Function.Arguments) {
				segments = append(segments, PromptSegment{
					Role:    RoleAssistant,
					Content: tc.Function.Arguments,
					User:    false,
				})
			}
		}
	}

	if req.Prompt != nil {
		switch v := req.Prompt.(type) {
		case string:
			if text := strings.TrimSpace(v); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: text,
					User:    true,
				})
			}
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok {
					if text := strings.TrimSpace(str); text != "" && !looksLikeMediaPayload(text) {
						segments = append(segments, PromptSegment{
							Role:    RoleUser,
							Content: text,
							User:    true,
						})
					}
				}
			}
		}
	}

	if req.Prefix != nil {
		if s, ok := req.Prefix.(string); ok {
			if text := strings.TrimSpace(s); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: text,
					User:    true,
				})
			}
		}
	}

	if req.Suffix != nil {
		if s, ok := req.Suffix.(string); ok {
			if text := strings.TrimSpace(s); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: text,
					User:    true,
				})
			}
		}
	}

	if inst := strings.TrimSpace(req.Instruction); inst != "" && !looksLikeMediaPayload(inst) {
		segments = append(segments, PromptSegment{
			Role:    RoleSystem,
			Content: inst,
			User:    false,
		})
	}

	if req.Input != nil {
		for _, inp := range req.ParseInput() {
			if text := strings.TrimSpace(inp); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: text,
					User:    true,
				})
			}
		}
	}

	return segments
}

func extractClaudeRequest(req *dto.ClaudeRequest) []PromptSegment {
	var segments []PromptSegment

	if req.System != nil {
		if req.IsStringSystem() {
			sys := strings.TrimSpace(req.GetStringSystem())
			if sys != "" && !looksLikeMediaPayload(sys) {
				segments = append(segments, PromptSegment{
					Role:    RoleSystem,
					Content: sys,
					User:    false,
				})
			}
		} else {
			for _, media := range req.ParseSystem() {
				if media.Type == "text" && media.Text != nil {
					sys := strings.TrimSpace(*media.Text)
					if sys != "" && !looksLikeMediaPayload(sys) {
						segments = append(segments, PromptSegment{
							Role:    RoleSystem,
							Content: sys,
							User:    false,
						})
					}
				}
			}
		}
	}

	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = RoleUser
		}

		if msg.Content != nil {
			if msg.IsStringContent() {
				text := strings.TrimSpace(msg.GetStringContent())
				if text != "" && !looksLikeMediaPayload(text) {
					segments = append(segments, PromptSegment{
						Role:    role,
						Content: text,
						User:    role == RoleUser,
					})
				}
			} else {
				medias, err := msg.ParseContent()
				if err == nil {
					for _, media := range medias {
						if (media.Type == "" || media.Type == "text" || media.Type == "input_text") && media.Text != nil {
							text := strings.TrimSpace(*media.Text)
							if text != "" && !looksLikeMediaPayload(text) {
								segments = append(segments, PromptSegment{
									Role:    role,
									Content: text,
									User:    role == RoleUser,
								})
							}
						} else if media.Type == "thinking" && media.Thinking != nil {
							text := strings.TrimSpace(*media.Thinking)
							if text != "" && !looksLikeMediaPayload(text) {
								segments = append(segments, PromptSegment{
									Role:    RoleAssistant,
									Content: text,
									User:    false,
								})
							}
						} else if media.Type == "tool_result" && media.Content != nil {
							for _, t := range extractTextPartsFromRaw(media.Content) {
								text := strings.TrimSpace(t)
								if text != "" && !looksLikeMediaPayload(text) {
									segments = append(segments, PromptSegment{
										Role:    RoleTool,
										Content: text,
										User:    false,
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return segments
}

func extractGeminiChatRequest(req *dto.GeminiChatRequest) []PromptSegment {
	var segments []PromptSegment

	if req.SystemInstructions != nil {
		for _, part := range req.SystemInstructions.Parts {
			text := strings.TrimSpace(part.Text)
			if text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleSystem,
					Content: text,
					User:    false,
				})
			}
		}
	}

	for _, content := range req.Contents {
		role := strings.ToLower(strings.TrimSpace(content.Role))
		if role == "" {
			role = RoleUser
		}
		for _, part := range content.Parts {
			text := strings.TrimSpace(part.Text)
			if text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    role,
					Content: text,
					User:    role == RoleUser,
				})
			}
			if part.CodeExecutionResult != nil && part.CodeExecutionResult.Output != "" {
				if out := strings.TrimSpace(part.CodeExecutionResult.Output); out != "" && !looksLikeMediaPayload(out) {
					segments = append(segments, PromptSegment{
						Role:    RoleTool,
						Content: out,
						User:    false,
					})
				}
			}
			if part.FunctionResponse != nil && len(part.FunctionResponse.Response) > 0 {
				for _, resText := range extractTextPartsFromRaw(part.FunctionResponse.Response) {
					if t := strings.TrimSpace(resText); t != "" && !looksLikeMediaPayload(t) {
						segments = append(segments, PromptSegment{
							Role:    RoleTool,
							Content: t,
							User:    false,
						})
					}
				}
			}
		}
	}

	for _, subReq := range req.Requests {
		subSegments := extractGeminiChatRequest(&subReq)
		segments = append(segments, subSegments...)
	}

	return segments
}

func extractGeminiEmbeddingRequest(req *dto.GeminiEmbeddingRequest) []PromptSegment {
	var segments []PromptSegment
	role := strings.ToLower(strings.TrimSpace(req.Content.Role))
	if role == "" {
		role = RoleUser
	}
	for _, part := range req.Content.Parts {
		text := strings.TrimSpace(part.Text)
		if text != "" && !looksLikeMediaPayload(text) {
			segments = append(segments, PromptSegment{
				Role:    role,
				Content: text,
				User:    true,
			})
		}
	}
	return segments
}

func extractGeminiBatchEmbeddingRequest(req *dto.GeminiBatchEmbeddingRequest) ([]PromptSegment, string) {
	var segments []PromptSegment
	modelName := ""
	for _, sub := range req.Requests {
		if sub == nil {
			continue
		}
		if modelName == "" {
			modelName = sub.Model
		}
		subSegments := extractGeminiEmbeddingRequest(sub)
		segments = append(segments, subSegments...)
	}
	return segments, modelName
}

func extractResponsesRequest(req *dto.OpenAIResponsesRequest) []PromptSegment {
	var segments []PromptSegment

	if len(req.Instructions) > 0 {
		var str string
		if err := common.Unmarshal(req.Instructions, &str); err == nil {
			if text := strings.TrimSpace(str); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleSystem,
					Content: text,
					User:    false,
				})
			}
		} else {
			var raw any
			if err := common.Unmarshal(req.Instructions, &raw); err == nil {
				for _, t := range extractTextPartsFromRaw(raw) {
					if text := strings.TrimSpace(t); text != "" && !looksLikeMediaPayload(text) {
						segments = append(segments, PromptSegment{
							Role:    RoleSystem,
							Content: text,
							User:    false,
						})
					}
				}
			}
		}
	}

	if len(req.Input) > 0 {
		var rawInput any
		if err := common.Unmarshal(req.Input, &rawInput); err == nil {
			inputSegments := extractResponsesInput(rawInput)
			segments = append(segments, inputSegments...)
		}
	}

	return segments
}

func extractResponsesCompactionRequest(req *dto.OpenAIResponsesCompactionRequest) []PromptSegment {
	var segments []PromptSegment

	if len(req.Instructions) > 0 {
		var str string
		if err := common.Unmarshal(req.Instructions, &str); err == nil {
			if text := strings.TrimSpace(str); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleSystem,
					Content: text,
					User:    false,
				})
			}
		} else {
			var raw any
			if err := common.Unmarshal(req.Instructions, &raw); err == nil {
				for _, t := range extractTextPartsFromRaw(raw) {
					if text := strings.TrimSpace(t); text != "" && !looksLikeMediaPayload(text) {
						segments = append(segments, PromptSegment{
							Role:    RoleSystem,
							Content: text,
							User:    false,
						})
					}
				}
			}
		}
	}

	if len(req.Input) > 0 {
		var rawInput any
		if err := common.Unmarshal(req.Input, &rawInput); err == nil {
			inputSegments := extractResponsesInput(rawInput)
			segments = append(segments, inputSegments...)
		}
	}

	return segments
}

func extractResponsesInput(value any) []PromptSegment {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text != "" && !looksLikeMediaPayload(text) {
			return []PromptSegment{{Role: RoleUser, Content: text, User: true}}
		}
		return nil

	case []any:
		var result []PromptSegment
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				text := strings.TrimSpace(entry)
				if text != "" && !looksLikeMediaPayload(text) {
					result = append(result, PromptSegment{Role: RoleUser, Content: text, User: true})
				}
			case map[string]any:
				role := strings.ToLower(strings.TrimSpace(interfaceToString(entry["role"])))
				if role == "" {
					role = RoleUser
				}
				texts := extractTextPartsFromRaw(entry["content"])
				if len(texts) == 0 {
					if t := strings.TrimSpace(interfaceToString(entry["text"])); t != "" {
						texts = append(texts, t)
					}
				}
				for _, text := range texts {
					text = strings.TrimSpace(text)
					if text != "" && !looksLikeMediaPayload(text) {
						result = append(result, PromptSegment{
							Role:    role,
							Content: text,
							User:    role == RoleUser,
						})
					}
				}
			}
		}
		return result

	case map[string]any:
		role := strings.ToLower(strings.TrimSpace(interfaceToString(typed["role"])))
		if role == "" {
			role = RoleUser
		}
		texts := extractTextPartsFromRaw(typed["content"])
		if len(texts) == 0 {
			if t := strings.TrimSpace(interfaceToString(typed["text"])); t != "" {
				texts = append(texts, t)
			}
		}
		var result []PromptSegment
		for _, text := range texts {
			text = strings.TrimSpace(text)
			if text != "" && !looksLikeMediaPayload(text) {
				result = append(result, PromptSegment{
					Role:    role,
					Content: text,
					User:    role == RoleUser,
				})
			}
		}
		return result

	default:
		return nil
	}
}

func extractAlphaSearchRequest(req *dto.AlphaSearchRequest) []PromptSegment {
	if len(req.RawBody) == 0 {
		return nil
	}
	var raw map[string]any
	if err := common.Unmarshal(req.RawBody, &raw); err != nil {
		return nil
	}
	if q, ok := raw["query"].(string); ok {
		text := strings.TrimSpace(q)
		if text != "" && !looksLikeMediaPayload(text) {
			return []PromptSegment{{
				Role:    RoleUser,
				Content: text,
				User:    true,
			}}
		}
	}
	return nil
}

func extractImageRequest(req *dto.ImageRequest) []PromptSegment {
	var segments []PromptSegment
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" && !looksLikeMediaPayload(prompt) {
		segments = append(segments, PromptSegment{
			Role:    RoleUser,
			Content: prompt,
			User:    true,
		})
	}

	if len(req.Extra) > 0 {
		if negRaw, ok := req.Extra["negative_prompt"]; ok {
			var negStr string
			if err := common.Unmarshal(negRaw, &negStr); err == nil {
				if negText := strings.TrimSpace(negStr); negText != "" && !looksLikeMediaPayload(negText) {
					segments = append(segments, PromptSegment{
						Role:    RoleUser,
						Content: negText,
						User:    true,
					})
				}
			}
		}
	}

	return segments
}

func extractEmbeddingRequest(req *dto.EmbeddingRequest) []PromptSegment {
	var segments []PromptSegment
	for _, text := range req.ParseInput() {
		trimmed := strings.TrimSpace(text)
		if trimmed != "" && !looksLikeMediaPayload(trimmed) {
			segments = append(segments, PromptSegment{
				Role:    RoleUser,
				Content: trimmed,
				User:    true,
			})
		}
	}
	return segments
}

func extractRerankRequest(req *dto.RerankRequest) []PromptSegment {
	var segments []PromptSegment
	if query := strings.TrimSpace(req.Query); query != "" && !looksLikeMediaPayload(query) {
		segments = append(segments, PromptSegment{
			Role:    RoleUser,
			Content: query,
			User:    true,
		})
	}

	for _, doc := range req.Documents {
		switch v := doc.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed != "" && !looksLikeMediaPayload(trimmed) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: trimmed,
					User:    true,
				})
			}
		case map[string]any:
			if text, ok := v["text"].(string); ok {
				trimmed := strings.TrimSpace(text)
				if trimmed != "" && !looksLikeMediaPayload(trimmed) {
					segments = append(segments, PromptSegment{
						Role:    RoleUser,
						Content: trimmed,
						User:    true,
					})
				}
			}
		}
	}
	return segments
}

func extractAudioRequest(req *dto.AudioRequest, c *gin.Context) []PromptSegment {
	var segments []PromptSegment

	if input := strings.TrimSpace(req.Input); input != "" && !looksLikeMediaPayload(input) {
		segments = append(segments, PromptSegment{
			Role:    RoleUser,
			Content: input,
			User:    true,
		})
	}

	if instructions := strings.TrimSpace(req.Instructions); instructions != "" && !looksLikeMediaPayload(instructions) {
		segments = append(segments, PromptSegment{
			Role:    RoleSystem,
			Content: instructions,
			User:    false,
		})
	}

	if len(req.RefText) > 0 {
		var refTextStr string
		if err := common.Unmarshal(req.RefText, &refTextStr); err == nil {
			if text := strings.TrimSpace(refTextStr); text != "" && !looksLikeMediaPayload(text) {
				segments = append(segments, PromptSegment{
					Role:    RoleUser,
					Content: text,
					User:    true,
				})
			}
		}
	}

	if c != nil && c.Request != nil {
		var formPrompt string
		if c.Request.MultipartForm != nil && len(c.Request.MultipartForm.Value["prompt"]) > 0 {
			formPrompt = c.Request.MultipartForm.Value["prompt"][0]
		} else if c.Request.PostForm != nil && len(c.Request.PostForm["prompt"]) > 0 {
			formPrompt = c.Request.PostForm["prompt"][0]
		} else if c.Request.Form != nil && len(c.Request.Form["prompt"]) > 0 {
			formPrompt = c.Request.Form["prompt"][0]
		}
		if prompt := strings.TrimSpace(formPrompt); prompt != "" && !looksLikeMediaPayload(prompt) {
			segments = append(segments, PromptSegment{
				Role:    RoleUser,
				Content: prompt,
				User:    true,
			})
		}
	}

	return segments
}

func extractTextPartsFromRaw(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var result []string
		for _, part := range typed {
			if obj, ok := part.(map[string]any); ok {
				typeName := strings.ToLower(interfaceToString(obj["type"]))
				if typeName == "" || typeName == "text" || typeName == "input_text" || typeName == "output_text" {
					if text := interfaceToString(obj["text"]); text != "" {
						result = append(result, text)
					}
				}
			} else if str, ok := part.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case map[string]any:
		if text := interfaceToString(typed["text"]); text != "" {
			return []string{text}
		}
	}
	return nil
}

func looksLikeMediaPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)

	// 纯独立 URL（不含空格/换行）
	if (strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) && !strings.ContainsAny(trimmed, " \t\r\n") {
		return true
	}

	// Data URL（无空格独立 Data URL 或标准 base64 data URL）
	if strings.HasPrefix(lower, "data:") {
		if !strings.ContainsAny(trimmed, " \t\r\n") || strings.Contains(lower, ";base64,") {
			return true
		}
	}

	// 排除长 Base64 字符串（>= 256 字符且仅含 Base64 字符集）
	if len(trimmed) >= 256 {
		isBase64 := true
		for _, r := range trimmed {
			alphaNumeric := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !alphaNumeric && r != '+' && r != '/' && r != '=' && r != '\n' && r != '\r' {
				isBase64 = false
				break
			}
		}
		if isBase64 {
			return true
		}
	}

	return false
}

func interfaceToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
