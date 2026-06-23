<template>
  <div class="integrations-page">
    <a-card
      title="云账号接入"
      :bordered="false"
    >
      <template #extra>
        <a-space>
          <a-button
            :loading="loading"
            @click="loadAccounts"
          >
            刷新
          </a-button>
          <a-button
            type="primary"
            @click="openCreate"
          >
            新建账号
          </a-button>
        </a-space>
      </template>

      <a-form
        :model="filters"
        layout="inline"
        class="filter-form"
      >
        <a-form-item label="Provider">
          <a-select
            v-model="filters.provider"
            allow-clear
            placeholder="全部"
            style="width: 160px"
          >
            <a-option value="huawei_cloud">
              华为云
            </a-option>
            <a-option value="signoz">
              SigNoz
            </a-option>
            <a-option value="prometheus">
              Prometheus
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="状态">
          <a-select
            v-model="filters.enabled"
            allow-clear
            placeholder="全部"
            style="width: 120px"
          >
            <a-option :value="true">
              启用
            </a-option>
            <a-option :value="false">
              禁用
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item>
          <a-button
            type="primary"
            @click="onSearch"
          >
            查询
          </a-button>
        </a-form-item>
      </a-form>

      <a-table
        :columns="columns"
        :data="accounts"
        :loading="loading"
        row-key="account_id"
        :pagination="pagination"
        @page-change="onPageChange"
        @page-size-change="onPageSizeChange"
      >
        <template #provider="{ record }">
          <a-tag>{{ record.provider }}</a-tag>
        </template>
        <template #enabled="{ record }">
          <a-tag :color="record.enabled ? 'green' : 'gray'">
            {{ record.enabled ? '启用' : '禁用' }}
          </a-tag>
        </template>
        <template #capabilities="{ record }">
          <a-space wrap>
            <a-tag
              v-for="cap in record.capabilities || []"
              :key="cap"
              size="small"
            >
              {{ cap }}
            </a-tag>
            <span v-if="!record.capabilities?.length">—</span>
          </a-space>
        </template>
        <template #last_check="{ record }">
          {{ record.last_check_status || '—' }}
        </template>
        <template #actions="{ record }">
          <a-space>
            <a-button
              type="text"
              size="small"
              :loading="checkingId === record.account_id"
              @click="onCheck(record.account_id)"
            >
              连通性
            </a-button>
            <a-button
              type="text"
              size="small"
              @click="openEdit(record)"
            >
              编辑
            </a-button>
            <a-popconfirm
              content="确定删除该接入账号？"
              @ok="onDelete(record.account_id)"
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

    <a-modal
      v-model:visible="formVisible"
      :title="editingId ? '编辑接入账号' : '新建接入账号'"
      :ok-loading="saving"
      width="560px"
      @ok="onSubmit"
      @cancel="closeForm"
    >
      <a-form
        :model="form"
        layout="vertical"
      >
        <a-form-item
          label="名称"
          required
        >
          <a-input
            v-model="form.name"
            placeholder="显示名称"
          />
        </a-form-item>
        <a-form-item
          label="Provider"
          required
        >
          <a-select
            v-model="form.provider"
            :disabled="!!editingId"
          >
            <a-option value="huawei_cloud">
              huawei_cloud
            </a-option>
            <a-option value="signoz">
              signoz
            </a-option>
            <a-option value="prometheus">
              prometheus
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item
          label="认证方式"
          required
        >
          <a-select v-model="form.auth_type">
            <a-option value="none">
              none（联调占位）
            </a-option>
            <a-option value="ak_sk">
              ak_sk
            </a-option>
            <a-option value="api_token">
              api_token
            </a-option>
            <a-option value="agency">
              agency
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="区域">
          <a-input
            v-model="regionsText"
            placeholder="逗号分隔，如 cn-north-4"
          />
        </a-form-item>
        <a-form-item
          label="Project ID"
          :required="needsHuaweiProjectID"
        >
          <a-input
            v-model="form.project_id"
            :placeholder="needsHuaweiProjectID ? '华为云 project_id（必填）' : '华为云 project_id（可选）'"
          />
        </a-form-item>
        <a-form-item
          v-if="form.auth_type === 'ak_sk'"
          label="Access Key"
        >
          <a-input
            v-model="credential.access_key"
            placeholder="仅写入，不回显"
          />
        </a-form-item>
        <a-form-item
          v-if="form.auth_type === 'ak_sk'"
          label="Secret Key"
        >
          <a-input-password
            v-model="credential.secret_key"
            placeholder="仅写入，不回显"
          />
        </a-form-item>
        <a-form-item
          v-if="form.auth_type === 'api_token'"
          label="API Token"
        >
          <a-input-password
            v-model="credential.api_token"
            placeholder="仅写入，不回显"
          />
        </a-form-item>
        <a-form-item
          v-if="needsBaseURL"
          label="Base URL"
          required
        >
          <a-input
            v-model="credential.base_url"
            placeholder="http://127.0.0.1:9090"
          />
        </a-form-item>
        <a-form-item label="归属团队">
          <a-input v-model="form.owner_team" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea
            v-model="form.description"
            :auto-size="{ minRows: 2, maxRows: 4 }"
          />
        </a-form-item>
        <a-form-item label="启用">
          <a-switch v-model="form.enabled" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import {
  checkIntegrationAccount,
  createIntegrationAccount,
  deleteIntegrationAccount,
  listIntegrationAccounts,
  updateIntegrationAccount,
  type IntegrationAccount
} from '@/api/integration'
import { getApiError } from '@/api/request'

const loading = ref(false)
const saving = ref(false)
const checkingId = ref('')
const accounts = ref<IntegrationAccount[]>([])
const formVisible = ref(false)
const editingId = ref('')

const filters = reactive<{ provider?: string; enabled?: boolean }>({})
const pagination = reactive({ current: 1, pageSize: 20, total: 0, showTotal: true })

const form = reactive({
  name: '',
  provider: 'huawei_cloud',
  auth_type: 'none',
  project_id: '',
  owner_team: '',
  description: '',
  enabled: true
})
const regionsText = ref('')
const credential = reactive({ access_key: '', secret_key: '', api_token: '', base_url: '' })

const needsBaseURL = computed(
  () => form.provider === 'prometheus' && form.auth_type !== 'none'
)

const needsHuaweiAKSK = computed(
  () => form.provider === 'huawei_cloud' && form.auth_type === 'ak_sk'
)

const needsHuaweiProjectID = computed(() => needsHuaweiAKSK.value)

const columns = [
  { title: '账号 ID', dataIndex: 'account_id', width: 280, ellipsis: true },
  { title: '名称', dataIndex: 'name', width: 160 },
  { title: 'Provider', slotName: 'provider', width: 130 },
  { title: '状态', slotName: 'enabled', width: 90 },
  { title: '能力', slotName: 'capabilities' },
  { title: '最近检查', slotName: 'last_check', width: 100 },
  { title: '操作', slotName: 'actions', width: 220 }
]

function parseRegions(text: string): string[] {
  return text.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean)
}

function buildCredential(): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  const baseURL = credential.base_url.trim()
  if (baseURL) {
    out.base_url = baseURL
  }
  if (form.auth_type === 'ak_sk' && (credential.access_key || credential.secret_key)) {
    out.access_key = credential.access_key
    out.secret_key = credential.secret_key
  }
  if (form.auth_type === 'api_token' && credential.api_token) {
    out.api_token = credential.api_token
  }
  if (Object.keys(out).length === 0) {
    return undefined
  }
  return out
}

async function loadAccounts() {
  loading.value = true
  try {
    const res = await listIntegrationAccounts({
      page: pagination.current,
      page_size: pagination.pageSize,
      provider: filters.provider,
      enabled: filters.enabled
    })
    accounts.value = res.items
    pagination.total = res.total
  } catch (err) {
    Message.error(getApiError(err)?.message || '加载账号失败')
  } finally {
    loading.value = false
  }
}

function onSearch() {
  pagination.current = 1
  loadAccounts()
}

function onPageChange(page: number) {
  pagination.current = page
  loadAccounts()
}

function onPageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.current = 1
  loadAccounts()
}

function resetForm() {
  editingId.value = ''
  form.name = ''
  form.provider = 'huawei_cloud'
  form.auth_type = 'none'
  form.project_id = ''
  form.owner_team = ''
  form.description = ''
  form.enabled = true
  regionsText.value = ''
  credential.access_key = ''
  credential.secret_key = ''
  credential.api_token = ''
  credential.base_url = ''
}

function openCreate() {
  resetForm()
  formVisible.value = true
}

function openEdit(record: IntegrationAccount) {
  editingId.value = record.account_id
  form.name = record.name
  form.provider = record.provider
  form.auth_type = record.auth_type
  form.project_id = record.project_id || ''
  form.owner_team = record.owner_team || ''
  form.description = record.description || ''
  form.enabled = record.enabled
  regionsText.value = (record.regions || []).join(', ')
  credential.access_key = ''
  credential.secret_key = ''
  credential.api_token = ''
  credential.base_url = ''
  formVisible.value = true
}

function closeForm() {
  formVisible.value = false
}

async function onSubmit() {
  if (!form.name.trim()) {
    Message.warning('请填写名称')
    return
  }
  if (needsBaseURL.value && !credential.base_url.trim()) {
    Message.warning('Prometheus 非 none 认证必须填写 Base URL')
    return
  }
  if (needsHuaweiAKSK.value) {
    if (parseRegions(regionsText.value).length === 0) {
      Message.warning('华为云 ak_sk 账号必须填写至少一个区域')
      return
    }
    if (!form.project_id.trim()) {
      Message.warning('华为云 ak_sk 账号必须填写 Project ID')
      return
    }
    if (!credential.access_key.trim() || !credential.secret_key.trim()) {
      Message.warning('华为云 ak_sk 账号必须填写 Access Key 与 Secret Key')
      return
    }
  }
  saving.value = true
  try {
    const regions = parseRegions(regionsText.value)
    const cred = buildCredential()
    if (editingId.value) {
      await updateIntegrationAccount(editingId.value, {
        name: form.name,
        auth_type: form.auth_type,
        regions,
        project_id: form.project_id,
        owner_team: form.owner_team,
        description: form.description,
        enabled: form.enabled,
        ...(cred ? { credential: cred } : {})
      })
      Message.success('账号已更新')
    } else {
      await createIntegrationAccount({
        name: form.name,
        provider: form.provider,
        auth_type: form.auth_type,
        regions,
        project_id: form.project_id,
        owner_team: form.owner_team,
        description: form.description,
        enabled: form.enabled,
        ...(cred ? { credential: cred } : {})
      })
      Message.success('账号已创建')
    }
    formVisible.value = false
    await loadAccounts()
  } catch (err) {
    Message.error(getApiError(err)?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function onCheck(accountId: string) {
  checkingId.value = accountId
  try {
    const res = await checkIntegrationAccount(accountId)
    Message.success(`连通性 ${res.status}：${(res.capabilities || []).join(', ') || res.message}`)
    await loadAccounts()
  } catch (err) {
    Message.error(getApiError(err)?.message || '连通性检查失败')
  } finally {
    checkingId.value = ''
  }
}

async function onDelete(accountId: string) {
  try {
    await deleteIntegrationAccount(accountId)
    Message.success('已删除')
    await loadAccounts()
  } catch (err) {
    Message.error(getApiError(err)?.message || '删除失败')
  }
}

onMounted(loadAccounts)
</script>

<style scoped lang="scss">
.filter-form {
  margin-bottom: 16px;
}
</style>
