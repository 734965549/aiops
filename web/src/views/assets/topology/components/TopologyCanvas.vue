<template>
  <div class="topology-canvas-shell">
    <div
      ref="containerRef"
      class="topology-graph"
      aria-label="CMDB 资源拓扑画布"
    />
    <a-alert
      v-if="canvasError"
      type="error"
      class="canvas-error"
    >
      {{ canvasError }}
    </a-alert>
    <div class="canvas-legend">
      <span><i class="legend-application" />应用</span>
      <span><i class="legend-resource" />资源</span>
      <span><i class="legend-manual" />手工关系</span>
      <span><i class="legend-stale" />Stale</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Graph, GraphData, IElementEvent } from '@antv/g6'
import type { AssetTopologyEdge, AssetTopologyNode } from '@/api/asset-topology'
import { nodeTypeColor, relationTypeLabel, resourceTypeLabel } from '../composables/topologyPresentation'

const props = withDefaults(defineProps<{
  nodes: AssetTopologyNode[]
  edges: AssetTopologyEdge[]
  direction?: 'LR' | 'TB'
}>(), {
  direction: 'LR'
})

const emit = defineEmits<{
  select: [nodeId: string]
}>()

const containerRef = ref<HTMLElement | null>(null)
const canvasError = ref('')
let graph: Graph | null = null

function graphData(): GraphData {
  return {
    nodes: props.nodes.map((node) => ({
      id: node.node_id,
      data: {
        name: node.name,
        entityType: node.entity_type,
        resourceType: resourceTypeLabel(node),
        syncStatus: node.sync_status || 'active'
      }
    })),
    edges: props.edges.map((edge) => ({
      id: edge.edge_id,
      source: edge.source_node_id,
      target: edge.target_node_id,
      data: { relationType: edge.relation_type, origin: edge.origin }
    }))
  }
}

async function renderGraph() {
  if (!graph) return
  const instance = graph
  instance.setData(graphData())
  instance.setLayout({ type: 'dagre', rankdir: props.direction, nodesep: 36, ranksep: 88 })
  await instance.render()
}

onMounted(async () => {
  if (!containerRef.value) return
  try {
    const { Graph: G6Graph, NodeEvent } = await import('@antv/g6')
    graph = new G6Graph({
      container: containerRef.value,
      autoResize: true,
      autoFit: 'view',
      padding: 40,
      zoomRange: [0.2, 2.5],
      animation: false,
      data: graphData(),
      layout: { type: 'dagre', rankdir: props.direction, nodesep: 36, ranksep: 88 },
      node: {
        type: 'rect',
        style: {
          size: [190, 62],
          radius: 6,
          lineWidth: 1.2,
          fill: (node) => {
            if (node.data?.entityType === 'application') return '#eff4ff'
            if (node.data?.syncStatus === 'stale') return '#fff7ed'
            return '#ffffff'
          },
          stroke: (node) => {
            if (node.data?.entityType === 'application') return '#2f6fed'
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
          highlight: { lineWidth: 2, shadowColor: '#7aa7ff', shadowBlur: 8 }
        }
      },
      edge: {
        type: 'line',
        style: {
          stroke: (edge) => edge.data?.origin === 'manual' ? '#2f6fed' : '#b8c2cf',
          lineWidth: (edge) => edge.data?.origin === 'manual' ? 1.6 : 1,
          endArrow: true,
          labelText: (edge) => edge.data?.origin === 'manual' ? relationTypeLabel(String(edge.data?.relationType)) : '',
          labelFontSize: 10,
          labelFill: '#475467',
          labelBackground: true,
          labelBackgroundFill: '#ffffff',
          labelBackgroundOpacity: 0.9,
          labelPadding: [2, 4]
        }
      },
      behaviors: [
        { type: 'zoom-canvas', key: 'zoom-canvas' },
        { type: 'drag-canvas', key: 'drag-canvas' },
        { type: 'click-select', key: 'click-select', multiple: false },
        { type: 'hover-activate', key: 'hover-activate', degree: 1 }
      ],
      plugins: props.nodes.length > 8
        ? [{ type: 'minimap', key: 'minimap', size: [160, 100], position: 'right-bottom' }]
        : []
    })
    graph.on(NodeEvent.CLICK, (event: IElementEvent) => emit('select', String(event.target.id)))
    await graph.render()
  } catch {
    canvasError.value = '画布初始化失败，请切换到列表视图。'
  }
})

watch(
  () => [props.nodes, props.edges, props.direction],
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

.canvas-error {
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

.legend-application { background: #2f6fed; }
.legend-resource { background: #64748b; }
.legend-manual { width: 14px !important; height: 2px !important; background: #2f6fed; }
.legend-stale { background: #ff7d00; }
</style>
