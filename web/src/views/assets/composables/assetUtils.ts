// 资产页面纯函数：label 掩码/排序与同步批次 message 解析。
// 抽离自 assets/index.vue 的 <script setup> 便于单测。
// 见 docs/huawei-ces-asset-sync-plan.md §9.1（labels 展示）与 §8.1（batch message 摘要）。

export type LabelEntry = {
  key: string
  displayValue: string
}

export type SyncBatchMessageSummary = {
  ces_total?: string
  discovered?: string
  failed_scopes?: string
  enriched?: string
  enrichment_failed?: string
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

// 敏感 label key 掩码正则；命中则展示 ******，避免 secret/token/password/key 等明文泄露。
export const sensitiveLabelKeyPattern = /(secret|token|password|passwd|pwd|key|authorization|credential)/i

export function maskLabelValue(key: string, value: string): string {
  if (!sensitiveLabelKeyPattern.test(key)) return value
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

export function parseSyncBatchMessage(message: string): SyncBatchMessageSummary {
  const summary: SyncBatchMessageSummary = {}
  const fields: (keyof SyncBatchMessageSummary)[] = [
    'ces_total',
    'discovered',
    'failed_scopes',
    'enriched',
    'enrichment_failed'
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
