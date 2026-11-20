<template>
  <div class="message-images">
    <a-image
      v-for="(img, index) in images"
      :key="index"
      :src="img"
      :alt="`附图 ${index + 1}`"
      width="120"
      height="90"
      fit="cover"
      class="message-image"
    />
  </div>
</template>

<script setup lang="ts">
defineProps<{ images: string[] }>()
</script>

<style scoped>
.message-images {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 6px;
}

.message-image {
  overflow: hidden;
  border-radius: 6px;
}
</style>
