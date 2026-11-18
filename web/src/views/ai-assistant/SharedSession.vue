<template>
  <div class="shared-page">
    <a-spin
      :loading="loading"
      class="shared-spin"
    >
      <a-card
        v-if="detail"
        :bordered="false"
        class="shared-card"
      >
        <template #title>
          <div class="shared-title">
            <span>{{ detail.session.title || '共享会话' }}</span>
            <a-tag
              size="small"
              color="gray"
            >
              只读分享
            </a-tag>
          </div>
        </template>
        <template #extra>
          <a-button
            size="small"
            @click="router.push('/ai-assistant')"
          >
            打开我的 AI 助手
          </a-button>
        </template>

        <a-alert
          type="info"
          :show-icon="true"
          class="shared-tip"
        >
          这是他人分享的会话快照式视图：只能查看历史消息与建议，不能继续提问、提交反馈或创建执行任务。
        </a-alert>

        <div class="shared-stream">
          <template
            v-for="message in detail.messages"
            :key="message.message_id"
          >
            <div
              v-if="message.role === 'user'"
              class="message-row message-row--user"
            >
              <div class="message-bubble message-bubble--user">
                {{ message.content }}
                <span class="message-time">{{ formatTime(message.created_at) }}</span>
              </div>
            </div>
            <div
              v-else
              class="message-row message-row--assistant"
            >
              <AnswerCard
                v-if="message.answer"
                :answer="message.answer"
                :tool-calls="message.tool_calls"
                :references="message.references"
                :busy="true"
              />
              <div
                v-else
                class="message-bubble message-bubble--assistant"
              >
                {{ message.content }}
              </div>
              <span class="message-time">{{ formatTime(message.created_at) }}</span>
            </div>
          </template>
          <a-empty
            v-if="!detail.messages.length"
            description="该会话暂无消息"
          />
        </div>
      </a-card>

      <a-result
        v-else-if="error"
        status="warning"
        title="分享链接不可用"
        :subtitle="error"
      >
        <template #extra>
          <a-button
            type="primary"
            @click="router.push('/ai-assistant')"
          >
            返回 AI 助手
          </a-button>
        </template>
      </a-result>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getSharedChatSession, type ChatSessionDetail } from '@/api/ai'
import { isApiHttpError } from '@/api/request'
import AnswerCard from './components/AnswerCard.vue'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const detail = ref<ChatSessionDetail | null>(null)
const error = ref('')

onMounted(load)

async function load() {
  const token = String(route.params.token || '')
  if (!token) {
    error.value = '链接缺少分享令牌'
    loading.value = false
    return
  }
  try {
    detail.value = await getSharedChatSession(token)
  } catch (err) {
    error.value = isApiHttpError(err) && err.status === 404
      ? '链接可能已过期、被撤销，或会话已被删除。'
      : '加载共享会话失败，请稍后重试。'
  } finally {
    loading.value = false
  }
}

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
.shared-page {
  padding: 16px;
}

.shared-spin {
  display: block;
}

.shared-card {
  max-width: 960px;
  margin: 0 auto;
}

.shared-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.shared-tip {
  margin-bottom: 14px;
}

.shared-stream {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.message-row {
  display: flex;
  flex-direction: column;
}

.message-row--user {
  align-items: flex-end;
}

.message-row--assistant {
  align-items: stretch;
  gap: 4px;
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

.message-bubble--assistant {
  max-width: 100%;
  color: var(--color-text-1);
  background: var(--color-fill-1);
  border-bottom-left-radius: 4px;
}

.message-time {
  margin-top: 2px;
  color: var(--color-text-3);
  font-size: 10px;
}
</style>
