<template>
  <div class="page-shell page-shell--flow dashboard">
    <PageHeader
      eyebrow="OPERATIONS OVERVIEW"
      title="运维总览"
      description="聚合告警、执行、资产与平台状态，优先处理当前待办。"
    >
      <template #actions>
        <a-tag :color="readiness?.status === 'ready' ? 'green' : 'orange'">
          {{ readiness?.status === 'ready' ? '平台就绪' : '待确认就绪状态' }}
        </a-tag>
        <a-button
          :loading="loading"
          @click="loadAll"
        >
          <template #icon>
            <IconRefresh />
          </template>
          刷新数据
        </a-button>
      </template>
    </PageHeader>

    <MetricCards
      class="dashboard-metrics"
      :summary="summary"
      :readiness="readiness"
      @navigate="go"
    />

    <a-row
      :gutter="16"
      class="dashboard-row equal-row"
    >
      <a-col
        :xs="24"
        :xl="14"
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
      <a-col
        :xs="24"
        :xl="10"
      >
        <RecentExecutions
          :executions="recentExecutions"
          :loading="loadingExecutions"
          @navigate="go"
          @open-execution="onExecutionOpen"
        />
      </a-col>
    </a-row>

    <ReadinessPanel
      class="dashboard-readiness"
      :version="version"
      :readiness="readiness"
      :loading="loadingHealth"
    />
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { IconRefresh } from '@arco-design/web-vue/es/icon'
import type { ExecutionTask } from '@/api/execution'
import PageHeader from '@/components/PageHeader.vue'
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
  if (path) router.push(path)
}

function onExecutionOpen(task: ExecutionTask) {
  router.push({ path: '/executions', query: { task_id: task.id } })
}

onMounted(loadAll)
</script>

<style scoped>
.dashboard {
  padding-right: 2px;
}

.dashboard-metrics {
  flex-shrink: 0;
}

.dashboard-row,
.dashboard-readiness {
  flex-shrink: 0;
  margin-top: 16px;
}

@media (max-width: 1199px) {
  .dashboard-row > .arco-col + .arco-col {
    margin-top: 16px;
  }
}
</style>
