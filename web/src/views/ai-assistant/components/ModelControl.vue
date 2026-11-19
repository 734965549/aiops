<template>
  <a-select
    v-if="pickerVisible"
    :model-value="modelValue"
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
  <a-tag
    v-else-if="options.length"
    class="model-tag"
    :title="tagHint"
  >
    模型 {{ tagLabel }}
  </a-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChatModelInfo } from '@/api/ai'
import { formatModelLabel } from '../composables/useChat'

const props = defineProps<{
  models?: ChatModelInfo[]
  modelValue?: string
  modelLocked?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-model', providerID: string): void
}>()

const options = computed(() => props.models ?? [])
// 仅锁定态用只读 Tag；未锁定时哪怕只有 1 个模型也渲染下拉，保证入口可见。
const pickerVisible = computed(() => !props.modelLocked && options.value.length > 0)
const selected = computed(
  () => options.value.find((model) => model.provider_id === props.modelValue) ?? options.value[0] ?? null
)
const tagLabel = computed(() => formatModelLabel(selected.value))
const tagHint = computed(() =>
  options.value.length < 2 ? '可在 AI 配置中启用更多平台直连模型' : '切换模型需新建会话'
)

function onSelect(value: unknown) {
  emit('select-model', String(value ?? ''))
}
</script>

<style scoped>
.model-select {
  flex: none;
  width: 200px;
}

.model-tag {
  flex: none;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>