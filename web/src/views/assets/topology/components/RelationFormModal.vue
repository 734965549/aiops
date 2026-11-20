<template>
  <a-modal
    v-model:visible="visible"
    title="新建资源关系"
    :ok-loading="loading || resourcesLoading"
    :ok-disabled="!canSubmit"
    :before-ok="beforeOk"
    @cancel="reset"
  >
    <a-spin :loading="resourcesLoading">
      <a-alert
        v-if="loadError"
        type="error"
        class="relation-load-error"
      >
        {{ loadError }}
      </a-alert>
      <a-form
        :model="form"
        layout="vertical"
      >
        <a-form-item
          label="源资源"
          required
        >
          <a-select
            v-model="form.source_resource_id"
            allow-search
            placeholder="选择关系起点"
            :options="sourceOptions"
          />
        </a-form-item>
        <a-form-item
          label="关系类型"
          required
        >
          <a-select
            v-model="form.relation_type"
            placeholder="选择关系类型"
            :options="relationTypeOptions"
          />
        </a-form-item>
        <a-form-item label="目标应用">
          <a-select
            v-model="targetApplicationId"
            :options="applicationOptions"
            placeholder="选择目标应用"
          />
        </a-form-item>
        <a-form-item
          label="目标资源"
          required
        >
          <a-select
            v-model="form.target_resource_id"
            allow-search
            placeholder="可跨应用选择关系终点"
            :options="targetOptions"
          />
        </a-form-item>
        <a-alert type="info">
          支持跨应用手工关系，禁止自环；云端派生关系只由已验证同步字段生成。
        </a-alert>
      </a-form>
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { Application } from '@/api/asset'
import {
  getAssetTopology,
  type AssetTopologyNode,
  type CreateAssetResourceRelationInput,
  type ResourceRelationType
} from '@/api/asset-topology'
import { relationTypeOptions, topologyResourceNodes } from '../composables/topologyPresentation'

const props = defineProps<{
  applications: Application[]
  currentApplicationId: string
  loading: boolean
  submit: (input: CreateAssetResourceRelationInput) => Promise<boolean>
}>()

const visible = defineModel<boolean>('visible', { default: false })
const targetApplicationId = ref('')
const sourceResources = ref<AssetTopologyNode[]>([])
const targetResources = ref<AssetTopologyNode[]>([])
const resourcesLoading = ref(false)
const loadError = ref('')

const form = reactive<{ source_resource_id: string; target_resource_id: string; relation_type: '' | ResourceRelationType }>({
  source_resource_id: '',
  target_resource_id: '',
  relation_type: 'depends_on'
})

const applicationOptions = computed(() => props.applications.map((app) => ({
  label: `${app.name}${app.environment ? ` · ${app.environment}` : ''}`,
  value: app.id
})))
const sourceOptions = computed(() => resourceOptions(sourceResources.value))
const targetOptions = computed(() => resourceOptions(targetResources.value))
const canSubmit = computed(() => Boolean(
  form.source_resource_id && form.target_resource_id && form.relation_type &&
  form.source_resource_id !== form.target_resource_id && !resourcesLoading.value && !loadError.value
))

watch(visible, async (open) => {
  if (!open) return
  reset()
  await loadResources()
})

watch(targetApplicationId, async () => {
  if (!visible.value) return
  form.target_resource_id = ''
  await loadTargetResources()
})

function resourceOptions(nodes: AssetTopologyNode[]) {
  return nodes.map((node) => ({
    label: `${node.name} · ${node.cloud_resource_type || node.resource_type || node.entity_id}`,
    value: node.entity_id
  }))
}

function reset() {
  form.source_resource_id = ''
  form.target_resource_id = ''
  form.relation_type = 'depends_on'
  targetApplicationId.value = props.currentApplicationId
  sourceResources.value = []
  targetResources.value = []
  loadError.value = ''
}

async function loadResources() {
  resourcesLoading.value = true
  loadError.value = ''
  try {
    const [source, target] = await Promise.all([
      getAssetTopology({ application_id: props.currentApplicationId, include_stale: true }),
      getAssetTopology({ application_id: targetApplicationId.value, include_stale: true })
    ])
    sourceResources.value = topologyResourceNodes(source.nodes)
    targetResources.value = topologyResourceNodes(target.nodes)
  } catch {
    loadError.value = '资源列表加载失败，请检查权限后重试。'
  } finally {
    resourcesLoading.value = false
  }
}

async function loadTargetResources() {
  resourcesLoading.value = true
  loadError.value = ''
  try {
    const result = await getAssetTopology({ application_id: targetApplicationId.value, include_stale: true })
    targetResources.value = topologyResourceNodes(result.nodes)
  } catch {
    loadError.value = '目标应用资源加载失败，请检查权限后重试。'
  } finally {
    resourcesLoading.value = false
  }
}

async function beforeOk() {
  if (!canSubmit.value || form.relation_type === '') return false
  return props.submit({
    source_resource_id: form.source_resource_id,
    target_resource_id: form.target_resource_id,
    relation_type: form.relation_type
  })
}
</script>

<style scoped>
.relation-load-error {
  margin-bottom: 12px;
}
</style>
