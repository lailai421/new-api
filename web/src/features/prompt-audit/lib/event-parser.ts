/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type {
  PromptAuditEventFilter,
  PromptAuditEventItem,
  PromptAuditEventListResponse,
} from '../types'

/**
 * 严格白名单投影：将后端返回的未知对象转换为 PromptAuditEventItem。
 * 绝对不包含 full_prompt、full_prompt_ciphertext、scanner_scores 或 scanner_evidence。
 */
export function sanitizeEventItem(raw: unknown): PromptAuditEventItem {
  const item = (raw && typeof raw === 'object' ? raw : {}) as Record<
    string,
    unknown
  >

  return {
    id: typeof item.id === 'number' ? item.id : Number(item.id) || 0,
    request_id: typeof item.request_id === 'string' ? item.request_id : '',
    user_id: typeof item.user_id === 'number' ? item.user_id : 0,
    username_snapshot:
      typeof item.username_snapshot === 'string' ? item.username_snapshot : '',
    user_email_snapshot:
      typeof item.user_email_snapshot === 'string'
        ? item.user_email_snapshot
        : '',
    token_id: typeof item.token_id === 'number' ? item.token_id : 0,
    token_name_snapshot:
      typeof item.token_name_snapshot === 'string'
        ? item.token_name_snapshot
        : '',
    group: typeof item.group === 'string' ? item.group : '',
    channel_id: typeof item.channel_id === 'number' ? item.channel_id : 0,
    channel_type: typeof item.channel_type === 'number' ? item.channel_type : 0,
    request_path:
      typeof item.request_path === 'string' ? item.request_path : '',
    protocol: typeof item.protocol === 'string' ? item.protocol : '',
    model: typeof item.model === 'string' ? item.model : '',
    stage: typeof item.stage === 'string' ? item.stage : '',
    audit_scope: typeof item.audit_scope === 'string' ? item.audit_scope : '',
    prompt_hash: typeof item.prompt_hash === 'string' ? item.prompt_hash : '',
    redacted_preview:
      typeof item.redacted_preview === 'string' ? item.redacted_preview : '',
    prompt_length:
      typeof item.prompt_length === 'number' ? item.prompt_length : 0,
    message_count:
      typeof item.message_count === 'number' ? item.message_count : 0,
    decision: typeof item.decision === 'string' ? item.decision : '',
    risk_level: typeof item.risk_level === 'string' ? item.risk_level : '',
    action: typeof item.action === 'string' ? item.action : '',
    categories: Array.isArray(item.categories)
      ? item.categories.map((c) => String(c))
      : [],
    matched_scanners: Array.isArray(item.matched_scanners)
      ? item.matched_scanners.map((s) => String(s))
      : [],
    scanner_backend:
      typeof item.scanner_backend === 'string' ? item.scanner_backend : '',
    scanner_version:
      typeof item.scanner_version === 'string' ? item.scanner_version : '',
    guard_endpoint_id:
      typeof item.guard_endpoint_id === 'string' ? item.guard_endpoint_id : '',
    policy_id: typeof item.policy_id === 'string' ? item.policy_id : '',
    policy_version:
      typeof item.policy_version === 'number' ? item.policy_version : 0,
    config_version:
      typeof item.config_version === 'number' ? item.config_version : 0,
    chunk_total: typeof item.chunk_total === 'number' ? item.chunk_total : 0,
    latency_ms: typeof item.latency_ms === 'number' ? item.latency_ms : 0,
    error_code:
      typeof item.error_code === 'string' ? item.error_code : undefined,
    created_at: typeof item.created_at === 'number' ? item.created_at : 0,
  }
}

export function parseEventListResponse(
  data: unknown
): PromptAuditEventListResponse {
  const obj = (data && typeof data === 'object' ? data : {}) as Record<
    string,
    unknown
  >
  const rawItems = Array.isArray(obj.items) ? obj.items : []
  const items = rawItems.map(sanitizeEventItem)

  return {
    items,
    total: typeof obj.total === 'number' ? obj.total : 0,
    page: typeof obj.page === 'number' ? obj.page : 1,
    page_size: typeof obj.page_size === 'number' ? obj.page_size : 20,
  }
}

export function normalizeEventFilters(
  filters: PromptAuditEventFilter
): Record<string, string | number> {
  const result: Record<string, string | number> = {
    p: filters.page > 0 ? filters.page : 1,
    page_size: filters.pageSize > 0 ? filters.pageSize : 20,
  }

  if (filters.startTime && filters.startTime > 0) {
    result.start_time = filters.startTime
  }
  if (filters.endTime && filters.endTime > 0) {
    result.end_time = filters.endTime
  }
  if (filters.userId && filters.userId > 0) {
    result.user_id = filters.userId
  }
  if (filters.tokenId && filters.tokenId > 0) {
    result.token_id = filters.tokenId
  }
  if (filters.group && filters.group.trim()) {
    result.group = filters.group.trim()
  }
  if (filters.model && filters.model.trim()) {
    result.model = filters.model.trim()
  }
  if (filters.protocol && filters.protocol.trim()) {
    result.protocol = filters.protocol.trim()
  }
  if (filters.decision && filters.decision.trim()) {
    result.decision = filters.decision.trim()
  }
  if (filters.riskLevel && filters.riskLevel.trim()) {
    result.risk_level = filters.riskLevel.trim()
  }
  if (filters.category && filters.category.trim()) {
    result.category = filters.category.trim()
  }
  if (filters.guardEndpointId && filters.guardEndpointId.trim()) {
    result.guard_endpoint_id = filters.guardEndpointId.trim()
  }
  if (filters.requestId && filters.requestId.trim()) {
    result.request_id = filters.requestId.trim()
  }
  if (filters.promptHash && filters.promptHash.trim()) {
    result.prompt_hash = filters.promptHash.trim()
  }

  return result
}
