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
        <div
          v-if="message.role === 'user'"
          class="message-row message-row--user"
        >
          <div class="message-bubble message-bubble--user">
            <button
              type="button"
              class="message-copy"
              title="复制消息"
              @click="copyUserMessage(message.content)"
            >
              <IconCopy :size="12" />
            </button>
            {{ message.content }}
            <span class="message-time">{{ formatTime(message.created_at) }}</span>
          </div>
        </div>
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
          <span class="message-time">{{ formatTime(message.created_at) }}</span>
        </div>
      </template>

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
        回答生成中断，以下为已保留的上下文与部分结果；可重新提问或缩小问题范围。
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
      <div
        v-if="modelOptions.length > 1"
        class="model-picker"
      >
        <span class="model-label">模型</span>
        <a-select
          :model-value="modelValue"
          size="mini"
          :disabled="modelLocked"
          class="model-select"
          @change="onSelectModel"
        >
          <a-option
            v-for="model in modelOptions"
            :key="model.provider_id"
            :value="model.provider_id"
          >
            {{ model.name }}<template v-if="model.model">（{{ model.model }}）</template>
          </a-option>
        </a-select>
        <span
          v-if="modelLocked"
          class="model-hint"
        >
          会话已开始，切换模型请新建会话
        </span>
      </div>
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
          :disabled="sending"
          @click="onQuickQuestion(question)"
        >
          {{ question }}
        </a-button>
      </div>
      <a-textarea
        v-model="input"
        :auto-size="{ minRows: compact ? 2 : 3, maxRows: 6 }"
        :placeholder="placeholder"
        :disabled="sending"
        @keydown.enter.exact.prevent="submit"
      />
      <div class="input-foot">
        <span class="input-hint">Enter 发送 · Shift + Enter 换行 · 执行类动作需人工确认</span>
        <a-button
          v-if="sending"
          size="small"
          status="warning"
          @click="emit('stop')"
        >
          停止生成
        </a-button>
        <a-button
          v-else
          type="primary"
          size="small"
          :disabled="!input.trim()"
          @click="submit"
        >
          发送
        </a-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { IconCopy, IconRobot } from '@arco-design/web-vue/es/icon'
import type { ChatAnswer, ChatMessage, ChatModelInfo } from '@/api/ai'
import AnswerCard from './AnswerCard.vue'
import { copyText } from '../composables/useCopy'
import { STAGE_LABELS, type ChatStage, type ChatUIStatus } from '../composables/useChat'

const props = defineProps<{
  messages: ChatMessage[]
  sending: boolean
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
  modelLocked?: boolean
}>()

const emit = defineEmits<{
  (e: 'send', content: string): void
  (e: 'retry'): void
  (e: 'stop'): void
  (e: 'feedback', messageId: string, feedback: 'valid' | 'invalid'): void
  (e: 'propose', answer: ChatAnswer, messageId?: string): void
  (e: 'navigate', url: string): void
  (e: 'select-model', providerID: string): void
}>()

const input = ref('')
const scrollRef = ref<HTMLElement | null>(null)
const placeholder = props.placeholder || '请输入你的问题，例如：帮我分析当前告警并给出排查步骤'
const modelOptions = computed(() => props.models ?? [])

function onSelectModel(value: unknown) {
  emit('select-model', String(value ?? ''))
}

async function copyUserMessage(content: string) {
  await copyText(content, '已复制消息')
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
  if (!content || props.sending) return
  input.value = ''
  emit('send', content)
}

function onQuickQuestion(question: string) {
  if (props.sending) return
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

function formatTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
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

.message-row--user {
  align-items: flex-end;
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

.message-bubble--user {
  position: relative;
  padding-right: 26px;
  color: #ffffff;
  background: #51663f;
  border-bottom-right-radius: 4px;
}

.message-copy {
  position: absolute;
  top: 4px;
  right: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: 4px;
  color: rgba(255, 255, 255, 0.9);
  background: transparent;
  opacity: 0;
  cursor: pointer;
  transition: opacity 0.15s ease;
}

.message-bubble--user:hover .message-copy {
  opacity: 0.9;
}

.message-copy:hover {
  background: rgba(255, 255, 255, 0.16);
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

.model-picker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.model-label {
  color: var(--color-text-3);
  font-size: 11px;
}

.model-select {
  width: 240px;
}

.model-hint {
  color: var(--color-text-3);
  font-size: 10px;
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
  justify-content: space-between;
  gap: 8px;
}

.input-hint {
  color: var(--color-text-3);
  font-size: 10px;
}

.chat-panel--compact .chat-stream {
  gap: 10px;
}

.chat-panel--compact .message-bubble {
  font-size: 12px;
}
</style>
