<template>
  <div class="page-shell">
    <div class="page-empty">
      <a-empty :description="`「${title}」模块尚未实现，预计在后续迭代中接入。`" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const title = computed(() => (route.meta.title as string) || '该模块')
</script>
