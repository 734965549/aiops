<template>
  <div class="model-control">
    <a-select
      :model-value="modelValue"
      :disabled="disabled"
      :bordered="false"
      size="mini"
      class="model-select"
      @change="onSelect"
    >
      <a-option
        v-for="model in options"
        :key="model.provider_id"
        :value="model.provider_id"
      >
        {{ formatModelLabel(model) }}
      </a-option>
    </a-select>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChatModelInfo } from '@/api/ai'
import { formatModelLabel } from '../composables/useChat'

const props = defineProps<{
  models?: ChatModelInfo[]
  modelValue?: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-model', providerID: string): void
}>()

const options = computed(() => props.models ?? [])

function onSelect(value: unknown) {
  emit('select-model', String(value ?? ''))
}
</script>

<style scoped>
.model-control {
  flex: 0 0 auto;
  width: 200px;
  max-width: 45vw;
}

.model-control :deep(.model-select) {
  width: 100%;
}
</style>
