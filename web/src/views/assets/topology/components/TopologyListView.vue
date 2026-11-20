<template>
  <a-tabs class="topology-list-tabs">
    <a-tab-pane key="resources" title="资源节点">
      <a-table
        :columns="resourceColumns"
        :data="resources"
        row-key="node_id"
        :pagination="false"
        size="small"
        @row-click="(record: TableData) => emit('select', record as AssetTopologyNode)"
      >
        <template #type="{ record }">
          <a-tag>{{ resourceTypeLabel(record as AssetTopologyNode) }}</a-tag>
        </template>
        <template #sync="{ record }">
          <a-tag :color="record.sync_status === 'stale' ? 'orange' : 'green'">
            {{ record.sync_status || 'active' }}
          </a-tag>
        </template>
      </a-table>
    </a-tab-pane>
    <a-tab-pane
      key="relations"
      :title="`手工关系 (${relations.length})`"
    >
      <a-table
        :columns="relationColumns"
        :data="relations"
        :loading="relationsLoading"
        row-key="relation_id"
        :pagination="false"
        size="small"
      >
        <template #type="{ record }">
          <a-tag color="arcoblue">{{ relationTypeLabel(record.relation_type) }}</a-tag>
        </template>
        <template #actions="{ record }">
          <a-popconfirm
            content="确定删除这条手工关系？"
            @ok="emit('delete', record.relation_id)"
          >
            <a-button
              type="text"
              size="mini"
              status="danger"
              @click.stop
            >
              删除
            </a-button>
          </a-popconfirm>
        </template>
      </a-table>
    </a-tab-pane>
  </a-tabs>
</template>

<script setup lang="ts">
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { AssetResourceRelation, AssetTopologyNode } from '@/api/asset-topology'
import { relationTypeLabel, resourceTypeLabel } from '../composables/topologyPresentation'

defineProps<{
  resources: AssetTopologyNode[]
  relations: AssetResourceRelation[]
  relationsLoading: boolean
}>()

const emit = defineEmits<{
  select: [node: AssetTopologyNode]
  delete: [relationId: string]
}>()

const resourceColumns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: '类型', slotName: 'type', width: 110 },
  { title: 'Region', dataIndex: 'region', width: 130 },
  { title: '同步状态', slotName: 'sync', width: 100 },
  { title: '资源 ID', dataIndex: 'entity_id', ellipsis: true }
]

const relationColumns = [
  { title: '源资源', dataIndex: 'source_name', ellipsis: true },
  { title: '关系', slotName: 'type', width: 110 },
  { title: '目标资源', dataIndex: 'target_name', ellipsis: true },
  { title: '来源', dataIndex: 'origin', width: 90 },
  { title: '操作', slotName: 'actions', width: 80 }
]
</script>

<style scoped>
.topology-list-tabs {
  min-height: 0;
}

:deep(.arco-table-tr) {
  cursor: pointer;
}
</style>
