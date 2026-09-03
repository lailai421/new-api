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
export type PromptAuditMode = 'off' | 'blocking'
export type PromptAuditTokenStatus = 'configured' | 'missing' | 'invalid'

export interface PromptAuditPublicEndpoint {
  id: string
  name: string
  protocol: 'openai_compatible'
  base_url: string
  model: string
  timeout_ms: number
  input_limit: number
  enabled: boolean
  has_token: boolean
  token_status: PromptAuditTokenStatus
}

export interface PromptAuditPublicConfig {
  enabled: boolean
  effective_mode: PromptAuditMode
  latest_turn_only: boolean
  store_pass_events: boolean
  all_groups: boolean
  groups: string[]
  scanners: string[]
  retention_days: number
  strategy: 'priority'
  endpoints: PromptAuditPublicEndpoint[]
  config_version: number
  updated_at: number
  updated_by: number
  change_summary?: string
}

export interface ScannerCatalogDefinition {
  id: string
  label: string
  label_zh: string
  description: string
  description_zh: string
}

export interface PromptAuditConfigResponse {
  config: PromptAuditPublicConfig
  scanners: ScannerCatalogDefinition[]
  config_version: number
}

export interface PromptAuditEndpointUpdateRequest {
  id: string
  name: string
  protocol: string
  base_url: string
  model: string
  timeout_ms: number
  input_limit: number
  enabled: boolean
  token?: string
  delete_token?: boolean
}

export interface PromptAuditConfigUpdateRequest {
  enabled: boolean
  latest_turn_only: boolean
  store_pass_events: boolean
  all_groups: boolean
  groups: string[]
  scanners: string[]
  retention_days: number
  strategy: string
  endpoints: PromptAuditEndpointUpdateRequest[]
  expected_config_version: number
}

export interface PromptAuditRuntimeResponse {
  mode: PromptAuditMode
  expected_config_version: number
  active_config_version: number
  config_loaded_at?: string
  config_load_error?: string
  degraded: boolean
  enabled_endpoints: number
  uncovered_plugins: string[]
}

export interface PromptAuditProbeRequest {
  endpoint_id?: string
  protocol?: string
  base_url?: string
  model?: string
  token?: string
  timeout_ms?: number
  input_limit?: number
}

export interface PromptAuditProbeResponse {
  success: boolean
  latency_ms: number
  model: string
  message: string
  error_code?: string
}

export interface PromptAuditEventFilter {
  startTime?: number
  endTime?: number
  userId?: number
  tokenId?: number
  group?: string
  model?: string
  protocol?: string
  decision?: string
  riskLevel?: string
  category?: string
  guardEndpointId?: string
  requestId?: string
  promptHash?: string
  page: number
  pageSize: number
}

export interface PromptAuditEventItem {
  id: number
  request_id: string
  user_id: number
  username_snapshot: string
  user_email_snapshot: string
  token_id: number
  token_name_snapshot: string
  group: string
  channel_id: number
  channel_type: number
  request_path: string
  protocol: string
  model: string
  stage: string
  audit_scope: string
  prompt_hash: string
  redacted_preview: string
  prompt_length: number
  message_count: number
  decision: string
  risk_level: string
  action: string
  categories: string[]
  matched_scanners: string[]
  scanner_backend: string
  scanner_version: string
  guard_endpoint_id: string
  policy_id: string
  policy_version: number
  config_version: number
  chunk_total: number
  latency_ms: number
  error_code?: string
  created_at: number
}

export interface PromptAuditEventDetailResponse extends PromptAuditEventItem {
  full_prompt: string
  scanner_scores?: Record<string, unknown>
  scanner_evidence?: Record<string, unknown>
}

export interface PromptAuditEventListResponse {
  items: PromptAuditEventItem[]
  total: number
  page: number
  page_size: number
}

export interface PromptAuditBatchDeleteResponse {
  deleted: number
}
