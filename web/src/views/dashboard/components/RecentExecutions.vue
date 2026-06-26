<template>
  <a-card
    title="最近执行结果"
    :bordered="false"
  >
    <template #extra>
      <a-link @click="emit('navigate', '/executions')">
        查看全部
      </a-link>
    </template>
    <a-table
      :columns="columns"
      :data="executions"
      :loading="loading"
      :pagination="false"
      row-key="id"
      size="small"
      @row-click="onRowClick"
    >
      <template #status="{ record }">
        <a-tag :color="executionStatusColor(record.status)">
          {{ executionStatusLabel(record.status) }}
        </a-tag>
      </template>
      <template #result="{ record }">
        {{ record.result_summary || record.error_message || '—' }}
      </template>
      <template #time="{ record }">
        {{ formatTime(record.finished_at || record.created_at) }}
      </template>
    </a-table>
  </a-card>
</template>

<script setup lang="ts">
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { ExecutionTask } from '@/api/execution'
import { executionStatusColor, executionStatusLabel, formatTime } from '../composables/useDashboardFormat'

defineProps<{
  executions: ExecutionTask[]
  loading: boolean
}>()

const emit = defineEmits<{
  navigate: [path: string]
  'open-execution': [task: ExecutionTask]
}>()

const columns = [
  { title: '任务', dataIndex: 'name', ellipsis: true },
  { title: '状态', slotName: 'status', width: 96 },
  { title: '结果', slotName: 'result', ellipsis: true },
  { title: '时间', slotName: 'time', width: 168 }
]

function onRowClick(record: TableData) {
  emit('open-execution', record as ExecutionTask)
}
</script>

<style scoped>
:deep(.arco-table-tr) {
  cursor: pointer;
}
</style>
