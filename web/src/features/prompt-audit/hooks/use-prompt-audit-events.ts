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
import { useQuery } from '@tanstack/react-query'

import { getPromptAuditEventDetail, getPromptAuditEvents } from '../api'
import { PROMPT_AUDIT_QUERY_KEYS } from '../constants'
import { normalizeEventFilters } from '../lib/event-parser'
import type { PromptAuditEventFilter } from '../types'

export function usePromptAuditEvents(filters: PromptAuditEventFilter) {
  const normalizedKeyParams = normalizeEventFilters(filters)

  return useQuery({
    queryKey: PROMPT_AUDIT_QUERY_KEYS.events(normalizedKeyParams),
    queryFn: () => getPromptAuditEvents(filters),
    staleTime: 1000 * 10,
  })
}

export function usePromptAuditEventDetail(
  eventId: number | null,
  options?: { enabled?: boolean }
) {
  const isEnabled = Boolean(options?.enabled && eventId && eventId > 0)

  return useQuery({
    queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(eventId ?? 0),
    queryFn: () => getPromptAuditEventDetail(eventId as number),
    enabled: isEnabled,
    staleTime: 0,
    gcTime: 0, // 敏感数据不缓存在内存垃圾回收池中
    refetchOnWindowFocus: false,
    retry: 1,
  })
}
