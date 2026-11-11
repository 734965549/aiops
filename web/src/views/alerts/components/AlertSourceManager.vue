<template>
  <a-modal
    :visible="visible"
    :width="MODAL_WIDTH"
    :body-style="{ padding: '0' }"
    :footer="false"
    unmount-on-close
    @update:visible="onVisibleChange"
    @open="onOpen"
    @close="onClose"
  >
    <template #title>
      <div class="source-modal-title">
        <span>告警接入源</span>
        <span class="source-modal-subtitle">Webhook 推送与 CES 拉取</span>
      </div>
    </template>

    <div class="source-manager">
      <section class="source-pane source-list-pane">
        <header class="source-pane-head">
          <div class="source-pane-heading">
            <span class="source-pane-title">已接入源</span>
            <span class="source-count">{{ sources.length }}</span>
            <span class="source-pane-hint">点击行编辑</span>
          </div>
          <a-button
            type="primary"
            size="small"
            @click="openCreate"
          >
            新建接入源
          </a-button>
        </header>

        <div class="source-pane-body">
          <a-alert
            v-if="loadError"
            type="error"
            :title="loadError"
            closable
            class="pane-alert"
            @close="loadError = ''"
          />
          <a-alert
            v-if="syncSummary"
            :type="syncSummary.type"
            class="pane-alert"
            :title="syncSummary.title"
            closable
            @close="syncSummary = null"
          >
            <div>{{ syncSummary.summary }}</div>
            <div
              v-for="(failure, index) in syncSummary.failures"
              :key="index"
              class="sync-failure"
            >
              {{ failure.region || '-' }} / {{ failure.project_id || '-' }}：{{ failure.reason }}
            </div>
          </a-alert>
          <a-table
            class="source-table page-table-fit"
            :columns="sourceColumns"
            :data="sources"
            :loading="loading"
            row-key="id"
            :pagination="false"
            :row-class="sourceRowClass"
            @row-click="onSelectSource"
          >
            <template #empty>
              <div class="source-empty">
                <a-empty description="暂无接入源" />
                <p>Webhook 源由外部系统推送；CES 拉取源由平台主动轮询。</p>
                <a-button
                  type="primary"
                  size="small"
                  @click="openCreate"
                >
                  新建接入源
                </a-button>
              </div>
            </template>
            <template #sourceName="{ record }">
              <div class="source-name">
                <span class="source-name-main">{{ record.name }}</span>
                <span class="source-name-id">{{ record.id }}</span>
              </div>
            </template>
            <template #sourceMode="{ record }">
              <a-tag
                :color="record.delivery_mode === 'pull' ? 'orange' : 'blue'"
                size="small"
              >
                {{ modeLabel(record.delivery_mode) }}
              </a-tag>
              <div class="source-subline">{{ sourceTypeLabel(record.type) }}</div>
            </template>
            <template #enabled="{ record }">
              <a-tag
                :color="record.enabled ? 'green' : 'gray'"
                size="small"
              >
                {{ record.enabled ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #lastSync="{ record }">
              <template v-if="record.delivery_mode === 'pull'">
                <div class="source-subline">{{ formatTime(record.last_sync_at) }}</div>
                <a-tag
                  v-if="record.last_sync_status"
                  :color="syncStatusColor(record.last_sync_status)"
                  size="small"
                >
                  {{ syncStatusLabel(record.last_sync_status) }}
                </a-tag>
                <span
                  v-else
                  class="source-subline"
                >尚未同步</span>
              </template>
              <span
                v-else
                class="source-subline"
              >—</span>
            </template>
            <template #sourceActions="{ record }">
              <a-space :size="0">
                <a-button
                  type="text"
                  size="mini"
                  @click.stop="onSelectSource(record)"
                >
                  编辑
                </a-button>
                <a-button
                  v-if="record.delivery_mode === 'pull'"
                  type="text"
                  size="mini"
                  :loading="syncingId === record.id"
                  @click.stop="onSync(record)"
                >
                  立即同步
                </a-button>
                <a-popconfirm
                  content="确认删除该接入源？"
                  @ok="onDelete(record.id)"
                >
                  <a-button
                    type="text"
                    status="danger"
                    size="mini"
                    @click.stop
                  >
                    删除
                  </a-button>
                </a-popconfirm>
              </a-space>
            </template>
          </a-table>
        </div>
      </section>

      <AlertSourceForm
        :key="formKey"
        :source="editingSource"
        @saved="onSaved"
        @create="openCreate"
        @close="onVisibleChange(false)"
      />
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import * as alertApi from '@/api/alert'
import type { AlertSource, AlertSyncResult } from '@/api/alert'
import { getApiError } from '@/api/request'
import AlertSourceForm from './AlertSourceForm.vue'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
}>()

const MODAL_WIDTH = 'min(1180px, calc(100vw - 48px))'

const sources = ref<AlertSource[]>([])
const loading = ref(false)
const loadError = ref('')
const syncingId = ref('')
const editingSource = ref<AlertSource | null>(null)
const formKey = ref(0)

const syncSummary = ref<{
  type: 'success' | 'warning' | 'error'
  title: string
  summary: string
  failures: AlertSyncResult['failed_scopes']
} | null>(null)

const sourceColumns = [
  { title: '接入源', slotName: 'sourceName', width: 220 },
  { title: '接入方式', slotName: 'sourceMode', width: 150 },
  { title: '状态', slotName: 'enabled', width: 80 },
  { title: '最近同步', slotName: 'lastSync', width: 160 },
  { title: '操作', slotName: 'sourceActions', width: 190 }
]

function modeLabel(mode?: string) {
  return mode === 'pull' ? 'CES 拉取' : 'Webhook'
}

function sourceTypeLabel(type: string) {
  switch (type) {
    case 'custom_webhook':
      return '通用 Webhook'
    case 'huawei_ces':
      return '华为云 CES'
    case 'signoz':
      return 'SigNoz'
    case 'zabbix':
      return 'Zabbix'
    default:
      return 'Alertmanager'
  }
}

function syncStatusLabel(status?: string) {
  switch (status) {
    case 'success':
      return '成功'
    case 'partial':
      return '部分成功'
    case 'failed':
      return '失败'
    default:
      return status || '—'
  }
}

function syncStatusColor(status?: string) {
  switch (status) {
    case 'success':
      return 'green'
    case 'partial':
      return 'orange'
    case 'failed':
      return 'red'
    default:
      return 'gray'
  }
}

function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

function sourceRowClass(record: TableData) {
  return { 'source-row-active': record.id === editingSource.value?.id }
}

function onVisibleChange(value: boolean) {
  emit('update:visible', value)
}

function onClose() {
  editingSource.value = null
  formKey.value += 1
  syncSummary.value = null
}

async function onOpen() {
  loadError.value = ''
  syncSummary.value = null
  editingSource.value = null
  formKey.value += 1
  await loadSources()
}

async function loadSources() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await alertApi.listAlertSources()
    sources.value = res.items
  } catch (err) {
    sources.value = []
    loadError.value = getApiError(err)?.message || '加载接入源失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingSource.value = null
  formKey.value += 1
}

function onSelectSource(record: TableData) {
  const source = sources.value.find((item) => item.id === record.id)
  editingSource.value = source ?? (record as AlertSource)
  formKey.value += 1
}

async function onSaved() {
  await loadSources()
  editingSource.value = null
  formKey.value += 1
  emit('changed')
}

async function onDelete(sourceId: string) {
  try {
    await alertApi.deleteAlertSource(sourceId)
    Message.success('已删除')
    if (editingSource.value?.id === sourceId) {
      editingSource.value = null
      formKey.value += 1
    }
    await loadSources()
    emit('changed')
  } catch (err) {
    Message.error(getApiError(err)?.message || '删除接入源失败')
  }
}

async function onSync(record: TableData) {
  const source = record as AlertSource
  if (syncingId.value) return
  syncingId.value = source.id
  try {
    const result = await alertApi.syncAlertSource(source.id)
    syncSummary.value = {
      type:
        result.status === 'success'
          ? 'success'
          : result.status === 'partial'
            ? 'warning'
            : 'error',
      title: `${source.name} 同步${syncStatusLabel(result.status)}`,
      summary: `新增 ${result.created}，更新 ${result.updated}，恢复 ${result.recovered}，忽略 ${result.ignored}，失败记录 ${result.failed_records}，耗时 ${result.duration_ms} ms`,
      failures: result.failed_scopes ?? []
    }
    await loadSources()
    emit('changed')
  } catch (err) {
    const apiErr = getApiError(err)
    syncSummary.value = {
      type: 'error',
      title: `${source.name} 同步失败`,
      summary:
        apiErr?.code === 'ABORTED'
          ? '该接入源正在同步中，请稍后重试'
          : apiErr?.message || '同步失败',
      failures: []
    }
  } finally {
    syncingId.value = ''
  }
}
</script>

<style scoped lang="scss">
.source-modal-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.source-modal-subtitle {
  font-size: 12px;
  font-weight: 400;
  color: var(--aiops-text-3);
}

.source-manager {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
  height: min(72vh, 640px);
  min-height: 0;
}

.source-pane {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.source-list-pane {
  border-right: 1px solid var(--aiops-border-soft);
}

.source-pane-head {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 56px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--aiops-border-soft);
  background: rgba(235, 234, 227, 0.45);
}

.source-pane-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.source-pane-title {
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--aiops-text-1);
}

.source-pane-hint {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  color: var(--aiops-text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-count {
  flex-shrink: 0;
  min-width: 20px;
  padding: 0 6px;
  border-radius: var(--radius-full);
  background: var(--aiops-accent-soft);
  color: var(--aiops-accent-strong);
  font-size: 12px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
}

.source-pane-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 14px 16px;
}

.pane-alert {
  margin-bottom: 12px;
}

.sync-failure {
  margin-top: 4px;
  font-size: 12px;
}

.source-list-pane :deep(.arco-table-tr) {
  cursor: pointer;
}

.source-list-pane :deep(.source-row-active .arco-table-td) {
  background: var(--aiops-accent-soft) !important;
}

.source-list-pane :deep(.source-row-active .arco-table-td:first-child) {
  box-shadow: inset 3px 0 0 var(--aiops-accent);
}

.source-name {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.source-name-main {
  overflow: hidden;
  color: var(--aiops-text-1);
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-name-id {
  overflow: hidden;
  color: var(--aiops-text-3);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-subline {
  margin-top: 2px;
  color: var(--aiops-text-3);
  font-size: 12px;
}

.source-empty {
  padding: 40px 16px;
  text-align: center;
}

.source-empty p {
  margin: 8px 0 16px;
  color: var(--aiops-text-3);
  font-size: 12px;
}

@media (max-width: 860px) {
  .source-manager {
    grid-template-columns: 1fr;
    height: auto;
  }

  .source-list-pane {
    border-right: 0;
    border-bottom: 1px solid var(--aiops-border-soft);
  }

  .source-pane-body {
    max-height: 38vh;
  }
}
</style>
