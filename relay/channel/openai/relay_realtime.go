package openai

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/promptaudit"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	// 首个文本事件前允许缓存的最大控制/音频帧数与最大总字节数
	maxRealtimeBufferedFrames = 128
	maxRealtimeBufferedBytes  = 4 * 1024 * 1024 // 4MB

	// 单帧提示词审计超时时间
	realtimeFrameAuditTimeout = 5 * time.Second
)

// safeWebSocketWriter 保证 gorilla/websocket 单连接同一时刻仅有一个并发写入器
type safeWebSocketWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *safeWebSocketWriter) WriteTextMessage(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn == nil {
		return errors.New("websocket connection is closed or nil")
	}
	return w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *safeWebSocketWriter) SetConn(conn *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn = conn
}

func (w *safeWebSocketWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		err := w.conn.Close()
		w.conn = nil
		return err
	}
	return nil
}

// OpenaiRealtimeHandler 负责 OpenAI Realtime WebSocket 的全生命周期转发。
// 在提示词审计开启且命中分组时，执行严格的延迟上游连接与逐帧门禁。
func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil {
		return types.NewError(fmt.Errorf("invalid client websocket connection"), types.ErrorCodeBadResponse), nil
	}

	shouldAudit := promptaudit.ShouldAuditRealtime(c, info)
	if !shouldAudit && info.TargetWs == nil {
		return types.NewError(fmt.Errorf("target websocket is nil when audit is not active"), types.ErrorCodeBadResponse), nil
	}
	if shouldAudit && info.TargetWs == nil && info.TargetWsDialer == nil {
		return types.NewError(fmt.Errorf("target websocket dialer is nil when audit is active"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientWriter := &safeWebSocketWriter{conn: info.ClientWs}
	targetWriter := &safeWebSocketWriter{conn: info.TargetWs}

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	var closeOnce sync.Once
	closeAll := func() {
		closeOnce.Do(func() {
			_ = clientWriter.Close()
			_ = targetWriter.Close()
		})
	}
	defer closeAll()

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}
	var usageMu sync.Mutex

	upstreamConnected := !shouldAudit
	var bufferedMessages [][]byte
	var bufferedBytes int

	// 启动上游读取协程的闭包函数（至多调用一次）
	var startTargetReaderOnce sync.Once
	startTargetReader := func(tConn *websocket.Conn) {
		startTargetReaderOnce.Do(func() {
			gopool.Go(func() {
				defer func() {
					if r := recover(); r != nil {
						select {
						case errChan <- fmt.Errorf("panic in target reader: %v", r):
						default:
						}
					}
				}()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						_, message, err := tConn.ReadMessage()
						if err != nil {
							if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
								select {
								case errChan <- fmt.Errorf("error reading from target: %v", err):
								default:
								}
							}
							close(targetClosed)
							return
						}

						info.SetFirstResponseTime()
						realtimeEvent := &dto.RealtimeEvent{}
						if err := common.Unmarshal(message, realtimeEvent); err != nil {
							select {
							case errChan <- fmt.Errorf("error unmarshalling upstream message: %v", err):
							default:
							}
							return
						}

						usageMu.Lock()
						if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
							realtimeUsage := realtimeEvent.Response.Usage
							if realtimeUsage != nil {
								usage.TotalTokens += realtimeUsage.TotalTokens
								usage.InputTokens += realtimeUsage.InputTokens
								usage.OutputTokens += realtimeUsage.OutputTokens
								usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
								usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
								usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
								usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
								usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
								err := preConsumeUsage(c, info, usage, sumUsage)
								if err != nil {
									usageMu.Unlock()
									select {
									case errChan <- fmt.Errorf("error consume usage: %v", err):
									default:
									}
									return
								}
								usage = &dto.RealtimeUsage{}
								localUsage = &dto.RealtimeUsage{}
							} else {
								textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
								if err != nil {
									usageMu.Unlock()
									select {
									case errChan <- fmt.Errorf("error counting text token: %v", err):
									default:
									}
									return
								}
								logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
								localUsage.TotalTokens += textToken + audioToken
								info.IsFirstRequest = false
								localUsage.InputTokens += textToken + audioToken
								localUsage.InputTokenDetails.TextTokens += textToken
								localUsage.InputTokenDetails.AudioTokens += audioToken
								err = preConsumeUsage(c, info, localUsage, sumUsage)
								if err != nil {
									usageMu.Unlock()
									select {
									case errChan <- fmt.Errorf("error consume usage: %v", err):
									default:
									}
									return
								}
								localUsage = &dto.RealtimeUsage{}
							}
						} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
							realtimeSession := realtimeEvent.Session
							if realtimeSession != nil {
								info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
								info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
							}
						} else {
							textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
							if err != nil {
								usageMu.Unlock()
								select {
								case errChan <- fmt.Errorf("error counting text token: %v", err):
								default:
								}
								return
							}
							localUsage.TotalTokens += textToken + audioToken
							localUsage.OutputTokens += textToken + audioToken
							localUsage.OutputTokenDetails.TextTokens += textToken
							localUsage.OutputTokenDetails.AudioTokens += audioToken
						}
						usageMu.Unlock()

						if err := clientWriter.WriteTextMessage(message); err != nil {
							select {
							case errChan <- fmt.Errorf("error writing to client: %v", err):
							default:
							}
							return
						}
					}
				}
			})
		})
	}

	// 若无需审计，现有上游连接已经建立，立即启动上游读协程
	if upstreamConnected && info.TargetWs != nil {
		startTargetReader(info.TargetWs)
	}

	// 客户端读取协程
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case errChan <- fmt.Errorf("panic in client reader: %v", r):
				default:
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, message, err := info.ClientWs.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						select {
						case errChan <- fmt.Errorf("error reading from client: %v", err):
						default:
						}
					}
					close(clientClosed)
					return
				}

				// 提取基础 event_id 供本地错误构造使用
				var baseEvt struct {
					EventID string `json:"event_id"`
					Type    string `json:"type"`
				}
				_ = common.Unmarshal(message, &baseEvt)
				eventID := baseEvt.EventID
				if eventID == "" {
					eventID = helper.GetLocalRealtimeID(c)
				}

				// 审计已开启状态下的逐帧处理与延迟连接控制
				if shouldAudit {
					frameCtx, frameCancel := context.WithTimeout(ctx, realtimeFrameAuditTimeout)
					auditRes := promptaudit.CheckRealtimeEvent(frameCtx, c, info, message)
					frameCancel()

					if auditRes.IsBlock {
						// 危险输入阻断：仅丢弃当前帧，发送本地 error event，保持连接，不计入 token，不写入上游
						_ = sendRealtimeErrorEvent(c, clientWriter, eventID, promptaudit.ErrorCodeBlocked, safeAuditMessage(promptaudit.ErrorCodeBlocked))
						continue
					}

					if auditRes.IsError {
						// 基础设施/存储/协议/降级错误：发送本地 error event 并退出连接
						_ = sendRealtimeErrorEvent(c, clientWriter, eventID, auditRes.ErrorCode, safeAuditMessage(auditRes.ErrorCode))
						select {
						case errChan <- fmt.Errorf("prompt audit failed closed: %s", auditRes.ErrorCode):
						default:
						}
						return
					}

					// 上游尚未连接时的首帧门禁处理
					if !upstreamConnected {
						if auditRes.IsNoPrompt {
							// 控制帧/纯音频帧：缓存并在首文本通过后刷新
							if len(bufferedMessages) >= maxRealtimeBufferedFrames || bufferedBytes+len(message) > maxRealtimeBufferedBytes {
								_ = sendRealtimeErrorEvent(c, clientWriter, eventID, promptaudit.ErrorCodeUnavailable, "Prompt audit buffer exceeded limit before initial text frame.")
								select {
								case errChan <- fmt.Errorf("realtime pre-text buffer limit exceeded"):
								default:
								}
								return
							}
							bufferedMessages = append(bufferedMessages, message)
							bufferedBytes += len(message)
							continue
						}

						// 收到首个通过审计的文本事件：先执行必要预扣，再拨号上游并按序刷新
						if info.NeedDeferredPreConsume {
							preErr := service.PreConsumeBilling(c, info.DeferredPreConsumeQuota, info)
							if preErr != nil {
								_ = sendRealtimeErrorEvent(c, clientWriter, eventID, "insufficient_quota", preErr.Error())
								select {
								case errChan <- fmt.Errorf("pre-consume quota failed: %w", preErr):
								default:
								}
								return
							}
							info.NeedDeferredPreConsume = false
						}

						targetConn, dialErr := info.TargetWsDialer()
						if dialErr != nil {
							_ = sendRealtimeErrorEvent(c, clientWriter, eventID, "upstream_connect_failed", "Failed to connect to upstream service.")
							select {
							case errChan <- fmt.Errorf("dial upstream realtime failed: %w", dialErr):
							default:
							}
							return
						}

						targetWriter.SetConn(targetConn)
						upstreamConnected = true

						// 按原始顺序刷新缓存的控制与音频帧
						for _, bufMsg := range bufferedMessages {
							if err := targetWriter.WriteTextMessage(bufMsg); err != nil {
								select {
								case errChan <- fmt.Errorf("error writing buffered message to target: %v", err):
								default:
								}
								return
							}
						}
						bufferedMessages = nil
						bufferedBytes = 0

						// 启动上游读协程
						startTargetReader(targetConn)
					}
				}

				// 通过审计或未开启审计：正常统计 token 并转发至上游
				realtimeEvent := &dto.RealtimeEvent{}
				if err := common.Unmarshal(message, realtimeEvent); err != nil {
					select {
					case errChan <- fmt.Errorf("error unmarshalling client message: %v", err):
					default:
					}
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate && realtimeEvent.Session != nil && realtimeEvent.Session.Tools != nil {
					info.RealtimeTools = realtimeEvent.Session.Tools
				}

				upstreamModel := getUpstreamModel(info)
				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, upstreamModel)
				if err != nil {
					select {
					case errChan <- fmt.Errorf("error counting text token: %v", err):
					default:
					}
					return
				}

				usageMu.Lock()
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken
				usageMu.Unlock()

				if err := targetWriter.WriteTextMessage(message); err != nil {
					select {
					case errChan <- fmt.Errorf("error writing to target: %v", err):
					default:
					}
					return
				}
			}
		}
	})

	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		logger.LogError(c, "realtime error: "+err.Error())
		cancel()
	case <-c.Done():
	}

	usageMu.Lock()
	defer usageMu.Unlock()

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens

	return service.PreWssConsumeQuota(ctx, info, usage)
}

func sendRealtimeErrorEvent(c *gin.Context, writer *safeWebSocketWriter, eventID string, code string, message string) error {
	if eventID == "" {
		eventID = helper.GetLocalRealtimeID(c)
	}
	errEvt := dto.RealtimeEvent{
		Type:    dto.RealtimeEventTypeError,
		EventId: eventID,
		Error: &types.OpenAIError{
			Type:    "invalid_request_error",
			Code:    code,
			Message: common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
		},
	}
	data, err := common.Marshal(errEvt)
	if err != nil {
		return err
	}
	return writer.WriteTextMessage(data)
}

func safeAuditMessage(code string) string {
	switch code {
	case promptaudit.ErrorCodeBlocked:
		return "Your request was blocked by prompt security audit."
	case promptaudit.ErrorCodeUnavailable:
		return "Prompt security audit service is currently unavailable."
	case promptaudit.ErrorCodeInvalidResponse:
		return "Prompt security audit service returned an invalid response."
	case promptaudit.ErrorCodeConfigDegraded:
		return "Prompt security audit configuration is currently degraded."
	case promptaudit.ErrorCodeRecordFailed:
		return "Failed to record prompt security audit event."
	case promptaudit.ErrorCodeUnsupportedProtocol:
		return "Unsupported prompt audit event or payload format."
	default:
		return "Prompt security audit failed."
	}
}

func getUpstreamModel(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	if info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName != "" {
		return info.ChannelMeta.UpstreamModelName
	}
	return info.OriginModelName
}
