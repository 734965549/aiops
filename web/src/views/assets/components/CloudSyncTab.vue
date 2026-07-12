<template>
  <a-card
    title="同步批次"
    :bordered="false"
    class="assets-card assets-card-fixed"
  >
    <template #extra>
      <a-space>
        <a-input
          v-model="syncAccountId"
          placeholder="接入账号 ID"
          style="width: 280px"
          allow-clear
        />
        <a-button
          type="primary"
          :loading="syncLoading"
          @click="emit('trigger-sync')"
        >
          立即同步
        </a-button>
        <a-button @click="emit('refresh')">
          刷新
        </a-button>
      </a-space>
    </template>
    <a-table
      :columns="syncBatchColumns"
      :data="syncBatches"
      :loading="syncBatchesLoading"
      row-key="batch_id"
      :pagination="syncPagination"
      :scroll="tableScroll"
      :bordered="false"
      @page-change="(page: number) => emit('page-change', page)"
      @page-size-change="(size: number) => emit('page-size-change', size)"
    >
      <template #status="{ record }">
        <a-tag :color="syncStatusColor((record as SyncBatch).status)">
          {{ statusLabel((record as SyncBatch).status) }}
        </a-tag>
      </template>
      <template #signal="{ record }">
        <a-tooltip
          v-if="hasSyncWarning(record as SyncBatch)"
          :content="signalHint(record as SyncBatch)"
        >
          <a-tag
            :color="signalTagColor(record as SyncBatch)"
            size="small"
          >
            风险
          </a-tag>
        </a-tooltip>
        <span v-else>-</span>
      </template>
      <template #message="{ record }">
        <a-tooltip
          v-if="(record as SyncBatch).message"
          :content="(record as SyncBatch).message"
        >
          <span class="assets-text-ellipsis">{{ (record as SyncBatch).message }}</span>
        </a-tooltip>
        <span v-else>-</span>
      </template>
      <template #actions="{ record }">
        <a-button
          type="text"
          size="small"
          @click="emit('open-detail', record as SyncBatch)"
        >
          详情
        </a-button>
      </template>
    </a-table>
  </a-card>
</template>

<script setup lang="ts">
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { SyncBatch } from '@/api/asset'

interface PaginationConfig {
  current: number
  pageSize: number
  total: number
  showTotal: boolean
  showPageSize: boolean
}

defineProps<{
  syncBatches: SyncBatch[]
  syncBatchesLoading: boolean
  syncBatchColumns: TableInstance['columns']
  syncPagination: PaginationConfig
  tableScroll: Record<string, unknown>
  syncLoading: boolean
}>()

const syncAccountId = defineModel<string>({ default: '' })

const emit = defineEmits<{
  'trigger-sync': []
  'refresh': []
  'open-detail': [batch: SyncBatch]
  'page-change': [page: number]
  'page-size-change': [size: number]
}>()

function hasSyncWarning(batch: SyncBatch) {
  return Boolean(batch.summary?.product_names_empty || batch.summary?.max_resources_reached || (batch.summary?.query_failed_types?.length ?? 0) > 0 || (batch.summary?.conversion_failed_types?.length ?? 0) > 0)
}

function signalHint(batch: SyncBatch) {
  const parts: string[] = []
  if (batch.summary?.product_names_empty) parts.push('兜底发现，可能不完整')
  if (batch.summary?.max_resources_reached) parts.push('查询上限截断，结果可能不完整')
  if ((batch.summary?.query_failed_types?.length ?? 0) > 0) parts.push(`查询失败类型：${batch.summary?.query_failed_types?.join(', ')}`)
  if ((batch.summary?.conversion_failed_types?.length ?? 0) > 0) parts.push(`转换失败类型：${batch.summary?.conversion_failed_types?.join(', ')}`)
  return parts.join('；')
}

function signalTagColor(batch: SyncBatch) {
  if (batch.summary?.product_names_empty) return 'gold'
  if (batch.summary?.max_resources_reached) return 'orange'
  return 'red'
}

function statusLabel(status: string) {
  switch (status) {
    case 'success':
      return '成功'
    case 'partial':
      return '部分完成'
    case 'failed':
      return '失败'
    case 'running':
      return '进行中'
    default:
      return status
  }
}

function syncStatusColor(status: string) {
  switch (status) {
    case 'success':
      return 'green'
    case 'partial':
      return 'orange'
    case 'failed':
      return 'red'
    default:
      return 'blue'
  }
}
</script>
