package promptaudit

import (
	"context"
	"fmt"
)

const (
	// 审计运行模式
	ModeOff      Mode = "off"
	ModeBlocking Mode = "blocking"

	// 最终决策类型
	DecisionAllow       DecisionKind = "allow"
	DecisionFlag        DecisionKind = "flag"
	DecisionBlock       DecisionKind = "block"
	DecisionUnavailable DecisionKind = "unavailable"
	DecisionInvalid     DecisionKind = "invalid"

	// 规范化事件结论
	EventPass     = "pass"
	EventFlag     = "flag"
	EventCritical = "critical"

	// 风险级别
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"

	// 处置动作
	ActionAllow = "Allow"
	ActionWarn  = "Warn"
	ActionBlock = "Block"

	// 稳定错误码
	ErrorCodeBlocked               = "prompt_guard_blocked"
	ErrorCodeUnavailable           = "prompt_guard_unavailable"
	ErrorCodeInvalidResponse       = "prompt_guard_invalid_response"
	ErrorCodeConfigDegraded        = "prompt_audit_config_degraded"
	ErrorCodeRecordFailed          = "prompt_audit_record_failed"
	ErrorCodeUnsupportedProtocol   = "prompt_audit_unsupported_protocol"
	ErrorCodeConfigConflict        = "prompt_audit_config_conflict"
	ErrorCodeEncryptionKeyRequired = "prompt_audit_encryption_key_required"

	// 节点协议与默认配置
	ProtocolOpenAICompatible    = "openai_compatible"
	ProtocolLLMClassifier       = "llm_classifier"
	DefaultGuardModel           = "sileader/qwen3guard:0.6b"
	StrategyPriority            = "priority"
	ScannerBackendQwen3Guard    = "qwen3guard-openai"
	ScannerBackendLLMClassifier = "llm-classifier-openai"

	// 阈值与边界
	DefaultTimeoutMS         = 3000
	DefaultLLMTimeoutMS      = 8000
	DefaultLLMMaxTokens      = 256
	DefaultLLMSeed           = 42
	MinTimeoutMS             = 100
	MaxTimeoutMS             = 30000
	DefaultInputLimit        = 4000
	MinInputLimit            = 128
	MaxInputLimit            = 100000
	MaxGuardResponseBytes    = 256 * 1024
	DefaultGlobalConcurrency = 64
	DefaultNodeConcurrency   = 16

	// 分段优先级分隔符（用于标记优先扫描片段）
	PrioritySeparator = "\x1e"

	// Option 存储键名
	OptionKeyPromptAuditConfigSecret = "PromptAuditConfigSecret"
)

type Mode string
type DecisionKind string

// PromptSnapshot 是请求入口向提示词审计领域核心提交的统一快照数据。
type PromptSnapshot struct {
	RequestID         string `json:"request_id"`
	UserID            int    `json:"user_id"`
	UsernameSnapshot  string `json:"username_snapshot"`
	UserEmailSnapshot string `json:"user_email_snapshot"`
	TokenID           int    `json:"token_id"`
	TokenNameSnapshot string `json:"token_name_snapshot"`
	Group             string `json:"group"`
	RequestPath       string `json:"request_path"`
	Protocol          string `json:"protocol"`
	Model             string `json:"model"`
	Stage             string `json:"stage"`
	FullPrompt        string `json:"full_prompt"` // 内存中完整输入，敏感字段，禁止直接入普通日志
	ScanText          string `json:"-"`           // 实际送审文本（完整输入或最新用户轮次）
	PromptHash        string `json:"prompt_hash"` // 完整原文的 SHA-256
	RedactedPreview   string `json:"redacted_preview"`
	PromptLength      int    `json:"prompt_length"`
	MessageCount      int    `json:"message_count"`
	AuditScope        string `json:"audit_scope"` // "full" 或 "latest_turn"
	ChannelID         int    `json:"channel_id,omitempty"`
	ChannelType       int    `json:"channel_type,omitempty"`
}

// Redacted 返回剥离了扫描明文的副本，用于安全持久化与元数据留存。
func (s PromptSnapshot) Redacted() PromptSnapshot {
	s.ScanText = ""
	return s
}

// NormalizedResult 为 Guard 节点执行判定后的规范化结构。
type NormalizedResult struct {
	Decision          string             `json:"decision"`
	RiskLevel         string             `json:"risk_level"`
	Action            string             `json:"action"`
	Safety            string             `json:"safety"`
	Categories        []string           `json:"categories"`
	MatchedScanners   []string           `json:"matched_scanners"`
	ScannerScores     map[string]float64 `json:"scanner_scores"`
	ScannerEvidence   map[string]string  `json:"scanner_evidence"`
	ScannerBackend    string             `json:"scanner_backend"`
	ScannerVersion    string             `json:"scanner_version"`
	GuardEndpointID   string             `json:"guard_endpoint_id"`
	PolicyID          string             `json:"policy_id"`
	PolicyVersion     int                `json:"policy_version"`
	ChunkTotal        int                `json:"chunk_total"`
	LatencyMS         int                `json:"latency_ms"`
	UnknownCategories []string           `json:"unknown_categories,omitempty"`
}

// Decision 是门禁评估最终返回给上游调用的不可变判定结果。
type Decision struct {
	Kind           DecisionKind      `json:"kind"`
	HTTPStatus     int               `json:"http_status"`
	ErrorCode      string            `json:"error_code,omitempty"`
	Result         *NormalizedResult `json:"result,omitempty"`
	AllowNextStage bool              `json:"allow_next_stage"`
}

// Endpoint 表示单个 Guard 节点的持久化/内部模型。
type Endpoint struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	TokenCiphertext string `json:"token_ciphertext,omitempty"`
	TimeoutMS       int    `json:"timeout_ms"`
	InputLimit      int    `json:"input_limit"`
	Enabled         bool   `json:"enabled"`
}

// ActiveEndpoint 是解密并完成运行时校验的可用 Guard 节点。
type ActiveEndpoint struct {
	ID           string
	Name         string
	Protocol     string
	BaseURL      string
	Model        string
	Token        string
	TimeoutMS    int
	InputLimit   int
	Enabled      bool
	TokenInvalid bool
}

// PublicEndpoint 是脱敏后的管理控制台/API 节点模型。
type PublicEndpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	TimeoutMS   int    `json:"timeout_ms"`
	InputLimit  int    `json:"input_limit"`
	Enabled     bool   `json:"enabled"`
	HasToken    bool   `json:"has_token"`
	TokenStatus string `json:"token_status"` // "configured", "missing", "invalid"
}

// ActiveConfig 是供门禁热路径安全消费的不可变运行时配置快照。
type ActiveConfig struct {
	Enabled         bool
	LatestTurnOnly  bool
	StorePassEvents bool
	AllGroups       bool
	Groups          []string
	GroupsMap       map[string]struct{}
	Scanners        []string
	ScannersMap     map[string]struct{}
	RetentionDays   int
	Strategy        string
	Endpoints       []ActiveEndpoint
	ConfigVersion   int64
	UpdatedAt       int64
	UpdatedBy       int
	ChangeSummary   string
}

// MatchesGroup 判断指定分组是否命中本配置生效范围（精确字符串匹配）。
func (c ActiveConfig) MatchesGroup(group string) bool {
	if !c.Enabled {
		return false
	}
	if c.AllGroups {
		return true
	}
	if len(c.GroupsMap) == 0 {
		return false
	}
	_, ok := c.GroupsMap[group]
	return ok
}

// EnabledEndpoints 返回已启用且凭据有效的节点列表。
func (c ActiveConfig) EnabledEndpoints() []ActiveEndpoint {
	result := make([]ActiveEndpoint, 0, len(c.Endpoints))
	for _, ep := range c.Endpoints {
		if ep.Enabled && !ep.TokenInvalid {
			result = append(result, ep)
		}
	}
	return result
}

// PublicConfig 是脱敏后的公开配置结构体，供管理 API 与前端读取。
type PublicConfig struct {
	Enabled         bool             `json:"enabled"`
	EffectiveMode   Mode             `json:"effective_mode"`
	LatestTurnOnly  bool             `json:"latest_turn_only"`
	StorePassEvents bool             `json:"store_pass_events"`
	AllGroups       bool             `json:"all_groups"`
	Groups          []string         `json:"groups"`
	Scanners        []string         `json:"scanners"`
	RetentionDays   int              `json:"retention_days"`
	Strategy        string           `json:"strategy"`
	Endpoints       []PublicEndpoint `json:"endpoints"`
	ConfigVersion   int64            `json:"config_version"`
	UpdatedAt       int64            `json:"updated_at"`
	UpdatedBy       int              `json:"updated_by"`
	ChangeSummary   string           `json:"change_summary"`
}

// ScannerDefinition 描述分类元信息。
type ScannerDefinition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LabelZH     string `json:"label_zh"`
	Description string `json:"description"`
}

// GuardError 表示 Guard 执行或解析阶段的确定性错误。
type GuardError struct {
	Code       string
	HTTPStatus int
	Retryable  bool
	Timeout    bool
	Cause      error
}

func (e *GuardError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *GuardError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ConfigStore 是配置持久化抽象接口，由 storage-api 实现并注入。
type ConfigStore interface {
	Load(ctx context.Context) (*Config, error)
	Save(ctx context.Context, cfg *Config, expectedVersion int64) error
}

// EventStore 是审计事件持久化抽象接口，由 storage-api 实现并注入。
type EventStore interface {
	Record(ctx context.Context, snapshot PromptSnapshot, decision *Decision, storePassEvents bool) error
}

// Encryptor 提供基于用途隔离的加解密能力。
type Encryptor interface {
	EncryptToken(plaintext string) (string, error)
	DecryptToken(ciphertext string) (string, error)
	EncryptPrompt(plaintext string) (string, error)
	DecryptPrompt(ciphertext string) (string, error)
}

// Evaluator 是门禁执行核心接口。
type Evaluator interface {
	Evaluate(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot) (*Decision, error)
}

// PromptScanner 是单个分片送审 Scanner 接口。
type PromptScanner interface {
	Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error)
}
