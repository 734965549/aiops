<template>
  <div class="ai-assistant">
    <a-tabs default-active-key="providers">
      <a-tab-pane
        key="providers"
        title="Provider 管理"
      >
        <a-row :gutter="16">
          <a-col :span="14">
            <a-card title="Provider 列表">
              <template #extra>
                <a-button
                  size="small"
                  :loading="loadingProviders"
                  @click="loadProviders"
                >
                  刷新
                </a-button>
              </template>
              <a-table
                :columns="providerColumns"
                :data="providers"
                :loading="loadingProviders"
                row-key="id"
                :pagination="false"
                @row-click="onSelectProvider"
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
                    @ok="onDeleteProvider(record.id)"
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
            </a-card>
          </a-col>
          <a-col :span="10">
            <a-card :title="form.id ? '编辑 Provider' : '新增 Provider'">
              <a-form
                :model="form"
                layout="vertical"
              >
                <a-form-item
                  label="ID"
                  required
                >
                  <a-input
                    v-model="form.id"
                    :disabled="!!editingId"
                    placeholder="demo-http-a"
                  />
                </a-form-item>
                <a-form-item
                  label="名称"
                  required
                >
                  <a-input
                    v-model="form.name"
                    placeholder="Demo HTTP Provider"
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
                    placeholder="http://127.0.0.1:9000"
                  />
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
                    当前已配置密钥
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
                    :loading="savingProvider"
                    @click="onSaveProvider"
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
      </a-tab-pane>

      <a-tab-pane
        key="invoke"
        title="工具调用"
      >
        <a-row :gutter="16">
          <a-col :span="12">
            <a-card title="调用参数">
              <a-form
                :model="invokeForm"
                layout="vertical"
              >
                <a-form-item
                  label="Provider"
                  required
                >
                  <a-select
                    v-model="invokeForm.provider_id"
                    placeholder="选择 Provider"
                    :options="providerOptions"
                    allow-search
                  />
                </a-form-item>
                <a-form-item
                  label="工具编码"
                  required
                >
                  <a-input
                    v-model="invokeForm.tool_code"
                    placeholder="alarm.analyze"
                  />
                </a-form-item>
                <a-form-item label="Resource">
                  <a-input
                    v-model="invokeForm.resource"
                    placeholder="alarm"
                  />
                </a-form-item>
                <a-form-item label="Action">
                  <a-input
                    v-model="invokeForm.action"
                    placeholder="analyze"
                  />
                </a-form-item>
                <a-form-item label="Payload (JSON)">
                  <a-textarea
                    v-model="payloadText"
                    :auto-size="{ minRows: 6, maxRows: 12 }"
                    placeholder="{&quot;alarm_id&quot;:&quot;a-123&quot;}"
                  />
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model="invokeForm.confirmed">
                    已人工确认（require_confirm 类工具必填）
                  </a-checkbox>
                </a-form-item>
                <a-button
                  type="primary"
                  :loading="invoking"
                  @click="onInvoke"
                >
                  调用
                </a-button>
              </a-form>
            </a-card>
          </a-col>
          <a-col :span="12">
            <a-card title="调用结果">
              <pre
                v-if="invokeResult"
                class="result-json"
              >{{ formattedResult }}</pre>
              <a-empty
                v-else
                description="提交后将在此展示 allowed / reason / data"
              />
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import {
  deleteProvider,
  invokeTool,
  listProviders,
  upsertProvider,
  type AIProvider,
  type InvokeToolResult
} from '@/api/ai'

const providers = ref<AIProvider[]>([])
const loadingProviders = ref(false)
const savingProvider = ref(false)
const invoking = ref(false)
const invokeResult = ref<InvokeToolResult | null>(null)
const editingId = ref('')
const formHasApiKey = ref(false)
const payloadText = ref('{}')

const providerColumns = [
  { title: 'ID', dataIndex: 'id', width: 140 },
  { title: '名称', dataIndex: 'name' },
  { title: '类型', dataIndex: 'type', width: 70 },
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
  type: 'a',
  base_url: '',
  api_key: '',
  timeout_ms: 30000,
  enabled: true,
  description: ''
})

const form = reactive(emptyForm())

const invokeForm = reactive({
  provider_id: '',
  tool_code: '',
  resource: '',
  action: '',
  confirmed: false
})

const providerOptions = computed(() =>
  providers.value.map((p) => ({ label: `${p.name} (${p.id})`, value: p.id }))
)

const formattedResult = computed(() =>
  invokeResult.value ? JSON.stringify(invokeResult.value, null, 2) : ''
)

async function loadProviders() {
  loadingProviders.value = true
  try {
    providers.value = await listProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    loadingProviders.value = false
  }
}

function onSelectProvider(record: Record<string, unknown>) {
  const p = record as unknown as AIProvider
  editingId.value = p.id
  formHasApiKey.value = !!p.has_api_key
  form.id = p.id
  form.name = p.name
  form.type = p.type
  form.base_url = p.base_url
  form.api_key = ''
  form.timeout_ms = p.timeout_ms ?? 30000
  form.enabled = p.enabled
  form.description = p.description ?? ''
}

function resetForm() {
  editingId.value = ''
  formHasApiKey.value = false
  Object.assign(form, emptyForm())
}

async function onSaveProvider() {
  if (!form.id.trim() || !form.name.trim() || !form.base_url.trim()) {
    Message.warning('请填写 ID、名称与 Base URL')
    return
  }
  savingProvider.value = true
  try {
    await upsertProvider({
      id: form.id.trim(),
      name: form.name.trim(),
      type: form.type,
      base_url: form.base_url.trim(),
      api_key: form.api_key.trim() || undefined,
      timeout_ms: form.timeout_ms,
      enabled: form.enabled,
      description: form.description.trim() || undefined
    })
    Message.success('保存成功')
    editingId.value = form.id.trim()
    await loadProviders()
  } finally {
    savingProvider.value = false
  }
}

async function onDeleteProvider(id: string) {
  try {
    await deleteProvider(id)
    Message.success('已删除')
    if (editingId.value === id) resetForm()
    await loadProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

function parsePayload(): Record<string, unknown> | undefined {
  const text = payloadText.value.trim()
  if (!text) return undefined
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
    throw new Error('payload must be a JSON object')
  } catch {
    Message.error('Payload 必须是合法 JSON 对象')
    throw new Error('invalid payload')
  }
}

async function onInvoke() {
  if (!invokeForm.provider_id || !invokeForm.tool_code.trim()) {
    Message.warning('请选择 Provider 并填写工具编码')
    return
  }
  let payload: Record<string, unknown> | undefined
  try {
    payload = parsePayload()
  } catch {
    return
  }
  invoking.value = true
  invokeResult.value = null
  try {
    invokeResult.value = await invokeTool({
      provider_id: invokeForm.provider_id,
      tool_code: invokeForm.tool_code.trim(),
      resource: invokeForm.resource.trim() || undefined,
      action: invokeForm.action.trim() || undefined,
      confirmed: invokeForm.confirmed,
      payload
    })
    if (invokeResult.value && !invokeResult.value.allowed) {
      Message.warning(invokeResult.value.reason || '工具调用被拒绝（allowed=false）')
    }
  } finally {
    invoking.value = false
  }
}

onMounted(loadProviders)
</script>

<style scoped>
.ai-assistant {
  min-height: 400px;
}
.result-json {
  margin: 0;
  padding: 12px;
  background: var(--color-fill-1);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.5;
  overflow: auto;
  max-height: 480px;
}
.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--color-text-3);
}
.text-muted {
  color: var(--color-text-3);
}
</style>
