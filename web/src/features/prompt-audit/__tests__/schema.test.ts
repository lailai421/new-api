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
import { describe, expect, it } from 'vitest'

import {
  createDefaultConfigFormValues,
  createDefaultEndpoint,
  promptAuditConfigFormSchema,
  validateBaseUrl,
} from '../lib/schema'

describe('validateBaseUrl', () => {
  it('accepts valid http and https URLs with host', () => {
    expect(validateBaseUrl('http://localhost:8000')).toBe(true)
    expect(validateBaseUrl('https://guard.internal.net:8443')).toBe(true)
    expect(validateBaseUrl('https://api.qwen3guard.ai/v1')).toBe(true)
  })

  it('rejects URLs with credentials', () => {
    expect(validateBaseUrl('https://admin:secret@guard.com')).toBe(false)
  })

  it('rejects URLs with query string or fragment', () => {
    expect(validateBaseUrl('https://guard.com?key=123')).toBe(false)
    expect(validateBaseUrl('https://guard.com#section')).toBe(false)
  })

  it('rejects invalid protocols or malformed URLs', () => {
    expect(validateBaseUrl('ftp://guard.com')).toBe(false)
    expect(validateBaseUrl('not-a-url')).toBe(false)
    expect(validateBaseUrl('')).toBe(false)
  })
})

describe('promptAuditConfigFormSchema', () => {
  it('validates default config form values when disabled', () => {
    const defaultVals = createDefaultConfigFormValues()
    expect(defaultVals.scanners).toContain('cyber_abuse')
    expect(defaultVals.scanners).toHaveLength(10)
    const parsed = promptAuditConfigFormSchema.safeParse(defaultVals)
    expect(parsed.success).toBe(true)
  })

  it('fails validation when enabled=true but no active endpoint exists', () => {
    const values = createDefaultConfigFormValues()
    values.enabled = true
    values.endpoints = []

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(false)
    if (!parsed.success) {
      expect(parsed.error.issues.some((i) => i.path.includes('enabled'))).toBe(
        true
      )
    }
  })

  it('fails validation when enabled=true and all endpoints are disabled', () => {
    const values = createDefaultConfigFormValues()
    values.enabled = true
    const ep = createDefaultEndpoint()
    ep.enabled = false
    values.endpoints = [ep]

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(false)
  })

  it('passes validation when enabled=true and at least one endpoint is enabled', () => {
    const values = createDefaultConfigFormValues()
    values.enabled = true
    const ep = createDefaultEndpoint()
    ep.enabled = true
    values.endpoints = [ep]

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(true)
  })

  it('rejects duplicate endpoint IDs', () => {
    const values = createDefaultConfigFormValues()
    const ep1 = createDefaultEndpoint()
    ep1.id = 'guard-node-1'
    const ep2 = createDefaultEndpoint()
    ep2.id = 'guard-node-1'
    values.endpoints = [ep1, ep2]

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(false)
    if (!parsed.success) {
      expect(
        parsed.error.issues.some((i) =>
          i.message.includes('Duplicate endpoint ID')
        )
      ).toBe(true)
    }
  })

  it('enforces non-empty groups when all_groups is false', () => {
    const values = createDefaultConfigFormValues()
    values.all_groups = false
    values.groups = []

    const parsedEmpty = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedEmpty.success).toBe(false)

    values.groups = ['   ', '']
    const parsedWhitespace = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedWhitespace.success).toBe(false)

    values.groups = ['vip', 'default']
    const parsedValid = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedValid.success).toBe(true)
  })

  it('enforces at least one scanner category', () => {
    const values = createDefaultConfigFormValues()
    values.scanners = []

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(false)
  })

  it('enforces retention_days >= 0', () => {
    const values = createDefaultConfigFormValues()
    values.retention_days = -1

    const parsed = promptAuditConfigFormSchema.safeParse(values)
    expect(parsed.success).toBe(false)
  })

  it('enforces endpoint timeout_ms and input_limit boundaries', () => {
    const values = createDefaultConfigFormValues()
    const ep = createDefaultEndpoint()
    ep.timeout_ms = 50 // below 100
    values.endpoints = [ep]

    expect(promptAuditConfigFormSchema.safeParse(values).success).toBe(false)

    ep.timeout_ms = 35000 // above 30000
    expect(promptAuditConfigFormSchema.safeParse(values).success).toBe(false)

    ep.timeout_ms = 3000
    ep.input_limit = 50 // below 128
    expect(promptAuditConfigFormSchema.safeParse(values).success).toBe(false)

    ep.input_limit = 200000 // above 100000
    expect(promptAuditConfigFormSchema.safeParse(values).success).toBe(false)
  })

  it('validates protocol and enforces model name for LLM classifier', () => {
    const values = createDefaultConfigFormValues()
    const ep = createDefaultEndpoint()
    ;(ep as { protocol: string }).protocol = 'llm_classifier'
    ep.model = 'deepseek-chat'
    ep.base_url = 'https://api.deepseek.com'
    ep.timeout_ms = 8000
    values.endpoints = [ep]

    // 1. 合法的 LLM 分类器
    const parsedValid = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedValid.success).toBe(true)

    // 2. LLM 分类器未提供模型名称 -> 校验失败
    ep.model = '   '
    const parsedNoModel = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedNoModel.success).toBe(false)
    if (!parsedNoModel.success) {
      expect(
        parsedNoModel.error.issues.some((i) =>
          i.message.includes('Model name is required for LLM classifier')
        )
      ).toBe(true)
    }

    // 3. 非法协议 -> 校验失败
    ep.model = 'deepseek-chat'
    ;(ep as { protocol: string }).protocol = 'unsupported_protocol'
    const parsedBadProto = promptAuditConfigFormSchema.safeParse(values)
    expect(parsedBadProto.success).toBe(false)
  })
})
