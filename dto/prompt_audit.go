package dto

// PromptAuditEndpointUpdateRequest 用于更新单个端点配置的 DTO。
type PromptAuditEndpointUpdateRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	TimeoutMS   int    `json:"timeout_ms"`
	InputLimit  int    `json:"input_limit"`
	Enabled     bool   `json:"enabled"`
	Token       string `json:"token,omitempty"`        // 写入专用明文，绝不在响应中回显
	DeleteToken bool   `json:"delete_token,omitempty"` // 为 true 时清空已有 Token
}

// PromptAuditConfigUpdateRequest 用于全量更新审计配置的 DTO。
type PromptAuditConfigUpdateRequest struct {
	Enabled               *bool                              `json:"enabled"`
	LatestTurnOnly        *bool                              `json:"latest_turn_only"`
	StorePassEvents       *bool                              `json:"store_pass_events"`
	AllGroups             *bool                              `json:"all_groups"`
	Groups                []string                           `json:"groups"`
	Scanners              []string                           `json:"scanners"`
	RetentionDays         *int                               `json:"retention_days"`
	Strategy              string                             `json:"strategy"`
	Endpoints             []PromptAuditEndpointUpdateRequest `json:"endpoints"`
	ExpectedConfigVersion int64                              `json:"expected_config_version"`
}

// PromptAuditProbeRequest 探测请求 DTO。
type PromptAuditProbeRequest struct {
	EndpointID string `json:"endpoint_id,omitempty"`
	// 支持行内临时探测
	Protocol   string `json:"protocol,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	Token      string `json:"token,omitempty"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
	InputLimit int    `json:"input_limit,omitempty"`
}

// PromptAuditProbeResponse 探测响应 DTO（绝不回显 Token）。
type PromptAuditProbeResponse struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Model     string `json:"model"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code,omitempty"`
}

// PromptAuditEventListRequest 列表查询过滤参数。
type PromptAuditEventListRequest struct {
	StartTime       int64  `form:"start_time"`
	EndTime         int64  `form:"end_time"`
	UserID          int    `form:"user_id"`
	TokenID         int    `form:"token_id"`
	Group           string `form:"group"`
	Model           string `form:"model"`
	Protocol        string `form:"protocol"`
	Decision        string `form:"decision"`
	RiskLevel       string `form:"risk_level"`
	Category        string `form:"category"`
	GuardEndpointID string `form:"guard_endpoint_id"`
	RequestID       string `form:"request_id"`
	PromptHash      string `form:"prompt_hash"`
	Page            int    `form:"p"`
	PageSize        int    `form:"page_size"`
}

// PromptAuditEventItem 列表单项 DTO，不包含 full_prompt 与 full_prompt_ciphertext。
type PromptAuditEventItem struct {
	ID                int64    `json:"id"`
	RequestID         string   `json:"request_id"`
	UserID            int      `json:"user_id"`
	UsernameSnapshot  string   `json:"username_snapshot"`
	UserEmailSnapshot string   `json:"user_email_snapshot"`
	TokenID           int      `json:"token_id"`
	TokenNameSnapshot string   `json:"token_name_snapshot"`
	Group             string   `json:"group"`
	ChannelID         int      `json:"channel_id"`
	ChannelType       int      `json:"channel_type"`
	RequestPath       string   `json:"request_path"`
	Protocol          string   `json:"protocol"`
	Model             string   `json:"model"`
	Stage             string   `json:"stage"`
	AuditScope        string   `json:"audit_scope"`
	PromptHash        string   `json:"prompt_hash"`
	RedactedPreview   string   `json:"redacted_preview"`
	PromptLength      int      `json:"prompt_length"`
	MessageCount      int      `json:"message_count"`
	Decision          string   `json:"decision"`
	RiskLevel         string   `json:"risk_level"`
	Action            string   `json:"action"`
	Categories        []string `json:"categories"`
	MatchedScanners   []string `json:"matched_scanners"`
	ScannerBackend    string   `json:"scanner_backend"`
	ScannerVersion    string   `json:"scanner_version"`
	GuardEndpointID   string   `json:"guard_endpoint_id"`
	PolicyID          string   `json:"policy_id"`
	PolicyVersion     int      `json:"policy_version"`
	ConfigVersion     int64    `json:"config_version"`
	ChunkTotal        int      `json:"chunk_total"`
	LatencyMS         int64    `json:"latency_ms"`
	ErrorCode         string   `json:"error_code,omitempty"`
	CreatedAt         int64    `json:"created_at"`
}

// PromptAuditEventDetailResponse 详情响应 DTO，包含解密后的 FullPrompt。
type PromptAuditEventDetailResponse struct {
	PromptAuditEventItem
	FullPrompt      string         `json:"full_prompt"`
	ScannerScores   map[string]any `json:"scanner_scores,omitempty"`
	ScannerEvidence map[string]any `json:"scanner_evidence,omitempty"`
}

// PromptAuditBatchDeleteRequest 批量删除请求 DTO。
type PromptAuditBatchDeleteRequest struct {
	IDs []int64 `json:"ids"`
}
