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
import { useQuery } from '@tanstack/react-query'
import { CheckSquare, Square } from 'lucide-react'
import { useMemo } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect, type Option } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { getUserGroups } from '@/lib/api'

import { DEFAULT_SCANNER_IDS, SCANNER_LABEL_KEYS } from '../constants'
import type { PromptAuditConfigFormValues } from '../lib/schema'
import type { ScannerCatalogDefinition } from '../types'

interface PolicyTabProps {
  form: UseFormReturn<PromptAuditConfigFormValues>
  scannersCatalog?: ScannerCatalogDefinition[]
}

export function PolicyTab({ form, scannersCatalog = [] }: PolicyTabProps) {
  const { t, i18n } = useTranslation()
  const isZh = i18n.language.startsWith('zh')

  // 获取用户可选分组列表
  const { data: userGroupsData } = useQuery({
    queryKey: ['user', 'self', 'groups'],
    queryFn: getUserGroups,
    staleTime: 1000 * 60 * 5,
  })

  const groupOptions: Option[] = useMemo(() => {
    const rawGroups = userGroupsData?.data
      ? Object.keys(userGroupsData.data)
      : []
    const currentGroups = form.watch('groups') || []
    const allSet = new Set([...rawGroups, ...currentGroups])

    return [...allSet].filter(Boolean).map((g) => ({
      label: g,
      value: g,
    }))
  }, [userGroupsData, form])

  const allGroups = form.watch('all_groups')
  const selectedScanners = form.watch('scanners') || []

  // 格式化 Catalog 扫描器定义
  const scanners = useMemo(() => {
    if (scannersCatalog.length > 0) {
      return scannersCatalog.map((s) => ({
        id: s.id,
        label:
          isZh && s.label_zh
            ? s.label_zh
            : t(SCANNER_LABEL_KEYS[s.id] ?? s.label),
        description:
          isZh && s.description_zh ? s.description_zh : s.description || '',
      }))
    }
    return DEFAULT_SCANNER_IDS.map((id) => ({
      id,
      label: t(SCANNER_LABEL_KEYS[id] ?? id),
      description: '',
    }))
  }, [scannersCatalog, isZh, t])

  const handleSelectAllScanners = () => {
    form.setValue(
      'scanners',
      scanners.map((s) => s.id),
      { shouldValidate: true, shouldDirty: true }
    )
  }

  const handleDeselectAllScanners = () => {
    form.setValue('scanners', [], { shouldValidate: true, shouldDirty: true })
  }

  return (
    <div className='space-y-6'>
      <div>
        <h2 className='text-lg font-medium tracking-tight'>
          {t('Audit Policy Configuration')}
        </h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Configure prompt audit behavior, target scopes, risk categories, and data retention.'
          )}
        </p>
      </div>

      <div className='grid gap-6 md:grid-cols-2'>
        {/* 全局主控与输入策略 */}
        <Card>
          <CardHeader>
            <CardTitle>{t('General Gate Rules')}</CardTitle>
            <CardDescription>
              {t('Control gate activation and payload extraction behavior.')}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-6'>
            {/* 启用审计 */}
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4 shadow-2xs'>
                  <div className='space-y-0.5 pr-4'>
                    <FormLabel className='text-base'>
                      {t('Enable Prompt Audit')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, user requests must pass synchronous audit before upstream dispatch. Requires at least one active endpoint.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-label={t('Enable Prompt Audit')}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            {/* 仅审计最新轮 */}
            <FormField
              control={form.control}
              name='latest_turn_only'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4 shadow-2xs'>
                  <div className='space-y-0.5 pr-4'>
                    <FormLabel className='text-base'>
                      {t('Audit Latest Turn Only')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Only scan the latest user input segment. Full prompt text will still be stored completely for administrator review.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-label={t('Audit Latest Turn Only')}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            {/* 保存通过事件 */}
            <FormField
              control={form.control}
              name='store_pass_events'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4 shadow-2xs'>
                  <div className='space-y-0.5 pr-4'>
                    <FormLabel className='text-base'>
                      {t('Store Passed Events')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Save Pass events to database. When disabled, only Block and Error events are recorded to conserve storage capacity.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-label={t('Store Passed Events')}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </CardContent>
        </Card>

        {/* 适用范围与保留策略 */}
        <Card>
          <CardHeader>
            <CardTitle>{t('Scope & Retention')}</CardTitle>
            <CardDescription>
              {t(
                'Target specific user groups and manage automatic data cleanup.'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent className='space-y-6'>
            {/* 全部分组开关 */}
            <FormField
              control={form.control}
              name='all_groups'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4 shadow-2xs'>
                  <div className='space-y-0.5 pr-4'>
                    <FormLabel className='text-base'>
                      {t('Apply to All Groups')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, requests from all user groups will be audited. Turn off to limit auditing to specified groups.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-label={t('Apply to All Groups')}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            {/* 指定分组选择 */}
            {!allGroups && (
              <FormField
                control={form.control}
                name='groups'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Target User Groups')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        options={groupOptions}
                        selected={field.value || []}
                        onChange={field.onChange}
                        allowCreate
                        placeholder={t('Select or type user group...')}
                        createLabel='Add "{{value}}"'
                        emptyText={t('No matching groups')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Specify at least one group. You can select existing groups or type new group strings.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* 数据保留天数 */}
            <FormField
              control={form.control}
              name='retention_days'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Data Retention (Days)')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-3'>
                      <Input
                        type='number'
                        min={0}
                        step={1}
                        value={field.value ?? 0}
                        onChange={(e) => {
                          const val = e.target.valueAsNumber
                          field.onChange(Number.isNaN(val) ? 0 : val)
                        }}
                        className='max-w-[12rem] font-mono'
                      />
                      <Badge
                        variant={field.value === 0 ? 'secondary' : 'outline'}
                      >
                        {field.value === 0
                          ? t('Permanent Retention')
                          : t('{{days}} Days Auto Cleanup', {
                              days: field.value,
                            })}
                      </Badge>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t(
                      '0 means keep audit events permanently without auto cleanup. Greater than 0 cleans expired events hourly on master node.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </CardContent>
        </Card>
      </div>

      {/* 风险分类勾选网格 */}
      <Card>
        <CardHeader className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <CardTitle>{t('Risk Scanner Categories')}</CardTitle>
            <CardDescription>
              {t(
                'Select which risk categories to detect. At least one category must be active.'
              )}
            </CardDescription>
          </div>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={handleSelectAllScanners}
              className='gap-1.5'
            >
              <CheckSquare className='size-3.5' />
              <span>{t('Select All')}</span>
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={handleDeselectAllScanners}
              className='gap-1.5'
            >
              <Square className='size-3.5' />
              <span>{t('Clear')}</span>
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <FormField
            control={form.control}
            name='scanners'
            render={() => (
              <FormItem>
                <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
                  {scanners.map((item) => {
                    const checked = selectedScanners.includes(item.id)
                    return (
                      <div
                        key={item.id}
                        onClick={() => {
                          const next = checked
                            ? selectedScanners.filter((id) => id !== item.id)
                            : [...selectedScanners, item.id]
                          form.setValue('scanners', next, {
                            shouldValidate: true,
                            shouldDirty: true,
                          })
                        }}
                        className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors ${
                          checked
                            ? 'border-primary/50 bg-primary/5'
                            : 'hover:bg-muted/50'
                        }`}
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={() => {
                            // Checked change handled by parent div click
                          }}
                          className='mt-0.5'
                          aria-label={item.label}
                        />
                        <div className='min-w-0 flex-1 space-y-0.5'>
                          <div className='text-sm leading-none font-medium'>
                            {item.label}
                          </div>
                          {item.description ? (
                            <p className='text-muted-foreground text-xs leading-relaxed'>
                              {item.description}
                            </p>
                          ) : (
                            <p className='text-muted-foreground font-mono text-xs'>
                              {item.id}
                            </p>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
                <FormMessage className='mt-3' />
              </FormItem>
            )}
          />
        </CardContent>
      </Card>
    </div>
  )
}
