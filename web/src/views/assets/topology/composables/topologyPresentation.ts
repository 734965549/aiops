import type { AssetTopologyEdge, AssetTopologyNode, ResourceRelationType } from '@/api/asset-topology'

export const relationTypeOptions: Array<{ label: string; value: ResourceRelationType }> = [
  { label: '依赖（depends_on）', value: 'depends_on' },
  { label: '挂载（attached_to）', value: 'attached_to' },
  { label: '位于（located_in）', value: 'located_in' },
  { label: '路由到（routes_to）', value: 'routes_to' }
]

export function relationTypeLabel(type: string) {
  if (type === 'contains') return '归属'
  return relationTypeOptions.find((item) => item.value === type)?.label.split('（')[0] || type
}

export function resourceTypeLabel(node: AssetTopologyNode) {
  if (node.entity_type === 'application') return '应用'
  return node.cloud_resource_type || node.resource_type || '资源'
}

export function nodeTypeColor(type: string) {
  const colors: Record<string, string> = {
    application: '#2f6fed',
    ecs: '#7c3aed',
    rds: '#0891b2',
    evs: '#d97706',
    vpc: '#059669',
    elb: '#dc2626'
  }
  return colors[type] || '#64748b'
}

export function topologyResourceNodes(nodes: AssetTopologyNode[]) {
  return nodes.filter((node) => node.entity_type === 'resource')
}

export function relatedNodeNames(nodeId: string, edges: AssetTopologyEdge[], direction: 'upstream' | 'downstream') {
  return edges
    .filter((edge) => edge.relation_type !== 'contains')
    .filter((edge) => direction === 'upstream' ? edge.target_node_id === nodeId : edge.source_node_id === nodeId)
    .map((edge) => direction === 'upstream' ? edge.source_node_id : edge.target_node_id)
}

export function formatTopologyTime(timestamp?: number) {
  if (!timestamp) return '—'
  return new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
}
