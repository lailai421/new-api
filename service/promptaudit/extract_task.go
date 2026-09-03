package promptaudit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// TaskAuditMeta 封装从请求级 pinned LoadedPlugin.Meta 传递给审计门禁的契约信息，
// 避免让 service/promptaudit 反向依赖 pkg/jsplugin。
type TaskAuditMeta struct {
	PluginKey           string
	AuditTextPaths      []string
	HasSubmitCapability bool
	Found               bool
}

// EvaluateJSONPointer 根据 RFC 6901 受限子集，从规范化对象或切片中解析指定路径的值。
func EvaluateJSONPointer(target any, pointer string) (any, bool, error) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, false, fmt.Errorf("JSON pointer %q must start with '/'", pointer)
	}
	tokens := strings.Split(pointer, "/")[1:]
	current := target
	for _, rawToken := range tokens {
		if current == nil {
			return nil, false, nil
		}
		// RFC 6901 转义：先还原 ~1 为 /，再还原 ~0 为 ~
		token := strings.ReplaceAll(strings.ReplaceAll(rawToken, "~1", "/"), "~0", "~")
		switch val := current.(type) {
		case map[string]any:
			next, exists := val[token]
			if !exists {
				return nil, false, nil
			}
			current = next
		case []any:
			if token == "" || (len(token) > 1 && token[0] == '0') {
				return nil, false, fmt.Errorf("invalid array index %q in JSON pointer", token)
			}
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx > 1000 {
				return nil, false, fmt.Errorf("invalid array index %q in JSON pointer", token)
			}
			if idx >= len(val) {
				return nil, false, nil
			}
			current = val[idx]
		default:
			return nil, false, fmt.Errorf("cannot index non-container node %T with token %q", current, token)
		}
	}
	return current, true, nil
}

// extractTextFromValue 从 JSON 指针解析的目标值中提取纯文本。
// 严格遵守白名单：仅接受非空字符串、字符串切片或合法文本内容块；
// 遇到非文本对象、包含二进制/URL/文件容器的对象或基础类型，返回明确类型错误。
func extractTextFromValue(val any) ([]string, error) {
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || looksLikeMediaPayload(trimmed) {
			return nil, nil
		}
		return []string{trimmed}, nil
	case []string:
		var results []string
		for _, s := range v {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" && !looksLikeMediaPayload(trimmed) {
				results = append(results, trimmed)
			}
		}
		return results, nil
	case []any:
		var results []string
		for _, item := range v {
			texts, err := extractTextFromValue(item)
			if err != nil {
				return nil, err
			}
			results = append(results, texts...)
		}
		return results, nil
	case map[string]any:
		// 严禁包含二进制、URL、文件引用容器
		for _, badKey := range []string{"image_url", "url", "data", "bytesBase64Encoded", "fileRef", "__fileRef", "binary_data_base64"} {
			if _, forbidden := v[badKey]; forbidden {
				return nil, fmt.Errorf("node contains forbidden media field %q", badKey)
			}
		}
		if typeVal, hasType := v["type"]; hasType {
			typeStr, _ := typeVal.(string)
			if typeStr == "text" || typeStr == "input_text" {
				if textVal, hasText := v["text"]; hasText {
					if textStr, ok := textVal.(string); ok {
						trimmed := strings.TrimSpace(textStr)
						if trimmed != "" && !looksLikeMediaPayload(trimmed) {
							return []string{trimmed}, nil
						}
						return nil, nil
					}
				}
			}
			return nil, fmt.Errorf("unknown or invalid content block type %q", typeStr)
		}
		if textVal, hasText := v["text"]; hasText && len(v) == 1 {
			if textStr, ok := textVal.(string); ok {
				trimmed := strings.TrimSpace(textStr)
				if trimmed != "" && !looksLikeMediaPayload(trimmed) {
					return []string{trimmed}, nil
				}
				return nil, nil
			}
		}
		return nil, fmt.Errorf("node object is not a valid text content block")
	default:
		return nil, fmt.Errorf("unsupported value type %T in prompt audit path", val)
	}
}

// ExtractTaskRequest 从规范化的 taskRequestBody 中按 auditTextPaths 提取提示词片段。
// 当具备提交能力的插件未声明 auditTextPaths 时，返回 ErrUnsupportedProtocol 实现失败关闭。
// 当全部声明路径未命中或内容为空时，返回 ErrNoPrompt。
func ExtractTaskRequest(taskRequestBody any, auditMeta TaskAuditMeta) ([]PromptSegment, error) {
	if auditMeta.HasSubmitCapability && len(auditMeta.AuditTextPaths) == 0 {
		return nil, ErrUnsupportedProtocol
	}
	if taskRequestBody == nil {
		return nil, ErrNoPrompt
	}
	var segments []PromptSegment
	for _, path := range auditMeta.AuditTextPaths {
		val, found, err := EvaluateJSONPointer(taskRequestBody, path)
		if err != nil {
			return nil, ErrUnsupportedProtocol
		}
		if !found || val == nil {
			continue
		}
		texts, err := extractTextFromValue(val)
		if err != nil {
			return nil, ErrUnsupportedProtocol
		}
		for _, t := range texts {
			segments = append(segments, PromptSegment{
				Role:    RoleUser,
				Content: t,
				User:    true,
			})
		}
	}
	if len(segments) == 0 {
		return nil, ErrNoPrompt
	}
	return segments, nil
}

// ExtractTaskPluginResponsesRequest 从 Responses Bridge 提取提示词片段。
// 执行双源合并与稳定去重：
// 源 A：从 Gin 上下文中保存的协议原始输入提取标准 Responses 字段（instructions, input）；
// 源 B：从规范化的 taskRequestBody 按照 auditTextPaths 提取插件 decode 补充的文本。
func ExtractTaskPluginResponsesRequest(c *gin.Context, taskRequestBody any, auditMeta TaskAuditMeta) ([]PromptSegment, error) {
	var segmentsA []PromptSegment
	if protoVal, exists := c.Get("protocol_request"); exists && protoVal != nil {
		// protocol_request 是 pluginruntime.ProtocolRequestContext
		// 通过 JSON 序列化再反序列化为 dto.OpenAIResponsesRequest
		if bytes, err := common.Marshal(protoVal); err == nil {
			var wrapper struct {
				RequestBody map[string]any `json:"requestBody"`
				Body        map[string]any `json:"body"`
			}
			if err := common.Unmarshal(bytes, &wrapper); err == nil {
				targetBody := wrapper.RequestBody
				if len(targetBody) == 0 && wrapper.Body != nil {
					if valMap, ok := wrapper.Body["value"].(map[string]any); ok {
						targetBody = valMap
					}
				}
				if len(targetBody) > 0 {
					if bodyBytes, bErr := common.Marshal(targetBody); bErr == nil {
						var req relaydto.OpenAIResponsesRequest
						if uErr := common.Unmarshal(bodyBytes, &req); uErr == nil {
							segmentsA = extractResponsesRequest(&req)
						}
					}
				}
			}
		}
	}

	var segmentsB []PromptSegment
	if len(auditMeta.AuditTextPaths) > 0 && taskRequestBody != nil {
		for _, path := range auditMeta.AuditTextPaths {
			val, found, err := EvaluateJSONPointer(taskRequestBody, path)
			if err != nil {
				return nil, ErrUnsupportedProtocol
			}
			if !found || val == nil {
				continue
			}
			texts, err := extractTextFromValue(val)
			if err != nil {
				return nil, ErrUnsupportedProtocol
			}
			for _, t := range texts {
				segmentsB = append(segmentsB, PromptSegment{
					Role:    RoleUser,
					Content: t,
					User:    true,
				})
			}
		}
	} else if auditMeta.HasSubmitCapability && len(auditMeta.AuditTextPaths) == 0 {
		return nil, ErrUnsupportedProtocol
	}

	// 稳定去重合并：源 A 优先，源 B 随后追加，内容相同的片段保留首个
	var merged []PromptSegment
	seen := make(map[string]struct{})
	for _, seg := range segmentsA {
		key := seg.Role + ":" + seg.Content
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			merged = append(merged, seg)
		}
	}
	for _, seg := range segmentsB {
		key := seg.Role + ":" + seg.Content
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			merged = append(merged, seg)
		}
	}

	if len(merged) == 0 {
		return nil, ErrNoPrompt
	}
	return merged, nil
}
