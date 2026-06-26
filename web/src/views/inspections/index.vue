<template>
  <div class="inspection-page">
    <a-card
      title="智能巡检"
      :bordered="false"
    >
      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane
          key="policies"
          title="巡检策略"
        >
          <a-space
            style="margin-bottom: 12px"
          >
            <a-button
              :loading="policiesLoading"
              @click="loadPolicies"
            >
              刷新
            </a-button>
            <a-button
              type="primary"
              @click="openCreatePolicy"
            >
              新建策略
            </a-button>
          </a-space>
          <a-table
            :columns="policyColumns"
            :data="policies"
            :loading="policiesLoading"
            row-key="policy_id"
            :pagination="policyPagination"
            @page-change="onPolicyPageChange"
          >
            <template #enabled="{ record }">
              <a-tag :color="record.enabled ? 'green' : 'gray'">
                {{ record.enabled ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #checks="{ record }">
              <a-space wrap>
                <a-tag
                  v-for="c in record.checks"
                  :key="c"
                  size="small"
                >
                  {{ c }}
                </a-tag>
              </a-space>
            </template>
            <template #actions="{ record }">
              <a-space>
                <a-button
                  type="text"
                  size="small"
                  :loading="triggeringId === record.policy_id"
                  @click="onTriggerRun(record.policy_id)"
                >
                  立即巡检
                </a-button>
                <a-button
                  type="text"
                  size="small"
                  status="danger"
                  @click="onDeletePolicy(record.policy_id)"
                >
                  删除
                </a-button>
              </a-space>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane
          key="runs"
          title="巡检运行"
        >
          <a-button
            :loading="runsLoading"
            style="margin-bottom: 12px"
            @click="loadRuns"
          >
            刷新
          </a-button>
          <a-table
            :columns="runColumns"
            :data="runs"
            :loading="runsLoading"
            row-key="run_id"
            :pagination="runPagination"
            @page-change="onRunPageChange"
          >
            <template #status="{ record }">
              <a-tag :color="runStatusColor(record.status)">
                {{ record.status }}
              </a-tag>
            </template>
            <template #actions="{ record }">
              <a-button
                type="text"
                size="small"
                @click="viewRunFindings(record.run_id)"
              >
                查看发现
              </a-button>
            </template>
          </a-table>
        </a-tab-pane>

        <a-tab-pane
          key="findings"
          title="发现与建议"
        >
          <a-form
            layout="inline"
            :model="findingFilters"
            class="filter-form"
          >
            <a-form-item label="Run ID">
              <a-input
                v-model="findingFilters.run_id"
                placeholder="run-xxx"
                style="width: 220px"
              />
            </a-form-item>
            <a-form-item label="风险">
              <a-select
                v-model="findingFilters.risk_level"
                allow-clear
                placeholder="全部"
                style="width: 120px"
              >
                <a-option value="low">
                  low
                </a-option>
                <a-option value="medium">
                  medium
                </a-option>
                <a-option value="high">
                  high
                </a-option>
                <a-option value="critical">
                  critical
                </a-option>
              </a-select>
            </a-form-item>
            <a-form-item>
              <a-button
                type="primary"
                :loading="findingsLoading"
                @click="loadFindings"
              >
                查询
              </a-button>
            </a-form-item>
          </a-form>
          <a-table
            :columns="findingColumns"
            :data="findings"
            :loading="findingsLoading"
            row-key="finding_id"
            :pagination="findingPagination"
            @page-change="onFindingPageChange"
          >
            <template #risk="{ record }">
              <a-tag :color="riskColor(record.risk_level)">
                {{ record.risk_level }}
              </a-tag>
            </template>
            <template #evidence="{ record }">
              <a-space wrap>
                <a-tag
                  v-for="ev in record.evidence_refs"
                  :key="ev"
                  size="small"
                >
                  {{ ev }}
                </a-tag>
              </a-space>
            </template>
            <template #recommendations="{ record }">
              <div
                v-for="rec in record.recommendations || []"
                :key="rec.recommendation_id"
                class="rec-item"
              >
                <strong>{{ rec.title }}</strong>
                <div class="rec-meta">
                  置信度 {{ (rec.confidence * 100).toFixed(0) }}% · {{ rec.suggested_action }}
                </div>
              </div>
              <span v-if="!record.recommendations?.length">—</span>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <a-drawer
      v-model:visible="policyDrawerVisible"
      title="新建巡检策略"
      :width="520"
      @ok="submitPolicy"
    >
      <a-form
        :model="policyForm"
        layout="vertical"
      >
        <a-form-item
          label="名称"
          required
        >
          <a-input v-model="policyForm.name" />
        </a-form-item>
        <a-form-item
          label="接入账号"
          required
        >
          <a-select
            v-model="policyForm.scope.account_id"
            allow-search
            placeholder="选择账号"
            :loading="accountsLoading"
          >
            <a-option
              v-for="acc in accountOptions"
              :key="acc.account_id"
              :value="acc.account_id"
            >
              {{ acc.name }} ({{ acc.provider }})
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="环境">
          <a-input
            v-model="policyForm.scope.environment"
            placeholder="prod"
          />
        </a-form-item>
        <a-form-item label="检查项">
          <a-select
            v-model="policyForm.checks"
            multiple
            placeholder="选择检查项"
          >
            <a-option value="metrics.cpu">
              metrics.cpu
            </a-option>
            <a-option value="metrics.memory">
              metrics.memory
            </a-option>
            <a-option value="metrics.disk">
              metrics.disk
            </a-option>
            <a-option value="traces.latency">
              traces.latency
            </a-option>
            <a-option value="traces.error_rate">
              traces.error_rate
            </a-option>
            <a-option value="logs.error_burst">
              logs.error_burst
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Cron 调度">
          <a-input
            v-model="policyForm.schedule"
            placeholder="*/15 * * * *"
          />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-space>
          <a-button @click="policyDrawerVisible = false">
            取消
          </a-button>
          <a-button
            type="primary"
            :loading="policySubmitting"
            @click="submitPolicy"
          >
            创建
          </a-button>
        </a-space>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import Modal from '@arco-design/web-vue/es/modal'
import { listIntegrationAccounts, type IntegrationAccount } from '@/api/integration'
import {
  createPolicy,
  deletePolicy,
  listFindings,
  listPolicies,
  listRuns,
  triggerRun,
  type InspectionFinding,
  type InspectionPolicy,
  type InspectionRun
} from '@/api/inspection'
import { getApiError } from '@/api/request'

const activeTab = ref('policies')
const policies = ref<InspectionPolicy[]>([])
const runs = ref<InspectionRun[]>([])
const findings = ref<InspectionFinding[]>([])
const policiesLoading = ref(false)
const runsLoading = ref(false)
const findingsLoading = ref(false)
const triggeringId = ref('')
const policyDrawerVisible = ref(false)
const policySubmitting = ref(false)
const accountsLoading = ref(false)
const accountOptions = ref<IntegrationAccount[]>([])

const policyPagination = reactive({ current: 1, pageSize: 10, total: 0 })
const runPagination = reactive({ current: 1, pageSize: 10, total: 0 })
const findingPagination = reactive({ current: 1, pageSize: 10, total: 0 })
const findingFilters = reactive({ run_id: '', risk_level: '' })

const policyForm = reactive({
  name: '',
  schedule: '*/15 * * * *',
  checks: ['metrics.cpu', 'metrics.memory', 'traces.error_rate'] as string[],
  scope: { account_id: '', provider: 'huawei_cloud', environment: 'prod' }
})

const policyColumns = [
  { title: '名称', dataIndex: 'name' },
  { title: '账号', dataIndex: 'scope.account_id' },
  { title: '状态', slotName: 'enabled', width: 90 },
  { title: '检查项', slotName: 'checks' },
  { title: '操作', slotName: 'actions', width: 180 }
]

const runColumns = [
  { title: 'Run ID', dataIndex: 'run_id', width: 280 },
  { title: '策略', dataIndex: 'policy_id', width: 280 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '摘要', dataIndex: 'summary' },
  { title: '操作', slotName: 'actions', width: 120 }
]

const findingColumns = [
  { title: '风险', slotName: 'risk', width: 90 },
  { title: '摘要', dataIndex: 'summary' },
  { title: '置信度', dataIndex: 'confidence', width: 90 },
  { title: '证据', slotName: 'evidence', width: 200 },
  { title: '建议', slotName: 'recommendations' }
]

function runStatusColor(status: string) {
  if (status === 'success') return 'green'
  if (status === 'partial') return 'orange'
  if (status === 'failed') return 'red'
  return 'blue'
}

function riskColor(level: string) {
  if (level === 'critical' || level === 'high') return 'red'
  if (level === 'medium') return 'orange'
  return 'arcoblue'
}

async function loadAccounts() {
  accountsLoading.value = true
  try {
    const res = await listIntegrationAccounts({ page: 1, page_size: 100, enabled: true })
    accountOptions.value = res.items || []
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    accountsLoading.value = false
  }
}

async function loadPolicies() {
  policiesLoading.value = true
  try {
    const res = await listPolicies({ page: policyPagination.current, page_size: policyPagination.pageSize })
    policies.value = res.items || []
    policyPagination.total = res.total || 0
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    policiesLoading.value = false
  }
}

async function loadRuns() {
  runsLoading.value = true
  try {
    const res = await listRuns({ page: runPagination.current, page_size: runPagination.pageSize })
    runs.value = res.items || []
    runPagination.total = res.total || 0
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    runsLoading.value = false
  }
}

async function loadFindings() {
  findingsLoading.value = true
  try {
    const res = await listFindings({
      page: findingPagination.current,
      page_size: findingPagination.pageSize,
      run_id: findingFilters.run_id || undefined,
      risk_level: findingFilters.risk_level || undefined
    })
    findings.value = res.items || []
    findingPagination.total = res.total || 0
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    findingsLoading.value = false
  }
}

function onPolicyPageChange(page: number) {
  policyPagination.current = page
  loadPolicies()
}

function onRunPageChange(page: number) {
  runPagination.current = page
  loadRuns()
}

function onFindingPageChange(page: number) {
  findingPagination.current = page
  loadFindings()
}

function openCreatePolicy() {
  policyForm.name = ''
  policyForm.scope.account_id = accountOptions.value[0]?.account_id || ''
  policyDrawerVisible.value = true
}

async function submitPolicy() {
  if (!policyForm.name || !policyForm.scope.account_id || !policyForm.checks.length) {
    Message.warning('请填写名称、账号和检查项')
    return
  }
  policySubmitting.value = true
  try {
    await createPolicy({
      name: policyForm.name,
      schedule: policyForm.schedule,
      scope: policyForm.scope,
      checks: policyForm.checks,
      enabled: true
    })
    Message.success('策略已创建')
    policyDrawerVisible.value = false
    await loadPolicies()
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    policySubmitting.value = false
  }
}

async function onTriggerRun(policyId: string) {
  triggeringId.value = policyId
  try {
    const run = await triggerRun(policyId)
    Message.success(`巡检已触发：${run.status}`)
    activeTab.value = 'runs'
    await loadRuns()
    if (run.run_id) {
      findingFilters.run_id = run.run_id
      activeTab.value = 'findings'
      await loadFindings()
    }
  } catch (e) {
    Message.error(getApiError(e)?.message || '加载账号失败')
  } finally {
    triggeringId.value = ''
  }
}

function viewRunFindings(runId: string) {
  findingFilters.run_id = runId
  activeTab.value = 'findings'
  loadFindings()
}

function onDeletePolicy(policyId: string) {
  Modal.confirm({
    title: '删除策略',
    content: '确认删除该巡检策略？',
    onOk: async () => {
      try {
        await deletePolicy(policyId)
        Message.success('已删除')
        await loadPolicies()
      } catch (e) {
        Message.error(getApiError(e)?.message || '删除策略失败')
      }
    }
  })
}

onMounted(async () => {
  await loadAccounts()
  await loadPolicies()
  await loadRuns()
})
</script>

<style scoped>
.filter-form {
  margin-bottom: 12px;
}
.rec-item {
  margin-bottom: 8px;
}
.rec-meta {
  font-size: 12px;
  color: var(--color-text-3);
}
</style>
