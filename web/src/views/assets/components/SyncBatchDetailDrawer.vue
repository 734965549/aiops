<template>
  <a-drawer
    v-model:visible="visible"
    width="520px"
    title="同步批次详情"
    :footer="false"
  >
    <a-spin
      :loading="loading"
      style="width: 100%"
    >
      <a-empty
        v-if="!batchDetail"
        description="暂无批次详情"
      />
      <template v-else>
        <a-alert
          v-if="batchDetail.status === 'partial'"
          type="warning"
          class="sync-detail-alert"
        >
          部分资源或增强信息失败，基础同步结果以批次 summary 为准，message 保留为排查说明；partial 不是失败。
        </a-alert>
        <a-alert
          v-if="batchDetail.summary?.product_names_empty"
          type="warning"
          class="sync-detail-alert"
        >
          兜底白名单已启用：当前结果不保证完整性，不能当作权威全量数据使用。
        </a-alert>
        <a-alert
          v-if="batchDetail.summary?.partial_reason"
          type="info"
          class="sync-detail-alert"
        >
          {{ humanizeSyncPartialReason(batchDetail.summary.partial_reason) }}
        </a-alert>
        <SyncBatchOverviewSection :items="overviewItems" />
        <SyncBatchDiagnosticsSection
          :scopes="batchDetail.summary?.scopes"
          :scope-cards="scopeCards"
          :signal-tags="signalTags"
          :selected-scope-key="selectedScopeKey"
          :selected-scope-trace="selectedScopeTrace"
          :selected-scope-trace-cards="selectedScopeTraceCards"
          :selected-scope-trace-tags="selectedScopeTraceTags"
          :selected-scope-trace-snippet="selectedScopeTraceSnippet"
          :selected-scope-signal-key="selectedScopeSignalKey"
          :scope-diagnostic-columns="scopeDiagnosticColumns"
          @toggle-scope="emit('toggle-scope', $event)"
          @open-scope="emit('open-scope', $event)"
          @open-signal="emit('open-signal', $event)"
          @copy-snippet="emit('copy-snippet')"
        />
      </template>
    </a-spin>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { SyncBatch, SyncBatchScopeSummary } from '@/api/asset'
import type { SyncBatchMessageSummary } from '../composables/assetUtils'
import { formatSyncBatchSummaryDisplay, humanizeSyncPartialReason } from '../composables/assetUtils'
import SyncBatchOverviewSection from './SyncBatchOverviewSection.vue'
import SyncBatchDiagnosticsSection from './SyncBatchDiagnosticsSection.vue'

interface ScopeCard {
  key: string
  label: string
  region: string
  value: string | number
  hint: string
}

interface SignalTag {
  label: string
  value: string
  color: string
}

interface TraceCard {
  key: string
  label: string
  value: string | number
}

interface TraceTag {
  key: string
  label: string
  value: string
  color: string
}

const visible = defineModel<boolean>('visible', { default: false })

const props = defineProps<{
  batchDetail: SyncBatch | null
  loading: boolean
  messageSummary: SyncBatchMessageSummary
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

const summaryDisplay = computed(() => (props.batchDetail?.summary ? formatSyncBatchSummaryDisplay(props.batchDetail.summary) : undefined))

const overviewItems = computed(() => [
  { label: '批次 ID', render: props.batchDetail?.batch_id || '-' },
  { label: '账号', render: props.batchDetail?.integration_account_id || '-' },
  { label: 'Provider', render: props.batchDetail?.provider || '-' },
  { label: '同步模式', render: summaryDisplay.value?.sync_mode || '-' },
  { label: '模式回退', render: props.messageSummary.config_mode_fallback || '-' },
  { label: '资源组', render: summaryDisplay.value?.resource_group_name || summaryDisplay.value?.resource_group_id ? `${summaryDisplay.value.resource_group_name || '-'} / ${summaryDisplay.value.resource_group_id || '-'}` : '-' },
  ...(props.messageSummary.resource_group_selection === 'fallback' ? [{ label: '资源组使用回退值', render: '是' }] : []),
  { label: '状态', render: props.batchDetail ? props.batchDetail.status : '-' },
  { label: '数量摘要', render: props.batchDetail ? `新建 ${props.batchDetail.created_count}，更新 ${props.batchDetail.updated_count}，完成 ${props.batchDetail.completed_count}，stale ${props.batchDetail.stale_count}，失败 ${props.batchDetail.failed_count}` : '-' },
  { label: '原始 message', render: props.batchDetail?.message || '-' },
  { label: '开始时间', render: formatTs(props.batchDetail?.started_at) },
  { label: '结束时间', render: formatTs(props.batchDetail?.finished_at) }
])

const emit = defineEmits<{
  'toggle-scope': [key: string]
  'open-scope': [scope: SyncBatchScopeSummary]
  'open-signal': [key: string]
  'copy-snippet': []
}>()

function formatTs(ts?: number) {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}
</script>

<style scoped>
.sync-detail-message {
  color: var(--color-text-1);
  overflow-wrap: anywhere;
  word-break: break-word;
}

.sync-detail-alert {
  margin-bottom: 12px;
}

.sync-detail-scopes {
  margin-top: 16px;
}

.sync-detail-scopes-help {
  margin-left: 4px;
  color: var(--color-text-3);
  cursor: help;
  vertical-align: middle;
}

.sync-detail-scopes-grid {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  gap: 12px;
}

.sync-detail-scopes-panel {
  min-width: 0;
}

.sync-detail-scope-card {
  margin-bottom: 12px;
  cursor: pointer;
}

.sync-detail-scope-card-active {
  border-color: rgb(var(--primary-6));
  box-shadow: 0 0 0 1px rgba(var(--primary-6), 0.14);
}

.sync-detail-scope-card-label {
  font-size: 12px;
  color: var(--color-text-2);
}

.sync-detail-scope-card-value {
  margin-top: 4px;
  font-size: 18px;
  font-weight: 600;
  color: var(--color-text-1);
}

.sync-detail-scope-card-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--color-text-3);
}

.sync-detail-trace-panel {
  margin-top: 16px;
}

.sync-detail-trace-snippet-card {
  margin-top: 12px;
}

.sync-detail-trace-snippet {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  color: var(--color-text-1);
}

.sync-detail-signal-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

:deep(.sync-detail-signal-tag-active) {
  border-color: rgb(var(--primary-6));
  background: rgba(var(--primary-6), 0.08);
}

.sync-detail-scopes-title {
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--color-text-1);
}

.sync-detail-scopes-subtitle {
  font-size: 13px;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--color-text-2);
}
</style>
