<template>
  <div class="page-shell topology-page">
    <PageHeader
      title="CMDB 资源拓扑"
      description="展示真实应用资源归属与手工依赖关系，观测调用链请前往观测查询。"
    >
      <template #actions>
        <a-button @click="router.push('/assets')">资产注册表</a-button>
        <a-button
          type="primary"
          :disabled="!resourceNodes.length"
          @click="relationModalVisible = true"
        >
          新建关系
        </a-button>
      </template>
    </PageHeader>

    <a-card
      :bordered="false"
      class="topology-filter-card"
    >
      <a-form
        :model="filters"
        layout="inline"
        class="topology-filters"
      >
        <a-form-item label="应用">
          <a-select
            v-model="selectedApplicationId"
            :loading="applicationsLoading"
            :options="applicationOptions"
            placeholder="选择应用"
            style="width: 220px"
            @change="onApplicationChange"
          />
        </a-form-item>
        <a-form-item label="关键字">
          <a-input
            v-model="filters.keyword"
            allow-clear
            placeholder="名称 / 资源 ID"
            style="width: 180px"
            @press-enter="loadTopology"
          />
        </a-form-item>
        <a-form-item label="Region">
          <a-select
            v-model="filters.region"
            allow-clear
            placeholder="全部"
            style="width: 140px"
            :options="regionOptions"
          />
        </a-form-item>
        <a-form-item label="云类型">
          <a-select
            v-model="filters.cloudResourceType"
            allow-clear
            placeholder="全部"
            style="width: 130px"
            :options="cloudTypeOptions"
          />
        </a-form-item>
        <a-form-item label="关系">
          <a-select
            v-model="filters.relationType"
            allow-clear
            placeholder="全部"
            style="width: 170px"
            :options="relationTypeOptions"
          />
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model="filters.includeStale">包含 Stale</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button
              type="primary"
              :loading="loading"
              @click="loadTopology"
            >查询</a-button>
            <a-button @click="resetFilters">重置</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-alert
      v-if="error"
      type="error"
      class="topology-alert"
      closable
      @close="error = ''"
    >{{ error }}</a-alert>

    <a-card
      :bordered="false"
      class="topology-workbench"
    >
      <div class="topology-toolbar">
        <div class="topology-summary">
          <strong>资源关系图</strong>
          <a-tag>{{ topology?.meta.node_count || 0 }} 节点</a-tag>
          <a-tag>{{ topology?.meta.edge_count || 0 }} 边</a-tag>
          <span v-if="topology?.meta.generated_at">更新于 {{ formatTopologyTime(topology.meta.generated_at) }}</span>
        </div>
        <div class="topology-actions">
          <template v-if="viewMode === 'chain'">
            <a-select
              v-model="rootResourceId"
              allow-search
              size="small"
              style="width: 190px"
              :options="rootOptions"
              placeholder="选择根资源"
              @change="focusTopology"
            />
            <a-select
              v-model="depth"
              size="small"
              style="width: 88px"
              :options="[1, 2, 3].map((value) => ({ label: `${value} 层`, value }))"
              @change="loadTopology"
            />
            <a-select
              v-model="relationDirection"
              size="small"
              style="width: 110px"
              :options="directionOptions"
              @change="loadTopology"
            />
          </template>
          <a-button
            size="small"
            :loading="loading || relationsLoading"
            @click="refreshAll"
          >刷新</a-button>
          <template v-if="viewMode !== 'list'">
            <a-button
              size="small"
              @click="canvasRef?.zoomOut()"
            >缩小</a-button>
            <a-button
              size="small"
              @click="canvasRef?.zoomIn()"
            >放大</a-button>
            <a-button
              size="small"
              @click="canvasRef?.fitView()"
            >适配</a-button>
            <a-button
              v-if="viewMode === 'chain'"
              size="small"
              @click="toggleDirection"
            >{{ direction === 'LR' ? '纵向布局' : '横向布局' }}</a-button>
          </template>
          <a-radio-group
            v-model="viewMode"
            type="button"
            size="small"
            @change="onViewModeChange"
          >
            <a-radio value="chain">链路</a-radio>
            <a-radio value="network">全景</a-radio>
            <a-radio value="list">列表</a-radio>
          </a-radio-group>
        </div>
      </div>

      <a-alert
        v-if="topology?.meta.truncated"
        type="warning"
        class="topology-alert"
      >
        当前应用资源过多，仅展示前 299 个资源；请使用筛选缩小范围。
      </a-alert>

      <a-alert
        v-if="topology?.meta.unresolved_relation_count"
        type="warning"
        class="topology-alert"
      >
        {{ topology.meta.unresolved_relation_count }} 个资源的关系端点未能唯一解析，未生成猜测关系。
      </a-alert>

      <a-spin
        :loading="loading"
        class="topology-spin"
      >
        <a-empty
          v-if="!selectedApplicationId"
          class="page-empty"
          description="暂无应用，请先在资产注册表创建应用"
        />
        <a-empty
          v-else-if="!resourceNodes.length && !loading"
          class="page-empty"
          description="当前应用没有匹配的资源，请调整筛选或先注册资源"
        />
        <TopologyCanvas
          v-else-if="viewMode !== 'list'"
          ref="canvasRef"
          :nodes="topology?.nodes || []"
          :edges="topology?.edges || []"
          :mode="viewMode === 'network' ? 'network' : 'chain'"
          :root-resource-id="rootResourceId"
          :direction="direction"
          @select="selectNode"
        />
        <TopologyListView
          v-else
          :resources="resourceNodes"
          :relations="relations"
          :relations-loading="relationsLoading"
          @select="openNode"
          @delete="removeRelation"
        />
      </a-spin>
    </a-card>

    <TopologyNodeDrawer
      v-model:visible="nodeDrawerVisible"
      :node="selectedNode"
      :nodes="topology?.nodes || []"
      :edges="topology?.edges || []"
      @open-registry="router.push('/assets')"
      @focus-node="focusTopology"
    />
    <RelationFormModal
      v-model:visible="relationModalVisible"
      :applications="applications"
      :current-application-id="selectedApplicationId"
      :loading="saving"
      :submit="submitRelation"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { AssetTopologyNode, CreateAssetResourceRelationInput } from '@/api/asset-topology'
import PageHeader from '@/components/PageHeader.vue'
import TopologyCanvas from './components/TopologyCanvas.vue'
import TopologyListView from './components/TopologyListView.vue'
import TopologyNodeDrawer from './components/TopologyNodeDrawer.vue'
import RelationFormModal from './components/RelationFormModal.vue'
import { useTopologyData } from './composables/useTopologyData'
import { formatTopologyTime, relationTypeOptions } from './composables/topologyPresentation'

const route = useRoute()
const router = useRouter()
const {
  applications,
  applicationsLoading,
  selectedApplicationId,
  rootResourceId,
  depth,
  relationDirection,
  topology,
  relations,
  resourceNodes,
  loading,
  relationsLoading,
  saving,
  error,
  filters,
  filterOptions,
  loadApplications,
  loadTopology,
  refreshAll,
  selectApplication,
  focusResource,
  clearRoot,
  resetFilters,
  createRelation,
  removeRelation
} = useTopologyData()

const viewMode = ref<'chain' | 'network' | 'list'>('chain')
const direction = ref<'LR' | 'TB'>('LR')
const canvasRef = ref<InstanceType<typeof TopologyCanvas> | null>(null)
const selectedNode = ref<AssetTopologyNode | null>(null)
const nodeDrawerVisible = ref(false)
const relationModalVisible = ref(false)

const applicationOptions = computed(() => applications.value.map((app) => ({
  label: `${app.name}${app.environment ? ` · ${app.environment}` : ''}`,
  value: app.id
})))
const regionOptions = computed(() => filterOptions.value.regions.map((value) => ({ label: value, value })))
const cloudTypeOptions = computed(() => filterOptions.value.cloud_resource_types.map((value) => ({ label: value, value })))
const rootOptions = computed(() => resourceNodes.value.map((node) => ({ label: node.name, value: node.entity_id })))
const directionOptions = [
  { label: '双向', value: 'both' },
  { label: '上游', value: 'upstream' },
  { label: '下游', value: 'downstream' }
]

async function onApplicationChange(value: unknown) {
  const applicationId = String(value)
  await selectApplication(applicationId)
  selectedNode.value = null
  await router.replace({ query: { ...route.query, application_id: applicationId } })
}

function selectNode(nodeId: string) {
  const node = topology.value?.nodes.find((item) => item.node_id === nodeId)
  if (node) openNode(node)
}

function openNode(node: AssetTopologyNode) {
  selectedNode.value = node
  nodeDrawerVisible.value = true
}

async function onViewModeChange(value: unknown) {
  const mode = String(value)
  if (mode === 'network') {
    await clearRoot()
  } else if (mode === 'chain' && !rootResourceId.value && resourceNodes.value.length) {
    await focusResource(resourceNodes.value[0].entity_id)
  }
}

async function focusTopology(value: unknown) {
  const resourceId = String(value)
  if (!resourceId) return
  viewMode.value = 'chain'
  nodeDrawerVisible.value = false
  await focusResource(resourceId)
}

function toggleDirection() {
  direction.value = direction.value === 'LR' ? 'TB' : 'LR'
}

async function submitRelation(input: CreateAssetResourceRelationInput) {
  const created = await createRelation(input)
  if (created) relationModalVisible.value = false
  return created
}

onMounted(async () => {
  await loadApplications(typeof route.query.application_id === 'string' ? route.query.application_id : '')
})
</script>

<style scoped>
.topology-filter-card {
  flex-shrink: 0;
  margin-bottom: 12px;
}

.topology-filter-card :deep(.arco-card-body) {
  padding: 12px 16px;
}

.topology-filters :deep(.arco-form-item) {
  margin-bottom: 8px;
}

.topology-alert {
  flex-shrink: 0;
  margin-bottom: 12px;
}

.topology-workbench {
  flex: 1;
  min-height: 0;
}

.topology-workbench :deep(.arco-card-body) {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.topology-toolbar {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.topology-summary,
.topology-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.topology-summary strong {
  font-size: 14px;
}

.topology-summary span {
  color: var(--aiops-text-3);
  font-size: 11px;
}

.topology-spin {
  flex: 1;
  min-height: 0;
}

.topology-spin :deep(.arco-spin-children) {
  height: 100%;
}

@media (max-width: 1100px) {
  .topology-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (max-width: 900px) {
  .topology-page {
    overflow: auto;
  }

  .topology-workbench {
    flex: none;
    min-height: 620px;
  }
}
</style>
