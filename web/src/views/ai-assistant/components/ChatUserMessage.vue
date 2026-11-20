<template>
  <div class="message-row message-row--user">
    <div class="message-bubble message-bubble--user">
      <ChatMessageImages
        v-if="message.images?.length"
        :images="message.images"
      />
      <div v-if="message.content">{{ message.content }}</div>
    </div>
    <div class="message-meta">
      <span class="message-time">{{ time }}</span>
      <button
        type="button"
        class="message-copy"
        title="复制消息"
        aria-label="复制消息"
        @click="copy"
      >
        <IconCopy :size="12" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { IconCopy } from '@arco-design/web-vue/es/icon'
import type { ChatMessage } from '@/api/ai'
import ChatMessageImages from './ChatMessageImages.vue'
import { copyText } from '../composables/useCopy'
import { formatMessageTime } from '../composables/format'

const props = defineProps<{ message: ChatMessage }>()

const time = computed(() => formatMessageTime(props.message.created_at))

async function copy() {
  await copyText(props.message.content, '已复制消息')
}
</script>

<style scoped>
.message-row {
  display: flex;
  flex-direction: column;
  max-width: 100%;
}

.message-row--user {
  align-items: flex-end;
  gap: 2px;
}

.message-bubble {
  max-width: 86%;
  padding: 9px 12px;
  border-radius: var(--radius-md);
  font-size: 13px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.message-bubble--user {
  color: #ffffff;
  background: #51663f;
  border-bottom-right-radius: 4px;
}

.message-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 2px;
}

.message-copy {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  border-radius: 4px;
  color: var(--color-text-3);
  background: transparent;
  opacity: 0.75;
  cursor: pointer;
  transition: opacity 0.15s ease;
}

.message-copy:hover {
  color: var(--color-text-1);
  background: var(--color-fill-2);
  opacity: 1;
}

.message-time {
  margin-top: 2px;
  color: var(--color-text-3);
  font-size: 10px;
}

.message-meta .message-time {
  margin-top: 0;
}
</style>
