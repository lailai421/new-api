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
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  CheckCircle2,
  KeyRound,
  Plus,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useFieldArray, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import {
  DEFAULT_GUARD_MODEL,
  DEFAULT_GUARD_PROTOCOL,
  DEFAULT_LLM_CLASSIFIER_BASE_URL,
  DEFAULT_LLM_CLASSIFIER_MODEL,
  DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS,
  DEFAULT_TIMEOUT_MS,
  PROTOCOL_LLM_CLASSIFIER,
  PROTOCOL_OPENAI_COMPATIBLE,
  TOKEN_STATUS_CONFIG,
} from '../constants'
import { useProbePromptAuditEndpoint } from '../hooks/use-prompt-audit-mutations'
import {
  createDefaultEndpoint,
  type PromptAuditConfigFormValues,
  type PromptAuditEndpointFormValues,
} from '../lib/schema'
import type { PromptAuditProbeResponse } from '../types'

interface EndpointsTabProps {
  form: UseFormReturn<PromptAuditConfigFormValues>
}

export function EndpointsTab({ form }: EndpointsTabProps) {
  const { t } = useTranslation()
  const { fields, append, remove, move } = useFieldArray({
    control: form.control,
    name: 'endpoints',
  })

  const probeMutation = useProbePromptAuditEndpoint()
  const [probeResults, setProbeResults] = useState<
    Record<string, PromptAuditProbeResponse | null>
  >({})
  const [probingIndex, setProbingIndex] = useState<number | null>(null)

  const handleAddEndpoint = () => {
    append(createDefaultEndpoint())
  }

  const handleProbe = async (index: number) => {
    const ep = form.getValues(`endpoints.${index}`)
    if (!ep) return

    setProbingIndex(index)
    try {
      // 优先使用当前表单的草稿参数进行完整探测，以支持新添或修改中的节点测试
      const res = await probeMutation.mutateAsync({
        endpoint_id: ep.id,
        protocol: ep.protocol,
        base_url: ep.base_url,
        model: ep.model,
        token: ep.token || undefined,
        timeout_ms: ep.timeout_ms,
        input_limit: ep.input_limit,
      })

      setProbeResults((prev) => ({
        ...prev,
        [ep.id]: res,
      }))

      if (res.success) {
        toast.success(
          t('Endpoint probe succeeded ({{ms}}ms)', { ms: res.latency_ms })
        )
      } else {
        toast.error(res.message || t('Endpoint probe failed'))
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t('Probe request failed')
      setProbeResults((prev) => ({
        ...prev,
        [ep.id]: {
          success: false,
          latency_ms: 0,
          model: ep.model,
          message: msg,
          error_code: 'probe_failed',
        },
      }))
      toast.error(msg)
    } finally {
      setProbingIndex(null)
    }
  }

  const handleProtocolChange = (index: number, newProto: string) => {
    const currentEp = form.getValues(`endpoints.${index}`)
    if (!currentEp) return
    const oldProto = currentEp.protocol || DEFAULT_GUARD_PROTOCOL

    if (newProto === oldProto) return

    form.setValue(
      `endpoints.${index}.protocol`,
      newProto as
        | typeof PROTOCOL_OPENAI_COMPATIBLE
        | typeof PROTOCOL_LLM_CLASSIFIER,
      { shouldDirty: true, shouldValidate: true }
    )

    if (newProto === PROTOCOL_LLM_CLASSIFIER) {
      if (!currentEp.model || currentEp.model === DEFAULT_GUARD_MODEL) {
        form.setValue(
          `endpoints.${index}.model`,
          DEFAULT_LLM_CLASSIFIER_MODEL,
          { shouldDirty: true }
        )
      }
      if (
        !currentEp.base_url ||
        currentEp.base_url === 'http://localhost:8000'
      ) {
        form.setValue(
          `endpoints.${index}.base_url`,
          DEFAULT_LLM_CLASSIFIER_BASE_URL,
          { shouldDirty: true }
        )
      }
      if (currentEp.timeout_ms === DEFAULT_TIMEOUT_MS) {
        form.setValue(
          `endpoints.${index}.timeout_ms`,
          DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS,
          { shouldDirty: true }
        )
      }
    } else if (newProto === PROTOCOL_OPENAI_COMPATIBLE) {
      if (currentEp.model === DEFAULT_LLM_CLASSIFIER_MODEL) {
        form.setValue(`endpoints.${index}.model`, DEFAULT_GUARD_MODEL, {
          shouldDirty: true,
        })
      }
      if (currentEp.base_url === DEFAULT_LLM_CLASSIFIER_BASE_URL) {
        form.setValue(`endpoints.${index}.base_url`, 'http://localhost:8000', {
          shouldDirty: true,
        })
      }
      if (currentEp.timeout_ms === DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS) {
        form.setValue(`endpoints.${index}.timeout_ms`, DEFAULT_TIMEOUT_MS, {
          shouldDirty: true,
        })
      }
    }
  }

  return (
    <div className='space-y-6'>
      <div className='flex flex-wrap items-center justify-between gap-4'>
        <div>
          <h2 className='text-lg font-medium tracking-tight'>
            {t('Guard Endpoint Pool')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Manage priority-ordered Guard endpoints for prompt scanning and automatic failover.'
            )}
          </p>
        </div>
        <Button
          type='button'
          onClick={handleAddEndpoint}
          size='sm'
          className='gap-1.5'
        >
          <Plus className='size-3.5' />
          <span>{t('Add Endpoint')}</span>
        </Button>
      </div>

      {fields.length === 0 ? (
        <Card className='border-dashed'>
          <CardContent className='flex flex-col items-center justify-center py-12 text-center'>
            <div className='bg-muted flex size-12 items-center justify-center rounded-full'>
              <AlertTriangle className='text-muted-foreground size-6' />
            </div>
            <h3 className='mt-4 text-base font-semibold'>
              {t('No Guard endpoints configured')}
            </h3>
            <p className='text-muted-foreground mt-1 max-w-sm text-sm'>
              {t(
                'Prompt audit requires at least one active Guard endpoint before it can be enabled.'
              )}
            </p>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={handleAddEndpoint}
              className='mt-4 gap-1.5'
            >
              <Plus className='size-3.5' />
              <span>{t('Add Your First Endpoint')}</span>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className='space-y-4'>
          {fields.map((fieldItem, index) => {
            const epValues = form.watch(
              `endpoints.${index}`
            ) as PromptAuditEndpointFormValues
            const probeResult = epValues?.id ? probeResults[epValues.id] : null
            const isProbing = probingIndex === index
            const isFirst = index === 0
            const isLast = index === fields.length - 1
            const tokenStatus = epValues?.token_status || 'missing'
            const statusConfig = TOKEN_STATUS_CONFIG[tokenStatus]

            return (
              <Card
                key={fieldItem.id}
                className='relative transition-shadow hover:shadow-2xs'
              >
                <CardHeader className='flex flex-row items-center justify-between gap-4 border-b pb-4'>
                  <div className='flex items-center gap-3'>
                    <Badge variant='outline' className='font-mono'>
                      #{index + 1}
                    </Badge>
                    <CardTitle className='text-base font-medium'>
                      {epValues?.name || t('Unnamed Endpoint')}
                    </CardTitle>
                    <Badge variant={statusConfig?.variant || 'secondary'}>
                      {t(statusConfig?.labelKey || tokenStatus)}
                    </Badge>
                  </div>

                  {/* 右上角操作：启停、上移、下移、删除 */}
                  <div className='flex items-center gap-1.5'>
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.enabled`}
                      render={({ field }) => (
                        <div className='flex items-center gap-2 pr-2'>
                          <span className='text-muted-foreground text-xs'>
                            {field.value ? t('Enabled') : t('Disabled')}
                          </span>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                            aria-label={t('Toggle endpoint active')}
                          />
                        </div>
                      )}
                    />

                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() => move(index, index - 1)}
                      disabled={isFirst}
                      title={t('Move up (higher priority)')}
                      aria-label={t('Move up')}
                    >
                      <ArrowUp className='size-4' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() => move(index, index + 1)}
                      disabled={isLast}
                      title={t('Move down (lower priority)')}
                      aria-label={t('Move down')}
                    >
                      <ArrowDown className='size-4' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      className='text-destructive hover:text-destructive'
                      onClick={() => remove(index)}
                      title={t('Remove endpoint')}
                      aria-label={t('Remove endpoint')}
                    >
                      <Trash2 className='size-4' />
                    </Button>
                  </div>
                </CardHeader>

                <CardContent className='space-y-4 pt-5'>
                  <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
                    {/* 节点 ID */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.id`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Endpoint ID')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              placeholder='e.g. guard-node-1'
                              className='font-mono text-xs'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    {/* 节点名称 */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.name`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Endpoint Name')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              placeholder='e.g. Primary Qwen3Guard'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    {/* Base URL */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.base_url`}
                      render={({ field }) => (
                        <FormItem className='sm:col-span-2'>
                          <FormLabel>{t('Base URL')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              placeholder={
                                epValues?.protocol === PROTOCOL_LLM_CLASSIFIER
                                  ? DEFAULT_LLM_CLASSIFIER_BASE_URL
                                  : 'http://localhost:8000'
                              }
                              className='font-mono text-xs'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
                    {/* 模型名称 */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.model`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              placeholder={
                                epValues?.protocol === PROTOCOL_LLM_CLASSIFIER
                                  ? DEFAULT_LLM_CLASSIFIER_MODEL
                                  : DEFAULT_GUARD_MODEL
                              }
                              className='font-mono text-xs'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    {/* 超时时间 */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.timeout_ms`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Timeout (ms)')}</FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={100}
                              max={30000}
                              step={100}
                              value={
                                field.value ??
                                (epValues?.protocol === PROTOCOL_LLM_CLASSIFIER
                                  ? DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS
                                  : DEFAULT_TIMEOUT_MS)
                              }
                              onChange={(e) => {
                                const val = e.target.valueAsNumber
                                if (Number.isNaN(val)) {
                                  const fallback =
                                    epValues?.protocol ===
                                    PROTOCOL_LLM_CLASSIFIER
                                      ? DEFAULT_LLM_CLASSIFIER_TIMEOUT_MS
                                      : DEFAULT_TIMEOUT_MS
                                  field.onChange(fallback)
                                } else {
                                  field.onChange(val)
                                }
                              }}
                              className='font-mono'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    {/* 输入上限 */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.input_limit`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Input Limit (chars)')}</FormLabel>
                          <FormControl>
                            <Input
                              type='number'
                              min={128}
                              max={100000}
                              step={100}
                              value={field.value ?? 4000}
                              onChange={(e) => {
                                const val = e.target.valueAsNumber
                                field.onChange(Number.isNaN(val) ? 4000 : val)
                              }}
                              className='font-mono'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    {/* 协议选择 */}
                    <FormField
                      control={form.control}
                      name={`endpoints.${index}.protocol`}
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Protocol')}</FormLabel>
                          <Select
                            value={field.value}
                            onValueChange={(val) => {
                              if (!val) return
                              handleProtocolChange(index, val)
                            }}
                          >
                            <FormControl>
                              <SelectTrigger className='font-mono text-xs'>
                                <SelectValue />
                              </SelectTrigger>
                            </FormControl>
                            <SelectContent>
                              <SelectItem value={PROTOCOL_OPENAI_COMPATIBLE}>
                                {t('Qwen3Guard (openai_compatible)')}
                              </SelectItem>
                              <SelectItem value={PROTOCOL_LLM_CLASSIFIER}>
                                {t('LLM classifier (llm_classifier)')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  {/* LLM 分类器提示说明 */}
                  {epValues?.protocol === PROTOCOL_LLM_CLASSIFIER && (
                    <div className='bg-muted/40 text-muted-foreground rounded-lg border border-dashed p-3 text-xs leading-relaxed'>
                      <div className='flex items-start gap-2'>
                        <AlertTriangle className='text-warning mt-0.5 size-4 shrink-0' />
                        <div className='space-y-1'>
                          <p className='text-foreground font-medium'>
                            {t('LLM Classifier Protocol Notice')}
                          </p>
                          <ul className='list-disc space-y-0.5 pl-4'>
                            <li>
                              {t(
                                'Must be a standard OpenAI-compatible chat model (e.g. deepseek-v4-flash); reasoning/thinking models are not supported.'
                              )}
                            </li>
                            <li>
                              {t(
                                'Do not point the Base URL to this gateway or its channels to prevent recursive loops.'
                              )}
                            </li>
                            <li>
                              {t(
                                'Model output formatting errors will trigger 503 fail-closed protection.'
                              )}
                            </li>
                            <li>
                              {t(
                                'API usage costs are billed directly to the upstream account of this node token and will not be deducted from user quotas.'
                              )}
                            </li>
                          </ul>
                        </div>
                      </div>
                    </div>
                  )}

                  {/* Write-only Token 区域 */}
                  <div className='bg-muted/20 rounded-lg border p-3'>
                    <div className='flex flex-wrap items-center justify-between gap-2'>
                      <div className='flex items-center gap-2'>
                        <KeyRound className='text-muted-foreground size-4' />
                        <span className='text-sm font-medium'>
                          {t('Access Token')}
                        </span>
                        <Badge
                          variant={statusConfig?.variant || 'secondary'}
                          className='text-xs'
                        >
                          {t(statusConfig?.labelKey || tokenStatus)}
                        </Badge>
                      </div>

                      {epValues?.has_token && !epValues.delete_token && (
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='text-destructive hover:text-destructive text-xs'
                          onClick={() => {
                            form.setValue(
                              `endpoints.${index}.delete_token`,
                              true,
                              {
                                shouldDirty: true,
                              }
                            )
                            form.setValue(`endpoints.${index}.token`, '', {
                              shouldDirty: true,
                            })
                          }}
                        >
                          {t('Clear Existing Token')}
                        </Button>
                      )}

                      {epValues?.delete_token && (
                        <div className='flex items-center gap-2'>
                          <span className='text-destructive text-xs'>
                            {t('Token marked for deletion on save')}
                          </span>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            className='text-xs'
                            onClick={() => {
                              form.setValue(
                                `endpoints.${index}.delete_token`,
                                false,
                                { shouldDirty: true }
                              )
                            }}
                          >
                            {t('Cancel Deletion')}
                          </Button>
                        </div>
                      )}
                    </div>

                    {!epValues?.delete_token && (
                      <FormField
                        control={form.control}
                        name={`endpoints.${index}.token`}
                        render={({ field }) => (
                          <FormItem className='mt-2.5'>
                            <FormControl>
                              <Input
                                type='password'
                                {...field}
                                value={field.value ?? ''}
                                placeholder={
                                  epValues?.has_token
                                    ? t(
                                        'Leave empty to keep existing encrypted token, or type new token'
                                      )
                                    : t(
                                        'Enter Guard service Bearer token (optional for local endpoints)'
                                      )
                                }
                                autoComplete='new-password'
                                className='font-mono text-xs'
                              />
                            </FormControl>
                            <FormDescription className='text-xs'>
                              {t(
                                'Token is write-only and encrypted with AES-256-GCM. It will never be echoed back in responses.'
                              )}
                            </FormDescription>
                          </FormItem>
                        )}
                      />
                    )}
                  </div>

                  {/* 探测操作与结果 */}
                  <div className='flex flex-wrap items-center justify-between gap-3 pt-1'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => handleProbe(index)}
                      disabled={isProbing}
                      className='gap-1.5'
                    >
                      {isProbing ? (
                        <Spinner className='size-3.5' />
                      ) : (
                        <Activity className='size-3.5' />
                      )}
                      <span>{t('Probe Endpoint')}</span>
                    </Button>

                    {probeResult && (
                      <div className='flex items-center gap-2 text-xs'>
                        {probeResult.success ? (
                          <div className='text-success flex items-center gap-1.5 font-medium'>
                            <CheckCircle2 className='size-3.5' />
                            <span>
                              {t('Probe Passed')} ({probeResult.latency_ms}ms)
                            </span>
                          </div>
                        ) : (
                          <div className='text-destructive flex items-center gap-1.5 font-medium'>
                            <AlertTriangle className='size-3.5' />
                            <span>
                              {t('Probe Failed')}: {probeResult.message}
                            </span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
