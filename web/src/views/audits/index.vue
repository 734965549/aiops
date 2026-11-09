<template>
  <div class="audit-page">
    <!-- 顶部聚合统计指标卡片 -->
    <a-row :gutter="16" class="stats-row">
      <a-col :span="6">
        <a-card class="stat-card" :bordered="false">
          <div class="stat-header">
            <span class="stat-label">总操作审计</span>
            <icon-file class="stat-icon icon-primary" />
          </div>
          <div class="stat-value">{{ total }}</div>
          <div class="stat-footer">当前系统记录的总操作数</div>
        </a-card>
      </a-col>
      <a-col :span="6">
        <a-card class="stat-card stat-clickable" :bordered="false" @click="quickFilterResourceType('alert')">
          <div class="stat-header">
            <span class="stat-label">告警动作审计</span>
            <icon-notification class="stat-icon icon-warning" />
          </div>
          <div class="stat-value">{{ alertActionCount }}</div>
          <div class="stat-footer">认领、静音、关闭等动作</div>
        </a-card>
      </a-col>
      <a-col :span="6">
        <a-card class="stat-card stat-clickable" :bordered="false" @click="quickFilterResourceType('execution')">
          <div class="stat-header">
            <span class="stat-label">执行确认审计</span>
            <icon-check-circle class="stat-icon icon-success" />
          </div>
          <div class="stat-value">{{ execActionCount }}</div>
          <div class="stat-footer">任务审批与操作确认</div>
        </a-card>
      </a-col>
      <a-col :span="6">
        <a-card class="stat-card stat-clickable" :bordered="false" @click="quickFilterResourceType('ai')">
          <div class="stat-header">
            <span class="stat-label">AI 工具与分析</span>
            <icon-robot class="stat-icon icon-purple" />
          </div>
          <div class="stat-value">{{ aiActionCount }}</div>
          <div class="stat-footer">模型推理与工具调用</div>
        </a-card>
      </a-col>
    </a-row>

    <!-- 主卡片：切换标签页（明细列表 vs 聚合分析） -->
    <a-card class="main-card">
      <div class="card-toolbar">
        <a-tabs v-model:active-key="activeTab" type="rounded">
          <a-tab-pane key="list">
            <template #title>
              <span><icon-unordered-list /> 操作明细</span>
            </template>
          </a-tab-pane>
          <a-tab-pane key="agg-resource">
            <template #title>
              <span><icon-apps /> 按资源聚合</span>
            </template>
          </a-tab-pane>
          <a-tab-pane key="agg-user">
            <template #title>
              <span><icon-user /> 按用户聚合</span>
            </template>
          </a-tab-pane>
          <a-tab-pane key="agg-trace">
            <template #title>
              <span><icon-branch /> 按 Trace 链路聚合</span>
            </template>
          </a-tab-pane>
        </a-tabs>

        <div class="toolbar-actions">
          <a-button type="outline" :loading="exporting" @click="exportCSV">
            <template #icon><icon-download /></template>
            导出 CSV
          </a-button>
          <a-button type="primary" :loading="loading" @click="onSearch">
            <template #icon><icon-refresh /></template>
            刷新
          </a-button>
        </div>
      </div>

      <!-- 搜索筛选栏 -->
      <a-form :model="query" layout="inline" class="query-form">
        <a-form-item label="资源类型">
          <a-select
            v-model="query.resource_type"
            placeholder="全部类型"
            allow-clear
            style="width: 150px"
            @change="onSearch"
          >
            <a-option value="alert">告警 (alert)</a-option>
            <a-option value="execution">执行 (execution)</a-option>
            <a-option value="runbook">预案 (runbook)</a-option>
            <a-option value="asset">资产 (asset)</a-option>
            <a-option value="ai">AI (ai)</a-option>
            <a-option value="identity">权限/用户 (identity)</a-option>
            <a-option value="inspection">巡检 (inspection)</a-option>
            <a-option value="observability">可观测 (observability)</a-option>
            <a-option value="integration">集成 (integration)</a-option>
          </a-select>
        </a-form-item>

        <a-form-item label="资源 ID">
          <a-input
            v-model="query.resource_id"
            placeholder="模糊或精确匹配"
            allow-clear
            style="width: 170px"
            @press-enter="onSearch"
          />
        </a-form-item>

        <a-form-item label="操作人 ID">
          <a-input
            v-model="query.user_id"
            placeholder="用户 UUID 或系统"
            allow-clear
            style="width: 170px"
            @press-enter="onSearch"
          />
        </a-form-item>

        <a-form-item label="操作动作">
          <a-input
            v-model="query.action"
            placeholder="如 confirm / silence"
            allow-clear
            style="width: 160px"
            @press-enter="onSearch"
          />
        </a-form-item>

        <a-form-item label="Trace ID">
          <a-input
            v-model="query.trace_id"
            placeholder="链路 Trace ID"
            allow-clear
            style="width: 190px"
            @press-enter="onSearch"
          />
        </a-form-item>

        <a-form-item>
          <a-space>
            <a-button type="primary" :loading="loading" @click="onSearch">
              <template #icon><icon-search /></template>
              查询
            </a-button>
            <a-button @click="onReset">重置</a-button>
          </a-space>
        </a-form-item>
      </a-form>

      <!-- 快速标签筛选提示（当由外部或卡片筛选过滤时） -->
      <div v-if="hasActiveFilters" class="active-filter-bar">
        <span class="filter-label">当前有效过滤：</span>
        <a-tag v-if="query.resource_type" closable @close="clearFilter('resource_type')">
          类型: {{ query.resource_type }}
        </a-tag>
        <a-tag v-if="query.resource_id" closable @close="clearFilter('resource_id')">
          资源: {{ query.resource_id }}
        </a-tag>
        <a-tag v-if="query.user_id" closable @close="clearFilter('user_id')">
          用户: {{ query.user_id }}
        </a-tag>
        <a-tag v-if="query.action" closable @close="clearFilter('action')">
          动作: {{ query.action }}
        </a-tag>
        <a-tag v-if="query.trace_id" closable @close="clearFilter('trace_id')">
          Trace: {{ query.trace_id }}
        </a-tag>
        <a-button type="text" size="mini" @click="onReset">清空全部</a-button>
      </div>

      <!-- Tab 1: 操作明细列表 -->
      <div v-if="activeTab === 'list'">
        <a-table
          :data="items"
          :loading="loading"
          row-key="id"
          :pagination="pagination"
          class="audit-table"
          @page-change="onPageChange"
          @page-size-change="onPageSizeChange"
        >
          <template #columns>
            <a-table-column title="时间" data-index="created_at" :width="170">
              <template #cell="{ record }">
                <span class="time-cell">{{ formatTime(record.created_at) }}</span>
              </template>
            </a-table-column>

            <a-table-column title="资源类型" data-index="resource_type" :width="110">
              <template #cell="{ record }">
                <a-tag :color="resourceTypeColor(record.resource_type)">
                  {{ record.resource_type }}
                </a-tag>
              </template>
            </a-table-column>

            <a-table-column title="资源 ID" data-index="resource_id" :width="200" ellipsis tooltip>
              <template #cell="{ record }">
                <a-link @click="filterByResource(record.resource_type, record.resource_id)">
                  {{ record.resource_id }}
                </a-link>
              </template>
            </a-table-column>

            <a-table-column title="操作动作" data-index="action" :width="130">
              <template #cell="{ record }">
                <a-tag :color="actionColor(record.action)" bordered>
                  {{ record.action }}
                </a-tag>
              </template>
            </a-table-column>

            <a-table-column title="操作人" data-index="user_id" :width="180" ellipsis tooltip>
              <template #cell="{ record }">
                <span v-if="!record.user_id" class="text-muted">系统 / 匿名</span>
                <a-link v-else @click="filterByUser(record.user_id)">
                  {{ record.user_id }}
                </a-link>
              </template>
            </a-table-column>

            <a-table-column title="Trace ID" :width="200" ellipsis tooltip>
              <template #cell="{ record }">
                <span v-if="extractTraceID(record)" class="trace-cell">
                  <a-link @click="filterByTrace(extractTraceID(record))">
                    {{ extractTraceID(record) }}
                  </a-link>
                  <a-tooltip content="复制 Trace ID">
                    <icon-copy class="copy-icon" @click.stop="copyText(extractTraceID(record))" />
                  </a-tooltip>
                </span>
                <span v-else class="text-muted">-</span>
              </template>
            </a-table-column>

            <a-table-column title="IP / UA" :width="150" ellipsis tooltip>
              <template #cell="{ record }">
                <span>{{ record.ip || '-' }}</span>
              </template>
            </a-table-column>

            <a-table-column title="操作" :width="90" fixed="right">
              <template #cell="{ record }">
                <a-button type="text" size="small" @click="viewDetail(record)">
                  详情
                </a-button>
              </template>
            </a-table-column>
          </template>
        </a-table>
      </div>

      <!-- Tab 2: 按资源聚合 -->
      <div v-else-if="activeTab === 'agg-resource'">
        <div class="agg-description">
          <icon-info-circle /> 基于当前筛选条件统计的活跃资源及对应变更动作频次。
        </div>
        <a-table
          :data="resourceAggregations"
          :loading="loading"
          row-key="key"
          :pagination="{ pageSize: 10, showTotal: true }"
        >
          <template #columns>
            <a-table-column title="资源类型" data-index="resource_type" :width="130">
              <template #cell="{ record }">
                <a-tag :color="resourceTypeColor(record.resource_type)">{{ record.resource_type }}</a-tag>
              </template>
            </a-table-column>
            <a-table-column title="资源 ID" data-index="resource_id" :width="240" ellipsis tooltip />
            <a-table-column title="操作总数" data-index="count" :width="100" />
            <a-table-column title="涉及动作" data-index="actions">
              <template #cell="{ record }">
                <a-space wrap>
                  <a-tag v-for="(cnt, act) in record.actions" :key="act" size="small">
                    {{ act }} ({{ cnt }})
                  </a-tag>
                </a-space>
              </template>
            </a-table-column>
            <a-table-column title="最后操作时间" data-index="last_time" :width="180">
              <template #cell="{ record }">
                {{ formatTime(record.last_time) }}
              </template>
            </a-table-column>
            <a-table-column title="操作" :width="120">
              <template #cell="{ record }">
                <a-button type="outline" size="mini" @click="filterByResource(record.resource_type, record.resource_id)">
                  查看明细
                </a-button>
              </template>
            </a-table-column>
          </template>
        </a-table>
      </div>

      <!-- Tab 3: 按用户聚合 -->
      <div v-else-if="activeTab === 'agg-user'">
        <div class="agg-description">
          <icon-info-circle /> 统计不同用户/系统账户的操作频率及主要涉及的资源类型。
        </div>
        <a-table
          :data="userAggregations"
          :loading="loading"
          row-key="user_id"
          :pagination="{ pageSize: 10, showTotal: true }"
        >
          <template #columns>
            <a-table-column title="操作人 / 用户 ID" data-index="user_id" :width="260" ellipsis tooltip>
              <template #cell="{ record }">
                <span v-if="record.user_id === 'SYSTEM'">
                  <a-tag color="gray">系统自动化 / 匿名</a-tag>
                </span>
                <span v-else>{{ record.user_id }}</span>
              </template>
            </a-table-column>
            <a-table-column title="操作频次" data-index="count" :width="110" />
            <a-table-column title="操作类型分布" data-index="resource_types">
              <template #cell="{ record }">
                <a-space wrap>
                  <a-tag
                    v-for="(cnt, type) in record.resource_types"
                    :key="type"
                    size="small"
                    :color="resourceTypeColor(type)"
                  >
                    {{ type }}: {{ cnt }}
                  </a-tag>
                </a-space>
              </template>
            </a-table-column>
            <a-table-column title="最后活跃时间" data-index="last_time" :width="180">
              <template #cell="{ record }">
                {{ formatTime(record.last_time) }}
              </template>
            </a-table-column>
            <a-table-column title="操作" :width="120">
              <template #cell="{ record }">
                <a-button
                  v-if="record.user_id !== 'SYSTEM'"
                  type="outline"
                  size="mini"
                  @click="filterByUser(record.user_id)"
                >
                  查看明细
                </a-button>
                <span v-else class="text-muted">-</span>
              </template>
            </a-table-column>
          </template>
        </a-table>
      </div>

      <!-- Tab 4: 按 Trace 聚合 -->
      <div v-else-if="activeTab === 'agg-trace'">
        <div class="agg-description">
          <icon-info-circle /> 将同一 HTTP/MQ 链路（Trace ID）内产生的一连串写操作、审核确认聚合呈现，便于全链路根因排查。
        </div>
        <a-table
          :data="traceAggregations"
          :loading="loading"
          row-key="trace_id"
          :pagination="{ pageSize: 10, showTotal: true }"
        >
          <template #columns>
            <a-table-column title="Trace ID" data-index="trace_id" :width="280" ellipsis tooltip>
              <template #cell="{ record }">
                <span class="trace-cell">
                  <span>{{ record.trace_id }}</span>
                  <a-tooltip content="复制 Trace ID">
                    <icon-copy class="copy-icon" @click.stop="copyText(record.trace_id)" />
                  </a-tooltip>
                </span>
              </template>
            </a-table-column>
            <a-table-column title="链路步骤数" data-index="count" :width="120" />
            <a-table-column title="关联动作轨迹" data-index="actions">
              <template #cell="{ record }">
                <a-space wrap>
                  <a-tag v-for="(act, idx) in record.actions" :key="idx" size="small" :color="actionColor(act)">
                    {{ act }}
                  </a-tag>
                </a-space>
              </template>
            </a-table-column>
            <a-table-column title="时间跨度" data-index="time_span" :width="200">
              <template #cell="{ record }">
                <span>{{ formatTime(record.start_time) }}</span>
              </template>
            </a-table-column>
            <a-table-column title="操作" :width="120">
              <template #cell="{ record }">
                <a-button type="outline" size="mini" @click="filterByTrace(record.trace_id)">
                  链路追溯
                </a-button>
              </template>
            </a-table-column>
          </template>
        </a-table>
      </div>
    </a-card>

    <!-- 详情抽屉 Drawer -->
    <a-drawer
      :visible="drawerVisible"
      title="操作审计详情"
      :width="600"
      unmount-on-close
      @cancel="drawerVisible = false"
    >
      <template v-if="selectedRecord">
        <a-descriptions :column="1" bordered size="medium">
          <a-descriptions-item label="审计 ID">
            <code>{{ selectedRecord.id }}</code>
          </a-descriptions-item>
          <a-descriptions-item label="记录时间">
            {{ formatTime(selectedRecord.created_at) }}
          </a-descriptions-item>
          <a-descriptions-item label="资源类型">
            <a-tag :color="resourceTypeColor(selectedRecord.resource_type)">
              {{ selectedRecord.resource_type }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="资源标识 (ID)">
            <code>{{ selectedRecord.resource_id }}</code>
          </a-descriptions-item>
          <a-descriptions-item label="动作名称 (Action)">
            <a-tag :color="actionColor(selectedRecord.action)">
              {{ selectedRecord.action }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="操作人 (User ID)">
            {{ selectedRecord.user_id || '系统 / 匿名' }}
          </a-descriptions-item>
          <a-descriptions-item label="Trace ID">
            <span v-if="extractTraceID(selectedRecord)" class="trace-cell">
              <code>{{ extractTraceID(selectedRecord) }}</code>
              <icon-copy class="copy-icon" @click="copyText(extractTraceID(selectedRecord))" />
            </span>
            <span v-else class="text-muted">无</span>
          </a-descriptions-item>
          <a-descriptions-item label="客户端 IP">
            {{ selectedRecord.ip || '-' }}
          </a-descriptions-item>
          <a-descriptions-item label="User-Agent">
            <span class="ua-text">{{ selectedRecord.user_agent || '-' }}</span>
          </a-descriptions-item>
        </a-descriptions>

        <a-divider style="margin: 20px 0 12px">业务载荷 (Payload)</a-divider>
        <pre class="json-payload">{{ prettyJSON(selectedRecord.payload) }}</pre>
      </template>

      <template #footer>
        <a-space>
          <a-button
            v-if="selectedRecord && extractTraceID(selectedRecord)"
            type="outline"
            @click="filterByTrace(extractTraceID(selectedRecord))"
          >
            筛选此 Trace 记录
          </a-button>
          <a-button type="primary" @click="drawerVisible = false">关闭</a-button>
        </a-space>
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import {
  IconSearch,
  IconRefresh,
  IconDownload,
  IconFile,
  IconNotification,
  IconCheckCircle,
  IconRobot,
  IconUnorderedList,
  IconApps,
  IconUser,
  IconBranch,
  IconCopy,
  IconInfoCircle
} from '@arco-design/web-vue/es/icon'
import { fetchAudits, type OperationAudit, type AuditListQuery } from '@/api/audit'

// 标签页控制
const activeTab = ref<'list' | 'agg-resource' | 'agg-user' | 'agg-trace'>('list')

// 查询参数
const query = reactive<AuditListQuery>({
  page: 1,
  page_size: 20,
  resource_type: '',
  resource_id: '',
  user_id: '',
  action: '',
  trace_id: ''
})

const loading = ref(false)
const exporting = ref(false)
const items = ref<OperationAudit[]>([])
const total = ref(0)

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showTotal: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100]
})

// 抽屉详情
const drawerVisible = ref(false)
const selectedRecord = ref<OperationAudit | null>(null)

// 是否有过滤
const hasActiveFilters = computed(() => {
  return !!(
    query.resource_type ||
    query.resource_id ||
    query.user_id ||
    query.action ||
    query.trace_id
  )
})

// 顶部各类型指标计数（基于当前列表或已加载数据计算）
const alertActionCount = computed(() => items.value.filter(i => i.resource_type === 'alert').length)
const execActionCount = computed(() => items.value.filter(i => i.resource_type === 'execution').length)
const aiActionCount = computed(() => items.value.filter(i => i.resource_type === 'ai').length)

// 提取 Trace ID
function extractTraceID(record: OperationAudit): string {
  if (!record.payload) return ''
  const tid = record.payload['trace_id']
  if (typeof tid === 'string') return tid
  return ''
}

// 颜色映射辅助函数
function resourceTypeColor(type: string | number): string {
  switch (String(type)) {
    case 'alert':
      return 'orangered'
    case 'execution':
      return 'green'
    case 'runbook':
      return 'blue'
    case 'asset':
      return 'cyan'
    case 'ai':
      return 'purple'
    case 'identity':
      return 'gold'
    case 'inspection':
      return 'magenta'
    case 'observability':
      return 'arcoblue'
    default:
      return 'gray'
  }
}

function actionColor(act: string): string {
  const a = act.toLowerCase()
  if (a.includes('confirm') || a.includes('create') || a.includes('start')) {
    return 'green'
  }
  if (a.includes('fail') || a.includes('delete') || a.includes('reject')) {
    return 'red'
  }
  if (a.includes('silence') || a.includes('close') || a.includes('update')) {
    return 'orange'
  }
  return 'arcoblue'
}

function formatTime(val: number | string | undefined): string {
  if (!val) return '-'
  const date = typeof val === 'number' ? new Date(val * 1000) : new Date(val)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', {
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

// 敏感字段识别：AK/SK、口令、令牌等一律脱敏后再展示或导出
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

function prettyJSON(obj: unknown): string {
  if (!obj) return '{}'
  try {
    return JSON.stringify(redactPayload(obj), null, 2)
  } catch {
    return String(obj)
  }
}

async function copyText(text: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    Message.success('Trace ID 已复制到剪贴板')
  } catch {
    Message.error('复制失败，请手动选择复制')
  }
}

// 数据查询
async function loadData() {
  loading.value = true
  try {
    const res = await fetchAudits({
      page: pagination.current,
      page_size: pagination.pageSize,
      resource_type: query.resource_type || undefined,
      resource_id: query.resource_id || undefined,
      user_id: query.user_id || undefined,
      action: query.action || undefined,
      trace_id: query.trace_id || undefined
    })
    items.value = res.items ?? []
    total.value = res.total ?? 0
    pagination.total = total.value
  } catch (err: unknown) {
    const msg = (err as Error)?.message || '获取审计日志失败'
    Message.error(msg)
  } finally {
    loading.value = false
  }
}

function onSearch() {
  pagination.current = 1
  loadData()
}

function onReset() {
  query.resource_type = ''
  query.resource_id = ''
  query.user_id = ''
  query.action = ''
  query.trace_id = ''
  pagination.current = 1
  loadData()
}

function clearFilter(key: 'resource_type' | 'resource_id' | 'user_id' | 'action' | 'trace_id') {
  query[key] = ''
  onSearch()
}

function onPageChange(page: number) {
  pagination.current = page
  loadData()
}

function onPageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.current = 1
  loadData()
}

// 快速过滤操作
function quickFilterResourceType(type: string) {
  query.resource_type = type
  activeTab.value = 'list'
  onSearch()
}

function filterByResource(type: string, id: string) {
  query.resource_type = type
  query.resource_id = id
  activeTab.value = 'list'
  onSearch()
}

function filterByUser(userId: string) {
  query.user_id = userId
  activeTab.value = 'list'
  onSearch()
}

function filterByTrace(traceId: string) {
  query.trace_id = traceId
  activeTab.value = 'list'
  drawerVisible.value = false
  onSearch()
}

function viewDetail(record: OperationAudit) {
  selectedRecord.value = record
  drawerVisible.value = true
}

// 聚合数据分析计算属性
interface ResourceAggItem {
  key: string
  resource_type: string
  resource_id: string
  count: number
  actions: Record<string, number>
  last_time: number
}

const resourceAggregations = computed<ResourceAggItem[]>(() => {
  const map = new Map<string, ResourceAggItem>()
  for (const item of items.value) {
    const key = `${item.resource_type}:${item.resource_id}`
    if (!map.has(key)) {
      map.set(key, {
        key,
        resource_type: item.resource_type,
        resource_id: item.resource_id,
        count: 0,
        actions: {},
        last_time: item.created_at
      })
    }
    const target = map.get(key)!
    target.count++
    target.actions[item.action] = (target.actions[item.action] || 0) + 1
    if (item.created_at > target.last_time) {
      target.last_time = item.created_at
    }
  }
  return Array.from(map.values()).sort((a, b) => b.count - a.count)
})

interface UserAggItem {
  user_id: string
  count: number
  resource_types: Record<string, number>
  last_time: number
}

const userAggregations = computed<UserAggItem[]>(() => {
  const map = new Map<string, UserAggItem>()
  for (const item of items.value) {
    const uid = item.user_id || 'SYSTEM'
    if (!map.has(uid)) {
      map.set(uid, {
        user_id: uid,
        count: 0,
        resource_types: {},
        last_time: item.created_at
      })
    }
    const target = map.get(uid)!
    target.count++
    target.resource_types[item.resource_type] = (target.resource_types[item.resource_type] || 0) + 1
    if (item.created_at > target.last_time) {
      target.last_time = item.created_at
    }
  }
  return Array.from(map.values()).sort((a, b) => b.count - a.count)
})

interface TraceAggItem {
  trace_id: string
  count: number
  actions: string[]
  start_time: number
}

const traceAggregations = computed<TraceAggItem[]>(() => {
  const map = new Map<string, TraceAggItem>()
  for (const item of items.value) {
    const tid = extractTraceID(item) || 'NO_TRACE'
    if (!map.has(tid)) {
      map.set(tid, {
        trace_id: tid,
        count: 0,
        actions: [],
        start_time: item.created_at
      })
    }
    const target = map.get(tid)!
    target.count++
    target.actions.push(item.action)
    if (item.created_at < target.start_time) {
      target.start_time = item.created_at
    }
  }
  return Array.from(map.values()).sort((a, b) => b.count - a.count)
})

// 导出 CSV 功能
function csvCell(value: unknown): string {
  const str = value == null ? '' : String(value)
  return `"${str.replace(/"/g, '""')}"`
}

async function exportCSV() {
  exporting.value = true
  try {
    // 拉取前 1000 条符合当前筛选条件的审计
    const res = await fetchAudits({
      page: 1,
      page_size: 1000,
      resource_type: query.resource_type || undefined,
      resource_id: query.resource_id || undefined,
      user_id: query.user_id || undefined,
      action: query.action || undefined,
      trace_id: query.trace_id || undefined
    })
    const exportItems = res.items ?? items.value
    if (!exportItems.length) {
      Message.warning('当前无数据可导出')
      return
    }

    const header = ['ID', '时间', '资源类型', '资源ID', '动作', '用户ID', 'TraceID', '客户端IP', 'UserAgent', '载荷Payload']
    const rows = exportItems.map((it: OperationAudit) => [
      it.id,
      formatTime(it.created_at),
      it.resource_type,
      it.resource_id,
      it.action,
      it.user_id || '',
      extractTraceID(it),
      it.ip || '',
      it.user_agent || '',
      JSON.stringify(redactPayload(it.payload || {}))
    ].map(csvCell).join(','))

    const csvContent = [header.map(csvCell).join(','), ...rows].join('\r\n')
    const blob = new Blob(['\uFEFF', csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `audit-export-${new Date().toISOString().slice(0, 10)}.csv`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
    Message.success(`成功导出 ${exportItems.length} 条审计记录`)
  } catch (e) {
    Message.error('导出审计记录失败: ' + (e as Error).message)
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.audit-page {
  padding: 16px;
  background-color: var(--color-fill-2);
  min-height: calc(100vh - 64px);
}

.stats-row {
  margin-bottom: 16px;
}

.stat-card {
  border-radius: 6px;
  background: var(--color-bg-2);
  transition: all 0.2s ease;
}

.stat-clickable {
  cursor: pointer;
}

.stat-clickable:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.08);
}

.stat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.stat-label {
  font-size: 13px;
  color: var(--color-text-3);
}

.stat-icon {
  font-size: 20px;
}

.icon-primary {
  color: rgb(var(--primary-6));
}

.icon-warning {
  color: rgb(var(--warning-6));
}

.icon-success {
  color: rgb(var(--success-6));
}

.icon-purple {
  color: #722ed1;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--color-text-1);
  margin: 8px 0 4px;
}

.stat-footer {
  font-size: 12px;
  color: var(--color-text-4);
}

.main-card {
  border-radius: 6px;
  background: var(--color-bg-2);
}

.card-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--color-border-2);
  padding-bottom: 8px;
}

.toolbar-actions {
  display: flex;
  gap: 10px;
}

.query-form {
  margin-bottom: 16px;
  padding: 12px;
  background: var(--color-fill-1);
  border-radius: 4px;
}

.active-filter-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  font-size: 12px;
}

.filter-label {
  color: var(--color-text-3);
}

.time-cell {
  font-size: 12px;
  color: var(--color-text-2);
}

.trace-cell {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.copy-icon {
  font-size: 13px;
  color: var(--color-text-3);
  cursor: pointer;
  transition: color 0.15s;
}

.copy-icon:hover {
  color: rgb(var(--primary-6));
}

.text-muted {
  color: var(--color-text-4);
}

.agg-description {
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--color-text-3);
  display: flex;
  align-items: center;
  gap: 6px;
}

.ua-text {
  word-break: break-all;
  font-size: 12px;
}

.json-payload {
  background: var(--color-fill-2);
  padding: 12px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  max-height: 280px;
  overflow-y: auto;
  border: 1px solid var(--color-border-2);
}
</style>
