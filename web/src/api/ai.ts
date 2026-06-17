import { http } from './request'

export interface AIProvider {
  id: string
  name: string
  type: 'a' | 'b' | 'c' | string
  base_url: string
  api_key?: string
  has_api_key?: boolean
  timeout_ms?: number
  headers?: Record<string, string>
  enabled: boolean
  description?: string
}

export interface UpsertProviderInput {
  id: string
  name: string
  type: string
  base_url: string
  api_key?: string
  timeout_ms?: number
  headers?: Record<string, string>
  enabled?: boolean
  description?: string
}

export interface InvokeToolInput {
  provider_id: string
  tool_code: string
  resource?: string
  action?: string
  owner_id?: string
  dept?: string
  team?: string
  region?: string
  tags?: string[]
  confirmed?: boolean
  payload?: Record<string, unknown>
}

export interface InvokeToolResult {
  allowed: boolean
  reason?: string
  mode?: string
  provider?: string
  data?: Record<string, unknown>
}

export function listProviders() {
  return http<AIProvider[]>({ url: '/api/ai/providers', method: 'get' })
}

export function upsertProvider(input: UpsertProviderInput) {
  return http<{ updated: boolean }>({
    url: '/api/ai/providers',
    method: 'post',
    data: input
  })
}

export function deleteProvider(id: string) {
  return http<{ deleted: boolean }>({
    url: `/api/ai/providers/${encodeURIComponent(id)}`,
    method: 'delete'
  })
}

export function invokeTool(input: InvokeToolInput) {
  return http<InvokeToolResult>({
    url: '/api/ai/tools/invoke',
    method: 'post',
    data: input
  })
}

export interface AnalyzeAlertInput {
  alert_id: string
  time_range?: string
  include_logs?: boolean
  include_metrics?: boolean
  include_changes?: boolean
}

export interface AnalyzeAlertResult {
  conversation_id: string
  summary: string
  risk_level: string
  recommendations: string[]
  references: string[]
}

export function analyzeAlert(input: AnalyzeAlertInput) {
  return http<AnalyzeAlertResult>({
    url: '/api/ai/analyze-alert',
    method: 'post',
    data: input
  })
}
