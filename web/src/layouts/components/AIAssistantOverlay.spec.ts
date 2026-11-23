// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, provide } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AIAssistantOverlay from './AIAssistantOverlay.vue'
import { ChatControllerKey, useChat } from '@/views/ai-assistant/composables/useChat'
import {
  createChatSession,
  getChatSession,
  listChatModels,
  listChatSessions,
  type ChatSession
} from '@/api/ai'
import { useAIContextStore } from '@/stores/aiContext'

vi.mock('@/api/ai', () => ({
  archiveChatSession: vi.fn(),
  createChatSession: vi.fn(),
  deleteChatSession: vi.fn(),
  getChatSession: vi.fn(),
  listChatModels: vi.fn(),
  listChatSessions: vi.fn(),
  sendChatFeedback: vi.fn(),
  streamChatMessage: vi.fn(),
  unarchiveChatSession: vi.fn()
}))
vi.mock('@/api/execution', () => ({ createExecutionTask: vi.fn() }))
vi.mock('@/api/request', () => ({
  getApiError: vi.fn(() => null),
  isApiHttpError: vi.fn(() => false)
}))
vi.mock('@/api/sse', () => ({ isAbortError: vi.fn(() => false) }))
vi.mock('vue-router', () => ({
  useRouter: vi.fn(() => ({ push: vi.fn() }))
}))
vi.mock('@arco-design/web-vue/es/message', () => ({
  default: {
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn()
  }
}))
vi.mock('@arco-design/web-vue/es/icon', () => ({
  IconLocation: { template: '<span />' }
}))
vi.mock('@arco-design/web-vue', () => ({
  Drawer: {
    props: ['visible'],
    template: '<div><slot name="title" /><slot /></div>'
  },
  Link: { template: '<a><slot /></a>' },
  Tag: { template: '<span><slot /></span>' }
}))
vi.mock('@/views/ai-assistant/components/ChatPanel.vue', () => ({
  default: { name: 'ChatPanel', template: '<div class="chat-panel-stub" />' }
}))
vi.mock('@/views/ai-assistant/components/SessionList.vue', () => ({
  default: {
    name: 'SessionList',
    props: ['sessions', 'activeId', 'loading'],
    emits: ['create', 'select', 'refresh'],
    template: `
      <div class="session-list-stub">
        <button class="create-session" @click="$emit('create')">新建会话</button>
        <button
          v-for="session in sessions"
          :key="session.session_id"
          class="select-session"
          @click="$emit('select', session.session_id)"
        >{{ session.title }}</button>
      </div>
    `
  }
}))

function makeSession(sessionId: string, title = sessionId): ChatSession {
  return {
    session_id: sessionId,
    title,
    status: 'active',
    context: {},
    created_at: '2026-11-23T10:00:00Z',
    updated_at: '2026-11-23T10:00:00Z'
  }
}

function mountOverlay() {
  const Host = defineComponent({
    setup() {
      provide(ChatControllerKey, useChat())
      return () => h(AIAssistantOverlay)
    }
  })
  return mount(Host)
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(listChatModels).mockResolvedValue([])
  vi.mocked(listChatSessions).mockResolvedValue([
    makeSession('session-1', '最近会话'),
    makeSession('session-2', '较早会话')
  ])
  vi.mocked(getChatSession).mockImplementation(async (sessionId) => ({
    session: makeSession(sessionId),
    messages: []
  }))
  vi.mocked(createChatSession).mockResolvedValue(makeSession('new-session', '新会话'))
})

describe('AIAssistantOverlay session picker', () => {
  it('loads choices without automatically opening the first session', async () => {
    const store = useAIContextStore()
    const wrapper = mountOverlay()

    store.openOverlay({ selectSession: true })
    await flushPromises()

    expect(listChatSessions).toHaveBeenCalledWith(20)
    expect(getChatSession).not.toHaveBeenCalled()
    expect(wrapper.find('.session-picker').exists()).toBe(true)
    expect(wrapper.findAll('.select-session')).toHaveLength(2)
    expect(wrapper.find('.chat-panel-stub').exists()).toBe(false)
  })

  it('opens the selected historical session', async () => {
    const store = useAIContextStore()
    const wrapper = mountOverlay()
    store.openOverlay({ selectSession: true })
    await flushPromises()

    await wrapper.findAll('.select-session')[1].trigger('click')
    await flushPromises()

    expect(getChatSession).toHaveBeenCalledWith('session-2')
    expect(wrapper.find('.session-picker').exists()).toBe(false)
    expect(wrapper.find('.chat-panel-stub').exists()).toBe(true)
  })

  it('creates a new session with the current alert context', async () => {
    const store = useAIContextStore()
    store.setPageContext({ page: 'alerts', page_title: '告警中心', alert_id: 'alert-1' })
    const wrapper = mountOverlay()
    store.openOverlay({ selectSession: true })
    await flushPromises()

    await wrapper.find('.create-session').trigger('click')
    await flushPromises()

    expect(createChatSession).toHaveBeenCalledWith({
      title: undefined,
      context: expect.objectContaining({ alert_id: 'alert-1' })
    })
    expect(wrapper.find('.session-picker').exists()).toBe(false)
    expect(wrapper.find('.chat-panel-stub').exists()).toBe(true)
  })

  it('keeps the default entry behavior unchanged', async () => {
    const store = useAIContextStore()
    const wrapper = mountOverlay()

    store.openOverlay()
    await flushPromises()

    expect(getChatSession).toHaveBeenCalledWith('session-1')
    expect(wrapper.find('.session-picker').exists()).toBe(false)
    expect(wrapper.find('.chat-panel-stub').exists()).toBe(true)
  })
})
