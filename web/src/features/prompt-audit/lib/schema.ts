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
import * as z from 'zod'

import {
  DEFAULT_GUARD_MODEL,
  DEFAULT_GUARD_PROTOCOL,
  DEFAULT_INPUT_LIMIT,
  DEFAULT_LLM_CLASSIFIER_BASE_URL,
  DEFAULT_LLM_CLASSIFIER_MODEL,
  DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS,
  DEFAULT_SCANNER_IDS,
  DEFAULT_TIMEOUT_MS,
  MAX_INPUT_LIMIT,
  MAX_TIMEOUT_MS,
  MIN_INPUT_LIMIT,
  MIN_TIMEOUT_MS,
  PROTOCOL_LLM_CLASSIFIER,
  PROTOCOL_OPENAI_COMPATIBLE,
} from '../constants'
import type { PromptAuditTokenStatus } from '../types'

/**
 * 校验 BaseURL：必须为 http/https，必须有 host，禁止 userinfo、query 和 fragment。
 */
export function validateBaseUrl(val: string): boolean {
  try {
    const url = new URL(val)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      return false
    }
    if (!url.hostname) {
      return false
    }
    if (url.username || url.password) {
      return false
    }
    if (url.search || url.hash) {
      return false
    }
    return true
  } catch {
    return false
  }
}

export const promptAuditEndpointFormSchema = z.object({
  id: z
    .string()
    .trim()
    .min(1, 'Endpoint ID is required')
    .max(64, 'Endpoint ID must be at most 64 characters'),
  name: z
    .string()
    .trim()
    .min(1, 'Endpoint name is required')
    .max(128, 'Endpoint name must be at most 128 characters'),
  protocol: z.enum([PROTOCOL_OPENAI_COMPATIBLE, PROTOCOL_LLM_CLASSIFIER]),
  base_url: z
    .string()
    .trim()
    .min(1, 'Base URL is required')
    .refine(
      validateBaseUrl,
      'Base URL must be a valid http or https URL without userinfo, query, or fragment'
    ),
  model: z.string().trim().min(1, 'Model name is required'),
  timeout_ms: z
    .number({ message: 'Timeout must be a number' })
    .int('Timeout must be an integer')
    .min(MIN_TIMEOUT_MS, `Timeout must be at least ${MIN_TIMEOUT_MS}ms`)
    .max(MAX_TIMEOUT_MS, `Timeout cannot exceed ${MAX_TIMEOUT_MS}ms`),
  input_limit: z
    .number({ message: 'Input limit must be a number' })
    .int('Input limit must be an integer')
    .min(
      MIN_INPUT_LIMIT,
      `Input limit must be at least ${MIN_INPUT_LIMIT} characters`
    )
    .max(
      MAX_INPUT_LIMIT,
      `Input limit cannot exceed ${MAX_INPUT_LIMIT} characters`
    ),
  enabled: z.boolean(),
  token: z.string().optional(),
  delete_token: z.boolean().optional(),
  // UI 仅读状态，不参与服务端写入
  has_token: z.boolean().optional(),
  token_status: z.custom<PromptAuditTokenStatus>().optional(),
})

export type PromptAuditEndpointFormValues = z.infer<
  typeof promptAuditEndpointFormSchema
>

export const promptAuditConfigFormSchema = z
  .object({
    enabled: z.boolean(),
    latest_turn_only: z.boolean(),
    store_pass_events: z.boolean(),
    all_groups: z.boolean(),
    groups: z.array(z.string()),
    scanners: z
      .array(z.string())
      .min(1, 'At least one scanner category must be selected'),
    retention_days: z
      .number({ message: 'Retention days must be a number' })
      .int('Retention days must be an integer')
      .min(0, 'Retention days must be at least 0 (0 for permanent)'),
    strategy: z.literal('priority'),
    endpoints: z.array(promptAuditEndpointFormSchema),
    expected_config_version: z.number().int().min(0),
  })
  .superRefine((data, ctx) => {
    // 1. 指定分组时必须有至少一个有效非空分组
    if (!data.all_groups) {
      const validGroups = [
        ...new Set(data.groups.map((g) => g.trim()).filter(Boolean)),
      ]
      if (validGroups.length === 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message:
            'At least one valid group must be specified when all groups is disabled',
          path: ['groups'],
        })
      }
    }

    // 2. 节点 ID 必须唯一
    const seenIds = new Set<string>()
    for (let i = 0; i < data.endpoints.length; i++) {
      const epId = data.endpoints[i].id.trim()
      if (epId) {
        if (seenIds.has(epId)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: `Duplicate endpoint ID "${epId}"`,
            path: ['endpoints', i, 'id'],
          })
        } else {
          seenIds.add(epId)
        }
      }
    }

    // 3. 启用审计时至少有一个启用的节点
    if (data.enabled) {
      const hasEnabledEndpoint = data.endpoints.some((ep) => ep.enabled)
      if (!hasEnabledEndpoint) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message:
            'At least one endpoint must be enabled when prompt audit is enabled',
          path: ['enabled'],
        })
      }
    }

    // 4. LLM 分类器节点必须指定模型名称
    for (let i = 0; i < data.endpoints.length; i++) {
      const ep = data.endpoints[i]
      if (ep.protocol === PROTOCOL_LLM_CLASSIFIER && !ep.model.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'Model name is required for LLM classifier',
          path: ['endpoints', i, 'model'],
        })
      }
    }
  })

export type PromptAuditConfigFormValues = z.infer<
  typeof promptAuditConfigFormSchema
>

export function createDefaultEndpoint(): PromptAuditEndpointFormValues {
  return {
    id: `guard-${Date.now().toString(36)}`,
    name: 'Qwen3Guard Endpoint',
    protocol: DEFAULT_GUARD_PROTOCOL,
    base_url: 'http://localhost:8000',
    model: DEFAULT_GUARD_MODEL,
    timeout_ms: DEFAULT_TIMEOUT_MS,
    input_limit: DEFAULT_INPUT_LIMIT,
    enabled: true,
    has_token: false,
    token_status: 'missing',
  }
}

export function createDefaultLLMClassifierEndpoint(): PromptAuditEndpointFormValues {
  return {
    id: `guard-${Date.now().toString(36)}`,
    name: 'LLM Classifier Endpoint',
    protocol: PROTOCOL_LLM_CLASSIFIER,
    base_url: DEFAULT_LLM_CLASSIFIER_BASE_URL,
    model: DEFAULT_LLM_CLASSIFIER_MODEL,
    timeout_ms: DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS,
    input_limit: DEFAULT_INPUT_LIMIT,
    enabled: true,
    has_token: false,
    token_status: 'missing',
  }
}

export function createDefaultConfigFormValues(): PromptAuditConfigFormValues {
  return {
    enabled: false,
    latest_turn_only: false,
    store_pass_events: true,
    all_groups: true,
    groups: [],
    scanners: [...DEFAULT_SCANNER_IDS],
    retention_days: 0,
    strategy: 'priority',
    endpoints: [],
    expected_config_version: 0,
  }
}
