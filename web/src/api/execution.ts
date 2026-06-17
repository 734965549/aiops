import { http } from './request'

export type ExecutionStatus =
  | 'pending_confirm'
  | 'pending_execute'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled'

export type ExecutionSourceType = 'alert' | 'manual' | 'ai_conversation'
export type ExecutionOperationType = 'restart' | 'scale' | 'script' | 'runbook' | 'custom'

export interface ExecutionTask {
  id: string
  name: string
  source_type: ExecutionSourceType | string
  source_id?: string
  operation_type: ExecutionOperationType | string
  target_type?: string
  target_id?: string
  target_name?: string
  environment?: string
  risk_level: string
  status: ExecutionStatus | string
  parameters: Record<string, unknown>
  rollback_plan?: Record<string, unknown>
  runbook_template_id?: string
  runbook_name?: string
  dry_run?: boolean
  result_summary?: string
  error_message?: string
  created_by?: string
  confirmed_by?: string
  executed_by?: string
  created_at: number
  confirmed_at?: number
  started_at?: number
  finished_at?: number
}

export interface ExecutionStep {
  id: string
  task_id: string
  step_order: number
  name: string
  action_type: string
  status: string
  risk_level?: string
  dry_run?: boolean
  parameters?: Record<string, unknown>
  rollback_plan?: Record<string, unknown>
  output: Record<string, unknown>
  error_message?: string
  started_at?: number
  finished_at?: number
}

export interface ExecutionTaskDetail {
  task: ExecutionTask
  steps: ExecutionStep[]
}

export interface CreateExecutionTaskInput {
  name?: string
  source_type: ExecutionSourceType | string
  source_id?: string
  operation_type?: ExecutionOperationType | string
  target_type?: string
  target_id?: string
  target_name?: string
  environment?: string
  parameters?: Record<string, unknown>
  rollback_plan?: Record<string, unknown>
  risk_level?: string
  runbook_template_id?: string
  dry_run?: boolean
}

export interface CreateExecutionTaskResult {
  task_id: string
  status: string
  risk_level: string
  confirm_url?: string
}

export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface ExecutionListQuery {
  page?: number
  page_size?: number
  status?: string
  source_type?: string
  source_id?: string
  keyword?: string
}

export function listExecutionTasks(query: ExecutionListQuery = {}) {
  return http<PageResult<ExecutionTask>>({
    url: '/api/executions/tasks',
    method: 'get',
    params: query
  })
}

export function getExecutionTask(taskId: string) {
  return http<ExecutionTaskDetail>({
    url: `/api/executions/tasks/${encodeURIComponent(taskId)}`,
    method: 'get'
  })
}

export function createExecutionTask(input: CreateExecutionTaskInput) {
  return http<CreateExecutionTaskResult>({
    url: '/api/executions/tasks',
    method: 'post',
    data: input
  })
}

export function confirmExecutionTask(taskId: string, confirmText: string) {
  return http<ExecutionTask>({
    url: `/api/executions/tasks/${encodeURIComponent(taskId)}/confirm`,
    method: 'post',
    data: { confirm: true, confirm_text: confirmText }
  })
}

export function executeTask(taskId: string) {
  return http<ExecutionTaskDetail>({
    url: `/api/executions/tasks/${encodeURIComponent(taskId)}/execute`,
    method: 'post',
    data: {}
  })
}
