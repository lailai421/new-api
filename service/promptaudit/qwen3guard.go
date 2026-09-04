package promptaudit

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

var AllScannerIDs = []string{
	"violent",
	"non_violent_illegal_acts",
	"sexual_content_or_sexual_acts",
	"pii",
	"suicide_and_self_harm",
	"unethical_acts",
	"politically_sensitive_topics",
	"copyright_violation",
	"jailbreak",
	"cyber_abuse",
}

var ScannerCatalog = map[string]ScannerDefinition{
	"violent": {
		ID:            "violent",
		Label:         "Violent",
		LabelZH:       "暴力",
		Description:   "Violence or threats of violence",
		DescriptionZH: "暴力或暴力威胁",
	},
	"non_violent_illegal_acts": {
		ID:            "non_violent_illegal_acts",
		Label:         "Non-violent Illegal Acts",
		LabelZH:       "非暴力违法行为",
		Description:   "Non-violent illegal activity",
		DescriptionZH: "非暴力违法犯罪活动",
	},
	"sexual_content_or_sexual_acts": {
		ID:            "sexual_content_or_sexual_acts",
		Label:         "Sexual Content or Sexual Acts",
		LabelZH:       "性内容或性行为",
		Description:   "Sexual content or sexual acts",
		DescriptionZH: "性内容或性行为",
	},
	"pii": {
		ID:            "pii",
		Label:         "PII",
		LabelZH:       "个人敏感信息",
		Description:   "Personal identifying information",
		DescriptionZH: "个人身份与隐私敏感信息",
	},
	"suicide_and_self_harm": {
		ID:            "suicide_and_self_harm",
		Label:         "Suicide & Self-Harm",
		LabelZH:       "自杀与自残",
		Description:   "Suicide or self-harm",
		DescriptionZH: "自杀或自残倾向与行为",
	},
	"unethical_acts": {
		ID:            "unethical_acts",
		Label:         "Unethical Acts",
		LabelZH:       "不道德行为",
		Description:   "Unethical behavior",
		DescriptionZH: "不道德或违背公序良俗的行为",
	},
	"politically_sensitive_topics": {
		ID:            "politically_sensitive_topics",
		Label:         "Politically Sensitive Topics",
		LabelZH:       "政治敏感话题",
		Description:   "Politically sensitive topics",
		DescriptionZH: "政治敏感与争议话题",
	},
	"copyright_violation": {
		ID:            "copyright_violation",
		Label:         "Copyright Violation",
		LabelZH:       "版权侵权",
		Description:   "Copyright infringement",
		DescriptionZH: "版权侵权与盗版侵权",
	},
	"jailbreak": {
		ID:            "jailbreak",
		Label:       "Jailbreak",
		LabelZH:     "越狱攻击",
		Description:   "Prompt injection or jailbreak attempt",
		DescriptionZH: "大模型提示词注入或越狱攻击",
	},
	"cyber_abuse": {
		ID:            "cyber_abuse",
		Label:         "Cyber Abuse",
		LabelZH:       "网络滥用",
		Description:   "Malware, exploits, unauthorized access, reverse engineering, and cracking",
		DescriptionZH: "恶意软件、漏洞利用、未授权访问、逆向与破解",
	},
}

var categoryAliases = map[string]string{
	"violent":                           "violent",
	"violence":                          "violent",
	"non violent illegal acts":          "non_violent_illegal_acts",
	"non-violent illegal acts":          "non_violent_illegal_acts",
	"sexual content or sexual acts":     "sexual_content_or_sexual_acts",
	"sexual":                            "sexual_content_or_sexual_acts",
	"pii":                               "pii",
	"personal identifying information":  "pii",
	"personal identifiable information": "pii",
	"suicide self harm":                 "suicide_and_self_harm",
	"suicide and self harm":             "suicide_and_self_harm",
	"suicide & self-harm":               "suicide_and_self_harm",
	"unethical acts":                    "unethical_acts",
	"unethical":                         "unethical_acts",
	"politically sensitive topics":      "politically_sensitive_topics",
	"political":                         "politically_sensitive_topics",
	"copyright violation":               "copyright_violation",
	"copyright":                         "copyright_violation",
	"jailbreak":                         "jailbreak",
	"prompt injection":                  "jailbreak",
	"cyber abuse":                       "cyber_abuse",
	"cyber":                             "cyber_abuse",
	"cyberattack":                       "cyber_abuse",
	"malware":                           "cyber_abuse",
	"hacking":                           "cyber_abuse",
	"exploit":                           "cyber_abuse",
	"reverse engineering":               "cyber_abuse",
	"cracking":                          "cyber_abuse",
	"ransomware":                        "cyber_abuse",
	"rat":                               "cyber_abuse",
	"c2":                                "cyber_abuse",
	"网络滥用":                              "cyber_abuse",
	"恶意软件":                              "cyber_abuse",
	"木马":                                "cyber_abuse",
	"破解":                                "cyber_abuse",
	"逆向":                                "cyber_abuse",
}

// NormalizeCategory 将分类名称归一化为标准的下划线标识符。
func NormalizeCategory(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", " ", "&", " and ", "/", " ", "-", " ", "–", " ", "—", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if canonical, ok := categoryAliases[normalized]; ok {
		return canonical
	}
	return strings.ReplaceAll(normalized, " ", "_")
}

// ParseQwen3Guard 严格解析 Qwen3Guard 输出内容并映射为标准判决结果。
// 规则：必须且仅能存在一个 Safety: 和一个 Categories: 主字段；未知或缺失主字段均为非法响应。
func ParseQwen3Guard(content string, enabledScanners []string) (*NormalizedResult, error) {
	var safety string
	var categoryLine string

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "safety:"):
			if safety != "" {
				return nil, &GuardError{Code: ErrorCodeInvalidResponse}
			}
			safety = strings.TrimSpace(trimmed[len("safety:"):])
		case strings.HasPrefix(lower, "categories:"):
			if categoryLine != "" {
				return nil, &GuardError{Code: ErrorCodeInvalidResponse}
			}
			categoryLine = strings.TrimSpace(trimmed[len("categories:"):])
		default:
			// 忽略 Refusal 等辅助性输出，不影响安全决策
		}
	}

	switch strings.ToLower(safety) {
	case "safe":
		safety = "Safe"
	case "controversial":
		safety = "Controversial"
	case "unsafe":
		safety = "Unsafe"
	default:
		return nil, &GuardError{Code: ErrorCodeInvalidResponse}
	}

	if categoryLine == "" {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse}
	}

	rawCategories := strings.Split(categoryLine, ",")
	return ApplySafetyDecision(safety, rawCategories, enabledScanners), nil
}

// ApplySafetyDecision 根据标准化安全评级、分类清单以及启用的 Scanner 列表计算最终安全决策。
// 该逻辑供 Qwen3Guard 与 LLM 分类器两路完全复用。
func ApplySafetyDecision(safety string, rawCategories []string, enabledScanners []string) *NormalizedResult {
	enabled := make(map[string]struct{}, len(enabledScanners))
	for _, scanner := range enabledScanners {
		enabled[NormalizeCategory(scanner)] = struct{}{}
	}

	known := make(map[string]struct{})
	unknown := make(map[string]struct{})

	for _, raw := range rawCategories {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "n/a") {
			continue
		}
		category := NormalizeCategory(raw)
		if _, ok := ScannerCatalog[category]; ok {
			known[category] = struct{}{}
		} else {
			unknown[unknownCategoryID(category)] = struct{}{}
		}
	}

	knownList := orderedScannerKeys(known)
	unknownList := sortedKeys(unknown)

	matched := make([]string, 0, len(knownList))
	for _, cat := range knownList {
		if _, ok := enabled[cat]; ok {
			matched = append(matched, cat)
		}
	}

	result := &NormalizedResult{
		Safety:            safety,
		Categories:        knownList,
		MatchedScanners:   matched,
		UnknownCategories: unknownList,
		ScannerScores:     make(map[string]float64),
		ScannerEvidence:   make(map[string]string),
		ScannerBackend:    ScannerBackendQwen3Guard,
		ScannerVersion:    "qwen3guard",
		PolicyID:          StrategyPriority,
		PolicyVersion:     1,
		Decision:          EventPass,
		RiskLevel:         RiskLow,
		Action:            ActionAllow,
	}

	score := 0.0
	if safety == "Controversial" {
		score = 0.5
		result.Decision = EventFlag
		result.RiskLevel = RiskMedium
		result.Action = ActionWarn
	}

	if safety == "Unsafe" {
		score = 1.0
		// 命中启用分类、未知分类或未提供已知分类时均 Block
		if len(matched) > 0 || len(unknownList) > 0 || len(knownList) == 0 {
			result.Decision = EventCritical
			result.RiskLevel = RiskCritical
			result.Action = ActionBlock
		} else {
			// 仅命中管理员禁用的已知分类时 Flag/Warn
			result.Decision = EventFlag
			result.RiskLevel = RiskHigh
			result.Action = ActionWarn
		}
	}

	for _, cat := range matched {
		result.ScannerScores[cat] = score
		result.ScannerEvidence[cat] = ScannerCatalog[cat].Label
		if safety == "Controversial" && isElevatedControversial(cat) {
			result.Decision = EventCritical
			result.RiskLevel = RiskCritical
			result.Action = ActionBlock
		}
	}

	return result
}

func unknownCategoryID(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return fmt.Sprintf("unknown:%x", digest[:8])
}

func isElevatedControversial(category string) bool {
	return category == "jailbreak" || category == "pii" || category == "suicide_and_self_harm" || category == "cyber_abuse"
}

func orderedScannerKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	remaining := make(map[string]struct{}, len(values))
	for key := range values {
		remaining[key] = struct{}{}
	}
	for _, scannerID := range AllScannerIDs {
		if _, ok := remaining[scannerID]; ok {
			result = append(result, scannerID)
			delete(remaining, scannerID)
		}
	}
	result = append(result, sortedKeys(remaining)...)
	return result
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// ScannerCatalogDefinitions 返回稳定排序的分类清单。
func ScannerCatalogDefinitions() []ScannerDefinition {
	result := make([]ScannerDefinition, 0, len(AllScannerIDs))
	for _, id := range AllScannerIDs {
		result = append(result, ScannerCatalog[id])
	}
	return result
}
