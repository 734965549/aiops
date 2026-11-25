import { http, ApiHttpError, type ApiResponse } from './request'
import { consumeSSE, type SSEFrame } from './sse'
import { useAuthStore } from '@/stores/auth'

export interface AIProvider {
  id: string
  name: string
  type: 'a' | 'b' | 'c' | string
  base_url: string
  model?: string
  api_key?: string
  has_api_key?: boolean
  timeout_ms?: number
  reasoning_effort?: string
  /** 类型 b 模型是否接受图片输入（多模态）；未开启时带图消息会被后端拒绝。 */
  vision_enabled?: boolean
  /** 模型上下文窗口（token）；0 或缺省表示未配置，使用全局 prompt 预算。 */
  context_window_tokens?: number
  /** 单次输出上限（max_tokens）；0 或缺省表示使用全局默认。 */
  max_output_tokens?: number
  headers?: Record<string, string>
  enabled: boolean
  description?: string
}

export interface UpsertProviderInput {
  id: string
  name: string
  type: string
  base_url: string
  model?: string
  api_key?: string
  timeout_ms?: number
  reasoning_effort?: string
  vision_enabled?: boolean
  context_window_tokens?: number
  max_output_tokens?: number
  headers?: Record<string, string>
  enabled?: boolean
  description?: string
}

export interface InvokeToolInput {
  provider_id: string
  tool_code: string
  resource?: string
  action?: string
  owner_id?: string
  dept?: string
  team?: string
  region?: string
  tags?: string[]
  confirmed?: boolean
  payload?: Record<string, unknown>
}

export interface InvokeToolResult {
  allowed: boolean
  reason?: string
  mode?: string
  provider?: string
  data?: Record<string, unknown>
}

export function listProviders() {
  return http<AIProvider[]>({ url: '/api/ai/providers', method: 'get' })
}

export function upsertProvider(input: UpsertProviderInput) {
  return http<{ updated: boolean }>({
    url: '/api/ai/providers',
    method: 'post',
    data: input
  })
}

export function deleteProvider(id: string) {
  return http<{ deleted: boolean }>({
    url: `/api/ai/providers/${encodeURIComponent(id)}`,
    method: 'delete'
  })
}

export function invokeTool(input: InvokeToolInput) {
  return http<InvokeToolResult>({
    url: '/api/ai/tools/invoke',
    method: 'post',
    data: input
  })
}

export interface AnalyzeAlertInput {
  alert_id: string
  time_range?: string
  include_logs?: boolean
  include_metrics?: boolean
  include_changes?: boolean
}

export interface AnalyzeAlertResult {
  conversation_id: string
  summary: string
  risk_level: string
  recommendations: string[]
  references: string[]
}

export function analyzeAlert(input: AnalyzeAlertInput) {
  return http<AnalyzeAlertResult>({
    url: '/api/ai/analyze-alert',
    method: 'post',
    data: input,
    // 模型为思考型，真实分析（含证据采集）可能超过全局 30s 超时。
    timeout: 90_000
  })
}

// ---- AI 运维助手会话与对话（ops/ai-contract.md §10）----

/** 页面上下文；只允许业务 ID，不允许凭据或自由文本。 */
export interface ChatSessionContext {
  page?: string
  page_title?: string
  alert_id?: string
  application_id?: string
  resource_id?: string
  resource_type?: string
  execution_id?: string
  time_range?: string
  /** 会话选定的对话模型（provider_id）；空表示使用默认模型。 */
  provider_id?: string
}

/** 可选对话模型摘要；不含 base_url/api_key/headers。 */
export interface ChatModelInfo {
  provider_id: string
  name: string
  model: string
  /** 该模型是否接受图片输入；false 时前端禁用图片入口。 */
  vision_enabled?: boolean
}

export interface ChatSession {
  session_id: string
  title: string
  status: 'active' | 'archived' | string
  context: ChatSessionContext
  created_at: string
  updated_at: string
}

export interface ChatAction {
  code: string
  label: string
  allowed: boolean
  reason?: string
  target?: string
}

export interface ChatDiagnosisFact {
  statement: string
  evidence_id?: string
  pointer?: string
}

export interface ChatRootCause {
  status: 'undetermined' | 'suspected' | 'confirmed' | string
  code: string
  statement: string
  evidence_ids: string[]
}

export interface ChatHypothesis {
  code: string
  statement: string
  missing_evidence: string[]
}

export interface ChatDiagnosis {
  facts: ChatDiagnosisFact[]
  root_cause: ChatRootCause
  hypotheses: ChatHypothesis[]
  missing_evidence: string[]
}

export interface ChatAnswer {
  summary: string
  evidence: string[]
  impact: string[]
  recommendations: string[]
  risk_level: 'low' | 'medium' | 'high' | 'info' | string
  require_confirm: boolean
  actions: ChatAction[]
  rollback: string
  uncertainties: string[]
  insufficient_context: boolean
  degraded?: boolean
  degraded_reason?: string
  /** 流式生成中断但保留了已获取上下文；前端按“不完整回答”提示，不按失败处理。 */
  incomplete?: boolean
  incomplete_reason?: string
  diagnosis?: ChatDiagnosis
}

export interface ChatToolCall {
  tool_code: string
  allowed: boolean
  mode?: string
  reason?: string
  status: string
  summary?: string
}

export interface ChatReference {
  type: string
  id: string
  label: string
  url?: string
}

export interface ChatMessage {
  message_id: string
  session_id: string
  role: 'user' | 'assistant' | string
  content: string
  /** 用户消息附图的 data URL 数组（migration 0059）。 */
  images?: string[]
  answer?: ChatAnswer
  tool_calls: ChatToolCall[]
  references: ChatReference[]
  feedback?: string
  created_at: string
}

export interface ChatSessionDetail {
  session: ChatSession
  messages: ChatMessage[]
}

/** 对话状态：ok / degraded / incomplete / insufficient_context（后端 status 原值）。 */
export type ChatResultStatus = 'ok' | 'degraded' | 'incomplete' | 'insufficient_context' | string

export interface SendChatMessageResult {
  session_id: string
  message_id: string
  status: ChatResultStatus
  answer: ChatAnswer
  tool_calls: ChatToolCall[]
  references: ChatReference[]
  context: ChatSessionContext
  created_at: string
}

export interface CreateChatSessionInput {
  title?: string
  context?: ChatSessionContext
}

export interface SendChatMessageInput {
  content: string
  /** 可选用户附图（data URL 数组，最多 3 张、单张解码后 <=2MB）；需模型开启 vision_enabled。 */
  images?: string[]
  context?: ChatSessionContext
  /** 可选对话模型；空时回退会话上下文与默认模型。 */
  provider_id?: string
  /** 幂等键：客户端超时重试时复用同一值，避免重复落消息与重复模型调用。 */
  client_message_id?: string
}

export function createChatSession(input: CreateChatSessionInput) {
  return http<ChatSession>({
    url: '/api/ai/chat/sessions',
    method: 'post',
    data: input
  })
}

/** GET /api/ai/chat/models：普通用户可见的启用中类型 b 模型。 */
export function listChatModels() {
  return http<ChatModelInfo[]>({
    url: '/api/ai/chat/models',
    method: 'get'
  })
}

export function listChatSessions(limit = 20) {
  return http<ChatSession[]>({
    url: '/api/ai/chat/sessions',
    method: 'get',
    params: { limit }
  })
}

export function getChatSession(sessionId: string) {
  return http<ChatSessionDetail>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}`,
    method: 'get'
  })
}

export function sendChatMessage(sessionId: string, input: SendChatMessageInput) {
  return http<SendChatMessageResult>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/messages`,
    method: 'post',
    data: input,
    // 对话可能触发工具取数 + 思考型模型，沿用分析链路的 90s 预算。
    timeout: 90_000
  })
}

export function sendChatFeedback(sessionId: string, messageId: string, feedback: 'valid' | 'invalid') {
  return http<ChatMessage>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}/feedback`,
    method: 'post',
    data: { feedback }
  })
}

// ---- 会话只读分享（ops/ai-contract.md §10.18）----

export interface ChatShare {
  share_id: string
  session_id: string
  status: 'active' | 'revoked' | string
  expires_at: string
  created_at: string
}

export interface CreatedChatShare extends ChatShare {
  /** 明文 token 只在创建响应返回一次，前端应立即展示/复制。 */
  token: string
  share_url: string
}

/** 创建只读分享链接；expiresInHours 缺省 168（7 天），允许 1–720。 */
export function createChatShare(sessionId: string, expiresInHours?: number) {
  return http<CreatedChatShare>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/shares`,
    method: 'post',
    data: expiresInHours ? { expires_in_hours: expiresInHours } : {}
  })
}

/** 会话的分享记录（含已撤销/过期），仅 owner 可查。 */
export function listChatShares(sessionId: string) {
  return http<{ shares: ChatShare[] }>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/shares`,
    method: 'get'
  })
}

/** 撤销分享链接；重复撤销幂等。 */
export function revokeChatShare(sessionId: string, shareId: string) {
  return http<ChatShare>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/shares/${encodeURIComponent(shareId)}`,
    method: 'delete'
  })
}

/** 通过分享 token 读取只读会话；不可用（过期/撤销/不存在）统一 404。 */
export function getSharedChatSession(token: string) {
  return http<ChatSessionDetail>({
    url: `/api/ai/chat/shares/${encodeURIComponent(token)}`,
    method: 'get'
  })
}

// ---- 流式对话（ops/ai-contract.md §10.17）----

/** 服务端推送的流式事件：阶段、工具观测、模型增量、最终回答、业务错误。 */
export type ChatStreamEvent =
  | { type: 'stage'; stage: string }
  | { type: 'tool'; tool: ChatToolCall }
  | { type: 'delta'; phase: 'thinking' | 'answer' | string; text: string }
  | { type: 'answer'; result: SendChatMessageResult }
  | { type: 'error'; code?: string; message?: string }

export interface StreamChatHandlers {
  /** 每收到一条事件回调一次；不回传答案本身，最终结果由 Promise 返回。 */
  onEvent?: (event: ChatStreamEvent) => void
  /** 主动停止：中断读取后服务端不会落 assistant 消息（契约 §10.17.4）。 */
  signal?: AbortSignal
}

/**
 * 流式发送消息：POST + fetch 读取 text/event-stream。
 * 收到 error 事件时抛 ApiHttpError（HTTP 状态已是 200，code 为业务错误码）；
 * 流结束但未收到 answer 时抛 ABORTED，调用方应回读会话。
 */
export async function streamChatMessage(
  sessionId: string,
  input: SendChatMessageInput,
  handlers: StreamChatHandlers = {}
): Promise<SendChatMessageResult> {
  const auth = useAuthStore()
  const url = `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/messages/stream`
  let response = await openChatStream(url, input, auth.token, handlers.signal)
  if (response.status === 401) {
    // 与 request.ts 的 401 语义对齐：token 过期先刷一次再重试。
    try {
      await auth.refresh()
    } catch {
      /* refresh 失败保持 401，交给调用方提示重新登录。 */
    }
    if (auth.token) {
      response = await openChatStream(url, input, auth.token, handlers.signal)
    }
  }
  if (!response.ok) {
    throw await toApiHttpError(response)
  }

  let result: SendChatMessageResult | null = null
  await consumeSSE(
    response,
    (frame) => {
      const event = decodeChatEvent(frame)
      if (!event) return
      handlers.onEvent?.(event)
      if (event.type === 'answer') {
        result = event.result
      } else if (event.type === 'error') {
        throw new ApiHttpError(200, event.code || 'UNKNOWN', event.message || 'AI 对话失败')
      }
    },
    handlers.signal
  )
  if (!result) {
    // 常见于客户端断开或服务端在推送 answer 前中断：不伪造回答，由调用方回读会话。
    throw new ApiHttpError(0, 'ABORTED', 'AI 对话已中断，未收到回答')
  }
  return result
}

async function openChatStream(
  url: string,
  input: SendChatMessageInput,
  token: string,
  signal?: AbortSignal
): Promise<Response> {
  return fetch(apiPath(url), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json; charset=utf-8',
      Accept: 'text/event-stream',
      ...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body: JSON.stringify(input),
    signal
  })
}

function decodeChatEvent(frame: SSEFrame): ChatStreamEvent | null {
  if (!frame.data) return null
  try {
    const parsed = JSON.parse(frame.data) as ChatStreamEvent
    if (parsed && typeof parsed.type === 'string') return parsed
  } catch {
    /* 单条坏事件不中断整个流。 */
  }
  return null
}

function apiPath(url: string): string {
  const base = import.meta.env.VITE_API_BASE || '/'
  if (!base || base === '/') return url
  return `${base.replace(/\/+$/, '')}/${url.replace(/^\/+/, '')}`
}

async function toApiHttpError(response: Response): Promise<ApiHttpError> {
  const body = (await response.json().catch(() => null)) as ApiResponse | null
  return new ApiHttpError(
    response.status,
    body?.code || 'UNKNOWN',
    body?.message || `${response.status} ${response.statusText}`,
    body?.trace_id
  )
}

/** 归档会话：归档后只读，仍可查看历史；重复归档幂等。 */
export function archiveChatSession(sessionId: string) {
  return http<ChatSession>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/archive`,
    method: 'post'
  })
}

/** 恢复归档会话，使其可以继续对话。 */
export function unarchiveChatSession(sessionId: string) {
  return http<ChatSession>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}/unarchive`,
    method: 'post'
  })
}

/** 删除会话及其全部消息（不可恢复）。 */
export function deleteChatSession(sessionId: string) {
  return http<{ deleted: boolean }>({
    url: `/api/ai/chat/sessions/${encodeURIComponent(sessionId)}`,
    method: 'delete'
  })
}

// ---- 知识沉淀（ops/ai-contract.md §10.15，鉴权 app:ai:knowledge:manage）----

export type KnowledgeScope = 'global' | 'application' | 'resource_type' | string
export type KnowledgeStatus = 'draft' | 'active' | 'disabled' | string

export interface KnowledgeItem {
  item_id: string
  scope: KnowledgeScope
  scope_ref: string
  title: string
  content: string
  origin: 'feedback_aggregate' | 'manual' | string
  status: KnowledgeStatus
  hit_count: number
  useful_count: number
  reviewed_by?: string
  reviewed_at?: number
  created_by?: string
  created_at: number
  updated_at: number
}

export interface KnowledgeItemInput {
  scope: KnowledgeScope
  scope_ref?: string
  title: string
  content: string
}

export interface KnowledgeAggregateResult {
  action: 'created' | 'updated' | 'skipped_active' | 'insufficient_cases' | string
  source_cases: number
  item?: KnowledgeItem
}

export function listKnowledgeItems(params: { status?: string; scope?: string; limit?: number; offset?: number } = {}) {
  return http<{ items: KnowledgeItem[] }>({
    url: '/api/ai/knowledge/items',
    method: 'get',
    params
  })
}

export function createKnowledgeItem(input: KnowledgeItemInput) {
  return http<KnowledgeItem>({
    url: '/api/ai/knowledge/items',
    method: 'post',
    data: input
  })
}

export function updateKnowledgeItem(itemId: string, input: KnowledgeItemInput) {
  return http<KnowledgeItem>({
    url: `/api/ai/knowledge/items/${encodeURIComponent(itemId)}`,
    method: 'put',
    data: input
  })
}

export function reviewKnowledgeItem(itemId: string, action: 'activate' | 'disable') {
  return http<KnowledgeItem>({
    url: `/api/ai/knowledge/items/${encodeURIComponent(itemId)}/review`,
    method: 'post',
    data: { action }
  })
}

export function deleteKnowledgeItem(itemId: string) {
  return http<{ deleted: boolean }>({
    url: `/api/ai/knowledge/items/${encodeURIComponent(itemId)}`,
    method: 'delete'
  })
}

/** 反馈聚合：把某应用下已确认有效的案例聚合成 draft（不覆盖 active 条目）。 */
export function aggregateKnowledgeFromFeedback(applicationId: string, minCases = 3) {
  return http<KnowledgeAggregateResult>({
    url: '/api/ai/knowledge/aggregate',
    method: 'post',
    data: { application_id: applicationId, min_cases: minCases }
  })
}

// ---- 记忆管理（ops/ai-contract.md §10.19，鉴权 app:ai:memory:manage）----

export type MemoryCaseStatus = 'active' | 'retired' | string

export interface MemoryCase {
  case_id: string
  source_type: string
  session_id?: string
  user_id: string
  application_id: string
  resource_id: string
  resource_type: string
  alert_signature?: string
  keywords?: string
  summary: string
  recommendations: string[]
  risk_level: string
  quality_score: number
  status: MemoryCaseStatus
  created_at: number
  updated_at: number
}

export interface MemoryCaseListParams {
  status?: string
  user_id?: string
  application_id?: string
  resource_id?: string
  keyword?: string
  limit?: number
  offset?: number
}

/** 管理端检索记忆条目；回答注入仍按归属用户隔离，本接口不影响检索范围。 */
export function listMemoryCases(params: MemoryCaseListParams = {}) {
  return http<{ items: MemoryCase[] }>({
    url: '/api/ai/memory/cases',
    method: 'get',
    params
  })
}

/** 手动停用/启用记忆条目；停用后不再参与后续回答检索，可重新启用。 */
export function updateMemoryCaseStatus(caseId: string, status: 'active' | 'retired') {
  return http<MemoryCase>({
    url: `/api/ai/memory/cases/${encodeURIComponent(caseId)}/status`,
    method: 'post',
    data: { status }
  })
}
