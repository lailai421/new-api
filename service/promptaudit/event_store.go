package promptaudit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// GormEventStore 实现 core EventStore 接口，将判定事件落库至主库 prompt_audit_events 表。
type GormEventStore struct {
	encryptor Encryptor
}

// NewGormEventStore 创建 GormEventStore 实例。
func NewGormEventStore(encryptor Encryptor) *GormEventStore {
	return &GormEventStore{
		encryptor: encryptor,
	}
}

// Record 根据 Decision 和 storePassEvents 规则将审计事件持久化。
// 规则：
// 1. Block、Unavailable、Invalid 以及携带错误码的失败事件必须写库；
// 2. Allow、Flag 事件当且仅当 storePassEvents == true 时写库；
// 3. storePassEvents == false 时跳过 Allow/Flag 落库直接返回 nil；
// 4. 需要写库但加密或数据库写入失败时，向调用方返回 ErrorCodeRecordFailed（503），保证失败关闭。
func (s *GormEventStore) Record(
	ctx context.Context,
	snapshot PromptSnapshot,
	decision *Decision,
	storePassEvents bool,
) error {
	shouldRecord := false
	if decision == nil {
		shouldRecord = true
	} else if decision.Kind == DecisionBlock || decision.Kind == DecisionUnavailable || decision.Kind == DecisionInvalid || decision.ErrorCode != "" {
		shouldRecord = true
	} else if storePassEvents {
		shouldRecord = true
	}

	if !shouldRecord {
		return nil
	}

	if s.encryptor == nil {
		return &GuardError{
			Code:       ErrorCodeRecordFailed,
			HTTPStatus: http.StatusServiceUnavailable,
			Cause:      errors.New("prompt audit encryptor not configured"),
		}
	}

	// 1. 完整提示词应用层加密
	ciphertext, err := s.encryptor.EncryptPrompt(snapshot.FullPrompt)
	if err != nil {
		return &GuardError{
			Code:       ErrorCodeRecordFailed,
			HTTPStatus: http.StatusServiceUnavailable,
			Cause:      fmt.Errorf("encrypt full prompt failed: %w", err),
		}
	}

	// 2. 提取并序列化判定结果字段
	decisionKind := ""
	riskLevel := ""
	action := ""
	scannerBackend := ""
	scannerVersion := ""
	guardEndpointID := ""
	policyID := ""
	policyVersion := 0
	chunkTotal := 0
	latencyMS := int64(0)
	errorCode := ""

	categoriesJSON := "[]"
	matchedScannersJSON := "[]"
	scannerScoresJSON := "{}"
	scannerEvidenceJSON := "{}"

	if decision != nil {
		decisionKind = string(decision.Kind)
		errorCode = decision.ErrorCode

		if decision.Result != nil {
			res := decision.Result
			riskLevel = res.RiskLevel
			action = res.Action
			scannerBackend = res.ScannerBackend
			scannerVersion = res.ScannerVersion
			guardEndpointID = res.GuardEndpointID
			policyID = res.PolicyID
			policyVersion = res.PolicyVersion
			chunkTotal = res.ChunkTotal
			latencyMS = int64(res.LatencyMS)

			if len(res.Categories) > 0 {
				if b, err := common.Marshal(res.Categories); err == nil {
					categoriesJSON = string(b)
				}
			}
			if len(res.MatchedScanners) > 0 {
				if b, err := common.Marshal(res.MatchedScanners); err == nil {
					matchedScannersJSON = string(b)
				}
			}
			if len(res.ScannerScores) > 0 {
				if b, err := common.Marshal(res.ScannerScores); err == nil {
					scannerScoresJSON = string(b)
				}
			}
			if len(res.ScannerEvidence) > 0 {
				if b, err := common.Marshal(res.ScannerEvidence); err == nil {
					scannerEvidenceJSON = string(b)
				}
			}
		}
	}

	// 3. 构建 GORM 持久化模型（ScanText 绝不落库，密文置入 FullPromptCiphertext）
	event := &model.PromptAuditEvent{
		RequestId:            snapshot.RequestID,
		UserId:               snapshot.UserID,
		UsernameSnapshot:     snapshot.UsernameSnapshot,
		UserEmailSnapshot:    snapshot.UserEmailSnapshot,
		TokenId:              snapshot.TokenID,
		TokenNameSnapshot:    snapshot.TokenNameSnapshot,
		Group:                snapshot.Group,
		ChannelId:            snapshot.ChannelID,
		ChannelType:          snapshot.ChannelType,
		RequestPath:          snapshot.RequestPath,
		Protocol:             snapshot.Protocol,
		Model:                snapshot.Model,
		Stage:                snapshot.Stage,
		AuditScope:           snapshot.AuditScope,
		PromptHash:           snapshot.PromptHash,
		RedactedPreview:      snapshot.RedactedPreview,
		FullPromptCiphertext: model.PromptCiphertext(ciphertext),
		PromptLength:         snapshot.PromptLength,
		MessageCount:         snapshot.MessageCount,
		Decision:             decisionKind,
		RiskLevel:            riskLevel,
		Action:               action,
		CategoriesJSON:       categoriesJSON,
		MatchedScannersJSON:  matchedScannersJSON,
		ScannerScoresJSON:    scannerScoresJSON,
		ScannerEvidenceJSON:  scannerEvidenceJSON,
		ScannerBackend:       scannerBackend,
		ScannerVersion:       scannerVersion,
		GuardEndpointId:      guardEndpointID,
		PolicyId:             policyID,
		PolicyVersion:        policyVersion,
		ChunkTotal:           chunkTotal,
		LatencyMS:            latencyMS,
		ErrorCode:            errorCode,
		CreatedAt:            time.Now().Unix(),
	}

	if err := model.CreatePromptAuditEvent(ctx, event); err != nil {
		return &GuardError{
			Code:       ErrorCodeRecordFailed,
			HTTPStatus: http.StatusServiceUnavailable,
			Cause:      fmt.Errorf("insert prompt audit event failed: %w", err),
		}
	}

	return nil
}
