package promptaudit

import (
	"context"
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// CheckRelayRequest 在业务上游调用、敏感词检查、token 估算和预扣费之前执行 HTTP Relay 请求的同步审计门禁。
func CheckRelayRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo, request dto.Request) *types.NewAPIError {
	if relayInfo != nil && relayInfo.RelayFormat == types.RelayFormatOpenAIRealtime {
		return nil
	}

	cfg, gErr := CheckAuditClientAccess(c)
	if gErr != nil {
		return NewRelayAuditError(gErr.Code, gErr.HTTPStatus)
	}
	if !cfg.Enabled {
		return nil
	}

	group := extractGroup(c, relayInfo)
	if !cfg.MatchesGroup(group) {
		return nil
	}

	segments, protocol, modelName, err := ExtractRelayRequest(request, c)
	if errors.Is(err, ErrNoPrompt) {
		return nil
	}
	if err != nil {
		return NewRelayAuditError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	snapshot, err := BuildPromptSnapshot(c, relayInfo, protocol, modelName, segments, cfg.LatestTurnOnly)
	if err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return nil
		}
		return NewRelayAuditError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	evaluator := GetEvaluator()
	if evaluator == nil {
		return NewRelayAuditError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	ctx := getRequestContext(c)
	decision, evalErr := evaluator.Evaluate(ctx, cfg, snapshot)
	if evalErr != nil {
		code := ErrorCodeUnavailable
		latencyMS := 0
		var gErr *GuardError
		if errors.As(evalErr, &gErr) {
			if gErr.Code != "" {
				code = gErr.Code
			}
			latencyMS = gErr.LatencyMS
		}
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, &Decision{Kind: DecisionUnavailable, ErrorCode: code, LatencyMS: latencyMS}, true)
		}
		return NewRelayAuditError(code, http.StatusServiceUnavailable)
	}

	if decision != nil && decision.Kind == DecisionBlock {
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, decision, true)
		}
		return NewRelayAuditError(ErrorCodeBlocked, http.StatusForbidden)
	}

	if decision != nil && !decision.AllowNextStage {
		return NewRelayAuditError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	if cfg.StorePassEvents {
		if decision != nil && decision.FromCache {
			return nil
		}
		store := GetEventStore()
		if store == nil {
			return NewRelayAuditError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
		if err := store.Record(ctx, snapshot, decision, true); err != nil {
			return NewRelayAuditError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
	}

	return nil
}

// CheckMidjourneyRequest 在业务提交前执行 Midjourney 请求的同步审计门禁。
func CheckMidjourneyRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo) *taskdto.MidjourneyResponse {
	cfg, gErr := CheckAuditClientAccess(c)
	if gErr != nil {
		return toMjError(gErr.Code, gErr.HTTPStatus)
	}
	if !cfg.Enabled {
		return nil
	}

	group := extractGroup(c, relayInfo)
	if !cfg.MatchesGroup(group) {
		return nil
	}

	segments, modelName, err := ExtractMidjourneyRequest(c, relayInfo)
	if errors.Is(err, ErrNoPrompt) {
		return nil
	}
	if err != nil {
		return toMjError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	snapshot, err := BuildPromptSnapshot(c, relayInfo, "midjourney", modelName, segments, cfg.LatestTurnOnly)
	if err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return nil
		}
		return toMjError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	evaluator := GetEvaluator()
	if evaluator == nil {
		return toMjError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	ctx := getRequestContext(c)
	decision, evalErr := evaluator.Evaluate(ctx, cfg, snapshot)
	if evalErr != nil {
		code := ErrorCodeUnavailable
		latencyMS := 0
		var gErr *GuardError
		if errors.As(evalErr, &gErr) {
			if gErr.Code != "" {
				code = gErr.Code
			}
			latencyMS = gErr.LatencyMS
		}
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, &Decision{Kind: DecisionUnavailable, ErrorCode: code, LatencyMS: latencyMS}, true)
		}
		return toMjError(code, http.StatusServiceUnavailable)
	}

	if decision != nil && decision.Kind == DecisionBlock {
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, decision, true)
		}
		return toMjError(ErrorCodeBlocked, http.StatusForbidden)
	}

	if decision != nil && !decision.AllowNextStage {
		return toMjError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	if cfg.StorePassEvents {
		if decision != nil && decision.FromCache {
			return nil
		}
		store := GetEventStore()
		if store == nil {
			return toMjError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
		if err := store.Record(ctx, snapshot, decision, true); err != nil {
			return toMjError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
	}

	return nil
}

func getRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func extractGroup(c *gin.Context, relayInfo *relaycommon.RelayInfo) string {
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
	return group
}

func auditErrorMessage(code string) string {
	if code == ErrorCodeCodexCLIRequired {
		return CodexCLIRequiredMessage
	}
	return code
}

// NewRelayAuditError 将审计错误包装为带 SkipRetry 的 Relay 错误，供 HTTP 与 Realtime 握手共用。
func NewRelayAuditError(code string, statusCode int) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(auditErrorMessage(code)),
		types.ErrorCode(code),
		statusCode,
		types.ErrOptionWithSkipRetry(),
	)
}

func toMjError(code string, statusCode int) *taskdto.MidjourneyResponse {
	return &taskdto.MidjourneyResponse{
		Code:        statusCode,
		Description: auditErrorMessage(code),
		Result:      code,
	}
}

// ContextKeyTaskAuditDone 标记该请求已完成提示词审计门禁，避免重复执行。
const ContextKeyTaskAuditDone = "prompt_audit_done"

func toTaskError(code string, statusCode int) *taskdto.TaskError {
	return &taskdto.TaskError{
		Code:       code,
		Message:    auditErrorMessage(code),
		StatusCode: statusCode,
		LocalError: true,
	}
}

// CheckTaskRequest 在业务 Task 提交前执行同步审计门禁。
func CheckTaskRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo, auditMeta TaskAuditMeta) *taskdto.TaskError {
	if c != nil && c.GetBool(ContextKeyTaskAuditDone) {
		return nil
	}

	cfg, gErr := CheckAuditClientAccess(c)
	if gErr != nil {
		return toTaskError(gErr.Code, gErr.HTTPStatus)
	}
	if !cfg.Enabled {
		return nil
	}

	group := extractGroup(c, relayInfo)
	if !cfg.MatchesGroup(group) {
		return nil
	}

	var taskRequestBody any
	if c != nil {
		if reqVal, exists := c.Get("task_request"); exists {
			taskRequestBody = reqVal
		}
	}

	segments, err := ExtractTaskRequest(taskRequestBody, auditMeta)
	if errors.Is(err, ErrNoPrompt) {
		return nil
	}
	if err != nil {
		return toTaskError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	modelName := ""
	if relayInfo != nil {
		modelName = relayInfo.OriginModelName
	}
	if modelName == "" && c != nil {
		modelName = c.GetString("resolved_task_model")
	}

	snapshot, err := BuildPromptSnapshot(c, relayInfo, "task", modelName, segments, cfg.LatestTurnOnly)
	if err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return nil
		}
		return toTaskError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	evaluator := GetEvaluator()
	if evaluator == nil {
		return toTaskError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	ctx := getRequestContext(c)
	decision, evalErr := evaluator.Evaluate(ctx, cfg, snapshot)
	if evalErr != nil {
		code := ErrorCodeUnavailable
		latencyMS := 0
		var gErr *GuardError
		if errors.As(evalErr, &gErr) {
			if gErr.Code != "" {
				code = gErr.Code
			}
			latencyMS = gErr.LatencyMS
		}
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, &Decision{Kind: DecisionUnavailable, ErrorCode: code, LatencyMS: latencyMS}, true)
		}
		return toTaskError(code, http.StatusServiceUnavailable)
	}

	if decision != nil && decision.Kind == DecisionBlock {
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, decision, true)
		}
		return toTaskError(ErrorCodeBlocked, http.StatusForbidden)
	}

	if decision != nil && !decision.AllowNextStage {
		return toTaskError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	if cfg.StorePassEvents {
		if decision != nil && decision.FromCache {
			if c != nil {
				c.Set(ContextKeyTaskAuditDone, true)
			}
			return nil
		}
		store := GetEventStore()
		if store == nil {
			return toTaskError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
		if err := store.Record(ctx, snapshot, decision, true); err != nil {
			return toTaskError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
	}

	if c != nil {
		c.Set(ContextKeyTaskAuditDone, true)
	}
	return nil
}

// CheckTaskPluginProtocolRequest 在 Responses Bridge 提交前执行同步审计门禁。
func CheckTaskPluginProtocolRequest(c *gin.Context, relayInfo *relaycommon.RelayInfo, auditMeta TaskAuditMeta) *taskdto.TaskError {
	if c != nil && c.GetBool(ContextKeyTaskAuditDone) {
		return nil
	}

	cfg, gErr := CheckAuditClientAccess(c)
	if gErr != nil {
		return toTaskError(gErr.Code, gErr.HTTPStatus)
	}
	if !cfg.Enabled {
		return nil
	}

	group := extractGroup(c, relayInfo)
	if !cfg.MatchesGroup(group) {
		return nil
	}

	var taskRequestBody any
	if c != nil {
		if reqVal, exists := c.Get("task_request"); exists {
			taskRequestBody = reqVal
		}
	}

	segments, err := ExtractTaskPluginResponsesRequest(c, taskRequestBody, auditMeta)
	if errors.Is(err, ErrNoPrompt) {
		return nil
	}
	if err != nil {
		return toTaskError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	modelName := ""
	if relayInfo != nil {
		modelName = relayInfo.OriginModelName
	}
	if modelName == "" && c != nil {
		modelName = c.GetString("resolved_task_model")
	}

	snapshot, err := BuildPromptSnapshot(c, relayInfo, "openai_responses", modelName, segments, cfg.LatestTurnOnly)
	if err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return nil
		}
		return toTaskError(ErrorCodeUnsupportedProtocol, http.StatusServiceUnavailable)
	}

	evaluator := GetEvaluator()
	if evaluator == nil {
		return toTaskError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	ctx := getRequestContext(c)
	decision, evalErr := evaluator.Evaluate(ctx, cfg, snapshot)
	if evalErr != nil {
		code := ErrorCodeUnavailable
		latencyMS := 0
		var gErr *GuardError
		if errors.As(evalErr, &gErr) {
			if gErr.Code != "" {
				code = gErr.Code
			}
			latencyMS = gErr.LatencyMS
		}
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, &Decision{Kind: DecisionUnavailable, ErrorCode: code, LatencyMS: latencyMS}, true)
		}
		return toTaskError(code, http.StatusServiceUnavailable)
	}

	if decision != nil && decision.Kind == DecisionBlock {
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, decision, true)
		}
		return toTaskError(ErrorCodeBlocked, http.StatusForbidden)
	}

	if decision != nil && !decision.AllowNextStage {
		return toTaskError(ErrorCodeUnavailable, http.StatusServiceUnavailable)
	}

	if cfg.StorePassEvents {
		if decision != nil && decision.FromCache {
			if c != nil {
				c.Set(ContextKeyTaskAuditDone, true)
			}
			return nil
		}
		store := GetEventStore()
		if store == nil {
			return toTaskError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
		if err := store.Record(ctx, snapshot, decision, true); err != nil {
			return toTaskError(ErrorCodeRecordFailed, http.StatusServiceUnavailable)
		}
	}

	if c != nil {
		c.Set(ContextKeyTaskAuditDone, true)
	}
	return nil
}
