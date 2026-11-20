<template>
  <div class="page-header">
    <div class="page-header-copy">
      <span
        v-if="eyebrow"
        class="page-header-eyebrow"
      >{{ eyebrow }}</span>
      <h2>{{ title }}</h2>
      <p v-if="description">
        {{ description }}
      </p>
    </div>
    <div
      v-if="$slots.actions"
      class="page-header-actions"
    >
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  description?: string
  eyebrow?: string
}>()
</script>

<style scoped>
.page-header {
  display: flex;
  flex-shrink: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.page-header-copy {
  min-width: 0;
}

.page-header-eyebrow {
  display: block;
  margin-bottom: 4px;
  color: var(--aiops-accent);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

h2 {
  margin: 0;
  color: var(--aiops-text-1);
  font-size: 20px;
  font-weight: 650;
  line-height: 1.4;
}

p {
  max-width: 760px;
  margin: 5px 0 0;
  color: var(--aiops-text-3);
  font-size: 12px;
  line-height: 1.6;
}

.page-header-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--space-2);
}

@media (max-width: 720px) {
  .page-header {
    flex-direction: column;
  }

  .page-header-actions {
    width: 100%;
    flex-wrap: wrap;
  }
}
</style>
