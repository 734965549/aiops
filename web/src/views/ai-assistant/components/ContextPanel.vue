<template>
  <div class="context-panel">
    <section>
      <h4>当前上下文</h4>
      <div class="context-grid">
        <span class="context-key">页面</span>
        <span class="context-value">{{ context.page_title || context.page || '未关联' }}</span>
        <template v-if="context.alert_id">
          <span class="context-key">告警</span>
          <a-link @click="emit('navigate', alertURL)">
            {{ context.alert_id }}
          </a-link>
        </template>
        <template v-if="context.application_id">
          <span class="context-key">应用</span>
          <span class="context-value">{{ context.application_id }}</span>
        </template>
        <template v-if="context.resource_id">
          <span class="context-key">资源</span>
          <span class="context-value">
            {{ context.resource_id }}
            <em v-if="context.resource_type">（{{ context.resource_type }}）</em>
          </span>
        </template>
        <template v-if="context.time_range">
          <span class="context-key">时间范围</span>
          <span class="context-value">{{ context.time_range }}</span>
        </template>
      </div>
      <div
        v-if="!hasResource"
        class="context-hint"
      >
        未关联告警或资源；打开告警/资源详情后提问，AI 会自动带入上下文。
      </div>
    </section>

    <section>
      <h4>可用工具</h4>
      <div class="tool-list">
        <div
          v-for="tool in TOOL_CATALOG"
          :key="tool.code"
          class="tool-item"
        >
          <div class="tool-head">
            <span class="tool-name">{{ tool.name }}</span>
            <a-tag
              size="small"
              :color="tool.mode === 'require_confirm' ? 'orange' : 'arcoblue'"
            >
              {{ tool.mode === 'require_confirm' ? '需确认' : '只读' }}
            </a-tag>
          </div>
          <div class="tool-meta">
            <span>{{ tool.code }}</span>
            <a-tag
              v-if="toolState(tool.code)"
              size="small"
              :color="toolState(tool.code)?.allowed ? 'green' : 'red'"
            >
              {{ toolState(tool.code)?.allowed ? '已调用' : '被拒绝' }}
            </a-tag>
          </div>
          <div
            v-if="toolState(tool.code)?.reason"
            class="tool-reason"
          >
            {{ toolReasonLabel(toolState(tool.code)?.reason) }}
          </div>
        </div>
      </div>
      <div class="context-hint">
        工具调用继承当前用户身份，服务端二次校验 RBAC 与工具权限；AI 不能直接执行命令。
      </div>
    </section>

    <section v-if="references.length">
      <h4>引用数据</h4>
      <div class="reference-list">
        <a-link
          v-for="ref in references"
          :key="`${ref.type}-${ref.id}`"
          :disabled="!ref.url"
          @click="ref.url && emit('navigate', ref.url)"
        >
          {{ ref.label || ref.id }}
        </a-link>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChatReference, ChatSessionContext, ChatToolCall } from '@/api/ai'
import { toolReasonLabel } from '../toolReason'

const props = defineProps<{
  context: ChatSessionContext
  toolCalls: ChatToolCall[]
  references: ChatReference[]
}>()

const emit = defineEmits<{
  (e: 'navigate', url: string): void
}>()

const TOOL_CATALOG = [
  { code: 'alarm.analyze', name: '读取告警快照', mode: 'read_only' },
  { code: 'metrics.query', name: '查询指标证据', mode: 'read_only' },
  { code: 'alert.rules.list', name: '查询告警规则', mode: 'read_only' },
  { code: 'accounts.list', name: '列出集成账号', mode: 'read_only' },
  { code: 'resource.analyze', name: '读取资源上下文', mode: 'read_only' },
  { code: 'resources.list', name: '发现云同步资源', mode: 'read_only' },
  { code: 'execution.tasks.propose', name: '生成待确认任务', mode: 'require_confirm' }
]

const toolCalls = computed(() => props.toolCalls ?? [])
const references = computed(() => props.references ?? [])
const hasResource = computed(
  () => !!(props.context.alert_id || props.context.application_id || props.context.resource_id)
)
const alertURL = computed(
  () => `/alerts?alert_id=${encodeURIComponent(props.context.alert_id || '')}`
)

function toolState(code: string): ChatToolCall | undefined {
  return [...toolCalls.value].reverse().find((call) => call.tool_code === code)
}
</script>

<style scoped>
.context-panel {
  flex: 1;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-size: 12px;
}

.context-panel h4 {
  margin: 0 0 8px;
  color: var(--color-text-2);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.context-grid {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr);
  gap: 4px 8px;
  align-items: baseline;
}

.context-key {
  color: var(--color-text-3);
  font-size: 11px;
}

.context-value {
  overflow-wrap: anywhere;
  color: var(--color-text-1);
}

.context-value em {
  color: var(--color-text-3);
  font-style: normal;
}

.context-hint {
  margin-top: 8px;
  color: var(--color-text-3);
  font-size: 11px;
  line-height: 1.55;
}

.tool-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tool-item {
  padding: 8px 10px;
  border: 1px solid rgba(81, 102, 63, 0.12);
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.6);
}

.tool-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.tool-name {
  color: var(--color-text-1);
  font-weight: 500;
}

.tool-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  margin-top: 4px;
  color: var(--color-text-3);
  font-size: 10px;
}

.tool-reason {
  margin-top: 4px;
  color: #b4483f;
  font-size: 11px;
  line-height: 1.5;
}

.reference-list {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}

.reference-list :deep(.arco-link) {
  font-size: 12px;
}
</style>
