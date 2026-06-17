<template>
  <a-card
    title="平台就绪状态 (/readyz)"
    :bordered="false"
  >
    <template #extra>
      <a-space>
        <a-tag :color="readinessColor">
          {{ readinessLabel }}
        </a-tag>
        <span
          v-if="version"
          class="version-meta"
        >
          {{ version.version.version }} · {{ version.env }}
        </span>
      </a-space>
    </template>

    <a-table
      :columns="columns"
      :data="healthRows"
      :loading="loading"
      :pagination="false"
      row-key="name"
      size="small"
    >
      <template #status="{ record }">
        <a-tag :color="checkStatusColor(record.status)">
          {{ checkStatusLabel(record.status) }}
        </a-tag>
      </template>
      <template #error="{ record }">
        {{ record.error || '—' }}
      </template>
    </a-table>
  </a-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ReadinessInfo, VersionInfo } from '@/api/system'
import { checkStatusColor, checkStatusLabel } from '../composables/useDashboardFormat'

const props = defineProps<{
  version: VersionInfo | null
  readiness: ReadinessInfo | null
  loading: boolean
}>()

const columns = [
  { title: '检查项', dataIndex: 'name', width: 120 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '说明', slotName: 'error', ellipsis: true }
]

const readinessLabel = computed(() => {
  if (!props.readiness) return '未探测'
  return props.readiness.status === 'ready' ? '就绪' : '未就绪'
})

const readinessColor = computed(() => {
  if (!props.readiness) return 'gray'
  return props.readiness.status === 'ready' ? 'green' : 'orange'
})

const healthRows = computed(() => props.readiness?.checks ?? [])
</script>

<style scoped>
.version-meta {
  color: var(--color-text-3);
  font-size: 12px;
}
</style>
