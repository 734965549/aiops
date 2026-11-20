import { describe, expect, it } from 'vitest'
import type { AssetTopologyEdge, AssetTopologyNode } from '@/api/asset-topology'
import { relatedNodeNames, relationTypeLabel, resourceTypeLabel, topologyResourceNodes } from './topologyPresentation'

const application: AssetTopologyNode = {
  node_id: 'application:app-1', entity_type: 'application', entity_id: 'app-1', name: 'payment'
}
const gateway: AssetTopologyNode = {
  node_id: 'resource:res-1', entity_type: 'resource', entity_id: 'res-1', name: 'gateway', resource_type: 'service'
}
const database: AssetTopologyNode = {
  node_id: 'resource:res-2', entity_type: 'resource', entity_id: 'res-2', name: 'database', cloud_resource_type: 'rds'
}

const edges: AssetTopologyEdge[] = [
  {
    edge_id: 'contains:res-1', source_node_id: application.node_id, target_node_id: gateway.node_id,
    relation_type: 'contains', origin: 'registry'
  },
  {
    edge_id: 'relation:rel-1', source_node_id: gateway.node_id, target_node_id: database.node_id,
    relation_type: 'depends_on', origin: 'manual'
  }
]

describe('topology presentation', () => {
  it('keeps applications out of resource selectors', () => {
    expect(topologyResourceNodes([application, gateway, database])).toEqual([gateway, database])
  })

  it('uses cloud resource type before generic resource type', () => {
    expect(resourceTypeLabel(application)).toBe('应用')
    expect(resourceTypeLabel(database)).toBe('rds')
    expect(resourceTypeLabel(gateway)).toBe('service')
  })

  it('maps persisted and derived relation labels', () => {
    expect(relationTypeLabel('contains')).toBe('归属')
    expect(relationTypeLabel('depends_on')).toBe('依赖')
    expect(relationTypeLabel('unknown')).toBe('unknown')
  })

  it('returns only manual upstream and downstream neighbors', () => {
    expect(relatedNodeNames(gateway.node_id, edges, 'downstream')).toEqual([database.node_id])
    expect(relatedNodeNames(gateway.node_id, edges, 'upstream')).toEqual([])
    expect(relatedNodeNames(database.node_id, edges, 'upstream')).toEqual([gateway.node_id])
  })
})
