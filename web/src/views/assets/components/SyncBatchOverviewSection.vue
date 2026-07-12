<template>
  <a-descriptions
    :column="1"
    bordered
    size="small"
  >
    <a-descriptions-item
      v-for="item in items"
      :key="item.label"
      :label="item.label"
    >
      <span>{{ item.render }}</span>
    </a-descriptions-item>
  </a-descriptions>
</template>

<script setup lang="ts">
export interface OverviewItem {
  label: string
  render: string | number
}

defineProps<{
  items: OverviewItem[]
}>()
</script>
