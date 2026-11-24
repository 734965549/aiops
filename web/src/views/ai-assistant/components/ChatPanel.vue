<template>
  <div
    class="chat-panel"
    :class="{ 'chat-panel--compact': compact }"
  >
    <div
      ref="scrollRef"
      class="chat-stream"
    >
      <a-empty
        v-if="!messages.length && !sending"
        :description="emptyHint || '输入问题开始对话；AI 会自动带入当前页面上下文'"
      >
        <template #image>
          <IconRobot :size="42" />
        </template>
      </a-empty>

      <template
        v-for="message in messages"
        :key="message.message_id"
      >
        <ChatUserMessage
          v-if="message.role === 'user'"
          :message="message"
        />
        <div
          v-else
          class="message-row message-row--assistant"
        >
          <AnswerCard
            v-if="message.answer"
            :answer="message.answer"
            :tool-calls="message.tool_calls"
            :references="message.references"
            :feedback="message.feedback"
            :message-id="message.message_id"
            :busy="sending || proposing"
            @feedback="onFeedback"
            @propose="onPropose"
            @navigate="onNavigate"
          />
          <div
            v-else
            class="message-bubble message-bubble--assistant"
          >
            {{ message.content }}
          </div>
          <span class="message-time">{{ formatMessageTime(message.created_at) }}</span>
        </div>
      </template>

      <div
        v-if="sending && ((liveTools?.length ?? 0) > 0 || !!liveThinking)"
        class="live-process"
      >
        <div
          v-if="liveTools?.length"
          class="live-tools"
        >
          <div
            v-for="(tool, index) in liveTools"
            :key="`${tool.tool_code}-${index}`"
            class="live-tool-row"
          >
            <span
              class="live-tool-dot"
              :class="`is-${tool.status || 'ok'}`"
            />
            <span class="live-tool-name">{{ toolLabel(tool.tool_code) }}</span>
            <span
              class="live-tool-status"
              :class="{ 'is-bad': !!tool.status && tool.status !== 'ok' }"
            >
              {{ toolStatusLabel(tool.status) }}
            </span>
            <span
              v-if="tool.status !== 'ok' && tool.reason"
              class="live-tool-reason"
            >
              {{ toolReasonLabel(tool.reason) }}
            </span>
          </div>
        </div>
        <div
          v-if="liveThinking"
          class="live-thinking"
        >
          <button
            type="button"
            class="live-thinking-head"
            @click="thinkingOpen = !thinkingOpen"
          >
            <span>思考过程{{ stage === 'generating' ? '（已折叠）' : '' }}</span>
            <span class="live-thinking-meta">{{ thinkingChars }} 字</span>
          </button>
          <div
            v-show="thinkingOpen"
            ref="thinkingBody"
            class="live-thinking-body"
          >
            {{ liveThinking }}
          </div>
        </div>
      </div>
      <div
        v-if="sending"
        class="stage-indicator"
      >
        <a-spin :size="14" />
        <span>
          {{ STAGE_LABELS[stage === 'idle' ? 'thinking' : stage] }}
          <template v-if="stageDeltaChars">
            · 已生成 {{ stageDeltaChars }} 字
          </template>
          <template v-else-if="waitSeconds > 0">
            · 已等待 {{ waitSeconds }}s
          </template>
        </span>
      </div>
    </div>

    <div class="chat-status">
      <a-alert
        v-if="status === 'failed'"
        type="error"
        :show-icon="true"
      >
        {{ failedNotice || 'AI 回答失败，平台不会基于失败回答执行任何动作。可重新提问或稍后再试。' }}
        <template #action>
          <a-button
            size="mini"
            :disabled="!lastUserContent || sending"
            @click="emit('retry')"
          >
            重新提问
          </a-button>
        </template>
      </a-alert>
      <a-alert
        v-else-if="status === 'incomplete'"
        type="warning"
        :show-icon="true"
      >
        {{ incompleteHint }}
        <template #action>
          <a-button
            size="mini"
            :disabled="!lastUserContent || sending"
            @click="emit('retry')"
          >
            重新提问
          </a-button>
        </template>
      </a-alert>
      <a-alert
        v-else-if="status === 'inconclusive'"
        type="warning"
        :show-icon="true"
      >
        取数预算内未得出结论，以下为已获取的只读证据；可缩小问题范围或换模型重问。
        <template #action>
          <a-button
            size="mini"
            :disabled="!lastUserContent || sending"
            @click="emit('retry')"
          >
            重新提问
          </a-button>
        </template>
      </a-alert>
      <a-alert
        v-else-if="status === 'insufficient_context'"
        type="warning"
        :show-icon="true"
      >
        {{ contextHint?.text }}
        <template
          v-if="contextHint?.path"
          #action
        >
          <a-button
            size="mini"
            @click="navigateContextHint"
          >
            {{ contextHint?.label }}
          </a-button>
        </template>
      </a-alert>
      <a-alert
        v-else-if="status === 'not_executable'"
        type="warning"
        :show-icon="true"
      >
        AI 建议包含当前账号不可执行的动作，执行入口已禁用；原因见回答中的按钮提示。
      </a-alert>
      <a-alert
        v-else-if="status === 'needs_confirm'"
        type="info"
        :show-icon="true"
      >
        该建议涉及变更动作：仅可生成待确认执行任务，必须在执行确认页再次确认后才会执行。
      </a-alert>
      <a-alert
        v-if="error"
        type="error"
        :show-icon="true"
      >
        {{ error }}
      </a-alert>
    </div>

    <div class="chat-input">
      <QueuedMessageList
        v-if="queuedMessages?.length"
        :messages="queuedMessages"
        @remove="emit('remove-queued', $event)"
        @prioritize="emit('prioritize-queued', $event)"
        @clear="emit('clear-queued')"
      />
      <div
        v-if="quickQuestions && quickQuestions.length && !messages.length"
        class="quick-questions"
      >
        <span class="quick-label">快捷问题</span>
        <a-button
          v-for="question in quickQuestions"
          :key="question"
          size="mini"
          type="outline"
          @click="onQuickQuestion(question)"
        >
          {{ question }}
        </a-button>
      </div>
      <ChatImagePicker
        ref="pickerRef"
        v-model="images"
        v-model:attaching="attaching"
        :vision-enabled="visionEnabled"
      />
      <a-textarea
        v-model="input"
        :auto-size="{ minRows: compact ? 2 : 3, maxRows: 6 }"
        :placeholder="placeholder"
        @keydown.enter.exact.prevent="submit"
        @paste="onPaste"
      />
      <div class="input-foot">
        <div class="input-foot-main">
          <a-button
            size="mini"
            type="text"
            class="attach-button"
            :disabled="!visionEnabled || attaching"
            :title="attachHint"
            @click="pickerRef?.onPickImages()"
          >
            <IconImage :size="16" />
          </a-button>
          <ModelControl
            v-if="modelOptions.length"
            :models="modelOptions"
            :model-value="modelValue"
            @select-model="onSelectModel"
          />
          <span
            class="input-hint"
            title="Enter 发送 · Shift + Enter 换行 · 执行类动作需人工确认"
          >
            Enter 发送 · Shift + Enter 换行 · 执行类动作需人工确认
          </span>
        </div>
        <div class="input-actions">
          <a-button
            v-if="sending"
            size="small"
            status="warning"
            @click="emit('stop')"
          >
            {{ queuedMessages?.length ? '停止并清空队列' : '停止生成' }}
          </a-button>
          <a-button
            type="primary"
            size="small"
            :disabled="(!input.trim() && images.length === 0) || attaching"
            @click="submit"
          >
            发送
          </a-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { IconImage, IconRobot } from '@arco-design/web-vue/es/icon'
import type { ChatAnswer, ChatMessage, ChatModelInfo, ChatToolCall } from '@/api/ai'
import AnswerCard from './AnswerCard.vue'
import ModelControl from './ModelControl.vue'
import ChatImagePicker from './ChatImagePicker.vue'
import ChatUserMessage from './ChatUserMessage.vue'
import QueuedMessageList from './QueuedMessageList.vue'
import { MAX_CHAT_IMAGES } from '../composables/imageInput'
import { formatMessageTime } from '../composables/format'
import { toolLabel, toolReasonLabel, toolStatusLabel, incompleteReasonLabel } from '../toolReason'
import { STAGE_LABELS } from '../composables/useChat'
import type { ChatStage, ChatUIStatus, QueuedChatMessage } from '../composables/chatSessionState'

const props = defineProps<{
  messages: ChatMessage[]
  sending: boolean
  queuedMessages?: QueuedChatMessage[]
  stage: ChatStage
  stageDeltaChars?: number
  status: ChatUIStatus
  error?: string
  proposing?: boolean
  lastUserContent?: string
  quickQuestions?: string[]
  compact?: boolean
  emptyHint?: string
  placeholder?: string
  models?: ChatModelInfo[]
  modelValue?: string
  /** 当前选中模型是否接受图片输入；false 时禁用图片按钮与粘贴。 */
  visionEnabled?: boolean
  /** 发送中的实时工具调用过程（SSE tool 事件），结束后由最终回答的工具标签接管。 */
  liveTools?: ChatToolCall[]
  /** 发送中的模型思考增量（reasoning_content），仅流中展示，不落库。 */
  liveThinking?: string
}>()

const emit = defineEmits<{
  (e: 'send', content: string, images?: string[]): void
  (e: 'retry'): void
  (e: 'stop'): void
  (e: 'remove-queued', queueId: string): void
  (e: 'prioritize-queued', queueId: string): void
  (e: 'clear-queued'): void
  (e: 'feedback', messageId: string, feedback: 'valid' | 'invalid'): void
  (e: 'propose', answer: ChatAnswer, messageId?: string): void
  (e: 'navigate', url: string): void
  (e: 'select-model', providerID: string): void
}>()

const input = ref('')
const scrollRef = ref<HTMLElement | null>(null)
const placeholder = props.placeholder || '请输入你的问题，例如：帮我分析当前告警并给出排查步骤'
const modelOptions = computed(() => props.models ?? [])

const incompleteReason = computed(() => {
  if (props.status !== 'incomplete') return ''
  const lastAssistant = [...props.messages].reverse().find((message) => message.role === 'assistant')
  return (lastAssistant?.answer?.incomplete_reason ?? '').trim()
})
const incompleteHint = computed(() => {
  if (incompleteReason.value === 'output_truncated') {
    return '回答达到模型输出长度上限，以下为已保留的上下文与部分结果；建议减少规则条数或缩短时间窗口后重试。'
  }
  const label = incompleteReasonLabel(incompleteReason.value)
  return label
    ? `回答生成中断（${label}），以下为已保留的上下文与部分结果；可重新提问或缩小问题范围。`
    : '回答生成中断，以下为已保留的上下文与部分结果；可重新提问或缩小问题范围。'
})

// 图片输入：由 ChatImagePicker 负责压缩与预览，submit 时随消息发送（最多 3 张）。
const images = ref<string[]>([])
const attaching = ref(false)
const pickerRef = ref<{ addImageFiles: (files: ArrayLike<File> | null) => Promise<void>; onPickImages: () => void } | null>(null)
const attachHint = computed(() =>
  props.visionEnabled
    ? `添加图片（最多 ${MAX_CHAT_IMAGES} 张，支持直接粘贴截图）`
    : '当前模型未开启图片输入：请在 AI 配置为该模型开启视觉能力，或切换视觉模型'
)

function onPaste(event: ClipboardEvent) {
  const files = event.clipboardData?.files
  if (!files || files.length === 0) return
  event.preventDefault()
  void pickerRef.value?.addImageFiles(files)
}

// 实时思考过程：思考阶段默认展开，进入回答生成阶段自动折叠；有新增量时自动滚到底部。
const thinkingOpen = ref(true)
const thinkingBody = ref<HTMLElement | null>(null)
const thinkingChars = computed(() => Array.from(props.liveThinking ?? '').length)

watch(
  () => props.stage,
  (value) => {
    if (value === 'generating') thinkingOpen.value = false
    else if (value === 'thinking' || value === 'querying_tools') thinkingOpen.value = true
  }
)

watch(
  () => props.liveThinking,
  async () => {
    if (!thinkingOpen.value) return
    await nextTick()
    const el = thinkingBody.value
    if (el) el.scrollTop = el.scrollHeight
  }
)

function onSelectModel(value: unknown) {
  emit('select-model', String(value ?? ''))
}

// 首块前等待提示：发送期间每秒累计，收到首个回答增量后切换为“已生成 N 字”。
const waitSeconds = ref(0)
let waitTimer: number | null = null

function stopWaitTimer() {
  if (waitTimer !== null) {
    window.clearInterval(waitTimer)
    waitTimer = null
  }
}

watch(
  () => props.sending,
  (sending) => {
    stopWaitTimer()
    waitSeconds.value = 0
    if (sending) {
      waitTimer = window.setInterval(() => {
        waitSeconds.value += 1
      }, 1000)
    }
  },
  { immediate: true }
)

onUnmounted(stopWaitTimer)

const lastAssistantAnswer = computed(
  () => [...props.messages].reverse().find((message) => message.role === 'assistant')?.answer
)

/** 限流失败时直接展示预计恢复时间，避免只给“稍后重试”。 */
const failedNotice = computed(() => {
  if (props.status !== 'failed') return ''
  const answer = lastAssistantAnswer.value
  if (answer?.degraded_reason !== 'provider_rate_limited') return ''
  return answer.uncertainties?.find((text) => text.includes('恢复')) || '模型服务当前限流，请稍后重试'
})

interface ContextHint {
  text: string
  path?: string
  label?: string
}

/** 上下文缺失原因 -> 具体补全指引（reason 码由服务端工具观测返回）。 */
const CONTEXT_HINT_RULES: Array<{ reasons: string[]; hint: ContextHint }> = [
  {
    reasons: ['no_account'],
    hint: {
      text: '缺少集成账号：请到云账号接入确认可用账号，或在提问中写明账号 ID；也可打开告警/资产详情后自动带入上下文。',
      path: '/integrations',
      label: '打开云账号接入'
    }
  },
  {
    reasons: ['no_namespace', 'no_dimensions'],
    hint: {
      text: '缺少命名空间或实例维度：在资产管理选中对应云资源后提问，或在提问中给出实例 ID。',
      path: '/assets',
      label: '打开资产管理'
    }
  },
  {
    reasons: ['no_region'],
    hint: {
      text: '缺少区域上下文：请在提问中补充 region，或从资产/观测页带入上下文。',
      path: '/assets',
      label: '打开资产管理'
    }
  },
  {
    reasons: ['no_metric'],
    hint: {
      text: '缺少指标名：请在提问中给出指标名，例如 SYS.RDS 的 cpu_util。'
    }
  }
]

const contextHint = computed<ContextHint | null>(() => {
  if (props.status !== 'insufficient_context') return null
  const lastAssistant = [...props.messages].reverse().find((message) => message.role === 'assistant')
  const reasons = new Set(
    (lastAssistant?.tool_calls ?? [])
      .filter((call) => call.status === 'skipped' && call.reason)
      .map((call) => call.reason as string)
  )
  for (const rule of CONTEXT_HINT_RULES) {
    if (rule.reasons.some((reason) => reasons.has(reason))) return rule.hint
  }
  return { text: '当前上下文不足，请先打开告警或资源详情，或在提问中补充应用、资源与时间范围。' }
})

function navigateContextHint() {
  const path = contextHint.value?.path
  if (path) emit('navigate', path)
}

watch(
  () => props.messages.length,
  async () => {
    await nextTick()
    const el = scrollRef.value
    if (el) el.scrollTop = el.scrollHeight
  }
)

watch(
  () => props.sending,
  async () => {
    await nextTick()
    const el = scrollRef.value
    if (el) el.scrollTop = el.scrollHeight
  }
)

function submit() {
  const content = input.value.trim()
  if (!content && images.value.length === 0) return
  const payload = [...images.value]
  input.value = ''
  images.value = []
  emit('send', content, payload)
}

function onQuickQuestion(question: string) {
  emit('send', question)
}

function onFeedback(messageId: string, feedback: 'valid' | 'invalid') {
  emit('feedback', messageId, feedback)
}

function onPropose(answer: ChatAnswer, messageId?: string) {
  emit('propose', answer, messageId)
}

function onNavigate(url: string) {
  emit('navigate', url)
}
</script>

<style scoped>
.chat-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.chat-stream {
  flex: 1;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 4px 2px 8px;
}

.message-row {
  display: flex;
  flex-direction: column;
  max-width: 100%;
}

.message-row--assistant {
  align-items: stretch;
  gap: 4px;
}

.message-bubble {
  max-width: 86%;
  padding: 9px 12px;
  border-radius: var(--radius-md);
  font-size: 13px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.attach-button {
  flex: none;
  padding: 0 4px;
  color: var(--color-text-2);
}

.message-bubble--assistant {
  max-width: 100%;
  color: var(--color-text-1);
  background: var(--color-fill-1);
  border-bottom-left-radius: 4px;
}

.message-time {
  margin-top: 2px;
  color: var(--color-text-3);
  font-size: 10px;
}

.stage-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--color-text-2);
  font-size: 12px;
}

.live-process {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid rgba(81, 102, 63, 0.14);
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.6);
  font-size: 12px;
}

.live-tools {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.live-tool-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.live-tool-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #7d9c5c;
}

.live-tool-dot.is-skipped {
  background: #d4a017;
}

.live-tool-dot.is-denied,
.live-tool-dot.is-error {
  background: #b4483f;
}

.live-tool-name {
  color: var(--color-text-1);
  flex: none;
}

.live-tool-status {
  color: var(--color-text-3);
  flex: none;
}

.live-tool-status.is-bad {
  color: #b4483f;
}

.live-tool-reason {
  color: var(--color-text-3);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.live-thinking {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.live-thinking-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--color-text-2);
  font-size: 11px;
  cursor: pointer;
  text-align: left;
}

.live-thinking-meta {
  color: var(--color-text-3);
}

.live-thinking-body {
  max-height: 160px;
  overflow-y: auto;
  padding: 6px 8px;
  border-radius: 4px;
  background: rgba(81, 102, 63, 0.06);
  color: var(--color-text-2);
  font-size: 11px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.chat-status {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-status :deep(.arco-alert) {
  padding: 6px 10px;
  font-size: 12px;
}

.chat-input {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid rgba(81, 102, 63, 0.14);
}

.quick-questions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.quick-label {
  color: var(--color-text-3);
  font-size: 11px;
}

.input-foot {
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.input-foot-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.input-hint {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-3);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-panel--compact .chat-stream {
  gap: 10px;
}

.chat-panel--compact .message-bubble {
  font-size: 12px;
}
</style>
