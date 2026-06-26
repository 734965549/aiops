<template>
  <div class="assets-page">
    <a-tabs
      v-model:active-key="activeTab"
      class="assets-tabs"
    >
      <a-tab-pane
        key="registry"
        title="注册表"
      >
        <a-row
          :gutter="16"
          class="registry-layout"
        >
          <a-col :span="10">
            <a-card
              title="应用"
              :bordered="false"
              class="assets-card assets-card-fixed"
            >
              <template #extra>
                <a-space>
                  <a-button @click="loadApplications">
                    刷新
                  </a-button>
                  <a-button
                    type="primary"
                    @click="openCreateApplication"
                  >
                    新建应用
                  </a-button>
                </a-space>
              </template>

              <a-table
                :columns="appColumns"
                :data="applications"
                :loading="appsLoading"
                row-key="id"
                :pagination="appPagination"
                :scroll="tableScroll"
                :bordered="false"
                @page-change="onAppPageChange"
                @page-size-change="onAppPageSizeChange"
                :row-class="appRowClass"
                @row-click="onSelectApplication"
              >
                <template #environment="{ record }">
                  {{ record.environment || '—' }}
                </template>
                <template #namespace="{ record }">
                  <a-tooltip
                    v-if="record.namespace"
                    :content="record.namespace"
                  >
                    <span class="assets-text-ellipsis">{{ record.namespace }}</span>
                  </a-tooltip>
                  <span v-else>—</span>
                </template>
                <template #actions="{ record }">
                  <a-space @click.stop>
                    <a-button
                      type="text"
                      size="small"
                      @click="openEditApplication(record as Application)"
                    >
                      编辑
                    </a-button>
                    <a-popconfirm
                      content="删除应用前需先清空其下所有资源，确定删除？"
                      @ok="confirmDeleteApplication(record as Application)"
                    >
                      <a-button
                        type="text"
                        size="small"
                        status="danger"
                      >
                        删除
                      </a-button>
                    </a-popconfirm>
                  </a-space>
                </template>
              </a-table>
            </a-card>
          </a-col>

          <a-col :span="14">
            <a-card
              :title="resourceCardTitle"
              :bordered="false"
              class="assets-card assets-card-fixed"
            >
              <template #extra>
                <a-space>
                  <a-button
                    :disabled="!selectedAppId"
                    @click="loadResources"
                  >
                    刷新
                  </a-button>
                  <a-button
                    type="primary"
                    :disabled="!selectedAppId"
                    @click="openCreateResource"
                  >
                    新建资源
                  </a-button>
                </a-space>
              </template>

              <a-empty
                v-if="!selectedAppId"
                class="assets-empty"
                description="请先选择左侧应用"
              />
              <a-table
                v-else
                :columns="resourceColumns"
                :data="resources"
                :loading="resourcesLoading"
                row-key="id"
                :pagination="resourcePagination"
                :scroll="resourceTableScroll"
                :bordered="false"
                @page-change="onResourcePageChange"
                @page-size-change="onResourcePageSizeChange"
                :row-class="resourceRowClass"
              >
                <template #source="{ record }">
                  <a-tag
                    v-if="(record as Resource).source === 'cloud_sync'"
                    color="arcoblue"
                    size="small"
                  >
                    云同步
                  </a-tag>
                  <a-tag
                    v-else
                    size="small"
                  >
                    手工
                  </a-tag>
                </template>
                <template #cloud_resource_id="{ record }">
                  <a-tooltip
                    v-if="(record as Resource).cloud_resource_id"
                    :content="(record as Resource).cloud_resource_id"
                  >
                    <span class="assets-text-ellipsis">{{ (record as Resource).cloud_resource_id }}</span>
                  </a-tooltip>
                  <span v-else>—</span>
                </template>
                <template #region="{ record }">
                  <a-tooltip
                    v-if="(record as Resource).region"
                    :content="(record as Resource).region"
                  >
                    <span class="assets-text-ellipsis">{{ (record as Resource).region }}</span>
                  </a-tooltip>
                  <span v-else>—</span>
                </template>
                <template #sync_status="{ record }">
                  <a-tag
                    v-if="(record as Resource).sync_status"
                    :color="(record as Resource).sync_status === 'stale' ? 'orangered' : 'green'"
                    size="small"
                  >
                    {{ (record as Resource).sync_status }}
                  </a-tag>
                  <span v-else>—</span>
                </template>
                <template #last_synced_at="{ record }">
                  {{ formatTs((record as Resource).last_synced_at) }}
                </template>
                <template #actions="{ record }">
                  <a-space>
                    <a-button
                      type="text"
                      size="small"
                      @click="openEditResource(record as Resource)"
                    >
                      编辑
                    </a-button>
                    <a-popconfirm
                      content="删除后历史告警仍可能引用该资源 ID，确定删除？"
                      @ok="confirmDeleteResource(record as Resource)"
                    >
                      <a-button
                        type="text"
                        size="small"
                        status="danger"
                      >
                        删除
                      </a-button>
                    </a-popconfirm>
                  </a-space>
                </template>
              </a-table>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <a-tab-pane
        key="cloud-sync"
        title="云同步"
      >
        <a-card
          title="同步批次"
          :bordered="false"
          class="assets-card assets-card-fixed"
        >
          <template #extra>
            <a-space>
              <a-input
                v-model="syncAccountId"
                placeholder="接入账号 ID"
                style="width: 280px"
                allow-clear
              />
              <a-button
                type="primary"
                :loading="syncLoading"
                @click="runCloudSync"
              >
                立即同步
              </a-button>
              <a-button @click="loadSyncBatches">
                刷新
              </a-button>
            </a-space>
          </template>
          <a-table
            :columns="syncBatchColumns"
            :data="syncBatches"
            :loading="syncBatchesLoading"
            row-key="batch_id"
            :pagination="syncPagination"
            :scroll="tableScroll"
            :bordered="false"
            @page-change="onSyncPageChange"
            @page-size-change="onSyncPageSizeChange"
          >
            <template #status="{ record }">
              <a-tag :color="syncStatusColor((record as assetApi.SyncBatch).status)">
                {{ (record as assetApi.SyncBatch).status }}
              </a-tag>
            </template>
          </a-table>
        </a-card>
      </a-tab-pane>

      <a-tab-pane
        key="match-rules"
        title="匹配规则"
      >
        <a-card
          title="告警匹配规则"
          :bordered="false"
          class="assets-card assets-card-fixed"
        >
          <template #extra>
            <a-space>
              <a-button @click="loadMatchRules">
                刷新
              </a-button>
              <a-button
                type="primary"
                @click="openCreateMatchRule"
              >
                新建规则
              </a-button>
            </a-space>
          </template>
          <a-table
            :columns="ruleColumns"
            :data="matchRules"
            :loading="rulesLoading"
            row-key="id"
            :pagination="rulePagination"
            :scroll="tableScroll"
            :bordered="false"
            @page-change="onRulePageChange"
            @page-size-change="onRulePageSizeChange"
          >
            <template #enabled="{ record }">
              <a-tag :color="(record as MatchRule).enabled ? 'green' : 'gray'">
                {{ (record as MatchRule).enabled ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #target_type="{ record }">
              {{ (record as MatchRule).target_type }}
            </template>
            <template #actions="{ record }">
              <a-space>
                <a-button
                  type="text"
                  size="small"
                  @click="openEditMatchRule(record as MatchRule)"
                >
                  编辑
                </a-button>
                <a-popconfirm
                  content="确定删除该匹配规则？"
                  @ok="confirmDeleteMatchRule(record as MatchRule)"
                >
                  <a-button
                    type="text"
                    size="small"
                    status="danger"
                  >
                    删除
                  </a-button>
                </a-popconfirm>
              </a-space>
            </template>
          </a-table>
        </a-card>
      </a-tab-pane>
    </a-tabs>

    <a-modal
      v-model:visible="appModalVisible"
      :title="appModalMode === 'edit' ? '编辑应用' : '新建应用'"
      :ok-loading="appSaving"
      @ok="submitApplication"
    >
      <a-form
        :model="appForm"
        layout="vertical"
      >
        <a-form-item
          label="应用名"
          required
        >
          <a-input
            v-model="appForm.name"
            placeholder="如 payment-service"
          />
        </a-form-item>
        <a-form-item label="环境">
          <a-select
            v-model="appForm.environment"
            allow-clear
            placeholder="prod / staging / dev"
          >
            <a-option value="prod">
              prod
            </a-option>
            <a-option value="staging">
              staging
            </a-option>
            <a-option value="dev">
              dev
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="默认 Namespace">
          <a-input
            v-model="appForm.namespace"
            placeholder="K8s namespace（可选）"
          />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea
            v-model="appForm.description"
            placeholder="业务线、负责人等备注（可选）"
            :auto-size="{ minRows: 2, maxRows: 4 }"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="resourceModalVisible"
      :title="resourceModalMode === 'edit' ? '编辑资源' : '新建资源'"
      :ok-loading="resourceSaving"
      @ok="submitResource"
    >
      <a-form
        :model="resourceForm"
        layout="vertical"
      >
        <a-form-item label="所属应用">
          <a-input
            :model-value="selectedAppName"
            disabled
          />
        </a-form-item>
        <a-form-item label="资源名">
          <a-input
            v-model="resourceForm.name"
            placeholder="显示名称（可选）"
          />
        </a-form-item>
        <a-form-item label="类型">
          <a-select
            v-model="resourceForm.resource_type"
            allow-clear
            placeholder="pod / node / host / service"
          >
            <a-option value="pod">
              pod
            </a-option>
            <a-option value="node">
              node
            </a-option>
            <a-option value="host">
              host
            </a-option>
            <a-option value="service">
              service
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Namespace">
          <a-input v-model="resourceForm.namespace" />
        </a-form-item>
        <a-form-item label="Pod">
          <a-input v-model="resourceForm.pod" />
        </a-form-item>
        <a-form-item label="Node">
          <a-input v-model="resourceForm.node" />
        </a-form-item>
        <a-form-item label="Instance">
          <a-input
            v-model="resourceForm.instance"
            placeholder="Prometheus instance 等"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="ruleModalVisible"
      :title="ruleModalMode === 'edit' ? '编辑匹配规则' : '新建匹配规则'"
      :ok-loading="ruleSaving"
      @ok="submitMatchRule"
    >
      <a-form
        :model="ruleForm"
        layout="vertical"
      >
        <a-form-item
          label="规则名"
          required
        >
          <a-input
            v-model="ruleForm.name"
            placeholder="如 payment 服务匹配"
          />
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="8">
            <a-form-item label="优先级">
              <a-input-number
                v-model="ruleForm.priority"
                :min="0"
                :max="9999"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="启用">
              <a-switch v-model="ruleForm.enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="目标类型">
              <a-select v-model="ruleForm.target_type">
                <a-option value="application">
                  application
                </a-option>
                <a-option value="resource">
                  resource
                </a-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="接入源">
          <a-select v-model="ruleForm.source_type">
            <a-option value="all">
              all
            </a-option>
            <a-option value="prometheus_alertmanager">
              prometheus_alertmanager
            </a-option>
            <a-option value="huawei_ces">
              huawei_ces
            </a-option>
            <a-option value="signoz">
              signoz
            </a-option>
          </a-select>
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="10">
            <a-form-item
              label="Label Key"
              required
            >
              <a-input
                v-model="ruleForm.label_key"
                placeholder="service"
              />
            </a-form-item>
          </a-col>
          <a-col :span="14">
            <a-form-item
              label="Label 匹配模式"
              required
            >
              <a-input
                v-model="ruleForm.label_value_pattern"
                placeholder="payment-*"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item
          label="绑定应用"
          required
        >
          <a-select
            v-model="ruleForm.application_id"
            allow-search
            placeholder="选择应用"
            @change="onRuleAppChange"
          >
            <a-option
              v-for="app in ruleApplicationOptions"
              :key="app.id"
              :value="app.id"
            >
              {{ app.name }} ({{ app.environment || 'default' }})
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item
          v-if="ruleForm.target_type === 'resource'"
          label="绑定资源"
          required
        >
          <a-select
            v-model="ruleForm.resource_id"
            allow-search
            placeholder="选择资源"
          >
            <a-option
              v-for="res in ruleResourceOptions"
              :key="res.id"
              :value="res.id"
            >
              {{ res.name || res.pod || res.id }}
            </a-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import * as assetApi from '@/api/asset'
import type { Application, MatchRule, Resource } from '@/api/asset'

const route = useRoute()
const router = useRouter()

const activeTab = ref('registry')
const applications = ref<Application[]>([])
const resources = ref<Resource[]>([])
const matchRules = ref<MatchRule[]>([])
const ruleApplicationOptions = ref<Application[]>([])
const ruleResourceOptions = ref<Resource[]>([])
const selectedAppId = ref('')
const highlightedResourceId = ref('')
const appsLoading = ref(false)
const resourcesLoading = ref(false)
const appModalVisible = ref(false)
const resourceModalVisible = ref(false)
const appModalMode = ref<'create' | 'edit'>('create')
const resourceModalMode = ref<'create' | 'edit'>('create')
const editingAppId = ref('')
const editingResourceId = ref('')
const appSaving = ref(false)
const resourceSaving = ref(false)
const rulesLoading = ref(false)
const ruleModalVisible = ref(false)
const ruleModalMode = ref<'create' | 'edit'>('create')
const editingRuleId = ref('')
const ruleSaving = ref(false)
const syncAccountId = ref('')
const syncLoading = ref(false)
const syncBatchesLoading = ref(false)
const syncBatches = ref<assetApi.SyncBatch[]>([])

const tableScroll = { y: 'calc(100vh - 330px)' }
const resourceTableScroll = { x: 1250, y: 'calc(100vh - 330px)' }

const appPagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })
const resourcePagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })
const syncPagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })
const rulePagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })

const ruleForm = reactive({
  name: '',
  enabled: true,
  priority: 100,
  target_type: 'application',
  source_type: 'all',
  label_key: 'service',
  label_value_pattern: '',
  application_id: '',
  resource_id: ''
})

const appForm = reactive({
  name: '',
  environment: 'prod',
  namespace: '',
  description: ''
})

const resourceForm = reactive({
  name: '',
  resource_type: 'pod',
  namespace: '',
  pod: '',
  node: '',
  instance: ''
})

const appColumns: TableInstance['columns'] = [
  { title: '应用名', dataIndex: 'name', ellipsis: true, tooltip: true, width: 120 },
  { title: '环境', slotName: 'environment', width: 80 },
  { title: 'Namespace', slotName: 'namespace', width: 110, ellipsis: true },
  { title: '描述', dataIndex: 'description', ellipsis: true, tooltip: true },
  { title: '操作', slotName: 'actions', width: 100 }
]

const resourceColumns: TableInstance['columns'] = [
  { title: '资源名', dataIndex: 'name', ellipsis: true, tooltip: true, width: 140 },
  { title: '来源', slotName: 'source', width: 80 },
  { title: '类型', dataIndex: 'resource_type', width: 80 },
  { title: '云资源 ID', slotName: 'cloud_resource_id', width: 150, ellipsis: true },
  { title: 'Region', slotName: 'region', width: 100 },
  { title: '同步状态', slotName: 'sync_status', width: 90 },
  { title: '最近同步', slotName: 'last_synced_at', width: 150 },
  { title: 'Namespace', dataIndex: 'namespace', width: 110, ellipsis: true, tooltip: true },
  { title: 'Pod', dataIndex: 'pod', width: 110, ellipsis: true, tooltip: true },
  { title: 'Instance', dataIndex: 'instance', width: 120, ellipsis: true, tooltip: true },
  { title: '操作', slotName: 'actions', width: 120, fixed: 'right' }
]

const syncBatchColumns: TableInstance['columns'] = [
  { title: '批次 ID', dataIndex: 'batch_id', ellipsis: true, tooltip: true },
  { title: '账号', dataIndex: 'integration_account_id', width: 220, ellipsis: true, tooltip: true },
  { title: '状态', slotName: 'status', width: 90 },
  { title: '新建', dataIndex: 'created_count', width: 70 },
  { title: '更新', dataIndex: 'updated_count', width: 70 },
  { title: 'Stale', dataIndex: 'stale_count', width: 70 },
  { title: '失败', dataIndex: 'failed_count', width: 70 },
  { title: '摘要', dataIndex: 'message', ellipsis: true, tooltip: true }
]

const ruleColumns: TableInstance['columns'] = [
  { title: '规则名', dataIndex: 'name', ellipsis: true, tooltip: true },
  { title: '优先级', dataIndex: 'priority', width: 80 },
  { title: '状态', slotName: 'enabled', width: 80 },
  { title: 'Label', dataIndex: 'label_key', width: 100, ellipsis: true, tooltip: true },
  { title: '模式', dataIndex: 'label_value_pattern', width: 140, ellipsis: true, tooltip: true },
  { title: '目标', slotName: 'target_type', width: 100 },
  { title: '接入源', dataIndex: 'source_type', width: 160, ellipsis: true, tooltip: true },
  { title: '应用 ID', dataIndex: 'application_id', width: 120, ellipsis: true, tooltip: true },
  { title: '操作', slotName: 'actions', width: 120 }
]

const selectedApplication = computed(() => {
  return applications.value.find((a) => a.id === selectedAppId.value)
    || ruleApplicationOptions.value.find((a) => a.id === selectedAppId.value)
})

const selectedAppName = computed(() => {
  return selectedApplication.value?.name || selectedAppId.value
})

const resourceCardTitle = computed(() => {
  if (!selectedAppId.value) return '资源'
  return `资源 · ${selectedAppName.value}`
})

function appRowClass(record: TableData) {
  const app = record as Application
  return app.id === selectedAppId.value ? 'assets-row-selected' : ''
}

function resourceRowClass(record: TableData) {
  const res = record as Resource
  return res.id === highlightedResourceId.value ? 'assets-row-highlight' : ''
}

function routeResourceId(): string {
  return typeof route.query.resource_id === 'string' ? route.query.resource_id : ''
}

async function scrollToHighlightedResource() {
  if (!highlightedResourceId.value) return
  await nextTick()
  const row = document.querySelector('.assets-row-highlight')
  row?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
}

function formatTs(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

function syncStatusColor(status: string) {
  switch (status) {
    case 'success':
      return 'green'
    case 'partial':
      return 'orange'
    case 'failed':
      return 'red'
    default:
      return 'blue'
  }
}

async function loadSyncBatches() {
  syncBatchesLoading.value = true
  try {
    const res = await assetApi.listSyncBatches({
      account_id: syncAccountId.value.trim() || undefined,
      page: syncPagination.current,
      page_size: syncPagination.pageSize
    })
    syncBatches.value = res.items ?? []
    syncPagination.total = res.total ?? 0
  } finally {
    syncBatchesLoading.value = false
  }
}

async function runCloudSync() {
  const accountId = syncAccountId.value.trim()
  if (!accountId) {
    Message.warning('请填写接入账号 ID')
    return
  }
  syncLoading.value = true
  try {
    const batch = await assetApi.triggerAssetSync(accountId)
    Message.success(
      `同步完成：新建 ${batch.created_count}，更新 ${batch.updated_count}，stale ${batch.stale_count}`
    )
    await loadSyncBatches()
    await loadApplications()
    if (batch.application_id) {
      selectedAppId.value = batch.application_id
      resourcePagination.current = 1
      await loadResources()
    }
  } catch (e: unknown) {
    Message.error(e instanceof Error ? e.message : '同步失败')
  } finally {
    syncLoading.value = false
  }
}

async function loadMatchRules() {
  rulesLoading.value = true
  try {
    const res = await assetApi.listMatchRules({
      page: rulePagination.current,
      page_size: rulePagination.pageSize
    })
    matchRules.value = res.items ?? []
    rulePagination.total = res.total ?? 0
  } finally {
    rulesLoading.value = false
  }
}

async function loadAllPages<T>(loader: (page: number, pageSize: number) => Promise<{ items?: T[]; total?: number }>, pageSize = 100) {
  const items: T[] = []
  let page = 1
  let total = 0
  do {
    const res = await loader(page, pageSize)
    const pageItems = res.items ?? []
    items.push(...pageItems)
    total = res.total ?? items.length
    if (pageItems.length === 0) break
    page += 1
  } while (items.length < total)
  return items
}

async function loadRuleApplications() {
  ruleApplicationOptions.value = await loadAllPages<Application>((page, pageSize) => assetApi.listApplications({
    page,
    page_size: pageSize
  }))
}

async function loadRuleResources(appId: string) {
  if (!appId) {
    ruleResourceOptions.value = []
    return
  }
  ruleResourceOptions.value = await loadAllPages<Resource>((page, pageSize) => assetApi.listResources(appId, {
    page,
    page_size: pageSize
  }))
}

async function openCreateMatchRule() {
  ruleModalMode.value = 'create'
  editingRuleId.value = ''
  ruleForm.name = ''
  ruleForm.enabled = true
  ruleForm.priority = 100
  ruleForm.target_type = 'application'
  ruleForm.source_type = 'all'
  ruleForm.label_key = 'service'
  ruleForm.label_value_pattern = ''
  ruleForm.application_id = selectedAppId.value || ''
  ruleForm.resource_id = ''
  await loadRuleApplications()
  void loadRuleResources(ruleForm.application_id)
  ruleModalVisible.value = true
}

async function openEditMatchRule(rule: MatchRule) {
  ruleModalMode.value = 'edit'
  editingRuleId.value = rule.id
  ruleForm.name = rule.name
  ruleForm.enabled = rule.enabled
  ruleForm.priority = rule.priority
  ruleForm.target_type = rule.target_type
  ruleForm.source_type = rule.source_type
  ruleForm.label_key = rule.label_key
  ruleForm.label_value_pattern = rule.label_value_pattern
  ruleForm.application_id = rule.application_id
  ruleForm.resource_id = rule.resource_id || ''
  await loadRuleApplications()
  void loadRuleResources(rule.application_id)
  ruleModalVisible.value = true
}

async function onRuleAppChange(value: string | number | boolean | Record<string, unknown> | (string | number | boolean | Record<string, unknown>)[]) {
  if (typeof value !== 'string' || !value) {
    return
  }
  ruleForm.resource_id = ''
  await loadRuleResources(value)
}

async function submitMatchRule() {
  if (!ruleForm.name.trim() || !ruleForm.label_key.trim() || !ruleForm.label_value_pattern.trim() || !ruleForm.application_id) {
    Message.warning('请填写规则名、Label、匹配模式并选择应用')
    return
  }
  if (ruleForm.target_type === 'resource' && !ruleForm.resource_id) {
    Message.warning('目标类型为 resource 时必须选择资源')
    return
  }
  ruleSaving.value = true
  try {
    const payload = {
      name: ruleForm.name.trim(),
      enabled: ruleForm.enabled,
      priority: ruleForm.priority,
      target_type: ruleForm.target_type,
      source_type: ruleForm.source_type,
      label_key: ruleForm.label_key.trim(),
      label_value_pattern: ruleForm.label_value_pattern.trim(),
      application_id: ruleForm.application_id,
      resource_id: ruleForm.target_type === 'resource' ? ruleForm.resource_id : undefined
    }
    if (ruleModalMode.value === 'edit' && editingRuleId.value) {
      await assetApi.updateMatchRule(editingRuleId.value, payload)
      Message.success('规则已更新')
    } else {
      await assetApi.createMatchRule(payload)
      Message.success('规则已创建')
    }
    ruleModalVisible.value = false
    rulePagination.current = 1
    await loadMatchRules()
  } finally {
    ruleSaving.value = false
  }
}

async function confirmDeleteMatchRule(rule: MatchRule) {
  try {
    await assetApi.deleteMatchRule(rule.id)
    Message.success('规则已删除')
    await loadMatchRules()
  } catch (e: unknown) {
    Message.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function loadApplications() {
  appsLoading.value = true
  try {
    const res = await assetApi.listApplications({
      page: appPagination.current,
      page_size: appPagination.pageSize
    })
    applications.value = res.items ?? []
    appPagination.total = res.total ?? 0
  } finally {
    appsLoading.value = false
  }
}

async function loadResources() {
  if (!selectedAppId.value) return
  resourcesLoading.value = true
  try {
    const res = await assetApi.listResources(selectedAppId.value, {
      page: resourcePagination.current,
      page_size: resourcePagination.pageSize
    })
    resources.value = res.items ?? []
    resourcePagination.total = res.total ?? 0
    const resourceId = routeResourceId()
    if (resourceId && resources.value.some((r) => r.id === resourceId)) {
      highlightedResourceId.value = resourceId
      await scrollToHighlightedResource()
    } else {
      highlightedResourceId.value = ''
    }
  } finally {
    resourcesLoading.value = false
  }
}

function onSelectApplication(record: TableData) {
  const app = record as Application
  if (selectedAppId.value === app.id) return
  selectedAppId.value = app.id
  resourcePagination.current = 1
  router.replace({ query: { ...route.query, application_id: app.id } })
  loadResources()
}

function openCreateApplication() {
  appModalMode.value = 'create'
  editingAppId.value = ''
  appForm.name = ''
  appForm.environment = 'prod'
  appForm.namespace = ''
  appForm.description = ''
  appModalVisible.value = true
}

function openEditApplication(app: Application) {
  appModalMode.value = 'edit'
  editingAppId.value = app.id
  appForm.name = app.name
  appForm.environment = app.environment || ''
  appForm.namespace = app.namespace || ''
  appForm.description = app.description || ''
  appModalVisible.value = true
}

function openCreateResource() {
  resourceModalMode.value = 'create'
  editingResourceId.value = ''
  resourceForm.name = ''
  resourceForm.resource_type = 'pod'
  resourceForm.namespace = selectedApplication.value?.namespace || ''
  resourceForm.pod = ''
  resourceForm.node = ''
  resourceForm.instance = ''
  resourceModalVisible.value = true
}

function openEditResource(res: Resource) {
  resourceModalMode.value = 'edit'
  editingResourceId.value = res.id
  resourceForm.name = res.name || ''
  resourceForm.resource_type = res.resource_type || 'pod'
  resourceForm.namespace = res.namespace || ''
  resourceForm.pod = res.pod || ''
  resourceForm.node = res.node || ''
  resourceForm.instance = res.instance || ''
  resourceModalVisible.value = true
}

async function submitApplication() {
  if (!appForm.name.trim()) {
    Message.warning('请填写应用名')
    return
  }
  appSaving.value = true
  try {
    if (appModalMode.value === 'edit' && editingAppId.value) {
      await assetApi.updateApplication(editingAppId.value, {
        name: appForm.name.trim(),
        environment: appForm.environment || undefined,
        namespace: appForm.namespace.trim() || undefined,
        description: appForm.description.trim() || undefined
      })
      Message.success('应用已更新')
    } else {
      const created = await assetApi.createApplication({
        name: appForm.name.trim(),
        environment: appForm.environment || undefined,
        namespace: appForm.namespace.trim() || undefined,
        description: appForm.description.trim() || undefined
      })
      Message.success('应用已创建')
      selectedAppId.value = created.id
      resourcePagination.current = 1
      router.replace({ query: { ...route.query, application_id: created.id } })
      await loadResources()
    }
    appModalVisible.value = false
    await loadApplications()
  } finally {
    appSaving.value = false
  }
}

async function confirmDeleteApplication(app: Application) {
  try {
    await assetApi.deleteApplication(app.id)
    Message.success('应用已删除')
    if (selectedAppId.value === app.id) {
      selectedAppId.value = ''
      resources.value = []
      const query = { ...route.query }
      delete query.application_id
      delete query.resource_id
      router.replace({ query })
    }
    await loadApplications()
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : '删除失败'
    Message.error(msg.includes('resource') ? '请先删除该应用下的所有资源' : msg)
  }
}

async function submitResource() {
  if (!selectedAppId.value) return
  resourceSaving.value = true
  try {
    if (resourceModalMode.value === 'edit' && editingResourceId.value) {
      await assetApi.updateResource(editingResourceId.value, {
        name: resourceForm.name.trim() || undefined,
        resource_type: resourceForm.resource_type || undefined,
        namespace: resourceForm.namespace.trim() || undefined,
        pod: resourceForm.pod.trim() || undefined,
        node: resourceForm.node.trim() || undefined,
        instance: resourceForm.instance.trim() || undefined
      })
      Message.success('资源已更新')
    } else {
      await assetApi.createResource({
        application_id: selectedAppId.value,
        name: resourceForm.name.trim() || undefined,
        resource_type: resourceForm.resource_type || undefined,
        namespace: resourceForm.namespace.trim() || undefined,
        pod: resourceForm.pod.trim() || undefined,
        node: resourceForm.node.trim() || undefined,
        instance: resourceForm.instance.trim() || undefined
      })
      Message.success('资源已创建')
    }
    resourceModalVisible.value = false
    resourcePagination.current = resourceModalMode.value === 'create' ? 1 : resourcePagination.current
    await loadResources()
  } finally {
    resourceSaving.value = false
  }
}

async function confirmDeleteResource(res: Resource) {
  try {
    await assetApi.deleteResource(res.id)
    Message.success('资源已删除')
    if (highlightedResourceId.value === res.id) {
      highlightedResourceId.value = ''
      const query = { ...route.query }
      delete query.resource_id
      router.replace({ query })
    }
    await loadResources()
  } catch (e: unknown) {
    Message.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function applyRouteSelection() {
  const q = typeof route.query.application_id === 'string' ? route.query.application_id : ''
  const resourceId = routeResourceId()
  if (!q && selectedAppId.value) {
    selectedAppId.value = ''
    highlightedResourceId.value = ''
    resources.value = []
    resourcePagination.total = 0
    return
  }
  if (q && q !== selectedAppId.value) {
    selectedAppId.value = q
    resourcePagination.current = 1
    await loadResources()
    return
  }
  if (resourceId && resourceId !== highlightedResourceId.value && selectedAppId.value) {
    await loadResources()
  }
}

function onAppPageChange(page: number) {
  appPagination.current = page
  loadApplications()
}

function onAppPageSizeChange(size: number) {
  appPagination.pageSize = size
  appPagination.current = 1
  loadApplications()
}

function onResourcePageChange(page: number) {
  resourcePagination.current = page
  loadResources()
}

function onResourcePageSizeChange(size: number) {
  resourcePagination.pageSize = size
  resourcePagination.current = 1
  loadResources()
}

function onSyncPageChange(page: number) {
  syncPagination.current = page
  loadSyncBatches()
}

function onSyncPageSizeChange(size: number) {
  syncPagination.pageSize = size
  syncPagination.current = 1
  loadSyncBatches()
}

function onRulePageChange(page: number) {
  rulePagination.current = page
  loadMatchRules()
}

function onRulePageSizeChange(size: number) {
  rulePagination.pageSize = size
  rulePagination.current = 1
  loadMatchRules()
}

watch(
  () => [route.query.application_id, route.query.resource_id],
  () => {
    void applyRouteSelection()
  }
)

onMounted(async () => {
  await loadApplications()
  await loadMatchRules()
  await loadSyncBatches()
  const q = typeof route.query.application_id === 'string' ? route.query.application_id : ''
  if (q) {
    selectedAppId.value = q
    resourcePagination.current = 1
    await loadResources()
  } else if (applications.value.length === 1) {
    selectedAppId.value = applications.value[0].id
    resourcePagination.current = 1
    await loadResources()
  }
})
</script>

<style scoped>
.assets-page {
  height: calc(100vh - 144px);
  min-height: 560px;
  overflow: hidden;
}

.assets-tabs,
:deep(.assets-tabs > .arco-tabs-content),
:deep(.assets-tabs > .arco-tabs-content > .arco-tabs-content-list),
:deep(.assets-tabs > .arco-tabs-content > .arco-tabs-content-list > .arco-tabs-pane) {
  height: 100%;
}

:deep(.assets-tabs > .arco-tabs-nav) {
  margin-bottom: 12px;
}

.registry-layout,
.registry-layout > .arco-col {
  height: 100%;
}

.assets-card {
  overflow: hidden;
}

.assets-card-fixed {
  height: calc(100% - 44px);
}

:deep(.assets-card .arco-card-body) {
  height: calc(100% - 50px);
  padding: 0 16px 12px;
  overflow: hidden;
}

:deep(.assets-card .arco-table) {
  height: 100%;
}

:deep(.assets-card .arco-table-container) {
  height: calc(100% - 44px);
}

:deep(.assets-card .arco-table-pagination) {
  margin-top: 10px;
}

.assets-text-ellipsis {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.assets-empty {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

:deep(.assets-row-selected .arco-table-td) {
  background-color: var(--color-fill-2);
}

:deep(.assets-row-highlight .arco-table-td) {
  background-color: rgb(var(--primary-1));
}
</style>
