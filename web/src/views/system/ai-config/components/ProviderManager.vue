<template>
  <a-row
    :gutter="16"
    class="equal-row"
  >
    <a-col
      :xs="24"
      :lg="14"
    >
      <a-card title="Provider 列表">
        <template #extra>
          <a-button
            size="small"
            :loading="loading"
            @click="loadProviders"
          >
            刷新
          </a-button>
        </template>
        <a-table
          :columns="providerColumns"
          :data="providers"
          :loading="loading"
          row-key="id"
          :pagination="false"
          @row-click="onSelect"
        >
          <template #enabled="{ record }">
            <a-tag :color="record.enabled ? 'green' : 'gray'">
              {{ record.enabled ? '启用' : '禁用' }}
            </a-tag>
          </template>
          <template #hasApiKey="{ record }">
            <span v-if="record.has_api_key">已配置</span>
            <span
              v-else
              class="text-muted"
            >未配置</span>
          </template>
          <template #actions="{ record }">
            <a-popconfirm
              content="确认删除该 Provider？"
              @ok="onDelete(record.id)"
            >
              <a-button
                type="text"
                status="danger"
                size="mini"
              >
                删除
              </a-button>
            </a-popconfirm>
          </template>
        </a-table>
        <div class="provider-hint">
          告警分析与 AI 助手都动态选择启用中的类型 B（平台直连模型）；页面启停或换模型无需重启。
        </div>
      </a-card>
    </a-col>
    <a-col
      :xs="24"
      :lg="10"
    >
      <a-card :title="form.id ? '编辑 Provider' : '新增 Provider'">
        <a-form
          :model="form"
          layout="vertical"
          size="small"
        >
          <a-form-item
            label="ID"
            required
          >
            <a-input
              v-model="form.id"
              :disabled="!!editingId"
              placeholder="ark"
            />
          </a-form-item>
          <a-form-item
            label="名称"
            required
          >
            <a-input
              v-model="form.name"
              placeholder="火山方舟"
            />
          </a-form-item>
          <a-form-item
            label="类型"
            required
          >
            <a-select
              v-model="form.type"
              :options="typeOptions"
            />
          </a-form-item>
          <a-form-item
            label="Base URL"
            required
          >
            <a-input
              v-model="form.base_url"
              placeholder="https://ark.cn-beijing.volces.com/api/v3"
            />
          </a-form-item>
          <a-form-item
            v-if="form.type === 'b'"
            label="模型 / Endpoint"
            required
          >
            <a-input
              v-model="form.model"
              placeholder="ep-xxxxxxxx（方舟推理接入点 ID）"
            />
            <div class="field-hint">
              类型 B 由平台直连 chat/completions；保存后立即生效
            </div>
          </a-form-item>
          <a-form-item label="API Key">
            <a-input-password
              v-model="form.api_key"
              placeholder="新增必填；编辑留空则保留原密钥"
            />
            <div
              v-if="editingId && formHasApiKey"
              class="field-hint"
            >
              当前已配置密钥（只显示掩码，不回显明文）
            </div>
          </a-form-item>
          <a-form-item label="超时 (ms)">
            <a-input-number
              v-model="form.timeout_ms"
              :min="1000"
              :step="1000"
              style="width: 100%"
            />
          </a-form-item>
          <a-form-item label="启用">
            <a-switch v-model="form.enabled" />
          </a-form-item>
          <a-form-item label="描述">
            <a-textarea
              v-model="form.description"
              :auto-size="{ minRows: 2 }"
            />
          </a-form-item>
          <a-space>
            <a-button
              type="primary"
              :loading="saving"
              @click="onSave"
            >
              保存
            </a-button>
            <a-button @click="resetForm">
              重置
            </a-button>
          </a-space>
        </a-form>
      </a-card>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import {
  deleteProvider,
  listProviders,
  upsertProvider,
  type AIProvider
} from '@/api/ai'

const providers = ref<AIProvider[]>([])
const loading = ref(false)
const saving = ref(false)
const editingId = ref('')
const formHasApiKey = ref(false)

const providerColumns = [
  { title: 'ID', dataIndex: 'id', width: 140 },
  { title: '名称', dataIndex: 'name' },
  { title: '类型', dataIndex: 'type', width: 70 },
  { title: '模型', dataIndex: 'model', width: 150 },
  { title: '密钥', slotName: 'hasApiKey', width: 80 },
  { title: '状态', slotName: 'enabled', width: 80 },
  { title: '操作', slotName: 'actions', width: 80 }
]

const typeOptions = [
  { label: 'A - HTTP API Key', value: 'a' },
  { label: 'B - OpenAI 兼容', value: 'b' },
  { label: 'C - 内部服务', value: 'c' }
]

const emptyForm = () => ({
  id: '',
  name: '',
  type: 'b',
  base_url: '',
  model: '',
  api_key: '',
  timeout_ms: 300000,
  enabled: true,
  description: ''
})

const form = reactive(emptyForm())

async function loadProviders() {
  loading.value = true
  try {
    providers.value = await listProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    loading.value = false
  }
}

function onSelect(record: Record<string, unknown>) {
  const provider = record as unknown as AIProvider
  editingId.value = provider.id
  formHasApiKey.value = !!provider.has_api_key
  form.id = provider.id
  form.name = provider.name
  form.type = provider.type
  form.base_url = provider.base_url
  form.model = provider.model ?? ''
  form.api_key = ''
  form.timeout_ms = provider.timeout_ms ?? 300000
  form.enabled = provider.enabled
  form.description = provider.description ?? ''
}

function resetForm() {
  editingId.value = ''
  formHasApiKey.value = false
  Object.assign(form, emptyForm())
}

async function onSave() {
  if (!form.id.trim() || !form.name.trim() || !form.base_url.trim()) {
    Message.warning('请填写 ID、名称与 Base URL')
    return
  }
  if (form.type === 'b' && !form.model.trim()) {
    Message.warning('类型 B 必须填写模型 / Endpoint')
    return
  }
  saving.value = true
  try {
    await upsertProvider({
      id: form.id.trim(),
      name: form.name.trim(),
      type: form.type,
      base_url: form.base_url.trim(),
      model: form.model.trim() || undefined,
      api_key: form.api_key.trim() || undefined,
      timeout_ms: form.timeout_ms,
      enabled: form.enabled,
      description: form.description.trim() || undefined
    })
    Message.success('保存成功')
    editingId.value = form.id.trim()
    await loadProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    saving.value = false
  }
}

async function onDelete(id: string) {
  try {
    await deleteProvider(id)
    Message.success('已删除')
    if (editingId.value === id) resetForm()
    await loadProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

onMounted(loadProviders)
</script>

<style scoped>
.provider-hint {
  margin-top: 10px;
  color: var(--color-text-3);
  font-size: 11px;
  line-height: 1.6;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--color-text-3);
}

.text-muted {
  color: var(--color-text-3);
}

.equal-row {
  flex: 1;
  min-height: 0;
}

.equal-row > .arco-col > .arco-card {
  min-height: 0;
}

:deep(.arco-card-body) {
  overflow: auto;
}

:deep(.arco-form-item) {
  margin-bottom: 10px;
}

:deep(.arco-form-item-label-col) {
  padding-bottom: 3px;
}
</style>