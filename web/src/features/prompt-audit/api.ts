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
import { api } from '@/lib/http-client'

import {
  normalizeEventFilters,
  parseEventListResponse,
} from './lib/event-parser'
import type {
  PromptAuditBatchDeleteResponse,
  PromptAuditConfigResponse,
  PromptAuditConfigUpdateRequest,
  PromptAuditEventDetailResponse,
  PromptAuditEventFilter,
  PromptAuditEventListResponse,
  PromptAuditProbeRequest,
  PromptAuditProbeResponse,
  PromptAuditPublicConfig,
  PromptAuditRuntimeResponse,
} from './types'

export async function getPromptAuditConfig(): Promise<PromptAuditConfigResponse> {
  const res = await api.get('/api/prompt-audit/config')
  return res.data?.data as PromptAuditConfigResponse
}

export async function updatePromptAuditConfig(
  payload: PromptAuditConfigUpdateRequest
): Promise<PromptAuditPublicConfig> {
  const res = await api.put('/api/prompt-audit/config', payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return res.data?.data as PromptAuditPublicConfig
}

export async function getPromptAuditRuntime(): Promise<PromptAuditRuntimeResponse> {
  const res = await api.get('/api/prompt-audit/runtime')
  return res.data?.data as PromptAuditRuntimeResponse
}

export async function probePromptAuditEndpoint(
  payload: PromptAuditProbeRequest
): Promise<PromptAuditProbeResponse> {
  const res = await api.post('/api/prompt-audit/endpoints/probe', payload, {
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return res.data?.data as PromptAuditProbeResponse
}

export async function getPromptAuditEvents(
  filters: PromptAuditEventFilter
): Promise<PromptAuditEventListResponse> {
  const params = normalizeEventFilters(filters)
  const res = await api.get('/api/prompt-audit/events', {
    params,
    disableDuplicate: true,
  })
  return parseEventListResponse(res.data?.data)
}

export async function getPromptAuditEventDetail(
  id: number
): Promise<PromptAuditEventDetailResponse> {
  const res = await api.get(`/api/prompt-audit/events/${id}`, {
    disableDuplicate: true,
  })
  return res.data?.data as PromptAuditEventDetailResponse
}

export async function deletePromptAuditEvent(
  id: number
): Promise<{ success: boolean; message: string }> {
  const res = await api.delete(`/api/prompt-audit/events/${id}`)
  return res.data
}

export async function batchDeletePromptAuditEvents(
  ids: number[]
): Promise<PromptAuditBatchDeleteResponse> {
  const res = await api.post('/api/prompt-audit/events/batch-delete', { ids })
  return res.data?.data as PromptAuditBatchDeleteResponse
}
