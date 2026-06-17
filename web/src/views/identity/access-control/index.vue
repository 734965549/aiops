<template>
  <div class="access-control-page">
    <a-card
      title="权限管理"
      :bordered="false"
    >
      <a-alert
        v-if="noAccess"
        type="warning"
        show-icon
        class="state-alert"
      >
        {{ accessError || '当前账号缺少权限管理所需权限，页面结构已保留但数据不可操作。' }}
      </a-alert>

      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane
          key="user-roles"
          title="用户角色绑定"
        >
          <div class="two-column">
            <section class="panel">
              <div class="panel-header">
                <div>
                  <strong>用户</strong>
                  <span>分页查询平台账号</span>
                </div>
                <a-button
                  size="small"
                  :loading="usersLoading"
                  @click="loadUsers"
                >
                  刷新
                </a-button>
              </div>

              <a-form
                :model="userFilters"
                layout="inline"
                class="filter-form"
              >
                <a-form-item label="关键字">
                  <a-input
                    v-model="userFilters.keyword"
                    allow-clear
                    placeholder="用户名 / 显示名 / 邮箱"
                    style="width: 220px"
                    @press-enter="searchUsers"
                  />
                </a-form-item>
                <a-form-item label="状态">
                  <a-select
                    v-model="userFilters.status"
                    allow-clear
                    placeholder="全部"
                    style="width: 130px"
                  >
                    <a-option value="active">
                      active
                    </a-option>
                    <a-option value="disabled">
                      disabled
                    </a-option>
                    <a-option value="locked">
                      locked
                    </a-option>
                  </a-select>
                </a-form-item>
                <a-form-item>
                  <a-button
                    type="primary"
                    @click="searchUsers"
                  >
                    查询
                  </a-button>
                </a-form-item>
              </a-form>

              <a-table
                row-key="id"
                size="small"
                :columns="userColumns"
                :data="users"
                :loading="usersLoading"
                :pagination="userPagination"
                @page-change="onUserPageChange"
                @page-size-change="onUserPageSizeChange"
                @row-click="selectUserFromTable"
              >
                <template #username="{ record }">
                  <a-typography-text strong>
                    {{ record.username }}
                  </a-typography-text>
                  <div class="sub-text">
                    {{ record.email || '-' }}
                  </div>
                </template>
                <template #status="{ record }">
                  <a-tag :color="record.status === 'active' ? 'green' : 'orange'">
                    {{ record.status }}
                  </a-tag>
                </template>
                <template #operations="{ record }">
                  <a-button
                    size="mini"
                    type="text"
                    @click.stop="selectUser(record)"
                  >
                    选择
                  </a-button>
                </template>
              </a-table>
            </section>

            <section class="panel">
              <div class="panel-header">
                <div>
                  <strong>角色绑定</strong>
                  <span>{{ selectedUserTitle }}</span>
                </div>
                <a-space>
                  <a-button
                    size="small"
                    :disabled="!selectedUserId || noAccess"
                    :loading="userRolesLoading"
                    @click="loadUserRoles"
                  >
                    刷新
                  </a-button>
                  <a-button
                    size="small"
                    type="primary"
                    :disabled="!selectedUserId || noAccess"
                    :loading="savingUserRoles"
                    @click="saveUserRoles"
                  >
                    保存
                  </a-button>
                </a-space>
              </div>

              <a-empty
                v-if="!selectedUserId"
                description="请选择用户"
              />
              <a-spin
                v-else
                :loading="userRolesLoading || dictionariesLoading"
              >
                <a-alert
                  type="info"
                  class="state-alert"
                >
                  PUT 只替换 manual 来源角色；LDAP 导入和外部组来源角色会保留并禁用显示。
                </a-alert>
                <a-checkbox-group
                  v-model="manualRoleIds"
                  class="check-list"
                >
                  <div
                    v-for="role in roles"
                    :key="role.id"
                    class="check-row"
                  >
                    <a-checkbox
                      :value="role.id"
                      :disabled="isDelegatedRole(role.id) || noAccess"
                    >
                      <span class="check-title">{{ role.name || role.code }}</span>
                      <span class="mono-text">{{ role.code }}</span>
                    </a-checkbox>
                    <a-tag
                      v-if="roleSourceOf(role.id)"
                      :color="roleSourceOf(role.id) === 'manual' ? 'blue' : 'purple'"
                    >
                      {{ roleSourceLabel(roleSourceOf(role.id)) }}
                    </a-tag>
                  </div>
                </a-checkbox-group>
              </a-spin>
            </section>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="role-grants"
          title="角色授权"
        >
          <section class="panel">
            <div class="panel-header">
              <div>
                <strong>角色授权</strong>
                <span>全量替换角色的权限、数据范围和 AI 工具权限</span>
              </div>
              <a-select
                v-model="selectedRoleId"
                allow-clear
                placeholder="选择角色"
                style="width: 280px"
              >
                <a-option
                  v-for="role in roles"
                  :key="role.id"
                  :value="role.id"
                >
                  {{ role.name }} ({{ role.code }})
                </a-option>
              </a-select>
            </div>

            <a-empty
              v-if="!selectedRoleId"
              description="请选择角色"
            />
            <a-spin
              v-else
              :loading="roleGrantsLoading || dictionariesLoading"
            >
              <div class="grant-grid">
                <div class="grant-section">
                  <div class="grant-title">
                    <strong>权限</strong>
                    <a-button
                      size="mini"
                      type="primary"
                      :disabled="noAccess"
                      :loading="savingPermissions"
                      @click="saveRolePermissions"
                    >
                      保存
                    </a-button>
                  </div>
                  <a-checkbox-group
                    v-model="rolePermissionIds"
                    class="check-list compact"
                  >
                    <div
                      v-for="permission in permissions"
                      :key="permission.id"
                      class="check-row"
                    >
                      <a-checkbox
                        :value="permission.id"
                        :disabled="noAccess"
                      >
                        <span class="check-title">{{ permission.name || permission.code }}</span>
                        <span class="mono-text">{{ permission.code }}</span>
                      </a-checkbox>
                    </div>
                  </a-checkbox-group>
                </div>

                <div class="grant-section">
                  <div class="grant-title">
                    <strong>数据范围</strong>
                    <a-button
                      size="mini"
                      type="primary"
                      :disabled="noAccess"
                      :loading="savingDataScopes"
                      @click="saveRoleDataScopes"
                    >
                      保存
                    </a-button>
                  </div>
                  <a-checkbox-group
                    v-model="roleDataScopeIds"
                    class="check-list compact"
                  >
                    <div
                      v-for="scope in dataScopes"
                      :key="scope.id"
                      class="check-row"
                    >
                      <a-checkbox
                        :value="scope.id"
                        :disabled="noAccess"
                      >
                        <span class="check-title">{{ scope.name || scope.code }}</span>
                        <span class="mono-text">{{ scope.code }} · {{ scope.scope_type }}</span>
                      </a-checkbox>
                    </div>
                  </a-checkbox-group>
                </div>

                <div class="grant-section">
                  <div class="grant-title">
                    <strong>AI 工具权限</strong>
                    <a-button
                      size="mini"
                      type="primary"
                      :disabled="noAccess"
                      :loading="savingTools"
                      @click="saveRoleAITools"
                    >
                      保存
                    </a-button>
                  </div>
                  <a-checkbox-group
                    v-model="roleAIToolPermissionIds"
                    class="check-list compact"
                  >
                    <div
                      v-for="tool in aiTools"
                      :key="tool.id"
                      class="check-row"
                    >
                      <a-checkbox
                        :value="tool.id"
                        :disabled="noAccess"
                      >
                        <span class="check-title">{{ tool.tool_name || tool.tool_code }}</span>
                        <span class="mono-text">{{ tool.tool_code }} · {{ tool.permission_mode }}</span>
                      </a-checkbox>
                    </div>
                  </a-checkbox-group>
                </div>
              </div>
            </a-spin>
          </section>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Message, type TableData } from '@arco-design/web-vue'
import { ApiHttpError, getApiError } from '@/api/request'
import * as identityAdminApi from '@/api/identity-admin'
import type {
  AIToolPermissionItem,
  DataScopeItem,
  IdentityUserItem,
  PermissionItem,
  RoleItem,
  UserRoleBindingItem
} from '@/api/identity-admin'

const activeTab = ref('user-roles')
const noAccess = ref(false)
const accessError = ref('')

const users = ref<IdentityUserItem[]>([])
const roles = ref<RoleItem[]>([])
const permissions = ref<PermissionItem[]>([])
const dataScopes = ref<DataScopeItem[]>([])
const aiTools = ref<AIToolPermissionItem[]>([])

const selectedUserId = ref('')
const userRoleBindings = ref<UserRoleBindingItem[]>([])
const manualRoleIds = ref<string[]>([])
const selectedRoleId = ref('')
const rolePermissionIds = ref<string[]>([])
const roleDataScopeIds = ref<string[]>([])
const roleAIToolPermissionIds = ref<string[]>([])

const dictionariesLoading = ref(false)
const usersLoading = ref(false)
const userRolesLoading = ref(false)
const roleGrantsLoading = ref(false)
const savingUserRoles = ref(false)
const savingPermissions = ref(false)
const savingDataScopes = ref(false)
const savingTools = ref(false)

const userFilters = reactive({
  keyword: '',
  status: ''
})

const userPagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showTotal: true,
  showPageSize: true
})

const userColumns = [
  { title: '用户', slotName: 'username', ellipsis: true },
  { title: '显示名', dataIndex: 'display_name', ellipsis: true },
  { title: '状态', slotName: 'status', width: 110 },
  { title: '操作', slotName: 'operations', width: 90 }
]

const selectedUser = computed(() => users.value.find((item) => item.id === selectedUserId.value) || null)
const selectedUserTitle = computed(() => {
  if (!selectedUser.value) return '未选择用户'
  return `${selectedUser.value.display_name || selectedUser.value.username} · ${selectedUser.value.id}`
})

function markAccessError(error: unknown) {
  const apiError = error instanceof ApiHttpError ? error : getApiError(error)
  if (apiError?.status === 403) {
    noAccess.value = true
    accessError.value = apiError.message
    return true
  }
  return false
}

async function runRequest<T>(request: () => Promise<T>): Promise<T | undefined> {
  try {
    return await request()
  } catch (error) {
    markAccessError(error)
    return undefined
  }
}

async function loadDictionaries() {
  dictionariesLoading.value = true
  try {
    const [rolePage, permissionPage, scopeResp, toolResp] = await Promise.all([
      runRequest(() => identityAdminApi.fetchRoles(500)),
      runRequest(() => identityAdminApi.fetchPermissions(1000)),
      runRequest(() => identityAdminApi.fetchDataScopes()),
      runRequest(() => identityAdminApi.fetchAIToolPermissions())
    ])
    roles.value = rolePage?.items ?? []
    permissions.value = permissionPage?.items ?? []
    dataScopes.value = scopeResp?.items ?? []
    aiTools.value = toolResp?.items ?? []
    if (!selectedRoleId.value && roles.value.length) {
      selectedRoleId.value = roles.value[0].id
    }
  } finally {
    dictionariesLoading.value = false
  }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const page = await runRequest(() => identityAdminApi.fetchUsers({
      page: userPagination.current,
      page_size: userPagination.pageSize,
      keyword: userFilters.keyword || undefined,
      status: userFilters.status || undefined
    }))
    users.value = page?.items ?? []
    userPagination.total = page?.total ?? 0
    if (!selectedUserId.value && users.value.length) {
      await selectUser(users.value[0])
    }
  } finally {
    usersLoading.value = false
  }
}

async function searchUsers() {
  userPagination.current = 1
  await loadUsers()
}

async function onUserPageChange(page: number) {
  userPagination.current = page
  await loadUsers()
}

async function onUserPageSizeChange(pageSize: number) {
  userPagination.pageSize = pageSize
  userPagination.current = 1
  await loadUsers()
}

async function selectUser(record: IdentityUserItem) {
  selectedUserId.value = record.id
  await loadUserRoles()
}

async function selectUserFromTable(record: TableData) {
  await selectUser(record as unknown as IdentityUserItem)
}

async function loadUserRoles() {
  if (!selectedUserId.value) return
  userRolesLoading.value = true
  try {
    const resp = await runRequest(() => identityAdminApi.fetchUserRoles(selectedUserId.value))
    userRoleBindings.value = resp?.items ?? []
    manualRoleIds.value = userRoleBindings.value
      .filter((item) => item.source === 'manual')
      .map((item) => item.id)
  } finally {
    userRolesLoading.value = false
  }
}

async function saveUserRoles() {
  if (!selectedUserId.value) return
  savingUserRoles.value = true
  try {
    const resp = await runRequest(() => identityAdminApi.replaceUserRoles(selectedUserId.value, manualRoleIds.value))
    if (!resp) return
    userRoleBindings.value = resp.items
    manualRoleIds.value = resp.items.filter((item) => item.source === 'manual').map((item) => item.id)
    Message.success('用户角色已保存')
  } finally {
    savingUserRoles.value = false
  }
}

function roleSourceOf(roleId: string) {
  return userRoleBindings.value.find((item) => item.id === roleId)?.source || ''
}

function isDelegatedRole(roleId: string) {
  const source = roleSourceOf(roleId)
  return source !== '' && source !== 'manual'
}

function roleSourceLabel(source: string) {
  if (source === 'manual') return 'manual'
  if (source === 'ldap_import') return 'LDAP'
  if (source === 'external_group') return '外部组'
  return source
}

async function loadRoleGrants() {
  if (!selectedRoleId.value) {
    rolePermissionIds.value = []
    roleDataScopeIds.value = []
    roleAIToolPermissionIds.value = []
    return
  }
  roleGrantsLoading.value = true
  try {
    const [permResp, scopeResp, toolResp] = await Promise.all([
      runRequest(() => identityAdminApi.fetchRolePermissions(selectedRoleId.value)),
      runRequest(() => identityAdminApi.fetchRoleDataScopes(selectedRoleId.value)),
      runRequest(() => identityAdminApi.fetchRoleAIToolPermissions(selectedRoleId.value))
    ])
    rolePermissionIds.value = permResp?.items.map((item) => item.id) ?? []
    roleDataScopeIds.value = scopeResp?.items.map((item) => item.id) ?? []
    roleAIToolPermissionIds.value = toolResp?.items.map((item) => item.id) ?? []
  } finally {
    roleGrantsLoading.value = false
  }
}

async function saveRolePermissions() {
  if (!selectedRoleId.value) return
  savingPermissions.value = true
  try {
    const resp = await runRequest(() => identityAdminApi.replaceRolePermissions(selectedRoleId.value, rolePermissionIds.value))
    if (!resp) return
    rolePermissionIds.value = resp.items.map((item) => item.id)
    Message.success('角色权限已保存')
  } finally {
    savingPermissions.value = false
  }
}

async function saveRoleDataScopes() {
  if (!selectedRoleId.value) return
  savingDataScopes.value = true
  try {
    const resp = await runRequest(() => identityAdminApi.replaceRoleDataScopes(selectedRoleId.value, roleDataScopeIds.value))
    if (!resp) return
    roleDataScopeIds.value = resp.items.map((item) => item.id)
    Message.success('数据范围已保存')
  } finally {
    savingDataScopes.value = false
  }
}

async function saveRoleAITools() {
  if (!selectedRoleId.value) return
  savingTools.value = true
  try {
    const resp = await runRequest(() => identityAdminApi.replaceRoleAIToolPermissions(selectedRoleId.value, roleAIToolPermissionIds.value))
    if (!resp) return
    roleAIToolPermissionIds.value = resp.items.map((item) => item.id)
    Message.success('AI 工具权限已保存')
  } finally {
    savingTools.value = false
  }
}

watch(selectedRoleId, () => {
  loadRoleGrants()
})

onMounted(async () => {
  await Promise.all([loadDictionaries(), loadUsers()])
})
</script>

<style scoped>
.access-control-page {
  min-width: 0;
}

.state-alert {
  margin-bottom: 14px;
}

.two-column {
  display: grid;
  grid-template-columns: minmax(420px, 1.05fr) minmax(360px, 0.95fr);
  gap: 16px;
}

.panel {
  min-width: 0;
  padding: 16px;
  border: 1px solid rgba(22, 93, 255, 0.12);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.72);
}

.panel-header,
.grant-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.panel-header strong,
.grant-title strong {
  display: block;
  color: #1d2538;
}

.panel-header span {
  display: block;
  margin-top: 3px;
  color: #6b778c;
  font-size: 12px;
}

.filter-form {
  margin-bottom: 12px;
}

.sub-text {
  margin-top: 2px;
  color: #7a869a;
  font-size: 12px;
}

.mono-text {
  margin-left: 8px;
  color: #5f6f89;
  font-family: Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}

.check-list {
  display: grid;
  gap: 8px;
}

.check-list.compact {
  max-height: 480px;
  overflow: auto;
  padding-right: 4px;
}

.check-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
  padding: 9px 10px;
  border: 1px solid rgba(28, 83, 160, 0.1);
  border-radius: 6px;
  background: #fff;
}

.check-title {
  font-weight: 600;
}

.grant-grid {
  display: grid;
  grid-template-columns: minmax(320px, 1.2fr) minmax(260px, 0.9fr) minmax(260px, 0.9fr);
  gap: 16px;
}

.grant-section {
  min-width: 0;
}

@media (max-width: 1200px) {
  .two-column,
  .grant-grid {
    grid-template-columns: 1fr;
  }
}
</style>
