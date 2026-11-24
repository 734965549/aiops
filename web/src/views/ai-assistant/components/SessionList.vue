<template>
  <div class="session-list">
    <a-button
      type="primary"
      long
      :disabled="loading"
      @click="emit('create')"
    >
      <template #icon>
        <IconPlus />
      </template>
      新建会话
    </a-button>
    <div class="session-toolbar">
      <span class="session-count">{{ sessions.length }} 个会话</span>
      <a-button
        size="mini"
        type="text"
        :loading="loading"
        @click="emit('refresh')"
      >
        刷新
      </a-button>
    </div>
    <a-spin
      :loading="loading"
      style="flex: 1; min-height: 0; overflow: auto; display: block"
    >
      <a-empty
        v-if="!sessions.length"
        description="暂无会话"
        class="session-empty"
      />
      <div
        v-for="session in sessions"
        :key="session.session_id"
        class="session-item"
        :class="{ 'is-active': session.session_id === activeId }"
        @click="emit('select', session.session_id)"
      >
        <div class="session-title">
          {{ session.title || '新会话' }}
        </div>
        <div class="session-meta">
          <a-tag
            v-if="sendingIds.includes(session.session_id)"
            size="small"
            color="arcoblue"
          >
            生成中
          </a-tag>
          <a-tag
            v-if="session.status === 'archived'"
            size="small"
            color="gray"
          >
            已归档
          </a-tag>
          <span>{{ formatTime(session.updated_at) }}</span>
          <span
            v-if="!compact"
            class="session-actions"
            @click.stop
          >
            <a-button
              size="mini"
              type="text"
              @click="emit('share', session.session_id)"
            >
              分享
            </a-button>
            <a-popconfirm
              v-if="session.status === 'archived'"
              content="恢复后可继续在该会话提问，确定恢复？"
              @ok="emit('unarchive', session.session_id)"
            >
              <a-button
                size="mini"
                type="text"
              >
                恢复
              </a-button>
            </a-popconfirm>
            <a-popconfirm
              v-else
              content="归档后会话只读，历史仍可查看，确定归档？"
              @ok="emit('archive', session.session_id)"
            >
              <a-button
                size="mini"
                type="text"
              >
                归档
              </a-button>
            </a-popconfirm>
            <a-popconfirm
              content="删除后会话与消息不可恢复，确定删除？"
              ok-text="删除"
              @ok="emit('delete', session.session_id)"
            >
              <a-button
                size="mini"
                type="text"
                status="danger"
              >
                删除
              </a-button>
            </a-popconfirm>
          </span>
        </div>
      </div>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { IconPlus } from '@arco-design/web-vue/es/icon'
import type { ChatSession } from '@/api/ai'

withDefaults(defineProps<{
  sessions: ChatSession[]
  activeId: string
  loading: boolean
  compact?: boolean
  sendingIds?: string[]
}>(), {
  compact: false,
  sendingIds: () => []
})

const emit = defineEmits<{
  (e: 'create'): void
  (e: 'select', sessionId: string): void
  (e: 'refresh'): void
  (e: 'archive', sessionId: string): void
  (e: 'unarchive', sessionId: string): void
  (e: 'delete', sessionId: string): void
  (e: 'share', sessionId: string): void
}>()

function formatTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}
</script>

<style scoped>
.session-list {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.session-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--color-text-3);
  font-size: 11px;
}

.session-item {
  padding: 8px 10px;
  margin-bottom: 6px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 150ms ease, border-color 150ms ease;
}

.session-item:hover {
  background: rgba(81, 102, 63, 0.07);
}

.session-item.is-active {
  border-color: rgba(81, 102, 63, 0.32);
  background: rgba(81, 102, 63, 0.12);
}

.session-title {
  overflow: hidden;
  color: var(--color-text-1);
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  color: var(--color-text-3);
  font-size: 10px;
}

.session-actions {
  display: inline-flex;
  gap: 2px;
  margin-left: auto;
  opacity: 0;
  transition: opacity 150ms ease;
}

.session-item:hover .session-actions,
.session-item.is-active .session-actions {
  opacity: 1;
}

.session-actions :deep(.arco-btn-size-mini) {
  height: 20px;
  padding: 0 4px;
  font-size: 11px;
}

.session-empty {
  margin-top: 24px;
}
</style>
