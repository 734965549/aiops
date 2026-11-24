<template>
  <div class="queue-panel">
    <div class="queue-header">
      <span>排队中 {{ messages.length }} 条</span>
      <a-button
        size="mini"
        type="text"
        @click="emit('clear')"
      >
        清空队列
      </a-button>
    </div>
    <div class="queue-list">
      <div
        v-for="(message, index) in messages"
        :key="message.queue_id"
        class="queue-item"
      >
        <span class="queue-order">{{ index === 0 ? '下一条' : `第 ${index + 1} 条` }}</span>
        <span
          class="queue-content"
          :title="message.content"
        >{{ message.content }}</span>
        <span class="queue-actions">
          <a-button
            v-if="index > 0"
            size="mini"
            type="text"
            @click="emit('prioritize', message.queue_id)"
          >
            置顶
          </a-button>
          <a-button
            size="mini"
            type="text"
            status="danger"
            @click="emit('remove', message.queue_id)"
          >
            删除
          </a-button>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { QueuedChatMessage } from '../composables/chatSessionState'

defineProps<{
  messages: QueuedChatMessage[]
}>()

const emit = defineEmits<{
  (e: 'remove', queueId: string): void
  (e: 'prioritize', queueId: string): void
  (e: 'clear'): void
}>()
</script>

<style scoped>
.queue-panel {
  padding: 8px 10px;
  border: 1px solid rgba(81, 102, 63, 0.16);
  border-radius: var(--radius-sm);
  background: rgba(81, 102, 63, 0.04);
}

.queue-header,
.queue-item,
.queue-actions {
  display: flex;
  align-items: center;
}

.queue-header {
  justify-content: space-between;
  color: var(--color-text-2);
  font-size: 12px;
  font-weight: 500;
}

.queue-list {
  max-height: 144px;
  margin-top: 4px;
  overflow-y: auto;
}

.queue-item {
  min-height: 28px;
  gap: 8px;
}

.queue-order {
  flex: 0 0 auto;
  color: var(--color-text-3);
  font-size: 10px;
}

.queue-content {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-2);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.queue-actions {
  flex: 0 0 auto;
  gap: 2px;
}

.queue-actions :deep(.arco-btn-size-mini) {
  padding: 0 4px;
}
</style>
