import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as identityApi from '@/api/identity'
import type { TokenPair } from '@/api/identity'

const TOKEN_KEY = 'aiops_token'
const REFRESH_KEY = 'aiops_refresh_token'
const USER_KEY = 'aiops_user'

export interface CurrentUser {
  id: string
  username: string
  display_name?: string
  email?: string
  status: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>('')
  const refreshToken = ref<string>('')
  const user = ref<CurrentUser | null>(null)

  function setToken(t: string) {
    token.value = t
    if (t) localStorage.setItem(TOKEN_KEY, t)
    else localStorage.removeItem(TOKEN_KEY)
  }

  function setRefreshToken(t: string) {
    refreshToken.value = t
    if (t) localStorage.setItem(REFRESH_KEY, t)
    else localStorage.removeItem(REFRESH_KEY)
  }

  function setUser(u: CurrentUser | null) {
    user.value = u
    if (u) localStorage.setItem(USER_KEY, JSON.stringify(u))
    else localStorage.removeItem(USER_KEY)
  }

  function applyTokenPair(pair: TokenPair) {
    setToken(pair.access_token)
    setRefreshToken(pair.refresh_token)
    setUser(pair.user)
  }

  function loadFromStorage() {
    const t = localStorage.getItem(TOKEN_KEY)
    if (t) token.value = t
    const rt = localStorage.getItem(REFRESH_KEY)
    if (rt) refreshToken.value = rt
    const u = localStorage.getItem(USER_KEY)
    if (u) {
      try {
        user.value = JSON.parse(u)
      } catch {
        user.value = null
      }
    }
  }

  async function login(username: string, password: string, providerId?: string) {
    const pair = providerId
      ? await identityApi.loginExternal({ provider_id: providerId, username, password })
      : await identityApi.login({ username, password })
    applyTokenPair(pair)
    return pair
  }

  async function loginWithOAuthCallback(providerId: string, code: string, state?: string) {
    const pair = await identityApi.completeOAuthCallback(providerId, code, state)
    applyTokenPair(pair)
    return pair
  }

  let refreshInFlight: Promise<TokenPair> | null = null

  async function refresh() {
    if (!refreshToken.value) {
      throw new Error('no refresh token')
    }
    if (refreshInFlight) {
      return refreshInFlight
    }
    refreshInFlight = identityApi
      .refresh({ refresh_token: refreshToken.value })
      .then((pair) => {
        applyTokenPair(pair)
        return pair
      })
      .finally(() => {
        refreshInFlight = null
      })
    return refreshInFlight
  }

  async function logout() {
    const rt = refreshToken.value
    setToken('')
    setRefreshToken('')
    setUser(null)
    if (rt) {
      try {
        await identityApi.logout(rt)
      } catch {
        /* 本地会话已清除，远端吊销失败可忽略。 */
      }
    }
  }

  return {
    token,
    refreshToken,
    user,
    setToken,
    setRefreshToken,
    setUser,
    applyTokenPair,
    loadFromStorage,
    login,
    loginWithOAuthCallback,
    refresh,
    logout
  }
})
