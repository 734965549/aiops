<template>
  <div class="observability-page">
    <a-card
      title="观测查询"
      :bordered="false"
    >
      <a-form
        :model="common"
        layout="inline"
        class="filter-form"
      >
        <a-form-item
          label="接入账号"
          required
        >
          <a-select
            v-model="common.account_id"
            allow-search
            placeholder="选择账号"
            style="width: 320px"
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
        <a-form-item label="时间范围">
          <a-range-picker
            v-model="timeRange"
            show-time
            style="width: 360px"
          />
        </a-form-item>
      </a-form>

      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane
          key="metrics"
          title="指标"
        >
          <a-form
            :model="metricsForm"
            layout="inline"
            class="query-form"
          >
            <a-form-item label="指标">
              <a-input
                v-model="metricsForm.metric"
                placeholder="cpu_util"
                style="width: 160px"
              />
            </a-form-item>
            <a-form-item label="采样周期(s)">
              <a-input-number
                v-model="metricsForm.period"
                :min="10"
                :max="3600"
                style="width: 120px"
              />
            </a-form-item>
            <a-form-item>
              <a-button
                type="primary"
                :loading="metricsLoading"
                @click="runMetricsQuery"
              >
                查询
              </a-button>
            </a-form-item>
          </a-form>
          <a-alert
            type="info"
            class="hint"
          >
            单次最多 7 天窗口、1440 个采样点，period 最小 10 秒。
          </a-alert>
          <a-table
            v-if="metricPoints.length"
            :columns="metricColumns"
            :data="metricPoints"
            row-key="key"
            :pagination="false"
            size="small"
          />
          <a-empty
            v-else
            description="暂无指标结果"
          />
          <div
            v-if="metricsEvidence"
            class="evidence"
          >
            证据 ID：<span class="mono">{{ metricsEvidence }}</span>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="logs"
          title="日志"
        >
          <a-form
            :model="logsForm"
            layout="inline"
            class="query-form"
          >
            <a-form-item label="服务">
              <a-input
                v-model="logsForm.service"
                placeholder="payment-service"
                style="width: 180px"
              />
            </a-form-item>
            <a-form-item label="关键词">
              <a-input
                v-model="logsForm.keyword"
                allow-clear
                style="width: 160px"
              />
            </a-form-item>
            <a-form-item>
              <a-button
                type="primary"
                :loading="logsLoading"
                @click="runLogsQuery"
              >
                搜索
              </a-button>
            </a-form-item>
          </a-form>
          <a-table
            :columns="logColumns"
            :data="logEntries"
            :loading="logsLoading"
            row-key="ref"
            :pagination="false"
            size="small"
          />
          <div
            v-if="logsEvidence"
            class="evidence"
          >
            证据 ID：<span class="mono">{{ logsEvidence }}</span>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="traces"
          title="链路"
        >
          <a-form
            :model="tracesForm"
            layout="inline"
            class="query-form"
          >
            <a-form-item label="服务">
              <a-input
                v-model="tracesForm.service"
                placeholder="payment-service"
                style="width: 180px"
              />
            </a-form-item>
            <a-form-item label="操作">
              <a-input
                v-model="tracesForm.operation"
                placeholder="POST /pay"
                style="width: 160px"
              />
            </a-form-item>
            <a-form-item label="Trace ID">
              <a-input
                v-model="tracesForm.trace_id"
                allow-clear
                style="width: 200px"
              />
            </a-form-item>
            <a-form-item label="最小延迟(ms)">
              <a-input-number
                v-model="tracesForm.min_latency_ms"
                :min="0"
                style="width: 120px"
              />
            </a-form-item>
            <a-form-item label="仅错误">
              <a-switch v-model="tracesForm.error_only" />
            </a-form-item>
            <a-form-item>
              <a-button
                type="primary"
                :loading="tracesLoading"
                @click="runTracesQuery"
              >
                查询
              </a-button>
            </a-form-item>
          </a-form>
          <a-table
            :columns="traceColumns"
            :data="traceSpans"
            :loading="tracesLoading"
            row-key="spanKey"
            :pagination="false"
            size="small"
          />
          <a-empty
            v-if="!tracesLoading && !traceSpans.length"
            description="暂无链路结果"
          />
          <div
            v-if="tracesEvidence"
            class="evidence"
          >
            证据 ID：<span class="mono">{{ tracesEvidence }}</span>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="topology"
          title="拓扑"
        >
          <a-form
            :model="topologyForm"
            layout="inline"
          >
            <a-form-item label="应用 ID">
              <a-input
                v-model="topologyForm.application_id"
                placeholder="app-demo"
                style="width: 200px"
              />
            </a-form-item>
            <a-form-item>
              <a-button
                type="primary"
                :loading="topologyLoading"
                @click="runTopologyQuery"
              >
                查询拓扑
              </a-button>
            </a-form-item>
          </a-form>
          <a-row :gutter="16">
            <a-col :span="12">
              <a-table
                title="节点"
                :columns="nodeColumns"
                :data="topologyNodes"
                row-key="node_id"
                :pagination="false"
                size="small"
              />
            </a-col>
            <a-col :span="12">
              <a-table
                title="边"
                :columns="edgeColumns"
                :data="topologyEdges"
                row-key="edgeKey"
                :pagination="false"
                size="small"
              />
            </a-col>
          </a-row>
          <div
            v-if="topologyEvidence"
            class="evidence"
          >
            证据 ID：<span class="mono">{{ topologyEvidence }}</span>
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { listIntegrationAccounts, type IntegrationAccount } from '@/api/integration'
import {
  queryMetrics,
  queryTopology,
  queryTraces,
  searchLogs,
  type LogEntry,
  type MetricPoint,
  type TraceSpan
} from '@/api/observability'
import { getApiError } from '@/api/request'

const activeTab = ref('metrics')
const accountsLoading = ref(false)
const accountOptions = ref<IntegrationAccount[]>([])
const common = reactive({ account_id: '' })
const timeRange = ref<[Date, Date]>(defaultTimeRange())

const metricsForm = reactive({ metric: 'cpu_util', period: 60 })
const metricsLoading = ref(false)
const metricsEvidence = ref('')
const metricSeriesRaw = ref<MetricPoint[]>([])

const logsForm = reactive({ service: '', keyword: '' })
const logsLoading = ref(false)
const logsEvidence = ref('')
const logEntries = ref<LogEntry[]>([])

const tracesForm = reactive({
  service: '',
  operation: '',
  trace_id: '',
  error_only: false,
  min_latency_ms: undefined as number | undefined
})
const tracesLoading = ref(false)
const tracesEvidence = ref('')
const traceSpansRaw = ref<TraceSpan[]>([])

const topologyForm = reactive({ application_id: '' })
const topologyLoading = ref(false)
const topologyEvidence = ref('')
const topologyNodes = ref<Array<{ node_id: string; name: string; type: string; error_rate?: number; p95_ms?: number }>>([])
const topologyEdges = ref<Array<{ edgeKey: string; from: string; to: string; call_count: number; error_rate?: number }>>([])

const metricColumns = [
  { title: '时间', dataIndex: 'tsLabel' },
  { title: '值', dataIndex: 'value' }
]

const logColumns = [
  { title: '时间', dataIndex: 'tsLabel', width: 180 },
  { title: '级别', dataIndex: 'level', width: 80 },
  { title: '服务', dataIndex: 'service', width: 140 },
  { title: '消息', dataIndex: 'message', ellipsis: true }
]

const traceColumns = [
  { title: 'Trace ID', dataIndex: 'trace_id', width: 200, ellipsis: true },
  { title: 'Span ID', dataIndex: 'span_id', width: 140, ellipsis: true },
  { title: '服务', dataIndex: 'service', width: 140 },
  { title: '操作', dataIndex: 'operation', ellipsis: true },
  { title: '耗时(ms)', dataIndex: 'duration_ms', width: 100 },
  { title: '状态', dataIndex: 'status', width: 80 },
  { title: '错误', dataIndex: 'errorLabel', width: 60 }
]

const traceSpans = computed(() =>
  traceSpansRaw.value.map((s, i) => ({
    ...s,
    spanKey: `${s.trace_id}-${s.span_id}-${i}`,
    errorLabel: s.error ? '是' : '否'
  }))
)

const nodeColumns = [
  { title: '节点', dataIndex: 'node_id' },
  { title: '类型', dataIndex: 'type', width: 100 },
  { title: '错误率', dataIndex: 'error_rate', width: 90 },
  { title: 'P95(ms)', dataIndex: 'p95_ms', width: 100 }
]

const edgeColumns = [
  { title: 'From', dataIndex: 'from' },
  { title: 'To', dataIndex: 'to' },
  { title: '调用量', dataIndex: 'call_count', width: 100 }
]

const metricPoints = computed(() =>
  metricSeriesRaw.value.map((p, i) => ({
    key: `${p.ts}-${i}`,
    tsLabel: new Date(p.ts * 1000).toLocaleString(),
    value: p.value
  }))
)

function defaultTimeRange(): [Date, Date] {
  const to = new Date()
  const from = new Date(to.getTime() - 3600 * 1000)
  return [from, to]
}

function timeBounds(): { from: number; to: number } | null {
  if (!timeRange.value?.[0] || !timeRange.value?.[1]) {
    Message.warning('请选择时间范围')
    return null
  }
  return {
    from: Math.floor(timeRange.value[0].getTime() / 1000),
    to: Math.floor(timeRange.value[1].getTime() / 1000)
  }
}

async function loadAccounts() {
  accountsLoading.value = true
  try {
    const res = await listIntegrationAccounts({ page: 1, page_size: 100, enabled: true })
    accountOptions.value = res.items
    if (!common.account_id && res.items.length) {
      common.account_id = res.items[0].account_id
    }
  } catch (err) {
    Message.error(getApiError(err)?.message || '加载接入账号失败')
  } finally {
    accountsLoading.value = false
  }
}

async function runMetricsQuery() {
  if (!common.account_id) {
    Message.warning('请选择接入账号')
    return
  }
  const bounds = timeBounds()
  if (!bounds) return
  metricsLoading.value = true
  try {
    const res = await queryMetrics({
      account_id: common.account_id,
      metric: metricsForm.metric,
      from: bounds.from,
      to: bounds.to,
      period: metricsForm.period
    })
    metricSeriesRaw.value = res.series?.[0]?.points || []
    metricsEvidence.value = res.evidence_id
  } catch (err) {
    Message.error(getApiError(err)?.message || '指标查询失败')
  } finally {
    metricsLoading.value = false
  }
}

async function runLogsQuery() {
  if (!common.account_id) {
    Message.warning('请选择接入账号')
    return
  }
  const bounds = timeBounds()
  if (!bounds) return
  logsLoading.value = true
  try {
    const res = await searchLogs({
      account_id: common.account_id,
      service: logsForm.service,
      keyword: logsForm.keyword,
      from: bounds.from,
      to: bounds.to,
      limit: 100
    })
    logEntries.value = (res.entries || []).map((e) => ({
      ...e,
      tsLabel: new Date(e.timestamp * 1000).toLocaleString()
    }))
    logsEvidence.value = res.evidence_id
  } catch (err) {
    Message.error(getApiError(err)?.message || '日志搜索失败')
  } finally {
    logsLoading.value = false
  }
}

async function runTracesQuery() {
  if (!common.account_id) {
    Message.warning('请选择接入账号')
    return
  }
  const bounds = timeBounds()
  if (!bounds) return
  tracesLoading.value = true
  try {
    const res = await queryTraces({
      account_id: common.account_id,
      service: tracesForm.service || undefined,
      operation: tracesForm.operation || undefined,
      trace_id: tracesForm.trace_id || undefined,
      error_only: tracesForm.error_only || undefined,
      min_latency_ms: tracesForm.min_latency_ms,
      from: bounds.from,
      to: bounds.to,
      limit: 50
    })
    traceSpansRaw.value = res.spans || []
    tracesEvidence.value = res.evidence_id
  } catch (err) {
    Message.error(getApiError(err)?.message || '链路查询失败')
  } finally {
    tracesLoading.value = false
  }
}

async function runTopologyQuery() {
  if (!common.account_id) {
    Message.warning('请选择接入账号')
    return
  }
  const bounds = timeBounds()
  if (!bounds) return
  topologyLoading.value = true
  try {
    const res = await queryTopology({
      account_id: common.account_id,
      application_id: topologyForm.application_id,
      from: bounds.from,
      to: bounds.to
    })
    topologyNodes.value = res.topology?.nodes || []
    topologyEdges.value = (res.topology?.edges || []).map((e, i) => ({
      ...e,
      edgeKey: `${e.from}-${e.to}-${i}`
    }))
    topologyEvidence.value = res.evidence_id
  } catch (err) {
    Message.error(getApiError(err)?.message || '拓扑查询失败')
  } finally {
    topologyLoading.value = false
  }
}

onMounted(loadAccounts)
</script>

<style scoped lang="scss">
.filter-form,
.query-form {
  margin-bottom: 16px;
}

.hint {
  margin-bottom: 12px;
}

.evidence {
  margin-top: 12px;
  color: var(--color-text-3);
  font-size: 12px;
}

.mono {
  font-family: ui-monospace, monospace;
}
</style>
