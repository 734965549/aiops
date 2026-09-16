import type { SyncBatchSummary, SyncBatchSummaryDisplay } from '@/api/asset'

// 资产页面纯函数：label 掩码/排序与同步批次 message 兼容解析。
// 抽离自 assets/index.vue 的 <script setup> 便于单测。
// 见 ops/huawei-ces-sync-contract.md §15.5（labels 展示契约）与 §15.3（批次 summary / summary.scopes[] 展示）。

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
}

export function formatSyncBatchSummary(summary?: SyncBatchSummary, message = ''): SyncBatchMessageSummary {
  if (summary) {
    return {
      ...formatSyncBatchSummaryDisplay(summary),
      completed: summary.completed_count !== undefined ? String(summary.completed_count) : undefined,
      failed_scopes: summary.failed_scopes?.length ? summary.failed_scopes.join(', ') : undefined,
      enrichment_failed: summary.enrichment_failed_types?.length ? summary.enrichment_failed_types.join(', ') : undefined,
      enrichment_warnings: summary.enrichment_warnings?.length ? summary.enrichment_warnings.join(', ') : undefined
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
    'writeback_failed'
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
