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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { DeleteEventDialog } from '../components/delete-event-dialog'

describe('DeleteEventDialog', () => {
  it('renders single deletion confirmation and triggers onConfirm', async () => {
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const onOpenChange = vi.fn()

    render(
      <DeleteEventDialog
        open
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
        isPending={false}
        count={1}
        singleId={108}
      />
    )

    expect(screen.getByText('Delete audit event #108?')).toBeInTheDocument()
    expect(
      screen.getByText(/This action cannot be undone/i)
    ).toBeInTheDocument()

    const confirmBtn = screen.getByRole('button', { name: /Confirm Delete/i })
    fireEvent.click(confirmBtn)
    expect(onConfirm).toHaveBeenCalled()
  })

  it('renders batch deletion title with count', () => {
    render(
      <DeleteEventDialog
        open
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
        isPending={false}
        count={25}
      />
    )

    expect(screen.getByText('Delete 25 audit events?')).toBeInTheDocument()
  })

  it('disables buttons while isPending=true', () => {
    render(
      <DeleteEventDialog
        open
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
        isPending
        count={1}
        singleId={108}
      />
    )

    expect(screen.getByRole('button', { name: /Cancel/i })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: /Confirm Delete/i })
    ).toBeDisabled()
  })
})
