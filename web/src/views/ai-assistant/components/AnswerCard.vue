<template>
  <div class="answer-card">
    <div class="answer-head">
      <div class="answer-tags">
        <a-tag :color="riskColor(answer.risk_level)">
          风险等级：{{ riskLabel(answer.risk_level) }}
        </a-tag>
        <a-tag :color="answer.require_confirm ? 'orange' : 'gray'">
          {{ answer.require_confirm ? '需要人工确认' : '无需人工确认' }}
        </a-tag>
        <a-tag
          v-if="answer.degraded"
          color="red"
        >
          降级回答
        </a-tag>
        <a-tag
          v-else-if="answer.insufficient_context"
          color="gray"
        >
          上下文不足
        </a-tag>
      </div>
      <div
        v-if="messageId"
        class="answer-feedback"
      >
        <a-button
          size="mini"
          type="text"
          :class="{ 'is-active': feedback === 'valid' }"
          :disabled="busy"
          @click="emit('feedback', messageId, 'valid')"
        >
          <template #icon>
            <IconThumbUp />
          </template>
          有效
        </a-button>
        <a-button
          size="mini"
          type="text"
          :class="{ 'is-active--bad': feedback === 'invalid' }"
          :disabled="busy"
          @click="emit('feedback', messageId, 'invalid')"
        >
          <template #icon>
            <IconThumbDown />
          </template>
          无效
        </a-button>
      </div>
    </div>

    <section class="answer-section answer-summary">
      <h4>结论摘要</h4>
      <p>{{ answer.summary || '（模型未返回摘要）' }}</p>
    </section>

    <section
      v-if="answer.evidence.length"
      class="answer-section"
    >
      <h4>分析依据</h4>
      <ul>
        <li
          v-for="(item, index) in answer.evidence"
          :key="`evidence-${index}`"
        >
          {{ item }}
        </li>
      </ul>
    </section>

    <section
      v-if="answer.impact.length"
      class="answer-section"
    >
      <h4>影响范围</h4>
      <ul>
        <li
          v-for="(item, index) in answer.impact"
          :key="`impact-${index}`"
        >
          {{ item }}
        </li>
      </ul>
    </section>

    <section
      v-if="answer.recommendations.length"
      class="answer-section"
    >
      <h4>推荐操作</h4>
      <ol>
        <li
          v-for="(item, index) in answer.recommendations"
          :key="`recommendation-${index}`"
        >
          {{ item }}
        </li>
      </ol>
    </section>

    <section
      v-if="answer.rollback"
      class="answer-section"
    >
      <h4>回滚建议</h4>
      <p>{{ answer.rollback }}</p>
    </section>

    <section
      v-if="answer.uncertainties.length"
      class="answer-section answer-uncertainty"
    >
      <h4>不确定性说明</h4>
      <ul>
        <li
          v-for="(item, index) in answer.uncertainties"
          :key="`uncertainty-${index}`"
        >
          {{ item }}
        </li>
      </ul>
    </section>

    <section
      v-if="toolCalls.length"
      class="answer-section"
    >
      <h4>工具调用</h4>
      <div class="tool-tags">
        <a-tooltip
          v-for="call in toolCalls"
          :key="call.tool_code"
          :content="toolTooltip(call)"
        >
          <a-tag :color="call.allowed ? 'arcoblue' : 'red'">
            {{ toolLabel(call.tool_code) }}
            <span class="tool-status">{{ call.allowed ? call.status : '被拒绝' }}</span>
          </a-tag>
        </a-tooltip>
      </div>
    </section>

    <section
      v-if="references.length"
      class="answer-section"
    >
      <h4>引用数据</h4>
      <div class="reference-list">
        <a-link
          v-for="ref in references"
          :key="`${ref.type}-${ref.id}`"
          :disabled="!ref.url"
          @click="onNavigate(ref.url)"
        >
          <template #icon>
            <IconLink />
          </template>
          {{ ref.label || ref.id }}
        </a-link>
      </div>
    </section>

    <div
      v-if="answer.actions.length"
      class="answer-actions"
    >
      <a-tooltip
        v-for="action in answer.actions"
        :key="action.code"
        :content="action.allowed ? '' : action.reason || '当前不可执行'"
        :disabled="action.allowed"
      >
        <a-button
          size="small"
          :type="action.code === 'execution.propose' ? 'primary' : 'outline'"
          :disabled="!action.allowed || busy"
          :loading="action.code === 'execution.propose' && busy"
          @click="onAction(action)"
        >
          {{ action.label }}
        </a-button>
      </a-tooltip>
      <span class="action-hint">执行类动作只创建待确认任务，不会直接执行</span>
      <span
        v-if="deniedReason"
        class="action-denied"
      >
        不可执行原因：{{ deniedReason }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IconLink, IconThumbDown, IconThumbUp } from '@arco-design/web-vue/es/icon'
import type { ChatAction, ChatAnswer, ChatReference, ChatToolCall } from '@/api/ai'

const props = defineProps<{
  answer: ChatAnswer
  toolCalls?: ChatToolCall[]
  references?: ChatReference[]
  feedback?: string
  messageId?: string
  busy?: boolean
}>()

const emit = defineEmits<{
  (e: 'feedback', messageId: string, feedback: 'valid' | 'invalid'): void
  (e: 'propose', answer: ChatAnswer, messageId?: string): void
  (e: 'navigate', url: string): void
}>()

const toolCalls = computed(() => props.toolCalls ?? [])
const references = computed(() => props.references ?? [])
// 被拒绝的动作必须在界面上直接说明原因，不能只依赖禁用态 tooltip。
const deniedReason = computed(
  () => props.answer.actions?.find((action) => !action.allowed && action.reason)?.reason ?? ''
)

const RISK_COLORS: Record<string, string> = {
  low: 'green',
  medium: 'orange',
  high: 'red',
  info: 'gray'
}

const RISK_LABELS: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
  info: '信息'
}

const TOOL_LABELS: Record<string, string> = {
  'alarm.analyze': '读取告警快照',
  'metrics.query': '查询指标证据',
  'execution.tasks.propose': '生成执行计划权限'
}

function riskColor(level: string) {
  return RISK_COLORS[level] ?? 'gray'
}

function riskLabel(level: string) {
  return RISK_LABELS[level] ?? level
}

function toolLabel(code: string) {
  return TOOL_LABELS[code] ?? code
}

function toolTooltip(call: ChatToolCall) {
  const parts = [call.summary || toolLabel(call.tool_code)]
  if (!call.allowed && call.reason) parts.push(`原因：${call.reason}`)
  if (call.mode) parts.push(`模式：${call.mode}`)
  return parts.join(' · ')
}

function onAction(action: ChatAction) {
  if (!action.allowed) return
  if (action.code === 'execution.propose') {
    emit('propose', props.answer, props.messageId)
    return
  }
  onNavigate(action.target)
}

function onNavigate(url?: string) {
  if (url) emit('navigate', url)
}
</script>

<style scoped>
.answer-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid rgba(81, 102, 63, 0.14);
  border-radius: var(--radius-md);
  background: rgba(255, 255, 255, 0.86);
}

.answer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}

.answer-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.answer-feedback {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.answer-feedback .is-active {
  color: #2f7d4f;
  background: rgba(47, 125, 79, 0.1);
}

.answer-feedback .is-active--bad {
  color: #b4483f;
  background: rgba(180, 72, 63, 0.1);
}

.answer-section h4 {
  margin: 0 0 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-2);
  letter-spacing: 0.02em;
}

.answer-section p {
  margin: 0;
  font-size: 13px;
  line-height: 1.65;
  color: var(--color-text-1);
  white-space: pre-wrap;
}

.answer-section ul,
.answer-section ol {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-1);
}

.answer-summary p {
  font-size: 14px;
  font-weight: 500;
}

.answer-uncertainty {
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  background: rgba(255, 196, 105, 0.12);
}

.answer-uncertainty h4 {
  color: #97661b;
}

.answer-uncertainty ul {
  color: #7d5a1f;
}

.tool-tags,
.reference-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.tool-status {
  margin-left: 2px;
  opacity: 0.75;
  font-size: 11px;
}

.reference-list {
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.reference-list :deep(.arco-link) {
  font-size: 12px;
}

.answer-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding-top: 4px;
  border-top: 1px dashed rgba(81, 102, 63, 0.16);
}

.action-hint {
  color: var(--color-text-3);
  font-size: 11px;
}

.action-denied {
  width: 100%;
  color: #b4483f;
  font-size: 11px;
  line-height: 1.5;
}
</style>
