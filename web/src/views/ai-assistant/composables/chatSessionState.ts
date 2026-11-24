import type {
  ChatMessage,
  ChatSessionContext,
  ChatToolCall,
  SendChatMessageResult
} from '@/api/ai'

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

export interface QueuedChatMessage {
  queue_id: string
  session_id: string
  content: string
  images: string[]
  context: ChatSessionContext
  provider_id?: string
  client_message_id: string
}

/** 单个会话的独立聊天状态；不同会话可同时生成，互不影响。 */
export interface SessionChatState {
  messages: ChatMessage[]
  sending: boolean
  queuedMessages: QueuedChatMessage[]
  stage: ChatStage
  deltaChars: number
  liveTools: ChatToolCall[]
  liveThinking: string
  uiStatus: ChatUIStatus
  error: string
  lastResult: SendChatMessageResult | null
  lastUserContent: string
  lastUserImages: string[]
  lastClientMessageId: string
  streamController: AbortController | null
  /** 是否已加载过服务端消息；后台生成中的会话再次切回时直接用本地状态。 */
  loaded: boolean
}

export function createSessionChatState(): SessionChatState {
  return {
    messages: [],
    sending: false,
    queuedMessages: [],
    stage: 'idle',
    deltaChars: 0,
    liveTools: [],
    liveThinking: '',
    uiStatus: 'idle',
    error: '',
    lastResult: null,
    lastUserContent: '',
    lastUserImages: [],
    lastClientMessageId: '',
    streamController: null,
    loaded: false
  }
}