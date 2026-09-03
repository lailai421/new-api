package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/promptaudit"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetPromptAuditConfig 获取当前提示词审计配置及可用扫描器元数据目录。
// 端点令牌已脱敏，响应包含 config_version 供前端 CAS 回填。
func GetPromptAuditConfig(c *gin.Context) {
	manager := promptaudit.GetManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "提示词审计服务未初始化",
		})
		return
	}

	store := promptaudit.GetConfigStore()
	encryptor := promptaudit.GetEncryptor()

	cfg, err := store.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "读取提示词审计配置失败: " + err.Error(),
		})
		return
	}
	if cfg == nil {
		def := promptaudit.DefaultConfig()
		cfg = &def
	}

	publicCfg := cfg.ToPublic(encryptor)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"config":         publicCfg,
			"scanners":       promptaudit.ScannerCatalogDefinitions(),
			"config_version": publicCfg.ConfigVersion,
		},
	})
}

// UpdatePromptAuditConfig 执行基于 expected_config_version 的 CAS 提示词审计配置全量更新。
// 支持单个端点增量更新 Token（明文写入）或通过 delete_token 清空已有 Token。
func UpdatePromptAuditConfig(c *gin.Context) {
	var req dto.PromptAuditConfigUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.ExpectedConfigVersion < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "expected_config_version 必须大于或等于 0",
		})
		return
	}

	store := promptaudit.GetConfigStore()
	encryptor := promptaudit.GetEncryptor()
	manager := promptaudit.GetManager()
	if store == nil || encryptor == nil || manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "提示词审计服务未完全初始化",
		})
		return
	}

	// 读取当前配置作为合并基准
	currentCfg, err := store.Load(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "加载当前配置失败: " + err.Error(),
		})
		return
	}
	if currentCfg == nil {
		def := promptaudit.DefaultConfig()
		currentCfg = &def
	}

	existingEndpoints := make(map[string]promptaudit.Endpoint, len(currentCfg.Endpoints))
	for _, ep := range currentCfg.Endpoints {
		existingEndpoints[ep.ID] = ep
	}

	// 合并更新字段
	if req.Enabled != nil {
		currentCfg.Enabled = *req.Enabled
	}
	if req.LatestTurnOnly != nil {
		currentCfg.LatestTurnOnly = *req.LatestTurnOnly
	}
	if req.StorePassEvents != nil {
		currentCfg.StorePassEvents = *req.StorePassEvents
	}
	if req.AllGroups != nil {
		currentCfg.AllGroups = *req.AllGroups
	}
	if req.Groups != nil {
		currentCfg.Groups = req.Groups
	}
	if req.Scanners != nil {
		currentCfg.Scanners = req.Scanners
	}
	if req.RetentionDays != nil {
		currentCfg.RetentionDays = *req.RetentionDays
	}
	if req.Strategy != "" {
		currentCfg.Strategy = req.Strategy
	}

	if req.Endpoints != nil {
		mergedEndpoints := make([]promptaudit.Endpoint, 0, len(req.Endpoints))
		for _, epReq := range req.Endpoints {
			tokenCiphertext := ""
			if existing, ok := existingEndpoints[epReq.ID]; ok {
				tokenCiphertext = existing.TokenCiphertext
			}
			if epReq.DeleteToken {
				tokenCiphertext = ""
			} else if epReq.Token != "" {
				enc, err := encryptor.EncryptToken(epReq.Token)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": fmt.Sprintf("端点 %s 令牌加密失败: %s", epReq.ID, err.Error()),
					})
					return
				}
				tokenCiphertext = enc
			}
			mergedEndpoints = append(mergedEndpoints, promptaudit.Endpoint{
				ID:              epReq.ID,
				Name:            epReq.Name,
				Protocol:        epReq.Protocol,
				BaseURL:         epReq.BaseURL,
				Model:           epReq.Model,
				TimeoutMS:       epReq.TimeoutMS,
				InputLimit:      epReq.InputLimit,
				Enabled:         epReq.Enabled,
				TokenCiphertext: tokenCiphertext,
			})
		}
		currentCfg.Endpoints = mergedEndpoints
	}

	// 规范化与静态校验
	if err := currentCfg.NormalizeAndValidate(promptaudit.HasStableCryptoSecret()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "配置校验失败: " + err.Error(),
		})
		return
	}

	// 记录操作者用户 ID
	currentCfg.UpdatedBy = c.GetInt("id")

	// CAS 保存
	if err := store.Save(c.Request.Context(), currentCfg, req.ExpectedConfigVersion); err != nil {
		var guardErr *promptaudit.GuardError
		if errors.As(err, &guardErr) && guardErr.Code == promptaudit.ErrorCodeConfigConflict {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "配置版本冲突，请刷新后重试",
				"error": gin.H{
					"code":    promptaudit.ErrorCodeConfigConflict,
					"message": err.Error(),
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "保存配置失败: " + err.Error(),
		})
		return
	}

	// 本地立即重载
	if err := manager.Reload(c.Request.Context()); err != nil {
		common.SysError("failed to reload prompt audit manager after update: " + err.Error())
	}

	recordManageAudit(c, "prompt_audit.config_update", map[string]any{
		"config_version": currentCfg.ConfigVersion,
		"enabled":        currentCfg.Enabled,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置更新成功",
		"data":    currentCfg.ToPublic(encryptor),
	})
}

// GetPromptAuditRuntime 获取当前提示词审计的运行时状态（降级标志、生效版本、端点运行时）。
func GetPromptAuditRuntime(c *gin.Context) {
	manager := promptaudit.GetManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "提示词审计服务未初始化",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    manager.RuntimeState(),
	})
}

// ProbePromptAuditEndpoint 探测指定端点（或临时配置的端点）的连通性与模型契约响应。
// 探测过程不包含敏感业务提示词，绝不回显令牌。
func ProbePromptAuditEndpoint(c *gin.Context) {
	var req dto.PromptAuditProbeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的探测参数: " + err.Error(),
		})
		return
	}

	manager := promptaudit.GetManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "提示词审计服务未初始化",
		})
		return
	}

	var targetEp promptaudit.ActiveEndpoint
	found := false

	if req.EndpointID != "" {
		for _, ep := range manager.Active().Endpoints {
			if ep.ID == req.EndpointID {
				targetEp = ep
				found = true
				break
			}
		}
	}

	if !found {
		// 使用行内临时参数
		if req.BaseURL == "" || req.Model == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "必须指定有效的 endpoint_id 或提供完整的 base_url 与 model",
			})
			return
		}
		timeout := req.TimeoutMS
		if timeout <= 0 {
			timeout = promptaudit.DefaultTimeoutMS
		}
		inputLimit := req.InputLimit
		if inputLimit <= 0 {
			inputLimit = promptaudit.DefaultInputLimit
		}
		targetEp = promptaudit.ActiveEndpoint{
			ID:         "probe-ep",
			Protocol:   promptaudit.ProtocolOpenAICompatible,
			BaseURL:    req.BaseURL,
			Model:      req.Model,
			Token:      req.Token,
			TimeoutMS:  timeout,
			InputLimit: inputLimit,
		}
	}

	scanner := promptaudit.NewOpenAICompatibleScanner()
	start := time.Now()
	_, err := scanner.Scan(c.Request.Context(), targetEp, "探测连通性测试输入文本", []string{"jailbreak"})
	latency := time.Since(start).Milliseconds()

	probeResp := dto.PromptAuditProbeResponse{
		Success:   err == nil,
		LatencyMS: latency,
		Model:     targetEp.Model,
	}

	if err != nil {
		probeResp.Message = err.Error()
		probeResp.ErrorCode = "probe_failed"
	} else {
		probeResp.Message = "探测成功"
	}

	recordManageAudit(c, "prompt_audit.endpoint_probe", map[string]any{
		"endpoint_id": targetEp.ID,
		"success":     probeResp.Success,
		"latency_ms":  latency,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    probeResp,
	})
}

// GetPromptAuditEvents 分页与多维筛选查询提示词审计事件列表。
// 响应数据显式忽略 full_prompt 与 full_prompt_ciphertext。
func GetPromptAuditEvents(c *gin.Context) {
	var req dto.PromptAuditEventListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "查询参数解析失败: " + err.Error(),
		})
		return
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}

	filter := model.PromptAuditEventQueryFilter{
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		UserID:          req.UserID,
		TokenID:         req.TokenID,
		Group:           req.Group,
		Model:           req.Model,
		Protocol:        req.Protocol,
		Decision:        req.Decision,
		RiskLevel:       req.RiskLevel,
		Category:        req.Category,
		GuardEndpointID: req.GuardEndpointID,
		RequestID:       req.RequestID,
		PromptHash:      req.PromptHash,
		Page:            page,
		PageSize:        pageSize,
	}

	events, total, err := model.GetPromptAuditEvents(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "查询审计事件失败: " + err.Error(),
		})
		return
	}

	items := make([]dto.PromptAuditEventItem, 0, len(events))
	for _, ev := range events {
		var categories []string
		if ev.CategoriesJSON != "" {
			_ = common.Unmarshal([]byte(ev.CategoriesJSON), &categories)
		}
		var matchedScanners []string
		if ev.MatchedScannersJSON != "" {
			_ = common.Unmarshal([]byte(ev.MatchedScannersJSON), &matchedScanners)
		}

		items = append(items, dto.PromptAuditEventItem{
			ID:                ev.Id,
			RequestID:         ev.RequestId,
			UserID:            ev.UserId,
			UsernameSnapshot:  ev.UsernameSnapshot,
			UserEmailSnapshot: ev.UserEmailSnapshot,
			TokenID:           ev.TokenId,
			TokenNameSnapshot: ev.TokenNameSnapshot,
			Group:             ev.Group,
			ChannelID:         ev.ChannelId,
			ChannelType:       ev.ChannelType,
			RequestPath:       ev.RequestPath,
			Protocol:          ev.Protocol,
			Model:             ev.Model,
			Stage:             ev.Stage,
			AuditScope:        ev.AuditScope,
			PromptHash:        ev.PromptHash,
			RedactedPreview:   ev.RedactedPreview,
			PromptLength:      ev.PromptLength,
			MessageCount:      ev.MessageCount,
			Decision:          ev.Decision,
			RiskLevel:         ev.RiskLevel,
			Action:            ev.Action,
			Categories:        categories,
			MatchedScanners:   matchedScanners,
			ScannerBackend:    ev.ScannerBackend,
			ScannerVersion:    ev.ScannerVersion,
			GuardEndpointID:   ev.GuardEndpointId,
			PolicyID:          ev.PolicyId,
			PolicyVersion:     ev.PolicyVersion,
			ConfigVersion:     ev.ConfigVersion,
			ChunkTotal:        ev.ChunkTotal,
			LatencyMS:         ev.LatencyMS,
			ErrorCode:         ev.ErrorCode,
			CreatedAt:         ev.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetPromptAuditEvent 查询单条审计事件详情，解密完整提示词，并设置 Cache-Control: no-store。
func GetPromptAuditEvent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的事件 ID",
		})
		return
	}

	event, err := model.GetPromptAuditEventByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "审计事件不存在",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "读取审计事件失败: " + err.Error(),
		})
		return
	}

	encryptor := promptaudit.GetEncryptor()
	fullPrompt := ""
	if event.FullPromptCiphertext != "" && encryptor != nil {
		decrypted, err := encryptor.DecryptPrompt(string(event.FullPromptCiphertext))
		if err != nil {
			fullPrompt = "[解密失败: 密文损坏或密钥不匹配]"
		} else {
			fullPrompt = decrypted
		}
	}

	var categories []string
	if event.CategoriesJSON != "" {
		_ = common.Unmarshal([]byte(event.CategoriesJSON), &categories)
	}
	var matchedScanners []string
	if event.MatchedScannersJSON != "" {
		_ = common.Unmarshal([]byte(event.MatchedScannersJSON), &matchedScanners)
	}
	var scannerScores map[string]any
	if event.ScannerScoresJSON != "" {
		_ = common.Unmarshal([]byte(event.ScannerScoresJSON), &scannerScores)
	}
	var scannerEvidence map[string]any
	if event.ScannerEvidenceJSON != "" {
		_ = common.Unmarshal([]byte(event.ScannerEvidenceJSON), &scannerEvidence)
	}

	resp := dto.PromptAuditEventDetailResponse{
		PromptAuditEventItem: dto.PromptAuditEventItem{
			ID:                event.Id,
			RequestID:         event.RequestId,
			UserID:            event.UserId,
			UsernameSnapshot:  event.UsernameSnapshot,
			UserEmailSnapshot: event.UserEmailSnapshot,
			TokenID:           event.TokenId,
			TokenNameSnapshot: event.TokenNameSnapshot,
			Group:             event.Group,
			ChannelID:         event.ChannelId,
			ChannelType:       event.ChannelType,
			RequestPath:       event.RequestPath,
			Protocol:          event.Protocol,
			Model:             event.Model,
			Stage:             event.Stage,
			AuditScope:        event.AuditScope,
			PromptHash:        event.PromptHash,
			RedactedPreview:   event.RedactedPreview,
			PromptLength:      event.PromptLength,
			MessageCount:      event.MessageCount,
			Decision:          event.Decision,
			RiskLevel:         event.RiskLevel,
			Action:            event.Action,
			Categories:        categories,
			MatchedScanners:   matchedScanners,
			ScannerBackend:    event.ScannerBackend,
			ScannerVersion:    event.ScannerVersion,
			GuardEndpointID:   event.GuardEndpointId,
			PolicyID:          event.PolicyId,
			PolicyVersion:     event.PolicyVersion,
			ConfigVersion:     event.ConfigVersion,
			ChunkTotal:        event.ChunkTotal,
			LatencyMS:         event.LatencyMS,
			ErrorCode:         event.ErrorCode,
			CreatedAt:         event.CreatedAt,
		},
		FullPrompt:      fullPrompt,
		ScannerScores:   scannerScores,
		ScannerEvidence: scannerEvidence,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

// DeletePromptAuditEvent 删除单条审计事件。
func DeletePromptAuditEvent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的事件 ID",
		})
		return
	}

	if err := model.DeletePromptAuditEventByID(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "删除审计事件失败: " + err.Error(),
		})
		return
	}

	recordManageAudit(c, "prompt_audit.event_delete", map[string]any{
		"id": id,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// BatchDeletePromptAuditEvents 按 ID 列表批量删除事件（最多 500 条）。
func BatchDeletePromptAuditEvents(c *gin.Context) {
	var req dto.PromptAuditBatchDeleteRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "ID 列表不能为空",
		})
		return
	}
	if len(req.IDs) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "单次批量删除不能超过 500 条",
		})
		return
	}

	deleted, err := model.BatchDeletePromptAuditEvents(c.Request.Context(), req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "批量删除失败: " + err.Error(),
		})
		return
	}

	recordManageAudit(c, "prompt_audit.event_batch_delete", map[string]any{
		"count":           deleted,
		"requested_count": len(req.IDs),
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "批量删除成功",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}
