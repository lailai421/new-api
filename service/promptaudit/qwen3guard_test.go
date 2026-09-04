package promptaudit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQwen3Guard_Safe(t *testing.T) {
	content := "Safety: Safe\nCategories: None"
	res, err := ParseQwen3Guard(content, AllScannerIDs)
	require.NoError(t, err)

	assert.Equal(t, "Safe", res.Safety)
	assert.Equal(t, EventPass, res.Decision)
	assert.Equal(t, RiskLow, res.RiskLevel)
	assert.Equal(t, ActionAllow, res.Action)
	assert.Empty(t, res.Categories)
	assert.Empty(t, res.MatchedScanners)
	assert.Empty(t, res.UnknownCategories)
}

func TestParseQwen3Guard_Controversial_Normal(t *testing.T) {
	content := "Safety: Controversial\nCategories: Politically Sensitive Topics"
	res, err := ParseQwen3Guard(content, AllScannerIDs)
	require.NoError(t, err)

	assert.Equal(t, "Controversial", res.Safety)
	assert.Equal(t, EventFlag, res.Decision)
	assert.Equal(t, RiskMedium, res.RiskLevel)
	assert.Equal(t, ActionWarn, res.Action)
	assert.Equal(t, []string{"politically_sensitive_topics"}, res.MatchedScanners)
	assert.Equal(t, 0.5, res.ScannerScores["politically_sensitive_topics"])
}

func TestParseQwen3Guard_Controversial_Elevated(t *testing.T) {
	elevatedCategories := []struct {
		raw      string
		expected string
	}{
		{raw: "Jailbreak", expected: "jailbreak"},
		{raw: "Prompt Injection", expected: "jailbreak"},
		{raw: "PII", expected: "pii"},
		{raw: "Personal Identifying Information", expected: "pii"},
		{raw: "Suicide & Self-Harm", expected: "suicide_and_self_harm"},
		{raw: "Suicide and Self Harm", expected: "suicide_and_self_harm"},
		{raw: "Cyber Abuse", expected: "cyber_abuse"},
		{raw: "cyber_abuse", expected: "cyber_abuse"},
		{raw: "malware", expected: "cyber_abuse"},
		{raw: "c2", expected: "cyber_abuse"},
		{raw: "rat", expected: "cyber_abuse"},
		{raw: "exploit", expected: "cyber_abuse"},
		{raw: "reverse engineering", expected: "cyber_abuse"},
		{raw: "cracking", expected: "cyber_abuse"},
		{raw: "网络滥用", expected: "cyber_abuse"},
		{raw: "木马", expected: "cyber_abuse"},
		{raw: "破解", expected: "cyber_abuse"},
		{raw: "逆向", expected: "cyber_abuse"},
	}

	for _, tc := range elevatedCategories {
		t.Run(tc.raw, func(t *testing.T) {
			content := "Safety: Controversial\nCategories: " + tc.raw
			res, err := ParseQwen3Guard(content, AllScannerIDs)
			require.NoError(t, err)

			assert.Equal(t, "Controversial", res.Safety)
			// 特殊类别必须升级为 Block
			assert.Equal(t, EventCritical, res.Decision)
			assert.Equal(t, RiskCritical, res.RiskLevel)
			assert.Equal(t, ActionBlock, res.Action)
			assert.Contains(t, res.MatchedScanners, tc.expected)
		})
	}
}

func TestParseQwen3Guard_Unsafe_Matched(t *testing.T) {
	content := "Safety: Unsafe\nCategories: Violent, Sexual Content or Sexual Acts"
	res, err := ParseQwen3Guard(content, AllScannerIDs)
	require.NoError(t, err)

	assert.Equal(t, "Unsafe", res.Safety)
	assert.Equal(t, EventCritical, res.Decision)
	assert.Equal(t, RiskCritical, res.RiskLevel)
	assert.Equal(t, ActionBlock, res.Action)
	assert.Equal(t, []string{"violent", "sexual_content_or_sexual_acts"}, res.MatchedScanners)
	assert.Equal(t, 1.0, res.ScannerScores["violent"])
}

func TestParseQwen3Guard_Unsafe_OnlyDisabledCategory(t *testing.T) {
	// 仅启用了 pii，但模型报出 violent（管理员禁用了 violent）
	content := "Safety: Unsafe\nCategories: Violent"
	res, err := ParseQwen3Guard(content, []string{"pii"})
	require.NoError(t, err)

	assert.Equal(t, "Unsafe", res.Safety)
	// 仅命中被管理员禁用的已知分类时降级为 Flag/Warn
	assert.Equal(t, EventFlag, res.Decision)
	assert.Equal(t, RiskHigh, res.RiskLevel)
	assert.Equal(t, ActionWarn, res.Action)
	assert.Empty(t, res.MatchedScanners)
	assert.Equal(t, []string{"violent"}, res.Categories)
}

func TestParseQwen3Guard_Unsafe_UnknownOrEmptyCategories(t *testing.T) {
	// 1. 报出未知的分类 -> 必须 Block
	content1 := "Safety: Unsafe\nCategories: Alien Bio-Weapon Hazard"
	res1, err := ParseQwen3Guard(content1, AllScannerIDs)
	require.NoError(t, err)
	assert.Equal(t, EventCritical, res1.Decision)
	assert.Equal(t, ActionBlock, res1.Action)
	assert.NotEmpty(t, res1.UnknownCategories)

	// 2. Unsafe 但分类为 None/空 -> 必须 Block
	content2 := "Safety: Unsafe\nCategories: None"
	res2, err := ParseQwen3Guard(content2, AllScannerIDs)
	require.NoError(t, err)
	assert.Equal(t, EventCritical, res2.Decision)
	assert.Equal(t, ActionBlock, res2.Action)
}

func TestParseQwen3Guard_StrictFormatting(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "missing safety line",
			content: "Categories: Violent",
			wantErr: true,
		},
		{
			name:    "missing categories line",
			content: "Safety: Safe",
			wantErr: true,
		},
		{
			name:    "duplicate safety line",
			content: "Safety: Safe\nSafety: Unsafe\nCategories: None",
			wantErr: true,
		},
		{
			name:    "duplicate categories line",
			content: "Safety: Safe\nCategories: None\nCategories: Violent",
			wantErr: true,
		},
		{
			name:    "unknown safety keyword",
			content: "Safety: Moderate\nCategories: None",
			wantErr: true,
		},
		{
			name:    "windows CRLF line breaks supported",
			content: "Safety: Safe\r\nCategories: None\r\nRefusal: N/A\r\n",
			wantErr: false,
		},
		{
			name:    "auxiliary lines ignored",
			content: "Refusal: Sorry\nSafety: Safe\nNote: Extra commentary\nCategories: None",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseQwen3Guard(tt.content, AllScannerIDs)
			if tt.wantErr {
				require.Error(t, err)
				var gErr *GuardError
				require.ErrorAs(t, err, &gErr)
				assert.Equal(t, ErrorCodeInvalidResponse, gErr.Code)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestApplySafetyDecision_Direct(t *testing.T) {
	// Safe
	resSafe := ApplySafetyDecision("Safe", []string{}, AllScannerIDs)
	assert.Equal(t, "Safe", resSafe.Safety)
	assert.Equal(t, EventPass, resSafe.Decision)
	assert.Equal(t, ActionAllow, resSafe.Action)

	// Controversial 普通
	resContro := ApplySafetyDecision("Controversial", []string{"politically_sensitive_topics"}, AllScannerIDs)
	assert.Equal(t, EventFlag, resContro.Decision)
	assert.Equal(t, ActionWarn, resContro.Action)
	assert.Equal(t, 0.5, resContro.ScannerScores["politically_sensitive_topics"])

	// Controversial 升级
	resElevated := ApplySafetyDecision("Controversial", []string{"jailbreak"}, AllScannerIDs)
	assert.Equal(t, EventCritical, resElevated.Decision)
	assert.Equal(t, ActionBlock, resElevated.Action)

	// Unsafe 命中禁用分类 -> Flag / High / Warn
	resDisabled := ApplySafetyDecision("Unsafe", []string{"violent"}, []string{"pii"})
	assert.Equal(t, EventFlag, resDisabled.Decision)
	assert.Equal(t, RiskHigh, resDisabled.RiskLevel)
	assert.Equal(t, ActionWarn, resDisabled.Action)
	assert.Empty(t, resDisabled.MatchedScanners)
	assert.Equal(t, []string{"violent"}, resDisabled.Categories)

	// Controversial 命中 cyber_abuse 必须升格为 Block
	resCyberContro := ApplySafetyDecision("Controversial", []string{"cyber_abuse"}, AllScannerIDs)
	assert.Equal(t, EventCritical, resCyberContro.Decision)
	assert.Equal(t, ActionBlock, resCyberContro.Action)
	assert.Equal(t, []string{"cyber_abuse"}, resCyberContro.MatchedScanners)

	// Unsafe 仅命中禁用的 cyber_abuse -> Flag / High / Warn
	resCyberDisabled := ApplySafetyDecision("Unsafe", []string{"cyber_abuse"}, []string{"pii"})
	assert.Equal(t, EventFlag, resCyberDisabled.Decision)
	assert.Equal(t, RiskHigh, resCyberDisabled.RiskLevel)
	assert.Equal(t, ActionWarn, resCyberDisabled.Action)
	assert.Empty(t, resCyberDisabled.MatchedScanners)
	assert.Equal(t, []string{"cyber_abuse"}, resCyberDisabled.Categories)
}

func TestScannerCatalog_CyberAbuseAndDescriptionZH(t *testing.T) {
	require.Len(t, AllScannerIDs, 10)
	assert.Equal(t, "cyber_abuse", AllScannerIDs[9])

	for _, id := range AllScannerIDs {
		def, ok := ScannerCatalog[id]
		require.True(t, ok, "scanner %s must exist in catalog", id)
		assert.NotEmpty(t, def.Label, "scanner %s must have Label", id)
		assert.NotEmpty(t, def.LabelZH, "scanner %s must have LabelZH", id)
		assert.NotEmpty(t, def.Description, "scanner %s must have Description", id)
		assert.NotEmpty(t, def.DescriptionZH, "scanner %s must have DescriptionZH", id)
	}

	cyber := ScannerCatalog["cyber_abuse"]
	assert.Equal(t, "cyber_abuse", cyber.ID)
	assert.Equal(t, "Cyber Abuse", cyber.Label)
	assert.Equal(t, "网络滥用", cyber.LabelZH)
	assert.Equal(t, "Malware, exploits, unauthorized access, reverse engineering, and cracking", cyber.Description)
	assert.Equal(t, "恶意软件、漏洞利用、未授权访问、逆向与破解", cyber.DescriptionZH)
}

