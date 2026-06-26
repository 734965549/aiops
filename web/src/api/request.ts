import axios, { AxiosError, type AxiosInstance, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import Message from '@arco-design/web-vue/es/message'
import { useAuthStore } from '@/stores/auth'

// 与后端 pkg/transport/http.Response 对齐的统一响应结构。
export interface ApiResponse<T = unknown> {
  code: string
  message: string
  trace_id?: string
  data?: T
}

export class ApiHttpError extends Error {
  status: number
  code: string
  traceId?: string

  constructor(status: number, code: string, message: string, traceId?: string) {
    super(message)
    this.name = 'ApiHttpError'
    this.status = status
    this.code = code
    this.traceId = traceId
  }
}

type RetryableConfig = InternalAxiosRequestConfig & { _retry?: boolean }

const MIGRATION_HINT =
  '若已登录仍持续 403，请确认已执行迁移 0002（admin 权限）与 0004（域账号导入权限），参见 ops/migration-contract.md'

const apiBase = import.meta.env.VITE_API_BASE || '/'

const request: AxiosInstance = axios.create({
  baseURL: apiBase,
  timeout: 30_000
})

function isAuthPublicPath(url?: string) {
  if (!url) return false
  return (
    url.includes('/api/identity/login') ||
    url.includes('/api/identity/refresh') ||
    url.includes('/api/identity/logout')
  )
}

export function getApiError(error: unknown): ApiHttpError | null {
  if (!axios.isAxiosError(error) || !error.response) return null
  const body = error.response.data as ApiResponse | undefined
  return new ApiHttpError(
    error.response.status,
    body?.code || 'UNKNOWN',
    body?.message || error.message,
    body?.trace_id
  )
}

async function redirectToLogin() {
  if (window.location.pathname === '/login') return
  const { default: router } = await import('@/router')
  const current = router.currentRoute.value
  if (current.meta.public) return
  router.push({ path: '/login', query: { redirect: current.fullPath } })
}

function permissionDeniedMessage(body?: ApiResponse) {
  const detail = body?.message || '当前账号缺少所需权限'
  if (body?.code === 'PERMISSION_DENIED') {
    return `权限不足：${detail}。${MIGRATION_HINT}`
  }
  return `权限不足：${detail}`
}

request.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const auth = useAuthStore()
  if (auth.token && config.headers) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

request.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => {
    const body = response.data
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code === 'OK') return response
      Message.error(body.message || '请求失败')
      return Promise.reject(new ApiHttpError(200, body.code, body.message || body.code, body.trace_id))
    }
    return response
  },
  async (error: AxiosError<ApiResponse>) => {
    if (error.response) {
      const body = error.response.data
      const status = error.response.status
      const original = error.config as RetryableConfig | undefined
      const apiError = new ApiHttpError(
        status,
        body?.code || 'UNKNOWN',
        body?.message || `${status} ${error.response.statusText}`,
        body?.trace_id
      )

      if (status === 401 && original && !isAuthPublicPath(original.url)) {
        const auth = useAuthStore()
        if (auth.refreshToken && !original._retry) {
          original._retry = true
          try {
            await auth.refresh()
            if (original.headers) {
              original.headers.Authorization = `Bearer ${auth.token}`
            }
            return request(original)
          } catch {
            /* refresh 失败，继续走登出逻辑。 */
          }
        }
        await auth.logout()
        redirectToLogin()
        Message.error(body?.message || '登录已失效，请重新登录')
        return Promise.reject(apiError)
      }

      if (status === 403) {
        Message.warning(permissionDeniedMessage(body))
        return Promise.reject(apiError)
      }

      if (status === 503 || body?.code === 'UNAVAILABLE') {
        Message.warning(body?.message || '服务暂不可用，请检查后端配置与依赖')
        return Promise.reject(apiError)
      }

      Message.error(body?.message || `${status} ${error.response.statusText}`)
      return Promise.reject(apiError)
    }

    Message.error(error.message || '网络异常')
    return Promise.reject(error)
  }
)

// 业务调用方期望直接拿到 data 字段，这里做一层泛型解包。
export async function http<T = unknown>(config: Parameters<AxiosInstance['request']>[0]): Promise<T> {
  const resp = await request.request<ApiResponse<T>>(config)
  return resp.data.data as T
}

export default request
