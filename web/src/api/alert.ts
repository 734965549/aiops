import { http } from './request'

export type AlertSeverity = 'p0' | 'p1' | 'p2' | 'p3' | 'info'
export type AlertStatus =
  | 'new'
  | 'acknowledged'
  | 'processing'
  | 'recovered'
  | 'closed'
  | 'silenced'

export type AlertEventType =
  | 'triggered'
  | 'updated'
  | 'recovered'
  | 'acknowledged'
  | 'assigned'
  | 'processing_started'
  | 'closed'
  | 'silenced'
  | 'unsilenced'
  | 'commented'
  | 'ai_analysis_requested'
  | 'execution_created'
  | 'execution_started'
  | 'execution_finished'

export interface Alert {
  id: string
  external_id?: string
  source: string
  source_id?: string
  source_name?: string
  fingerprint: string
  dedup_key: string
  name: string
  summary?: string
  description?: string
  severity: AlertSeverity | string
  status: AlertStatus | string
  rule_id?: string
  rule_name?: string
  business_line?: string
  environment?: string
  application_id?: string
  application_name?: string
  resource_id?: string
  resource_type?: string
  resource_name?: string
  owner_user_id?: string
  assignee_user_id?: string
  labels: Record<string, string>
  annotations: Record<string, string>
  occurrence_count: number
  first_seen_at: number
  last_seen_at: number
  recovered_at?: number
  acknowledged_at?: number
  closed_at?: number
  silenced_until?: number
  created_at: number
  updated_at: number
}

export interface AlertEvent {
  id: string
  alert_id: string
  event_type: AlertEventType | string
  actor_type: string
  actor_id?: string
  actor_name?: string
  message?: string
  payload: Record<string, unknown>
  created_at: number
}

export interface AlertDetail {
  alert: Alert
  events: AlertEvent[]
  related: Record<string, unknown>
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface AlertListQuery {
  page?: number
  page_size?: number
  status?: string
  severity?: string
  source?: string
  source_id?: string
  business_line?: string
  environment?: string
  application_id?: string
  resource_id?: string
  assignee_user_id?: string
  keyword?: string
  active_only?: boolean
  from?: number
  to?: number
}

export interface AlertSource {
  id: string
  name: string
  type: string
  enabled: boolean
  secret_masked?: string
  environment?: string
  business_line?: string
  description?: string
  created_at: number
  updated_at: number
}

export interface CreateAlertSourceInput {
  id: string
  name: string
  type?: string
  enabled?: boolean
  secret: string
  environment?: string
  business_line?: string
  description?: string
}

export interface UpdateAlertSourceInput {
  name?: string
  type?: string
  enabled?: boolean
  secret?: string
  environment?: string
  business_line?: string
  description?: string
}

export interface AIAnalysisInput {
  time_range?: string
  include_logs?: boolean
  include_metrics?: boolean
  include_changes?: boolean
}

export function listAlerts(params?: AlertListQuery) {
  return http<PageResult<Alert>>({
    url: '/api/alerts',
    method: 'get',
    params
  })
}

export function getAlert(alertId: string) {
  return http<AlertDetail>({
    url: `/api/alerts/${encodeURIComponent(alertId)}`,
    method: 'get'
  })
}

export function acknowledgeAlert(alertId: string, message?: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/acknowledge`,
    method: 'post',
    data: message ? { message } : {}
  })
}

export function assignAlert(alertId: string, assigneeUserId: string, message?: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/assign`,
    method: 'post',
    data: { assignee_user_id: assigneeUserId, message }
  })
}

export function startProcessingAlert(alertId: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/start-processing`,
    method: 'post',
    data: {}
  })
}

export function recoverAlert(alertId: string, message?: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/recover`,
    method: 'post',
    data: message ? { message } : {}
  })
}

export function closeAlert(alertId: string, resolution: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/close`,
    method: 'post',
    data: { resolution }
  })
}

export function silenceAlert(alertId: string, reason: string, durationS: number) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/silence`,
    method: 'post',
    data: { reason, duration_s: durationS }
  })
}

export function unsilenceAlert(alertId: string) {
  return http<Alert>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/unsilence`,
    method: 'post',
    data: {}
  })
}

export function commentAlert(alertId: string, message: string) {
  return http<AlertEvent>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/comments`,
    method: 'post',
    data: { message }
  })
}

export function requestAlertAIAnalysis(alertId: string, input?: AIAnalysisInput) {
  return http<AlertEvent>({
    url: `/api/alerts/${encodeURIComponent(alertId)}/ai-analysis`,
    method: 'post',
    data: input ?? {}
  })
}

export function listAlertSources() {
  return http<{ items: AlertSource[] }>({
    url: '/api/alerts/sources',
    method: 'get'
  })
}

export function getAlertSource(sourceId: string) {
  return http<AlertSource>({
    url: `/api/alerts/sources/${encodeURIComponent(sourceId)}`,
    method: 'get'
  })
}

export function createAlertSource(input: CreateAlertSourceInput) {
  return http<AlertSource>({
    url: '/api/alerts/sources',
    method: 'post',
    data: input
  })
}

export function updateAlertSource(sourceId: string, input: UpdateAlertSourceInput) {
  return http<AlertSource>({
    url: `/api/alerts/sources/${encodeURIComponent(sourceId)}`,
    method: 'put',
    data: input
  })
}

export function deleteAlertSource(sourceId: string) {
  return http<{ deleted: boolean }>({
    url: `/api/alerts/sources/${encodeURIComponent(sourceId)}`,
    method: 'delete'
  })
}
