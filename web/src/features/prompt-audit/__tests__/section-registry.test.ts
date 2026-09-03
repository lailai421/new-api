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
  getSecuritySectionContent,
  getSecuritySectionMeta,
  getSecuritySectionNavItems,
  SECURITY_SECTION_IDS,
} from '@/features/system-settings/security/section-registry'

describe('security section registry integration', () => {
  it('registers prompt-audit section correctly', () => {
    expect(SECURITY_SECTION_IDS).toContain('prompt-audit')

    const meta = getSecuritySectionMeta('prompt-audit')
    expect(meta?.titleKey).toBe('Prompt Audit')

    const t = ((key: string) => key) as unknown as Parameters<
      typeof getSecuritySectionNavItems
    >[0]
    const navItems = getSecuritySectionNavItems(t)
    const auditItem = navItems.find(
      (item) => item.url === '/system-settings/security/prompt-audit'
    )
    expect(auditItem).toBeDefined()
    expect(auditItem?.title).toBe('Prompt Audit')

    const content = getSecuritySectionContent('prompt-audit', {} as never)
    expect(content).toBeDefined()
  })
})
