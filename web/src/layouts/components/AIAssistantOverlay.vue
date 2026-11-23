<template>
  <a-drawer
    v-model:visible="visible"
    placement="right"
    :width="drawerWidth"
    :footer="false"
    unmount-on-close
  >
    <template #title>
      <div class="overlay-title">
        <span>AI 助手</span>
        <a-tag
          size="small"
          color="arcoblue"
        >
          全局浮层
        </a-tag>
      </div>
    </template>

    <div class="overlay-body">
      <div class="context-bar">
        <IconLocation
          :size="14"
          class="context-icon"
        />
        <div class="context-copy">
          <span class="context-label">当前上下文</span>
          <span class="context-value">{{ contextSummary }}</span>
        </div>
        <a-link @click="openWorkbench">
          完整工作台
        </a-link>
      </div>

      <div
        v-if="sessionPickerVisible"
        class="session-picker"
      >
        <div class="session-picker-header">
          <span class="session-picker-title">选择会话</span>
          <span class="session-picker-description">继续历史会话，或新建会话分析当前告警。</span>
        </div>
        <SessionList
          compact
          :sessions="sessions"
          :active-id="activeSessionId"
          :loading="loadingSessions || loadingMessages"
          @create="onCreateSession"
          @select="onSelectSession"
          @refresh="loadSessionChoices"
        />
      </div>

      <ChatPanel
        v-else
        compact
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
        placeholder="请输入问题，例如：分析当前页面"
        empty-hint="在任意页面唤起，自动带入当前页面上下文；只分析与建议，不改动生产。"
        @select-model="selectModel"
        @send="onSend"
        @retry="retry"
        @stop="stop"
        @feedback="submitFeedback"
        @propose="proposeExecution"
        @navigate="onNavigate"
      />
    </div>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { IconLocation } from '@arco-design/web-vue/es/icon'
import ChatPanel from '@/views/ai-assistant/components/ChatPanel.vue'
import SessionList from '@/views/ai-assistant/components/SessionList.vue'
import { useSharedChat } from '@/views/ai-assistant/composables/useChat'
import { useAIContextStore } from '@/stores/aiContext'

const router = useRouter()
const contextStore = useAIContextStore()
const {
  sessions,
  messages,
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
  activeSessionId,
  loadingSessions,
  loadingMessages,
  loadSessions,
  loadSessionChoices,
  selectSession,
  startNewSession,
  send,
  retry,
  stop,
  submitFeedback,
  proposeExecution
} = useSharedChat()

const quickQuestions = ['分析当前页面', '生成排查步骤', 'SYS.RDS 规则体检', '有什么风险']
const sessionPickerVisible = ref(false)

// 窄屏（手机 / 小窗）使用全宽，避免固定宽度抽屉超出视口。
const drawerWidth = typeof window !== 'undefined' && window.innerWidth < 560 ? '100%' : 480

const visible = computed({
  get: () => contextStore.overlayVisible,
  set: (value: boolean) => {
    if (value) contextStore.openOverlay()
    else contextStore.closeOverlay()
  }
})

watch([visible, () => contextStore.overlaySessionPicker], async ([open, picker]) => {
  if (!open) return
  sessionPickerVisible.value = picker
  if (picker) {
    await loadSessionChoices()
  } else if (!activeSessionId.value) {
    // 普通入口保持原行为：首次打开时加载并选中最近会话。
    await loadSessions()
  }
})

async function onSelectSession(sessionId: string) {
  try {
    await selectSession(sessionId)
    sessionPickerVisible.value = false
  } catch {
    /* 加载失败时保留会话选择页。 */
  }
}

async function onCreateSession() {
  const session = await startNewSession()
  if (session) sessionPickerVisible.value = false
}

async function onSend(content: string, images?: string[]) {
  try {
    await send(content, undefined, images ?? [])
  } catch {
    /* 错误已在面板内展示。 */
  }
}

function onNavigate(url: string) {
  if (!url) return
  contextStore.closeOverlay()
  router.push(url)
}

function openWorkbench() {
  contextStore.closeOverlay()
  router.push('/ai-assistant')
}
</script>

<style scoped>
.overlay-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.overlay-body {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.session-picker {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.session-picker-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.session-picker-title {
  color: var(--color-text-1);
  font-size: 14px;
  font-weight: 600;
}

.session-picker-description {
  color: var(--color-text-3);
  font-size: 12px;
}

.context-bar {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid rgba(81, 102, 63, 0.16);
  border-radius: var(--radius-sm);
  background: rgba(81, 102, 63, 0.06);
}

.context-icon {
  color: #51663f;
}

.context-copy {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.context-label {
  color: var(--color-text-3);
  font-size: 10px;
}

.context-value {
  overflow: hidden;
  color: var(--color-text-1);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.arco-drawer-body) {
  height: calc(100% - 60px);
  padding: 12px 14px 14px;
  overflow: hidden;
}
</style>