<template>
  <div class="dashboard">
    <div class="dashboard-toolbar">
      <div>
        <span class="toolbar-kicker">LIVE OPERATIONS</span>
        <span class="toolbar-hint">平台运行摘要 · 数据来自告警、执行与 Runbook 闭环</span>
      </div>
      <a-button
        :loading="loading"
        @click="loadAll"
      >
        刷新
      </a-button>
    </div>

    <MetricCards
      :summary="summary"
      :readiness="readiness"
      @navigate="go"
    />

    <a-row
      :gutter="16"
      class="row"
    >
      <a-col
        :xs="24"
        :lg="14"
      >
        <RecentExecutions
          :executions="recentExecutions"
          :loading="loadingExecutions"
          @navigate="go"
          @open-execution="onExecutionOpen"
        />
      </a-col>

      <a-col
        :xs="24"
        :lg="10"
      >
        <RunbookRecommendations
          :executions="recentExecutions"
          :loading-executions="loadingExecutions"
          :loading-recommendations="loadingRecommendations"
          :total-count="runbookTotalCount"
          :enabled-count="runbookEnabledCount"
          :recommendations="runbookRecommendations"
          @navigate="go"
          @open-execution="onExecutionOpen"
        />
      </a-col>
    </a-row>

    <ReadinessPanel
      class="row"
      :version="version"
      :readiness="readiness"
      :loading="loadingHealth"
    />
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import type { ExecutionTask } from '@/api/execution'
import { useDashboardData } from './composables/useDashboardData'

const MetricCards = defineAsyncComponent(() => import('./components/MetricCards.vue'))
const RecentExecutions = defineAsyncComponent(() => import('./components/RecentExecutions.vue'))
const RunbookRecommendations = defineAsyncComponent(() => import('./components/RunbookRecommendations.vue'))
const ReadinessPanel = defineAsyncComponent(() => import('./components/ReadinessPanel.vue'))

const router = useRouter()

const {
  loading,
  loadingExecutions,
  loadingRecommendations,
  loadingHealth,
  summary,
  recentExecutions,
  runbookTotalCount,
  runbookEnabledCount,
  runbookRecommendations,
  version,
  readiness,
  loadAll
} = useDashboardData()

function go(path: string) {
  if (!path) return
  router.push(path)
}

function onExecutionOpen(task: ExecutionTask) {
  router.push({ path: '/executions', query: { task_id: task.id } })
}

onMounted(loadAll)
</script>

<style scoped>
.dashboard {
  padding: 0;
}
.dashboard-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  padding: 18px;
  border: 1px solid rgba(22, 93, 255, 0.12);
  border-radius: 8px;
  background:
    linear-gradient(135deg, rgba(10, 26, 55, 0.92), rgba(18, 68, 114, 0.84)),
    radial-gradient(circle at 82% 20%, rgba(0, 220, 197, 0.22), transparent 28%);
  box-shadow: 0 18px 50px rgba(12, 31, 65, 0.16);
  color: #eaf7ff;
}
.toolbar-kicker {
  display: block;
  margin-bottom: 6px;
  color: #7ff7ee;
  font-size: 11px;
  font-weight: 800;
}
.toolbar-hint {
  color: rgba(234, 247, 255, 0.8);
  font-size: 14px;
}
.row {
  margin-top: 16px;
}

@media (max-width: 900px) {
  .dashboard-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
