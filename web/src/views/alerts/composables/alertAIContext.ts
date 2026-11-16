import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAIContextStore } from '@/stores/aiContext'
import type { Alert } from '@/api/alert'

/**
 * 告警页与 AI 助手的上下文桥接：
 * - 打开告警详情时同步页面上下文（告警/应用/资源），供 AI 助手自动带入；
 * - 关闭详情时清理资源上下文；
 * - 支持 `/alerts?alert_id=...` 深链（AI 引用跳转）打开详情。
 */
export function syncAlertAIContext(alert?: Partial<Alert> | null) {
  const store = useAIContextStore()
  if (!alert?.id) {
    store.clearResourceContext('alerts', '告警中心')
    return
  }
  store.setPageContext({
    page: 'alerts',
    page_title: '告警详情',
    alert_id: alert.id,
    application_id: alert.application_id,
    resource_id: alert.resource_id,
    resource_type: alert.resource_type,
    time_range: '30m'
  })
}

export function clearAlertAIContext() {
  useAIContextStore().clearResourceContext('alerts', '告警中心')
}

/** 页面挂载后按 query 打开详情，并监听同一页面内的引用跳转。 */
export function useAlertDeepLink(openDetail: (alertId: string) => Promise<void>) {
  const route = useRoute()
  const router = useRouter()

  function currentDeepLink(): string {
    return typeof route.query.alert_id === 'string' ? route.query.alert_id : ''
  }

  async function openFromQuery() {
    const alertId = currentDeepLink()
    if (alertId) await openDetail(alertId)
  }

  /** 关闭详情时移除深链参数，保证再次点击同一引用仍能打开。 */
  function resetDeepLink() {
    if (route.query.alert_id) {
      router.replace({ path: '/alerts' })
    }
  }

  watch(
    () => route.query.alert_id,
    async (value) => {
      if (typeof value !== 'string' || !value) return
      await openDetail(value)
    }
  )
  onMounted(openFromQuery)
  return { resetDeepLink }
}
