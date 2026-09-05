package promptaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

var (
	// ErrNoPrompt 表示请求协议已知且结构合法，但确实不包含需要审计的文本内容。
	ErrNoPrompt = errors.New("prompt audit: no prompt content found")

	// ErrUnsupportedProtocol 表示未知协议格式或无法建立确定性提取契约，审计开启时必须失败关闭。
	ErrUnsupportedProtocol = errors.New("prompt audit: unsupported protocol or payload format")

	bearerPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+\-/]+=*`)
	apiKeyPattern = regexp.MustCompile(`(?i)\b(sk|rk|pk|api[_-]?key|token|secret|password)[-_:=\s]+[A-Za-z0-9._~+\-/]{8,}`)
	canaryPattern = regexp.MustCompile(`(?i)([A-Z]+_CANARY_)[A-Za-z0-9_-]+`)
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phonePattern  = regexp.MustCompile(`(?:\+?\d[\d\s().-]{8,}\d)`)
)

const (
	RoleSystem    = "system"
	RoleDeveloper = "developer"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"

	MaxRedactedPreviewRunes = 96

	AgentsMdHeader              = "# AGENTS.md instructions"
	InstructionsOpenTag         = "<INSTRUCTIONS>"
	InstructionsCloseTag        = "</INSTRUCTIONS>"
	AgentsMdReplaceNotice       = "These AGENTS.md instructions replace"
	AgentsMdNoLongerApplyNotice = "The previously provided AGENTS.md instructions no longer apply."
)

// PromptSegment 描述从客户端输入中提取的一段结构化文本及其所属角色。
type PromptSegment struct {
	Role    string
	Content string
	User    bool
}

// IsUser 检查该分段是否属于用户角色输入。
func (s PromptSegment) IsUser() bool {
	if s.User {
		return true
	}
	role := strings.ToLower(strings.TrimSpace(s.Role))
	return role == RoleUser
}

// IsAssistant 检查该分段是否属于模型/助手输出历史。
func (s PromptSegment) IsAssistant() bool {
	role := strings.ToLower(strings.TrimSpace(s.Role))
	return role == RoleAssistant || role == "model"
}

// NormalizeSegments 清除空白段落并去除内容首尾空白字符。
func NormalizeSegments(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		trimmed := strings.TrimSpace(seg.Content)
		if trimmed != "" {
			seg.Content = trimmed
			result = append(result, seg)
		}
	}
	return result
}

// IsAgentsMdEnvelope 判定给定的文本是否为完整的 Codex AGENTS.md 信封或通知。
// 识别契约与 Codex 保持一致：文本以 # AGENTS.md instructions 起头，且同时包含 <INSTRUCTIONS> 与 </INSTRUCTIONS>，
// 或者属于替换/移除通知变体。
func IsAgentsMdEnvelope(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, AgentsMdHeader) {
		return false
	}
	openIdx := strings.Index(trimmed, InstructionsOpenTag)
	closeIdx := strings.Index(trimmed, InstructionsCloseTag)
	if openIdx != -1 && closeIdx != -1 && openIdx < closeIdx {
		return true
	}
	if strings.Contains(trimmed, AgentsMdReplaceNotice) {
		return true
	}
	if strings.Contains(trimmed, AgentsMdNoLongerApplyNotice) {
		return true
	}
	return false
}

// stripAgentsMdEnvelopeFromText 从单段文本中剥离 AGENTS.md 信封或通知。
// 整段是信封返回空字符串；混合段切除信封闭区间保留剩余真实文本。
func stripAgentsMdEnvelopeFromText(text string) string {
	for {
		headerIdx := strings.Index(text, AgentsMdHeader)
		if headerIdx == -1 {
			break
		}

		sub := text[headerIdx:]
		openIdx := strings.Index(sub, InstructionsOpenTag)
		closeIdx := strings.Index(sub, InstructionsCloseTag)

		// 完整的 <INSTRUCTIONS> ... </INSTRUCTIONS> 信封
		if openIdx != -1 && closeIdx != -1 && openIdx < closeIdx {
			endIdx := headerIdx + closeIdx + len(InstructionsCloseTag)
			text = text[:headerIdx] + text[endIdx:]
			continue
		}

		// 替换通知
		replaceIdx := strings.Index(sub, AgentsMdReplaceNotice)
		if replaceIdx != -1 {
			endNotice := replaceIdx + len(AgentsMdReplaceNotice)
			if newlineIdx := strings.Index(sub[endNotice:], "\n"); newlineIdx != -1 {
				endIdx := headerIdx + endNotice + newlineIdx
				text = text[:headerIdx] + text[endIdx:]
			} else {
				text = text[:headerIdx]
			}
			continue
		}

		// 移除通知
		removeIdx := strings.Index(sub, AgentsMdNoLongerApplyNotice)
		if removeIdx != -1 {
			endNotice := removeIdx + len(AgentsMdNoLongerApplyNotice)
			if newlineIdx := strings.Index(sub[endNotice:], "\n"); newlineIdx != -1 {
				endIdx := headerIdx + endNotice + newlineIdx
				text = text[:headerIdx] + text[endIdx:]
			} else {
				text = text[:headerIdx]
			}
			continue
		}

		// 虽包含 header 但不符合信封或通知契约（如用户普通讨论），避免死循环跳出
		break
	}
	return text
}

// StripAgentsMdEnvelopes 剥离 Codex 注入的 AGENTS.md 信封。
// 非 user 分段原样保留；整段为信封的 user 分段丢弃；混合段切除信封区间保留真实文本。
func StripAgentsMdEnvelopes(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		if !seg.IsUser() {
			result = append(result, seg)
			continue
		}

		stripped := stripAgentsMdEnvelopeFromText(seg.Content)
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" {
			continue
		}

		seg.Content = trimmed
		result = append(result, seg)
	}
	return result
}

const (
	CodexTitlePrefix   = "Generate a concise, single-line task title of at most "
	CodexCatchUpPrefix = "Write a brief catch-up for a user returning to this Codex task."

	// Codex 本地压缩（非 OpenAI 托管模型）会把该模板作为独立 user 消息追加到 /v1/responses。
	CodexCompactionPrefix        = "You are performing a CONTEXT CHECKPOINT COMPACTION."
	CodexCompactionHandoffMarker = "Create a handoff summary for another LLM that will resume the task."
	// 较新的中断压缩路径：Interrupted. + You are now performing...
	CodexCompactionNowMarker           = "You are now performing a CONTEXT CHECKPOINT COMPACTION."
	CodexCompactionToolsDisabledMarker = "Tools access is disabled for the duration of the compaction."
)

type pairedTag struct {
	open  string
	close string
}

var codexContextualTags = []pairedTag{
	{open: "<environment_context>", close: "</environment_context>"},
	{open: "<skills_instructions>", close: "</skills_instructions>"},
	{open: "<plugins_instructions>", close: "</plugins_instructions>"},
	{open: "<user_shell_command>", close: "</user_shell_command>"},
	{open: "<turn_aborted>", close: "</turn_aborted>"},
}

// stripPairedTagsFromText 剥离 Codex 注入的自动上下文成对标记块（ASCII 大小写不敏感）。
// 若整段在 trim 后以开标签起头并以对应闭标签结尾，则整段丢弃（返回空字符串）；
// 否则从文本中剔除所有完整的 open...close 闭区间（含标签自身），再 trim。
func stripPairedTagsFromText(text string) string {
	trimmed := strings.TrimSpace(text)
	lowerTrimmed := strings.ToLower(trimmed)

	// 先检查整段是否恰好为一个成对标记块
	for _, tag := range codexContextualTags {
		if strings.HasPrefix(lowerTrimmed, tag.open) && strings.HasSuffix(lowerTrimmed, tag.close) {
			return ""
		}
	}

	// 循环删除文本中所有成对标签闭区间
	for _, tag := range codexContextualTags {
		openLen := len(tag.open)
		closeLen := len(tag.close)
		for {
			lower := strings.ToLower(text)
			openIdx := strings.Index(lower, tag.open)
			if openIdx == -1 {
				break
			}
			closeIdx := strings.Index(lower[openIdx+openLen:], tag.close)
			if closeIdx == -1 {
				// 没有匹配的闭标签，不符合完整成对闭区间，退出
				break
			}
			endIdx := openIdx + openLen + closeIdx + closeLen
			text = text[:openIdx] + text[endIdx:]
		}
	}

	return strings.TrimSpace(text)
}

// StripCodexContextualUser 剥离 Codex 注入的成对标记块（环境上下文、技能、插件、用户 shell 命令、中止标记等）。
// 非 user 分段原样保留；整段为标记块的丢弃；混合段切除标记块保留真实正文。
func StripCodexContextualUser(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		if !seg.IsUser() {
			result = append(result, seg)
			continue
		}

		stripped := stripPairedTagsFromText(seg.Content)
		if stripped == "" {
			continue
		}

		seg.Content = stripped
		result = append(result, seg)
	}
	return result
}

var codexTitleUserPromptPattern = regexp.MustCompile(`(?m)^User prompt:[ \t]*\r?\n?`)

// unwrapCodexTitleFromText 从 Codex 标题生成模板中提取真实用户提示词正文。
// 契约：trim 后以 "Generate a concise, single-line task title of at most " 起头，且含独立行 "User prompt:"。
// 仅保留 "User prompt:" 之后的真实正文；若正文为空则整段丢弃（返回空字符串）。
// 用户讨论起标题但没有完整模板契约时，保持原样。
func unwrapCodexTitleFromText(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, CodexTitlePrefix) {
		return text
	}

	loc := codexTitleUserPromptPattern.FindStringIndex(trimmed)
	if loc == nil {
		return text
	}

	userPrompt := strings.TrimSpace(trimmed[loc[1]:])
	return userPrompt
}

// UnwrapCodexThreadTitle 展开并抽取所有 user 分段中的 Codex 标题生成模板正文。
func UnwrapCodexThreadTitle(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		if !seg.IsUser() {
			result = append(result, seg)
			continue
		}

		unwrapped := unwrapCodexTitleFromText(seg.Content)
		trimmed := strings.TrimSpace(unwrapped)
		if trimmed == "" {
			continue
		}

		seg.Content = trimmed
		result = append(result, seg)
	}
	return result
}

var codexCatchUpRecentConversationPattern = regexp.MustCompile(`(?m)^Recent conversation:[ \t]*\r?\n?`)

// isCodexCatchUpTemplate 判定是否为 Codex 回看摘要自动请求。
// 契约：trim 后以 CodexCatchUpPrefix 起头，且含独立行 "Recent conversation:"。
// 用户讨论 catch-up 但没有这套完整模板时，不得误伤。
func isCodexCatchUpTemplate(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, CodexCatchUpPrefix) {
		return false
	}
	return codexCatchUpRecentConversationPattern.FindStringIndex(trimmed) != nil
}

// StripCodexCatchUp 丢弃 Codex 回看摘要自动 user 段。
// 匹配模板的整段丢弃（摘要里的 User/Assistant 是会话回放，不是新的用户输入）；
// 非 user 分段与未匹配模板的 user 分段原样保留。
func StripCodexCatchUp(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		if !seg.IsUser() {
			result = append(result, seg)
			continue
		}
		if isCodexCatchUpTemplate(seg.Content) {
			continue
		}
		result = append(result, seg)
	}
	return result
}

// isCodexCompactionTemplate 判定是否为 Codex CLI 上下文压缩自动请求。
// 契约：完整压缩模板，而不是用户讨论 “CONTEXT CHECKPOINT COMPACTION”。
func isCodexCompactionTemplate(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, CodexCompactionPrefix) && strings.Contains(trimmed, CodexCompactionHandoffMarker) {
		return true
	}
	if strings.Contains(trimmed, CodexCompactionNowMarker) && strings.Contains(trimmed, CodexCompactionToolsDisabledMarker) {
		return true
	}
	return false
}

// StripCodexCompaction 丢弃 Codex CLI 上下文压缩自动 user 段。
// 匹配模板的整段丢弃（压缩指令不是用户提问，送审会被误判为 jailbreak）；
// 非 user 分段与未匹配模板的 user 分段原样保留。
func StripCodexCompaction(segments []PromptSegment) []PromptSegment {
	result := make([]PromptSegment, 0, len(segments))
	for _, seg := range segments {
		if !seg.IsUser() {
			result = append(result, seg)
			continue
		}
		if isCodexCompactionTemplate(seg.Content) {
			continue
		}
		result = append(result, seg)
	}
	return result
}

// JoinSegments 将所有段落按稳定顺序拼接为完整原文，不进行截断。
func JoinSegments(segments []PromptSegment) string {
	normalized := NormalizeSegments(segments)
	if len(normalized) == 0 {
		return ""
	}
	texts := make([]string, len(normalized))
	for i, seg := range normalized {
		texts[i] = seg.Content
	}
	return strings.Join(texts, "\n\n")
}

// JoinUserSegments 提取并拼接所有用户角色的段落，保持原顺序，用双换行符分隔。
// 非用户角色（system, developer, assistant, tool 等）不包含在内。若无用户输入返回空字符串。
func JoinUserSegments(segments []PromptSegment) string {
	normalized := NormalizeSegments(segments)
	if len(normalized) == 0 {
		return ""
	}
	var userTexts []string
	for _, seg := range normalized {
		if seg.IsUser() {
			userTexts = append(userTexts, seg.Content)
		}
	}
	if len(userTexts) == 0 {
		return ""
	}
	return strings.Join(userTexts, "\n\n")
}

// SelectUserScanSegments 构造送审文本分段切片。
// 仅包含用户角色分段，排除 system/assistant/tool/人设等非用户内容。
// 找不到任何用户输入时返回 nil，绝不退回全文。
// 当 latestTurnOnly 为 false 时：返回该请求全部用户分段，将最后一段用户输入置于切片首位作为优先扫描段，其余按原顺序排列；
// 当 latestTurnOnly 为 true 时：仅提取最新一轮连续用户分段（合并为单个元素），不附带 assistant 输出历史。
func SelectUserScanSegments(segments []PromptSegment, latestTurnOnly bool) []string {
	normalized := NormalizeSegments(segments)
	if len(normalized) == 0 {
		return nil
	}

	if latestTurnOnly {
		latestUserEnd := -1
		for i := len(normalized) - 1; i >= 0; i-- {
			if normalized[i].IsUser() {
				latestUserEnd = i
				break
			}
		}
		if latestUserEnd < 0 {
			return nil
		}

		latestUserStart := latestUserEnd
		for latestUserStart > 0 && normalized[latestUserStart-1].IsUser() {
			latestUserStart--
		}

		currentUserTexts := make([]string, 0, latestUserEnd-latestUserStart+1)
		for _, s := range normalized[latestUserStart : latestUserEnd+1] {
			currentUserTexts = append(currentUserTexts, s.Content)
		}
		return []string{strings.Join(currentUserTexts, "\n\n")}
	}

	// latestTurnOnly == false：全部用户分段，最后一段置于首位
	var userIndices []int
	for i, seg := range normalized {
		if seg.IsUser() {
			userIndices = append(userIndices, i)
		}
	}
	if len(userIndices) == 0 {
		return nil
	}
	if len(userIndices) == 1 {
		return []string{normalized[userIndices[0]].Content}
	}

	lastUserIdx := userIndices[len(userIndices)-1]
	result := make([]string, 0, len(userIndices))
	result = append(result, normalized[lastUserIdx].Content)
	for _, idx := range userIndices[:len(userIndices)-1] {
		result = append(result, normalized[idx].Content)
	}
	return result
}

// SelectLatestTurnSegments 提取最新用户轮次分段。为兼容保留，若未找到用户输入返回 nil，不再退回全部分段。
func SelectLatestTurnSegments(segments []PromptSegment) []string {
	return SelectUserScanSegments(segments, true)
}

// AllSegmentTextsPrioritized 将最新用户段落置于首位，其余段落按原顺序排布，用于默认全局审计。
func AllSegmentTextsPrioritized(normalized []PromptSegment) []string {
	if len(normalized) == 0 {
		return nil
	}

	priorityIndex := -1
	for i := len(normalized) - 1; i >= 0; i-- {
		if normalized[i].IsUser() {
			priorityIndex = i
			break
		}
	}

	if priorityIndex < 0 {
		result := make([]string, len(normalized))
		for i, s := range normalized {
			result[i] = s.Content
		}
		return result
	}

	result := make([]string, 0, len(normalized))
	result = append(result, normalized[priorityIndex].Content)
	for i, s := range normalized {
		if i != priorityIndex {
			result = append(result, s.Content)
		}
	}
	return result
}

// BuildScanText 使用 PrioritySeparator 组合分段，使 Evaluator 能识别首要扫描优先级。
func BuildScanText(segmentTexts []string) string {
	if len(segmentTexts) == 0 {
		return ""
	}
	if len(segmentTexts) == 1 {
		return segmentTexts[0]
	}
	return segmentTexts[0] + PrioritySeparator + strings.Join(segmentTexts[1:], "\n\n")
}

// CalculatePromptHash 计算完整原文的 SHA-256 哈希。
func CalculatePromptHash(fullPrompt string) string {
	sum := sha256.Sum256([]byte(fullPrompt))
	return hex.EncodeToString(sum[:])
}

// BuildPromptPreview 生成 96-rune 脱敏预览文本。
// 先脱敏 Bearer token、API key、密码、邮箱、手机号等模式，再按 Unicode rune 截取最多 96 字符。
func BuildPromptPreview(value string) string {
	value = bearerPattern.ReplaceAllString(value, "Bearer ***")
	value = apiKeyPattern.ReplaceAllStringFunc(value, func(match string) string {
		if idx := strings.IndexAny(match, ":= \t"); idx >= 0 {
			return match[:idx+1] + "***"
		}
		return "***"
	})
	value = canaryPattern.ReplaceAllString(value, "${1}***")
	value = emailPattern.ReplaceAllString(value, "***@***")
	value = phonePattern.ReplaceAllString(value, "***PHONE***")

	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) > MaxRedactedPreviewRunes {
		return string(runes[:MaxRedactedPreviewRunes])
	}
	return string(runes)
}

// BuildPromptSnapshot 结合上下文环境与提取结果组装 PromptSnapshot。
func BuildPromptSnapshot(
	c *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	protocol string,
	modelName string,
	segments []PromptSegment,
	latestTurnOnly bool,
) (PromptSnapshot, error) {
	if len(NormalizeSegments(segments)) == 0 {
		return PromptSnapshot{}, ErrNoPrompt
	}

	// 剥离 Codex 注入的 AGENTS.md 信封、成对上下文标记块、标题生成模板、回看摘要及压缩指令，确保后续只包含用户真实提示词
	cleanSegments := StripAgentsMdEnvelopes(segments)
	cleanSegments = StripCodexContextualUser(cleanSegments)
	cleanSegments = UnwrapCodexThreadTitle(cleanSegments)
	cleanSegments = StripCodexCatchUp(cleanSegments)
	cleanSegments = StripCodexCompaction(cleanSegments)
	normalized := NormalizeSegments(cleanSegments)

	fullPrompt := JoinUserSegments(normalized)

	auditScope := "user"
	if latestTurnOnly {
		auditScope = "latest_turn"
	}
	scanSegments := SelectUserScanSegments(normalized, latestTurnOnly)
	scanText := BuildScanText(scanSegments)

	userMessageCount := 0
	for _, seg := range normalized {
		if seg.IsUser() {
			userMessageCount++
		}
	}

	reqID := ""
	if relayInfo != nil && relayInfo.RequestId != "" {
		reqID = relayInfo.RequestId
	} else if c != nil {
		reqID = c.GetString(common.RequestIdKey)
	}
	if reqID == "" {
		reqID = common.NewRequestId()
	}

	userID := 0
	if relayInfo != nil && relayInfo.UserId != 0 {
		userID = relayInfo.UserId
	} else if c != nil {
		userID = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	}

	username := ""
	if c != nil {
		username = c.GetString("username")
		if username == "" {
			username = common.GetContextKeyString(c, constant.ContextKeyUserName)
		}
	}

	userEmail := ""
	if relayInfo != nil && relayInfo.UserEmail != "" {
		userEmail = relayInfo.UserEmail
	} else if c != nil {
		userEmail = c.GetString("user_email")
		if userEmail == "" {
			userEmail = common.GetContextKeyString(c, constant.ContextKeyUserEmail)
		}
	}

	tokenID := 0
	if relayInfo != nil && relayInfo.TokenId != 0 {
		tokenID = relayInfo.TokenId
	} else if c != nil {
		tokenID = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	}

	tokenName := ""
	if c != nil {
		tokenName = c.GetString("token_name")
	}

	group := ""
	if relayInfo != nil {
		if relayInfo.UsingGroup != "" {
			group = relayInfo.UsingGroup
		} else {
			group = relayInfo.TokenGroup
		}
	}
	if group == "" && c != nil {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		}
	}

	requestPath := ""
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		requestPath = relayInfo.RequestURLPath
	} else if c != nil && c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}

	if modelName == "" && relayInfo != nil {
		modelName = relayInfo.OriginModelName
	}

	channelID := 0
	channelType := 0
	if relayInfo != nil && relayInfo.ChannelMeta != nil {
		channelID = relayInfo.ChannelId
		channelType = relayInfo.ChannelType
	}
	if channelID == 0 && c != nil {
		channelID = c.GetInt("channel_id")
	}
	if channelType == 0 && c != nil {
		channelType = c.GetInt("channel_type")
	}

	snapshot := PromptSnapshot{
		RequestID:         reqID,
		UserID:            userID,
		UsernameSnapshot:  username,
		UserEmailSnapshot: userEmail,
		TokenID:           tokenID,
		TokenNameSnapshot: tokenName,
		Group:             group,
		RequestPath:       requestPath,
		Protocol:          protocol,
		Model:             modelName,
		Stage:             "http",
		FullPrompt:        fullPrompt,
		ScanText:          scanText,
		PromptHash:        CalculatePromptHash(fullPrompt),
		RedactedPreview:   BuildPromptPreview(scanText),
		PromptLength:      utf8.RuneCountInString(fullPrompt),
		MessageCount:      userMessageCount,
		AuditScope:        auditScope,
		ChannelID:         channelID,
		ChannelType:       channelType,
	}

	return snapshot, nil
}
