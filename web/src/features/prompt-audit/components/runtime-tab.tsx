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
  CheckCircle2,
  Clock,
  RefreshCw,
  Server,
  Shield,
  ShieldAlert,
  ShieldOff,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

import { usePromptAuditRuntime } from '../hooks/use-prompt-audit-runtime'

export function RuntimeTab() {
  const { t } = useTranslation()
  const {
    data: runtime,
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = usePromptAuditRuntime()

  if (isLoading) {
    return (
      <div className='space-y-4'>
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {[1, 2, 3, 4].map((id) => (
            <Skeleton
              key={`runtime-card-skel-${id}`}
              className='h-28 w-full rounded-xl'
            />
          ))}
        </div>
        <Skeleton className='h-40 w-full rounded-xl' />
      </div>
    )
  }

  if (isError) {
    return (
      <Alert variant='destructive'>
        <AlertTriangle className='size-4' />
        <AlertTitle>{t('Failed to load runtime state')}</AlertTitle>
        <AlertDescription className='mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <span>
            {error instanceof Error ? error.message : t('An error occurred')}
          </span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => refetch()}
            disabled={isFetching}
          >
            {isFetching ? (
              <Spinner className='size-3.5' />
            ) : (
              <RefreshCw className='size-3.5' />
            )}
            <span>{t('Retry')}</span>
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  if (!runtime) {
    return null
  }

  const isBlocking = runtime.mode === 'blocking'
  const hasVersionMismatch =
    runtime.expected_config_version !== runtime.active_config_version

  return (
    <div className='space-y-6'>
      {/* 头部状态与操作 */}
      <div className='flex flex-wrap items-center justify-between gap-4'>
        <div>
          <h2 className='text-lg font-medium tracking-tight'>
            {t('Audit Runtime Status')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('Live status of prompt audit gate and active configurations.')}
          </p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => refetch()}
          disabled={isFetching}
          className='gap-2'
        >
          {isFetching ? (
            <Spinner className='size-3.5' />
          ) : (
            <RefreshCw className='size-3.5' />
          )}
          <span>{t('Refresh Status')}</span>
        </Button>
      </div>

      {/* Degraded 严重告警 */}
      {runtime.degraded && (
        <Alert variant='destructive'>
          <ShieldAlert className='text-destructive size-5' />
          <AlertTitle className='font-semibold'>
            {t('Prompt Audit Service Degraded')}
          </AlertTitle>
          <AlertDescription className='mt-1 text-sm leading-relaxed'>
            <p>
              {t(
                'The audit service is currently in degraded fail-closed mode. Requests matching audit scope will be rejected with HTTP 503 until configuration or key errors are resolved.'
              )}
            </p>
            {runtime.config_load_error && (
              <p className='bg-destructive/10 mt-2 rounded p-2 font-mono text-xs'>
                {t('Error code')}: {runtime.config_load_error}
              </p>
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* 核心指标栅格 */}
      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        {/* 有效模式 */}
        <Card>
          <CardHeader className='flex flex-row items-center justify-between pb-2'>
            <CardTitle className='text-muted-foreground text-sm font-medium'>
              {t('Effective Mode')}
            </CardTitle>
            {isBlocking ? (
              <Shield className='text-primary size-4' />
            ) : (
              <ShieldOff className='text-muted-foreground size-4' />
            )}
          </CardHeader>
          <CardContent>
            <div className='flex items-center gap-2'>
              <span className='text-2xl font-bold'>
                {isBlocking ? t('Blocking') : t('Off')}
              </span>
              <Badge variant={isBlocking ? 'default' : 'secondary'}>
                {isBlocking ? t('Active') : t('Inactive')}
              </Badge>
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {isBlocking
                ? t('Synchronous blocking before upstream')
                : t('Prompt audit gate is disabled')}
            </p>
          </CardContent>
        </Card>

        {/* 激活配置版本 */}
        <Card>
          <CardHeader className='flex flex-row items-center justify-between pb-2'>
            <CardTitle className='text-muted-foreground text-sm font-medium'>
              {t('Config Version')}
            </CardTitle>
            <CheckCircle2 className='text-muted-foreground size-4' />
          </CardHeader>
          <CardContent>
            <div className='flex items-center gap-2'>
              <span className='font-mono text-2xl font-bold'>
                v{runtime.active_config_version}
              </span>
              {hasVersionMismatch && (
                <Badge variant='warning'>{t('Syncing')}</Badge>
              )}
            </div>
            <p className='text-muted-foreground mt-1 font-mono text-xs'>
              {t('Expected')}: v{runtime.expected_config_version}
            </p>
          </CardContent>
        </Card>

        {/* 启用节点 */}
        <Card>
          <CardHeader className='flex flex-row items-center justify-between pb-2'>
            <CardTitle className='text-muted-foreground text-sm font-medium'>
              {t('Active Endpoints')}
            </CardTitle>
            <Server className='text-muted-foreground size-4' />
          </CardHeader>
          <CardContent>
            <div className='font-mono text-2xl font-bold'>
              {runtime.enabled_endpoints}
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {runtime.enabled_endpoints > 0
                ? t('Configured and ready for failover')
                : t('No active endpoint available')}
            </p>
          </CardContent>
        </Card>

        {/* 加载时间 */}
        <Card>
          <CardHeader className='flex flex-row items-center justify-between pb-2'>
            <CardTitle className='text-muted-foreground text-sm font-medium'>
              {t('Loaded At')}
            </CardTitle>
            <Clock className='text-muted-foreground size-4' />
          </CardHeader>
          <CardContent>
            <div className='text-sm font-medium'>
              {runtime.config_loaded_at
                ? dayjs(runtime.config_loaded_at).format('YYYY-MM-DD HH:mm:ss')
                : t('Not loaded yet')}
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {runtime.config_loaded_at
                ? dayjs(runtime.config_loaded_at).fromNow?.() || ''
                : ''}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* 未覆盖插件提示 */}
      {runtime.uncovered_plugins && runtime.uncovered_plugins.length > 0 && (
        <Alert className='border-warning/50 bg-warning/5 text-card-foreground'>
          <AlertTriangle className='text-warning size-5' />
          <AlertTitle className='font-medium'>
            {t('Uncovered Task Plugins')}
          </AlertTitle>
          <AlertDescription className='mt-2 space-y-2'>
            <p className='text-sm'>
              {t(
                'The following submit plugins do not declare prompt audit paths. When global audit is enabled, submissions to these plugins will fail-closed with 503:'
              )}
            </p>
            <div className='flex flex-wrap gap-1.5 pt-1'>
              {runtime.uncovered_plugins.map((plugin) => (
                <Badge key={plugin} variant='outline' className='font-mono'>
                  {plugin}
                </Badge>
              ))}
            </div>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
