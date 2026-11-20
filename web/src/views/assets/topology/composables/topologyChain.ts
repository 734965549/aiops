import type { AssetTopologyEdge } from '@/api/asset-topology'

export interface ChainPosition {
  x: number
  y: number
  role: 'root' | 'upstream' | 'downstream'
}

export function buildChainLayout(edges: AssetTopologyEdge[], rootResourceId: string): Map<string, ChainPosition> {
  const rootId = `resource:${rootResourceId}`
  const positions = new Map<string, ChainPosition>([[rootId, { x: 0, y: 0, role: 'root' }]])
  const relationEdges = edges.filter((edge) => edge.relation_type !== 'contains')
  walk('upstream')
  walk('downstream')
  return positions

  function walk(direction: 'upstream' | 'downstream') {
    const queue = [rootId]
    const depths = new Map([[rootId, 0]])
    while (queue.length) {
      const current = queue.shift()!
      const currentDepth = depths.get(current)!
      for (const edge of relationEdges) {
        const neighbor = direction === 'upstream'
          ? edge.target_node_id === current ? edge.source_node_id : ''
          : edge.source_node_id === current ? edge.target_node_id : ''
        if (!neighbor || depths.has(neighbor)) continue
        const depth = currentDepth + 1
        depths.set(neighbor, depth)
        positions.set(neighbor, {
          x: direction === 'upstream' ? -depth * 250 : depth * 250,
          y: 0,
          role: direction
        })
        queue.push(neighbor)
      }
    }
  }
}

export function layoutChainRows(positions: Map<string, ChainPosition>) {
  const groups = new Map<number, string[]>()
  for (const [nodeId, position] of positions) {
    const nodes = groups.get(position.x) || []
    nodes.push(nodeId)
    groups.set(position.x, nodes)
  }
  for (const nodeIds of groups.values()) {
    nodeIds.sort()
    nodeIds.forEach((nodeId, index) => {
      const position = positions.get(nodeId)!
      position.y = (index - (nodeIds.length - 1) / 2) * 105
    })
  }
  return positions
}

export function traceRelations(edges: AssetTopologyEdge[], selectedNodeId: string) {
  const nodes = new Set<string>([selectedNodeId])
  const visited = new Set<string>()
  const edgeIds = new Set<string>()
  visit(selectedNodeId, 'upstream')
  visit(selectedNodeId, 'downstream')
  return { nodes, edgeIds }

  function visit(current: string, direction: 'upstream' | 'downstream') {
    for (const edge of edges) {
      if (edge.relation_type === 'contains') continue
      const matches = direction === 'upstream' ? edge.target_node_id === current : edge.source_node_id === current
      if (!matches) continue
      edgeIds.add(edge.edge_id)
      const neighbor = direction === 'upstream' ? edge.source_node_id : edge.target_node_id
      const key = `${direction}:${neighbor}`
      if (visited.has(key)) continue
      visited.add(key)
      nodes.add(neighbor)
      visit(neighbor, direction)
    }
  }
}
