import { http } from './request'
import type { PageResult } from './audit'

export type IntegrationProvider = 'huawei_cloud' | 'signoz' | 'prometheus'
export type IntegrationAuthType = 'ak_sk' | 'agency' | 'api_token' | 'none'

export interface IntegrationAccount {
  account_id: string
  name: string
  provider: IntegrationProvider | string
  auth_type: IntegrationAuthType | string
  regions?: string[]
  project_id?: string
  has_credential: boolean
  enabled: boolean
  owner_team?: string
  description?: string
  capabilities?: string[]
  last_check_status?: string
  created_at: number
  updated_at: number
}

export interface ConnectivityCheckResult {
  status: string
  provider: string
  capabilities: string[]
  checked_at: number
  message: string
}

export interface ListAccountsQuery {
  page?: number
  page_size?: number
  provider?: string
  enabled?: boolean
  keyword?: string
}

export interface CreateAccountInput {
  account_id?: string
  name: string
  provider: string
  auth_type: string
  regions?: string[]
  project_id?: string
  credential?: Record<string, string>
  enabled?: boolean
  owner_team?: string
  description?: string
}

export interface UpdateAccountInput {
  name?: string
  provider?: string
  auth_type?: string
  regions?: string[]
  project_id?: string
  credential?: Record<string, string>
  enabled?: boolean
  owner_team?: string
  description?: string
}

export function listIntegrationAccounts(query: ListAccountsQuery = {}) {
  return http<PageResult<IntegrationAccount>>({
    url: '/api/integrations/accounts',
    method: 'get',
    params: query
  })
}

export function getIntegrationAccount(accountId: string) {
  return http<IntegrationAccount>({
    url: `/api/integrations/accounts/${encodeURIComponent(accountId)}`,
    method: 'get'
  })
}

export function createIntegrationAccount(input: CreateAccountInput) {
  return http<IntegrationAccount>({
    url: '/api/integrations/accounts',
    method: 'post',
    data: input
  })
}

export function updateIntegrationAccount(accountId: string, input: UpdateAccountInput) {
  return http<IntegrationAccount>({
    url: `/api/integrations/accounts/${encodeURIComponent(accountId)}`,
    method: 'put',
    data: input
  })
}

export function deleteIntegrationAccount(accountId: string) {
  return http<{ account_id: string; deleted: boolean }>({
    url: `/api/integrations/accounts/${encodeURIComponent(accountId)}`,
    method: 'delete'
  })
}

export function checkIntegrationAccount(accountId: string) {
  return http<ConnectivityCheckResult>({
    url: `/api/integrations/accounts/${encodeURIComponent(accountId)}/check`,
    method: 'post'
  })
}
