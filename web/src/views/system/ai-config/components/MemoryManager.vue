<template>
  <div class="memory-manager">
    <a-alert
      type="info"
      class="memory-tip"
    >
      记忆条目由「有效反馈 + 非降级回答」自动沉淀。手动停用后不再参与后续回答检索，可随时重新启用；
      启停动作会写入审计，但不会修改案例内容与质量分。
    </a-alert>

    <div class="memory-toolbar">
      <a-space wrap>
        <a-select
          v-model="statusFilter"
          placeholder="全部状态"
          allow-clear
          style="width: 140px"
          @change="load"
        >
          <a-option value="active">
            生效中
          </a-option>
          <a-option value="retired">
            已停用
          </a-option>
        </a-select>
        <a-input
          v-model="keywordFilter"
          placeholder="摘要 / 关键词"
          allow-clear
          style="width: 200px"
          @press-enter="load"
        />
        <a-input
          v-model="userFilter"
          placeholder="归属用户 ID"
          allow-clear
          style="width: 170px"
          @press-enter="load"
        />
        <a-button
          type="primary"
          :loading="loading"
          @click="load"
        >
          查询
        </a-button>
        <a-button @click="onReset">
          重置
        </a-button>
      </a-space>
    </div>

    <a-spin
      :loading="loading"
      style="display: block"
    >
      <a-empty
        v-if="!items.length"
        description="暂无记忆条目：有效反馈且未降级的回答会自动沉淀为相似案例"
      />
      <a-table
        v-else
        :data="items"
        :pagination="false"
        row-key="case_id"
        size="small"
      >
        <template #columns>
          <a-table-column
            title="摘要"
            :ellipsis="true"
            :tooltip="true"
          >
            <template #cell="{ record }">
              {{ record.summary || '—' }}
            </template>
          </a-table-column>
          <a-table-column
            title="归属 / 范围"
            :width="200"
          >
            <template #cell="{ record }">
              <div>{{ record.user_id || '—' }}</div>
              <div class="sub-text">
                {{ record.application_id || '未关联应用' }}
                <span v-if="record.resource_id"> / {{ record.resource_id }}</span>
              </div>
            </template>
          </a-table-column>
          <a-table-column
            title="来源"
            :width="110"
          >
            <template #cell="{ record }">
              {{ sourceLabel(record.source_type) }}
            </template>
          </a-table-column>
          <a-table-column
            title="质量分"
            :width="88"
          >
            <template #cell="{ record }">
              <span :class="{ 'score-negative': record.quality_score < 0 }">
                {{ record.quality_score }}
              </span>
            </template>
          </a-table-column>
          <a-table-column
            title="状态"
            :width="96"
          >
            <template #cell="{ record }">
              <a-tag
                size="small"
                :color="record.status === 'active' ? 'green' : 'gray'"
              >
                {{ statusLabel(record.status) }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column
            title="更新时间"
            :width="160"
          >
            <template #cell="{ record }">
              {{ formatTime(record.updated_at) }}
            </template>
          </a-table-column>
          <a-table-column
            title="操作"
            :width="140"
          >
            <template #cell="{ record }">
              <a-space>
                <a-button
                  size="mini"
                  type="text"
                  @click="openDetail(record)"
                >
                  详情
                </a-button>
                <a-popconfirm
                  v-if="record.status === 'active'"
                  content="停用后该案例不再参与后续回答检索，确定停用？"
                  ok-text="停用"
                  @ok="onToggle(record, 'retired')"
                >
                  <a-button
                    size="mini"
                    type="text"
                    status="warning"
                  >
                    停用
                  </a-button>
                </a-popconfirm>
                <a-button
                  v-else
                  size="mini"
                  type="text"
                  @click="onToggle(record, 'active')"
                >
                  启用
                </a-button>
              </a-space>
            </template>
          </a-table-column>
        </template>
      </a-table>
    </a-spin>

    <a-modal
      v-model:visible="detailVisible"
      title="记忆条目详情"
      :footer="false"
      width="720px"
      unmount-on-close
    >
      <div
        v-if="detail"
        class="memory-detail"
      >
        <a-descriptions
          :column="2"
          bordered
          size="small"
        >
          <a-descriptions-item
            label="Case ID"
            :span="2"
          >
            {{ detail.case_id }}
          </a-descriptions-item>
          <a-descriptions-item label="归属用户">
            {{ detail.user_id || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="来源">
            {{ sourceLabel(detail.source_type) }}
          </a-descriptions-item>
          <a-descriptions-item label="应用">
            {{ detail.application_id || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="资源">
            {{ detail.resource_id || '—' }}
            <span
              v-if="detail.resource_type"
              class="sub-text"
            >（{{ detail.resource_type }}）</span>
          </a-descriptions-item>
          <a-descriptions-item label="质量分">
            {{ detail.quality_score }}
          </a-descriptions-item>
          <a-descriptions-item label="状态">
            {{ statusLabel(detail.status) }}
          </a-descriptions-item>
          <a-descriptions-item
            label="风险等级"
            :span="2"
          >
            {{ riskLabel(detail.risk_level) }}
          </a-descriptions-item>
          <a-descriptions-item
            label="告警指纹"
            :span="2"
          >
            {{ detail.alert_signature || '—' }}
          </a-descriptions-item>
          <a-descriptions-item
            label="关键词"
            :span="2"
          >
            {{ detail.keywords || '—' }}
          </a-descriptions-item>
        </a-descriptions>

        <section class="detail-block">
          <strong>结论摘要</strong>
          <p>{{ detail.summary || '—' }}</p>
        </section>

        <section class="detail-block">
          <strong>推荐操作</strong>
          <ol v-if="detail.recommendations && detail.recommendations.length">
            <li
              v-for="(item, index) in detail.recommendations"
              :key="index"
            >
              {{ item }}
            </li>
          </ol>
          <p v-else>
            —
          </p>
        </section>

        <div class="detail-foot">
          创建 {{ formatTime(detail.created_at) }} · 更新 {{ formatTime(detail.updated_at) }}
        </div>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import {
  listMemoryCases,
  updateMemoryCaseStatus,
  type MemoryCase
} from '@/api/ai'

const items = ref<MemoryCase[]>([])
const loading = ref(false)
const statusFilter = ref('')
const keywordFilter = ref('')
const userFilter = ref('')
const detailVisible = ref(false)
const detail = ref<MemoryCase | null>(null)

const SOURCE_LABELS: Record<string, string> = {
  chat_message: '对话沉淀',
  alert_analysis: '告警分析',
  knowledge_item: '知识沉淀'
}

const STATUS_LABELS: Record<string, string> = {
  active: '生效中',
  retired: '已停用'
}

const RISK_LABELS: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  info: '信息'
}

function sourceLabel(value: string) {
  return SOURCE_LABELS[value] ?? value ?? '—'
}

function statusLabel(value: string) {
  return STATUS_LABELS[value] ?? value ?? '—'
}

function riskLabel(value: string) {
  return RISK_LABELS[value] ?? value ?? '—'
}

function formatTime(ts: number) {
  if (!ts || ts <= 0) return '—'
  return new Date(ts * 1000).toLocaleString('zh-CN', { hour12: false })
}

async function load() {
  loading.value = true
  try {
    const params: {
      status?: string
      keyword?: string
      user_id?: string
      limit: number
    } = { limit: 100 }
    if (statusFilter.value) params.status = statusFilter.value
    if (keywordFilter.value.trim()) params.keyword = keywordFilter.value.trim()
    if (userFilter.value.trim()) params.user_id = userFilter.value.trim()
    const res = await listMemoryCases(params)
    items.value = res.items ?? []
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    loading.value = false
  }
}

function onReset() {
  statusFilter.value = ''
  keywordFilter.value = ''
  userFilter.value = ''
  load()
}

function openDetail(record: MemoryCase) {
  detail.value = record
  detailVisible.value = true
}

async function onToggle(record: MemoryCase, target: 'active' | 'retired') {
  try {
    await updateMemoryCaseStatus(record.case_id, target)
    Message.success(target === 'retired' ? '已停用，不再参与后续检索' : '已启用，重新参与检索')
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

onMounted(load)
</script>

<style scoped>
.memory-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.memory-tip :deep(.arco-alert-content) {
  line-height: 1.6;
}

.memory-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.sub-text {
  color: var(--color-text-3);
  font-size: 11px;
}

.score-negative {
  color: var(--color-danger-6);
  font-weight: 600;
}

.memory-detail {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-block strong {
  display: block;
  margin-bottom: 6px;
  color: var(--color-text-1);
}

.detail-block p,
.detail-block ol {
  margin: 0;
  padding-left: 18px;
  color: var(--color-text-2);
  font-size: 13px;
  line-height: 1.7;
}

.detail-block p {
  padding-left: 0;
}

.detail-foot {
  padding-top: 10px;
  border-top: 1px solid var(--color-border-2);
  color: var(--color-text-3);
  font-size: 12px;
}
</style>
