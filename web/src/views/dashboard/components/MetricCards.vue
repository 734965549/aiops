<template>
  <div class="metric-grid">
    <a-card
      v-for="card in cards"
      :key="card.key"
      class="metric-card"
      :bordered="false"
      :hoverable="Boolean(card.link)"
      @click="emit('navigate', card.link)"
    >
      <div class="metric-head">
        <span class="metric-title">{{ card.title }}</span>
        <span
          v-if="card.link"
          class="metric-link"
        >↗</span>
      </div>
      <div
        class="metric-value numeric"
        :class="card.valueClass"
      >
        {{ card.display }}
      </div>
      <div class="metric-tip">{{ card.tip }}</div>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { DashboardSummary } from '@/api/dashboard'
import type { ReadinessInfo } from '@/api/system'

const props = defineProps<{
  summary: DashboardSummary | null
  readiness: ReadinessInfo | null
}>()

const emit = defineEmits<{
  navigate: [path: string]
}>()

function displayCount(value: number | null) {
  return value === null ? '—' : String(value)
}

const cards = computed(() => {
  const summary = props.summary
  const ready = props.readiness?.status === 'ready'
  return [
    {
      key: 'active',
      title: '活跃告警',
      display: displayCount(summary?.alerts.active_total ?? null),
      tip: '当前未关闭告警',
      link: '/alerts',
      valueClass: summary?.alerts.active_total ? 'value-warn' : ''
    },
    {
      key: 'p0',
      title: 'P0 告警',
      display: displayCount(summary?.alerts.p0 ?? null),
      tip: '最高优先级',
      link: '/alerts',
      valueClass: summary?.alerts.p0 ? 'value-danger' : ''
    },
    {
      key: 'p1',
      title: 'P1 告警',
      display: displayCount(summary?.alerts.p1 ?? null),
      tip: '高优先级',
      link: '/alerts',
      valueClass: summary?.alerts.p1 ? 'value-warn' : ''
    },
    {
      key: 'pending',
      title: '待确认执行',
      display: displayCount(summary?.executions.pending_confirm ?? null),
      tip: '等待人工 CONFIRM',
      link: '/executions',
      valueClass: summary?.executions.pending_confirm ? 'value-warn' : ''
    },
    {
      key: 'assets',
      title: '注册资源',
      display: summary ? String(summary.assets.resources) : '—',
      tip: summary ? `${summary.assets.applications} 个应用` : '应用与资源总数',
      link: '/assets',
      valueClass: ''
    },
    {
      key: 'ready',
      title: '平台状态',
      display: props.readiness ? (ready ? '就绪' : '未就绪') : '—',
      tip: props.readiness ? `运行 ${Math.round((props.readiness.uptime_ms || 0) / 1000)}s` : '等待 /readyz',
      link: '',
      valueClass: props.readiness ? (ready ? 'value-ok' : 'value-warn') : ''
    }
  ]
})
</script>

<style scoped>
.metric-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}

.metric-card {
  min-width: 0;
  min-height: 118px;
  border: 1px solid var(--aiops-border);
  cursor: default;
}

.metric-card:has(.metric-link) {
  cursor: pointer;
}

.metric-card :deep(.arco-card-body) {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px;
}

.metric-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--aiops-text-2);
  font-size: 12px;
}

.metric-link {
  color: var(--aiops-text-3);
  font-size: 13px;
}

.metric-value {
  margin-top: 8px;
  color: var(--aiops-text-1);
  font-size: 27px;
  font-weight: 650;
  line-height: 1.25;
}

.metric-value.value-danger {
  color: rgb(var(--red-6));
}

.metric-value.value-warn {
  color: rgb(var(--orange-6));
}

.metric-value.value-ok {
  color: rgb(var(--green-6));
}

.metric-tip {
  margin-top: auto;
  padding-top: 6px;
  overflow: hidden;
  color: var(--aiops-text-3);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1399px) {
  .metric-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
