<template>
  <a-row
    :gutter="16"
    class="stats-row"
  >
    <a-col
      v-for="card in cards"
      :key="card.title"
      :xs="12"
      :lg="6"
    >
      <a-card
        class="stat-card"
        :class="{ 'stat-clickable': card.resourceType }"
        :bordered="false"
        @click="card.resourceType && emit('select', card.resourceType)"
      >
        <div class="stat-header">
          <span class="stat-label">{{ card.title }}</span>
          <component
            :is="card.icon"
            class="stat-icon"
            :style="{ color: card.color }"
          />
        </div>
        <div class="stat-value numeric">{{ card.value }}</div>
        <div class="stat-footer">{{ card.description }}</div>
      </a-card>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IconCheckCircle, IconFile, IconNotification, IconRobot } from '@arco-design/web-vue/es/icon'

const props = defineProps<{
  total: number
  alertCount: number
  executionCount: number
  aiCount: number
}>()

const emit = defineEmits<{
  select: [resourceType: string]
}>()

const cards = computed(() => [
  { title: '总操作审计', value: props.total, description: '当前系统记录的总操作数', icon: IconFile, color: 'rgb(var(--primary-6))', resourceType: '' },
  { title: '告警动作审计', value: props.alertCount, description: '认领、静音、关闭等动作', icon: IconNotification, color: 'rgb(var(--warning-6))', resourceType: 'alert' },
  { title: '执行确认审计', value: props.executionCount, description: '任务审批与操作确认', icon: IconCheckCircle, color: 'rgb(var(--success-6))', resourceType: 'execution' },
  { title: 'AI 工具与分析', value: props.aiCount, description: '模型推理与工具调用', icon: IconRobot, color: '#722ed1', resourceType: 'ai' }
])
</script>

<style scoped>
.stats-row {
  flex-shrink: 0;
}

.stat-card {
  height: 100%;
  border: 1px solid var(--aiops-border);
}

.stat-clickable {
  cursor: pointer;
}

.stat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stat-label {
  color: var(--aiops-text-3);
  font-size: 12px;
}

.stat-icon {
  font-size: 19px;
}

.stat-value {
  margin: 8px 0 4px;
  color: var(--aiops-text-1);
  font-size: 24px;
  font-weight: 650;
}

.stat-footer {
  color: var(--aiops-text-3);
  font-size: 11px;
}

@media (max-width: 991px) {
  .stats-row > .arco-col {
    margin-bottom: 12px;
  }
}
</style>
