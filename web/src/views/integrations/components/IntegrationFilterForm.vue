<template>
  <a-form
    :model="formModel"
    layout="inline"
    class="filter-form"
  >
    <a-form-item label="Provider">
      <a-select
        v-model="provider"
        allow-clear
        placeholder="全部"
        style="width: 140px"
        :options="providerOptions"
      />
    </a-form-item>
    <a-form-item label="状态">
      <a-select
        v-model="enabled"
        allow-clear
        placeholder="全部"
        style="width: 140px"
        :options="enabledOptions"
      />
    </a-form-item>
    <a-form-item>
      <a-space>
        <a-button
          type="primary"
          @click="emit('search')"
        >查询</a-button>
        <a-button @click="emit('reset')">重置</a-button>
      </a-space>
    </a-form-item>
  </a-form>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const provider = defineModel<string | undefined>('provider')
const enabled = defineModel<boolean | undefined>('enabled')
const emit = defineEmits<{
  search: []
  reset: []
}>()

const providerOptions = [
  { label: '华为云', value: 'huawei_cloud' },
  { label: 'SigNoz', value: 'signoz' },
  { label: 'Prometheus', value: 'prometheus' }
]
const enabledOptions = [
  { label: '启用', value: true },
  { label: '禁用', value: false }
]

const formModel = computed(() => ({ provider: provider.value, enabled: enabled.value }))
</script>
