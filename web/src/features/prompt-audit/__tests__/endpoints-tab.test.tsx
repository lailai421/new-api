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
import { useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import * as api from '../api'
import { EndpointsTab } from '../components/endpoints-tab'
import {
  DEFAULT_GUARD_MODEL,
  DEFAULT_LLM_CLASSIFIER_BASE_URL,
  DEFAULT_LLM_CLASSIFIER_MODEL,
  DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS,
  PROTOCOL_LLM_CLASSIFIER,
  PROTOCOL_OPENAI_COMPATIBLE,
} from '../constants'
import {
  createDefaultConfigFormValues,
  createDefaultEndpoint,
  type PromptAuditConfigFormValues,
} from '../lib/schema'

function TestFormHarness(props: {
  initialValues?: Partial<PromptAuditConfigFormValues>
}) {
  const form = useForm<PromptAuditConfigFormValues>({
    defaultValues: {
      ...createDefaultConfigFormValues(),
      ...props.initialValues,
    },
  })

  return (
    <Form {...form}>
      <EndpointsTab form={form} />
    </Form>
  )
}

describe('EndpointsTab Protocol & Probe', () => {
  const createTestClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

  it('renders default endpoint as openai_compatible and allows probing with protocol', async () => {
    const probeSpy = vi
      .spyOn(api, 'probePromptAuditEndpoint')
      .mockResolvedValue({
        success: true,
        latency_ms: 120,
        model: 'sileader/qwen3guard:0.6b',
        message: '探测成功',
      })

    const ep = createDefaultEndpoint()
    render(
      <QueryClientProvider client={createTestClient()}>
        <TestFormHarness initialValues={{ endpoints: [ep] }} />
      </QueryClientProvider>
    )

    // 默认展示 Qwen3Guard 协议与对应默认模型
    expect(screen.getByDisplayValue(DEFAULT_GUARD_MODEL)).toBeDefined()
    expect(screen.getByDisplayValue('http://localhost:8000')).toBeDefined()

    // 点击 Probe
    const probeBtn = screen.getByRole('button', { name: /probe endpoint/i })
    fireEvent.click(probeBtn)

    await waitFor(() => {
      expect(probeSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          protocol: PROTOCOL_OPENAI_COMPATIBLE,
          model: DEFAULT_GUARD_MODEL,
          base_url: 'http://localhost:8000',
        })
      )
    })
  })

  it('shows notice and updates placeholders when protocol is llm_classifier', async () => {
    const ep = createDefaultEndpoint()
    ep.protocol = PROTOCOL_LLM_CLASSIFIER
    ep.model = DEFAULT_LLM_CLASSIFIER_MODEL
    ep.base_url = DEFAULT_LLM_CLASSIFIER_BASE_URL
    ep.timeout_ms = DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS

    render(
      <QueryClientProvider client={createTestClient()}>
        <TestFormHarness initialValues={{ endpoints: [ep] }} />
      </QueryClientProvider>
    )

    // 应展示 LLM 须知文案
    expect(screen.getByText(/LLM Classifier Protocol Notice/i)).toBeDefined()
    expect(
      screen.getByText(/Must be a standard OpenAI-compatible chat model/i)
    ).toBeDefined()
    expect(
      screen.getByText(/Do not point the Base URL to this gateway/i)
    ).toBeDefined()
    expect(
      screen.getByText(/Model output formatting errors will trigger 503/i)
    ).toBeDefined()

    expect(screen.getByDisplayValue(DEFAULT_LLM_CLASSIFIER_MODEL)).toBeDefined()
    expect(
      screen.getByDisplayValue(DEFAULT_LLM_CLASSIFIER_BASE_URL)
    ).toBeDefined()
    expect(screen.getByDisplayValue('8000')).toBeDefined()
  })

  it('probes with llm_classifier protocol when selected', async () => {
    const probeSpy = vi
      .spyOn(api, 'probePromptAuditEndpoint')
      .mockResolvedValue({
        success: true,
        latency_ms: 350,
        model: 'deepseek-chat',
        message: '探测成功',
      })

    const ep = createDefaultEndpoint()
    ep.protocol = PROTOCOL_LLM_CLASSIFIER
    ep.model = 'deepseek-chat'
    ep.base_url = 'https://api.deepseek.com'
    ep.timeout_ms = 8000

    render(
      <QueryClientProvider client={createTestClient()}>
        <TestFormHarness initialValues={{ endpoints: [ep] }} />
      </QueryClientProvider>
    )

    const probeBtn = screen.getByRole('button', { name: /probe endpoint/i })
    fireEvent.click(probeBtn)

    await waitFor(() => {
      expect(probeSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          protocol: PROTOCOL_LLM_CLASSIFIER,
          model: 'deepseek-chat',
          base_url: 'https://api.deepseek.com',
          timeout_ms: 8000,
        })
      )
    })
  })
})
