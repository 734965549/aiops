<template>
  <div class="audits-page">
    <a-card
      title="审计中心"
      :bordered="false"
    >
      <template #extra>
        <a-space>
          <a-button
            :loading="exporting"
            @click="exportCsv"
          >
            导出 CSV
          </a-button>
          <a-button
            :loading="loading"
            @click="loadAudits"
          >
            刷新
          </a-button>
        </a-space>
      </template>

      <a-form
        :model="filters"
        layout="inline"
        class="filter-form"
      >
        <a-form-item label="资源类型">
          <a-input
            v-model="filters.resource_type"
            allow-clear
            placeholder="alert / execution / resource"
            style="width: 200px"
            @press-enter="onSearch"
          />
        </a-form-item>
        <a-form-item label="资源 ID">
          <a-input
            v-model="filters.resource_id"
            allow-clear
            placeholder="业务 ID"
            style="width: 220px"
            @press-enter="onSearch"
          />
        </a-form-item>
        <a-form-item label="用户 ID">
          <a-input
            v-model="filters.user_id"
            allow-clear
            placeholder="操作者"
            style="width: 180px"
            @press-enter="onSearch"
          />
        </a-form-item>
        <a-form-item label="动作">
          <a-input
            v-model="filters.action"
            allow-clear
            placeholder="create / close / execute"
            style="width: 180px"
            @press-enter="onSearch"
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
        :data="audits"
        :loading="loading"
        row-key="id"
        :pagination="pagination"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
        @row-click="onRowClick"
      >
        <template #created_at="{ record }">
          {{ formatTime(record.created_at) }}
        </template>
        <template #user_id="{ record }">
          {{ record.user_id || '—' }}
        </template>
        <template #resource_type="{ record }">
          <a-tag>{{ record.resource_type }}</a-tag>
        </template>
        <template #resource_id="{ record }">
          <a-typography-text
            class="mono-text"
            :ellipsis="{ rows: 1 }"
          >
            {{ record.resource_id }}
          </a-typography-text>
        </template>
        <template #action="{ record }">
          <a-tag color="arcoblue">
            {{ record.action }}
          </a-tag>
        </template>
        <template #payload="{ record }">
          <a-typography-text
            class="payload-brief"
            :ellipsis="{ rows: 1 }"
          >
            {{ formatPayloadBrief(record.payload) }}
          </a-typography-text>
        </template>
      </a-table>
    </a-card>

    <a-drawer
      v-model:visible="detailVisible"
      width="min(760px, calc(100vw - 24px))"
      title="审计详情"
      unmount-on-close
      @cancel="closeDetail"
    >
      <template v-if="selectedAudit">
        <a-descriptions
          :column="2"
          bordered
          size="small"
          class="detail-desc"
        >
          <a-descriptions-item label="审计 ID">
            <span class="mono-text">{{ selectedAudit.id }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="时间">
            {{ formatTime(selectedAudit.created_at) }}
          </a-descriptions-item>
          <a-descriptions-item label="用户 ID">
            {{ selectedAudit.user_id || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="动作">
            <a-tag color="arcoblue">
              {{ selectedAudit.action }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="资源类型">
            <a-tag>{{ selectedAudit.resource_type }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="资源 ID">
            <span class="mono-text">{{ selectedAudit.resource_id }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="IP">
            {{ selectedAudit.ip || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="User Agent">
            {{ selectedAudit.user_agent || '—' }}
          </a-descriptions-item>
        </a-descriptions>

        <a-card
          title="Payload"
          :bordered="false"
          class="payload-card"
        >
          <pre class="payload-json">{{ formatPayloadJson(selectedAudit.payload) }}</pre>
        </a-card>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import * as auditApi from '@/api/audit'
import type { AuditListQuery, OperationAudit } from '@/api/audit'

const audits = ref<OperationAudit[]>([])
const selectedAudit = ref<OperationAudit | null>(null)
const loading = ref(false)
const exporting = ref(false)
const detailVisible = ref(false)

const filters = reactive({
  resource_type: '',
  resource_id: '',
  user_id: '',
  action: ''
})

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showTotal: true,
  showPageSize: true
})

const columns = [
  { title: '时间', slotName: 'created_at', width: 170 },
  { title: '用户', slotName: 'user_id', width: 150 },
  { title: '资源类型', slotName: 'resource_type', width: 120 },
  { title: '资源 ID', slotName: 'resource_id', ellipsis: true },
  { title: '动作', slotName: 'action', width: 150 },
  { title: 'IP', dataIndex: 'ip', width: 140 },
  { title: 'Payload', slotName: 'payload', ellipsis: true }
]

const queryBase = computed<AuditListQuery>(() => ({
  resource_type: filters.resource_type || undefined,
  resource_id: filters.resource_id || undefined,
  user_id: filters.user_id || undefined,
  action: filters.action || undefined
}))

function buildQuery(page = pagination.current, pageSize = pagination.pageSize): AuditListQuery {
  return {
    ...queryBase.value,
    page,
    page_size: pageSize
  }
}

function isSensitiveKey(key: string) {
  const normalized = key.toLowerCase().replace(/[-_\s]/g, '')
  if (normalized === 'ak' || normalized === 'sk') return true
  return /password|secret|token|apikey|authorization|jwt|accesskey/.test(normalized)
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Object.prototype.toString.call(value) === '[object Object]'
}

function redactPayload(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => redactPayload(item))
  }
  if (isPlainObject(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, isSensitiveKey(key) ? '******' : redactPayload(item)])
    )
  }
  return value
}

function formatPayloadBrief(payload: Record<string, unknown>) {
  const redacted = redactPayload(payload)
  const text = JSON.stringify(redacted)
  if (!text || text === '{}') return '—'
  return text.length > 100 ? `${text.slice(0, 100)}…` : text
}

function formatPayloadJson(payload: Record<string, unknown>) {
  return JSON.stringify(redactPayload(payload), null, 2)
}

function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

async function loadAudits() {
  loading.value = true
  try {
    const res = await auditApi.fetchAudits(buildQuery())
    audits.value = res.items
    pagination.total = res.total
  } finally {
    loading.value = false
  }
}

function onRowClick(record: TableData) {
  selectedAudit.value = record as OperationAudit
  detailVisible.value = true
}

function closeDetail() {
  detailVisible.value = false
  selectedAudit.value = null
}

function onSearch() {
  pagination.current = 1
  loadAudits()
}

function onResetFilters() {
  filters.resource_type = ''
  filters.resource_id = ''
  filters.user_id = ''
  filters.action = ''
  onSearch()
}

function onPageChange(page: number) {
  pagination.current = page
  loadAudits()
}

function onPageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.current = 1
  loadAudits()
}

function csvCell(value: unknown) {
  const text = value == null ? '' : String(value)
  return `"${text.replace(/"/g, '""')}"`
}

function downloadCsv(rows: OperationAudit[]) {
  const header = ['created_at', 'user_id', 'resource_type', 'resource_id', 'action', 'ip', 'payload']
  const lines = rows.map((row) =>
    [
      formatTime(row.created_at),
      row.user_id || '',
      row.resource_type,
      row.resource_id,
      row.action,
      row.ip || '',
      JSON.stringify(redactPayload(row.payload))
    ]
      .map(csvCell)
      .join(',')
  )
  const csv = [header.join(','), ...lines].join('\r\n')
  const blob = new Blob(['\uFEFF', csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `audit-export-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

async function exportCsv() {
  exporting.value = true
  try {
    const first = await auditApi.fetchAudits(buildQuery(1, 100))
    const rows = [...first.items]
    const total = first.total
    if (total === 0) {
      Message.info('当前筛选条件下没有可导出的审计记录')
      return
    }
    for (let page = 2; rows.length < total; page += 1) {
      const res = await auditApi.fetchAudits(buildQuery(page, 100))
      if (!res.items.length) break
      rows.push(...res.items)
    }
    downloadCsv(rows)
    Message.success(`已导出 ${rows.length} 条审计记录`)
  } finally {
    exporting.value = false
  }
}

onMounted(loadAudits)
</script>

<style scoped>
.audits-page {
  min-height: 100%;
}

.filter-form {
  margin-bottom: 16px;
}

.detail-desc {
  margin-bottom: 16px;
}

.payload-card {
  margin-top: 8px;
}

.payload-json {
  max-height: 420px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border-radius: 6px;
  background: var(--color-fill-2);
  color: var(--color-text-1);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}

.payload-brief,
.mono-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
</style>
