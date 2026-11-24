// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createChatSession,
  getChatSession,
  listChatModels,
  listChatSessions,
  streamChatMessage,
  type ChatSession,
  type SendChatMessageResult
} from '@/api/ai'
import { useChat } from './useChat'

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

interface PendingStream {
  sessionId: string
  content: string
  resolve: (result: SendChatMessageResult) => void
}

let controller!: ReturnType<typeof useChat>
let serverSessions: ChatSession[] = []
let pendingStreams: PendingStream[] = []
let createdCount = 0

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

function makeResult(sessionId: string, summary: string): SendChatMessageResult {
  return {
    session_id: sessionId,
    message_id: `msg-${summary}`,
    status: 'ok',
    answer: {
      summary,
      evidence: [],
      impact: [],
      recommendations: [],
      risk_level: 'info',
      require_confirm: false,
      actions: [],
      rollback: '无变更动作',
      uncertainties: [],
      insufficient_context: false
    },
    tool_calls: [],
    references: [],
    context: {},
    created_at: '2026-11-23T10:00:00Z'
  }
}

function mountHost() {
  const Host = defineComponent({
    setup() {
      controller = useChat()
      return () => h('div')
    }
  })
  return mount(Host)
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  createdCount = 0
  pendingStreams = []
  serverSessions = [makeSession('session-1', '会话一'), makeSession('session-2', '会话二')]
  vi.mocked(listChatModels).mockResolvedValue([])
  vi.mocked(listChatSessions).mockImplementation(async () => [...serverSessions])
  vi.mocked(getChatSession).mockImplementation(async (sessionId) => ({
    session: serverSessions.find((session) => session.session_id === sessionId) ?? makeSession(sessionId),
    messages: []
  }))
  vi.mocked(createChatSession).mockImplementation(async () => {
    const session = makeSession(`new-session-${++createdCount}`, '新会话')
    serverSessions = [session, ...serverSessions]
    return session
  })
  vi.mocked(streamChatMessage).mockImplementation((sessionId, input) => {
    return new Promise<SendChatMessageResult>((resolve) => {
      pendingStreams.push({ sessionId, content: input.content, resolve })
    })
  })
})

describe('useChat per-session concurrency', () => {
  it('runs different sessions concurrently and keeps same-session messages queued', async () => {
    const wrapper = mountHost()
    await flushPromises()
    await controller.loadSessions()
    await flushPromises()
    expect(controller.activeSessionId.value).toBe('session-1')

    const firstSend = controller.send('first question')
    await flushPromises()
    expect(pendingStreams).toHaveLength(1)
    expect(pendingStreams[0].sessionId).toBe('session-1')
    expect(controller.sending.value).toBe(true)

    // 生成中仍可新建并切换到另一个会话。
    const created = await controller.startNewSession()
    expect(created?.session_id).toBe('new-session-1')
    expect(controller.activeSessionId.value).toBe('new-session-1')

    const secondSend = controller.send('second question')
    await flushPromises()
    expect(pendingStreams).toHaveLength(2)
    expect(pendingStreams[1].sessionId).toBe('new-session-1')
    expect(controller.sendingSessionIds.value).toEqual(
      expect.arrayContaining(['session-1', 'new-session-1'])
    )

    // 同一会话继续提问进入队列，不并发。
    await controller.send('third question')
    expect(controller.queuedMessages.value).toHaveLength(1)
    expect(controller.queuedMessages.value[0].content).toBe('third question')

    // 后台会话完成，不应影响当前会话的生成状态。
    pendingStreams[0].resolve(makeResult('session-1', 'first answer'))
    await firstSend
    await flushPromises()
    expect(controller.activeSessionId.value).toBe('new-session-1')
    expect(controller.sending.value).toBe(true)

    // 切回后台会话可直接看到已完成回答。
    await controller.selectSession('session-1')
    expect(controller.messages.value.some((message) => message.answer?.summary === 'first answer')).toBe(true)
    expect(controller.sending.value).toBe(false)

    // 切回仍在生成的会话：完成后自动发送队列中的第三条。
    await controller.selectSession('new-session-1')
    expect(controller.sending.value).toBe(true)
    pendingStreams[1].resolve(makeResult('new-session-1', 'second answer'))
    await secondSend
    await flushPromises()
    expect(pendingStreams).toHaveLength(3)
    expect(pendingStreams[2].content).toBe('third question')
    pendingStreams[2].resolve(makeResult('new-session-1', 'third answer'))
    await flushPromises()
    expect(controller.queuedMessages.value).toHaveLength(0)
    expect(controller.messages.value.some((message) => message.answer?.summary === 'third answer')).toBe(true)

    wrapper.unmount()
  })
})