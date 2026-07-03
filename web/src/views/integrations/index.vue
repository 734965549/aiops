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
              :loading="syncingId === record.account_id"
              @click="onSyncAssets(record.account_id)"
            >
              同步资源
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

        <template v-if="isHuaweiCloud">
          <a-divider orientation="left">
            CES 资源同步配置
          </a-divider>
          <a-alert
            type="info"
            class="sync-config-alert"
          >
            配置将写入 extra_config；AK/SK、Token、密码仍只能通过凭据字段写入。
          </a-alert>
          <a-form-item label="同步模式">
            <a-select v-model="huaweiExtra.sync_mode">
              <a-option value="ces">
                CES 资源同步（推荐）
              </a-option>
              <a-option value="hybrid">
                混合同步
              </a-option>
              <a-option value="native">
                原生云资产同步（兼容旧路径）
              </a-option>
            </a-select>
          </a-form-item>
          <a-alert
            v-if="huaweiExtra.sync_mode === 'hybrid'"
            type="warning"
            class="sync-config-alert"
          >
            混合同步会先按指定 CES 资源分组发现资源，再按权限补充已支持类型详情；EVS/VPC 详情增强尚未支持，增强失败不影响基础资源入库。
          </a-alert>
          <a-alert
            v-if="huaweiExtra.sync_mode === 'native'"
            type="warning"
            class="sync-config-alert"
          >
            原生云资产同步仅兼容旧路径，不保证与 CES 控制台全部资源数量一致。
          </a-alert>
          <a-form-item label="资源组名称">
            <a-input
              v-model="huaweiExtra.resource_group_name"
              placeholder="默认 全部资源"
            />
          </a-form-item>
          <a-form-item label="资源组 ID">
            <a-input
              v-model="huaweiExtra.resource_group_id"
              placeholder="可选；填写后优先于资源组名称"
            />
          </a-form-item>
          <a-form-item label="企业项目 ID">
            <a-input
              v-model="huaweiExtra.enterprise_project_id"
              placeholder="可选；如 all_granted_eps"
            />
          </a-form-item>
          <a-form-item label="单次同步上限">
            <a-input-number
              v-model="huaweiExtra.max_resources"
              :min="1"
              :max="20000"
              :precision="0"
              placeholder="默认 20000"
              style="width: 100%"
            />
          </a-form-item>
          <a-form-item label="Region Project 映射">
            <a-textarea
              v-model="regionProjectsText"
              placeholder="每行一个：cn-south-1=project_id"
              :auto-size="{ minRows: 2, maxRows: 5 }"
            />
          </a-form-item>
          <a-alert
            v-if="showRegionProjectFallback"
            type="warning"
            class="sync-config-alert"
          >
            多区域未配置完整 region_projects 时，未配置区域会回落使用账号 Project ID：{{ missingRegionProjects.join(', ') }}。
          </a-alert>
        </template>

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
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import {
  checkIntegrationAccount,
  createIntegrationAccount,
  deleteIntegrationAccount,
  listIntegrationAccounts,
  updateIntegrationAccount,
  type HuaweiCloudExtraConfig,
  type HuaweiCloudSyncMode,
  type IntegrationAccount
} from '@/api/integration'
import {
  getSyncBatchNotice,
  isAssetSyncInProgressError,
  pollSyncBatch,
  SyncStillRunningError,
  triggerAssetSync
} from '@/api/asset'
import { getApiError } from '@/api/request'
import {
  extractUnknownExtraConfig,
  formatRegionProjects,
  mergeHuaweiExtraConfig,
  parseRegionProjects,
  parseRegions
} from './composables/huaweiConfig'

const loading = ref(false)
const saving = ref(false)
const checkingId = ref('')
const syncingId = ref('')
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
const regionProjectsText = ref('')
const credential = reactive({ access_key: '', secret_key: '', api_token: '', base_url: '' })
const huaweiExtra = reactive({
  sync_mode: 'ces' as HuaweiCloudSyncMode,
  resource_group_name: '全部资源',
  resource_group_id: '',
  enterprise_project_id: '',
  max_resources: 20000
})
const preservedExtraConfig = ref<Record<string, unknown>>({})

const isHuaweiCloud = computed(() => form.provider === 'huawei_cloud')

const needsBaseURL = computed(
  () => form.provider === 'prometheus' && form.auth_type !== 'none'
)

const needsHuaweiAKSK = computed(
  () => form.provider === 'huawei_cloud' && form.auth_type === 'ak_sk'
)

const needsHuaweiProjectID = computed(() => needsHuaweiAKSK.value)

const parsedRegionProjects = computed(() => parseRegionProjects(regionProjectsText.value).items)

const missingRegionProjects = computed(() => {
  const configured = new Set(parsedRegionProjects.value.map((item) => item.region.toLowerCase()))
  return parseRegions(regionsText.value).filter((region) => !configured.has(region.toLowerCase()))
})

const showRegionProjectFallback = computed(() => {
  return isHuaweiCloud.value && parseRegions(regionsText.value).length > 1 && missingRegionProjects.value.length > 0
})

const columns = [
  { title: '账号 ID', dataIndex: 'account_id', width: 280, ellipsis: true },
  { title: '名称', dataIndex: 'name', width: 160 },
  { title: 'Provider', slotName: 'provider', width: 130 },
  { title: '状态', slotName: 'enabled', width: 90 },
  { title: '能力', slotName: 'capabilities' },
  { title: '最近检查', slotName: 'last_check', width: 100 },
  { title: '操作', slotName: 'actions', width: 300 }
]

function resetHuaweiExtra() {
  huaweiExtra.sync_mode = 'ces'
  huaweiExtra.resource_group_name = '全部资源'
  huaweiExtra.resource_group_id = ''
  huaweiExtra.enterprise_project_id = ''
  huaweiExtra.max_resources = 20000
  regionProjectsText.value = ''
  preservedExtraConfig.value = {}
}

function readHuaweiExtraConfig(record: IntegrationAccount) {
  resetHuaweiExtra()
  const extra = record.extra_config
  if (!extra || typeof extra !== 'object' || Array.isArray(extra)) return
  const config = extra as HuaweiCloudExtraConfig
  preservedExtraConfig.value = extractUnknownExtraConfig(config as Record<string, unknown>)
  if (config.sync_mode) huaweiExtra.sync_mode = config.sync_mode
  if (typeof config.resource_group_name === 'string') huaweiExtra.resource_group_name = config.resource_group_name
  if (typeof config.resource_group_id === 'string') huaweiExtra.resource_group_id = config.resource_group_id
  if (typeof config.enterprise_project_id === 'string') huaweiExtra.enterprise_project_id = config.enterprise_project_id
  if (typeof config.max_resources === 'number') huaweiExtra.max_resources = config.max_resources
  regionProjectsText.value = formatRegionProjects(config.region_projects)
}

function buildHuaweiExtraConfig(): HuaweiCloudExtraConfig | undefined {
  if (!isHuaweiCloud.value) return undefined
  const regionProjects = parseRegionProjects(regionProjectsText.value).items
  return mergeHuaweiExtraConfig(preservedExtraConfig.value, {
    sync_mode: huaweiExtra.sync_mode,
    resource_group_name: huaweiExtra.resource_group_name,
    resource_group_id: huaweiExtra.resource_group_id,
    enterprise_project_id: huaweiExtra.enterprise_project_id,
    max_resources: huaweiExtra.max_resources,
    region_projects: regionProjects
  })
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
  resetHuaweiExtra()
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
  readHuaweiExtraConfig(record)
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
    if (!editingId.value && (!credential.access_key.trim() || !credential.secret_key.trim())) {
      Message.warning('新建华为云 ak_sk 账号必须填写 Access Key 与 Secret Key')
      return
    }
  }
  if (isHuaweiCloud.value) {
    if (!huaweiExtra.max_resources || huaweiExtra.max_resources < 1 || huaweiExtra.max_resources > 20000) {
      Message.warning('单次同步上限必须在 1 到 20000 之间')
      return
    }
    const parsedRegionProjectResult = parseRegionProjects(regionProjectsText.value)
    if (parsedRegionProjectResult.errors.length > 0) {
      Message.warning(parsedRegionProjectResult.errors[0])
      return
    }
  }
  saving.value = true
  try {
    const regions = parseRegions(regionsText.value)
    const cred = buildCredential()
    const extraConfig = buildHuaweiExtraConfig()
    if (editingId.value) {
      await updateIntegrationAccount(editingId.value, {
        name: form.name,
        auth_type: form.auth_type,
        regions,
        project_id: form.project_id,
        owner_team: form.owner_team,
        description: form.description,
        enabled: form.enabled,
        ...(extraConfig ? { extra_config: extraConfig } : {}),
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
        ...(extraConfig ? { extra_config: extraConfig } : {}),
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

// 组件卸载时取消进行中的同步轮询，避免泄漏与对已销毁组件的 Message 调用。
let syncPollingStopped = false
onBeforeUnmount(() => {
  syncPollingStopped = true
})

async function onSyncAssets(accountId: string) {
  syncingId.value = accountId
  try {
    // 触发同步：后端立即返回 running 批次，随后轮询到终态。
    const running = await triggerAssetSync(accountId)
    const batch = await pollSyncBatch(running.batch_id, { shouldStop: () => syncPollingStopped })
    const notice = getSyncBatchNotice(batch)
    Message[notice.type](notice.content)
  } catch (err) {
    if (isAssetSyncInProgressError(err)) {
      Message.warning('该账号正在同步，请稍后重试')
      return
    }
    if (err instanceof SyncStillRunningError) {
      Message.info(err.message)
      return
    }
    if (err instanceof Error && err.message === 'polling cancelled') {
      // 组件卸载取消，不提示
      return
    }
    Message.error(getApiError(err)?.message || '资源同步失败')
  } finally {
    syncingId.value = ''
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

.sync-config-alert {
  margin-bottom: 16px;
}
</style>
