<template>
  <a-drawer
    v-model:visible="visible"
    :width="420"
    :title="node?.name || '节点详情'"
    unmount-on-close
  >
    <template v-if="node">
      <div class="node-summary">
        <a-tag :color="node.entity_type === 'application' ? 'arcoblue' : 'gray'">
          {{ resourceTypeLabel(node) }}
        </a-tag>
        <a-tag
          v-if="node.entity_type === 'resource'"
          :color="node.sync_status === 'stale' ? 'orange' : 'green'"
        >
          {{ node.sync_status || 'active' }}
        </a-tag>
      </div>
      <a-descriptions
        :column="1"
        bordered
        size="small"
      >
        <a-descriptions-item label="业务 ID">
          <span class="mono">{{ node.entity_id }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="应用 ID">
          <span class="mono">{{ node.application_id || '—' }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="环境">{{ node.environment || '—' }}</a-descriptions-item>
        <a-descriptions-item label="资源类型">{{ node.resource_type || '—' }}</a-descriptions-item>
        <a-descriptions-item label="云资源类型">{{ node.cloud_resource_type || '—' }}</a-descriptions-item>
        <a-descriptions-item label="云资源 ID">
          <span class="mono">{{ node.cloud_resource_id || '—' }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="Region">{{ node.region || '—' }}</a-descriptions-item>
        <a-descriptions-item label="来源">{{ node.source || 'registry' }}</a-descriptions-item>
        <a-descriptions-item label="最近同步">{{ formatTopologyTime(node.last_synced_at) }}</a-descriptions-item>
      </a-descriptions>

      <section class="relation-section">
        <h3>上游依赖</h3>
        <div class="relation-tags">
          <a-tag
            v-for="name in upstreamNames"
            :key="name"
          >{{ name }}</a-tag>
          <span
            v-if="!upstreamNames.length"
            class="text-muted"
          >暂无手工上游关系</span>
        </div>
      </section>
      <section class="relation-section">
        <h3>下游依赖</h3>
        <div class="relation-tags">
          <a-tag
            v-for="name in downstreamNames"
            :key="name"
          >{{ name }}</a-tag>
          <span
            v-if="!downstreamNames.length"
            class="text-muted"
          >暂无手工下游关系</span>
        </div>
      </section>
      <a-link @click="emit('open-registry')">前往资产注册表</a-link>
    </template>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { AssetTopologyEdge, AssetTopologyNode } from '@/api/asset-topology'
import { formatTopologyTime, relatedNodeNames, resourceTypeLabel } from '../composables/topologyPresentation'

const props = defineProps<{
  node: AssetTopologyNode | null
  nodes: AssetTopologyNode[]
  edges: AssetTopologyEdge[]
}>()

const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{
  'open-registry': []
}>()

const upstreamNames = computed(() => relatedNames('upstream'))
const downstreamNames = computed(() => relatedNames('downstream'))

function relatedNames(direction: 'upstream' | 'downstream') {
  if (!props.node) return []
  const names = new Map(props.nodes.map((node) => [node.node_id, node.name]))
  return relatedNodeNames(props.node.node_id, props.edges, direction).map((id) => names.get(id) || id)
}
</script>

<style scoped>
.node-summary {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.relation-section {
  margin: 20px 0;
}

.relation-section h3 {
  margin: 0 0 10px;
  font-size: 13px;
  font-weight: 600;
}

.relation-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.mono {
  overflow-wrap: anywhere;
}
</style>
