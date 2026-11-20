<template>
  <a-form
    :model="formModel"
    layout="inline"
    class="filter-form"
  >
    <a-form-item label="关键词">
      <a-input
        v-model="keyword"
        allow-clear
        placeholder="名称 / 摘要 / 资源"
        style="width: 200px"
        @press-enter="emit('search')"
      />
    </a-form-item>
    <a-form-item label="状态">
      <a-select
        v-model="status"
        allow-clear
        placeholder="全部"
        style="width: 140px"
        :options="statusOptions"
      />
    </a-form-item>
    <a-form-item label="级别">
      <a-select
        v-model="severity"
        allow-clear
        placeholder="全部"
        style="width: 140px"
        :options="severityOptions"
      />
    </a-form-item>
    <a-form-item label="接入源">
      <a-select
        v-model="sourceId"
        allow-clear
        placeholder="全部"
        style="width: 180px"
        :options="sourceOptions"
      />
    </a-form-item>
    <a-form-item>
      <a-checkbox v-model="activeOnly">仅活跃告警</a-checkbox>
    </a-form-item>
    <a-form-item>
      <a-space>
        <a-button
          type="primary"
          @click="emit('search')"
        >
          查询
        </a-button>
        <a-button @click="emit('reset')">重置</a-button>
      </a-space>
    </a-form-item>
  </a-form>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { statusOptions, severityOptions } from '../composables/alertPresentation'

defineProps<{
  sourceOptions: Array<{ label: string; value: string }>
}>()

const keyword = defineModel<string>('keyword', { required: true })
const status = defineModel<string>('status', { required: true })
const severity = defineModel<string>('severity', { required: true })
const sourceId = defineModel<string>('sourceId', { required: true })
const activeOnly = defineModel<boolean>('activeOnly', { required: true })

const formModel = computed(() => ({
  keyword: keyword.value,
  status: status.value,
  severity: severity.value,
  source_id: sourceId.value,
  active_only: activeOnly.value
}))

const emit = defineEmits<{
  search: []
  reset: []
}>()
</script>
