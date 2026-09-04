package promptaudit

import (
	"context"
	"errors"
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// RealtimeAuditResult 封装 Realtime 单帧事件的审计门禁结果
type RealtimeAuditResult struct {
	Allowed    bool
	IsNoPrompt bool
	IsBlock    bool
	IsError    bool
	ErrorCode  string
	StatusCode int
	EventType  string
	Decision   *Decision
}

// ShouldAuditRealtime 判断当前 Realtime 请求是否需要进入审计门禁（启用且命中分组，或配置降级）
func ShouldAuditRealtime(c *gin.Context, relayInfo *relaycommon.RelayInfo) bool {
	mgr := GetManager()
	if mgr == nil {
		return false
	}
	// 配置降级时必须进入门禁以触发失败关闭
	if mgr.IsDegraded() {
		return true
	}
	cfg := mgr.Active()
	if !cfg.Enabled {
		return false
	}
	group := extractGroup(c, relayInfo)
	return cfg.MatchesGroup(group)
}

// CheckRealtimeEvent 对客户端上行 WebSocket 帧执行同步门禁评估。
// 遵循失败关闭原则：配置降级、协议未知/非法、Guard 异常及必要事件写入失败均返回对应错误码。
func CheckRealtimeEvent(ctx context.Context, c *gin.Context, relayInfo *relaycommon.RelayInfo, rawMessage []byte) RealtimeAuditResult {
	mgr := GetManager()
	if mgr == nil {
		return RealtimeAuditResult{Allowed: true}
	}

	if mgr.IsDegraded() {
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  ErrorCodeConfigDegraded,
			StatusCode: http.StatusServiceUnavailable,
		}
	}

	cfg := mgr.Active()
	if !cfg.Enabled {
		return RealtimeAuditResult{Allowed: true}
	}

	group := extractGroup(c, relayInfo)
	if !cfg.MatchesGroup(group) {
		return RealtimeAuditResult{Allowed: true}
	}

	segments, eventType, err := ExtractRealtimeEvent(rawMessage)
	if errors.Is(err, ErrNoPrompt) {
		return RealtimeAuditResult{
			Allowed:    true,
			IsNoPrompt: true,
			EventType:  eventType,
		}
	}
	if err != nil {
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  ErrorCodeUnsupportedProtocol,
			StatusCode: http.StatusServiceUnavailable,
			EventType:  eventType,
		}
	}

	modelName := ""
	if relayInfo != nil {
		modelName = relayInfo.OriginModelName
	}

	snapshot, err := BuildPromptSnapshot(c, relayInfo, "openai_realtime", modelName, segments, cfg.LatestTurnOnly)
	if err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return RealtimeAuditResult{
				Allowed:    true,
				IsNoPrompt: true,
				EventType:  eventType,
			}
		}
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  ErrorCodeUnsupportedProtocol,
			StatusCode: http.StatusServiceUnavailable,
			EventType:  eventType,
		}
	}
	snapshot.Stage = eventType

	evaluator := GetEvaluator()
	if evaluator == nil {
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  ErrorCodeUnavailable,
			StatusCode: http.StatusServiceUnavailable,
			EventType:  eventType,
		}
	}

	if ctx == nil {
		ctx = getRequestContext(c)
	}

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
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  code,
			StatusCode: http.StatusServiceUnavailable,
			EventType:  eventType,
		}
	}

	if decision != nil && decision.Kind == DecisionBlock {
		store := GetEventStore()
		if store != nil {
			if err := store.Record(ctx, snapshot, decision, true); err != nil {
				// 必要事件落库失败，升级为失败关闭
				return RealtimeAuditResult{
					Allowed:    false,
					IsError:    true,
					ErrorCode:  ErrorCodeRecordFailed,
					StatusCode: http.StatusServiceUnavailable,
					EventType:  eventType,
				}
			}
		}
		return RealtimeAuditResult{
			Allowed:    false,
			IsBlock:    true,
			ErrorCode:  ErrorCodeBlocked,
			StatusCode: http.StatusForbidden,
			EventType:  eventType,
			Decision:   decision,
		}
	}

	if decision != nil && !decision.AllowNextStage {
		store := GetEventStore()
		if store != nil {
			_ = store.Record(ctx, snapshot, decision, true)
		}
		return RealtimeAuditResult{
			Allowed:    false,
			IsError:    true,
			ErrorCode:  ErrorCodeUnavailable,
			StatusCode: http.StatusServiceUnavailable,
			EventType:  eventType,
			Decision:   decision,
		}
	}

	if cfg.StorePassEvents {
		store := GetEventStore()
		if store == nil {
			return RealtimeAuditResult{
				Allowed:    false,
				IsError:    true,
				ErrorCode:  ErrorCodeRecordFailed,
				StatusCode: http.StatusServiceUnavailable,
				EventType:  eventType,
			}
		}
		if err := store.Record(ctx, snapshot, decision, true); err != nil {
			return RealtimeAuditResult{
				Allowed:    false,
				IsError:    true,
				ErrorCode:  ErrorCodeRecordFailed,
				StatusCode: http.StatusServiceUnavailable,
				EventType:  eventType,
			}
		}
	}

	return RealtimeAuditResult{
		Allowed:   true,
		EventType: eventType,
		Decision:  decision,
	}
}
