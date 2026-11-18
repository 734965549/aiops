import { useAIContextStore } from '@/stores/aiContext'
import type { Resource } from '@/api/asset'

/**
 * 资产页与 AI 助手的上下文桥接（ops/ai-contract.md §10.8）。
 *
 * 资产页比告警页多一层：选中的应用决定应用上下文，行内高亮的资源决定资源上下文。
 * 只写入业务 ID、名称与类型，不写标签明文或任何凭据。
 */
export interface AssetAIContextInput {
  applicationId?: string
  /** 应用名仅用于标题展示，服务端只保存 page_title。 */
  applicationName?: string
  resource?: Resource | null
}

export function syncAssetAIContext(input: AssetAIContextInput) {
  const store = useAIContextStore()
  const applicationId = (input.applicationId ?? '').trim()
  const resource = input.resource
  if (!applicationId && !resource?.id) {
    store.clearResourceContext('assets', '资产管理')
    return
  }
  const pageTitle = resource?.id
    ? `资源 · ${resource.name || resource.id}`
    : `应用 · ${input.applicationName || applicationId}`
  store.setPageContext({
    page: 'assets',
    page_title: pageTitle,
    application_id: applicationId || undefined,
    resource_id: resource?.id,
    // 云同步资源优先使用云资源类型（ecs/rds/...），回落到注册时的 resource_type。
    resource_type: resource?.cloud_resource_type || resource?.resource_type
  })
}

export function clearAssetAIContext() {
  useAIContextStore().clearResourceContext('assets', '资产管理')
}