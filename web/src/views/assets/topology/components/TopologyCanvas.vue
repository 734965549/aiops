<template>
  <div class="topology-canvas-shell">
    <div
      ref="containerRef"
      class="topology-graph"
      :aria-label="mode === 'chain' ? 'CMDB 上下游链路画布' : 'CMDB 全景关系网络'"
    />
    <a-alert
      v-if="canvasError"
      type="error"
      class="canvas-error"
    >
      {{ canvasError }}
    </a-alert>
    <a-alert
      v-if="showChainEmpty"
      type="info"
      class="canvas-empty"
    >
      当前根资源暂无上下游关系，可新建关系或切换全景查看归属。
    </a-alert>
    <div class="canvas-legend">
      <template v-if="mode === 'chain'">
        <span><i class="legend-root" />根资源</span>
        <span><i class="legend-upstream" />上游</span>
        <span><i class="legend-downstream" />下游</span>
      </template>
      <template v-else>
        <span><i class="legend-root" />应用</span>
        <span><i class="legend-upstream" />资源</span>
      </template>
      <span><i class="legend-manual" />手工关系</span>
      <span><i class="legend-cloud" />云派生关系</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Graph, GraphData, IElementEvent } from '@antv/g6'
import type { AssetTopologyEdge, AssetTopologyNode } from '@/api/asset-topology'
import { buildChainLayout, layoutChainRows, traceRelations } from '../composables/topologyChain'
import { nodeTypeColor, relationTypeLabel, resourceTypeLabel } from '../composables/topologyPresentation'

const props = withDefaults(defineProps<{
  nodes: AssetTopologyNode[]
  edges: AssetTopologyEdge[]
  mode?: 'chain' | 'network'
  rootResourceId?: string
  direction?: 'LR' | 'TB'
}>(), {
  mode: 'chain',
  rootResourceId: '',
  direction: 'LR'
})

const emit = defineEmits<{
  select: [nodeId: string]
}>()

const containerRef = ref<HTMLElement | null>(null)
const canvasError = ref('')
let graph: Graph | null = null

const showChainEmpty = computed(() => {
  if (props.mode !== 'chain' || !props.rootResourceId) return false
  return !props.edges.some((edge) => edge.relation_type !== 'contains')
})

function chainPositions() {
  return layoutChainRows(buildChainLayout(props.edges, props.rootResourceId))
}

function graphData(): GraphData {
  const positions = props.mode === 'chain' ? chainPositions() : new Map()
  const nodes = props.mode === 'chain'
    ? props.nodes.filter((node) => node.entity_type === 'resource')
    : props.nodes
  const edges = props.mode === 'chain'
    ? props.edges.filter((edge) => edge.relation_type !== 'contains')
    : props.edges
  return {
    nodes: nodes.map((node) => ({
      id: node.node_id,
      data: {
        name: node.name,
        entityType: node.entity_type,
        resourceType: resourceTypeLabel(node),
        syncStatus: node.sync_status || 'active',
        role: positions.get(node.node_id)?.role || 'network'
      }
    })),
    edges: edges.map((edge) => ({
      id: edge.edge_id,
      source: edge.source_node_id,
      target: edge.target_node_id,
      data: { relationType: edge.relation_type, origin: edge.origin }
    }))
  }
}

function layoutOptions() {
  if (props.mode === 'chain') return { type: 'cmdb-chain', animation: false }
  return {
    type: 'force',
    preventOverlap: true,
    nodeSize: [190, 62],
    linkDistance: 150,
    animation: false
  }
}

async function renderGraph() {
  if (!graph) return
  const instance = graph
  instance.setData(graphData())
  instance.setLayout(layoutOptions())
  await instance.render()
}

async function highlightRelations(nodeId: string) {
  if (!graph) return
  const trace = traceRelations(props.edges, nodeId)
  const states: Record<string, string[]> = {}
  for (const node of graphData().nodes || []) states[String(node.id)] = trace.nodes.has(String(node.id)) ? ['highlight'] : []
  for (const edge of graphData().edges || []) states[String(edge.id)] = trace.edgeIds.has(String(edge.id)) ? ['highlight'] : []
  await graph.setElementState(states)
}

async function clearHighlight() {
  if (!graph) return
  const states: Record<string, string[]> = {}
  for (const node of graphData().nodes || []) states[String(node.id)] = []
  for (const edge of graphData().edges || []) states[String(edge.id)] = []
  await graph.setElementState(states)
}

onMounted(async () => {
  if (!containerRef.value) return
  try {
    const {
      Graph: G6Graph,
      NodeEvent,
      CanvasEvent,
      BaseLayout,
      ExtensionCategory,
      register
    } = await import('@antv/g6')

    class CMDBChainLayout extends BaseLayout {
      id = 'cmdb-chain'

      async execute(model: GraphData): Promise<GraphData> {
        const positions = chainPositions()
        return {
          ...model,
          nodes: (model.nodes || []).map((node) => {
            const position = positions.get(String(node.id)) || { x: 0, y: 0 }
            const x = props.direction === 'LR' ? position.x : position.y * 2.2
            const y = props.direction === 'LR' ? position.y : position.x
            return { ...node, style: { ...node.style, x, y } }
          })
        }
      }
    }
    register(ExtensionCategory.LAYOUT, 'cmdb-chain', CMDBChainLayout)

    graph = new G6Graph({
      container: containerRef.value,
      autoResize: true,
      autoFit: 'view',
      padding: 40,
      zoomRange: [0.2, 2.5],
      animation: false,
      data: graphData(),
      layout: layoutOptions(),
      node: {
        type: 'rect',
        style: {
          size: [190, 62],
          radius: 6,
          lineWidth: 1.2,
          fill: (node) => {
            if (node.data?.role === 'root' || node.data?.entityType === 'application') return '#eff4ff'
            if (node.data?.role === 'downstream') return '#f0fdf4'
            if (node.data?.syncStatus === 'stale') return '#fff7ed'
            return '#ffffff'
          },
          stroke: (node) => {
            if (node.data?.role === 'root' || node.data?.entityType === 'application') return '#2f6fed'
            if (node.data?.syncStatus === 'stale') return '#ff7d00'
            return nodeTypeColor(String(node.data?.resourceType || ''))
          },
          labelText: (node) => `${String(node.data?.name || node.id)}\n${String(node.data?.resourceType || '')}`,
          labelPlacement: 'center',
          labelFontSize: 12,
          labelFontWeight: 500,
          labelFill: '#1d2939',
          labelMaxWidth: 164,
          labelWordWrap: true,
          labelMaxLines: 2,
          icon: false,
          port: false
        },
        state: {
          selected: { lineWidth: 2.4, stroke: '#2f6fed', shadowColor: '#2f6fed', shadowBlur: 10 },
          highlight: { lineWidth: 2.2, stroke: '#2f6fed', shadowColor: '#7aa7ff', shadowBlur: 10 }
        }
      },
      edge: {
        type: 'line',
        style: {
          stroke: (edge) => edge.data?.origin === 'manual' ? '#2f6fed' : edge.data?.origin === 'cloud_sync' ? '#0891b2' : '#b8c2cf',
          lineWidth: (edge) => edge.data?.origin === 'contains' ? 1 : 1.5,
          endArrow: true,
          labelText: (edge) => edge.data?.origin === 'contains' ? '' : relationTypeLabel(String(edge.data?.relationType)),
          labelFontSize: 10,
          labelFill: '#475467',
          labelBackground: true,
          labelBackgroundFill: '#ffffff',
          labelBackgroundOpacity: 0.9,
          labelPadding: [2, 4]
        },
        state: {
          highlight: { stroke: '#2f6fed', lineWidth: 2.6 }
        }
      },
      behaviors: [
        { type: 'zoom-canvas', key: 'zoom-canvas' },
        { type: 'drag-canvas', key: 'drag-canvas' },
        { type: 'click-select', key: 'click-select', multiple: false },
        { type: 'hover-activate', key: 'hover-activate', degree: 1 }
      ],
      plugins: props.mode === 'network' && props.nodes.length > 8
        ? [{ type: 'minimap', key: 'minimap', size: [160, 100], position: 'right-bottom' }]
        : []
    })
    graph.on(NodeEvent.CLICK, async (event: IElementEvent) => {
      const nodeId = String(event.target.id)
      await highlightRelations(nodeId)
      emit('select', nodeId)
    })
    graph.on(CanvasEvent.CLICK, () => { void clearHighlight() })
    await graph.render()
  } catch {
    canvasError.value = '画布初始化失败，请切换到列表视图。'
  }
})

watch(
  () => [props.nodes, props.edges, props.mode, props.rootResourceId, props.direction],
  () => { void renderGraph() },
  { deep: true }
)

async function fitView() {
  await graph?.fitView()
}

async function zoomIn() {
  if (graph) await graph.zoomTo(Math.min(graph.getZoom() * 1.2, 2.5))
}

async function zoomOut() {
  if (graph) await graph.zoomTo(Math.max(graph.getZoom() / 1.2, 0.2))
}

onBeforeUnmount(() => {
  graph?.destroy()
  graph = null
})

defineExpose({ fitView, zoomIn, zoomOut, relayout: renderGraph })
</script>

<style scoped>
.topology-canvas-shell {
  position: relative;
  height: 100%;
  min-height: 420px;
  overflow: hidden;
  border: 1px solid var(--aiops-border-soft);
  border-radius: 6px;
  background-color: #f8fafc;
  background-image: radial-gradient(#d7dee8 0.7px, transparent 0.7px);
  background-size: 18px 18px;
}

.topology-graph {
  width: 100%;
  height: 100%;
}

.canvas-error,
.canvas-empty {
  position: absolute;
  top: 12px;
  right: 12px;
  left: 12px;
}

.canvas-legend {
  position: absolute;
  bottom: 12px;
  left: 12px;
  display: flex;
  gap: 12px;
  padding: 7px 10px;
  border: 1px solid var(--aiops-border);
  border-radius: 6px;
  color: var(--aiops-text-3);
  background: rgba(255, 255, 255, 0.94);
  font-size: 11px;
}

.canvas-legend span {
  display: flex;
  align-items: center;
  gap: 5px;
}

.canvas-legend i {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 2px;
}

.legend-root { background: #2f6fed; }
.legend-upstream { background: #64748b; }
.legend-downstream { background: #059669; }
.legend-manual { width: 14px !important; height: 2px !important; background: #2f6fed; }
.legend-cloud { width: 14px !important; height: 2px !important; background: #0891b2; }
</style>
