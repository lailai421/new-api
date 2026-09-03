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
import dayjs from 'dayjs'
import {
  AlertTriangle,
  Eye,
  Filter,
  RefreshCw,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { DECISION_CONFIG, RISK_LEVEL_CONFIG } from '../constants'
import { usePromptAuditEvents } from '../hooks/use-prompt-audit-events'
import {
  useBatchDeletePromptAuditEvents,
  useDeletePromptAuditEvent,
} from '../hooks/use-prompt-audit-mutations'
import type { PromptAuditEventFilter, PromptAuditEventItem } from '../types'
import { DeleteEventDialog } from './delete-event-dialog'
import { EventDetailSheet } from './event-detail-sheet'

export function EventsTab() {
  const { t } = useTranslation()

  // 筛选与分页状态
  const [filters, setFilters] = useState<PromptAuditEventFilter>({
    page: 1,
    pageSize: 20,
    requestId: '',
    userId: undefined,
    decision: '',
    riskLevel: '',
    group: '',
    model: '',
  })

  // 临时搜索输入状态
  const [searchInput, setSearchInput] = useState('')

  // 多选与弹窗状态
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [activeDetailId, setActiveDetailId] = useState<number | null>(null)
  const [deleteTargetId, setDeleteTargetId] = useState<number | null>(null)
  const [isBatchDeleting, setIsBatchDeleting] = useState(false)

  const {
    data: eventsData,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = usePromptAuditEvents(filters)

  const deleteSingleMutation = useDeletePromptAuditEvent()
  const deleteBatchMutation = useBatchDeletePromptAuditEvents()

  const items = eventsData?.items || []
  const total = eventsData?.total || 0
  const totalPages = Math.max(1, Math.ceil(total / filters.pageSize))

  // 行选择控制
  const isAllCurrentSelected =
    items.length > 0 && items.every((item) => selectedIds.has(item.id))
  const isSomeCurrentSelected =
    items.some((item) => selectedIds.has(item.id)) && !isAllCurrentSelected

  const handleToggleSelectAll = () => {
    if (isAllCurrentSelected) {
      const next = new Set(selectedIds)
      for (const item of items) {
        next.delete(item.id)
      }
      setSelectedIds(next)
    } else {
      const next = new Set(selectedIds)
      for (const item of items) {
        next.add(item.id)
      }
      setSelectedIds(next)
    }
  }

  const handleToggleSelectRow = (id: number) => {
    const next = new Set(selectedIds)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    setSelectedIds(next)
  }

  const handleApplySearch = () => {
    setFilters((prev) => ({
      ...prev,
      page: 1,
      requestId: searchInput.trim() || undefined,
    }))
    setSelectedIds(new Set())
  }

  const handleResetFilters = () => {
    setSearchInput('')
    setFilters({
      page: 1,
      pageSize: 20,
      requestId: '',
      userId: undefined,
      decision: '',
      riskLevel: '',
      group: '',
      model: '',
    })
    setSelectedIds(new Set())
  }

  // 单条删除处理
  const handleConfirmSingleDelete = async () => {
    if (!deleteTargetId) return
    try {
      await deleteSingleMutation.mutateAsync(deleteTargetId)
      toast.success(t('Audit event deleted successfully'))
      setDeleteTargetId(null)
      setSelectedIds((prev) => {
        const next = new Set(prev)
        next.delete(deleteTargetId)
        return next
      })
    } catch (err: unknown) {
      toast.error(
        err instanceof Error ? err.message : t('Failed to delete audit event')
      )
    }
  }

  // 批量删除处理
  const handleConfirmBatchDelete = async () => {
    const ids = [...selectedIds]
    if (ids.length === 0 || ids.length > 500) {
      toast.error(t('Please select between 1 and 500 items'))
      return
    }

    try {
      const res = await deleteBatchMutation.mutateAsync(ids)
      toast.success(
        t('Successfully deleted {{count}} audit events', { count: res.deleted })
      )
      setIsBatchDeleting(false)
      setSelectedIds(new Set())
    } catch (err: unknown) {
      toast.error(
        err instanceof Error ? err.message : t('Failed to batch delete events')
      )
    }
  }

  const renderTableRows = () => {
    if (isLoading) {
      return [1, 2, 3, 4, 5].map((idx) => (
        <TableRow key={`events-table-skel-${idx}`}>
          <TableCell colSpan={8} className='py-3 text-center'>
            <Skeleton className='h-7 w-full rounded' />
          </TableCell>
        </TableRow>
      ))
    }

    if (items.length === 0) {
      return (
        <TableRow>
          <TableCell
            colSpan={8}
            className='text-muted-foreground py-12 text-center text-sm'
          >
            <Filter className='mx-auto mb-2 size-6 opacity-40' />
            <p>{t('No prompt audit events match current filters.')}</p>
          </TableCell>
        </TableRow>
      )
    }

    return items.map((item: PromptAuditEventItem) => {
      const isSelected = selectedIds.has(item.id)
      const decisionCfg = DECISION_CONFIG[item.decision.toLowerCase()]
      const riskCfg = RISK_LEVEL_CONFIG[item.risk_level.toLowerCase()]

      return (
        <TableRow
          key={item.id}
          data-state={isSelected ? 'selected' : undefined}
        >
          <TableCell className='px-3'>
            <Checkbox
              checked={isSelected}
              onCheckedChange={() => handleToggleSelectRow(item.id)}
              aria-label={`Select event #${item.id}`}
            />
          </TableCell>

          <TableCell className='text-muted-foreground font-mono text-xs'>
            {item.created_at
              ? dayjs.unix(item.created_at).format('MM-DD HH:mm:ss')
              : '-'}
          </TableCell>

          <TableCell>
            {decisionCfg ? (
              <Badge variant={decisionCfg.variant}>
                {t(decisionCfg.labelKey)}
              </Badge>
            ) : (
              <Badge variant='outline'>{item.decision}</Badge>
            )}
          </TableCell>

          <TableCell>
            {riskCfg ? (
              <Badge variant={riskCfg.variant}>{t(riskCfg.labelKey)}</Badge>
            ) : (
              <Badge variant='outline'>{item.risk_level}</Badge>
            )}
          </TableCell>

          <TableCell className='text-xs'>
            <div className='max-w-[10rem] truncate font-medium'>
              {item.username_snapshot || `UID: ${item.user_id}`}
            </div>
            <div className='text-muted-foreground flex max-w-[10rem] items-center gap-1 truncate text-[11px]'>
              <span>{item.group || 'default'}</span>
              <span>•</span>
              <span>
                {item.token_name_snapshot || `Token #${item.token_id}`}
              </span>
            </div>
          </TableCell>

          <TableCell className='text-xs'>
            <div className='max-w-[9rem] truncate font-mono font-medium'>
              {item.model || '-'}
            </div>
            <div className='text-muted-foreground max-w-[9rem] truncate font-mono text-[11px]'>
              {item.protocol || '-'}
            </div>
          </TableCell>

          <TableCell className='max-w-xs'>
            <div
              className='text-muted-foreground truncate font-mono text-xs'
              title={item.redacted_preview}
            >
              {item.redacted_preview || t('[No preview available]')}
            </div>
            {item.categories && item.categories.length > 0 && (
              <div className='mt-1 flex flex-wrap gap-1'>
                {item.categories.slice(0, 2).map((cat) => (
                  <Badge
                    key={cat}
                    variant='destructive'
                    className='h-4 px-1 text-[10px]'
                  >
                    {cat}
                  </Badge>
                ))}
                {item.categories.length > 2 && (
                  <span className='text-muted-foreground text-[10px]'>
                    +{item.categories.length - 2}
                  </span>
                )}
              </div>
            )}
          </TableCell>

          <TableCell className='text-right'>
            <div className='flex items-center justify-end gap-1'>
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => setActiveDetailId(item.id)}
                title={t('View full prompt details')}
                aria-label={t('View details')}
              >
                <Eye className='size-3.5' />
              </Button>
              <Button
                variant='ghost'
                size='icon-sm'
                className='text-destructive hover:text-destructive'
                onClick={() => setDeleteTargetId(item.id)}
                title={t('Delete event')}
                aria-label={t('Delete event')}
              >
                <Trash2 className='size-3.5' />
              </Button>
            </div>
          </TableCell>
        </TableRow>
      )
    })
  }

  return (
    <div className='space-y-4'>
      {/* 标题栏 */}
      <div className='flex flex-wrap items-center justify-between gap-4'>
        <div>
          <h2 className='text-lg font-medium tracking-tight'>
            {t('Prompt Audit Events')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Inspect gate decisions, detected risk categories, and unredacted prompt contents.'
            )}
          </p>
        </div>

        <div className='flex items-center gap-2'>
          {selectedIds.size > 0 && (
            <Button
              variant='destructive'
              size='sm'
              onClick={() => setIsBatchDeleting(true)}
              className='gap-1.5'
            >
              <Trash2 className='size-3.5' />
              <span>
                {t('Delete Selected ({{count}})', { count: selectedIds.size })}
              </span>
            </Button>
          )}

          <Button
            variant='outline'
            size='sm'
            onClick={() => refetch()}
            disabled={isFetching}
            className='gap-1.5'
          >
            {isFetching ? (
              <Spinner className='size-3.5' />
            ) : (
              <RefreshCw className='size-3.5' />
            )}
            <span>{t('Refresh')}</span>
          </Button>
        </div>
      </div>

      {/* 筛选与搜索工具条 */}
      <div className='bg-card flex flex-wrap items-center gap-2.5 rounded-lg border p-3 shadow-2xs'>
        {/* 请求 ID / 搜索 */}
        <div className='flex min-w-[14rem] flex-1 items-center gap-1.5 sm:max-w-xs'>
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleApplySearch()}
            placeholder={t('Search by Request ID...')}
            className='h-8 font-mono text-xs'
          />
          <Button
            variant='secondary'
            size='sm'
            onClick={handleApplySearch}
            className='h-8 px-2.5'
          >
            <Search className='size-3.5' />
          </Button>
        </div>

        {/* 判定过滤 */}
        <div className='w-28'>
          <Select
            value={filters.decision || 'all'}
            onValueChange={(val) => {
              setFilters((prev) => ({
                ...prev,
                page: 1,
                decision: val === 'all' ? '' : (val ?? ''),
              }))
              setSelectedIds(new Set())
            }}
          >
            <SelectTrigger className='h-8 text-xs'>
              <SelectValue placeholder={t('Decision')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{t('All Decisions')}</SelectItem>
              <SelectItem value='allow'>{t('Allow')}</SelectItem>
              <SelectItem value='warn'>{t('Warn')}</SelectItem>
              <SelectItem value='block'>{t('Block')}</SelectItem>
              <SelectItem value='error'>{t('Error')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* 风险级别过滤 */}
        <div className='w-32'>
          <Select
            value={filters.riskLevel || 'all'}
            onValueChange={(val) => {
              setFilters((prev) => ({
                ...prev,
                page: 1,
                riskLevel: val === 'all' ? '' : (val ?? ''),
              }))
              setSelectedIds(new Set())
            }}
          >
            <SelectTrigger className='h-8 text-xs'>
              <SelectValue placeholder={t('Risk Level')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all'>{t('All Risks')}</SelectItem>
              <SelectItem value='low'>{t('Low Risk')}</SelectItem>
              <SelectItem value='medium'>{t('Medium Risk')}</SelectItem>
              <SelectItem value='high'>{t('High Risk')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* 分页大小 */}
        <div className='w-24'>
          <Select
            value={String(filters.pageSize)}
            onValueChange={(val) => {
              setFilters((prev) => ({
                ...prev,
                page: 1,
                pageSize: Number(val),
              }))
              setSelectedIds(new Set())
            }}
          >
            <SelectTrigger className='h-8 text-xs'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='10'>10 / {t('page')}</SelectItem>
              <SelectItem value='20'>20 / {t('page')}</SelectItem>
              <SelectItem value='50'>50 / {t('page')}</SelectItem>
              <SelectItem value='100'>100 / {t('page')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* 重置按钮 */}
        {(filters.requestId || filters.decision || filters.riskLevel) && (
          <Button
            variant='ghost'
            size='sm'
            onClick={handleResetFilters}
            className='text-muted-foreground h-8 gap-1 text-xs'
          >
            <X className='size-3.5' />
            <span>{t('Reset')}</span>
          </Button>
        )}
      </div>

      {/* 错误提示 */}
      {isError && (
        <Alert variant='destructive'>
          <AlertTriangle className='size-4' />
          <AlertTitle>{t('Failed to load audit events')}</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : t('An error occurred')}
          </AlertDescription>
        </Alert>
      )}

      {/* 数据表格区 */}
      <div className='bg-card rounded-lg border shadow-2xs'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-10 px-3'>
                <Checkbox
                  checked={isAllCurrentSelected}
                  indeterminate={isSomeCurrentSelected}
                  onCheckedChange={handleToggleSelectAll}
                  aria-label={t('Select all items on current page')}
                />
              </TableHead>
              <TableHead className='w-36'>{t('Time')}</TableHead>
              <TableHead className='w-24'>{t('Decision')}</TableHead>
              <TableHead className='w-24'>{t('Risk Level')}</TableHead>
              <TableHead className='w-44'>{t('Identity & Group')}</TableHead>
              <TableHead className='w-36'>{t('Protocol & Model')}</TableHead>
              <TableHead className='min-w-[12rem]'>
                {t('Redacted Preview')}
              </TableHead>
              <TableHead className='w-24 text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>{renderTableRows()}</TableBody>
        </Table>
      </div>

      {/* 分页控制 */}
      <div className='flex flex-wrap items-center justify-between gap-4 px-1'>
        <div className='text-muted-foreground text-xs'>
          {t('Total {{total}} records, page {{page}} of {{pages}}', {
            total,
            page: filters.page,
            pages: totalPages,
          })}
        </div>

        <Pagination className='mx-0 w-auto'>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                text={t('Previous')}
                onClick={() => {
                  if (filters.page > 1) {
                    setFilters((p) => ({ ...p, page: p.page - 1 }))
                    setSelectedIds(new Set())
                  }
                }}
                className={
                  filters.page <= 1
                    ? 'pointer-events-none opacity-50'
                    : 'cursor-pointer'
                }
              />
            </PaginationItem>
            <PaginationItem>
              <span className='px-3 font-mono text-xs font-medium'>
                {filters.page} / {totalPages}
              </span>
            </PaginationItem>
            <PaginationItem>
              <PaginationNext
                text={t('Next')}
                onClick={() => {
                  if (filters.page < totalPages) {
                    setFilters((p) => ({ ...p, page: p.page + 1 }))
                    setSelectedIds(new Set())
                  }
                }}
                className={
                  filters.page >= totalPages
                    ? 'pointer-events-none opacity-50'
                    : 'cursor-pointer'
                }
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>

      {/* 详情按需 Sheet 抽屉 */}
      <EventDetailSheet
        eventId={activeDetailId}
        open={Boolean(activeDetailId)}
        onOpenChange={(open) => {
          if (!open) {
            setActiveDetailId(null)
          }
        }}
      />

      {/* 单条删除弹窗 */}
      <DeleteEventDialog
        open={Boolean(deleteTargetId)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTargetId(null)
          }
        }}
        onConfirm={handleConfirmSingleDelete}
        isPending={deleteSingleMutation.isPending}
        count={1}
        singleId={deleteTargetId || undefined}
      />

      {/* 批量删除弹窗 */}
      <DeleteEventDialog
        open={isBatchDeleting}
        onOpenChange={setIsBatchDeleting}
        onConfirm={handleConfirmBatchDelete}
        isPending={deleteBatchMutation.isPending}
        count={selectedIds.size}
      />
    </div>
  )
}
