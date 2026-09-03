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
}

var ScannerCatalog = map[string]ScannerDefinition{
	"violent": {
		ID:          "violent",
		Label:       "Violent",
		LabelZH:     "暴力",
		Description: "Violence or threats of violence",
	},
	"non_violent_illegal_acts": {
		ID:          "non_violent_illegal_acts",
		Label:       "Non-violent Illegal Acts",
		LabelZH:     "非暴力违法行为",
		Description: "Non-violent illegal activity",
	},
	"sexual_content_or_sexual_acts": {
		ID:          "sexual_content_or_sexual_acts",
		Label:       "Sexual Content or Sexual Acts",
		LabelZH:     "性内容或性行为",
		Description: "Sexual content or sexual acts",
	},
	"pii": {
		ID:          "pii",
		Label:       "PII",
		LabelZH:     "个人敏感信息",
		Description: "Personal identifying information",
	},
	"suicide_and_self_harm": {
		ID:          "suicide_and_self_harm",
		Label:       "Suicide & Self-Harm",
		LabelZH:     "自杀与自残",
		Description: "Suicide or self-harm",
	},
	"unethical_acts": {
		ID:          "unethical_acts",
		Label:       "Unethical Acts",
		LabelZH:     "不道德行为",
		Description: "Unethical behavior",
	},
	"politically_sensitive_topics": {
		ID:          "politically_sensitive_topics",
		Label:       "Politically Sensitive Topics",
		LabelZH:     "政治敏感话题",
		Description: "Politically sensitive topics",
	},
	"copyright_violation": {
		ID:          "copyright_violation",
		Label:       "Copyright Violation",
		LabelZH:     "版权侵权",
		Description: "Copyright infringement",
	},
	"jailbreak": {
		ID:          "jailbreak",
		Label:       "Jailbreak",
		LabelZH:     "越狱攻击",
		Description: "Prompt injection or jailbreak attempt",
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

	enabled := make(map[string]struct{}, len(enabledScanners))
	for _, scanner := range enabledScanners {
		enabled[NormalizeCategory(scanner)] = struct{}{}
	}

	known := make(map[string]struct{})
	unknown := make(map[string]struct{})

	for _, raw := range strings.Split(categoryLine, ",") {
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
		ScannerBackend:    "qwen3guard-openai",
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

	return result, nil
}

func unknownCategoryID(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return fmt.Sprintf("unknown:%x", digest[:8])
}

func isElevatedControversial(category string) bool {
	return category == "jailbreak" || category == "pii" || category == "suicide_and_self_harm"
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
