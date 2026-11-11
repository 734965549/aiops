import type { Resource, SyncBatchSummary, SyncBatchSummaryDisplay } from '@/api/asset'

// 资产页面纯函数：label 掩码/排序、长 ID 短码、同步批次 message 兼容解析。
// 抽离自 assets/index.vue 的 <script setup> 便于单测。
// 见 ops/huawei-ces-sync-contract.md §15.5（labels 展示契约）与 §15.3（批次 summary / summary.scopes[] 展示）。

/**
 * 表格内的长 ID（云账号 / 云资源 ID / 批次 ID）只显示首段 + 尾段，全文交给 tooltip。
 * 直接把 UUID / 36 位 ID 放进单元格会把列撑宽，进而逼出整表横向滚动条。
 */
export function shortId(value?: string, head = 8, tail = 4): string {
  const text = (value || '').trim()
  if (!text) return ''
  if (text.length <= head + tail + 1) return text
  return `${text.slice(0, head)}…${text.slice(-tail)}`
}

/**
 * 资源表可排序字段白名单，与后端 ops/cloud-observability-contract.md §5.5.4 一致。
 * 表头排序必须走服务端，不能对当前页做本地假排序。
 */
export const SORTABLE_RESOURCE_FIELDS = [
  'name',
  'source',
  'resource_type',
  'cloud_resource_type',
  'integration_account_id',
  'cloud_resource_id',
  'region',
  'sync_status',
  'last_synced_at',
  'namespace',
  'pod',
  'instance',
  'sync_batch_id',
  'created_at'
  // updated_at 暂不开放排序：203 部署版本的后端排序白名单未覆盖该字段（实测回退默认）。
  // 后端补齐 "updated_at": "updated_at" 后再加入本白名单，避免表头出现点了没反应的箭头。
] as const

export interface ResourceSortParams {
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface ResourceFilterState {
  keyword: string
  source: string
  cloud_resource_type: string
  region: string
  sync_status: string
}

export interface ResourceSortState {
  field: string
  direction: '' | 'ascend' | 'descend'
}

export const RESOURCE_FILTER_QUERY_KEYS = [
  'keyword',
  'source',
  'cloud_resource_type',
  'region',
  'sync_status'
] as const

function queryText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/** URL query -> 筛选状态；非字符串或缺失值统一回落空字符串。 */
export function resourceFiltersFromQuery(query: Record<string, unknown>): ResourceFilterState {
  return {
    keyword: queryText(query.keyword),
    source: queryText(query.source),
    cloud_resource_type: queryText(query.cloud_resource_type),
    region: queryText(query.region),
    sync_status: queryText(query.sync_status)
  }
}

/** URL query -> 排序状态；非法字段或方向一律回落默认排序。 */
export function resourceSortFromQuery(query: Record<string, unknown>): ResourceSortState {
  const sortBy = queryText(query.sort_by)
  if (!sortBy) {
    return { field: '', direction: '' }
  }
  const params = toResourceSortParams(sortBy, queryText(query.sort_order) === 'desc' ? 'descend' : 'ascend')
  if (!params.sort_by) {
    return { field: '', direction: '' }
  }
  return {
    field: params.sort_by,
    direction: params.sort_order === 'desc' ? 'descend' : 'ascend'
  }
}

/** 把筛选/排序写回 query；空值删除参数，保留 application_id / resource_id 等其它键。 */
export function withResourceQuery(
  base: Record<string, unknown>,
  filters: ResourceFilterState,
  sort: ResourceSortState
): Record<string, unknown> {
  const query = { ...base }
  const put = (key: string, value: string) => {
    if (value) {
      query[key] = value
    } else {
      delete query[key]
    }
  }
  for (const key of RESOURCE_FILTER_QUERY_KEYS) {
    put(key, (filters[key] || '').trim())
  }
  put('sort_by', sort.field)
  put('sort_order', sort.direction === 'descend' ? 'desc' : sort.direction === 'ascend' ? 'asc' : '')
  return query
}

/** Arco 的 sorterChange(direction: '' | 'ascend' | 'descend') -> 服务端排序参数。 */
export function toResourceSortParams(field: string, direction: string): ResourceSortParams {
  if (!direction || !(SORTABLE_RESOURCE_FIELDS as readonly string[]).includes(field)) {
    return {}
  }
  return {
    sort_by: field,
    sort_order: direction === 'descend' ? 'desc' : 'asc'
  }
}

/**
 * 资源表的「附加列」元数据。
 * 默认视图只保留验收要求的核心列（来源、云资源 ID、Region、同步状态等），
 * 其余低频列由用户在「列设置」里按需勾选，避免 15 列同时挤在窄栏里。
 */
export type OptionalResourceColumnKey =
  | 'last_synced_at'
  | 'sync_batch_id'
  | 'labels'
  | 'namespace'
  | 'pod'
  | 'instance'
  | 'created_at'
  | 'updated_at'

export interface OptionalResourceColumn {
  key: OptionalResourceColumnKey
  label: string
  width: number
  slotName?: string
  dataIndex?: string
}

export const OPTIONAL_RESOURCE_COLUMNS: OptionalResourceColumn[] = [
  { key: 'last_synced_at', label: '最近同步', width: 150, slotName: 'last_synced_at' },
  { key: 'sync_batch_id', label: '最近成功批次', width: 180, slotName: 'sync_batch_id' },
  { key: 'labels', label: 'Labels', width: 220, slotName: 'labels' },
  { key: 'namespace', label: 'Namespace', width: 110, dataIndex: 'namespace' },
  { key: 'pod', label: 'Pod', width: 110, dataIndex: 'pod' },
  { key: 'instance', label: 'Instance', width: 120, dataIndex: 'instance' },
  { key: 'created_at', label: '创建时间', width: 160, slotName: 'created_at' },
  { key: 'updated_at', label: '更新时间', width: 160, slotName: 'updated_at' }
]

/** 列设置的本地持久化 key；读取时必须走白名单校验。 */
export const ASSET_COLUMNS_STORAGE_KEY = 'aiops.asset.visible-extra-columns'

/**
 * localStorage 的值不可信（旧版本遗留 key、手工篡改、脏数据）：
 * 只保留当前定义中存在的列，按定义顺序返回并去重。
 */
export function sanitizeVisibleColumns(value: unknown): OptionalResourceColumnKey[] {
  if (!Array.isArray(value)) return []
  return OPTIONAL_RESOURCE_COLUMNS
    .map((col) => col.key)
    .filter((key) => value.includes(key))
}

/** CSV 单元格统一加引号并转义内部双引号，避免逗号/换行/公式注入破坏表格结构。 */
export function csvCell(value: unknown): string {
  const text = value == null ? '' : String(value)
  return `"${text.replace(/"/g, '""')}"`
}

const RESOURCE_CSV_COLUMNS: Array<[string, (row: Resource) => unknown]> = [
  ['资源 ID', (r) => r.id],
  ['资源名', (r) => r.name],
  ['来源', (r) => r.source],
  ['类型', (r) => r.resource_type],
  ['云类型', (r) => r.cloud_resource_type],
  ['云账号', (r) => r.integration_account_id],
  ['云资源 ID', (r) => r.cloud_resource_id],
  ['Region', (r) => r.region],
  ['同步状态', (r) => r.sync_status],
  ['Namespace', (r) => r.namespace],
  ['Pod', (r) => r.pod],
  ['Node', (r) => r.node],
  ['Instance', (r) => r.instance],
  ['最近同步', (r) => r.last_synced_at ? new Date(r.last_synced_at * 1000).toISOString() : ''],
  ['最近成功批次', (r) => r.sync_batch_id],
  ['Labels', (r) => r.labels ? JSON.stringify(r.labels) : ''],
  ['创建时间', (r) => r.created_at ? new Date(r.created_at * 1000).toISOString() : ''],
  ['更新时间', (r) => r.updated_at ? new Date(r.updated_at * 1000).toISOString() : '']
]

/** 生成带表头的 CSV 文本；时间戳统一 ISO，Excel 打开时由下载侧补 UTF-8 BOM。 */
export function buildResourceCsv(rows: Resource[]): string {
  const header = RESOURCE_CSV_COLUMNS.map(([label]) => csvCell(label)).join(',')
  const lines = rows.map((row) => RESOURCE_CSV_COLUMNS.map(([, get]) => csvCell(get(row))).join(','))
  return [header, ...lines].join('\r\n')
}

export type LabelEntry = {
  key: string
  displayValue: string
}

export type SyncBatchMessageSummary = {
  sync_mode?: string
  resource_group_name?: string
  resource_group_id?: string
  config_mode_fallback?: string
  resource_group_selection?: string
  partial_reason?: string
  ces_total?: string
  discovered?: string
  failed_scopes?: string
  completed?: string
  created?: string
  updated?: string
  stale?: string
  failed?: string
  enriched?: string
  enrichment_failed?: string
  enrichment_warnings?: string
  enrichment_stage_error?: string
  writeback_failed?: string
  max_resources_reached?: string
  product_names_empty?: string
  dimension_groups?: string
  group_coverage_mismatch?: string
}

// 展示排序：已知关键字段在前，其余按字典序追加。
export const preferredLabelKeys = [
  'namespace',
  'dim_name',
  'resource_group_id',
  'resource_group_name',
  'enterprise_project_id',
  'private_ip',
  'flavor',
  'vpc_id',
  'az',
  'volume_type',
  'size_gb',
  'attached_to',
  'cidr',
  'subnet_count',
  'engine',
  'capacity_gb',
  'spec_code'
]

// 敏感 label key 掩码：归一化为小写后做敏感词包含判断，兼容 clientSecret、accessToken 等驼峰命名。
// 命中则展示 ******，避免 secret/token/password/key 等明文泄露。
const SENSITIVE_LABEL_DENYLIST = new Set<string>(['ak', 'sk', 'accesskey', 'secretkey'])

const SENSITIVE_WORDS = [
  'secret', 'token', 'password', 'passwd', 'pwd', 'key', 'authorization', 'credential'
]

// 安全白名单：归一化后包含敏感词但确认为非敏感的 key，避免 keypair_name / keyword 等正常字段被误伤。
const SENSITIVE_LABEL_SAFELIST = new Set<string>([
  'keypair_name', 'keypair_id', 'keypair',
  'keyword', 'monkey'
])

export function isSensitiveLabelKey(key: string): boolean {
  if (!key) return false
  const normalized = key.toLowerCase()
  if (SENSITIVE_LABEL_DENYLIST.has(normalized)) return true
  if (SENSITIVE_LABEL_SAFELIST.has(normalized)) return false
  return SENSITIVE_WORDS.some((word) => normalized.includes(word))
}

export function maskLabelValue(key: string, value: string): string {
  if (!isSensitiveLabelKey(key)) return value
  if (!value) return ''
  return '******'
}

export function labelEntries(labels?: Record<string, string>): LabelEntry[] {
  if (!labels) return []
  const keys = Object.keys(labels).filter((key) => labels[key] !== undefined && labels[key] !== '')
  const preferred = preferredLabelKeys.filter((key) => keys.includes(key))
  const rest = keys.filter((key) => !preferredLabelKeys.includes(key)).sort((a, b) => a.localeCompare(b))
  return [...preferred, ...rest].map((key) => ({
    key,
    displayValue: maskLabelValue(key, String(labels[key]))
  }))
}

export function humanizeSyncPartialReason(reason?: string): string {
  const text = (reason || '').trim()
  if (!text) return ''
  return text
    .replaceAll('product_names_empty=true', '兜底白名单结果')
    .replaceAll('max_resources_reached=true', '查询上限截断')
    .replaceAll('query_failed_types=', '查询失败类型：')
    .replaceAll('conversion_failed_types=', '转换失败类型：')
    .replaceAll('group_coverage_mismatch=', '维度组覆盖对账不符：')
}

export function formatSyncBatchSummary(summary?: SyncBatchSummary, message = ''): SyncBatchMessageSummary {
  if (summary) {
    return {
      ...formatSyncBatchSummaryDisplay(summary),
      completed: summary.completed_count !== undefined ? String(summary.completed_count) : undefined,
      failed_scopes: summary.failed_scopes?.length ? summary.failed_scopes.join(', ') : undefined,
      enrichment_failed: summary.enrichment_failed_types?.length ? summary.enrichment_failed_types.join(', ') : undefined,
      enrichment_warnings: summary.enrichment_warnings?.length ? summary.enrichment_warnings.join(', ') : undefined,
      dimension_groups: summary.dimension_group_count !== undefined && summary.dimension_group_count > 0 ? String(summary.dimension_group_count) : undefined,
      group_coverage_mismatch: summary.group_coverage_mismatches?.length ? summary.group_coverage_mismatches.join(', ') : undefined
    }
  }
  return parseSyncBatchMessage(message)
}

export function formatSyncBatchSummaryDisplay(summary: SyncBatchSummaryDisplay): SyncBatchMessageSummary {
  return {
    sync_mode: summary.sync_mode,
    resource_group_name: summary.resource_group_name,
    resource_group_id: summary.resource_group_id,
    ces_total: summary.ces_total !== undefined ? String(summary.ces_total) : undefined,
    discovered: summary.discovered_count !== undefined ? String(summary.discovered_count) : undefined,
    enriched: summary.enriched_count !== undefined ? String(summary.enriched_count) : undefined,
    partial_reason: summary.partial_reason,
    max_resources_reached: summary.max_resources_reached ? 'true' : undefined,
    product_names_empty: summary.product_names_empty ? 'true' : undefined
  }
}

export interface SyncBatchSignalTag { label: string; value: string; color: string }

/**
 * 把批次摘要的结构化信号转成展示用标签。
 * 聚合口径（全部资源组并集）额外给出实际覆盖的资源组数量；
 * dimension 级资源组给出覆盖组数（按资源实际维度发现）；
 * scope 不完整时明确提示本轮不标记 stale。
 */
export function buildSyncBatchSignalTags(summary?: SyncBatchSummary): SyncBatchSignalTag[] {
  if (!summary) return []
  const tags: SyncBatchSignalTag[] = []
  if (summary.max_resources_reached) tags.push({ label: 'Max reached', value: '是', color: 'orange' })
  if (summary.product_names_empty) tags.push({ label: 'Product names empty', value: '是；仅为兜底白名单结果，不保证完整性', color: 'gold' })
  if ((summary.resource_group_count ?? 0) > 1) {
    tags.push({ label: 'Resource groups', value: `${summary.resource_group_count} 个分组（全部资源并集）`, color: 'blue' })
  }
  // dimension 级资源组按已知 namespace 服务枚举 + 资源实际维度发现，不是权威 scope，
  // 见 dimension 组支持方案 §4.2/§4.7。
  if ((summary.dimension_group_count ?? 0) > 0) {
    tags.push({ label: 'Dimension groups', value: `${summary.dimension_group_count} 个（按资源实际维度发现）`, color: 'blue' })
  }
  if ((summary.group_coverage_mismatches?.length ?? 0) > 0) {
    tags.push({ label: 'Coverage mismatch', value: `${summary.group_coverage_mismatches!.join(', ')}：声明总数大于实际发现，可能存在未覆盖服务`, color: 'orange' })
  }
  if (summary.scope_incomplete) {
    tags.push({ label: 'Scope incomplete', value: '有资源组未覆盖；本轮不标记 stale', color: 'orange' })
  }
  if ((summary.query_failed_types?.length ?? 0) > 0) tags.push({ label: 'Query failed types', value: summary.query_failed_types!.join(', '), color: 'red' })
  if ((summary.conversion_failed_types?.length ?? 0) > 0) tags.push({ label: 'Conversion failed types', value: summary.conversion_failed_types!.join(', '), color: 'red' })
  if ((summary.enrichment_failed_types?.length ?? 0) > 0) tags.push({ label: 'Enrichment failed types', value: summary.enrichment_failed_types!.join(', '), color: 'red' })
  if ((summary.enrichment_warnings?.length ?? 0) > 0) tags.push({ label: 'Enrichment warnings', value: summary.enrichment_warnings!.join(', '), color: 'orange' })
  if (summary.enrichment_stage_error) tags.push({ label: 'Enrichment stage error', value: summary.enrichment_stage_error, color: 'red' })
  if ((summary.writeback_failed_count ?? 0) > 0) tags.push({ label: 'Writeback failed', value: String(summary.writeback_failed_count), color: 'red' })
  if ((summary.failed_scopes?.length ?? 0) > 0) tags.push({ label: 'Failed scopes', value: summary.failed_scopes!.join(', '), color: 'gray' })
  return tags
}

export function parseSyncBatchMessage(message: string): SyncBatchMessageSummary {
  const summary: SyncBatchMessageSummary = {}
  const fields: (keyof SyncBatchMessageSummary)[] = [
    'sync_mode',
    'resource_group_name',
    'resource_group_id',
    'config_mode_fallback',
    'resource_group_selection',
    'partial_reason',
    'ces_total',
    'discovered',
    'failed_scopes',
    'completed',
    'created',
    'updated',
    'stale',
    'failed',
    'enriched',
    'enrichment_failed',
    'writeback_failed',
    'dimension_groups',
    'group_coverage_mismatch'
  ]
  const parts = message.split(';').map((part) => part.trim()).filter(Boolean)
  for (const field of fields) {
    const values = parts
      .map((part) => part.match(new RegExp(`${field}=([^\\s]+)`))?.[1])
      .filter((value): value is string => Boolean(value))
    if (values.length) {
      summary[field] = values.join(', ')
    }
  }
  return summary
}
