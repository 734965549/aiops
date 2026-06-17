import { ref } from 'vue'
import { fetchDashboardSummary, type DashboardSummary } from '@/api/dashboard'
import { listRunbookRecommendations, type RunbookRecommendation } from '@/api/runbook'
import { fetchReadiness, fetchVersion, type ReadinessInfo, type VersionInfo } from '@/api/system'
import type { ExecutionTask } from '@/api/execution'

export interface RecommendationGroup {
  alertId: string
  alertName: string
  severity: string
  items: RunbookRecommendation[]
}

export function useDashboardData() {
  const loading = ref(false)
  const loadingExecutions = ref(false)
  const loadingRecommendations = ref(false)
  const loadingHealth = ref(false)

  const summary = ref<DashboardSummary | null>(null)
  const recentExecutions = ref<ExecutionTask[]>([])
  const runbookTotalCount = ref<number | null>(null)
  const runbookEnabledCount = ref<number | null>(null)
  const runbookRecommendations = ref<RecommendationGroup[]>([])
  const version = ref<VersionInfo | null>(null)
  const readiness = ref<ReadinessInfo | null>(null)

  async function loadRunbookRecommendations(data: DashboardSummary) {
    loadingRecommendations.value = true
    try {
      const targets = (data.processing_alerts ?? []).slice(0, 3)
      const groups: RecommendationGroup[] = []
      await Promise.all(
        targets.map(async (alert) => {
          try {
            const rec = await listRunbookRecommendations(alert.id)
            if (rec.items.length) {
              groups.push({
                alertId: alert.id,
                alertName: alert.name,
                severity: alert.severity,
                items: rec.items.slice(0, 3)
              })
            }
          } catch {
            /* 单条推荐失败不影响其它告警 */
          }
        })
      )
      runbookRecommendations.value = groups
    } catch {
      runbookRecommendations.value = []
    } finally {
      loadingRecommendations.value = false
    }
  }

  async function loadSummary() {
    loadingExecutions.value = true
    try {
      const data = await fetchDashboardSummary()
      summary.value = data
      recentExecutions.value = data.executions.recent ?? []
      runbookTotalCount.value = data.runbooks.total
      runbookEnabledCount.value = data.runbooks.enabled
      await loadRunbookRecommendations(data)
    } catch {
      summary.value = null
      recentExecutions.value = []
      runbookTotalCount.value = null
      runbookEnabledCount.value = null
      runbookRecommendations.value = []
    } finally {
      loadingExecutions.value = false
    }
  }

  async function loadHealth() {
    loadingHealth.value = true
    try {
      const [ver, ready] = await Promise.all([fetchVersion(), fetchReadiness()])
      version.value = ver
      readiness.value = ready
    } catch {
      version.value = null
      readiness.value = null
    } finally {
      loadingHealth.value = false
    }
  }

  async function loadAll() {
    loading.value = true
    try {
      await Promise.all([loadSummary(), loadHealth()])
    } finally {
      loading.value = false
    }
  }

  return {
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
  }
}
