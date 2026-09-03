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
import { useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import {
  AlertTriangle,
  Clock,
  Hash,
  Key,
  Layers,
  Shield,
  User,
} from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'

import {
  DECISION_CONFIG,
  PROMPT_AUDIT_QUERY_KEYS,
  RISK_LEVEL_CONFIG,
} from '../constants'
import { usePromptAuditEventDetail } from '../hooks/use-prompt-audit-events'

interface EventDetailSheetProps {
  eventId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EventDetailSheet({
  eventId,
  open,
  onOpenChange,
}: EventDetailSheetProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const {
    data: event,
    isLoading,
    isError,
    error,
  } = usePromptAuditEventDetail(eventId, { enabled: open })

  // 当关闭 Sheet 或切换 eventId 时，立即主动从 React Query 缓存中移除敏感详情数据
  useEffect(() => {
    if (!open && eventId) {
      queryClient.removeQueries({
        queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(eventId),
      })
    }
  }, [open, eventId, queryClient])

  // 组件销毁时兜底清理
  useEffect(() => {
    return () => {
      if (eventId) {
        queryClient.removeQueries({
          queryKey: PROMPT_AUDIT_QUERY_KEYS.eventDetail(eventId),
        })
      }
    }
  }, [eventId, queryClient])

  const decisionConfig = event?.decision
    ? DECISION_CONFIG[event.decision.toLowerCase()]
    : null
  const riskConfig = event?.risk_level
    ? RISK_LEVEL_CONFIG[event.risk_level.toLowerCase()]
    : null

  const renderContent = () => {
    if (isLoading) {
      return (
        <div className='space-y-6'>
          <div className='grid grid-cols-2 gap-4'>
            <Skeleton className='h-16 w-full rounded-lg' />
            <Skeleton className='h-16 w-full rounded-lg' />
            <Skeleton className='h-16 w-full rounded-lg' />
            <Skeleton className='h-16 w-full rounded-lg' />
          </div>
          <Skeleton className='h-48 w-full rounded-lg' />
        </div>
      )
    }

    if (isError) {
      return (
        <div className='border-destructive/30 bg-destructive/10 text-destructive rounded-lg border p-4'>
          <div className='flex items-center gap-2 font-medium'>
            <AlertTriangle className='size-4' />
            <span>{t('Failed to load event details')}</span>
          </div>
          <p className='mt-1 text-sm'>
            {error instanceof Error ? error.message : t('An error occurred')}
          </p>
        </div>
      )
    }

    if (!event) return null

    return (
      <div className='space-y-6'>
        {/* 元数据属性卡片网格 */}
        <div className='grid gap-3 sm:grid-cols-2'>
          {/* 请求信息 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <Hash className='size-3.5' />
              <span>{t('Request ID')}</span>
            </div>
            <div
              className='truncate font-mono font-medium'
              title={event.request_id}
            >
              {event.request_id || '-'}
            </div>
          </div>

          {/* 时间戳 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <Clock className='size-3.5' />
              <span>{t('Created At')}</span>
            </div>
            <div className='font-mono font-medium'>
              {event.created_at
                ? dayjs.unix(event.created_at).format('YYYY-MM-DD HH:mm:ss')
                : '-'}
            </div>
          </div>

          {/* 用户信息 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <User className='size-3.5' />
              <span>{t('User')}</span>
            </div>
            <div className='truncate font-medium'>
              {event.username_snapshot || `UID: ${event.user_id}`}
              {event.user_email_snapshot && (
                <span className='text-muted-foreground ml-1'>
                  ({event.user_email_snapshot})
                </span>
              )}
            </div>
          </div>

          {/* 令牌与分组 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <Key className='size-3.5' />
              <span>{t('Token & Group')}</span>
            </div>
            <div className='truncate font-medium'>
              <span>
                {event.token_name_snapshot || `Token #${event.token_id}`}
              </span>
              <Badge variant='outline' className='ml-1.5 text-[10px]'>
                {event.group || 'default'}
              </Badge>
            </div>
          </div>

          {/* 协议与模型 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <Layers className='size-3.5' />
              <span>{t('Protocol & Model')}</span>
            </div>
            <div className='truncate font-mono font-medium'>
              {event.protocol} / {event.model}
            </div>
          </div>

          {/* Guard 节点与耗时 */}
          <div className='space-y-1 rounded-lg border p-3 text-xs'>
            <div className='text-muted-foreground flex items-center gap-1.5 font-medium'>
              <Shield className='size-3.5' />
              <span>{t('Guard Evaluation')}</span>
            </div>
            <div className='truncate font-medium'>
              <span className='font-mono'>
                {event.guard_endpoint_id || '-'}
              </span>
              <span className='text-muted-foreground ml-1.5'>
                ({event.latency_ms}ms, {event.chunk_total} chunks)
              </span>
            </div>
          </div>
        </div>

        {/* 命中风险分类 */}
        <div className='space-y-2'>
          <h4 className='text-sm font-medium'>{t('Detected Categories')}</h4>
          {event.categories && event.categories.length > 0 ? (
            <div className='flex flex-wrap gap-1.5'>
              {event.categories.map((cat) => (
                <Badge key={cat} variant='destructive'>
                  {cat}
                </Badge>
              ))}
            </div>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t('No high-risk categories detected')}
            </p>
          )}
        </div>

        {/* 完整提示词展示 */}
        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <h4 className='text-sm font-medium'>
              {t('Full Unredacted Prompt')}
            </h4>
            <div className='flex items-center gap-2'>
              <span className='text-muted-foreground font-mono text-xs'>
                {event.prompt_length} {t('chars')}
              </span>
              <CopyButton
                value={event.full_prompt || ''}
                size='sm'
                className='h-7 text-xs'
              />
            </div>
          </div>

          <div className='bg-muted/40 relative rounded-lg border p-3.5'>
            <pre className='max-h-96 overflow-y-auto font-mono text-xs leading-relaxed break-words whitespace-pre-wrap select-text'>
              {event.full_prompt || t('[Empty prompt content]')}
            </pre>
          </div>
        </div>
      </div>
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex h-full w-full flex-col gap-0 p-0 sm:max-w-2xl'
      >
        <SheetHeader className='border-b p-6'>
          <div className='flex items-center gap-2'>
            <Badge variant='outline' className='font-mono'>
              #{eventId}
            </Badge>
            {decisionConfig && (
              <Badge variant={decisionConfig.variant}>
                {t(decisionConfig.labelKey)}
              </Badge>
            )}
            {riskConfig && (
              <Badge variant={riskConfig.variant}>
                {t(riskConfig.labelKey)}
              </Badge>
            )}
          </div>
          <SheetTitle className='mt-2 text-xl font-semibold'>
            {t('Prompt Audit Event Details')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Audit snapshot, security evaluation, and unredacted prompt text.'
            )}
          </SheetDescription>
        </SheetHeader>

        <ScrollArea className='flex-1 p-6'>{renderContent()}</ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
