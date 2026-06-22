import { http } from './request'
import type { PageResult } from './audit'

export interface PolicyScope {
  environment?: string
  account_id: string
  provider?: string
  application_ids?: string[]
  resource_types?: string[]
}

export interface InspectionPolicy {
  policy_id: string
  name: string
  enabled: boolean
  schedule?: string
  scope: PolicyScope
  checks: string[]
  agent_profile?: string
  notification_policy_id?: string
  created_at: number
  updated_at: number
}

export interface TimelineEvent {
  ts: number
  event: string
  detail?: string
}

export interface InspectionRun {
  run_id: string
  policy_id: string
  status: string
  trigger_type: string
  summary?: string
  timeline?: TimelineEvent[]
  started_at?: number
  finished_at?: number
  created_at: number
  updated_at: number
}

export interface AffectedResource {
  type: string
  id: string
  name?: string
}

export interface Recommendation {
  recommendation_id: string
  finding_id: string
  run_id: string
  title: string
  reason?: string
  suggested_action?: string
  risk_level: string
  status: string
  can_create_execution: boolean
  confidence: number
  uncertainty?: string
  created_at: number
}

export interface InspectionFinding {
  finding_id: string
  run_id: string
  policy_id: string
  risk_level: string
  category?: string
  summary: string
  detail?: string
  affected_resources?: AffectedResource[]
  evidence_refs: string[]
  recommendations?: Recommendation[]
  confidence: number
  uncertainty?: string
  created_at: number
}

export interface CreatePolicyInput {
  policy_id?: string
  name: string
  enabled?: boolean
  schedule?: string
  scope: PolicyScope
  checks: string[]
  agent_profile?: string
  notification_policy_id?: string
}

export function listPolicies(params?: { page?: number; page_size?: number; enabled?: boolean; keyword?: string }) {
  return http<PageResult<InspectionPolicy>>({
    url: '/api/inspections/policies',
    method: 'get',
    params
  })
}

export function getPolicy(policyId: string) {
  return http<InspectionPolicy>({
    url: `/api/inspections/policies/${policyId}`,
    method: 'get'
  })
}

export function createPolicy(input: CreatePolicyInput) {
  return http<InspectionPolicy>({
    url: '/api/inspections/policies',
    method: 'post',
    data: input
  })
}

export function updatePolicy(policyId: string, input: Partial<CreatePolicyInput>) {
  return http<InspectionPolicy>({
    url: `/api/inspections/policies/${policyId}`,
    method: 'put',
    data: input
  })
}

export function deletePolicy(policyId: string) {
  return http<{ policy_id: string }>({
    url: `/api/inspections/policies/${policyId}`,
    method: 'delete'
  })
}

export function triggerRun(policyId: string) {
  return http<InspectionRun>({
    url: `/api/inspections/policies/${policyId}/runs`,
    method: 'post'
  })
}

export function getRun(runId: string) {
  return http<InspectionRun>({
    url: `/api/inspections/runs/${runId}`,
    method: 'get'
  })
}

export function listRuns(params?: { page?: number; page_size?: number; policy_id?: string; status?: string }) {
  return http<PageResult<InspectionRun>>({
    url: '/api/inspections/runs',
    method: 'get',
    params
  })
}

export function listFindings(params?: {
  page?: number
  page_size?: number
  run_id?: string
  policy_id?: string
  risk_level?: string
}) {
  return http<PageResult<InspectionFinding>>({
    url: '/api/inspections/findings',
    method: 'get',
    params
  })
}
