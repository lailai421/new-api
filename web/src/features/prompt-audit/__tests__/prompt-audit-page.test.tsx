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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import * as api from '../api'
import { PromptAuditPage } from '../prompt-audit-page'

describe('PromptAuditPage integration', () => {
  const createTestClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

  it('handles remote config loading, tab switching, and CAS conflict', async () => {
    vi.spyOn(api, 'getPromptAuditRuntime').mockResolvedValue({
      mode: 'off',
      expected_config_version: 1,
      active_config_version: 1,
      degraded: false,
      enabled_endpoints: 1,
      uncovered_plugins: [],
    })

    vi.spyOn(api, 'getPromptAuditConfig').mockResolvedValue({
      config: {
        enabled: false,
        effective_mode: 'off',
        latest_turn_only: false,
        store_pass_events: true,
        all_groups: true,
        groups: [],
        scanners: ['violent', 'jailbreak'],
        retention_days: 0,
        strategy: 'priority',
        endpoints: [
          {
            id: 'ep-1',
            name: 'Primary Guard',
            protocol: 'openai_compatible',
            base_url: 'http://localhost:8000',
            model: 'sileader/qwen3guard:0.6b',
            timeout_ms: 3000,
            input_limit: 4000,
            enabled: true,
            has_token: true,
            token_status: 'configured',
          },
        ],
        config_version: 5,
        updated_at: 1772500000,
        updated_by: 1,
      },
      scanners: [
        {
          id: 'violent',
          label: 'Violent',
          label_zh: '暴力内容',
          description: 'Violent text',
          description_zh: '暴力行为描述',
        },
        {
          id: 'jailbreak',
          label: 'Jailbreak',
          label_zh: '越狱攻击',
          description: 'Jailbreak attempts',
          description_zh: '试图绕过安全限制',
        },
      ],
      config_version: 5,
    })

    const updateConfigSpy = vi
      .spyOn(api, 'updatePromptAuditConfig')
      .mockRejectedValue({
        isAxiosError: true,
        response: {
          status: 409,
          data: {
            error: {
              code: 'prompt_audit_config_conflict',
              message: 'version mismatch',
            },
          },
        },
      })

    render(
      <QueryClientProvider client={createTestClient()}>
        <PromptAuditPage />
      </QueryClientProvider>
    )

    // 默认展示 Runtime 状态
    await waitFor(() => {
      expect(screen.getByText('Audit Runtime Status')).toBeInTheDocument()
    })

    // 切换到策略选项卡
    const policyTabTrigger = screen.getByRole('tab', {
      name: /Policy & Scanners/i,
    })
    fireEvent.click(policyTabTrigger)

    await waitFor(() => {
      expect(screen.getByText('Audit Policy Configuration')).toBeInTheDocument()
    })

    // 校验保存按钮出现
    const saveButton = screen.getByRole('button', {
      name: /Save Configuration/i,
    })
    expect(saveButton).toBeInTheDocument()

    // 点击保存，验证携带当前的 expected_config_version=5
    fireEvent.click(saveButton)

    await waitFor(() => {
      expect(updateConfigSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          expected_config_version: 5,
        })
      )
    })
  })
})
