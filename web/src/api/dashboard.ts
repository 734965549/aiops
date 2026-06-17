import { http } from './request'
import type { ExecutionTask } from './execution'

export interface DashboardSummary {
  alerts: {
    active_total: number
    p0: number
    p1: number
  }
  executions: {
    pending_confirm: number
    recent: ExecutionTask[]
  }
  assets: {
    applications: number
    resources: number
  }
  runbooks: {
    total: number
    enabled: number
  }
  processing_alerts: Array<{
    id: string
    name: string
    severity: string
    status: string
  }>
}

export function fetchDashboardSummary() {
  return http<DashboardSummary>({
    url: '/api/dashboard/summary',
    method: 'get'
  })
}
