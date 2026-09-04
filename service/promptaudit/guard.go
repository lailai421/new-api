package promptaudit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// SplitRunes 按照 Unicode rune 边界对文本进行切片，优先保留并切分前置优先段。
// 绝对不按字节截断，不丢失任何 Unicode 字符。
func SplitRunes(value string, limit int) []string {
	if limit <= 0 {
		limit = DefaultInputLimit
	}
	if value == "" {
		return nil
	}

	segments := strings.Split(value, PrioritySeparator)
	chunks := make([]string, 0, len(segments))

	for _, segment := range segments {
		if segment == "" {
			continue
		}
		runes := []rune(segment)
		for start := 0; start < len(runes); start += limit {
			end := start + limit
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, string(runes[start:end]))
		}
	}
	return chunks
}

// AggregateResults 将多分片执行结果按最高风险规则聚合为单一最终结论。
func AggregateResults(results []*NormalizedResult, latency time.Duration) (*NormalizedResult, error) {
	if len(results) == 0 {
		return nil, errors.New("prompt guard produced no complete result")
	}

	aggregated := &NormalizedResult{
		Decision:        EventPass,
		RiskLevel:       RiskLow,
		Action:          ActionAllow,
		ScannerBackend:  "qwen3guard-openai",
		ScannerVersion:  "qwen3guard",
		PolicyID:        StrategyPriority,
		PolicyVersion:   1,
		Categories:      []string{},
		MatchedScanners: []string{},
		ScannerScores:   make(map[string]float64),
		ScannerEvidence: make(map[string]string),
		ChunkTotal:      len(results),
		LatencyMS:       int(latency.Milliseconds()),
	}

	categoriesMap := make(map[string]struct{})
	matchedMap := make(map[string]struct{})
	unknownMap := make(map[string]struct{})

	for _, res := range results {
		if res == nil {
			return nil, errors.New("prompt guard partial result is nil")
		}

		if severityScore(res.Decision) > severityScore(aggregated.Decision) {
			aggregated.Decision = res.Decision
			aggregated.RiskLevel = res.RiskLevel
			aggregated.Action = res.Action
			aggregated.Safety = res.Safety
			aggregated.GuardEndpointID = res.GuardEndpointID
			aggregated.ScannerBackend = res.ScannerBackend
			aggregated.ScannerVersion = res.ScannerVersion
			aggregated.PolicyID = res.PolicyID
			aggregated.PolicyVersion = res.PolicyVersion
		}

		if aggregated.GuardEndpointID == "" {
			aggregated.GuardEndpointID = res.GuardEndpointID
			if res.ScannerBackend != "" {
				aggregated.ScannerBackend = res.ScannerBackend
			}
			aggregated.ScannerVersion = res.ScannerVersion
		}

		for _, cat := range res.Categories {
			categoriesMap[cat] = struct{}{}
		}
		for _, m := range res.MatchedScanners {
			matchedMap[m] = struct{}{}
		}
		for _, u := range res.UnknownCategories {
			unknownMap[u] = struct{}{}
		}
		for scanner, score := range res.ScannerScores {
			if score > aggregated.ScannerScores[scanner] {
				aggregated.ScannerScores[scanner] = score
			}
		}
		for scanner, ev := range res.ScannerEvidence {
			if _, exists := aggregated.ScannerEvidence[scanner]; !exists {
				aggregated.ScannerEvidence[scanner] = ev
			}
		}
	}

	aggregated.Categories = orderedScannerKeys(categoriesMap)
	aggregated.MatchedScanners = orderedScannerKeys(matchedMap)
	aggregated.UnknownCategories = sortedKeys(unknownMap)

	return aggregated, nil
}

func severityScore(decision string) int {
	switch decision {
	case EventCritical:
		return 3
	case EventFlag:
		return 2
	default:
		return 1
	}
}

// OpenAICompatibleScanner 实现标准 OpenAI Chat Completions 协议的 Qwen3Guard Scanner。
type OpenAICompatibleScanner struct {
	clients sync.Map
}

func NewOpenAICompatibleScanner() *OpenAICompatibleScanner {
	return &OpenAICompatibleScanner{}
}

func (s *OpenAICompatibleScanner) clientFor(endpoint ActiveEndpoint) (*http.Client, error) {
	key := fmt.Sprintf("%s|%s|%d", endpoint.ID, endpoint.BaseURL, endpoint.TimeoutMS)
	if cached, ok := s.clients.Load(key); ok {
		if c, valid := cached.(*http.Client); valid {
			return c, nil
		}
	}

	client, err := NewSecureHTTPClient(endpoint)
	if err != nil {
		return nil, err
	}
	s.clients.Store(key, client)
	return client, nil
}

func (s *OpenAICompatibleScanner) Scan(ctx context.Context, endpoint ActiveEndpoint, chunk string, enabledScanners []string) (*NormalizedResult, error) {
	client, err := s.clientFor(endpoint)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}

	reqURL, err := ChatCompletionsURL(endpoint.BaseURL)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}

	payload := map[string]any{
		"model": endpoint.Model,
		"messages": []map[string]string{
			{"role": "user", "content": chunk},
		},
		"temperature": 0,
		"max_tokens":  64,
		"seed":        42,
	}

	bodyBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Cause: err}
	}

	req.Header.Set("Content-Type", "application/json")
	if endpoint.Token != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		isTimeout := errors.Is(err, context.DeadlineExceeded)
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			isTimeout = true
		}
		return nil, &GuardError{
			Code:      ErrorCodeUnavailable,
			Retryable: true,
			Timeout:   isTimeout,
			Cause:     err,
		}
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, &GuardError{
			Code:       ErrorCodeUnavailable,
			HTTPStatus: resp.StatusCode,
			Retryable:  retryable,
			Cause:      fmt.Errorf("upstream guard http error: %d", resp.StatusCode),
		}
	}

	limited := io.LimitReader(resp.Body, MaxGuardResponseBytes+1)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeUnavailable, Retryable: true, Cause: err}
	}
	if int64(len(respBody)) > MaxGuardResponseBytes {
		return nil, &GuardError{
			Code:      ErrorCodeInvalidResponse,
			Retryable: false,
			Cause:     fmt.Errorf("guard response exceeded maximum allowed bytes: %d", len(respBody)),
		}
	}

	content, err := extractOpenAIContent(respBody)
	if err != nil {
		return nil, &GuardError{Code: ErrorCodeInvalidResponse, Retryable: false, Cause: err}
	}

	result, err := ParseQwen3Guard(content, enabledScanners)
	if err != nil {
		return nil, err
	}
	result.GuardEndpointID = endpoint.ID
	result.ScannerVersion = endpoint.Model

	return result, nil
}

func extractOpenAIContent(body []byte) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content          any    `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &resp); err != nil || len(resp.Choices) == 0 {
		return "", errors.New("invalid openai completion response envelope")
	}

	choice := resp.Choices[0]
	content := choice.Message.Content
	switch v := content.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
				return "", errors.New("empty guard response content: model output reasoning_content instead of standard content, please disable thinking mode or use a non-reasoning model")
			}
			return "", errors.New("empty guard response content")
		}
		return v, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
				return "", errors.New("empty guard response parts: model output reasoning_content instead of standard content, please disable thinking mode or use a non-reasoning model")
			}
			return "", errors.New("empty guard response parts")
		}
		return strings.Join(parts, "\n"), nil
	default:
		if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
			return "", errors.New("unexpected guard response content format: model output reasoning_content instead of standard content, please disable thinking mode or use a non-reasoning model")
		}
		return "", errors.New("unexpected guard response content format")
	}
}

// GuardEvaluator 实现提示词审计核心 Evaluator 接口，负责并发隔离、故障切换与判定聚合。
type GuardEvaluator struct {
	scanner      PromptScanner
	globalSem    chan struct{}
	perNodeLimit int
	nodeMu       sync.Mutex
	nodes        map[string]chan struct{}
}

func NewGuardEvaluator(scanner PromptScanner) *GuardEvaluator {
	return NewGuardEvaluatorWithLimits(scanner, DefaultGlobalConcurrency, DefaultNodeConcurrency)
}

func NewGuardEvaluatorWithLimits(scanner PromptScanner, globalLimit, perNodeLimit int) *GuardEvaluator {
	if globalLimit < 1 {
		globalLimit = DefaultGlobalConcurrency
	}
	if perNodeLimit < 1 {
		perNodeLimit = DefaultNodeConcurrency
	}
	return &GuardEvaluator{
		scanner:      scanner,
		globalSem:    make(chan struct{}, globalLimit),
		perNodeLimit: perNodeLimit,
		nodes:        make(map[string]chan struct{}),
	}
}

func (g *GuardEvaluator) nodeSemaphore(id string) chan struct{} {
	g.nodeMu.Lock()
	defer g.nodeMu.Unlock()
	sem := g.nodes[id]
	if sem == nil {
		sem = make(chan struct{}, g.perNodeLimit)
		g.nodes[id] = sem
	}
	return sem
}

func minimumInputLimit(endpoints []ActiveEndpoint) int {
	limit := DefaultInputLimit
	for i, ep := range endpoints {
		val := ep.InputLimit
		if val <= 0 {
			val = DefaultInputLimit
		}
		if i == 0 || val < limit {
			limit = val
		}
	}
	return limit
}

// Evaluate 执行一次完整的提示词审计评估。
func (g *GuardEvaluator) Evaluate(ctx context.Context, cfg ActiveConfig, snapshot PromptSnapshot) (*Decision, error) {
	if !cfg.Enabled {
		return &Decision{
			Kind:           DecisionAllow,
			HTTPStatus:     200,
			AllowNextStage: true,
		}, nil
	}

	start := time.Now()
	baseFields := map[string]any{
		"request_id":          snapshot.RequestID,
		"user_id":             snapshot.UserID,
		"token_id":            snapshot.TokenID,
		"group":               snapshot.Group,
		"protocol":            snapshot.Protocol,
		"model":               snapshot.Model,
		"stage":               snapshot.Stage,
		"audit_scope":         snapshot.AuditScope,
		"config_version":      cfg.ConfigVersion,
		"input_chars":         snapshot.PromptLength,
		"upstream_dispatched": false,
		"billing_preconsumed": false,
	}
	LogGuardInfo(ctx, EventGuardStarted, baseFields)

	endpoints := cfg.EnabledEndpoints()
	if len(endpoints) == 0 {
		failFields := copyLogFields(baseFields)
		failFields["status"] = "failed"
		failFields["error_code"] = ErrorCodeUnavailable
		failFields["latency_ms"] = time.Since(start).Milliseconds()
		LogGuardWarn(ctx, EventGuardFailed, failFields)
		return nil, &GuardError{
			Code:       ErrorCodeUnavailable,
			HTTPStatus: 503,
			Cause:      errors.New("no usable enabled guard endpoints"),
		}
	}

	// 全局并发信号量控制
	select {
	case g.globalSem <- struct{}{}:
		defer func() { <-g.globalSem }()
	default:
		failFields := copyLogFields(baseFields)
		failFields["status"] = "failed"
		failFields["error_code"] = ErrorCodeUnavailable
		failFields["latency_ms"] = time.Since(start).Milliseconds()
		LogGuardWarn(ctx, EventGuardFailed, failFields)
		return nil, &GuardError{
			Code:       ErrorCodeUnavailable,
			HTTPStatus: 503,
			Cause:      errors.New("global guard bulkhead capacity full"),
		}
	}

	// 整体有界超时
	timeoutMS := endpoints[0].TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = DefaultTimeoutMS
	}
	evalCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	inputLimit := minimumInputLimit(endpoints)
	chunks := SplitRunes(snapshot.ScanText, inputLimit)
	if len(chunks) == 0 {
		allowFields := copyLogFields(baseFields)
		allowFields["status"] = "allowed"
		allowFields["decision"] = DecisionAllow
		allowFields["latency_ms"] = time.Since(start).Milliseconds()
		LogGuardInfo(ctx, EventGuardAllowed, allowFields)
		return &Decision{
			Kind:           DecisionAllow,
			HTTPStatus:     200,
			AllowNextStage: true,
		}, nil
	}

	results := make([]*NormalizedResult, 0, len(chunks))
	for _, chunk := range chunks {
		res, err := g.scanChunk(evalCtx, cfg, endpoints, chunk)
		if err != nil {
			var gErr *GuardError
			code := ErrorCodeUnavailable
			if errors.As(err, &gErr) && gErr.Code != "" {
				code = gErr.Code
			}
			failFields := copyLogFields(baseFields)
			failFields["status"] = "failed"
			failFields["error_code"] = code
			failFields["latency_ms"] = time.Since(start).Milliseconds()
			LogGuardWarn(ctx, EventGuardFailed, failFields)
			return nil, err
		}
		res.ChunkTotal = len(chunks)
		results = append(results, res)

		// 任何一块命中 Block，立即短路终止，不再发起后续请求
		if res.Action == ActionBlock {
			break
		}
	}

	aggregated, err := AggregateResults(results, time.Since(start))
	if err != nil {
		failFields := copyLogFields(baseFields)
		failFields["status"] = "failed"
		failFields["error_code"] = ErrorCodeInvalidResponse
		failFields["latency_ms"] = time.Since(start).Milliseconds()
		LogGuardWarn(ctx, EventGuardFailed, failFields)
		return nil, &GuardError{
			Code:       ErrorCodeInvalidResponse,
			HTTPStatus: 503,
			Cause:      err,
		}
	}
	aggregated.ChunkTotal = len(chunks)

	kind := DecisionAllow
	httpStatus := 200
	if aggregated.Action == ActionWarn {
		kind = DecisionFlag
		httpStatus = 200
	}
	if aggregated.Action == ActionBlock {
		kind = DecisionBlock
		httpStatus = 403
	}

	decision := &Decision{
		Kind:           kind,
		HTTPStatus:     httpStatus,
		Result:         aggregated,
		AllowNextStage: (kind == DecisionAllow || kind == DecisionFlag),
	}
	if kind == DecisionBlock {
		decision.ErrorCode = ErrorCodeBlocked
		blockFields := copyLogFields(baseFields)
		blockFields["status"] = "blocked"
		blockFields["decision"] = kind
		blockFields["risk_level"] = aggregated.RiskLevel
		blockFields["action"] = aggregated.Action
		blockFields["guard_endpoint_id"] = aggregated.GuardEndpointID
		blockFields["chunk_total"] = aggregated.ChunkTotal
		blockFields["latency_ms"] = aggregated.LatencyMS
		blockFields["error_code"] = ErrorCodeBlocked
		LogGuardWarn(ctx, EventGuardBlocked, blockFields)
	} else {
		allowFields := copyLogFields(baseFields)
		allowFields["status"] = "allowed"
		allowFields["decision"] = kind
		allowFields["risk_level"] = aggregated.RiskLevel
		allowFields["action"] = aggregated.Action
		allowFields["guard_endpoint_id"] = aggregated.GuardEndpointID
		allowFields["chunk_total"] = aggregated.ChunkTotal
		allowFields["latency_ms"] = aggregated.LatencyMS
		LogGuardInfo(ctx, EventGuardAllowed, allowFields)
	}

	return decision, nil
}

func copyLogFields(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (g *GuardEvaluator) scanChunk(ctx context.Context, cfg ActiveConfig, endpoints []ActiveEndpoint, chunk string) (*NormalizedResult, error) {
	var lastErr error

	for _, ep := range endpoints {
		sem := g.nodeSemaphore(ep.ID)

		// 单节点并发控制
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return nil, &GuardError{
				Code:       ErrorCodeUnavailable,
				HTTPStatus: 503,
				Retryable:  true,
				Timeout:    true,
				Cause:      ctx.Err(),
			}
		default:
			// 单节点容量耗尽，视作可重试错误切换下一节点
			lastErr = &GuardError{
				Code:       ErrorCodeUnavailable,
				HTTPStatus: 503,
				Retryable:  true,
				Cause:      fmt.Errorf("node %s bulkhead capacity full", ep.ID),
			}
			continue
		}

		res, err := g.scanner.Scan(ctx, ep, chunk, cfg.Scanners)
		<-sem

		if err == nil && res != nil {
			return res, nil
		}

		if err == nil {
			err = &GuardError{
				Code:       ErrorCodeInvalidResponse,
				HTTPStatus: 503,
				Retryable:  false,
				Cause:      errors.New("scanner returned nil result without error"),
			}
		}

		lastErr = err

		// 若错误不可重试（例如 4xx 配置错误、非法响应格式），不得盲目 failover，直接退出
		var guardErr *GuardError
		if errors.As(err, &guardErr) && !guardErr.Retryable {
			return nil, err
		}
	}

	if lastErr == nil {
		lastErr = &GuardError{Code: ErrorCodeUnavailable, HTTPStatus: 503}
	}
	return nil, lastErr
}
