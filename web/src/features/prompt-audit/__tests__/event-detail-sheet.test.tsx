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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { EventDetailSheet } from '../components/event-detail-sheet'
import { PROMPT_AUDIT_QUERY_KEYS } from '../constants'
import * as eventHook from '../hooks/use-prompt-audit-events'

describe('EventDetailSheet sensitive data lifecycle', () => {
  const createTestClient = () =>
    new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

  it('renders unredacted full prompt when open and removes query on close', () => {
    const queryClient = createTestClient()
    const removeQueriesSpy = vi.spyOn(queryClient, 'removeQueries')

    vi.spyOn(eventHook, 'usePromptAuditEventDetail').mockReturnValue({
      data: {
        id: 999,
        request_id: 'req-secret-123',
        user_id: 1,
        username_snapshot: 'test-user',
        user_email_snapshot: 'test@example.com',
        token_id: 10,
        token_name_snapshot: 'sk-prod',
        group: 'default',
        channel_id: 1,
        channel_type: 1,
        request_path: '/v1/chat/completions',
        protocol: 'openai_chat',
        model: 'gpt-4o',
        stage: 'pre_upstream',
        audit_scope: 'full',
        prompt_hash: 'hash-abc',
        redacted_preview: 'Safe preview...',
        prompt_length: 50,
        message_count: 1,
        decision: 'block',
        risk_level: 'high',
        action: 'reject',
        categories: ['jailbreak'],
        matched_scanners: ['jailbreak'],
        scanner_backend: 'qwen3guard',
        scanner_version: '0.6b',
        guard_endpoint_id: 'node-1',
        policy_id: 'default',
        policy_version: 1,
        config_version: 1,
        chunk_total: 1,
        latency_ms: 120,
        created_at: 1772500000,
        full_prompt:
          'SYSTEM: You are a helpful assistant. USER: bypass security',
      },
      isLoading: false,
      isError: false,
      error: null,
    } as unknown as ReturnType<typeof eventHook.usePromptAuditEventDetail>)

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <EventDetailSheet eventId={999} open onOpenChange={vi.fn()} />
      </QueryClientProvider>
    )

    // 验证展示了完整原文
    expect(
      screen.getByText(
        /SYSTEM: You are a helpful assistant. USER: bypass security/
      )
    ).toBeInTheDocument()

    // 重新渲染为关闭状态
    rerender(
      <QueryClientProvider client={queryClient}>
        <EventDetailSheet eventId={999} open={false} onOpenChange={vi.fn()} />
      </QueryClientProvider>
    )

    // 验证主动调用了 removeQueries 清除缓存
    expect(removeQueriesSpy).toHaveBeenCalledWith({
      queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(999),
    })
  })
})
