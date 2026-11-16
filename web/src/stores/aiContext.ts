import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { ChatSessionContext } from '@/api/ai'

/**
 * 当前页面上下文：AI 助手（独立页与全局浮层）自动带入页面资源。
 * 只保存业务 ID 和标题，不保存凭据、告警内容或自由文本。
 */
export interface AIPageContext extends ChatSessionContext {
  page: string
  page_title: string
}

export const useAIContextStore = defineStore('aiContext', () => {
  const current = ref<AIPageContext>({ page: '', page_title: '' })
  const overlayVisible = ref(false)

  const hasResourceContext = computed(
    () => !!(current.value.alert_id || current.value.application_id || current.value.resource_id)
  )

  const summary = computed(() => {
    const parts: string[] = []
    if (current.value.page_title) parts.push(current.value.page_title)
    if (current.value.alert_id) parts.push(`告警 ${current.value.alert_id}`)
    if (current.value.application_id) parts.push(`应用 ${current.value.application_id}`)
    if (current.value.resource_id && !current.value.alert_id) parts.push(`资源 ${current.value.resource_id}`)
    if (current.value.time_range) parts.push(`时间范围 ${current.value.time_range}`)
    return parts.length ? parts.join(' · ') : '未关联页面资源'
  })

  function setPageContext(context: Partial<AIPageContext>) {
    const next: AIPageContext = {
      page: (context.page ?? current.value.page ?? '').trim(),
      page_title: (context.page_title ?? '').trim(),
      alert_id: trimOrUndefined(context.alert_id),
      application_id: trimOrUndefined(context.application_id),
      resource_id: trimOrUndefined(context.resource_id),
      resource_type: trimOrUndefined(context.resource_type),
      time_range: trimOrUndefined(context.time_range)
    }
    current.value = next
  }

  /** 路由切换时设置基础上下文；同一页面保留已登记的资源上下文。 */
  function setPageBase(page: string, pageTitle: string) {
    const normalized = (page || '').trim()
    if (!normalized) return
    if (normalized === current.value.page) {
      current.value = { ...current.value, page_title: pageTitle || current.value.page_title }
      return
    }
    current.value = { page: normalized, page_title: pageTitle || '' }
  }

  function clearResourceContext(page: string, pageTitle: string) {
    current.value = { page, page_title: pageTitle }
  }

  function toRequestContext(): ChatSessionContext {
    const payload: ChatSessionContext = {}
    if (current.value.page) payload.page = current.value.page
    if (current.value.page_title) payload.page_title = current.value.page_title
    if (current.value.alert_id) payload.alert_id = current.value.alert_id
    if (current.value.application_id) payload.application_id = current.value.application_id
    if (current.value.resource_id) payload.resource_id = current.value.resource_id
    if (current.value.resource_type) payload.resource_type = current.value.resource_type
    if (current.value.time_range) payload.time_range = current.value.time_range
    return payload
  }

  function openOverlay() {
    overlayVisible.value = true
  }

  function closeOverlay() {
    overlayVisible.value = false
  }

  function toggleOverlay() {
    overlayVisible.value = !overlayVisible.value
  }

  return {
    current,
    overlayVisible,
    hasResourceContext,
    summary,
    setPageContext,
    setPageBase,
    clearResourceContext,
    toRequestContext,
    openOverlay,
    closeOverlay,
    toggleOverlay
  }
})

function trimOrUndefined(value?: string): string | undefined {
  const trimmed = (value ?? '').trim()
  return trimmed || undefined
}
