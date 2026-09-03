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

import { RuntimeTab } from '../components/runtime-tab'
import * as runtimeHook from '../hooks/use-prompt-audit-runtime'

describe('RuntimeTab', () => {
  const createTestClient = () =>
    new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })

  it('renders blocking active runtime status correctly', () => {
    vi.spyOn(runtimeHook, 'usePromptAuditRuntime').mockReturnValue({
      data: {
        mode: 'blocking',
        expected_config_version: 3,
        active_config_version: 3,
        config_loaded_at: '2026-09-03T12:00:00Z',
        degraded: false,
        enabled_endpoints: 2,
        uncovered_plugins: [],
      },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      isFetching: false,
    } as unknown as ReturnType<typeof runtimeHook.usePromptAuditRuntime>)

    render(
      <QueryClientProvider client={createTestClient()}>
        <RuntimeTab />
      </QueryClientProvider>
    )

    expect(screen.getByText('Blocking')).toBeInTheDocument()
    expect(screen.getByText('v3')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(
      screen.queryByText(/Prompt Audit Service Degraded/)
    ).not.toBeInTheDocument()
  })

  it('renders prominent alert when runtime is degraded', () => {
    vi.spyOn(runtimeHook, 'usePromptAuditRuntime').mockReturnValue({
      data: {
        mode: 'blocking',
        expected_config_version: 2,
        active_config_version: 1,
        config_load_error: 'stable_crypto_secret_missing',
        degraded: true,
        enabled_endpoints: 0,
        uncovered_plugins: ['kling_video'],
      },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      isFetching: false,
    } as unknown as ReturnType<typeof runtimeHook.usePromptAuditRuntime>)

    render(
      <QueryClientProvider client={createTestClient()}>
        <RuntimeTab />
      </QueryClientProvider>
    )

    // Degraded 警告
    expect(
      screen.getByText('Prompt Audit Service Degraded')
    ).toBeInTheDocument()
    expect(screen.getByText(/stable_crypto_secret_missing/)).toBeInTheDocument()

    // 未覆盖插件提示
    expect(screen.getByText('Uncovered Task Plugins')).toBeInTheDocument()
    expect(screen.getByText('kling_video')).toBeInTheDocument()
  })
})
