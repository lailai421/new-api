package promptaudit

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Config 表示提示词审计内部存储配置，序列化后保存至 Option 表。
type Config struct {
	Enabled         bool       `json:"enabled"`
	LatestTurnOnly  bool       `json:"latest_turn_only"`
	StorePassEvents bool       `json:"store_pass_events"`
	AllGroups       bool       `json:"all_groups"`
	Groups          []string   `json:"groups"`
	Scanners        []string   `json:"scanners"`
	RetentionDays   int        `json:"retention_days"`
	Strategy        string     `json:"strategy"`
	Endpoints       []Endpoint `json:"endpoints"`
	ConfigVersion   int64      `json:"config_version"`
	UpdatedAt       int64      `json:"updated_at"`
	UpdatedBy       int        `json:"updated_by"`
	ChangeSummary   string     `json:"change_summary"`
}

// legacyDefaultScannerIDs 冻结旧九类默认分类集合，用于在配置规范化时平滑升级到十类。
var legacyDefaultScannerIDs = []string{
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

// DefaultConfig 返回审计默认安全配置。
// 规则：默认关闭、全部分组、审计完整输入、保存 Pass 事件、永久保留(0)、默认全部分类全开、priority 策略。
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		LatestTurnOnly:  false,
		StorePassEvents: true,
		AllGroups:       true,
		Groups:          []string{},
		Scanners:        append([]string(nil), AllScannerIDs...),
		RetentionDays:   0,
		Strategy:        StrategyPriority,
		Endpoints:       []Endpoint{},
		ConfigVersion:   1,
		UpdatedAt:       0,
		UpdatedBy:       0,
		ChangeSummary:   "default configuration",
	}
}

// ParseConfig 从 JSON 字符串反序列化配置。
func ParseConfig(raw string) (*Config, error) {
	cfg := DefaultConfig()
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return &cfg, nil
	}
	if err := common.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal prompt audit config: %w", err)
	}
	return &cfg, nil
}

// SerializeConfig 将配置序列化为 JSON 字符串。
func (c *Config) Serialize() (string, error) {
	bytes, err := common.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("marshal prompt audit config: %w", err)
	}
	return string(bytes), nil
}

// NormalizeAndValidate 对配置执行确定性规范化与边界校验。
func (c *Config) NormalizeAndValidate(hasStableSecret bool) error {
	if c.Strategy == "" {
		c.Strategy = StrategyPriority
	}
	if c.Strategy != StrategyPriority {
		return fmt.Errorf("unsupported prompt audit strategy: %s", c.Strategy)
	}

	if c.RetentionDays < 0 {
		return errors.New("retention_days cannot be negative")
	}

	// 1. 规范化分组配置
	if c.AllGroups {
		c.Groups = []string{}
	} else {
		groupSet := make(map[string]struct{}, len(c.Groups))
		cleanGroups := make([]string, 0, len(c.Groups))
		for _, g := range c.Groups {
			trimmed := strings.TrimSpace(g)
			if trimmed == "" {
				continue
			}
			if _, exists := groupSet[trimmed]; !exists {
				groupSet[trimmed] = struct{}{}
				cleanGroups = append(cleanGroups, trimmed)
			}
		}
		sort.Strings(cleanGroups)
		c.Groups = cleanGroups
		if len(c.Groups) == 0 {
			return errors.New("prompt audit requires at least one non-empty group when all_groups is false")
		}
	}

	// 2. 规范化 Scanner 分类
	if len(c.Scanners) == 0 {
		return errors.New("prompt audit requires at least one scanner category")
	}
	scannerSet := make(map[string]struct{}, len(c.Scanners))
	for _, raw := range c.Scanners {
		canonical := NormalizeCategory(raw)
		if _, ok := ScannerCatalog[canonical]; !ok {
			return fmt.Errorf("unknown scanner category: %s", raw)
		}
		scannerSet[canonical] = struct{}{}
	}
	// 若配置的分类集合严格等于升级前的旧九类默认全集，自动并入 cyber_abuse
	if isLegacyDefaultScanners(scannerSet) {
		scannerSet["cyber_abuse"] = struct{}{}
	}
	// 按 AllScannerIDs 的稳定顺序排序
	normalizedScanners := make([]string, 0, len(scannerSet))
	for _, id := range AllScannerIDs {
		if _, ok := scannerSet[id]; ok {
			normalizedScanners = append(normalizedScanners, id)
		}
	}
	c.Scanners = normalizedScanners
	if len(c.Scanners) == 0 {
		return errors.New("prompt audit requires at least one valid scanner category")
	}

	// 3. 规范化节点配置
	idSet := make(map[string]struct{}, len(c.Endpoints))
	cleanEndpoints := make([]Endpoint, 0, len(c.Endpoints))
	for i := range c.Endpoints {
		ep := c.Endpoints[i]
		ep.ID = strings.TrimSpace(ep.ID)
		ep.Name = strings.TrimSpace(ep.Name)
		if ep.ID == "" {
			return fmt.Errorf("endpoint[%d] missing id", i)
		}
		if ep.Name == "" {
			return fmt.Errorf("endpoint[%d] missing name", i)
		}
		if _, exists := idSet[ep.ID]; exists {
			return fmt.Errorf("duplicate endpoint id: %s", ep.ID)
		}
		idSet[ep.ID] = struct{}{}

		normalizedURL, err := NormalizeBaseURL(ep.BaseURL)
		if err != nil {
			return fmt.Errorf("endpoint[%s] invalid base_url: %w", ep.ID, err)
		}
		ep.BaseURL = normalizedURL

		if ep.Protocol == "" {
			ep.Protocol = ProtocolOpenAICompatible
		}
		if ep.Protocol != ProtocolOpenAICompatible && ep.Protocol != ProtocolLLMClassifier {
			return fmt.Errorf("endpoint[%s] unsupported protocol: %s", ep.ID, ep.Protocol)
		}

		if ep.Protocol == ProtocolOpenAICompatible {
			if strings.TrimSpace(ep.Model) == "" {
				ep.Model = DefaultGuardModel
			} else {
				ep.Model = strings.TrimSpace(ep.Model)
			}
		} else if ep.Protocol == ProtocolLLMClassifier {
			ep.Model = strings.TrimSpace(ep.Model)
			if ep.Model == "" {
				return fmt.Errorf("endpoint[%s] missing model for protocol %s", ep.ID, ep.Protocol)
			}
		}

		if ep.TimeoutMS == 0 {
			if ep.Protocol == ProtocolLLMClassifier {
				ep.TimeoutMS = DefaultLLMTimeoutMS
			} else {
				ep.TimeoutMS = DefaultTimeoutMS
			}
		} else if ep.TimeoutMS < MinTimeoutMS || ep.TimeoutMS > MaxTimeoutMS {
			return fmt.Errorf("endpoint[%s] timeout_ms must be between %d and %d", ep.ID, MinTimeoutMS, MaxTimeoutMS)
		}

		if ep.InputLimit == 0 {
			ep.InputLimit = DefaultInputLimit
		} else if ep.InputLimit < MinInputLimit || ep.InputLimit > MaxInputLimit {
			return fmt.Errorf("endpoint[%s] input_limit must be between %d and %d", ep.ID, MinInputLimit, MaxInputLimit)
		}

		cleanEndpoints = append(cleanEndpoints, ep)
	}
	c.Endpoints = cleanEndpoints

	// 4. 当启用审计时，强制校验密钥稳定性与至少存在一个可用节点
	if c.Enabled {
		if !hasStableSecret {
			return &GuardError{
				Code:       ErrorCodeEncryptionKeyRequired,
				HTTPStatus: 400,
				Cause:      errors.New("stable CRYPTO_SECRET or SESSION_SECRET must be configured before enabling prompt audit"),
			}
		}

		hasUsableNode := false
		for _, ep := range c.Endpoints {
			if ep.Enabled {
				hasUsableNode = true
				break
			}
		}
		if !hasUsableNode {
			return errors.New("prompt audit requires at least one enabled endpoint when enabled")
		}
	}

	return nil
}

// ToPublic 转换为脱敏后的对外展示配置。
func (c *Config) ToPublic(encryptor Encryptor) PublicConfig {
	publicEndpoints := make([]PublicEndpoint, 0, len(c.Endpoints))
	for _, ep := range c.Endpoints {
		pub := PublicEndpoint{
			ID:         ep.ID,
			Name:       ep.Name,
			Protocol:   ep.Protocol,
			BaseURL:    ep.BaseURL,
			Model:      ep.Model,
			TimeoutMS:  ep.TimeoutMS,
			InputLimit: ep.InputLimit,
			Enabled:    ep.Enabled,
		}
		if ep.TokenCiphertext != "" {
			pub.HasToken = true
			if encryptor != nil {
				if _, err := encryptor.DecryptToken(ep.TokenCiphertext); err != nil {
					pub.TokenStatus = "invalid"
				} else {
					pub.TokenStatus = "configured"
				}
			} else {
				pub.TokenStatus = "configured"
			}
		} else {
			pub.HasToken = false
			pub.TokenStatus = "missing"
		}
		publicEndpoints = append(publicEndpoints, pub)
	}

	effectiveMode := ModeOff
	if c.Enabled {
		effectiveMode = ModeBlocking
	}

	return PublicConfig{
		Enabled:         c.Enabled,
		EffectiveMode:   effectiveMode,
		LatestTurnOnly:  c.LatestTurnOnly,
		StorePassEvents: c.StorePassEvents,
		AllGroups:       c.AllGroups,
		Groups:          append([]string(nil), c.Groups...),
		Scanners:        append([]string(nil), c.Scanners...),
		RetentionDays:   c.RetentionDays,
		Strategy:        c.Strategy,
		Endpoints:       publicEndpoints,
		ConfigVersion:   c.ConfigVersion,
		UpdatedAt:       c.UpdatedAt,
		UpdatedBy:       c.UpdatedBy,
		ChangeSummary:         c.ChangeSummary,
		HasStableCryptoSecret: HasStableCryptoSecret(),
	}
}

// ToActive 解密凭据并构建不可变的运行时活跃配置。
func (c *Config) ToActive(encryptor Encryptor) ActiveConfig {
	activeEndpoints := make([]ActiveEndpoint, 0, len(c.Endpoints))
	for _, ep := range c.Endpoints {
		act := ActiveEndpoint{
			ID:         ep.ID,
			Name:       ep.Name,
			Protocol:   ep.Protocol,
			BaseURL:    ep.BaseURL,
			Model:      ep.Model,
			TimeoutMS:  ep.TimeoutMS,
			InputLimit: ep.InputLimit,
			Enabled:    ep.Enabled,
		}
		if ep.TokenCiphertext != "" {
			if encryptor != nil {
				token, err := encryptor.DecryptToken(ep.TokenCiphertext)
				if err != nil {
					act.TokenInvalid = true
				} else {
					act.Token = token
				}
			} else {
				act.TokenInvalid = true
			}
		}
		activeEndpoints = append(activeEndpoints, act)
	}

	groupsMap := make(map[string]struct{}, len(c.Groups))
	for _, g := range c.Groups {
		groupsMap[g] = struct{}{}
	}

	scannersMap := make(map[string]struct{}, len(c.Scanners))
	for _, s := range c.Scanners {
		scannersMap[s] = struct{}{}
	}

	return ActiveConfig{
		Enabled:         c.Enabled,
		LatestTurnOnly:  c.LatestTurnOnly,
		StorePassEvents: c.StorePassEvents,
		AllGroups:       c.AllGroups,
		Groups:          append([]string(nil), c.Groups...),
		GroupsMap:       groupsMap,
		Scanners:        append([]string(nil), c.Scanners...),
		ScannersMap:     scannersMap,
		RetentionDays:   c.RetentionDays,
		Strategy:        c.Strategy,
		Endpoints:       activeEndpoints,
		ConfigVersion:   c.ConfigVersion,
		UpdatedAt:       c.UpdatedAt,
		UpdatedBy:       c.UpdatedBy,
		ChangeSummary:   c.ChangeSummary,
	}
}

func isLegacyDefaultScanners(scannerSet map[string]struct{}) bool {
	if len(scannerSet) != len(legacyDefaultScannerIDs) {
		return false
	}
	for _, id := range legacyDefaultScannerIDs {
		if _, ok := scannerSet[id]; !ok {
			return false
		}
	}
	return true
}
