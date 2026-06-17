import { http } from './request'

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

export function listApplications() {
  return http<{ items: Application[] }>({
    url: '/api/assets/applications',
    method: 'get'
  })
}

export function createApplication(input: CreateApplicationInput) {
  return http<Application>({
    url: '/api/assets/applications',
    method: 'post',
    data: input
  })
}

export function listResources(applicationId: string) {
  return http<{ items: Resource[] }>({
    url: `/api/assets/applications/${encodeURIComponent(applicationId)}/resources`,
    method: 'get'
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

export function listMatchRules() {
  return http<{ items: MatchRule[] }>({
    url: '/api/assets/match-rules',
    method: 'get'
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
