// 华为云接入配置纯函数：region/region_projects 解析、未知 extra_config 保留与合并。
// 抽离自 integrations/index.vue 的 <script setup> 便于单测。
// 见 ops/huawei-ces-sync-contract.md §5.3/§11。
//
// 字段语义：
// - 展示字段：resource_group_name、resource_group_id、sync_mode、region_projects
// - 判定字段：region_projects、project_id、max_resources、enterprise_project_id
// - 审计/保留字段：未知 extra_config，编辑时原样保留，不参与表单判定
import type {
  HuaweiCloudExtraConfig,
  HuaweiCloudRegionProject,
  HuaweiCloudSyncMode
} from '@/api/integration'

// 已知的华为云 extra_config 字段；不在此列表的字段视为“未知配置”，编辑时保留，避免表单覆盖丢失。
export const HUAWEI_KNOWN_EXTRA_KEYS = [
  'sync_mode',
  'resource_group_name',
  'resource_group_id',
  'enterprise_project_id',
  'max_resources',
  'region_projects'
] as readonly string[]

export function parseRegions(text: string): string[] {
  return text.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean)
}

function tokenizeEscaped(text: string, separator: string): string[] {
  const parts: string[] = []
  let current = ''
  for (let i = 0; i < text.length; i += 1) {
    const char = text[i]
    if (char === '\\' && i + 1 < text.length) {
      const next = text[i + 1]
      if (next === '\\' || next === separator || next === '=') {
        current += next
        i += 1
        continue
      }
      current += char
      continue
    }
    if (char === separator) {
      parts.push(current)
      current = ''
      continue
    }
    current += char
  }
  parts.push(current)
  return parts
}

function escapeRegionProjectValue(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/,/g, '\\,').replace(/=/g, '\\=')
}

export function parseRegionProjects(text: string): { items: HuaweiCloudRegionProject[]; errors: string[] } {
  const items: HuaweiCloudRegionProject[] = []
  const errors: string[] = []
  const seen = new Set<string>()
  text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .forEach((line, index) => {
      if (!line) return
      const pairs: Record<string, string> = {}
      const segments = tokenizeEscaped(line, ',')

      segments.forEach((segment) => {
        const [rawKey, ...rest] = tokenizeEscaped(segment, '=')
        const key = rawKey.trim()
        const value = rest.join('=').trim()
        if (key && value) {
          pairs[key] = value
        }
      })

      const region = pairs.region
      const projectId = pairs.project_id
      if (!region || !projectId) {
        errors.push(`第 ${index + 1} 行格式错误，请使用 region=xxx,project_id=xxx[,resource_group_id=xxx]；值中的逗号和等号可分别使用 \\, / \\=`)
        return
      }

      const key = region.toLowerCase()
      if (seen.has(key)) {
        errors.push(`第 ${index + 1} 行 region 重复：${region}`)
        return
      }

      seen.add(key)
      items.push({
        region,
        project_id: projectId,
        resource_group_id: pairs.resource_group_id,
        resource_group_name: pairs.resource_group_name
      })
    })
  return { items, errors }
}

export function formatRegionProjects(items?: HuaweiCloudRegionProject[]): string {
  return (items || [])
    .map((item) => {
      const parts = [
        `region=${escapeRegionProjectValue(item.region)}`,
        `project_id=${escapeRegionProjectValue(item.project_id)}`
      ]
      if (item.resource_group_id) parts.push(`resource_group_id=${escapeRegionProjectValue(item.resource_group_id)}`)
      if (item.resource_group_name) parts.push(`resource_group_name=${escapeRegionProjectValue(item.resource_group_name)}`)
      return parts.join(',')
    })
    .join('\n')
}

// 提取 extra_config 中的非已知字段，编辑时保留，避免表单覆盖丢失自定义配置。
export function extractUnknownExtraConfig(config: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(config).filter(([key]) => !HUAWEI_KNOWN_EXTRA_KEYS.includes(key))
  )
}

export interface HuaweiExtraFormFields {
  sync_mode: HuaweiCloudSyncMode
  resource_group_name: string
  resource_group_id: string
  enterprise_project_id: string
  max_resources: number
  region_projects?: HuaweiCloudRegionProject[]
}

// 合并保留的未知配置与表单已知字段，生成最终 extra_config。
// 保留字段在前，已知字段按表单值覆盖（trim 空串、非法 max_resources、空 region_projects 跳过）。
export function mergeHuaweiExtraConfig(
  preserved: Record<string, unknown>,
  fields: HuaweiExtraFormFields
): HuaweiCloudExtraConfig {
  const config: HuaweiCloudExtraConfig = { ...preserved } as HuaweiCloudExtraConfig
  if (fields.sync_mode) config.sync_mode = fields.sync_mode
  const resourceGroupName = fields.resource_group_name.trim()
  if (!resourceGroupName || ['全部资源', 'All resources', 'All Resources'].includes(resourceGroupName)) {
    delete config.resource_group_name
  } else {
    config.resource_group_name = resourceGroupName
  }
  if (fields.resource_group_id.trim()) config.resource_group_id = fields.resource_group_id.trim()
  if (fields.enterprise_project_id.trim()) {
    config.enterprise_project_id = fields.enterprise_project_id.trim()
  }
  if (fields.max_resources && fields.max_resources > 0) {
    config.max_resources = fields.max_resources
  }
  if (fields.region_projects && fields.region_projects.length > 0) {
    config.region_projects = fields.region_projects
  }
  return config
}
