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

      <ChatPanel
        compact
        :messages="messages"
        :sending="sending"
        :stage="stage"
        :stage-delta-chars="deltaChars"
        :status="uiStatus"
        :error="error"
        :proposing="proposing"
        :last-user-content="lastUserContent"
        :quick-questions="quickQuestions"
        placeholder="请输入问题，例如：分析当前页面"
        empty-hint="在任意页面唤起，自动带入当前页面上下文；只分析与建议，不改动生产。"
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
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { IconLocation } from '@arco-design/web-vue/es/icon'
import ChatPanel from '@/views/ai-assistant/components/ChatPanel.vue'
import { useChat } from '@/views/ai-assistant/composables/useChat'
import { useAIContextStore } from '@/stores/aiContext'

const router = useRouter()
const contextStore = useAIContextStore()
const {
  messages,
  sending,
  proposing,
  error,
  stage,
  deltaChars,
  uiStatus,
  lastUserContent,
  contextSummary,
  activeSessionId,
  loadSessions,
  send,
  retry,
  stop,
  submitFeedback,
  proposeExecution
} = useChat()

const quickQuestions = ['分析当前页面', '生成排查步骤', 'SYS.RDS 规则体检', '有什么风险']

// 窄屏（手机 / 小窗）使用全宽，避免固定宽度抽屉超出视口。
const drawerWidth = typeof window !== 'undefined' && window.innerWidth < 560 ? '100%' : 480

const visible = computed({
  get: () => contextStore.overlayVisible,
  set: (value: boolean) => {
    if (value) contextStore.openOverlay()
    else contextStore.closeOverlay()
  }
})

watch(visible, async (open) => {
  if (!open) return
  // 打开时同步一次会话列表；无会话时发送首条消息会自动创建。
  if (!activeSessionId.value) {
    await loadSessions()
  }
})

async function onSend(content: string) {
  try {
    await send(content)
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