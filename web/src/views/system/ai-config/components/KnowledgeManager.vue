<template>
  <div class="knowledge-manager">
    <div class="knowledge-toolbar">
      <a-space>
        <a-select
          v-model="statusFilter"
          placeholder="全部状态"
          allow-clear
          style="width: 140px"
          @change="load"
        >
          <a-option value="draft">草稿</a-option>
          <a-option value="active">已生效</a-option>
          <a-option value="disabled">已停用</a-option>
        </a-select>
        <a-select
          v-model="scopeFilter"
          placeholder="全部范围"
          allow-clear
          style="width: 150px"
          @change="load"
        >
          <a-option value="global">全局</a-option>
          <a-option value="application">应用</a-option>
          <a-option value="resource_type">资源类型</a-option>
        </a-select>
        <a-button
          :loading="loading"
          @click="load"
        >
          刷新
        </a-button>
      </a-space>
      <a-space>
        <a-input
          v-model="aggregateAppId"
          placeholder="应用 ID（反馈聚合）"
          allow-clear
          style="width: 220px"
        />
        <a-button
          :loading="aggregating"
          @click="onAggregate"
        >
          反馈聚合生成草稿
        </a-button>
        <a-button
          type="primary"
          @click="openCreate"
        >
          新建知识
        </a-button>
      </a-space>
    </div>

    <a-spin
      :loading="loading"
      style="display: block"
    >
      <a-empty
        v-if="!items.length"
        description="暂无知识条目：可人工新建，或从历史有效案例聚合生成草稿"
      />
      <a-table
        v-else
        :data="items"
        :pagination="false"
        row-key="item_id"
        size="small"
      >
        <template #columns>
          <a-table-column
            title="标题"
            data-index="title"
            :ellipsis="true"
            :tooltip="true"
          />
          <a-table-column
            title="范围"
            :width="150"
          >
            <template #cell="{ record }">
              <a-tag size="small">
                {{ scopeLabel(record.scope) }}
              </a-tag>
              <span class="scope-ref">{{ record.scope_ref || '—' }}</span>
            </template>
          </a-table-column>
          <a-table-column
            title="来源"
            :width="110"
          >
            <template #cell="{ record }">
              {{ originLabel(record.origin) }}
            </template>
          </a-table-column>
          <a-table-column
            title="状态"
            :width="100"
          >
            <template #cell="{ record }">
              <a-tag
                size="small"
                :color="statusColor(record.status)"
              >
                {{ statusLabel(record.status) }}
              </a-tag>
            </template>
          </a-table-column>
          <a-table-column
            title="命中 / 有效"
            :width="110"
          >
            <template #cell="{ record }">
              {{ record.hit_count }} / {{ record.useful_count }}
            </template>
          </a-table-column>
          <a-table-column
            title="操作"
            :width="230"
          >
            <template #cell="{ record }">
              <a-space>
                <a-button
                  v-if="record.status !== 'active'"
                  size="mini"
                  type="text"
                  @click="onReview(record, 'activate')"
                >
                  激活
                </a-button>
                <a-button
                  v-if="record.status === 'active'"
                  size="mini"
                  type="text"
                  @click="onReview(record, 'disable')"
                >
                  停用
                </a-button>
                <a-button
                  size="mini"
                  type="text"
                  @click="openEdit(record)"
                >
                  编辑
                </a-button>
                <a-popconfirm
                  content="删除后不可恢复，确定删除该知识条目？"
                  ok-text="删除"
                  @ok="onDelete(record)"
                >
                  <a-button
                    size="mini"
                    type="text"
                    status="danger"
                  >
                    删除
                  </a-button>
                </a-popconfirm>
              </a-space>
            </template>
          </a-table-column>
        </template>
      </a-table>
    </a-spin>

    <a-modal
      v-model:visible="editorVisible"
      :title="editingId ? '编辑知识条目' : '新建知识条目'"
      :ok-loading="saving"
      unmount-on-close
      @ok="onSave"
    >
      <a-form :model="form" layout="vertical">
        <a-form-item label="生效范围">
          <a-select v-model="form.scope">
            <a-option value="global">全局</a-option>
            <a-option value="application">应用</a-option>
            <a-option value="resource_type">资源类型</a-option>
          </a-select>
        </a-form-item>
        <a-form-item
          v-if="form.scope !== 'global'"
          :label="form.scope === 'application' ? '应用 ID' : '资源类型（如 ecs / pod）'"
        >
          <a-input v-model="form.scope_ref" />
        </a-form-item>
        <a-form-item label="标题">
          <a-input
            v-model="form.title"
            :max-length="120"
            show-word-limit
          />
        </a-form-item>
        <a-form-item label="内容">
          <a-textarea
            v-model="form.content"
            :auto-size="{ minRows: 4, maxRows: 10 }"
            :max-length="2000"
            show-word-limit
          />
        </a-form-item>
        <a-alert
          v-if="editingId && editingStatus === 'active'"
          type="warning"
        >
          编辑已生效条目会回到「草稿」，必须重新审核激活后才会再次注入回答。
        </a-alert>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import {
  aggregateKnowledgeFromFeedback,
  createKnowledgeItem,
  deleteKnowledgeItem,
  listKnowledgeItems,
  reviewKnowledgeItem,
  updateKnowledgeItem,
  type KnowledgeItem,
  type KnowledgeItemInput
} from '@/api/ai'

const items = ref<KnowledgeItem[]>([])
const loading = ref(false)
const saving = ref(false)
const aggregating = ref(false)
const statusFilter = ref('')
const scopeFilter = ref('')
const aggregateAppId = ref('')

const editorVisible = ref(false)
const editingId = ref('')
const editingStatus = ref('')
const form = reactive<KnowledgeItemInput>({ scope: 'global', scope_ref: '', title: '', content: '' })

const SCOPE_LABELS: Record<string, string> = {
  global: '全局',
  application: '应用',
  resource_type: '资源类型'
}

const STATUS_LABELS: Record<string, string> = {
  draft: '草稿',
  active: '已生效',
  disabled: '已停用'
}

const ORIGIN_LABELS: Record<string, string> = {
  manual: '人工录入',
  feedback_aggregate: '反馈聚合'
}

function scopeLabel(value: string) {
  return SCOPE_LABELS[value] ?? value
}

function statusLabel(value: string) {
  return STATUS_LABELS[value] ?? value
}

function originLabel(value: string) {
  return ORIGIN_LABELS[value] ?? value
}

function statusColor(value: string) {
  if (value === 'active') return 'green'
  if (value === 'disabled') return 'gray'
  return 'orange'
}

async function load() {
  loading.value = true
  try {
    const params: { status?: string; scope?: string; limit?: number } = { limit: 100 }
    if (statusFilter.value) params.status = statusFilter.value
    if (scopeFilter.value) params.scope = scopeFilter.value
    const res = await listKnowledgeItems(params)
    items.value = res.items ?? []
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = ''
  editingStatus.value = ''
  form.scope = 'global'
  form.scope_ref = ''
  form.title = ''
  form.content = ''
  editorVisible.value = true
}

function openEdit(record: KnowledgeItem) {
  editingId.value = record.item_id
  editingStatus.value = record.status
  form.scope = record.scope
  form.scope_ref = record.scope_ref
  form.title = record.title
  form.content = record.content
  editorVisible.value = true
}

async function onSave() {
  if (!form.title.trim() || !form.content.trim()) {
    Message.warning('标题与内容均不能为空')
    return
  }
  saving.value = true
  try {
    const payload: KnowledgeItemInput = {
      scope: form.scope,
      scope_ref: form.scope === 'global' ? '' : (form.scope_ref ?? '').trim(),
      title: form.title.trim(),
      content: form.content.trim()
    }
    if (editingId.value) {
      await updateKnowledgeItem(editingId.value, payload)
      Message.success('已保存，需重新审核激活后才会生效')
    } else {
      await createKnowledgeItem(payload)
      Message.success('已创建草稿，审核激活后生效')
    }
    editorVisible.value = false
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    saving.value = false
  }
}

async function onReview(record: KnowledgeItem, action: 'activate' | 'disable') {
  try {
    await reviewKnowledgeItem(record.item_id, action)
    Message.success(action === 'activate' ? '已激活，立即参与回答' : '已停用，立即停止注入')
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

async function onDelete(record: KnowledgeItem) {
  try {
    await deleteKnowledgeItem(record.item_id)
    Message.success('已删除')
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

async function onAggregate() {
  const applicationId = aggregateAppId.value.trim()
  if (!applicationId) {
    Message.warning('请填写用于聚合的应用 ID')
    return
  }
  aggregating.value = true
  try {
    const result = await aggregateKnowledgeFromFeedback(applicationId)
    if (result.action === 'insufficient_cases') {
      Message.warning(`有效案例不足（当前 ${result.source_cases} 条），暂不生成草稿`)
    } else if (result.action === 'skipped_active') {
      Message.info('该应用已有生效的聚合知识，未覆盖；如需更新请先停用')
    } else if (result.action === 'created') {
      Message.success(`已生成草稿（来源 ${result.source_cases} 条案例）`)
    } else {
      Message.success(`已更新草稿（来源 ${result.source_cases} 条案例）`)
    }
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    aggregating.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.knowledge-manager {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.knowledge-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px;
}

.scope-ref {
  margin-left: 6px;
  color: var(--color-text-3);
  font-size: 11px;
}
</style>