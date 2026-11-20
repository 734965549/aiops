<template>
  <a-modal
    v-model:visible="visible"
    title="新建资源关系"
    :ok-loading="loading"
    :ok-disabled="!canSubmit"
    :before-ok="beforeOk"
    @cancel="reset"
  >
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
          :options="resourceOptions"
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
      <a-form-item
        label="目标资源"
        required
      >
        <a-select
          v-model="form.target_resource_id"
          allow-search
          placeholder="选择关系终点"
          :options="resourceOptions"
        />
      </a-form-item>
      <a-alert type="info">
        一期仅支持同一应用内的手工关系，不允许创建自环。
      </a-alert>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { AssetTopologyNode, CreateAssetResourceRelationInput, ResourceRelationType } from '@/api/asset-topology'
import { relationTypeOptions } from '../composables/topologyPresentation'

const props = defineProps<{
  resources: AssetTopologyNode[]
  loading: boolean
  submit: (input: CreateAssetResourceRelationInput) => Promise<boolean>
}>()

const visible = defineModel<boolean>('visible', { default: false })

const form = reactive<{ source_resource_id: string; target_resource_id: string; relation_type: '' | ResourceRelationType }>({
  source_resource_id: '',
  target_resource_id: '',
  relation_type: 'depends_on'
})

const resourceOptions = computed(() => props.resources.map((node) => ({
  label: `${node.name} · ${node.cloud_resource_type || node.resource_type || node.entity_id}`,
  value: node.entity_id
})))

const canSubmit = computed(() => Boolean(
  form.source_resource_id && form.target_resource_id && form.relation_type && form.source_resource_id !== form.target_resource_id
))

watch(visible, (open) => {
  if (open) reset()
})

function reset() {
  form.source_resource_id = ''
  form.target_resource_id = ''
  form.relation_type = 'depends_on'
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
