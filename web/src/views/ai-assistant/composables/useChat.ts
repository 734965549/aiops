import { computed, inject, onMounted, onUnmounted, ref, type InjectionKey } from 'vue'
import { useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import {
  archiveChatSession,
  createChatSession,
  deleteChatSession,
  getChatSession,
  listChatModels,
  listChatSessions,
  sendChatFeedback,
  streamChatMessage,
  unarchiveChatSession,
  type ChatAnswer,
  type ChatMessage,
  type ChatModelInfo,
  type ChatResultStatus,
  type ChatSession,
  type ChatSessionContext,
  type ChatStreamEvent,
  type ChatToolCall,
  type SendChatMessageResult
} from '@/api/ai'
import { createExecutionTask } from '@/api/execution'
import { getApiError, isApiHttpError } from '@/api/request'
import { isAbortError } from '@/api/sse'
import { useAIContextStore } from '@/stores/aiContext'
import { IMAGE_ONLY_PROMPT } from './imageInput'

/** 请求进行中的可见阶段（同步接口，仅作进度提示）。 */
export type ChatStage = 'idle' | 'thinking' | 'querying_tools' | 'generating'

/** 回答结束后的 UI 状态（页面原型 §23.2）。 */
export type ChatUIStatus =
  | 'idle'
  | 'ok'
  | 'needs_confirm'
  | 'not_executable'
  | 'insufficient_context'
  | 'incomplete'
  | 'inconclusive'
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
  insufficient_context: 'AI 上下文不足，需补充信息',
  incomplete: 'AI 回答生成中断',
  inconclusive: 'AI 未得出结论',
  failed: 'AI 回答失败'
}

/** 模型展示文案：provider 显示名与模型 ID 相同时去重，避免 "X（X）"。 */
export function formatModelLabel(model: ChatModelInfo | null | undefined): string {
  if (!model) return ''
  const name = (model.name || '').trim()
  const modelID = (model.model || '').trim()
  const fallback = (model.provider_id || '').trim()
  if (!modelID || modelID === name) return name || modelID || fallback
  return name ? `${name}（${modelID}）` : modelID || fallback
}

/**
 * AI 助手对话逻辑：独立页与全局浮层共用。
 * 所有状态只在本组合式函数内维护，不写 localStorage（避免凭据/上下文泄漏）。
 */
export function useChat() {
  const router = useRouter()
  const contextStore = useAIContextStore()

  const sessions = ref<ChatSession[]>([])
  // 可选模型列表与当前选择；切换后从下一条消息生效。
  const models = ref<ChatModelInfo[]>([])
  const selectedProviderID = ref('')
  const activeSessionId = ref('')
  const activeSession = ref<ChatSession | null>(null)
  const messages = ref<ChatMessage[]>([])
  const loadingSessions = ref(false)
  const loadingMessages = ref(false)
  const sending = ref(false)
  const proposing = ref(false)
  const error = ref('')
  const stage = ref<ChatStage>('idle')
  // delta 事件只做“已生成 N 字”进度提示，不渲染半成品 JSON。
  const deltaChars = ref(0)
  // 发送中的实时过程：工具调用逐条到达（SSE tool 事件）与模型思考增量（reasoning_content）。
  // 思考内容只在流中展示，不落库；发送开始时清空，结束后由最终回答的工具调用展示接管。
  const liveTools = ref<ChatToolCall[]>([])
  const liveThinking = ref('')
  const uiStatus = ref<ChatUIStatus>('idle')
  const lastUserContent = ref('')
  // 最近一次提问的附图（data URL），重试时与幂等键一起复用。
  const lastUserImages = ref<string[]>([])
  // 最近一次提问的幂等键，重试时复用（ops/ai-contract.md §10.6.5）。
  const lastClientMessageId = ref('')
  const lastResult = ref<SendChatMessageResult | null>(null)

  // 流式请求的取消句柄；用户点「停止」时中断，服务端按 §10.17.4 不落 assistant 消息。
  let streamController: AbortController | null = null

  const contextSummary = computed(() => contextStore.summary)
  const requestContext = computed<ChatSessionContext>(() => contextStore.toRequestContext())
  const sessionList = computed(() => sessions.value)
  const hasSession = computed(() => !!activeSessionId.value)
  const modelOptions = computed(() => models.value)
  const selectedModel = computed(
    () => models.value.find((model) => model.provider_id === selectedProviderID.value) ?? null
  )
  /** 拉取可选模型；失败时隐藏选择器（单模型用户无感知）。 */
  async function loadModels() {
    try {
      const list = await listChatModels()
      const hadModels = models.value.length > 0
      models.value = list
      if (!selectedProviderID.value && list.length > 0) {
        selectedProviderID.value = list[0].provider_id
      } else if (hadModels && selectedProviderID.value && !modelOptions.value.some(
        (model) => model.provider_id === selectedProviderID.value
      )) {
        // 选中的模型已被删除/禁用：回退到当前第一项，与服务端回退语义一致。
        selectedProviderID.value = list[0]?.provider_id || ''
      }
    } catch {
      models.value = []
    }
  }

  function selectModel(providerID: string) {
    if (sending.value) return
    selectedProviderID.value = providerID
    if (activeSession.value) {
      activeSession.value = {
        ...activeSession.value,
        context: { ...activeSession.value.context, provider_id: providerID || undefined }
      }
    }
  }

  /** 会话上下文 + 当前模型选择；发送消息/新建会话共用。 */
  function chatRequestContext(): ChatSessionContext {
    return {
      ...requestContext.value,
      provider_id: selectedProviderID.value || undefined
    }
  }

  onMounted(() => {
    void loadModels()
  })

  function beginStage() {
    stage.value = 'thinking'
    liveTools.value = []
    liveThinking.value = ''
  }

  function endStage() {
    stage.value = 'idle'
    deltaChars.value = 0
    streamController = null
  }

  onUnmounted(() => streamController?.abort())

  /** 把服务端 stage 事件映射到既有 UI 阶段。 */
  function mapServerStage(serverStage: string): ChatStage {
    if (serverStage === 'answering') return 'generating'
    // gathering（取数/权限校验）与 thinking（模型调用）都归入「思考中」。
    return 'thinking'
  }

  function handleStreamEvent(event: ChatStreamEvent) {
    if (event.type === 'stage') {
      stage.value = mapServerStage(event.stage)
    } else if (event.type === 'tool') {
      // 工具观测到达即说明正在取数，先于模型阶段给出提示；新一轮模型输出重新计数。
      stage.value = 'querying_tools'
      deltaChars.value = 0
      if (event.tool) liveTools.value = [...liveTools.value, event.tool]
    } else if (event.type === 'delta') {
      if (event.phase === 'thinking' && event.text) {
        // 思考增量实时展示在过程面板中（仅流中可见，不落库）。
        liveThinking.value += event.text
      } else if (event.phase === 'answer' && event.text) {
        deltaChars.value += Array.from(event.text).length
      }
    }
    // 未知事件类型（例如未来新增的通知）直接忽略，避免破坏现有流。
  }

  function deriveUIStatus(status: ChatResultStatus, answer?: ChatAnswer): ChatUIStatus {
    if (status === 'incomplete' || answer?.incomplete) return 'incomplete'
    if (status === 'degraded') {
      // 取数预算耗尽属于「已保留证据但未收敛」，按橙色未结论呈现，不当作模型失败。
      const reason = (answer?.degraded_reason || '').trim()
      if (reason === 'agent_loop_exhausted' || reason === 'turn_budget_exceeded') return 'inconclusive'
      return 'failed'
    }
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
    // 优先展示会话上下文保存的 provider，列表未加载或模型已删除时回退默认。
    const sessionProvider = detail.session.context?.provider_id || ''
    if (sessionProvider && (models.value.length === 0 || models.value.some(
      (model) => model.provider_id === sessionProvider
    ))) {
      selectedProviderID.value = sessionProvider
    } else if (models.value.length > 0) {
      selectedProviderID.value = models.value[0].provider_id
    }
    const lastAssistant = [...detail.messages].reverse().find((message) => message.role === 'assistant')
    if (lastAssistant?.answer) {
      uiStatus.value = deriveUIStatus(
        lastAssistant.answer.incomplete
          ? 'incomplete'
          : lastAssistant.answer.degraded
            ? 'degraded'
            : lastAssistant.answer.insufficient_context
              ? 'insufficient_context'
              : 'ok',
        lastAssistant.answer
      )
    } else {
      uiStatus.value = 'idle'
    }
  }

  async function loadSessions() {
    await loadSessionList(true)
  }

  async function loadSessionChoices() {
    await loadSessionList(false)
  }

  async function loadSessionList(autoSelect: boolean) {
    loadingSessions.value = true
    try {
      sessions.value = await listChatSessions(20)
      if (autoSelect && !activeSessionId.value && sessions.value.length > 0) {
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
      context: chatRequestContext()
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
      const session = await createSession()
      Message.success('已创建新会话')
      return session
    } catch {
      /* 错误已由拦截器提示。 */
      return null
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

  async function send(rawContent: string, clientMessageId?: string, images: string[] = []) {
    const content = rawContent.trim()
    if ((!content && images.length === 0) || sending.value) return
    // 纯图片消息与服务端兜底提问保持一致（ops/ai-contract.md §10.20）。
    const effectiveContent = content || IMAGE_ONLY_PROMPT
    // 归档会话只读：服务端会返回 409，前端提前拦截避免丢失已输入内容。
    if (activeSession.value?.status === 'archived') {
      Message.warning('会话已归档，请先恢复归档或新建会话再继续提问')
      return
    }
    const sessionId = await ensureSession()
    if (!sessionId) return

    error.value = ''
    sending.value = true
    deltaChars.value = 0
    lastUserContent.value = effectiveContent
    lastUserImages.value = [...images]
    // 同一条提问的重试复用同一个幂等键，服务端据此返回首次结果而不是重复落消息。
    const clientId = clientMessageId || newClientMessageId()
    lastClientMessageId.value = clientId
    beginStage()
    const optimisticUserMessage: ChatMessage = {
      message_id: `local-${Date.now()}`,
      session_id: sessionId,
      role: 'user',
      content: effectiveContent,
      images: images.length > 0 ? [...images] : undefined,
      tool_calls: [],
      references: [],
      created_at: new Date().toISOString()
    }
    messages.value = [...messages.value, optimisticUserMessage]

    const controller = new AbortController()
    streamController = controller
    try {
      const result = await streamChatMessage(
        sessionId,
        {
          content: effectiveContent,
          images: images.length > 0 ? images : undefined,
          context: chatRequestContext(),
          provider_id: selectedProviderID.value || undefined,
          client_message_id: clientId
        },
        {
          signal: controller.signal,
          onEvent: handleStreamEvent
        }
      )
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
    } catch (err) {
      if (isAbortError(err)) {
        // 用户主动停止：服务端不落回答，回读会话以去掉本地乐观消息。
        error.value = ''
        uiStatus.value = 'idle'
        try {
          await selectSession(sessionId)
        } catch {
          /* 回读失败时保留本地状态。 */
        }
        Message.info('已停止生成')
        return
      }
      const apiError = getApiError(err) ?? (isApiHttpError(err) ? err : null)
      // 后端可能已落库用户消息；失败后回读服务端会话，避免本地与服务端漂移。
      try {
        await selectSession(sessionId)
      } catch {
        /* 回读失败时保留本地错误提示。 */
      }
      if (apiError?.code === 'FAILED_PRECONDITION') {
        // 409 有两种业务语义：会话被归档，或幂等键对应的上一次请求仍在处理/已中断。
        const detail = apiError.message || ''
        error.value = detail.includes('archived')
          ? '会话已归档，请先恢复归档或新建会话再继续提问'
          : '上一次相同提问仍在处理或已中断；请刷新会话查看结果后再决定是否重新提问'
      } else if (apiError?.status === 403) {
        // 流式接口不经过 axios 拦截器，这里补齐权限提示。
        error.value = '当前账号缺少 AI 对话权限，请联系管理员'
      } else if (apiError?.status === 503) {
        error.value = 'AI 对话服务暂不可用，请检查后端配置与模型 Provider'
      } else {
        error.value = 'AI 回答失败，请稍后重试'
      }
      uiStatus.value = 'failed'
    } finally {
      sending.value = false
      endStage()
    }
  }

  /** 用户主动停止生成：中断流读取，服务端按取消语义不写 assistant 消息。 */
  function stop() {
    streamController?.abort()
    streamController = null
  }

  async function retry() {
    if (!lastUserContent.value && lastUserImages.value.length === 0) return
    // 重试复用同一幂等键与附图：服务端已生成答案时直接回放，不会产生重复消息。
    await send(lastUserContent.value, lastClientMessageId.value || undefined, lastUserImages.value)
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

  /** 归档会话：归档后只读，历史仍可查看。 */
  async function archiveSession(sessionId: string) {
    if (!sessionId) return
    try {
      const updated = await archiveChatSession(sessionId)
      applyStatusChange(updated)
      Message.success('会话已归档')
    } catch {
      /* 错误已由拦截器提示。 */
    }
  }

  /** 恢复归档会话，可继续提问。 */
  async function unarchiveSession(sessionId: string) {
    if (!sessionId) return
    try {
      const updated = await unarchiveChatSession(sessionId)
      applyStatusChange(updated)
      Message.success('会话已恢复')
    } catch {
      /* 错误已由拦截器提示。 */
    }
  }

  /** 删除会话及其消息；删除当前会话后自动切到下一个会话。 */
  async function deleteSession(sessionId: string) {
    if (!sessionId) return
    try {
      await deleteChatSession(sessionId)
      sessions.value = sessions.value.filter((session) => session.session_id !== sessionId)
      if (activeSessionId.value === sessionId) {
        resetActiveSession()
        const next = sessions.value[0]
        if (next) await selectSession(next.session_id)
      }
      Message.success('会话已删除')
    } catch {
      /* 错误已由拦截器提示。 */
    }
  }

  function applyStatusChange(updated: ChatSession) {
    sessions.value = sessions.value.map((session) =>
      session.session_id === updated.session_id ? { ...session, ...updated } : session
    )
    if (activeSession.value?.session_id === updated.session_id) {
      activeSession.value = { ...activeSession.value, ...updated }
    }
  }

  function resetActiveSession() {
    activeSession.value = null
    activeSessionId.value = ''
    messages.value = []
    uiStatus.value = 'idle'
    deltaChars.value = 0
    lastResult.value = null
    error.value = ''
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
    deltaChars,
    liveTools,
    liveThinking,
    uiStatus,
    lastResult,
    lastUserContent,
    contextSummary,
    requestContext,
    hasSession,
    modelOptions,
    selectedModel,
    selectModel,
    loadSessions,
    loadSessionChoices,
    selectSession,
    startNewSession,
    ensureSession,
    send,
    retry,
    stop,
    refreshSessionList,
    submitFeedback,
    archiveSession,
    unarchiveSession,
    deleteSession,
    proposeExecution,
    executionAction
  }
}

export type ChatController = ReturnType<typeof useChat>
export const ChatControllerKey: InjectionKey<ChatController> = Symbol('ChatController')

export function useSharedChat(): ChatController {
  const controller = inject(ChatControllerKey)
  if (!controller) throw new Error('AI chat controller is not provided')
  return controller
}

function truncate(value: string, limit: number): string {
  const runes = Array.from(value || '')
  return runes.length <= limit ? value : runes.slice(0, limit).join('')
}

/** 生成幂等键；优先 crypto.randomUUID，非安全上下文回落时间戳+随机串。 */
function newClientMessageId(): string {
  const cryptoApi = globalThis.crypto as Crypto | undefined
  if (cryptoApi && typeof cryptoApi.randomUUID === 'function') return cryptoApi.randomUUID()
  return `cm-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`
}
