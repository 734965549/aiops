<template>
  <div class="page-shell ai-workbench">
    <PageHeader
      title="AI 运维助手"
      description="只分析与建议，不直接执行；执行类动作只生成待确认任务，全程写入审计。"
    >
      <template #actions>
        <a-button
          :loading="loadingSessions"
          @click="loadSessions"
        >刷新会话</a-button>
        <a-button
          type="primary"
          @click="startNewSession"
        >新建会话</a-button>
      </template>
    </PageHeader>

    <a-row
      :gutter="12"
      class="workbench-body"
    >
      <a-col
        :xs="24"
        :md="6"
        :lg="5"
        class="workbench-col"
      >
        <a-card
          title="会话"
          :bordered="false"
          class="workbench-card"
        >
          <SessionList
            :sessions="sessions"
            :active-id="activeSessionId"
            :loading="loadingSessions"
            @create="startNewSession"
            @select="onSelectSession"
            @refresh="loadSessions"
            @archive="archiveSession"
            @unarchive="unarchiveSession"
            @delete="deleteSession"
            @share="openShare"
          />
        </a-card>
      </a-col>

      <a-col
        :xs="24"
        :md="12"
        :lg="14"
        class="workbench-col"
      >
        <a-card
          :title="activeSession?.title || '当前会话'"
          :bordered="false"
          class="workbench-card"
        >
          <template #extra>
            <span class="session-context">{{ contextSummary }}</span>
          </template>
          <ChatPanel
            :messages="messages"
            :sending="sending"
            :stage="stage"
            :stage-delta-chars="deltaChars"
            :live-tools="liveTools"
            :live-thinking="liveThinking"
            :status="uiStatus"
            :error="error"
            :proposing="proposing"
            :last-user-content="lastUserContent"
            :quick-questions="quickQuestions"
            :models="modelOptions"
            :model-value="selectedModel?.provider_id || ''"
            :vision-enabled="selectedModel?.vision_enabled ?? false"
            @select-model="selectModel"
            @send="onSend"
            @retry="retry"
            @stop="stop"
            @feedback="submitFeedback"
            @propose="proposeExecution"
            @navigate="onNavigate"
          />
        </a-card>
      </a-col>

      <a-col
        :xs="24"
        :md="6"
        :lg="5"
        class="workbench-col"
      >
        <a-card
          title="上下文与工具"
          :bordered="false"
          class="workbench-card"
        >
          <ContextPanel
            :context="contextStore.current"
            :model-label="modelLabel"
            :tool-calls="latestToolCalls"
            :references="latestReferences"
            @navigate="onNavigate"
          />
        </a-card>
      </a-col>
    </a-row>

    <ShareDialog
      v-model:visible="shareVisible"
      :session-id="shareSessionId"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import SessionList from './components/SessionList.vue'
import ChatPanel from './components/ChatPanel.vue'
import ContextPanel from './components/ContextPanel.vue'
import ShareDialog from './components/ShareDialog.vue'
import { useChat, formatModelLabel } from './composables/useChat'
import { useAIContextStore } from '@/stores/aiContext'
import PageHeader from '@/components/PageHeader.vue'

const router = useRouter()
const contextStore = useAIContextStore()
const quickQuestions = [
  '分析当前告警并给出排查步骤',
  '分析 SYS.RDS 命名空间的告警规则合理性',
  '最近 7 天的指标趋势有什么异常',
  '生成一份处置计划'
]
const {
  sessions,
  activeSession,
  activeSessionId,
  messages,
  loadingSessions,
  sending,
  proposing,
  error,
  stage,
  deltaChars,
  liveTools,
  liveThinking,
  uiStatus,
  lastUserContent,
  contextSummary,
  modelOptions,
  selectedModel,
  selectModel,
  loadSessions,
  selectSession,
  startNewSession,
  send,
  retry,
  stop,
  submitFeedback,
  archiveSession,
  unarchiveSession,
  deleteSession,
  proposeExecution
} = useChat()

const modelLabel = computed(() => formatModelLabel(selectedModel.value))

const latestToolCalls = computed(() => {
  const lastAssistant = [...messages.value].reverse().find((message) => message.role === 'assistant')
  return lastAssistant?.tool_calls ?? []
})

const latestReferences = computed(() => {
  const lastAssistant = [...messages.value].reverse().find((message) => message.role === 'assistant')
  return lastAssistant?.references ?? []
})

// 分享弹窗：token 只在创建响应返回一次，关闭后只能重新生成。
const shareVisible = ref(false)
const shareSessionId = ref('')

function openShare(sessionId: string) {
  shareSessionId.value = sessionId
  shareVisible.value = true
}

async function onSend(content: string, images?: string[]) {
  await send(content, undefined, images ?? [])
}

async function onSelectSession(sessionId: string) {
  await selectSession(sessionId)
}

function onNavigate(url: string) {
  if (url) router.push(url)
}

onMounted(async () => {
  // 页面基础上下文由布局按路由同步；这里只加载会话列表。
  await loadSessions()
})
</script>

<style scoped>
.ai-workbench {
  gap: 0;
}

.workbench-body {
  flex: 1;
  min-height: 0;
  display: flex;
}

.workbench-col {
  height: 100%;
  min-height: 0;
  display: flex;
}

.workbench-card {
  flex: 1;
  /* 会话标题改为提问全文（nowrap）后 min-content 变宽，没有 min-width:0 时卡片会被撑破、横穿中间列 */
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.workbench-card :deep(.arco-card-header) {
  flex: 0 0 auto;
}

.workbench-card :deep(.arco-card-body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 12px 14px;
}

.session-context {
  max-width: 320px;
  overflow: hidden;
  color: var(--color-text-3);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .workbench-col {
    height: auto;
  }
}
</style>
