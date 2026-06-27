import { http, isApiHttpError } from './request'
import type { PageResult } from './audit'

export interface Application {
  id: string
  name: string
  environment?: string
  namespace?: string
  description?: string
  created_at: number
  updated_at: number
}

export interface Resource {
  id: string
  application_id: string
  name?: string
  resource_type?: string
  namespace?: string
  pod?: string
  node?: string
  instance?: string
  source?: string
  integration_account_id?: string
  cloud_resource_id?: string
  cloud_resource_type?: string
  region?: string
  sync_status?: string
  last_synced_at?: number
  sync_batch_id?: string
  labels?: Record<string, string>
  created_at: number
  updated_at: number
}

export interface CreateApplicationInput {
  id?: string
  name: string
  environment?: string
  namespace?: string
  description?: string
}

export interface CreateResourceInput {
  id?: string
  application_id: string
  name?: string
  resource_type?: string
  namespace?: string
  pod?: string
  node?: string
  instance?: string
}

export interface UpdateResourceInput {
  name?: string
  resource_type?: string
  namespace?: string
  pod?: string
  node?: string
  instance?: string
}

export interface MatchRule {
  id: string
  name: string
  enabled: boolean
  priority: number
  target_type: string
  source_type: string
  label_key: string
  label_value_pattern: string
  application_id: string
  resource_id?: string
  created_at: number
  updated_at: number
}

export interface CreateMatchRuleInput {
  id?: string
  name: string
  enabled?: boolean
  priority?: number
  target_type?: string
  source_type?: string
  label_key: string
  label_value_pattern: string
  application_id: string
  resource_id?: string
}

export interface UpdateMatchRuleInput {
  name: string
  enabled?: boolean
  priority?: number
  target_type?: string
  source_type?: string
  label_key: string
  label_value_pattern: string
  application_id: string
  resource_id?: string
}

export interface UpdateApplicationInput {
  name: string
  environment?: string
  namespace?: string
  description?: string
}

export function listApplications(params?: { page?: number; page_size?: number }) {
  return http<PageResult<Application>>({
    url: '/api/assets/applications',
    method: 'get',
    params
  })
}

export function createApplication(input: CreateApplicationInput) {
  return http<Application>({
    url: '/api/assets/applications',
    method: 'post',
    data: input
  })
}

export function listResources(applicationId: string, params?: { page?: number; page_size?: number }) {
  return http<PageResult<Resource>>({
    url: `/api/assets/applications/${encodeURIComponent(applicationId)}/resources`,
    method: 'get',
    params
  })
}

export function createResource(input: CreateResourceInput) {
  return http<Resource>({
    url: '/api/assets/resources',
    method: 'post',
    data: input
  })
}

export function updateApplication(id: string, input: UpdateApplicationInput) {
  return http<Application>({
    url: `/api/assets/applications/${encodeURIComponent(id)}`,
    method: 'put',
    data: input
  })
}

export function deleteApplication(id: string) {
  return http<{ deleted: boolean }>({
    url: `/api/assets/applications/${encodeURIComponent(id)}`,
    method: 'delete'
  })
}

export function updateResource(id: string, input: UpdateResourceInput) {
  return http<Resource>({
    url: `/api/assets/resources/${encodeURIComponent(id)}`,
    method: 'put',
    data: input
  })
}

export function deleteResource(id: string) {
  return http<{ deleted: boolean }>({
    url: `/api/assets/resources/${encodeURIComponent(id)}`,
    method: 'delete'
  })
}

export function listMatchRules(params?: { page?: number; page_size?: number }) {
  return http<PageResult<MatchRule>>({
    url: '/api/assets/match-rules',
    method: 'get',
    params
  })
}

export function createMatchRule(input: CreateMatchRuleInput) {
  return http<MatchRule>({
    url: '/api/assets/match-rules',
    method: 'post',
    data: input
  })
}

export function updateMatchRule(id: string, input: UpdateMatchRuleInput) {
  return http<MatchRule>({
    url: `/api/assets/match-rules/${encodeURIComponent(id)}`,
    method: 'put',
    data: input
  })
}

export function deleteMatchRule(id: string) {
  return http<{ deleted: boolean }>({
    url: `/api/assets/match-rules/${encodeURIComponent(id)}`,
    method: 'delete'
  })
}

export interface SyncBatch {
  batch_id: string
  integration_account_id: string
  provider: string
  status: string
  created_count: number
  updated_count: number
  stale_count: number
  failed_count: number
  message?: string
  application_id?: string
  started_at: number
  finished_at?: number
  created_at: number
  updated_at: number
}

export interface SyncBatchNotice {
  type: 'success' | 'warning' | 'error'
  content: string
}

export function getSyncBatchNotice(batch: SyncBatch): SyncBatchNotice {
  const summary = `新建 ${batch.created_count}，更新 ${batch.updated_count}，stale ${batch.stale_count}，失败 ${batch.failed_count}`
  const detail = batch.message ? `：${batch.message}` : ''
  switch (batch.status) {
    case 'success':
      return { type: 'success', content: `同步完成：${summary}${detail}` }
    case 'partial':
      return { type: 'warning', content: `同步部分完成：${summary}${detail}` }
    case 'failed':
      return { type: 'error', content: `同步失败：${summary}${detail}` }
    default:
      return { type: 'warning', content: `同步状态 ${batch.status}：${summary}${detail}` }
  }
}

export function triggerAssetSync(accountId: string) {
  return http<SyncBatch>({
    url: '/api/assets/sync',
    method: 'post',
    data: { account_id: accountId }
  })
}

export function isAssetSyncInProgressError(error: unknown) {
  return (
    isApiHttpError(error) &&
    error.status === 409 &&
    error.code === 'ALREADY_EXISTS' &&
    error.message === 'sync already in progress for this account'
  )
}

export function listSyncBatches(params?: { page?: number; page_size?: number; account_id?: string }) {
  return http<PageResult<SyncBatch>>({
    url: '/api/assets/sync/batches',
    method: 'get',
    params
  })
}

export function getSyncBatch(batchId: string) {
  return http<SyncBatch>({
    url: `/api/assets/sync/batches/${encodeURIComponent(batchId)}`,
    method: 'get'
  })
}

/**
 * 同步仍在进行（轮询超过 timeoutMs）时抛出，调用方可提示用户去同步批次页查看。
 */
export class SyncStillRunningError extends Error {
  batchId: string
  constructor(batchId: string) {
    super('同步仍在进行，可在同步批次页查看')
    this.name = 'SyncStillRunningError'
    this.batchId = batchId
  }
}

/**
 * 轮询同步批次直到终态（success/partial/failed）或超时。
 * 触发同步后端立即返回 running 批次，前端用此 helper 轮询 GetSyncBatch 拿终态结果。
 *
 * @param batchId 触发同步返回的 batch_id
 * @param opts.intervalMs 轮询间隔，默认 2000ms
 * @param opts.timeoutMs 轮询上限，默认 600000ms（10min）；超时抛 SyncStillRunningError
 * @param opts.shouldStop 可选取消谓词，返回 true 立即停止轮询并抛错（用于组件卸载取消）
 */
export async function pollSyncBatch(
  batchId: string,
  opts: { intervalMs?: number; timeoutMs?: number; shouldStop?: () => boolean } = {}
): Promise<SyncBatch> {
  const interval = opts.intervalMs ?? 2000
  const deadline = Date.now() + (opts.timeoutMs ?? 600_000)
  while (true) {
    if (opts.shouldStop?.()) {
      throw new Error('polling cancelled')
    }
    if (Date.now() > deadline) {
      throw new SyncStillRunningError(batchId)
    }
    const batch = await getSyncBatch(batchId)
    if (batch.status !== 'running') {
      return batch
    }
    await new Promise((resolve) => setTimeout(resolve, interval))
  }
}
