<template>
  <a-card
    title="Runbook 推荐与使用"
    :bordered="false"
  >
    <template #extra>
      <a-link @click="emit('navigate', '/runbooks')">
        预案管理
      </a-link>
    </template>
    <a-descriptions
      :column="1"
      size="small"
      bordered
      class="runbook-summary"
    >
      <a-descriptions-item label="已启用预案">
        {{ enabledCount ?? '—' }} / {{ totalCount ?? '—' }}
      </a-descriptions-item>
    </a-descriptions>

    <div class="section-label">
      最近 Runbook 执行
    </div>
    <a-table
      :columns="usageColumns"
      :data="recentRunbookExecutions"
      :loading="loadingExecutions"
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
      <template #dry_run="{ record }">
        <a-tag
          v-if="record.dry_run"
          size="small"
          color="arcoblue"
        >
          dry-run
        </a-tag>
        <span v-else>—</span>
      </template>
    </a-table>

    <div class="section-label">
      处理中告警的推荐预案
    </div>
    <a-spin :loading="loadingRecommendations">
      <a-empty
        v-if="!recommendations.length"
        description="暂无处理中告警或未匹配到预案"
      />
      <div
        v-for="group in recommendations"
        v-else
        :key="group.alertId"
        class="rec-group"
      >
        <div class="rec-alert">
          <a-link @click="emit('navigate', '/alerts')">
            {{ group.alertName }}
          </a-link>
          <a-tag
            size="small"
            :color="severityColor(group.severity)"
          >
            {{ severityLabel(group.severity) }}
          </a-tag>
        </div>
        <ul class="rec-list">
          <li
            v-for="item in group.items"
            :key="item.template_id"
          >
            {{ item.name }}
            <span class="rec-reason">{{ item.matched_reason }}</span>
          </li>
        </ul>
      </div>
    </a-spin>
  </a-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { ExecutionTask } from '@/api/execution'
import type { RecommendationGroup } from '../composables/useDashboardData'
import {
  executionStatusColor,
  executionStatusLabel,
  severityColor,
  severityLabel
} from '../composables/useDashboardFormat'

const props = defineProps<{
  executions: ExecutionTask[]
  loadingExecutions: boolean
  loadingRecommendations: boolean
  totalCount: number | null
  enabledCount: number | null
  recommendations: RecommendationGroup[]
}>()

const emit = defineEmits<{
  navigate: [path: string]
  'open-execution': [task: ExecutionTask]
}>()

const usageColumns = [
  { title: '预案', dataIndex: 'runbook_name', ellipsis: true },
  { title: '状态', slotName: 'status', width: 88 },
  { title: 'dry-run', slotName: 'dry_run', width: 80 }
]

const recentRunbookExecutions = computed(() =>
  props.executions.filter((t) => t.runbook_template_id || t.operation_type === 'runbook').slice(0, 5)
)

function onRowClick(record: TableData) {
  emit('open-execution', record as ExecutionTask)
}
</script>

<style scoped>
.section-label {
  margin: 16px 0 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-2);
}
.runbook-summary {
  margin-bottom: 4px;
}
.rec-group {
  margin-bottom: 12px;
  padding: 8px 12px;
  background: var(--color-fill-1);
  border-radius: 4px;
}
.rec-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.rec-list {
  margin: 0;
  padding-left: 18px;
  color: var(--color-text-2);
  font-size: 13px;
  line-height: 1.7;
}
.rec-reason {
  margin-left: 6px;
  color: var(--color-text-3);
  font-size: 12px;
}
:deep(.arco-table-tr) {
  cursor: pointer;
}
</style>
