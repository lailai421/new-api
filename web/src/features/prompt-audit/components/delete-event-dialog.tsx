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
import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Spinner } from '@/components/ui/spinner'

interface DeleteEventDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => Promise<void>
  isPending: boolean
  count: number
  singleId?: number
}

export function DeleteEventDialog({
  open,
  onOpenChange,
  onConfirm,
  isPending,
  count,
  singleId,
}: DeleteEventDialogProps) {
  const { t } = useTranslation()

  const isBatch = count > 1 || !singleId

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia className='bg-destructive/10 text-destructive'>
            <Trash2 className='size-5' />
          </AlertDialogMedia>
          <AlertDialogTitle>
            {isBatch
              ? t('Delete {{count}} audit events?', { count })
              : t('Delete audit event #{{id}}?', { id: singleId })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(
              'This action cannot be undone. The selected audit events, unredacted prompt ciphertexts, and verification metadata will be permanently removed from the database.'
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={isPending}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            variant='destructive'
            disabled={isPending}
            onClick={async (e) => {
              e.preventDefault()
              await onConfirm()
            }}
            className='gap-1.5'
          >
            {isPending && <Spinner className='size-3.5' />}
            <span>{t('Confirm Delete')}</span>
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
