package promptaudit

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Realtime 客户端支持与已知事件类型常量
const (
	RealtimeEventClientSessionUpdate        = "session.update"
	RealtimeEventClientConversationCreate   = "conversation.item.create"
	RealtimeEventClientConversationTruncate = "conversation.item.truncate"
	RealtimeEventClientConversationDelete   = "conversation.item.delete"
	RealtimeEventClientInputAudioAppend     = "input_audio_buffer.append"
	RealtimeEventClientInputAudioCommit     = "input_audio_buffer.commit"
	RealtimeEventClientInputAudioClear      = "input_audio_buffer.clear"
	RealtimeEventClientResponseCreate       = "response.create"
	RealtimeEventClientResponseCancel       = "response.cancel"
)

// realtimeBaseEvent 用于快速解析事件类型和基础元数据
type realtimeBaseEvent struct {
	EventID string `json:"event_id"`
	Type    string `json:"type"`
}

// realtimeSessionUpdateEvent 专门用于解析 session.update 事件
type realtimeSessionUpdateEvent struct {
	Session *realtimeSessionData `json:"session"`
}

type realtimeSessionData struct {
	Instructions string         `json:"instructions"`
	Tools        []realtimeTool `json:"tools"`
}

type realtimeTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// realtimeConversationItemEvent 专门用于解析 conversation.item.create 事件
type realtimeConversationItemEvent struct {
	PreviousItemID string        `json:"previous_item_id"`
	Item           *realtimeItem `json:"item"`
}

type realtimeItem struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Content []realtimeContent `json:"content"`
	CallID  string            `json:"call_id"`
	Output  string            `json:"output"` // 用于 function_call_output
}

type realtimeContent struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	Audio      string `json:"audio"` // Base64 二进制音频，审计时忽略
	Transcript string `json:"transcript"`
}

// realtimeResponseCreateEvent 专门用于解析 response.create 事件
type realtimeResponseCreateEvent struct {
	Response *realtimeResponseData `json:"response"`
}

type realtimeResponseData struct {
	Instructions string         `json:"instructions"`
	Tools        []realtimeTool `json:"tools"`
	Input        []realtimeItem `json:"input"`
}

// ExtractRealtimeEvent 从客户端原始 WebSocket 文本消息中提取需要审计的文本分段。
// 仅按已知协议事件与字段确定性提取，绝不递归收集任意 JSON 字符串，排除 Base64、控制参数及字段名。
// 已知无文本帧返回 ErrNoPrompt；未知事件、非法 JSON 或契约不符返回 ErrUnsupportedProtocol。
func ExtractRealtimeEvent(rawMessage []byte) (segments []PromptSegment, eventType string, err error) {
	if len(rawMessage) == 0 {
		return nil, "", ErrUnsupportedProtocol
	}

	var base realtimeBaseEvent
	if err := common.Unmarshal(rawMessage, &base); err != nil {
		return nil, "", ErrUnsupportedProtocol
	}

	eventType = strings.TrimSpace(base.Type)
	if eventType == "" {
		return nil, "", ErrUnsupportedProtocol
	}

	switch eventType {
	case RealtimeEventClientInputAudioAppend,
		RealtimeEventClientInputAudioCommit,
		RealtimeEventClientInputAudioClear,
		RealtimeEventClientConversationTruncate,
		RealtimeEventClientConversationDelete,
		RealtimeEventClientResponseCancel:
		// 已知纯控制或纯二进制音频帧，无需文本审计
		return nil, eventType, ErrNoPrompt

	case RealtimeEventClientSessionUpdate:
		var ev realtimeSessionUpdateEvent
		if err := common.Unmarshal(rawMessage, &ev); err != nil {
			return nil, eventType, ErrUnsupportedProtocol
		}
		if ev.Session == nil {
			return nil, eventType, ErrNoPrompt
		}
		segments = extractSessionData(ev.Session)

	case RealtimeEventClientConversationCreate:
		var ev realtimeConversationItemEvent
		if err := common.Unmarshal(rawMessage, &ev); err != nil {
			return nil, eventType, ErrUnsupportedProtocol
		}
		if ev.Item == nil {
			return nil, eventType, ErrNoPrompt
		}
		segments = extractItemData(ev.Item)

	case RealtimeEventClientResponseCreate:
		var ev realtimeResponseCreateEvent
		if err := common.Unmarshal(rawMessage, &ev); err != nil {
			return nil, eventType, ErrUnsupportedProtocol
		}
		if ev.Response == nil {
			return nil, eventType, ErrNoPrompt
		}
		segments = extractResponseData(ev.Response)

	default:
		// 未知或未经支持的事件类型，存在携带未知文本绕过的风险，失败关闭
		return nil, eventType, ErrUnsupportedProtocol
	}

	normalized := NormalizeSegments(segments)
	if len(normalized) == 0 {
		return nil, eventType, ErrNoPrompt
	}

	return normalized, eventType, nil
}

// extractSessionData 从 Session 数据中提取 instructions 和 tools 描述/Schema 文本
func extractSessionData(s *realtimeSessionData) []PromptSegment {
	if s == nil {
		return nil
	}
	var segments []PromptSegment
	if inst := strings.TrimSpace(s.Instructions); inst != "" {
		segments = append(segments, PromptSegment{
			Role:    RoleSystem,
			Content: inst,
			User:    false,
		})
	}
	for _, tool := range s.Tools {
		toolSegs := extractToolSegments(tool)
		segments = append(segments, toolSegs...)
	}
	return segments
}

// extractItemData 从 Item 数据中提取文本
func extractItemData(item *realtimeItem) []PromptSegment {
	if item == nil {
		return nil
	}
	var segments []PromptSegment
	role := strings.ToLower(strings.TrimSpace(item.Role))
	if role == "" {
		role = RoleUser
	}
	isUser := (role == RoleUser)

	// 处理 function_call_output 类型的工具响应文本
	if strings.EqualFold(item.Type, "function_call_output") {
		if out := strings.TrimSpace(item.Output); out != "" {
			segments = append(segments, PromptSegment{
				Role:    RoleTool,
				Content: out,
				User:    false,
			})
		}
		return segments
	}

	for _, c := range item.Content {
		cType := strings.ToLower(strings.TrimSpace(c.Type))
		switch cType {
		case "input_text", "text":
			if txt := strings.TrimSpace(c.Text); txt != "" {
				segments = append(segments, PromptSegment{
					Role:    role,
					Content: txt,
					User:    isUser,
				})
			}
		case "input_audio":
			// 音频二进制 Base64 (c.Audio) 明确排除，但转写文本 (transcript) 必须被审计
			if trans := strings.TrimSpace(c.Transcript); trans != "" {
				segments = append(segments, PromptSegment{
					Role:    role,
					Content: trans,
					User:    isUser,
				})
			}
		}
	}
	return segments
}

// extractResponseData 从 Response 数据中提取 instructions、tools 与输入 items
func extractResponseData(resp *realtimeResponseData) []PromptSegment {
	if resp == nil {
		return nil
	}
	var segments []PromptSegment
	if inst := strings.TrimSpace(resp.Instructions); inst != "" {
		segments = append(segments, PromptSegment{
			Role:    RoleSystem,
			Content: inst,
			User:    false,
		})
	}
	for _, tool := range resp.Tools {
		segments = append(segments, extractToolSegments(tool)...)
	}
	for _, item := range resp.Input {
		itemCopy := item
		segments = append(segments, extractItemData(&itemCopy)...)
	}
	return segments
}

// extractToolSegments 提取工具描述及参数 Schema 中有业务文本含义的 description 属性
func extractToolSegments(tool realtimeTool) []PromptSegment {
	var segments []PromptSegment
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		segments = append(segments, PromptSegment{
			Role:    RoleSystem,
			Content: desc,
			User:    false,
		})
	}
	if len(tool.Parameters) > 0 {
		schemaTexts := extractSchemaDescriptions(tool.Parameters)
		for _, txt := range schemaTexts {
			segments = append(segments, PromptSegment{
				Role:    RoleSystem,
				Content: txt,
				User:    false,
			})
		}
	}
	return segments
}

// extractSchemaDescriptions 递归提取 JSON Schema 中的 description 字符串
// 严格限制只提取键名为 "description" 的文本，不扫描字段名称、类型 (type) 或元数据
func extractSchemaDescriptions(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	var result []string

	if descVal, ok := schema["description"]; ok {
		if descStr, isStr := descVal.(string); isStr {
			if trimmed := strings.TrimSpace(descStr); trimmed != "" {
				result = append(result, trimmed)
			}
		}
	}

	// 检查 properties
	if propsVal, ok := schema["properties"]; ok {
		if propsMap, isMap := propsVal.(map[string]any); isMap {
			for _, subProp := range propsMap {
				if subMap, isSubMap := subProp.(map[string]any); isSubMap {
					subDescs := extractSchemaDescriptions(subMap)
					result = append(result, subDescs...)
				}
			}
		}
	}

	// 检查 items（数组类型子项）
	if itemsVal, ok := schema["items"]; ok {
		if itemsMap, isItemsMap := itemsVal.(map[string]any); isItemsMap {
			subDescs := extractSchemaDescriptions(itemsMap)
			result = append(result, subDescs...)
		}
	}

	return result
}
