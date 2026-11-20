import { http } from './request'

export type ResourceRelationType = 'depends_on' | 'attached_to' | 'located_in' | 'routes_to'

export interface AssetTopologyNode {
  node_id: string
  entity_type: 'application' | 'resource'
  entity_id: string
  name: string
  application_id?: string
  environment?: string
  resource_type?: string
  cloud_resource_type?: string
  cloud_resource_id?: string
  region?: string
  sync_status?: string
  source?: string
  last_synced_at?: number
}

export interface AssetTopologyEdge {
  edge_id: string
  source_node_id: string
  target_node_id: string
  relation_type: 'contains' | ResourceRelationType
  origin: 'registry' | 'manual'
}

export interface AssetTopologySnapshot {
  nodes: AssetTopologyNode[]
  edges: AssetTopologyEdge[]
  meta: {
    generated_at: number
    truncated: boolean
    node_count: number
    edge_count: number
  }
}

export interface AssetTopologyQuery {
  application_id: string
  region?: string
  cloud_resource_type?: string
  relation_type?: ResourceRelationType
  keyword?: string
  include_stale?: boolean
}

export interface AssetResourceRelation {
  relation_id: string
  source_resource_id: string
  source_name: string
  target_resource_id: string
  target_name: string
  relation_type: ResourceRelationType
  origin: 'manual'
  created_at: number
  updated_at: number
}

export interface CreateAssetResourceRelationInput {
  source_resource_id: string
  target_resource_id: string
  relation_type: ResourceRelationType
}

export function getAssetTopology(params: AssetTopologyQuery) {
  return http<AssetTopologySnapshot>({
    url: '/api/assets/topology',
    method: 'get',
    params
  })
}

export function listAssetResourceRelations(params: { application_id: string; relation_type?: ResourceRelationType }) {
  return http<{ items: AssetResourceRelation[]; total: number }>({
    url: '/api/assets/resource-relations',
    method: 'get',
    params
  })
}

export function createAssetResourceRelation(data: CreateAssetResourceRelationInput) {
  return http<AssetResourceRelation>({
    url: '/api/assets/resource-relations',
    method: 'post',
    data
  })
}

export function deleteAssetResourceRelation(relationId: string) {
  return http<{ deleted: boolean }>({
    url: `/api/assets/resource-relations/${encodeURIComponent(relationId)}`,
    method: 'delete'
  })
}
