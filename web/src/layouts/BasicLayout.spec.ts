// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BasicLayout from './BasicLayout.vue'
import {
  createChatSession,
  getChatSession,
  listChatModels,
  listChatSessions,
  streamChatMessage,
  type ChatSession,
  type SendChatMessageResult
} from '@/api/ai'
import { fetchCurrentUser } from '@/api/system'
import { useAuthStore } from '@/stores/auth'
import { useSharedChat, type ChatController } from '@/views/ai-assistant/composables/useChat'

const captured = vi.hoisted(() => ({ overlay: null as ChatController | null }))

vi.mock('./components/AIAssistantOverlay.vue', async () => {
  const { defineComponent, h } = await import('vue')
  const { useSharedChat } = await import('@/views/ai-assistant/composables/useChat')
  return {
    default: defineComponent({
      name: 'AIAssistantOverlay',
      setup() {
        captured.overlay = useSharedChat()
        return () => h('div')
      }
    })
  }
})
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
vi.mock('@/api/system', () => ({ fetchCurrentUser: vi.fn() }))
vi.mock('vue-router', () => ({
  useRoute: vi.fn(() => ({ path: '/ai-assistant', meta: { title: 'AI 运维助手' } })),
  useRouter: vi.fn(() => ({ push: vi.fn(), replace: vi.fn() }))
}))
vi.mock('@arco-design/web-vue/es/message', () => ({
  default: {
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    error: vi.fn()
  }
}))
vi.mock('@arco-design/web-vue/es/icon', () => {
  const icon = { template: '<span />' }
  return {
    IconBarChart: icon,
    IconBook: icon,
    IconBranch: icon,
    IconBulb: icon,
    IconCloud: icon,
    IconCommand: icon,
    IconDashboard: icon,
    IconFile: icon,
    IconNotification: icon,
    IconRobot: icon,
    IconStorage: icon,
    IconTool: icon
  }
})
vi.mock('@arco-design/web-vue', () => {
  const passthrough = { template: '<div><slot /></div>' }
  return {
    Layout: passthrough,
    LayoutSider: passthrough,
    LayoutHeader: passthrough,
    LayoutContent: passthrough,
    Menu: passthrough,
    MenuItem: passthrough,
    Tooltip: passthrough,
    Button: { template: '<button><slot name="icon" /><slot /></button>' }
  }
})

function makeSession(): ChatSession {
  return {
    session_id: 'session-1',
    title: '当前会话',
    status: 'active',
    context: {},
    created_at: '2026-11-23T10:00:00Z',
    updated_at: '2026-11-23T10:00:00Z'
  }
}

function makeResult(): SendChatMessageResult {
  return {
    session_id: 'session-1',
    message_id: 'assistant-1',
    status: 'ok',
    answer: {
      summary: '回答完成',
      evidence: [],
      impact: [],
      recommendations: [],
      risk_level: 'info',
      require_confirm: false,
      actions: [],
      rollback: '',
      uncertainties: [],
      insufficient_context: false
    },
    tool_calls: [],
    references: [],
    context: {},
    created_at: '2026-11-23T10:01:00Z'
  }
}

function mountLayout() {
  const controllers: ChatController[] = []
  let switchPage = () => {}
  const makeConsumer = (name: string) => defineComponent({
    name,
    setup() {
      const chat = useSharedChat()
      controllers.push(chat)
      return () => h('div', `${name}:${chat.sending.value}:${chat.messages.value.length}`)
    }
  })
  const Workbench = makeConsumer('Workbench')
  const AlertPage = makeConsumer('AlertPage')
  const RouterViewStub = defineComponent({
    setup() {
      const workbench = ref(true)
      switchPage = () => {
        workbench.value = !workbench.value
      }
      return () => h(workbench.value ? Workbench : AlertPage)
    }
  })
  const wrapper = mount(BasicLayout, {
    global: {
      stubs: { RouterView: RouterViewStub }
    }
  })
  return {
    wrapper,
    controllers,
    switchPage: () => switchPage()
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  captured.overlay = null
  vi.clearAllMocks()
  const auth = useAuthStore()
  auth.setToken('test-token')
  auth.setUser({ id: 'user-1', username: 'tester', status: 'active' })
  vi.mocked(fetchCurrentUser).mockResolvedValue({ id: 'user-1', username: 'tester', status: 'active' })
  vi.mocked(listChatModels).mockResolvedValue([])
  vi.mocked(listChatSessions).mockResolvedValue([])
  vi.mocked(createChatSession).mockResolvedValue(makeSession())
  vi.mocked(getChatSession).mockResolvedValue({ session: makeSession(), messages: [] })
})

describe('BasicLayout shared chat controller', () => {
  it('keeps one in-flight stream and shared progress across route page remounts', async () => {
    let signal: AbortSignal | undefined
    let resolveStream!: (result: SendChatMessageResult) => void
    vi.mocked(streamChatMessage).mockImplementation((_sessionId, _input, handlers) => {
      signal = handlers?.signal
      return new Promise((resolve) => {
        resolveStream = resolve
      })
    })
    const { controllers, switchPage } = mountLayout()
    await flushPromises()
    const chat = controllers[0]
    expect(captured.overlay).toBe(chat)

    const sendPromise = chat.send('分析当前告警')
    await vi.waitFor(() => expect(streamChatMessage).toHaveBeenCalledTimes(1))
    expect(chat.sending.value).toBe(true)

    switchPage()
    await nextTick()
    expect(controllers).toHaveLength(2)
    expect(controllers[1]).toBe(chat)
    expect(controllers[1].sending.value).toBe(true)
    expect(signal?.aborted).toBe(false)

    switchPage()
    await nextTick()
    expect(controllers).toHaveLength(3)
    expect(controllers[2]).toBe(chat)
    expect(signal?.aborted).toBe(false)

    resolveStream(makeResult())
    await sendPromise
    expect(chat.sending.value).toBe(false)
    expect(chat.messages.value.map((message) => message.role)).toEqual(['user', 'assistant'])
  })

  it('aborts the in-flight stream only when the whole layout unmounts', async () => {
    let signal: AbortSignal | undefined
    vi.mocked(streamChatMessage).mockImplementation((_sessionId, _input, handlers) => {
      signal = handlers?.signal
      return new Promise((_resolve, reject) => {
        handlers?.signal?.addEventListener('abort', () => {
          const error = new Error('aborted')
          error.name = 'AbortError'
          reject(error)
        })
      })
    })
    const { wrapper, controllers } = mountLayout()
    await flushPromises()

    const sendPromise = controllers[0].send('分析当前告警')
    await vi.waitFor(() => expect(streamChatMessage).toHaveBeenCalledTimes(1))
    expect(signal?.aborted).toBe(false)

    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    await sendPromise
  })

  it('queues messages in order and supports priority and removal across pages', async () => {
    const resolvers: Array<(result: SendChatMessageResult) => void> = []
    const requestedContent: string[] = []
    vi.mocked(streamChatMessage).mockImplementation((_sessionId, input) => {
      requestedContent.push(input.content)
      return new Promise((resolve) => {
        resolvers.push(resolve)
      })
    })
    const { controllers, switchPage } = mountLayout()
    await flushPromises()
    const chat = controllers[0]

    const firstSend = chat.send('第一条')
    await vi.waitFor(() => expect(resolvers).toHaveLength(1))
    await chat.send('第二条')
    await chat.send('第三条')
    await chat.send('第四条')
    expect(chat.queuedMessages.value.map((item) => item.content)).toEqual(['第二条', '第三条', '第四条'])

    const queued = chat.queuedMessages.value
    chat.prioritizeQueuedMessage(queued[1].queue_id)
    chat.removeQueuedMessage(queued[2].queue_id)
    expect(chat.queuedMessages.value.map((item) => item.content)).toEqual(['第三条', '第二条'])

    switchPage()
    await nextTick()
    expect(controllers[1]).toBe(chat)
    expect(controllers[1].queuedMessages.value.map((item) => item.content)).toEqual(['第三条', '第二条'])

    resolvers[0](makeResult())
    await firstSend
    await vi.waitFor(() => expect(resolvers).toHaveLength(2))
    expect(requestedContent).toEqual(['第一条', '第三条'])

    resolvers[1](makeResult())
    await vi.waitFor(() => expect(resolvers).toHaveLength(3))
    resolvers[2](makeResult())
    await vi.waitFor(() => expect(chat.sending.value).toBe(false))
    expect(requestedContent).toEqual(['第一条', '第三条', '第二条'])
    expect(chat.queuedMessages.value).toEqual([])
  })

  it('stops the current stream and clears all queued messages', async () => {
    let signal: AbortSignal | undefined
    vi.mocked(streamChatMessage).mockImplementation((_sessionId, _input, handlers) => {
      signal = handlers?.signal
      return new Promise((_resolve, reject) => {
        handlers?.signal?.addEventListener('abort', () => {
          const error = new Error('aborted')
          error.name = 'AbortError'
          reject(error)
        })
      })
    })
    const { controllers } = mountLayout()
    await flushPromises()
    const chat = controllers[0]

    const firstSend = chat.send('当前问题')
    await vi.waitFor(() => expect(streamChatMessage).toHaveBeenCalledTimes(1))
    await chat.send('排队问题')
    expect(chat.queuedMessages.value).toHaveLength(1)

    chat.stop()
    expect(signal?.aborted).toBe(true)
    expect(chat.queuedMessages.value).toEqual([])
    await firstSend
    expect(streamChatMessage).toHaveBeenCalledTimes(1)
  })
})
