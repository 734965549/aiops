<template>
  <section class="source-pane source-form-pane">
    <header class="source-pane-head">
      <div class="source-pane-heading">
        <span class="source-pane-title">
          {{ editing ? '编辑接入源' : '新建接入源' }}
        </span>
        <span class="source-pane-hint">
          {{ editing ? source?.id : '填写基础信息后创建' }}
        </span>
      </div>
      <a-button
        v-if="editing"
        size="mini"
        @click="emit('create')"
      >
        切换为新建
      </a-button>
    </header>

    <div class="source-pane-body source-form-scroll">
      <a-form
        :model="form"
        layout="vertical"
        class="source-form"
      >
        <div class="source-form-grid">
          <a-form-item
            label="ID"
            required
          >
            <a-input
              v-model="form.id"
              :disabled="editing"
              placeholder="prod-ces"
            />
            <div class="field-hint">
              接入源标识，创建后不可修改
            </div>
          </a-form-item>
          <a-form-item
            label="名称"
            required
          >
            <a-input
              v-model="form.name"
              placeholder="生产 CES 告警"
            />
          </a-form-item>
          <a-form-item
            label="接入方式"
            class="source-form-full"
          >
            <a-radio-group
              v-model="form.delivery_mode"
              type="button"
              :disabled="editing"
              @change="onDeliveryModeChange"
            >
              <a-radio value="webhook">
                Webhook 推送
              </a-radio>
              <a-radio value="pull">
                CES 拉取
              </a-radio>
            </a-radio-group>
            <div class="field-hint">
              {{ deliveryModeHint }}
            </div>
          </a-form-item>
          <a-form-item label="类型">
            <a-select
              v-if="form.delivery_mode === 'webhook'"
              v-model="form.type"
              :options="webhookTypeOptions"
            />
            <a-input
              v-else
              model-value="huawei_ces"
              disabled
            />
          </a-form-item>
          <a-form-item label="环境">
            <a-input
              v-model="form.environment"
              placeholder="prod"
            />
          </a-form-item>
          <a-form-item label="业务线">
            <a-input
              v-model="form.business_line"
              placeholder="payment"
            />
          </a-form-item>
          <a-form-item label="状态">
            <div class="source-switch">
              <a-switch v-model="form.enabled" />
              <span>
                {{ form.enabled ? '启用接收' : '暂停接收' }}
              </span>
            </div>
          </a-form-item>
          <a-form-item
            v-if="form.delivery_mode === 'webhook'"
            :label="editing ? '密钥（留空保留原值）' : 'Webhook 密钥'"
            :required="!editing"
            class="source-form-full"
          >
            <a-input-password
              v-model="form.secret"
              placeholder="X-AIOPS-Webhook-Token"
            />
            <div
              v-if="editing && editingSecretMasked"
              class="field-hint"
            >
              当前已配置密钥：{{ editingSecretMasked }}
            </div>
          </a-form-item>
          <template v-else>
            <a-form-item
              label="华为云账号"
              required
              class="source-form-full"
            >
              <a-select
                v-model="form.account_id"
                :options="accountOptions"
                :loading="loadingAccounts"
                :disabled="editing"
                placeholder="选择已启用且声明 alerts 能力的 huawei_cloud 账号"
                allow-search
              />
              <div
                v-if="accountError"
                class="field-hint field-error"
              >
                {{ accountError }}
              </div>
              <div
                v-else-if="editing"
                class="field-hint"
              >
                拉取源绑定账号创建后不可修改
              </div>
              <div
                v-else
                class="field-hint"
              >
                仅列出已启用、provider=huawei_cloud 且有 app:integrations:read 权限的账号
              </div>
            </a-form-item>
            <a-form-item label="轮询间隔（秒）">
              <a-input-number
                v-model="form.pull_interval_seconds"
                :min="0"
                :max="86400"
                :step="60"
                style="width: 100%"
              />
              <div class="field-hint">
                0 表示仅手动同步；定时拉取下限 60 秒
              </div>
            </a-form-item>
            <a-form-item label="区域筛选">
              <a-select
                v-model="form.region_filter"
                :options="regionOptions"
                mode="multiple"
                allow-clear
                placeholder="默认使用账号全部区域"
              />
              <div class="field-hint">
                留空使用账号全部区域；只允许选择账号已配置区域
              </div>
            </a-form-item>
          </template>
          <a-form-item
            label="备注"
            class="source-form-full"
          >
            <a-textarea
              v-model="form.description"
              :auto-size="{ minRows: 2, maxRows: 3 }"
            />
          </a-form-item>
        </div>

        <div
          v-if="form.delivery_mode === 'webhook'"
          class="source-endpoint"
        >
          <div class="source-endpoint-label">
            {{ editing ? '当前 Webhook 地址' : 'Webhook 地址预览' }}
          </div>
          <div
            v-if="form.id.trim()"
            class="source-endpoint-url"
          >
            <a-typography-text
              copyable
              :copy-text="webhookUrl()"
            >
              {{ webhookUrl() }}
            </a-typography-text>
          </div>
          <div
            v-else
            class="field-hint"
          >
            填写 ID 后自动生成
          </div>
          <div class="field-hint">
            请求头需携带 X-AIOPS-Webhook-Token；密钥仅显示掩码
          </div>
        </div>
        <div
          v-else
          class="source-endpoint"
        >
          <div class="source-endpoint-label">
            CES 拉取说明
          </div>
          <div class="field-hint">
            平台按区域和项目分页读取 CES V2 告警记录；手工同步最多等待约 20 秒。
            账号凭据在每轮拉取前重新解析，AK/SK 不会进入日志或审计。
          </div>
        </div>
      </a-form>
    </div>

    <footer class="source-pane-foot">
      <a-button
        size="small"
        @click="emit('create')"
      >
        重置
      </a-button>
      <div class="source-foot-right">
        <a-button
          size="small"
          @click="emit('close')"
        >
          关闭
        </a-button>
        <a-button
          type="primary"
          size="small"
          :loading="saving"
          @click="onSave"
        >
          {{ editing ? '保存修改' : '创建接入源' }}
        </a-button>
      </div>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import * as alertApi from '@/api/alert'
import type { AlertDeliveryMode, AlertSource } from '@/api/alert'
import { listIntegrationAccounts } from '@/api/integration'
import type { IntegrationAccount } from '@/api/integration'
import { getApiError } from '@/api/request'

const props = defineProps<{ source: AlertSource | null }>()
const emit = defineEmits<{
  (e: 'saved'): void
  (e: 'create'): void
  (e: 'close'): void
}>()

const editing = computed(() => !!props.source)
const editingSecretMasked = computed(() => props.source?.secret_masked || '')

const accounts = ref<IntegrationAccount[]>([])
const loadingAccounts = ref(false)
const accountError = ref('')
const saving = ref(false)

const form = reactive({
  id: '',
  name: '',
  type: 'prometheus_alertmanager',
  enabled: true,
  environment: '',
  business_line: '',
  description: '',
  delivery_mode: 'webhook' as AlertDeliveryMode,
  secret: '',
  account_id: '',
  pull_interval_seconds: 300,
  region_filter: [] as string[]
})

const webhookTypeOptions = [
  { label: 'Prometheus Alertmanager', value: 'prometheus_alertmanager' },
  { label: '通用 Webhook', value: 'custom_webhook' },
  { label: '华为云 CES（Webhook）', value: 'huawei_ces' }
]

const accountOptions = computed(() =>
  accounts.value.map((account) => ({
    label: `${account.name} (${account.account_id})`,
    value: account.account_id
  }))
)

const selectedAccount = computed(() =>
  accounts.value.find((account) => account.account_id === form.account_id)
)

const regionOptions = computed(() =>
  (selectedAccount.value?.regions ?? []).map((region) => ({
    label: region,
    value: region
  }))
)

const deliveryModeHint = computed(() => {
  if (editing.value) {
    return '接入方式创建后不可修改；更换方式请新建接入源'
  }
  if (form.delivery_mode === 'pull') {
    return '平台主动轮询 CES 告警记录，需要绑定华为云账号'
  }
  return '外部系统通过共享密钥推送告警，平台不主动访问云 API'
})

function fillFromSource() {
  const source = props.source
  if (!source) return
  const isPull = source.delivery_mode === 'pull'
  form.id = source.id
  form.name = source.name
  form.type = source.type
  form.enabled = source.enabled
  form.environment = source.environment || ''
  form.business_line = source.business_line || ''
  form.description = source.description || ''
  form.delivery_mode = isPull ? 'pull' : 'webhook'
  form.secret = ''
  form.account_id = source.account_id || ''
  form.pull_interval_seconds = source.pull_config?.pull_interval_seconds ?? 300
  form.region_filter = [...(source.pull_config?.region_filter ?? [])]
}

function onDeliveryModeChange(value: string | number | boolean) {
  if (value === 'pull') {
    form.type = 'huawei_ces'
    form.secret = ''
  } else if (form.type === 'huawei_ces') {
    form.type = 'prometheus_alertmanager'
  }
}

function webhookUrl() {
  const base = import.meta.env.VITE_API_BASE || window.location.origin
  const ingestPath =
    form.type === 'custom_webhook'
      ? `/api/alerts/ingest/webhook/${form.id}`
      : `/api/alerts/ingest/alertmanager/${form.id}`
  return `${base.replace(/\/$/, '')}${ingestPath}`
}

async function loadAccounts() {
  loadingAccounts.value = true
  accountError.value = ''
  try {
    const res = await listIntegrationAccounts({
      provider: 'huawei_cloud',
      enabled: true,
      page_size: 100
    })
    accounts.value = res.items
  } catch (err) {
    accounts.value = []
    const apiErr = getApiError(err)
    accountError.value =
      apiErr?.status === 403
        ? '当前账号缺少 app:integrations:read 权限，无法选择拉取账号'
        : apiErr?.message || '加载集成账号失败'
  } finally {
    loadingAccounts.value = false
  }
}

function validateForm(): string {
  if (!form.id.trim() || !form.name.trim()) {
    return '请填写 ID 与名称'
  }
  if (form.delivery_mode === 'webhook') {
    if (!editing.value && !form.secret.trim()) {
      return '请填写 Webhook 密钥'
    }
    return ''
  }
  if (!form.account_id) {
    return '请选择华为云账号'
  }
  const interval = form.pull_interval_seconds
  if (interval !== 0 && (interval < 60 || interval > 86400)) {
    return '轮询间隔需为 0（仅手动）或 60..86400 秒'
  }
  return ''
}

async function onSave() {
  const invalid = validateForm()
  if (invalid) {
    Message.warning(invalid)
    return
  }
  saving.value = true
  try {
    if (props.source) {
      await alertApi.updateAlertSource(props.source.id, {
        name: form.name,
        type: form.type,
        enabled: form.enabled,
        secret: form.secret || undefined,
        environment: form.environment || undefined,
        business_line: form.business_line || undefined,
        description: form.description || undefined,
        pull_config:
          form.delivery_mode === 'pull'
            ? {
                pull_interval_seconds: form.pull_interval_seconds,
                severity_map: props.source.pull_config?.severity_map,
                region_filter: form.region_filter
              }
            : undefined
      })
      Message.success('接入源已更新')
    } else {
      await alertApi.createAlertSource({
        id: form.id.trim(),
        name: form.name.trim(),
        type: form.type,
        enabled: form.enabled,
        secret: form.delivery_mode === 'webhook' ? form.secret.trim() : undefined,
        environment: form.environment || undefined,
        business_line: form.business_line || undefined,
        description: form.description || undefined,
        delivery_mode: form.delivery_mode,
        account_id: form.delivery_mode === 'pull' ? form.account_id : undefined,
        pull_config:
          form.delivery_mode === 'pull'
            ? {
                pull_interval_seconds: form.pull_interval_seconds,
                region_filter: form.region_filter
              }
            : undefined
      })
      Message.success('接入源已创建')
    }
    emit('saved')
  } catch (err) {
    Message.error(getApiError(err)?.message || '保存接入源失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fillFromSource()
  loadAccounts()
})
</script>

<style scoped lang="scss">
.source-pane {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
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

.source-pane-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.source-form-scroll {
  padding: 16px 18px;
}

.source-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 14px;
}

.source-form-grid :deep(.arco-form-item) {
  margin-bottom: 14px;
}

.source-form-grid > :deep(.source-form-full) {
  grid-column: 1 / -1;
}

.source-switch {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 32px;
  color: var(--aiops-text-2);
  font-size: 13px;
}

.field-hint {
  margin-top: 4px;
  font-size: 12px;
  color: var(--aiops-text-3);
}

.field-error {
  color: rgb(var(--danger-6));
}

.source-endpoint {
  margin-top: 4px;
  padding: 12px 14px;
  border: 1px solid var(--aiops-border-soft);
  border-radius: var(--radius-md);
  background: rgba(255, 255, 252, 0.72);
}

.source-endpoint-label {
  margin-bottom: 6px;
  color: var(--aiops-text-2);
  font-size: 12px;
  font-weight: 600;
}

.source-endpoint-url {
  overflow-wrap: anywhere;
  font-size: 12px;
}

.source-pane-foot {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 18px;
  border-top: 1px solid var(--aiops-border-soft);
  background: rgba(235, 234, 227, 0.55);
}

.source-foot-right {
  display: flex;
  gap: 8px;
}

@media (max-width: 860px) {
  .source-form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
