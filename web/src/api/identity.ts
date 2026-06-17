import axios, { type AxiosResponse } from 'axios'
import type { ApiResponse } from './request'
import type { CurrentUser } from '@/stores/auth'

const apiBase = import.meta.env.VITE_API_BASE || '/'

/** 公开接口使用独立 client，避免与鉴权拦截器循环依赖。 */
const publicClient = axios.create({
  baseURL: apiBase,
  timeout: 30_000
})

async function unwrapPublic<T>(promise: Promise<AxiosResponse<ApiResponse<T>>>): Promise<T> {
  const resp = await promise
  const body = resp.data
  if (body?.code !== 'OK') {
    throw new Error(body?.message || body?.code || 'request failed')
  }
  return body.data as T
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  access_expires_at: number
  refresh_expires_at: number
  token_type: string
  user: CurrentUser
}

export interface LoginInput {
  username: string
  password: string
  provider_id?: string
}

export interface ExternalLoginInput {
  provider_id: string
  username: string
  password: string
}

export interface IdentityProviderInfo {
  id: string
  type: 'ldap' | 'ad' | 'oauth2' | 'oidc' | 'sso' | 'local'
  name: string
  enabled: boolean
  priority: number
}

export interface LoginProvidersResponse {
  providers: IdentityProviderInfo[]
}

export interface OAuthAuthorizeResponse {
  authorization_url: string
  state: string
}

export interface RefreshInput {
  refresh_token: string
}

export interface AuthorizationInput {
  resource?: string
  action?: string
  object_owner?: string
  object_dept?: string
  object_team?: string
  object_region?: string
  object_tags?: string[]
  tool_code?: string
  require_confirmed?: boolean
  required_permission?: string
}

export interface AuthorizationResult {
  allowed: boolean
  reason?: string
  matched_role_names?: string[]
  matched_permissions?: string[]
  matched_scopes?: string[]
  tool_mode?: string
}

export function login(input: LoginInput) {
  return unwrapPublic(
    publicClient.post<ApiResponse<TokenPair>>('/api/identity/login', input)
  )
}

export function loginExternal(input: ExternalLoginInput) {
  return unwrapPublic(
    publicClient.post<ApiResponse<TokenPair>>('/api/identity/login/external', input)
  )
}

export function fetchLoginProviders() {
  return unwrapPublic(
    publicClient.get<ApiResponse<LoginProvidersResponse>>('/api/identity/login/providers')
  )
}

export function fetchOAuthAuthorizeURL(providerId: string) {
  return unwrapPublic(
    publicClient.get<ApiResponse<OAuthAuthorizeResponse>>(`/api/identity/oauth/${encodeURIComponent(providerId)}/authorize`)
  )
}

export function completeOAuthCallback(providerId: string, code: string, state?: string) {
  return unwrapPublic(
    publicClient.post<ApiResponse<TokenPair>>(`/api/identity/oauth/${encodeURIComponent(providerId)}/callback`, {
      provider_id: providerId,
      code,
      state
    })
  )
}

export function refresh(input: RefreshInput) {
  return unwrapPublic(
    publicClient.post<ApiResponse<TokenPair>>('/api/identity/refresh', input)
  )
}

export function logout(refreshToken: string) {
  return unwrapPublic(
    publicClient.post<ApiResponse<{ logged_out: boolean }>>('/api/identity/logout', {
      refresh_token: refreshToken
    })
  )
}

export async function authorize(input: AuthorizationInput) {
  const { http } = await import('./request')
  return http<AuthorizationResult>({
    url: '/api/identity/authorize',
    method: 'post',
    data: input
  })
}
