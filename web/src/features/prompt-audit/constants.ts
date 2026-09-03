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
export const PROMPT_AUDIT_QUERY_KEYS = {
  config: ['prompt-audit', 'config'] as const,
  runtime: ['prompt-audit', 'runtime'] as const,
  events: (filters: Record<string, unknown>) =>
    ['prompt-audit', 'events', filters] as const,
  eventDetail: (id: number) => ['prompt-audit', 'event-detail', id] as const,
}

export const ERROR_CODE_CONFIG_CONFLICT = 'prompt_audit_config_conflict'

export const DEFAULT_GUARD_MODEL = 'sileader/qwen3guard:0.6b'
export const DEFAULT_GUARD_PROTOCOL = 'openai_compatible'
export const DEFAULT_TIMEOUT_MS = 3000
export const DEFAULT_INPUT_LIMIT = 4000
export const MIN_TIMEOUT_MS = 100
export const MAX_TIMEOUT_MS = 30000
export const MIN_INPUT_LIMIT = 128
export const MAX_INPUT_LIMIT = 100000

export const DEFAULT_SCANNER_IDS = [
  'violent',
  'non_violent_illegal_acts',
  'sexual_content_or_sexual_acts',
  'pii',
  'suicide_and_self_harm',
  'unethical_acts',
  'politically_sensitive_topics',
  'copyright_violation',
  'jailbreak',
] as const

export const SCANNER_LABEL_KEYS: Record<string, string> = {
  violent: 'Violent',
  non_violent_illegal_acts: 'Non-violent Illegal Acts',
  sexual_content_or_sexual_acts: 'Sexual Content or Sexual Acts',
  pii: 'Personally Identifiable Information',
  suicide_and_self_harm: 'Suicide and Self-harm',
  unethical_acts: 'Unethical Acts',
  politically_sensitive_topics: 'Politically Sensitive Topics',
  copyright_violation: 'Copyright Violation',
  jailbreak: 'Jailbreak',
}

export const DECISION_CONFIG: Record<
  string,
  {
    labelKey: string
    variant: 'default' | 'secondary' | 'destructive' | 'warning' | 'outline'
  }
> = {
  allow: { labelKey: 'Allow', variant: 'secondary' },
  warn: { labelKey: 'Warn', variant: 'warning' },
  block: { labelKey: 'Block', variant: 'destructive' },
  error: { labelKey: 'Error', variant: 'destructive' },
}

export const RISK_LEVEL_CONFIG: Record<
  string,
  {
    labelKey: string
    variant: 'default' | 'secondary' | 'destructive' | 'warning' | 'outline'
  }
> = {
  low: { labelKey: 'Low Risk', variant: 'secondary' },
  medium: { labelKey: 'Medium Risk', variant: 'warning' },
  high: { labelKey: 'High Risk', variant: 'destructive' },
}

export const TOKEN_STATUS_CONFIG: Record<
  string,
  {
    labelKey: string
    variant: 'default' | 'secondary' | 'destructive' | 'warning' | 'outline'
  }
> = {
  configured: { labelKey: 'Configured', variant: 'secondary' },
  missing: { labelKey: 'Missing Token', variant: 'warning' },
  invalid: { labelKey: 'Invalid Key', variant: 'destructive' },
}

export const PROMPT_AUDIT_TABS = {
  RUNTIME: 'runtime',
  POLICY: 'policy',
  ENDPOINTS: 'endpoints',
  EVENTS: 'events',
} as const
