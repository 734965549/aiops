import { computed, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import {
  createChatSession,
  getChatSession,
  listChatSessions,
  sendChatFeedback,
  sendChatMessage,
  type ChatAnswer,
  type ChatMessage,
  type ChatResultStatus,
  type ChatSession,
  type ChatSessionContext,
  type SendChatMessageResult
} from '@/api/ai'
import { createExecutionTask } from '@/api/execution'
import { useAIContextStore } from '@/stores/aiContext'

/** 请求进行中的可见阶段（同步接口，仅作进度提示）。 */
export type ChatStage = 'idle' | 'thinking' | 'querying_tools' | 'generating'

/** 回答结束后的 UI 状态（页面原型 §23.2）。 */
export type ChatUIStatus =
  | 'idle'
  | 'ok'
  | 'needs_confirm'
  | 'not_executable'
  | 'insufficient_context'
  | 'failed'

export const STAGE_LABELS: Record<Exclude<ChatStage, 'idle'>, string> = {
  thinking: 'AI 思考中…',
  querying_tools: 'AI 查询工具中（告警 / 指标）…',
  generating: 'AI 生成回答中…'
}

export const STATUS_LABELS: Record<ChatUIStatus, string> = {
  idle: '',
  ok: '回答完成',
  needs_confirm: 'AI 建议需人工确认',
  not_executable: 'AI 建议不可执行',
  insufficient_context: 'AI 无足够上下文',
  failed: 'AI 回答失败'
}

/**
 * AI 助手对话逻辑：独立页与全局浮层共用。
 * 所有状态只在本组合式函数内维护，不写 localStorage（避免凭据/上下文泄漏）。
 */
export function useChat() {
  const router = useRouter()
  const contextStore = useAIContextStore()

  const sessions = ref<ChatSession[]>([])
  const activeSessionId = ref('')
  const activeSession = ref<ChatSession | null>(null)
  const messages = ref<ChatMessage[]>([])
  const loadingSessions = ref(false)
  const loadingMessages = ref(false)
  const sending = ref(false)
  const proposing = ref(false)
  const error = ref('')
  const stage = ref<ChatStage>('idle')
  const uiStatus = ref<ChatUIStatus>('idle')
  const lastUserContent = ref('')
  const lastResult = ref<SendChatMessageResult | null>(null)

  let stageTimers: number[] = []

  const contextSummary = computed(() => contextStore.summary)
  const requestContext = computed<ChatSessionContext>(() => contextStore.toRequestContext())
  const sessionList = computed(() => sessions.value)
  const hasSession = computed(() => !!activeSessionId.value)

  function clearStageTimers() {
    stageTimers.forEach((timer) => window.clearTimeout(timer))
    stageTimers = []
  }

  function beginStage() {
    clearStageTimers()
    stage.value = 'thinking'
    // 同步接口无法回传真实阶段，用固定节奏提示“工具查询/生成回答”，让等待过程可感知。
    stageTimers = [
      window.setTimeout(() => {
        if (sending.value) stage.value = 'querying_tools'
      }, 1200),
      window.setTimeout(() => {
        if (sending.value) stage.value = 'generating'
      }, 3200)
    ]
  }

  function endStage() {
    clearStageTimers()
    stage.value = 'idle'
  }

  onUnmounted(clearStageTimers)

  function deriveUIStatus(status: ChatResultStatus, answer?: ChatAnswer): ChatUIStatus {
    if (status === 'degraded') return 'failed'
    if (status === 'insufficient_context' || answer?.insufficient_context) return 'insufficient_context'
    const execution = answer?.actions?.find((action) => action.code === 'execution.propose')
    if (execution && !execution.allowed) return 'not_executable'
    if (answer?.require_confirm) return 'needs_confirm'
    return 'ok'
  }

  function applySessionDetail(detail: { session: ChatSession; messages: ChatMessage[] }) {
    activeSession.value = detail.session
    activeSessionId.value = detail.session.session_id
    messages.value = detail.messages
    const lastAssistant = [...detail.messages].reverse().find((message) => message.role === 'assistant')
    if (lastAssistant?.answer) {
      uiStatus.value = deriveUIStatus(
        lastAssistant.answer.degraded ? 'degraded' : lastAssistant.answer.insufficient_context ? 'insufficient_context' : 'ok',
        lastAssistant.answer
      )
    } else {
      uiStatus.value = 'idle'
    }
  }

  async function loadSessions() {
    loadingSessions.value = true
    try {
      sessions.value = await listChatSessions(20)
      if (!activeSessionId.value && sessions.value.length > 0) {
        await selectSession(sessions.value[0].session_id)
      }
    } catch {
      /* 错误已由拦截器提示。 */
    } finally {
      loadingSessions.value = false
    }
  }

  async function selectSession(sessionId: string) {
    if (!sessionId) return
    loadingMessages.value = true
    error.value = ''
    try {
      const detail = await getChatSession(sessionId)
      applySessionDetail(detail)
    } catch (err) {
      error.value = '加载会话失败，请稍后重试'
      throw err
    } finally {
      loadingMessages.value = false
    }
  }

  async function createSession(title?: string) {
    const session = await createChatSession({
      title: title?.trim() || undefined,
      context: requestContext.value
    })
    sessions.value = [session, ...sessions.value]
    activeSession.value = session
    activeSessionId.value = session.session_id
    messages.value = []
    uiStatus.value = 'idle'
    lastResult.value = null
    return session
  }

  async function startNewSession() {
    try {
      await createSession()
      Message.success('已创建新会话')
    } catch {
      /* 错误已由拦截器提示。 */
    }
  }

  async function ensureSession(): Promise<string | null> {
    if (activeSessionId.value) return activeSessionId.value
    try {
      const session = await createSession()
      return session.session_id
    } catch {
      return null
    }
  }

  async function send(rawContent: string) {
    const content = rawContent.trim()
    if (!content || sending.value) return
    const sessionId = await ensureSession()
    if (!sessionId) return

    error.value = ''
    sending.value = true
    lastUserContent.value = content
    beginStage()
    const optimisticUserMessage: ChatMessage = {
      message_id: `local-${Date.now()}`,
      session_id: sessionId,
      role: 'user',
      content,
      tool_calls: [],
      references: [],
      created_at: new Date().toISOString()
    }
    messages.value = [...messages.value, optimisticUserMessage]

    try {
      const result = await sendChatMessage(sessionId, {
        content,
        context: requestContext.value
      })
      lastResult.value = result
      const assistantMessage: ChatMessage = {
        message_id: result.message_id,
        session_id: result.session_id,
        role: 'assistant',
        content: JSON.stringify(result.answer),
        answer: result.answer,
        tool_calls: result.tool_calls,
        references: result.references,
        created_at: result.created_at
      }
      messages.value = [...messages.value, assistantMessage]
      uiStatus.value = deriveUIStatus(result.status, result.answer)
      if (activeSession.value) {
        activeSession.value = { ...activeSession.value, context: result.context }
      }
      await refreshSessionList()
    } catch {
      // 后端可能已落库用户消息；失败后回读服务端会话，避免本地与服务端漂移。
      try {
        await selectSession(sessionId)
      } catch {
        /* 回读失败时保留本地错误提示。 */
      }
      error.value = 'AI 回答失败，请稍后重试'
      uiStatus.value = 'failed'
    } finally {
      sending.value = false
      endStage()
    }
  }

  async function retry() {
    if (!lastUserContent.value) return
    await send(lastUserContent.value)
  }

  async function refreshSessionList() {
    try {
      const list = await listChatSessions(20)
      sessions.value = list
      const current = list.find((session) => session.session_id === activeSessionId.value)
      if (current) activeSession.value = current
    } catch {
      /* 列表刷新失败不影响本轮回答展示。 */
    }
  }

  async function submitFeedback(messageId: string, feedback: 'valid' | 'invalid') {
    if (!activeSessionId.value || !messageId) return
    const previous = messages.value
    messages.value = messages.value.map((message) =>
      message.message_id === messageId ? { ...message, feedback } : message
    )
    try {
      await sendChatFeedback(activeSessionId.value, messageId, feedback)
      Message.success(feedback === 'valid' ? '已标记为有效' : '已标记为无效')
    } catch {
      messages.value = previous
    }
  }

  function executionAction(answer?: ChatAnswer) {
    return answer?.actions?.find((action) => action.code === 'execution.propose')
  }

  /**
   * 生成待确认执行任务：只创建 Execution Task，真实执行由确认页与状态机约束。
   * 返回 task_id，调用方负责跳转。
   */
  async function proposeExecution(answer?: ChatAnswer, messageId?: string): Promise<string | null> {
    if (!answer || proposing.value) return null
    const action = executionAction(answer)
    if (!action) {
      Message.warning('当前回答没有可用的执行计划入口')
      return null
    }
    if (!action.allowed) {
      Message.warning(action.reason || '无权限创建执行计划，请联系管理员')
      return null
    }
    const context = requestContext.value
    const riskLevel = answer.risk_level === 'high' || answer.risk_level === 'medium' ? answer.risk_level : undefined
    proposing.value = true
    try {
      const created = await createExecutionTask({
        name: `AI 建议：${truncate(answer.summary, 40) || '处置任务'}`,
        source_type: 'ai_conversation',
        source_id: activeSessionId.value || undefined,
        operation_type: 'custom',
        target_type: context.resource_type || (context.alert_id ? 'alert' : ''),
        target_id: context.resource_id || context.alert_id || '',
        target_name: context.page_title || '',
        parameters: {
          session_id: activeSessionId.value,
          message_id: messageId || lastResult.value?.message_id,
          summary: answer.summary,
          recommendations: answer.recommendations,
          risk_level: answer.risk_level,
          rollback: answer.rollback
        },
        rollback_plan: answer.rollback ? { description: answer.rollback } : {},
        risk_level: riskLevel
      })
      Message.success(`已创建待确认任务（${created.risk_level}），请核对风险后确认执行`)
      // 浮层场景下先收起抽屉，避免遮挡执行确认页。
      contextStore.closeOverlay()
      router.push({ path: '/executions', query: { task_id: created.task_id } })
      return created.task_id
    } catch {
      return null
    } finally {
      proposing.value = false
    }
  }

  return {
    sessions: sessionList,
    activeSession,
    activeSessionId,
    messages,
    loadingSessions,
    loadingMessages,
    sending,
    proposing,
    error,
    stage,
    uiStatus,
    lastResult,
    lastUserContent,
    contextSummary,
    requestContext,
    hasSession,
    loadSessions,
    selectSession,
    startNewSession,
    ensureSession,
    send,
    retry,
    refreshSessionList,
    submitFeedback,
    proposeExecution,
    executionAction
  }
}

function truncate(value: string, limit: number): string {
  const runes = Array.from(value || '')
  return runes.length <= limit ? value : runes.slice(0, limit).join('')
}
