import { http } from './request'

export interface VersionInfo {
  app: string
  env: string
  version: {
    version: string
    commit: string
    build_at: string
  }
}

/** 与 identity-api-contract.md §5.4 / 后端 CurrentUserDTO 对齐。 */
export interface CurrentUserDTO {
  id: string
  username: string
  display_name?: string
  email?: string
  status: string
}

export interface HealthCheck {
  name: string
  status: string
  error?: string
  details?: Record<string, unknown>
}

export interface ReadinessInfo {
  status: 'ready' | 'not_ready' | string
  checks: HealthCheck[]
  uptime_ms: number
}

export function fetchVersion() {
  return http<VersionInfo>({ url: '/version', method: 'get' })
}

export function fetchReadiness() {
  return http<ReadinessInfo>({ url: '/readyz', method: 'get' })
}

export function fetchCurrentUser() {
  return http<CurrentUserDTO>({ url: '/api/identity/me', method: 'get' })
}
