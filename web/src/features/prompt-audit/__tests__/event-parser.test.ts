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
  normalizeEventFilters,
  parseEventListResponse,
  sanitizeEventItem,
} from '../lib/event-parser'

describe('event-parser', () => {
  it('strictly strips full_prompt and ciphertext from raw list items', () => {
    const rawWithSensitive = {
      id: 101,
      request_id: 'req-abc-123',
      user_id: 1,
      decision: 'block',
      risk_level: 'high',
      redacted_preview: 'Safe preview text...',
      full_prompt: 'SUPER_SENSITIVE_LEAKED_PROMPT_CONTENT',
      full_prompt_ciphertext: 'ENCRYPTED_BLOB_LEAK',
      scanner_scores: { jailbreak: 0.99 },
      scanner_evidence: { match: 'violation' },
      categories: ['jailbreak'],
      created_at: 1772500000,
    }

    const sanitized = sanitizeEventItem(rawWithSensitive)
    expect(sanitized.id).toBe(101)
    expect(sanitized.request_id).toBe('req-abc-123')
    expect(sanitized.redacted_preview).toBe('Safe preview text...')

    // 严禁包含敏感字段
    expect('full_prompt' in sanitized).toBe(false)
    expect('full_prompt_ciphertext' in sanitized).toBe(false)
    expect('scanner_scores' in sanitized).toBe(false)
    expect('scanner_evidence' in sanitized).toBe(false)
  })

  it('safely handles malformed fields and non-array categories', () => {
    const malformed = {
      id: '202',
      categories: 'not-an-array',
      matched_scanners: null,
    }

    const sanitized = sanitizeEventItem(malformed)
    expect(sanitized.id).toBe(202)
    expect(Array.isArray(sanitized.categories)).toBe(true)
    expect(sanitized.categories).toEqual([])
    expect(Array.isArray(sanitized.matched_scanners)).toBe(true)
    expect(sanitized.matched_scanners).toEqual([])
  })

  it('parses list response and wraps items securely', () => {
    const rawResponse = {
      items: [
        {
          id: 1,
          request_id: 'r1',
          full_prompt: 'leak',
        },
      ],
      total: 100,
      page: 2,
      page_size: 20,
    }

    const parsed = parseEventListResponse(rawResponse)
    expect(parsed.total).toBe(100)
    expect(parsed.page).toBe(2)
    expect(parsed.page_size).toBe(20)
    expect(parsed.items).toHaveLength(1)
    expect('full_prompt' in parsed.items[0]).toBe(false)
  })

  it('normalizes event filters removing empty or falsy values', () => {
    const normalized = normalizeEventFilters({
      page: 2,
      pageSize: 50,
      requestId: '   ',
      group: '  vip  ',
      decision: 'block',
      userId: 0,
      tokenId: 5,
    })

    expect(normalized).toEqual({
      p: 2,
      page_size: 50,
      group: 'vip',
      decision: 'block',
      token_id: 5,
    })
    expect('request_id' in normalized).toBe(false)
    expect('user_id' in normalized).toBe(false)
  })
})
