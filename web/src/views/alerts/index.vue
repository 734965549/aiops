<template>
  <div class="alerts-page">
    <a-card
      title="告警中心"
      :bordered="false"
    >
      <template #extra>
        <a-space>
          <a-button
            v-if="canManageSources"
            @click="openSourceModal"
          >
            接入源管理
          </a-button>
          <a-button
            :loading="loadingList"
            @click="loadAlerts"
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
        <a-form-item label="关键词">
          <a-input
            v-model="filters.keyword"
            allow-clear
            placeholder="名称 / 摘要 / 资源"
            style="width: 200px"
            @press-enter="onSearch"
          />
        </a-form-item>
        <a-form-item label="状态">
          <a-select
            v-model="filters.status"
            allow-clear
            placeholder="全部"
            style="width: 140px"
            :options="statusOptions"
          />
        </a-form-item>
        <a-form-item label="级别">
          <a-select
            v-model="filters.severity"
            allow-clear
            placeholder="全部"
            style="width: 100px"
            :options="severityOptions"
          />
        </a-form-item>
        <a-form-item label="接入源">
          <a-select
            v-model="filters.source_id"
            allow-clear
            placeholder="全部"
            style="width: 180px"
            :options="sourceFilterOptions"
          />
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model="filters.active_only">
            仅活跃告警
          </a-checkbox>
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

      <a-alert
        v-if="listLoadError"
        type="error"
        :title="listLoadError"
        closable
        style="margin-bottom: 12px"
        @close="listLoadError = ''"
      />

      <a-table
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
      :width="720"
      :title="detail?.alert.name || '告警详情'"
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
            <a-tag :color="statusColor(detail.alert.status)">
              {{ statusLabel(detail.alert.status) }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="级别">
            <a-tag :color="severityColor(detail.alert.severity)">
              {{ severityLabel(detail.alert.severity) }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="来源">
            {{ sourceLabel(detail.alert) }}
          </a-descriptions-item>
          <a-descriptions-item label="触发次数">
            {{ detail.alert.occurrence_count }}
          </a-descriptions-item>
          <a-descriptions-item
            label="摘要"
            :span="2"
          >
            {{ detail.alert.summary || '—' }}
          </a-descriptions-item>
          <a-descriptions-item
            label="描述"
            :span="2"
          >
            {{ detail.alert.description || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="环境">
            {{ detail.alert.environment || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="应用">
            <RouterLink
              v-if="detail.alert.application_id"
              :to="assetLink(detail.alert.application_id)"
            >
              {{ detail.alert.application_name || detail.alert.application_id }}
            </RouterLink>
            <span v-else>{{ detail.alert.application_name || '—' }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="资源">
            <RouterLink
              v-if="detail.alert.resource_id"
              :to="assetLink(detail.alert.application_id, detail.alert.resource_id)"
            >
              {{ detail.alert.resource_name || detail.alert.resource_id }}
            </RouterLink>
            <span v-else>{{ detail.alert.resource_name || '—' }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="首次触发">
            {{ formatTime(detail.alert.first_seen_at) }}
          </a-descriptions-item>
          <a-descriptions-item label="最近更新">
            {{ formatTime(detail.alert.last_seen_at) }}
          </a-descriptions-item>
        </a-descriptions>

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
              :loading="aiLoading"
              @click="openAIModal"
            >
              AI 分析
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

    <!-- 接入源管理 -->
    <a-modal
      v-model:visible="sourceModalVisible"
      title="告警接入源"
      :width="900"
      :footer="false"
      unmount-on-close
      @open="loadSources"
    >
      <a-row :gutter="16">
        <a-col :span="14">
          <a-table
            :columns="sourceColumns"
            :data="sources"
            :loading="loadingSources"
            row-key="id"
            :pagination="false"
            @row-click="onSelectSource"
          >
            <template #enabled="{ record }">
              <a-tag :color="record.enabled ? 'green' : 'gray'">
                {{ record.enabled ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #secretMasked="{ record }">
              <span v-if="record.secret_masked">{{ record.secret_masked }}</span>
              <span
                v-else
                class="text-muted"
              >未配置</span>
            </template>
            <template #webhook="{ record }">
              <a-typography-text
                copyable
                :copy-text="webhookUrl(record)"
              >
                {{ webhookUrl(record) }}
              </a-typography-text>
            </template>
            <template #sourceActions="{ record }">
              <a-popconfirm
                content="确认删除该接入源？"
                @ok="onDeleteSource(record.id)"
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
            </template>
          </a-table>
        </a-col>
        <a-col :span="10">
          <a-card :title="editingSourceId ? '编辑接入源' : '新增接入源'">
            <a-form
              :model="sourceForm"
              layout="vertical"
            >
              <a-form-item
                label="ID（Webhook URL 路径）"
                required
              >
                <a-input
                  v-model="sourceForm.id"
                  :disabled="!!editingSourceId"
                  placeholder="prod-am"
                />
              </a-form-item>
              <a-form-item
                label="名称"
                required
              >
                <a-input
                  v-model="sourceForm.name"
                  placeholder="生产 Alertmanager"
                />
              </a-form-item>
              <a-form-item label="类型">
                <a-select
                  v-model="sourceForm.type"
                  :options="sourceTypeOptions"
                />
              </a-form-item>
              <a-form-item
                :label="editingSourceId ? '密钥（留空保留原值）' : 'Webhook 密钥'"
                :required="!editingSourceId"
              >
                <a-input-password
                  v-model="sourceForm.secret"
                  placeholder="X-AIOPS-Webhook-Token"
                />
                <div
                  v-if="editingSourceId && editingSourceSecretMasked"
                  class="field-hint"
                >
                  当前已配置密钥：{{ editingSourceSecretMasked }}
                </div>
              </a-form-item>
              <a-form-item label="环境">
                <a-input
                  v-model="sourceForm.environment"
                  placeholder="prod"
                />
              </a-form-item>
              <a-form-item label="业务线">
                <a-input
                  v-model="sourceForm.business_line"
                  placeholder="payment"
                />
              </a-form-item>
              <a-form-item label="备注">
                <a-textarea
                  v-model="sourceForm.description"
                  :auto-size="{ minRows: 2, maxRows: 4 }"
                />
              </a-form-item>
              <a-form-item>
                <a-checkbox v-model="sourceForm.enabled">
                  启用
                </a-checkbox>
              </a-form-item>
              <a-space>
                <a-button
                  type="primary"
                  :loading="savingSource"
                  @click="onSaveSource"
                >
                  {{ editingSourceId ? '保存' : '创建' }}
                </a-button>
                <a-button @click="resetSourceForm">
                  重置
                </a-button>
              </a-space>
            </a-form>
          </a-card>
        </a-col>
      </a-row>
    </a-modal>

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

    <a-modal
      v-model:visible="showAIModal"
      title="AI 告警分析"
      :width="640"
      :ok-loading="aiLoading"
      @before-ok="beforeAIAnalysis"
      @cancel="resetAIModal"
    >
      <a-form
        :model="aiForm"
        layout="vertical"
      >
        <a-form-item label="时间范围">
          <a-input
            v-model="aiForm.time_range"
            placeholder="30m"
          />
        </a-form-item>
        <a-form-item>
          <a-space direction="vertical">
            <a-checkbox v-model="aiForm.include_logs">
              包含日志
            </a-checkbox>
            <a-checkbox v-model="aiForm.include_metrics">
              包含指标
            </a-checkbox>
            <a-checkbox v-model="aiForm.include_changes">
              包含变更
            </a-checkbox>
          </a-space>
        </a-form-item>
      </a-form>
      <a-alert
        v-if="aiResult"
        type="success"
        class="ai-result"
      >
        <div><strong>风险等级：</strong>{{ aiResult.risk_level }}</div>
        <div><strong>摘要：</strong>{{ aiResult.summary }}</div>
        <div v-if="aiResult.recommendations?.length">
          <strong>建议：</strong>
          <ul>
            <li
              v-for="(r, i) in aiResult.recommendations"
              :key="i"
            >
              {{ r }}
            </li>
          </ul>
        </div>
      </a-alert>
    </a-modal>

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
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import { useAuthStore } from '@/stores/auth'
import { getApiError } from '@/api/request'
import { analyzeAlert, type AnalyzeAlertResult } from '@/api/ai'
import * as alertApi from '@/api/alert'
import type { Alert, AlertDetail, AlertEvent, AlertSource } from '@/api/alert'
import * as executionApi from '@/api/execution'
import * as runbookApi from '@/api/runbook'
import type { RunbookRecommendation } from '@/api/runbook'

const router = useRouter()

const auth = useAuthStore()

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
const aiLoading = ref(false)
const executionLoading = ref(false)
const runbookLoading = ref(false)
const runbookRecommendations = ref<RunbookRecommendation[]>([])
const savingSource = ref(false)
const canManageSources = ref(false)

const detailVisible = ref(false)
const sourceModalVisible = ref(false)
const showCloseModal = ref(false)
const showSilenceModal = ref(false)
const showAssignModal = ref(false)
const showCommentModal = ref(false)
const showAIModal = ref(false)
const showExecutionModal = ref(false)

const editingSourceId = ref('')
const editingSourceSecretMasked = ref('')
const aiResult = ref<AnalyzeAlertResult | null>(null)

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
const aiForm = reactive({
  time_range: '30m',
  include_logs: false,
  include_metrics: false,
  include_changes: false
})

const runbookForm = reactive({
  template_id: '',
  name: '',
  dry_run: true,
  parameters_json: '{}'
})

const sourceForm = reactive({
  id: '',
  name: '',
  type: 'prometheus_alertmanager',
  secret: '',
  environment: '',
  business_line: '',
  description: '',
  enabled: true
})

const statusOptions = [
  { label: '新建', value: 'new' },
  { label: '已认领', value: 'acknowledged' },
  { label: '处理中', value: 'processing' },
  { label: '已恢复', value: 'recovered' },
  { label: '已关闭', value: 'closed' },
  { label: '已静默', value: 'silenced' }
]

const severityOptions = [
  { label: 'P0', value: 'p0' },
  { label: 'P1', value: 'p1' },
  { label: 'P2', value: 'p2' },
  { label: 'P3', value: 'p3' },
  { label: 'Info', value: 'info' }
]

const sourceTypeOptions = [
  { label: 'Prometheus Alertmanager', value: 'prometheus_alertmanager' },
  { label: '通用 Webhook', value: 'custom_webhook' }
]

const columns = [
  { title: '告警名称', dataIndex: 'name', ellipsis: true },
  { title: '级别', slotName: 'severity', width: 80 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '来源', slotName: 'source', width: 140, ellipsis: true },
  { title: '环境', dataIndex: 'environment', width: 80 },
  { title: '次数', dataIndex: 'occurrence_count', width: 70 },
  { title: '最近更新', slotName: 'last_seen_at', width: 170 },
  { title: '操作', slotName: 'actions', width: 80 }
]

const sourceColumns = [
  { title: 'ID', dataIndex: 'id', width: 100 },
  { title: '名称', dataIndex: 'name' },
  { title: '类型', dataIndex: 'type', width: 160, ellipsis: true },
  { title: '密钥', slotName: 'secretMasked', width: 80 },
  { title: '状态', slotName: 'enabled', width: 80 },
  { title: 'Webhook', slotName: 'webhook', ellipsis: true },
  { title: '操作', slotName: 'sourceActions', width: 80 }
]

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

function severityLabel(s: string) {
  const map: Record<string, string> = {
    p0: 'P0',
    p1: 'P1',
    p2: 'P2',
    p3: 'P3',
    info: 'Info'
  }
  return map[s] || s
}

function severityColor(s: string) {
  const map: Record<string, string> = {
    p0: 'red',
    p1: 'orangered',
    p2: 'orange',
    p3: 'gold',
    info: 'arcoblue'
  }
  return map[s] || 'gray'
}

function statusLabel(s: string) {
  const map: Record<string, string> = {
    new: '新建',
    acknowledged: '已认领',
    processing: '处理中',
    recovered: '已恢复',
    closed: '已关闭',
    silenced: '已静默'
  }
  return map[s] || s
}

function statusColor(s: string) {
  const map: Record<string, string> = {
    new: 'red',
    acknowledged: 'orangered',
    processing: 'orange',
    recovered: 'green',
    closed: 'gray',
    silenced: 'purple'
  }
  return map[s] || 'gray'
}

function eventTypeLabel(t: string) {
  const map: Record<string, string> = {
    triggered: '首次触发',
    updated: '重复触发',
    recovered: '已恢复',
    acknowledged: '认领',
    assigned: '转派',
    processing_started: '开始处理',
    closed: '关闭',
    silenced: '静默',
    unsilenced: '取消静默',
    commented: '备注',
    ai_analysis_requested: 'AI 分析',
    execution_created: '创建执行任务',
    execution_started: '开始执行',
    execution_finished: '执行完成'
  }
  return map[t] || t
}

function executionIdFromEvent(ev: AlertEvent): string | null {
  const id = ev.payload?.execution_id
  return typeof id === 'string' && id ? id : null
}

function goToExecution(taskId: string) {
  router.push({ path: '/executions', query: { task_id: taskId } })
}

function assetLink(applicationId?: string, resourceId?: string) {
  const query: Record<string, string> = {}
  if (applicationId) query.application_id = applicationId
  if (resourceId) query.resource_id = resourceId
  return { path: '/assets', query }
}

function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

function sourceLabel(alert: Alert) {
  return alert.source_name || alert.source_id || alert.source || '—'
}

function webhookUrl(source: Pick<AlertSource, 'id' | 'type'>) {
  const base = import.meta.env.VITE_API_BASE || window.location.origin
  const ingestPath =
    source.type === 'custom_webhook'
      ? `/api/alerts/ingest/webhook/${source.id}`
      : `/api/alerts/ingest/alertmanager/${source.id}`
  return `${base.replace(/\/$/, '')}${ingestPath}`
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
  loadingDetail.value = true
  try {
    detail.value = await alertApi.getAlert(alertId)
  } finally {
    loadingDetail.value = false
  }
}

function closeDetail() {
  detailVisible.value = false
  detail.value = null
  selectedAlertId.value = ''
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
  aiResult.value = null
  showAIModal.value = true
}

function resetAIModal() {
  aiResult.value = null
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

async function beforeAIAnalysis(): Promise<boolean> {
  if (!selectedAlertId.value) return false
  aiLoading.value = true
  try {
    await alertApi.requestAlertAIAnalysis(selectedAlertId.value, {
      time_range: aiForm.time_range,
      include_logs: aiForm.include_logs,
      include_metrics: aiForm.include_metrics,
      include_changes: aiForm.include_changes
    })
    aiResult.value = await analyzeAlert({
      alert_id: selectedAlertId.value,
      time_range: aiForm.time_range,
      include_logs: aiForm.include_logs,
      include_metrics: aiForm.include_metrics,
      include_changes: aiForm.include_changes
    })
    Message.success('AI 分析完成')
    await refreshDetail()
    // 保留弹窗以展示分析结果，由用户手动关闭。
    return false
  } catch {
    return false
  } finally {
    aiLoading.value = false
  }
}

function openSourceModal() {
  if (!canManageSources.value) {
    Message.warning('当前账号无接入源管理权限')
    return
  }
  sourceModalVisible.value = true
}

function resetSourceForm() {
  editingSourceId.value = ''
  editingSourceSecretMasked.value = ''
  sourceForm.id = ''
  sourceForm.name = ''
  sourceForm.type = 'prometheus_alertmanager'
  sourceForm.secret = ''
  sourceForm.environment = ''
  sourceForm.business_line = ''
  sourceForm.description = ''
  sourceForm.enabled = true
}

function onSelectSource(record: TableData) {
  const src = record as AlertSource
  editingSourceId.value = src.id
  editingSourceSecretMasked.value = src.secret_masked || ''
  sourceForm.id = src.id
  sourceForm.name = src.name
  sourceForm.type = src.type
  sourceForm.secret = ''
  sourceForm.environment = src.environment || ''
  sourceForm.business_line = src.business_line || ''
  sourceForm.description = src.description || ''
  sourceForm.enabled = src.enabled
}

async function onSaveSource() {
  if (!sourceForm.id.trim() || !sourceForm.name.trim()) {
    Message.warning('请填写 ID 与名称')
    return
  }
  if (!editingSourceId.value && !sourceForm.secret.trim()) {
    Message.warning('请填写 Webhook 密钥')
    return
  }
  savingSource.value = true
  try {
    if (editingSourceId.value) {
      await alertApi.updateAlertSource(editingSourceId.value, {
        name: sourceForm.name,
        type: sourceForm.type,
        enabled: sourceForm.enabled,
        secret: sourceForm.secret || undefined,
        environment: sourceForm.environment || undefined,
        business_line: sourceForm.business_line || undefined,
        description: sourceForm.description || undefined
      })
      Message.success('接入源已更新')
    } else {
      await alertApi.createAlertSource({
        id: sourceForm.id.trim(),
        name: sourceForm.name.trim(),
        type: sourceForm.type,
        enabled: sourceForm.enabled,
        secret: sourceForm.secret.trim(),
        environment: sourceForm.environment || undefined,
        business_line: sourceForm.business_line || undefined,
        description: sourceForm.description || undefined
      })
      Message.success('接入源已创建')
    }
    await loadSources()
    resetSourceForm()
  } finally {
    savingSource.value = false
  }
}

async function onDeleteSource(sourceId: string) {
  await alertApi.deleteAlertSource(sourceId)
  Message.success('已删除')
  if (editingSourceId.value === sourceId) {
    resetSourceForm()
  }
  await loadSources()
}

onMounted(async () => {
  assignForm.assignee_user_id = auth.user?.id || ''
  await Promise.allSettled([loadSources(), loadAlerts()])
})
</script>

<style scoped lang="scss">
.alerts-page {
  .filter-form {
    margin-bottom: 16px;
  }

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

  .ai-result {
    margin-top: 12px;
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

  .field-hint {
    margin-top: 4px;
    font-size: 12px;
    color: var(--color-text-3);
  }

  .text-muted {
    color: var(--color-text-3);
  }
}
</style>
