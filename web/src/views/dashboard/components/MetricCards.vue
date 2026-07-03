<template>
  <a-row :gutter="16">
    <a-col
      v-for="(card, index) in cards"
      :key="card.key"
      :xs="12"
      :sm="8"
      :lg="4"
    >
      <a-card
        class="metric-card"
        :bordered="false"
        hoverable
        @click="emit('navigate', card.link)"
      >
        <div class="metric-head">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <span>↗</span>
        </div>
        <div class="metric-title">
          {{ card.title }}
        </div>
        <div
          class="metric-value"
          :class="card.valueClass"
        >
          {{ card.display }}
        </div>
        <div class="metric-tip">
          {{ card.tip }}
        </div>
      </a-card>
    </a-col>
  </a-row>
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

function displayCount(n: number | null) {
  return n === null ? '—' : String(n)
}

const cards = computed(() => {
  const s = props.summary
  const ready = props.readiness?.status === 'ready'
  return [
    {
      key: 'active',
      title: '活跃告警',
      display: displayCount(s?.alerts.active_total ?? null),
      tip: '未关闭的告警总数',
      link: '/alerts',
      valueClass: s?.alerts.active_total ? 'value-warn' : ''
    },
    {
      key: 'p0',
      title: 'P0 告警',
      display: displayCount(s?.alerts.p0 ?? null),
      tip: '最高优先级活跃告警',
      link: '/alerts',
      valueClass: s?.alerts.p0 ? 'value-danger' : ''
    },
    {
      key: 'p1',
      title: 'P1 告警',
      display: displayCount(s?.alerts.p1 ?? null),
      tip: '高优先级活跃告警',
      link: '/alerts',
      valueClass: s?.alerts.p1 ? 'value-warn' : ''
    },
    {
      key: 'pending',
      title: '待确认执行',
      display: displayCount(s?.executions.pending_confirm ?? null),
      tip: '等待人工 CONFIRM 的任务',
      link: '/executions',
      valueClass: s?.executions.pending_confirm ? 'value-warn' : ''
    },
    {
      key: 'assets',
      title: '注册资源',
      display: s ? `${s.assets.resources}` : '—',
      tip: s ? `${s.assets.applications} 个应用 · 点击管理资产` : '应用与资源注册总数',
      link: '/assets',
      valueClass: ''
    },
    {
      key: 'ready',
      title: '平台就绪',
      display: props.readiness ? (ready ? '就绪' : '未就绪') : '—',
      tip: props.readiness ? `运行 ${Math.round((props.readiness.uptime_ms || 0) / 1000)}s` : '探测 /readyz',
      link: '',
      valueClass: props.readiness ? (ready ? 'value-ok' : 'value-warn') : ''
    }
  ]
})
</script>

<style scoped>
.metric-card {
  cursor: pointer;
  min-height: 168px;
  position: relative;
  overflow: hidden;
  background: rgba(255, 255, 252, 0.68) !important;
}
.metric-card::before {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  height: 3px;
  pointer-events: none;
  background: #50643f;
  transform: scaleX(0.22);
  transform-origin: left;
  transition: transform 220ms ease;
}
.metric-card:hover::before {
  transform: scaleX(1);
}
.metric-head {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  color: #858a81;
  font-family: var(--aiops-display);
  font-size: 10px;
  font-style: italic;
}
.metric-card .metric-title {
  position: relative;
  color: #646960;
  font-size: 12px;
}
.metric-card .metric-value {
  position: relative;
  margin: 7px 0 8px;
  color: #20251f;
  font-family: var(--aiops-display);
  font-size: 34px;
  font-style: italic;
  font-weight: 500;
  letter-spacing: -0.04em;
}
.metric-card .metric-value.value-danger {
  color: rgb(var(--red-6));
}
.metric-card .metric-value.value-warn {
  color: rgb(var(--orange-6));
}
.metric-card .metric-value.value-ok {
  color: rgb(var(--green-6));
}
.metric-card .metric-tip {
  position: relative;
  color: #858a81;
  font-size: 11px;
  line-height: 1.5;
}
</style>
