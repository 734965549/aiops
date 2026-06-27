// 华为云接入配置纯函数：region/region_projects 解析、未知 extra_config 保留与合并。
// 抽离自 integrations/index.vue 的 <script setup> 便于单测。
// 见 docs/huawei-ces-asset-sync-plan.md §5.3/§11/§20。
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

export function parseRegionProjects(text: string): { items: HuaweiCloudRegionProject[]; errors: string[] } {
  const items: HuaweiCloudRegionProject[] = []
  const errors: string[] = []
  const seen = new Set<string>()
  text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .forEach((line, index) => {
      if (!line) return
      const [regionPart, ...projectParts] = line.split('=')
      const region = regionPart.trim()
      const projectId = projectParts.join('=').trim()
      if (!region || !projectId) {
        errors.push(`第 ${index + 1} 行格式错误，请使用 region=project_id`)
        return
      }
      const key = region.toLowerCase()
      if (seen.has(key)) {
        errors.push(`第 ${index + 1} 行 region 重复：${region}`)
        return
      }
      seen.add(key)
      items.push({ region, project_id: projectId })
    })
  return { items, errors }
}

export function formatRegionProjects(items?: HuaweiCloudRegionProject[]): string {
  return (items || []).map((item) => `${item.region}=${item.project_id}`).join('\n')
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
  if (fields.resource_group_name.trim()) config.resource_group_name = fields.resource_group_name.trim()
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
