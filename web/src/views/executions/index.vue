<template>
  <div class="executions-page">
    <a-card
      title="执行任务"
      :bordered="false"
    >
      <template #extra>
        <a-button
          :loading="loadingList"
          @click="loadTasks"
        >
          刷新
        </a-button>
      </template>

      <a-form
        :model="filters"
        layout="inline"
        class="filter-form"
      >
        <a-form-item label="关键词">
          <a-input
            v-model="filters.keyword"
            allow-clear
            placeholder="任务名 / 目标"
            style="width: 200px"
            @press-enter="onSearch"
          />
        </a-form-item>
        <a-form-item label="状态">
          <a-select
            v-model="filters.status"
            allow-clear
            placeholder="全部"
            style="width: 160px"
            :options="statusOptions"
          />
        </a-form-item>
        <a-form-item label="来源">
          <a-select
            v-model="filters.source_type"
            allow-clear
            placeholder="全部"
            style="width: 140px"
            :options="sourceTypeOptions"
          />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button
              type="primary"
              @click="onSearch"
            >
              查询
            </a-button>
            <a-button @click="onResetFilters">
              重置
            </a-button>
          </a-space>
        </a-form-item>
      </a-form>

      <a-table
        :columns="columns"
        :data="tasks"
        :loading="loadingList"
        row-key="id"
        :pagination="pagination"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
        @row-click="onRowClick"
      >
        <template #status="{ record }">
          <a-tag :color="statusColor(record.status)">
            {{ statusLabel(record.status) }}
          </a-tag>
        </template>
        <template #risk_level="{ record }">
          <a-tag>{{ record.risk_level }}</a-tag>
        </template>
        <template #source="{ record }">
          {{ sourceLabel(record) }}
        </template>
        <template #created_at="{ record }">
          {{ formatTime(record.created_at) }}
        </template>
      </a-table>
    </a-card>

    <a-drawer
      v-model:visible="detailVisible"
      :width="720"
      :title="detail?.task.name || '任务详情'"
      unmount-on-close
      @cancel="closeDetail"
    >
      <template v-if="detail">
        <a-descriptions
          :column="2"
          bordered
          size="small"
          class="detail-desc"
        >
          <a-descriptions-item label="状态">
            <a-tag :color="statusColor(detail.task.status)">
              {{ statusLabel(detail.task.status) }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="风险">
            {{ detail.task.risk_level }}
          </a-descriptions-item>
          <a-descriptions-item label="操作类型">
            {{ detail.task.operation_type }}
          </a-descriptions-item>
          <a-descriptions-item label="来源">
            {{ sourceLabel(detail.task) }}
          </a-descriptions-item>
          <a-descriptions-item label="目标">
            {{ detail.task.target_name || detail.task.target_id || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="环境">
            {{ detail.task.environment || '—' }}
          </a-descriptions-item>
          <a-descriptions-item
            v-if="detail.task.runbook_name || detail.task.runbook_template_id"
            label="Runbook"
          >
            {{ detail.task.runbook_name || detail.task.runbook_template_id }}
            <a-tag
              v-if="detail.task.dry_run"
              size="small"
              color="arcoblue"
              style="margin-left: 8px"
            >
              dry-run
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item
            v-if="detail.task.result_summary"
            label="结果"
            :span="2"
          >
            {{ detail.task.result_summary }}
          </a-descriptions-item>
          <a-descriptions-item
            v-if="detail.task.error_message"
            label="错误"
            :span="2"
          >
            {{ detail.task.error_message }}
          </a-descriptions-item>
        </a-descriptions>

        <div class="action-bar">
          <a-space>
            <a-button
              v-if="canConfirm"
              type="primary"
              :loading="actionLoading"
              @click="openConfirmModal"
            >
              确认执行
            </a-button>
            <a-button
              v-if="canExecute"
              type="primary"
              :loading="actionLoading"
              @click="onExecute"
            >
              开始执行
            </a-button>
          </a-space>
        </div>

        <a-card
          title="执行步骤"
          :bordered="false"
          class="steps-card"
        >
          <a-table
            :columns="stepColumns"
            :data="detail.steps"
            :pagination="false"
            row-key="id"
            size="small"
            :row-class="stepRowClass"
          >
            <template #status="{ record }">
              <a-tag :color="stepStatusColor(record.status)">
                {{ record.status }}
              </a-tag>
            </template>
            <template #dry_run="{ record }">
              <a-tag
                v-if="record.dry_run"
                size="small"
                color="arcoblue"
              >
                是
              </a-tag>
              <span v-else>—</span>
            </template>
            <template #parameters="{ record }">
              {{ formatJsonBrief(record.parameters) }}
            </template>
            <template #output="{ record }">
              {{ formatJsonBrief(record.output) }}
            </template>
            <template #error="{ record }">
              <span
                v-if="record.error_message"
                class="step-error"
              >{{ record.error_message }}</span>
              <span v-else>—</span>
            </template>
          </a-table>
        </a-card>
      </template>
      <a-spin
        v-else
        :loading="loadingDetail"
        style="width: 100%; min-height: 200px"
      />
    </a-drawer>

    <a-modal
      v-model:visible="confirmModalVisible"
      title="确认执行"
      :ok-loading="actionLoading"
      :ok-button-props="{ disabled: confirmTextInput !== 'CONFIRM' }"
      @before-ok="beforeConfirm"
      @cancel="closeConfirmModal"
    >
      <p class="confirm-hint">
        {{ confirmHint }}
      </p>
      <a-input
        v-model="confirmTextInput"
        placeholder="输入 CONFIRM"
        allow-clear
        @press-enter="onConfirmEnter"
      />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message, type TableData } from '@arco-design/web-vue'
import * as executionApi from '@/api/execution'
import type { ExecutionTask, ExecutionTaskDetail } from '@/api/execution'

const route = useRoute()
const router = useRouter()

const tasks = ref<ExecutionTask[]>([])
const detail = ref<ExecutionTaskDetail | null>(null)
const selectedTaskId = ref('')

const loadingList = ref(false)
const loadingDetail = ref(false)
const actionLoading = ref(false)
const detailVisible = ref(false)
const confirmModalVisible = ref(false)
const confirmTextInput = ref('')

const filters = reactive({
  keyword: '',
  status: '',
  source_type: ''
})

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showTotal: true,
  showPageSize: true
})

const statusOptions = [
  { label: '待确认', value: 'pending_confirm' },
  { label: '待执行', value: 'pending_execute' },
  { label: '执行中', value: 'running' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failed' }
]

const sourceTypeOptions = [
  { label: '告警', value: 'alert' },
  { label: '手动', value: 'manual' }
]

const columns = [
  { title: '任务名称', dataIndex: 'name', ellipsis: true },
  { title: '状态', slotName: 'status', width: 110 },
  { title: '风险', slotName: 'risk_level', width: 90 },
  { title: '操作', dataIndex: 'operation_type', width: 100 },
  { title: '来源', slotName: 'source', width: 140 },
  { title: '目标', dataIndex: 'target_name', ellipsis: true },
  { title: '创建时间', slotName: 'created_at', width: 170 }
]

const stepColumns = [
  { title: '#', dataIndex: 'step_order', width: 50 },
  { title: '步骤', dataIndex: 'name', ellipsis: true },
  { title: '风险', dataIndex: 'risk_level', width: 80 },
  { title: 'dry-run', slotName: 'dry_run', width: 80 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '参数', slotName: 'parameters', ellipsis: true },
  { title: '输出', slotName: 'output', ellipsis: true },
  { title: '错误', slotName: 'error', ellipsis: true }
]

const currentStatus = computed(() => detail.value?.task.status ?? '')
const canConfirm = computed(() => currentStatus.value === 'pending_confirm')
const canExecute = computed(() => currentStatus.value === 'pending_execute')

const confirmHint = computed(() => {
  const level = detail.value?.task.risk_level
  const label =
    level === 'critical'
      ? '严重风险'
      : level === 'high'
        ? '高风险'
        : level === 'medium'
          ? '中风险'
          : '待确认'
  return `该操作为${label}任务，请在下方输入 CONFIRM 以确认执行。`
})

function statusLabel(s: string) {
  const map: Record<string, string> = {
    pending_confirm: '待确认',
    pending_execute: '待执行',
    running: '执行中',
    success: '成功',
    failed: '失败',
    cancelled: '已取消'
  }
  return map[s] || s
}

function statusColor(s: string) {
  const map: Record<string, string> = {
    pending_confirm: 'orangered',
    pending_execute: 'orange',
    running: 'arcoblue',
    success: 'green',
    failed: 'red',
    cancelled: 'gray'
  }
  return map[s] || 'gray'
}

function stepStatusColor(s: string) {
  const map: Record<string, string> = {
    pending: 'gray',
    running: 'arcoblue',
    success: 'green',
    failed: 'red'
  }
  return map[s] || 'gray'
}

function sourceLabel(task: ExecutionTask) {
  if (task.source_type === 'alert') {
    return task.source_id ? `告警 ${task.source_id.slice(0, 8)}…` : '告警'
  }
  return task.source_type || '—'
}

function formatJsonBrief(obj?: Record<string, unknown>) {
  if (!obj || !Object.keys(obj).length) return '—'
  const text = JSON.stringify(obj)
  return text.length > 80 ? `${text.slice(0, 80)}…` : text
}

function stepRowClass(record: Record<string, unknown>) {
  return record.status === 'failed' ? 'step-row-failed' : ''
}

function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

async function loadTasks() {
  loadingList.value = true
  try {
    const res = await executionApi.listExecutionTasks({
      page: pagination.current,
      page_size: pagination.pageSize,
      keyword: filters.keyword || undefined,
      status: filters.status || undefined,
      source_type: filters.source_type || undefined
    })
    tasks.value = res.items
    pagination.total = res.total
  } finally {
    loadingList.value = false
  }
}

function onRowClick(record: TableData) {
  openDetail((record as ExecutionTask).id)
}

async function openDetail(taskId: string) {
  selectedTaskId.value = taskId
  detailVisible.value = true
  loadingDetail.value = true
  try {
    detail.value = await executionApi.getExecutionTask(taskId)
  } finally {
    loadingDetail.value = false
  }
}

function closeDetail() {
  detailVisible.value = false
  detail.value = null
  selectedTaskId.value = ''
}

function onSearch() {
  pagination.current = 1
  loadTasks()
}

function onResetFilters() {
  filters.keyword = ''
  filters.status = ''
  filters.source_type = ''
  onSearch()
}

function onPageChange(page: number) {
  pagination.current = page
  loadTasks()
}

function onPageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.current = 1
  loadTasks()
}

function openConfirmModal() {
  confirmTextInput.value = ''
  confirmModalVisible.value = true
}

function closeConfirmModal() {
  confirmModalVisible.value = false
  confirmTextInput.value = ''
}

async function beforeConfirm(): Promise<boolean> {
  if (!selectedTaskId.value) return false
  if (confirmTextInput.value !== 'CONFIRM') {
    Message.warning('请输入 CONFIRM 以确认')
    return false
  }
  actionLoading.value = true
  try {
    await executionApi.confirmExecutionTask(selectedTaskId.value, confirmTextInput.value)
    Message.success('已确认，可开始执行')
    confirmTextInput.value = ''
    detail.value = await executionApi.getExecutionTask(selectedTaskId.value)
    await loadTasks()
    return true
  } catch {
    return false
  } finally {
    actionLoading.value = false
  }
}

async function onConfirmEnter() {
  if (await beforeConfirm()) {
    confirmModalVisible.value = false
  }
}

async function onExecute() {
  if (!selectedTaskId.value) return
  actionLoading.value = true
  try {
    detail.value = await executionApi.executeTask(selectedTaskId.value)
    Message.success('执行完成')
    await loadTasks()
  } finally {
    actionLoading.value = false
  }
}

onMounted(async () => {
  await loadTasks()
  const taskId = route.query.task_id
  if (typeof taskId === 'string' && taskId) {
    await openDetail(taskId)
    router.replace({ path: route.path })
  }
})
</script>

<style scoped>
.filter-form {
  margin-bottom: 16px;
}
.detail-desc {
  margin-bottom: 16px;
}
.action-bar {
  margin-bottom: 16px;
}
.steps-card {
  margin-top: 8px;
}
.confirm-hint {
  margin-bottom: 12px;
  color: var(--color-text-2);
}
:deep(.step-row-failed) {
  background-color: rgba(var(--red-1), 0.35);
}
.step-error {
  color: rgb(var(--red-6));
}
</style>
