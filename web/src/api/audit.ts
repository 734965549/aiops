import { http } from './request'

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface OperationAudit {
  id: string
  user_id?: string
  resource_type: string
  resource_id: string
  action: string
  payload: Record<string, unknown>
  ip?: string
  user_agent?: string
  created_at: number
}

export interface AuditListQuery {
  page?: number
  page_size?: number
  resource_type?: string
  resource_id?: string
  user_id?: string
  action?: string
}

export function fetchAudits(query: AuditListQuery = {}) {
  return http<PageResult<OperationAudit>>({
    url: '/api/audits',
    method: 'get',
    params: query
  })
}
