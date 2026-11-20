import { describe, expect, it } from 'vitest'
import type { AssetTopologyEdge } from '@/api/asset-topology'
import { buildChainLayout, layoutChainRows, traceRelations } from './topologyChain'

const edges: AssetTopologyEdge[] = [
  { edge_id: 'e1', source_node_id: 'resource:a', target_node_id: 'resource:b', relation_type: 'depends_on', origin: 'manual' },
  { edge_id: 'e2', source_node_id: 'resource:b', target_node_id: 'resource:c', relation_type: 'depends_on', origin: 'manual' },
  { edge_id: 'e3', source_node_id: 'resource:c', target_node_id: 'resource:d', relation_type: 'located_in', origin: 'cloud_sync' },
  { edge_id: 'e4', source_node_id: 'resource:x', target_node_id: 'resource:b', relation_type: 'depends_on', origin: 'manual' },
  { edge_id: 'e5', source_node_id: 'resource:c', target_node_id: 'resource:b', relation_type: 'depends_on', origin: 'manual' }
]

describe('topology chain layout', () => {
  it('places upstream and downstream resources around the root', () => {
    const positions = layoutChainRows(buildChainLayout(edges, 'b'))
    expect(positions.get('resource:b')).toEqual({ x: 0, y: 0, role: 'root' })
    expect(positions.get('resource:a')?.role).toBe('upstream')
    expect(positions.get('resource:a')?.x).toBe(-250)
    expect(positions.get('resource:c')?.role).toBe('downstream')
    expect(positions.get('resource:c')?.x).toBe(250)
    expect(positions.get('resource:d')?.x).toBe(500)
    expect(positions.get('resource:x')?.x).toBe(-250)
  })

  it('traces complete paths and remains safe on cyclic dependencies', () => {
    const trace = traceRelations(edges, 'resource:b')
    expect(trace.nodes).toEqual(new Set(['resource:b', 'resource:a', 'resource:x', 'resource:c', 'resource:d']))
    expect(trace.edgeIds).toEqual(new Set(['e1', 'e2', 'e3', 'e4', 'e5']))
  })
})
