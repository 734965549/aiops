<template>
  <a-alert
    type="success"
    class="ai-result"
  >
    <div><strong>风险等级：</strong>{{ result.risk_level }}</div>
    <div><strong>摘要：</strong>{{ result.summary }}</div>
    <div v-if="result.recommendations?.length">
      <strong>建议：</strong>
      <ul>
        <li
          v-for="(r, i) in result.recommendations"
          :key="i"
        >
          {{ r }}
        </li>
      </ul>
    </div>
    <div v-if="result.references?.length">
      <strong>证据来源：</strong>
      <ul>
        <li
          v-for="(r, i) in result.references"
          :key="i"
        >
          {{ r }}
        </li>
      </ul>
    </div>
  </a-alert>
</template>

<script setup lang="ts">
import type { AnalyzeAlertResult } from '@/api/ai'

defineProps<{ result: AnalyzeAlertResult }>()
</script>

<style scoped lang="scss">
.ai-result {
  margin-top: 12px;
}
</style>
