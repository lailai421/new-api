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
import { useMutation, useQueryClient } from '@tanstack/react-query'

import {
  batchDeletePromptAuditEvents,
  deletePromptAuditEvent,
  probePromptAuditEndpoint,
  updatePromptAuditConfig,
} from '../api'
import { PROMPT_AUDIT_QUERY_KEYS } from '../constants'
import type {
  PromptAuditConfigUpdateRequest,
  PromptAuditProbeRequest,
} from '../types'

export function useUpdatePromptAuditConfig() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (payload: PromptAuditConfigUpdateRequest) =>
      updatePromptAuditConfig(payload),
    gcTime: 0,
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: PROMPT_AUDIT_QUERY_KEYS.config,
      })
      queryClient.invalidateQueries({
        queryKey: PROMPT_AUDIT_QUERY_KEYS.runtime,
      })
    },
  })
}

export function useProbePromptAuditEndpoint() {
  return useMutation({
    mutationFn: (payload: PromptAuditProbeRequest) =>
      probePromptAuditEndpoint(payload),
    gcTime: 0,
  })
}

export function useDeletePromptAuditEvent() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: number) => deletePromptAuditEvent(id),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({
        queryKey: ['prompt-audit', 'events'],
      })
      queryClient.removeQueries({
        queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(id),
      })
    },
  })
}

export function useBatchDeletePromptAuditEvents() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (ids: number[]) => batchDeletePromptAuditEvents(ids),
    onSuccess: (_, ids) => {
      queryClient.invalidateQueries({
        queryKey: ['prompt-audit', 'events'],
      })
      for (const id of ids) {
        queryClient.removeQueries({
          queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(id),
        })
      }
    },
  })
}
