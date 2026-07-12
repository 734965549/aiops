<template>
  <div
    v-if="scopes?.length"
    class="sync-detail-scopes"
  >
    <div class="sync-detail-scopes-title">
      按区域摘要
      <a-tooltip
        content="左侧为可执行字段，用于确认本轮实际执行的 region / project / scope；右侧为诊断字段，用于排查失败与截断信号。"
      >
        <IconQuestionCircle class="sync-detail-scopes-help" />
      </a-tooltip>
    </div>
    <div class="sync-detail-scopes-grid">
      <div class="sync-detail-scopes-panel">
        <div class="sync-detail-scopes-subtitle">
          执行摘要卡片
        </div>
        <a-row :gutter="12">
          <a-col
            v-for="item in scopeCards"
            :key="item.key"
            :span="12"
          >
            <a-card
              class="sync-detail-scope-card"
              :class="{ 'sync-detail-scope-card-active': selectedScopeKey === item.key }"
              :bordered="true"
              size="small"
              @click="item.key && emit('toggle-scope', item.key)"
            >
              <div class="sync-detail-scope-card-label">
                {{ item.label }}
              </div>
              <div class="sync-detail-scope-card-value">
                {{ item.value }}
              </div>
              <div class="sync-detail-scope-card-hint">
                {{ item.hint }}
              </div>
            </a-card>
          </a-col>
        </a-row>
      </div>
      <div class="sync-detail-scopes-panel">
        <div class="sync-detail-scopes-subtitle">
          诊断表格
        </div>
        <a-table
          :data="scopes"
          :columns="scopeDiagnosticColumns"
          :pagination="false"
          size="small"
          :scroll="{ x: 840 }"
          :row-class="(record: unknown) => (selectedScopeKey && scopeKey(record as SyncBatchScopeSummary) === selectedScopeKey ? 'sync-detail-scope-row-active' : '')"
          @row-click="(record: unknown) => emit('open-scope', record as SyncBatchScopeSummary)"
        >
          <template #failed_scopes="{ record }">
            <span v-if="record.failed_scopes?.length">{{ record.failed_scopes.join(', ') }}</span>
            <span v-else>-</span>
          </template>
          <template #query_failed_types="{ record }">
            <span v-if="record.query_failed_types?.length">{{ record.query_failed_types.join(', ') }}</span>
            <span v-else>-</span>
          </template>
          <template #conversion_failed_types="{ record }">
            <span v-if="record.conversion_failed_types?.length">{{ record.conversion_failed_types.join(', ') }}</span>
            <span v-else>-</span>
          </template>
          <template #enrichment_failed_types="{ record }">
            <span v-if="record.enrichment_failed_types?.length">{{ record.enrichment_failed_types.join(', ') }}</span>
            <span v-else>-</span>
          </template>
          <template #enrichment_warnings="{ record }">
            <span v-if="record.enrichment_warnings?.length">{{ record.enrichment_warnings.join(', ') }}</span>
            <span v-else>-</span>
          </template>
          <template #max_resources_reached="{ record }">
            <a-tag
              v-if="record.max_resources_reached"
              color="orange"
              size="small"
            >
              是
            </a-tag>
            <span v-else>-</span>
          </template>
        </a-table>
      </div>
    </div>
    <div
      v-if="selectedScopeTrace"
      class="sync-detail-trace-panel"
    >
      <div class="sync-detail-scopes-subtitle">
        当前 scope 追踪
      </div>
      <div class="sync-detail-signal-tags">
        <a-tag
          v-for="tag in selectedScopeTraceTags"
          :key="tag.key"
          :color="tag.color"
          size="small"
          :class="{ 'sync-detail-signal-tag-active': selectedScopeSignalKey === tag.key }"
          @click="emit('open-signal', tag.key)"
        >
          {{ tag.label }}：{{ tag.value }}
        </a-tag>
      </div>
      <a-row :gutter="12">
        <a-col
          v-for="item in selectedScopeTraceCards"
          :key="item.key"
          :span="12"
        >
          <a-card
            class="sync-detail-scope-card"
            :bordered="true"
            size="small"
          >
            <div class="sync-detail-scope-card-label">
              {{ item.label }}
            </div>
            <div class="sync-detail-scope-card-value">
              {{ item.value }}
            </div>
          </a-card>
        </a-col>
      </a-row>
      <a-card
        class="sync-detail-trace-snippet-card"
        :bordered="true"
        size="small"
      >
        <template #title>
          三段式排障定位块
        </template>
        <template #extra>
          <a-button
            type="text"
            size="small"
            @click="emit('copy-snippet')"
          >
            复制
          </a-button>
        </template>
        <pre class="sync-detail-trace-snippet">{{ selectedScopeTraceSnippet || '-' }}</pre>
      </a-card>
    </div>
    <div class="sync-detail-signal-tags">
      <a-tag
        v-for="tag in signalTags"
        :key="tag.label + tag.value"
        :color="tag.color"
        size="small"
      >
        {{ tag.label }}：{{ tag.value }}
      </a-tag>
    </div>
  </div>
</template>

<script setup lang="ts">
import { IconQuestionCircle } from '@arco-design/web-vue/es/icon'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { SyncBatchScopeSummary } from '@/api/asset'

interface ScopeCard { key: string; label: string; region: string; value: string | number; hint: string }
interface SignalTag { label: string; value: string; color: string }
interface TraceCard { key: string; label: string; value: string | number }
interface TraceTag { key: string; label: string; value: string; color: string }

defineProps<{
  scopes?: SyncBatchScopeSummary[]
  scopeCards: ScopeCard[]
  signalTags: SignalTag[]
  selectedScopeKey: string
  selectedScopeTrace: SyncBatchScopeSummary | null
  selectedScopeTraceCards: TraceCard[]
  selectedScopeTraceTags: TraceTag[]
  selectedScopeTraceSnippet: string
  selectedScopeSignalKey: string
  scopeDiagnosticColumns: TableInstance['columns']
}>()

const emit = defineEmits<{
  'toggle-scope': [key: string]
  'open-scope': [scope: SyncBatchScopeSummary]
  'open-signal': [key: string]
  'copy-snippet': []
}>()

function scopeKey(scope: SyncBatchScopeSummary): string {
  return [scope.region, scope.project_id || '', scope.resource_group_id || '', scope.resource_group_name || ''].join('|')
}
</script>
