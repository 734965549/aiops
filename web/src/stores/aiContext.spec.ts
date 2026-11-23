// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAIContextStore } from './aiContext'

describe('aiContext store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('keeps resource ids and drops credentials-like free text', () => {
    const store = useAIContextStore()
    store.setPageContext({
      page: 'assets',
      page_title: ' 资源 · ecs-1 ',
      application_id: ' app-1 ',
      resource_id: ' i-1 ',
      resource_type: 'ecs'
    })
    expect(store.current.application_id).toBe('app-1')
    expect(store.current.resource_id).toBe('i-1')
    expect(store.current.resource_type).toBe('ecs')
    expect(store.current.page_title).toBe('资源 · ecs-1')
    expect(store.hasResourceContext).toBe(true)
  })

  it('treats execution_id as resource-level context', () => {
    const store = useAIContextStore()
    store.setPageContext({ page: 'executions', page_title: '执行任务 · 重启服务', execution_id: ' task-1 ' })
    expect(store.current.execution_id).toBe('task-1')
    expect(store.hasResourceContext).toBe(true)
    expect(store.summary).toContain('执行任务 task-1')
    const payload = store.toRequestContext()
    expect(payload.execution_id).toBe('task-1')
  })

  it('clears resource context when switching pages', () => {
    const store = useAIContextStore()
    store.setPageContext({ page: 'alerts', alert_id: 'a-1', resource_id: 'r-1' })
    store.clearResourceContext('executions', '执行中心')
    expect(store.current.alert_id).toBeUndefined()
    expect(store.current.resource_id).toBeUndefined()
    expect(store.current.execution_id).toBeUndefined()
    expect(store.hasResourceContext).toBe(false)
    expect(store.summary).toBe('执行中心')
  })

  it('keeps resource context when re-entering the same page base', () => {
    const store = useAIContextStore()
    store.setPageContext({ page: 'assets', application_id: 'app-1' })
    store.setPageBase('assets', '资产管理')
    expect(store.current.application_id).toBe('app-1')
    expect(store.current.page_title).toBe('资产管理')
  })

  it('resets context when the page base changes', () => {
    const store = useAIContextStore()
    store.setPageContext({ page: 'assets', application_id: 'app-1', resource_id: 'i-1' })
    store.setPageBase('executions', '执行中心')
    expect(store.current.application_id).toBeUndefined()
    expect(store.current.resource_id).toBeUndefined()
    expect(store.current.page).toBe('executions')
  })

  it('enables the session picker only for an explicit overlay entry', () => {
    const store = useAIContextStore()
    store.openOverlay({ selectSession: true })
    expect(store.overlayVisible).toBe(true)
    expect(store.overlaySessionPicker).toBe(true)

    store.closeOverlay()
    store.toggleOverlay()
    expect(store.overlayVisible).toBe(true)
    expect(store.overlaySessionPicker).toBe(false)

    store.closeOverlay()
    store.openOverlay()
    expect(store.overlayVisible).toBe(true)
    expect(store.overlaySessionPicker).toBe(false)
  })
})