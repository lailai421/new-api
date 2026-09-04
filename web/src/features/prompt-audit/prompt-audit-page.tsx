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
import { zodResolver } from '@hookform/resolvers/zod'
import { isAxiosError } from 'axios'
import {
  Activity,
  AlertTriangle,
  FileText,
  RefreshCw,
  Save,
  Server,
  Shield,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { type FieldErrors, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { EndpointsTab } from './components/endpoints-tab'
import { EventsTab } from './components/events-tab'
import { PolicyTab } from './components/policy-tab'
import { RuntimeTab } from './components/runtime-tab'
import { ERROR_CODE_CONFIG_CONFLICT, PROMPT_AUDIT_TABS } from './constants'
import { usePromptAuditConfig } from './hooks/use-prompt-audit-config'
import { useUpdatePromptAuditConfig } from './hooks/use-prompt-audit-mutations'
import {
  createDefaultConfigFormValues,
  promptAuditConfigFormSchema,
  type PromptAuditConfigFormValues,
} from './lib/schema'
import type {
  PromptAuditConfigUpdateRequest,
  PromptAuditPublicConfig,
} from './types'

export function PromptAuditPage() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<string>(PROMPT_AUDIT_TABS.RUNTIME)

  const {
    data: configData,
    isLoading: isConfigLoading,
    isError: isConfigError,
    error: configError,
    refetch: refetchConfig,
    isFetching: isConfigFetching,
  } = usePromptAuditConfig()

  const updateMutation = useUpdatePromptAuditConfig()

  const form = useForm<PromptAuditConfigFormValues>({
    resolver: zodResolver(promptAuditConfigFormSchema),
    defaultValues: createDefaultConfigFormValues(),
  })

  // 封装统一的表单重置逻辑
  const resetFormWithConfig = useCallback(
    (c: PromptAuditPublicConfig) => {
      form.reset({
        enabled: c.enabled,
        latest_turn_only: c.latest_turn_only,
        store_pass_events: c.store_pass_events,
        all_groups: c.all_groups,
        groups: c.groups || [],
        scanners: c.scanners || [],
        retention_days: c.retention_days ?? 0,
        strategy: 'priority',
        endpoints: (c.endpoints || []).map((ep) => ({
          id: ep.id,
          name: ep.name,
          protocol: ep.protocol,
          base_url: ep.base_url,
          model: ep.model,
          timeout_ms: ep.timeout_ms,
          input_limit: ep.input_limit,
          enabled: ep.enabled,
          has_token: ep.has_token,
          token_status: ep.token_status,
          token: '',
          delete_token: false,
        })),
        expected_config_version: c.config_version,
      })
    },
    [form]
  )

  // 当远端配置加载成功后，重置本地表单
  useEffect(() => {
    if (configData?.config) {
      resetFormWithConfig(configData.config)
    }
  }, [configData?.config, resetFormWithConfig])

  // 重新加载配置并强制同步回显到表单
  const handleReload = async () => {
    try {
      const res = await refetchConfig()
      if (res.data?.config) {
        resetFormWithConfig(res.data.config)
        toast.success(t('Prompt audit configuration reloaded'))
      } else {
        toast.error(t('Failed to reload configuration'))
      }
    } catch (err: unknown) {
      toast.error(
        err instanceof Error ? err.message : t('Failed to reload configuration')
      )
    }
  }

  // 表单校验失败时的兜底提示与页面路由
  const onInvalidConfig = (
    errors: FieldErrors<PromptAuditConfigFormValues>
  ) => {
    let firstErrorMsg = ''

    if (errors.enabled?.message) {
      firstErrorMsg = String(errors.enabled.message)
    } else if (errors.groups?.message) {
      firstErrorMsg = String(errors.groups.message)
    } else if (errors.scanners?.message) {
      firstErrorMsg = String(errors.scanners.message)
    } else if (errors.retention_days?.message) {
      firstErrorMsg = String(errors.retention_days.message)
    } else if (errors.endpoints) {
      if (typeof errors.endpoints.message === 'string') {
        firstErrorMsg = errors.endpoints.message
      } else if (Array.isArray(errors.endpoints)) {
        for (let i = 0; i < errors.endpoints.length; i++) {
          const epErr = errors.endpoints[i]
          if (epErr) {
            const firstField = Object.values(epErr)[0] as
              | { message?: string }
              | undefined
            if (firstField?.message) {
              firstErrorMsg = `[${t('Endpoint')} ${i + 1}] ${firstField.message}`
              break
            }
          }
        }
      }
    }

    if (!firstErrorMsg && errors.root?.message) {
      firstErrorMsg = String(errors.root.message)
    }

    const displayMsg = firstErrorMsg
      ? t(firstErrorMsg)
      : t('Please check form fields for errors')

    toast.error(displayMsg)

    // 自动切换到包含错误项的选项卡
    const hasPolicyErrors = Boolean(
      errors.enabled ||
      errors.groups ||
      errors.scanners ||
      errors.retention_days
    )
    const hasEndpointErrors = Boolean(errors.endpoints)

    if (hasEndpointErrors && !hasPolicyErrors) {
      setActiveTab(PROMPT_AUDIT_TABS.ENDPOINTS)
    } else if (hasPolicyErrors) {
      setActiveTab(PROMPT_AUDIT_TABS.POLICY)
    }
  }

  const onSaveConfig = async (values: PromptAuditConfigFormValues) => {
    const payload: PromptAuditConfigUpdateRequest = {
      enabled: values.enabled,
      latest_turn_only: values.latest_turn_only,
      store_pass_events: values.store_pass_events,
      all_groups: values.all_groups,
      groups: values.all_groups ? [] : values.groups,
      scanners: values.scanners,
      retention_days: values.retention_days,
      strategy: values.strategy,
      endpoints: values.endpoints.map((ep) => ({
        id: ep.id,
        name: ep.name,
        protocol: ep.protocol,
        base_url: ep.base_url,
        model: ep.model,
        timeout_ms: ep.timeout_ms,
        input_limit: ep.input_limit,
        enabled: ep.enabled,
        token: ep.token ? ep.token.trim() : undefined,
        delete_token: ep.delete_token || undefined,
      })),
      expected_config_version: values.expected_config_version,
    }

    try {
      const updated = await updateMutation.mutateAsync(payload)
      toast.success(t('Prompt audit configuration saved successfully'))

      // 成功后重置表单为服务端返回的最新状态，清空明文 Token
      resetFormWithConfig(updated)
    } catch (err: unknown) {
      if (isAxiosError(err)) {
        const resData = err.response?.data as
          | { error?: { code?: string; message?: string }; message?: string }
          | undefined
        const code = resData?.error?.code

        if (
          err.response?.status === 409 ||
          code === ERROR_CODE_CONFIG_CONFLICT
        ) {
          toast.error(
            t(
              'Configuration conflict detected. Another administrator updated settings. Reloading latest config.'
            )
          )
          // 冲突时立即重新拉取远端配置重置本地表单
          const res = await refetchConfig()
          if (res.data?.config) {
            resetFormWithConfig(res.data.config)
          }
          return
        }

        if (
          code === 'prompt_audit_encryption_key_required' ||
          resData?.message?.includes('prompt_audit_encryption_key_required')
        ) {
          toast.error(
            t(
              'Persistent CRYPTO_SECRET or SESSION_SECRET must be configured in your environment or .env before enabling prompt audit.'
            )
          )
          return
        }

        toast.error(
          resData?.message || err.message || t('Failed to save configuration')
        )
      } else {
        toast.error(
          err instanceof Error ? err.message : t('Failed to save configuration')
        )
      }
    }
  }

  const isFormTab =
    activeTab === PROMPT_AUDIT_TABS.POLICY ||
    activeTab === PROMPT_AUDIT_TABS.ENDPOINTS

  const renderPolicyContent = () => {
    if (isConfigLoading) {
      return (
        <div className='space-y-4'>
          <Skeleton className='h-48 w-full rounded-xl' />
          <Skeleton className='h-64 w-full rounded-xl' />
        </div>
      )
    }
    if (isConfigError) {
      return (
        <Alert variant='destructive'>
          <AlertTriangle className='size-4' />
          <AlertTitle>{t('Failed to load configuration')}</AlertTitle>
          <AlertDescription>
            {configError instanceof Error
              ? configError.message
              : t('An error occurred')}
          </AlertDescription>
        </Alert>
      )
    }
    return (
      <PolicyTab
        form={form}
        scannersCatalog={configData?.scanners || []}
        hasStableCryptoSecret={configData?.config?.has_stable_crypto_secret}
        onNavigateToEndpoints={() => setActiveTab(PROMPT_AUDIT_TABS.ENDPOINTS)}
      />
    )
  }

  const formErrors = form.formState.errors
  const hasPolicyError = Boolean(
    formErrors.enabled ||
    formErrors.groups ||
    formErrors.scanners ||
    formErrors.retention_days
  )
  const hasEndpointsError = Boolean(formErrors.endpoints)

  return (
    <div className='w-full space-y-6'>
      {/* 顶部主选项卡切换 */}
      <Tabs
        value={activeTab}
        onValueChange={setActiveTab}
        className='w-full space-y-6'
      >
        <div className='flex flex-wrap items-center justify-between gap-4 border-b pb-4'>
          <div className='overflow-x-auto pb-1 sm:pb-0'>
            <TabsList className='h-9'>
              <TabsTrigger
                value={PROMPT_AUDIT_TABS.RUNTIME}
                className='gap-1.5 text-xs'
              >
                <Activity className='size-3.5' />
                <span>{t('Runtime Status')}</span>
              </TabsTrigger>
              <TabsTrigger
                value={PROMPT_AUDIT_TABS.POLICY}
                className='gap-1.5 text-xs'
              >
                <Shield className='size-3.5' />
                <span>{t('Policy & Scanners')}</span>
                {hasPolicyError && (
                  <span className='bg-destructive ml-0.5 size-1.5 rounded-full' />
                )}
              </TabsTrigger>
              <TabsTrigger
                value={PROMPT_AUDIT_TABS.ENDPOINTS}
                className='gap-1.5 text-xs'
              >
                <Server className='size-3.5' />
                <span>{t('Guard Endpoints')}</span>
                {hasEndpointsError && (
                  <span className='bg-destructive ml-0.5 size-1.5 rounded-full' />
                )}
              </TabsTrigger>
              <TabsTrigger
                value={PROMPT_AUDIT_TABS.EVENTS}
                className='gap-1.5 text-xs'
              >
                <FileText className='size-3.5' />
                <span>{t('Audit Events')}</span>
              </TabsTrigger>
            </TabsList>
          </div>

          {/* 表单页签下的统一保存与重置按钮 */}
          {isFormTab && (
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={handleReload}
                disabled={isConfigFetching || updateMutation.isPending}
                className='gap-1.5'
              >
                {isConfigFetching ? (
                  <Spinner className='size-3.5' />
                ) : (
                  <RefreshCw className='size-3.5' />
                )}
                <span>{t('Reload')}</span>
              </Button>
              <Button
                type='button'
                size='sm'
                onClick={form.handleSubmit(onSaveConfig, onInvalidConfig)}
                disabled={updateMutation.isPending || isConfigLoading}
                className='gap-1.5'
              >
                {updateMutation.isPending ? (
                  <Spinner className='size-3.5' />
                ) : (
                  <Save className='size-3.5' />
                )}
                <span>{t('Save Configuration')}</span>
              </Button>
            </div>
          )}
        </div>

        {/* 运行状态 Tab */}
        <TabsContent value={PROMPT_AUDIT_TABS.RUNTIME} className='outline-none'>
          <RuntimeTab />
        </TabsContent>

        {/* 策略表单与节点池 Tab（包裹在统一 Form Provider 中） */}
        <Form {...form}>
          <TabsContent
            value={PROMPT_AUDIT_TABS.POLICY}
            className='outline-none'
          >
            {renderPolicyContent()}
          </TabsContent>

          <TabsContent
            value={PROMPT_AUDIT_TABS.ENDPOINTS}
            className='outline-none'
          >
            {isConfigLoading ? (
              <div className='space-y-4'>
                <Skeleton className='h-36 w-full rounded-xl' />
                <Skeleton className='h-36 w-full rounded-xl' />
              </div>
            ) : (
              <EndpointsTab form={form} />
            )}
          </TabsContent>
        </Form>

        {/* 审计事件 Tab */}
        <TabsContent value={PROMPT_AUDIT_TABS.EVENTS} className='outline-none'>
          <EventsTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
