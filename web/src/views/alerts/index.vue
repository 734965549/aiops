<template>
  <div class="page-shell">
    <PageHeader
      title="告警中心"
      description="集中处理告警信号、分析上下文并跟踪处置闭环。"
    >
      <template #actions>
        <a-button
          :loading="loadingList"
          @click="loadAlerts"
        >
          刷新
        </a-button>
        <a-button
          v-if="canManageSources"
          type="primary"
          @click="openSourceModal"
        >
          接入源管理
        </a-button>
      </template>
    </PageHeader>

    <a-card
      :bordered="false"
      class="page-card"
    >
      <AlertFilterForm
        v-model:keyword="filters.keyword"
        v-model:status="filters.status"
        v-model:severity="filters.severity"
        v-model:source-id="filters.source_id"
        v-model:active-only="filters.active_only"
        :source-options="sourceFilterOptions"
        @search="onSearch"
        @reset="onResetFilters"
      />

      <a-alert
        v-if="listLoadError"
        type="error"
        :title="listLoadError"
        closable
        style="margin-bottom: 12px"
        @close="listLoadError = ''"
      />

      <div class="data-panel-heading">
        <h3 class="data-panel-title">
          告警记录
          <span class="data-panel-count">共 {{ pagination.total }} 条</span>
        </h3>
        <a-tag
          v-if="filters.active_only"
          color="arcoblue"
        >
          仅活跃告警
        </a-tag>
      </div>

      <a-table
        class="page-table"
        :columns="columns"
        :data="alerts"
        :loading="loadingList"
        row-key="id"
        :pagination="pagination"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
        @row-click="onRowClick"
      >
        <template #severity="{ record }">
          <a-tag :color="severityColor(record.severity)">
            {{ severityLabel(record.severity) }}
          </a-tag>
        </template>
        <template #status="{ record }">
          <a-tag :color="statusColor(record.status)">
            {{ statusLabel(record.status) }}
          </a-tag>
        </template>
        <template #source="{ record }">
          {{ sourceLabel(record) }}
        </template>
        <template #last_seen_at="{ record }">
          {{ formatTime(record.last_seen_at) }}
        </template>
        <template #actions="{ record }">
          <a-button
            type="text"
            size="mini"
            @click.stop="openDetail(record.id)"
          >
            详情
          </a-button>
        </template>
      </a-table>
    </a-card>

    <a-drawer
      v-model:visible="detailVisible"
      :width="isNarrow ? '100%' : 720"
      :title="detail?.alert ? alertHeadline(detail.alert) : '告警详情'"
      unmount-on-close
      :footer="false"
      @cancel="closeDetail"
    >
      <!-- 请求失败态：允许 error 色；字段降级态一律 info/中性，二者不得互相回归。 -->
      <template v-if="detailError">
        <a-alert
          type="error"
          class="detail-error"
        >
          <div>{{ detailError }}</div>
          <a-button
            size="mini"
            class="detail-retry"
            @click="openDetail(selectedAlertId)"
          >
            重试
          </a-button>
        </a-alert>
      </template>
      <template v-else-if="detail">
        <AlertBasicInfo
          :alert="detail.alert"
          @check-match-rules="goMatchRules"
        />

        <AlertDetailEnhancement :alert="detail.alert" />

        <div class="action-bar">
          <a-space wrap>
            <a-button
              v-if="canAcknowledge"
              type="primary"
              :loading="actionLoading"
              @click="onAcknowledge"
            >
              认领
            </a-button>
            <a-button
              v-if="canStartProcessing"
              type="primary"
              :loading="actionLoading"
              @click="onStartProcessing"
            >
              开始处理
            </a-button>
            <a-button
              v-if="canRecover"
              :loading="actionLoading"
              @click="onRecover"
            >
              标记恢复
            </a-button>
            <a-button
              v-if="canClose"
              status="warning"
              :loading="actionLoading"
              @click="showCloseModal = true"
            >
              关闭
            </a-button>
            <a-button
              v-if="canSilence"
              :loading="actionLoading"
              @click="showSilenceModal = true"
            >
              静默
            </a-button>
            <a-button
              v-if="canUnsilence"
              :loading="actionLoading"
              @click="onUnsilence"
            >
              取消静默
            </a-button>
            <a-button
              v-if="canAssign"
              @click="showAssignModal = true"
            >
              转派
            </a-button>
            <a-button
              v-if="canComment"
              @click="showCommentModal = true"
            >
              备注
            </a-button>
            <a-button
              v-if="canAIAnalysis"
              type="outline"
              @click="openAIModal"
            >
              AI 分析
            </a-button>
            <a-button
              v-if="canAIAnalysis"
              type="outline"
              @click="askAI"
            >
              问 AI 助手
            </a-button>
            <a-button
              v-if="canCreateExecution"
              type="outline"
              :loading="executionLoading"
              @click="openRunbookModal"
            >
              推荐处置预案
            </a-button>
          </a-space>
        </div>

        <a-card
          title="时间线"
          :bordered="false"
          class="timeline-card"
        >
          <a-timeline v-if="detail.events.length">
            <a-timeline-item
              v-for="ev in detail.events"
              :key="ev.id"
            >
              <div class="event-title">
                <a-tag size="small">
                  {{ eventTypeLabel(ev.event_type) }}
                </a-tag>
                <span class="event-time">{{ formatTime(ev.created_at) }}</span>
              </div>
              <div
                v-if="ev.message"
                class="event-msg"
              >
                {{ ev.message }}
              </div>
              <div
                v-if="ev.actor_name || ev.actor_id"
                class="event-actor"
              >
                {{ ev.actor_name || ev.actor_id }}
              </div>
              <div
                v-if="executionIdFromEvent(ev)"
                class="event-link"
              >
                <a-link @click="goToExecution(executionIdFromEvent(ev)!)">
                  查看任务 {{ executionIdFromEvent(ev)!.slice(0, 8) }}…
                </a-link>
              </div>
            </a-timeline-item>
          </a-timeline>
          <a-empty
            v-else
            description="暂无事件"
          />
        </a-card>
      </template>
      <a-spin
        v-else
        :loading="loadingDetail"
        style="width: 100%; min-height: 200px"
      />
    </a-drawer>

    <AlertSourceManager
      v-model:visible="sourceModalVisible"
      @changed="loadSources"
    />

    <!-- 动作弹窗 -->
    <a-modal
      v-model:visible="showCloseModal"
      title="关闭告警"
      :ok-loading="modalLoading"
      @before-ok="beforeClose"
    >
      <a-form
        :model="closeForm"
        layout="vertical"
      >
        <a-form-item
          label="处理结论"
          required
        >
          <a-textarea
            v-model="closeForm.resolution"
            placeholder="请填写关闭原因或处理结论"
            :auto-size="{ minRows: 3, maxRows: 6 }"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="showSilenceModal"
      title="静默告警"
      :ok-loading="modalLoading"
      @before-ok="beforeSilence"
    >
      <a-form
        :model="silenceForm"
        layout="vertical"
      >
        <a-form-item
          label="静默原因"
          required
        >
          <a-input
            v-model="silenceForm.reason"
            placeholder="维护窗口 / 已知问题"
          />
        </a-form-item>
        <a-form-item
          label="静默时长（秒）"
          required
        >
          <a-input-number
            v-model="silenceForm.duration_s"
            :min="60"
            :max="2592000"
            style="width: 100%"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="showAssignModal"
      title="转派告警"
      :ok-loading="modalLoading"
      @before-ok="beforeAssign"
    >
      <a-form
        :model="assignForm"
        layout="vertical"
      >
        <a-form-item
          label="处理人用户 ID"
          required
        >
          <a-input
            v-model="assignForm.assignee_user_id"
            :placeholder="auth.user?.id || '用户 UUID'"
          />
        </a-form-item>
        <a-form-item label="转派说明">
          <a-input
            v-model="assignForm.message"
            placeholder="可选"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="showCommentModal"
      title="添加备注"
      :ok-loading="modalLoading"
      @before-ok="beforeComment"
    >
      <a-textarea
        v-model="commentForm.message"
        placeholder="备注内容"
        :auto-size="{ minRows: 3, maxRows: 6 }"
      />
    </a-modal>

    <AlertAIAnalysisModal
      v-model:visible="showAIModal"
      :alert-id="selectedAlertId"
      @analyzed="refreshDetail"
    />

    <a-modal
      v-model:visible="showExecutionModal"
      title="推荐处置预案"
      :width="720"
      :ok-loading="executionLoading"
      ok-text="创建执行任务"
      :ok-button-props="{ disabled: !runbookForm.template_id }"
      @before-ok="beforeCreateFromRunbook"
      @cancel="resetRunbookForm"
      @open="loadRunbookRecommendations"
    >
      <a-spin :loading="runbookLoading">
        <a-empty
          v-if="!runbookRecommendations.length && !runbookLoading"
          description="暂无匹配的处置预案"
        />
        <a-radio-group
          v-else
          v-model="runbookForm.template_id"
          direction="vertical"
          class="runbook-list"
        >
          <a-radio
            v-for="item in runbookRecommendations"
            :key="item.template_id"
            :value="item.template_id"
          >
            <div class="runbook-item">
              <div class="runbook-title">
                {{ item.name }}
                <a-tag size="small">
                  {{ item.risk_level }}
                </a-tag>
                <a-tag
                  v-if="item.dry_run_supported"
                  size="small"
                  color="arcoblue"
                >
                  支持 dry-run
                </a-tag>
              </div>
              <div class="runbook-meta">
                {{ item.matched_reason }} · {{ item.steps_count }} 步
              </div>
              <div
                v-if="item.description"
                class="runbook-desc"
              >
                {{ item.description }}
              </div>
            </div>
          </a-radio>
        </a-radio-group>
        <a-divider v-if="runbookRecommendations.length" />
        <a-form
          v-if="runbookRecommendations.length"
          :model="runbookForm"
          layout="vertical"
        >
          <a-form-item label="任务名称">
            <a-input
              v-model="runbookForm.name"
              placeholder="可选，默认使用预案名称"
            />
          </a-form-item>
          <a-form-item
            v-if="selectedRunbook?.dry_run_supported"
            label="Dry-run"
          >
            <a-switch v-model="runbookForm.dry_run" />
            <span class="dry-run-hint">开启后仅模拟执行，不触发真实变更</span>
          </a-form-item>
          <a-form-item label="参数 (JSON)">
            <a-textarea
              v-model="runbookForm.parameters_json"
              :auto-size="{ minRows: 2, maxRows: 6 }"
              placeholder="{&quot;service_name&quot;:&quot;payment-service&quot;,&quot;replicas&quot;:3}"
            />
          </a-form-item>
          <a-alert type="info">
            {{ runbookCreateHint }}
          </a-alert>
        </a-form>
      </a-spin>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import { useAuthStore } from '@/stores/auth'
import { useAIContextStore } from '@/stores/aiContext'
import { getApiError } from '@/api/request'
import * as alertApi from '@/api/alert'
import type { Alert, AlertDetail, AlertEvent, AlertSource } from '@/api/alert'
import * as executionApi from '@/api/execution'
import * as runbookApi from '@/api/runbook'
import type { RunbookRecommendation } from '@/api/runbook'
import PageHeader from '@/components/PageHeader.vue'
import { severityColor, severityLabel, statusColor, statusLabel, sourceLabel, formatTime } from './composables/alertDisplay'
import { alertHeadline } from './composables/alertSemantics'
import AlertBasicInfo from './components/AlertBasicInfo.vue'
import AlertFilterForm from './components/AlertFilterForm.vue'
import AlertSourceManager from './components/AlertSourceManager.vue'
import AlertDetailEnhancement from './components/AlertDetailEnhancement.vue'
import AlertAIAnalysisModal from './components/AlertAIAnalysisModal.vue'
import { columns, eventTypeLabel } from './composables/alertPresentation'
import { clearAlertAIContext, syncAlertAIContext, useAlertDeepLink } from './composables/alertAIContext'

const router = useRouter()

const auth = useAuthStore()
const aiContext = useAIContextStore()
const deepLink = useAlertDeepLink(openDetail)

const alerts = ref<Alert[]>([])
const sources = ref<AlertSource[]>([])
const detail = ref<AlertDetail | null>(null)
const selectedAlertId = ref('')
const loadingList = ref(false)
const listLoadError = ref('')
const loadingDetail = ref(false)
const loadingSources = ref(false)
const actionLoading = ref(false)
const modalLoading = ref(false)
const executionLoading = ref(false)
const runbookLoading = ref(false)
const runbookRecommendations = ref<RunbookRecommendation[]>([])
const canManageSources = ref(false)

const detailVisible = ref(false)
const detailError = ref('')
const sourceModalVisible = ref(false)
const showCloseModal = ref(false)
const showSilenceModal = ref(false)
const showAssignModal = ref(false)
const showCommentModal = ref(false)
const showAIModal = ref(false)
const showExecutionModal = ref(false)

// 窄视口抽屉改全宽，避免固定 720px 溢出；matchMedia 缺失（SSR/jsdom）时安全跳过。
const isNarrow = ref(false)
let narrowMql: MediaQueryList | null = null
function onNarrowChange(e: MediaQueryListEvent | MediaQueryList) {
  isNarrow.value = e.matches
}

const filters = reactive({
  keyword: '',
  status: '',
  severity: '',
  source_id: '',
  active_only: true
})

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showTotal: true,
  showPageSize: true
})

const closeForm = reactive({ resolution: '' })
const silenceForm = reactive({ reason: '', duration_s: 3600 })
const assignForm = reactive({ assignee_user_id: '', message: '' })
const commentForm = reactive({ message: '' })

const runbookForm = reactive({
  template_id: '',
  name: '',
  dry_run: true,
  parameters_json: '{}'
})

const sourceFilterOptions = computed(() =>
  sources.value.map((s) => ({ label: s.name, value: s.id }))
)

const currentStatus = computed(() => detail.value?.alert.status ?? '')

const canAcknowledge = computed(() => currentStatus.value === 'new')
const canStartProcessing = computed(() => currentStatus.value === 'acknowledged')
const canRecover = computed(() => currentStatus.value === 'processing')
const canClose = computed(() =>
  ['new', 'acknowledged', 'processing', 'recovered'].includes(currentStatus.value)
)
const canSilence = computed(() =>
  ['new', 'acknowledged', 'processing'].includes(currentStatus.value)
)
const canUnsilence = computed(() => currentStatus.value === 'silenced')
const canAssign = computed(() => currentStatus.value !== 'closed' && currentStatus.value !== '')
const canComment = computed(() => currentStatus.value !== 'closed' && currentStatus.value !== '')
const canAIAnalysis = computed(() => currentStatus.value !== 'closed' && currentStatus.value !== '')
const canCreateExecution = computed(() => currentStatus.value === 'processing')

const selectedRunbook = computed(() =>
  runbookRecommendations.value.find((r) => r.template_id === runbookForm.template_id)
)

const runbookCreateHint = computed(() => {
  const level = selectedRunbook.value?.risk_level || 'medium'
  if (level === 'low') {
    return '低风险任务创建后可直接执行，结果会回写告警时间线。'
  }
  return '创建后将跳转到任务详情。中高风险任务需输入 CONFIRM 确认后再执行；结果会回写告警时间线。'
})

function executionIdFromEvent(ev: AlertEvent): string | null {
  const id = ev.payload?.execution_id
  return typeof id === 'string' && id ? id : null
}

function goToExecution(taskId: string) {
  router.push({ path: '/executions', query: { task_id: taskId } })
}

async function loadAlerts() {
  loadingList.value = true
  listLoadError.value = ''
  try {
    const res = await alertApi.listAlerts({
      page: pagination.current,
      page_size: pagination.pageSize,
      keyword: filters.keyword || undefined,
      status: filters.status || undefined,
      severity: filters.severity || undefined,
      source_id: filters.source_id || undefined,
      active_only: filters.active_only
    })
    alerts.value = res.items
    pagination.total = res.total
  } catch (err) {
    alerts.value = []
    pagination.total = 0
    listLoadError.value = getApiError(err)?.message || '加载告警列表失败'
  } finally {
    loadingList.value = false
  }
}

// 直接以 GET /api/alerts/sources 的成功/403 判断 app:alerts:ingest，
// 避免依赖 POST /api/identity/authorize（其本身需要 identity.authorization:execute）。
async function loadSources(): Promise<boolean> {
  loadingSources.value = true
  try {
    const res = await alertApi.listAlertSources()
    sources.value = res.items
    canManageSources.value = true
    return true
  } catch (err) {
    sources.value = []
    const apiErr = getApiError(err)
    if (apiErr?.status === 403) {
      canManageSources.value = false
    }
    return false
  } finally {
    loadingSources.value = false
  }
}

async function openDetail(alertId: string) {
  selectedAlertId.value = alertId
  detailVisible.value = true
  detail.value = null
  detailError.value = ''
  loadingDetail.value = true
  try {
    detail.value = await alertApi.getAlert(alertId)
    syncAlertAIContext(detail.value.alert)
  } catch (err) {
    // 请求失败态允许 error 色；规则字段降级态一律 info/中性，二者不得互相回归。
    const apiErr = getApiError(err)
    detailError.value = apiErr?.status === 403
      ? '无权限查看该告警详情'
      : apiErr?.message || '加载告警详情失败'
  } finally {
    loadingDetail.value = false
  }
}

function closeDetail() {
  detailVisible.value = false
  detail.value = null
  detailError.value = ''
  selectedAlertId.value = ''
  clearAlertAIContext()
  deepLink.resetDeepLink()
}

function goMatchRules() {
  closeDetail()
  router.push({ path: '/assets', query: { tab: 'match-rules' } })
}

/** 打开全局 AI 浮层，并把当前告警作为页面上下文带入。 */
function askAI() {
  const alert = detail.value?.alert
  if (!alert) {
    Message.warning('请先加载告警详情')
    return
  }
  syncAlertAIContext(alert)
  aiContext.openOverlay()
}

function onRowClick(record: TableData) {
  openDetail((record as Alert).id)
}

function onSearch() {
  pagination.current = 1
  loadAlerts()
}

function onResetFilters() {
  filters.keyword = ''
  filters.status = ''
  filters.severity = ''
  filters.source_id = ''
  filters.active_only = true
  onSearch()
}

function onPageChange(page: number) {
  pagination.current = page
  loadAlerts()
}

function onPageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.current = 1
  loadAlerts()
}

async function refreshDetail() {
  if (!selectedAlertId.value) return
  detail.value = await alertApi.getAlert(selectedAlertId.value)
  await loadAlerts()
}

async function runAction(fn: () => Promise<unknown>, successMsg: string) {
  actionLoading.value = true
  try {
    await fn()
    Message.success(successMsg)
    await refreshDetail()
  } finally {
    actionLoading.value = false
  }
}

async function submitModalAction(fn: () => Promise<unknown>, successMsg: string): Promise<boolean> {
  modalLoading.value = true
  try {
    await fn()
    Message.success(successMsg)
    await refreshDetail()
    return true
  } catch {
    return false
  } finally {
    modalLoading.value = false
  }
}

function onAcknowledge() {
  if (!selectedAlertId.value) return
  runAction(
    () => alertApi.acknowledgeAlert(selectedAlertId.value),
    '认领成功'
  )
}

function onStartProcessing() {
  if (!selectedAlertId.value) return
  runAction(
    () => alertApi.startProcessingAlert(selectedAlertId.value),
    '已开始处理'
  )
}

function onRecover() {
  if (!selectedAlertId.value) return
  runAction(
    () => alertApi.recoverAlert(selectedAlertId.value, '手动标记恢复'),
    '已标记恢复'
  )
}

async function beforeClose(): Promise<boolean> {
  if (!selectedAlertId.value || !closeForm.resolution.trim()) {
    Message.warning('请填写处理结论')
    return false
  }
  const ok = await submitModalAction(
    () => alertApi.closeAlert(selectedAlertId.value, closeForm.resolution.trim()),
    '告警已关闭'
  )
  if (ok) closeForm.resolution = ''
  return ok
}

async function beforeSilence(): Promise<boolean> {
  if (!selectedAlertId.value || !silenceForm.reason.trim()) {
    Message.warning('请填写静默原因')
    return false
  }
  return submitModalAction(
    () =>
      alertApi.silenceAlert(
        selectedAlertId.value,
        silenceForm.reason.trim(),
        silenceForm.duration_s
      ),
    '告警已静默'
  )
}

function onUnsilence() {
  if (!selectedAlertId.value) return
  runAction(
    () => alertApi.unsilenceAlert(selectedAlertId.value),
    '已取消静默'
  )
}

async function beforeAssign(): Promise<boolean> {
  if (!selectedAlertId.value || !assignForm.assignee_user_id.trim()) {
    Message.warning('请填写处理人用户 ID')
    return false
  }
  return submitModalAction(
    () =>
      alertApi.assignAlert(
        selectedAlertId.value,
        assignForm.assignee_user_id.trim(),
        assignForm.message || undefined
      ),
    '转派成功'
  )
}

async function beforeComment(): Promise<boolean> {
  if (!selectedAlertId.value || !commentForm.message.trim()) {
    Message.warning('请填写备注内容')
    return false
  }
  const ok = await submitModalAction(
    () => alertApi.commentAlert(selectedAlertId.value, commentForm.message.trim()),
    '备注已添加'
  )
  if (ok) commentForm.message = ''
  return ok
}

function openAIModal() {
  showAIModal.value = true
}

function openRunbookModal() {
  resetRunbookForm()
  showExecutionModal.value = true
}

function resetRunbookForm() {
  runbookForm.template_id = ''
  runbookForm.name = ''
  runbookForm.dry_run = true
  runbookForm.parameters_json = '{}'
  runbookRecommendations.value = []
}

async function loadRunbookRecommendations() {
  if (!selectedAlertId.value) return
  runbookLoading.value = true
  try {
    const res = await runbookApi.listRunbookRecommendations(selectedAlertId.value)
    runbookRecommendations.value = res.items
    if (res.items.length === 1) {
      runbookForm.template_id = res.items[0].template_id
    }
  } finally {
    runbookLoading.value = false
  }
}

function parseRunbookParameters(): Record<string, unknown> | null {
  const raw = runbookForm.parameters_json.trim()
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
    Message.warning('参数必须是 JSON 对象')
    return null
  } catch {
    Message.warning('参数 JSON 格式无效')
    return null
  }
}

async function beforeCreateFromRunbook(): Promise<boolean> {
  if (!selectedAlertId.value || !runbookForm.template_id) return false
  const parameters = parseRunbookParameters()
  if (parameters === null) return false
  executionLoading.value = true
  try {
    const created = await executionApi.createExecutionTask({
      name: runbookForm.name.trim() || undefined,
      source_type: 'alert',
      source_id: selectedAlertId.value,
      runbook_template_id: runbookForm.template_id,
      dry_run: runbookForm.dry_run,
      parameters
    })
    Message.success('执行任务已创建')
    await refreshDetail()
    showExecutionModal.value = false
    router.push({ path: '/executions', query: { task_id: created.task_id } })
    return true
  } catch {
    return false
  } finally {
    executionLoading.value = false
  }
}

function openSourceModal() {
  if (!canManageSources.value) {
    Message.warning('当前账号无接入源管理权限')
    return
  }
  sourceModalVisible.value = true
}

onMounted(async () => {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    narrowMql = window.matchMedia('(max-width: 900px)')
    onNarrowChange(narrowMql)
    narrowMql.addEventListener?.('change', onNarrowChange)
  }
  assignForm.assignee_user_id = auth.user?.id || ''
  await Promise.allSettled([loadSources(), loadAlerts()])
})

onUnmounted(() => narrowMql?.removeEventListener?.('change', onNarrowChange))
</script>

<style scoped lang="scss">
.detail-desc {
  margin-bottom: 16px;
}

.action-bar {
  margin: 16px 0;
}

.timeline-card {
  margin-top: 8px;
  padding: 0;
}

.event-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.event-time {
  color: var(--color-text-3);
  font-size: 12px;
}

.event-msg {
  margin-top: 4px;
}

.event-actor {
  margin-top: 2px;
  color: var(--color-text-3);
  font-size: 12px;
}

.runbook-list {
  width: 100%;
}

.runbook-item {
  margin-left: 8px;
}

.runbook-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.runbook-meta,
.runbook-desc {
  color: var(--color-text-3);
  font-size: 12px;
  margin-top: 4px;
}

.dry-run-hint {
  margin-left: 8px;
  color: var(--color-text-3);
  font-size: 12px;
}
</style>
