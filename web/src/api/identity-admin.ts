import { http } from './request'

export interface LDAPConnectionInput {
  provider_id: string
  type: 'ldap' | 'ad'
  server_url: string
  bind_dn?: string
  bind_password?: string
  base_dn: string
  start_tls?: boolean
  insecure_skip_verify?: boolean
  browse_org_filter?: string
  browse_user_filter?: string
  attr_subject?: string
}

export interface LDAPConnectResult {
  session_id: string
  provider_id: string
  base_dn: string
  expires_in: number
}

export interface LDAPOrganization {
  dn: string
  name: string
}

export interface LDAPDirectoryUser {
  external_subject: string
  dn?: string
  external_username: string
  display_name: string
  email: string
  imported: boolean
}

export interface RoleItem {
  id: string
  code: string
  name: string
  description: string
  status: string
  is_system: boolean
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface IdentityUserItem {
  id: string
  username: string
  display_name: string
  email: string
  status: string
  created_at: number
  updated_at: number
}

export interface UserRoleBindingItem extends RoleItem {
  source: 'manual' | 'ldap_import' | 'external_group' | string
}

export interface PermissionItem {
  id: string
  code: string
  name: string
  resource: string
  action: string
  description: string
}

export interface DataScopeItem {
  id: string
  code: string
  name: string
  scope_type: string
  scope_config: Record<string, unknown>
  description: string
}

export interface AIToolPermissionItem {
  id: string
  tool_code: string
  tool_name: string
  permission_mode: string
  permits_unconfirmed_invoke: boolean
  description: string
}

export interface ImportLDAPUsersResult {
  created: number
  skipped: number
  failed: number
  users: Array<{
    external_subject: string
  dn?: string
    status: 'created' | 'skipped' | 'failed'
    message?: string
    user?: { id: string; username: string; display_name: string; email: string; status: string }
  }>
}

export function connectLDAPSession(input: LDAPConnectionInput) {
  return http<LDAPConnectResult>({
    url: '/api/identity/admin/ldap/connect',
    method: 'post',
    data: input
  })
}

export function closeLDAPSession(sessionId: string) {
  return http<{ closed: boolean }>({
    url: `/api/identity/admin/ldap/sessions/${encodeURIComponent(sessionId)}`,
    method: 'delete'
  })
}

export function browseLDAPOrganizations(sessionId: string, parentDn?: string) {
  return http<{ organizations: LDAPOrganization[] }>({
    url: `/api/identity/admin/ldap/sessions/${encodeURIComponent(sessionId)}/organizations`,
    method: 'get',
    params: parentDn ? { parent_dn: parentDn } : undefined
  })
}

export function previewLDAPUsers(sessionId: string, orgDn: string, limit = 100) {
  return http<{ users: LDAPDirectoryUser[] }>({
    url: `/api/identity/admin/ldap/sessions/${encodeURIComponent(sessionId)}/users`,
    method: 'get',
    params: { org_dn: orgDn, limit }
  })
}

export function importLDAPUsers(
  sessionId: string,
  payload: {
    org_dn?: string
    external_subjects?: string[]
    import_all?: boolean
    role_codes?: string[]
  }
) {
  return http<ImportLDAPUsersResult>({
    url: `/api/identity/admin/ldap/sessions/${encodeURIComponent(sessionId)}/import`,
    method: 'post',
    data: payload
  })
}

export function fetchRoles(pageSize = 100) {
  return http<PageResult<RoleItem>>({
    url: '/api/identity/roles',
    method: 'get',
    params: { page: 1, page_size: pageSize, status: 'active' }
  })
}

export function fetchUsers(params: { page?: number; page_size?: number; status?: string; keyword?: string } = {}) {
  return http<PageResult<IdentityUserItem>>({
    url: '/api/identity/admin/users',
    method: 'get',
    params
  })
}

export function fetchUserRoles(userId: string) {
  return http<{ items: UserRoleBindingItem[] }>({
    url: `/api/identity/admin/users/${encodeURIComponent(userId)}/roles`,
    method: 'get'
  })
}

export function replaceUserRoles(userId: string, roleIds: string[]) {
  return http<{ items: UserRoleBindingItem[] }>({
    url: `/api/identity/admin/users/${encodeURIComponent(userId)}/roles`,
    method: 'put',
    data: { role_ids: roleIds }
  })
}

export function fetchPermissions(pageSize = 500) {
  return http<PageResult<PermissionItem>>({
    url: '/api/identity/permissions',
    method: 'get',
    params: { page: 1, page_size: pageSize }
  })
}

export function fetchDataScopes() {
  return http<{ items: DataScopeItem[] }>({
    url: '/api/identity/data-scopes',
    method: 'get'
  })
}

export function fetchAIToolPermissions() {
  return http<{ items: AIToolPermissionItem[] }>({
    url: '/api/identity/ai-tool-permissions',
    method: 'get'
  })
}

export function fetchRolePermissions(roleId: string) {
  return http<{ items: PermissionItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/permissions`,
    method: 'get'
  })
}

export function replaceRolePermissions(roleId: string, permissionIds: string[]) {
  return http<{ items: PermissionItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/permissions`,
    method: 'put',
    data: { permission_ids: permissionIds }
  })
}

export function fetchRoleDataScopes(roleId: string) {
  return http<{ items: DataScopeItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/data-scopes`,
    method: 'get'
  })
}

export function replaceRoleDataScopes(roleId: string, dataScopeIds: string[]) {
  return http<{ items: DataScopeItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/data-scopes`,
    method: 'put',
    data: { data_scope_ids: dataScopeIds }
  })
}

export function fetchRoleAIToolPermissions(roleId: string) {
  return http<{ items: AIToolPermissionItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/ai-tool-permissions`,
    method: 'get'
  })
}

export function replaceRoleAIToolPermissions(roleId: string, toolPermissionIds: string[]) {
  return http<{ items: AIToolPermissionItem[] }>({
    url: `/api/identity/admin/roles/${encodeURIComponent(roleId)}/ai-tool-permissions`,
    method: 'put',
    data: { tool_permission_ids: toolPermissionIds }
  })
}
