<template>
  <div class="ldap-import-page">
    <a-card
      title="LDAP / AD 域账号导入"
      :bordered="false"
    >
      <a-alert
        type="info"
        class="tip"
      >
        填写目录连接信息并连通后，可在左侧浏览组织单元、右侧勾选用户导入平台。
        「身份源 ID」须与后续登录配置中的 provider id 一致，导入后用户方可域登录。
        服务账号密码仅保存在短期浏览会话中（约 30 分钟），不会写入前端本地存储。
      </a-alert>

      <a-form
        :model="connForm"
        layout="vertical"
        class="conn-form"
      >
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item
              label="身份源 ID"
              required
            >
              <a-input
                v-model="connForm.provider_id"
                placeholder="corp-ad"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="类型">
              <a-select
                v-model="connForm.type"
                :disabled="connected"
              >
                <a-option value="ldap">
                  LDAP
                </a-option>
                <a-option value="ad">
                  Active Directory
                </a-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item
              label="Server URL"
              required
            >
              <a-input
                v-model="connForm.server_url"
                placeholder="ldaps://ad.example.com:636"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Bind DN（服务账号）">
              <a-input
                v-model="connForm.bind_dn"
                placeholder="CN=svc-aiops,OU=Service,DC=corp,DC=local"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Bind 密码">
              <a-input-password
                v-model="connForm.bind_password"
                placeholder="服务账号密码"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item
              label="Base DN"
              required
            >
              <a-input
                v-model="connForm.base_dn"
                placeholder="DC=corp,DC=local"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="传输安全">
              <a-space>
                <a-checkbox
                  v-model="connForm.start_tls"
                  :disabled="connected"
                >
                  StartTLS
                </a-checkbox>
                <a-checkbox
                  v-model="connForm.insecure_skip_verify"
                  :disabled="connected"
                >
                  跳过证书校验（仅 dev）
                </a-checkbox>
              </a-space>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Subject 属性（attr_subject）">
              <a-input
                v-model="connForm.attr_subject"
                placeholder="entryUUID / objectGUID，须与登录配置一致"
                :disabled="connected"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-space>
          <a-button
            v-if="!connected"
            type="primary"
            :loading="connecting"
            @click="onConnect"
          >
            连接目录
          </a-button>
          <a-button
            v-else
            status="warning"
            @click="onDisconnect"
          >
            断开连接
          </a-button>
          <span
            v-if="connected"
            class="session-meta"
          >
            已连接 · Base DN: {{ sessionBaseDn }} · 会话 {{ sessionExpiresIn }}s 内有效
          </span>
        </a-space>
      </a-form>
    </a-card>

    <a-row
      v-if="connected"
      :gutter="16"
      class="browse-panel"
    >
      <a-col :span="8">
        <a-card
          title="组织单元"
          :bordered="false"
        >
          <a-spin :loading="treeLoading">
            <a-tree
              v-if="treeData.length"
              :data="treeData"
              :load-more="loadOrgChildren"
              block-node
              @select="onOrgSelect"
            />
            <a-empty
              v-else
              description="暂无组织数据"
            />
          </a-spin>
        </a-card>
      </a-col>

      <a-col :span="16">
        <a-card
          :bordered="false"
        >
          <template #title>
            <span>目录用户</span>
            <span
              v-if="selectedOrgDn"
              class="org-label"
            > — {{ selectedOrgName }}</span>
          </template>
          <template #extra>
            <a-space>
              <a-select
                v-model="selectedRoleCodes"
                multiple
                allow-clear
                placeholder="导入后绑定角色"
                style="min-width: 220px"
              >
                <a-option
                  v-for="role in roles"
                  :key="role.code"
                  :value="role.code"
                >
                  {{ role.name }} ({{ role.code }})
                </a-option>
              </a-select>
              <a-button
                :disabled="!selectedOrgDn"
                :loading="usersLoading"
                @click="reloadUsers"
              >
                刷新
              </a-button>
              <a-button
                type="primary"
                :disabled="selectedSubjects.length === 0"
                :loading="importing"
                @click="onImportSelected"
              >
                导入选中 ({{ selectedSubjects.length }})
              </a-button>
              <a-button
                :disabled="!selectedOrgDn"
                :loading="importing"
                @click="onImportAll"
              >
                导入当前 OU 全部
              </a-button>
            </a-space>
          </template>

          <a-table
            v-model:selected-keys="selectedSubjects"
            row-key="external_subject"
            :columns="userColumns"
            :data="directoryUsers"
            :loading="usersLoading"
            :row-selection="rowSelection"
            :pagination="false"
            size="small"
          >
            <template #imported="{ record }">
              <a-tag :color="record.imported ? 'green' : 'gray'">
                {{ record.imported ? '已导入' : '未导入' }}
              </a-tag>
            </template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>

    <a-modal
      v-model:visible="resultVisible"
      title="导入结果"
      :footer="false"
      width="640px"
    >
      <a-descriptions
        :column="3"
        bordered
        size="small"
      >
        <a-descriptions-item label="成功">
          {{ importResult?.created ?? 0 }}
        </a-descriptions-item>
        <a-descriptions-item label="跳过">
          {{ importResult?.skipped ?? 0 }}
        </a-descriptions-item>
        <a-descriptions-item label="失败">
          {{ importResult?.failed ?? 0 }}
        </a-descriptions-item>
      </a-descriptions>
      <a-table
        class="result-table"
        size="mini"
        :columns="resultColumns"
        :data="importResult?.users ?? []"
        :pagination="false"
      />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import type { TableColumnData, TableRowSelection, TreeNodeData } from '@arco-design/web-vue'
import {
  browseLDAPOrganizations,
  closeLDAPSession,
  connectLDAPSession,
  fetchRoles,
  importLDAPUsers,
  previewLDAPUsers,
  type ImportLDAPUsersResult,
  type LDAPConnectionInput,
  type LDAPDirectoryUser,
  type RoleItem
} from '@/api/identity-admin'

const connForm = reactive<LDAPConnectionInput>({
  provider_id: 'corp-ad',
  type: 'ad',
  server_url: 'ldaps://ad.example.com:636',
  bind_dn: '',
  bind_password: '',
  base_dn: 'DC=corp,DC=local',
  start_tls: false,
  insecure_skip_verify: true,
  attr_subject: 'objectGUID'
})

const connecting = ref(false)
const connected = ref(false)
const sessionId = ref('')
const sessionBaseDn = ref('')
const sessionExpiresIn = ref(0)

const treeData = ref<TreeNodeData[]>([])
const treeLoading = ref(false)
const selectedOrgDn = ref('')
const selectedOrgName = ref('')

const directoryUsers = ref<LDAPDirectoryUser[]>([])
const usersLoading = ref(false)
const selectedSubjects = ref<string[]>([])
const roles = ref<RoleItem[]>([])
const selectedRoleCodes = ref<string[]>(['viewer'])

const importing = ref(false)
const resultVisible = ref(false)
const importResult = ref<ImportLDAPUsersResult | null>(null)

const rowSelection = reactive<TableRowSelection>({
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false
})

const userColumns: TableColumnData[] = [
  { title: '登录名', dataIndex: 'external_username', width: 140 },
  { title: '显示名', dataIndex: 'display_name', ellipsis: true },
  { title: '邮箱', dataIndex: 'email', ellipsis: true },
  { title: '状态', slotName: 'imported', width: 90 }
]

const resultColumns: TableColumnData[] = [
  { title: 'DN / Subject', dataIndex: 'external_subject', ellipsis: true },
  { title: '结果', dataIndex: 'status', width: 80 },
  { title: '说明', dataIndex: 'message', ellipsis: true }
]

async function onConnect() {
  if (!connForm.provider_id?.trim() || !connForm.server_url?.trim() || !connForm.base_dn?.trim()) {
    Message.warning('请填写身份源 ID、Server URL 和 Base DN')
    return
  }
  connecting.value = true
  try {
    const res = await connectLDAPSession({ ...connForm })
    sessionId.value = res.session_id
    sessionBaseDn.value = res.base_dn
    sessionExpiresIn.value = res.expires_in
    connected.value = true
    selectedOrgDn.value = res.base_dn
    selectedOrgName.value = res.base_dn
    treeData.value = [{
      key: res.base_dn,
      title: res.base_dn,
      isLeaf: false
    }]
    await reloadUsers()
    Message.success('目录连接成功')
  } catch {
    /* 错误已由拦截器提示 */
  } finally {
    connecting.value = false
  }
}

async function onDisconnect() {
  if (sessionId.value) {
    try {
      await closeLDAPSession(sessionId.value)
    } catch {
      /* ignore */
    }
  }
  connected.value = false
  sessionId.value = ''
  treeData.value = []
  directoryUsers.value = []
  selectedSubjects.value = []
  selectedOrgDn.value = ''
}

async function loadOrgChildren(node: TreeNodeData) {
  const parentDn = String(node.key ?? '')
  const res = await browseLDAPOrganizations(sessionId.value, parentDn || undefined)
  node.children = res.organizations.map((org) => ({
    key: org.dn,
    title: org.name,
    isLeaf: false
  }))
}

function onOrgSelect(keys: (string | number)[]) {
  const dn = String(keys[0] ?? '')
  if (!dn) return
  selectedOrgDn.value = dn
  const node = findTreeNode(treeData.value, dn)
  selectedOrgName.value = node?.title ? String(node.title) : dn
  selectedSubjects.value = []
  reloadUsers()
}

function findTreeNode(nodes: TreeNodeData[], key: string): TreeNodeData | undefined {
  for (const node of nodes) {
    if (node.key === key) return node
    if (node.children?.length) {
      const hit = findTreeNode(node.children, key)
      if (hit) return hit
    }
  }
  return undefined
}

async function reloadUsers() {
  if (!sessionId.value || !selectedOrgDn.value) return
  usersLoading.value = true
  try {
    const res = await previewLDAPUsers(sessionId.value, selectedOrgDn.value, 200)
    directoryUsers.value = res.users
  } finally {
    usersLoading.value = false
  }
}

async function onImportSelected() {
  if (!sessionId.value || selectedSubjects.value.length === 0) return
  importing.value = true
  try {
    importResult.value = await importLDAPUsers(sessionId.value, {
      external_subjects: [...selectedSubjects.value],
      role_codes: selectedRoleCodes.value
    })
    resultVisible.value = true
    Message.success(`导入完成：成功 ${importResult.value.created}，跳过 ${importResult.value.skipped}`)
    await reloadUsers()
    selectedSubjects.value = []
  } finally {
    importing.value = false
  }
}

async function onImportAll() {
  if (!sessionId.value || !selectedOrgDn.value) return
  importing.value = true
  try {
    importResult.value = await importLDAPUsers(sessionId.value, {
      org_dn: selectedOrgDn.value,
      import_all: true,
      role_codes: selectedRoleCodes.value
    })
    resultVisible.value = true
    Message.success(`导入完成：成功 ${importResult.value.created}，跳过 ${importResult.value.skipped}`)
    await reloadUsers()
    selectedSubjects.value = []
  } finally {
    importing.value = false
  }
}

onMounted(async () => {
  try {
    const res = await fetchRoles()
    roles.value = res.items.filter((r) => r.status === 'active')
  } catch {
    /* 无权限时静默，角色绑定可选 */
  }
})

onBeforeUnmount(() => {
  if (sessionId.value) {
    closeLDAPSession(sessionId.value).catch(() => undefined)
  }
})
</script>

<style scoped lang="scss">
.ldap-import-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.tip {
  margin-bottom: 16px;
}
.conn-form {
  margin-top: 8px;
}
.session-meta {
  color: var(--color-text-3);
  font-size: 13px;
}
.browse-panel {
  min-height: 480px;
}
.org-label {
  color: var(--color-text-3);
  font-size: 13px;
  font-weight: normal;
}
.result-table {
  margin-top: 12px;
}
</style>
