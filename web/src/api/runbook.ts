import { http } from './request'

export interface RunbookRecommendation {
  template_id: string
  name: string
  description: string
  risk_level: string
  operation_type: string
  matched_reason: string
  steps_count: number
  dry_run_supported: boolean
  parameter_schema: Record<string, unknown>
}

export interface RunbookTemplate {
  template_id: string
  name: string
  description: string
  enabled: boolean
  operation_type: string
  risk_level: string
  match_alert_name?: string
  match_resource_type?: string
  match_environment?: string
  parameter_schema: Record<string, unknown>
  rollback_plan?: Record<string, unknown>
  created_by?: string
  created_at: number
  updated_at: number
}

export interface RunbookStep {
  step_id: string
  template_id: string
  step_order: number
  name: string
  action_type: string
  risk_level: string
  dry_run_supported: boolean
  default_dry_run: boolean
  parameter_schema: Record<string, unknown>
  default_parameters: Record<string, unknown>
  rollback_plan?: Record<string, unknown>
  timeout_seconds: number
}

export interface RunbookTemplateDetail {
  template: RunbookTemplate
  steps: RunbookStep[]
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export function listRunbookRecommendations(alertId: string) {
  return http<{ items: RunbookRecommendation[] }>({
    url: '/api/runbooks/recommendations',
    method: 'get',
    params: { alert_id: alertId }
  })
}

export function listRunbookTemplates(query: { page?: number; page_size?: number; keyword?: string } = {}) {
  return http<PageResult<RunbookTemplate>>({
    url: '/api/runbooks/templates',
    method: 'get',
    params: query
  })
}

export function getRunbookTemplate(templateId: string) {
  return http<RunbookTemplateDetail>({
    url: `/api/runbooks/templates/${encodeURIComponent(templateId)}`,
    method: 'get'
  })
}

export function updateRunbookTemplate(templateId: string, data: Record<string, unknown>) {
  return http<RunbookTemplateDetail>({
    url: `/api/runbooks/templates/${encodeURIComponent(templateId)}`,
    method: 'put',
    data
  })
}
