<template>
  <a-card
    title="处置建议与 Runbook"
    :bordered="false"
  >
    <template #extra>
      <a-link @click="emit('navigate', '/runbooks')">预案管理</a-link>
    </template>

    <div class="runbook-summary">
      <div>
        <span class="summary-label">已启用预案</span>
        <strong class="numeric">{{ enabledCount ?? '—' }} <small>/ {{ totalCount ?? '—' }}</small></strong>
      </div>
      <a-tag color="arcoblue">覆盖 {{ recommendations.length }} 条处理中告警</a-tag>
    </div>

    <section class="recommendation-section">
      <div class="section-heading">
        <h3>处理中告警的推荐预案</h3>
        <span>优先展示可执行的处置建议</span>
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
            <a-link @click="emit('navigate', '/alerts')">{{ group.alertName }}</a-link>
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
              <span>{{ item.name }}</span>
              <span class="rec-reason">{{ item.matched_reason }}</span>
            </li>
          </ul>
        </div>
      </a-spin>
    </section>

    <section class="usage-section">
      <div class="section-heading">
        <h3>最近 Runbook 执行</h3>
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
          >dry-run</a-tag>
          <span v-else>—</span>
        </template>
      </a-table>
    </section>
  </a-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { ExecutionTask } from '@/api/execution'
import type { RecommendationGroup } from '../composables/useDashboardData'
import { executionStatusColor, executionStatusLabel, severityColor, severityLabel } from '../composables/useDashboardFormat'

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
  props.executions.filter((task) => task.runbook_template_id || task.operation_type === 'runbook').slice(0, 5)
)

function onRowClick(record: TableData) {
  emit('open-execution', record as ExecutionTask)
}
</script>

<style scoped>
.runbook-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--aiops-border-soft);
  border-radius: 6px;
  background: var(--aiops-surface-muted);
}

.summary-label {
  display: block;
  color: var(--aiops-text-3);
  font-size: 11px;
}

.runbook-summary strong {
  display: block;
  margin-top: 4px;
  font-size: 22px;
  font-weight: 650;
}

.runbook-summary small {
  color: var(--aiops-text-3);
  font-size: 12px;
  font-weight: 400;
}

.recommendation-section,
.usage-section {
  margin-top: 16px;
}

.section-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.section-heading h3 {
  margin: 0;
  color: var(--aiops-text-1);
  font-size: 13px;
  font-weight: 600;
}

.section-heading span {
  color: var(--aiops-text-3);
  font-size: 11px;
}

.rec-group {
  margin-bottom: 8px;
  padding: 10px 12px;
  border: 1px solid var(--aiops-border-soft);
  border-radius: 6px;
  background: var(--aiops-surface-muted);
}

.rec-alert {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rec-list {
  margin: 8px 0 0;
  padding: 0;
  color: var(--aiops-text-2);
  font-size: 12px;
  list-style: none;
}

.rec-list li {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 4px 0;
}

.rec-reason {
  overflow: hidden;
  color: var(--aiops-text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.arco-table-tr) {
  cursor: pointer;
}
</style>
