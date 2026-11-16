import { http } from './request'

export interface AIProvider {
  id: string
  name: string
  type: 'a' | 'b' | 'c' | string
  base_url: string
  model?: string
  api_key?: string
  has_api_key?: boolean
  timeout_ms?: number
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
  time_range?: string
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

/** 对话状态：ok / degraded / insufficient_context（后端 status 原值）。 */
export type ChatResultStatus = 'ok' | 'degraded' | 'insufficient_context' | string

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
  context?: ChatSessionContext
}

export function createChatSession(input: CreateChatSessionInput) {
  return http<ChatSession>({
    url: '/api/ai/chat/sessions',
    method: 'post',
    data: input
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
