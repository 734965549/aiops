<template>
  <a-modal
    :visible="visible"
    title="分享会话（只读）"
    :footer="false"
    width="560px"
    @cancel="close"
  >
    <a-alert
      type="info"
      :show-icon="true"
      class="share-tip"
    >
      链接仅平台内已登录且具备 AI 对话权限的成员可打开；对方只能查看历史，不能继续提问、反馈或执行任何动作。
    </a-alert>

    <div class="share-create">
      <span class="share-label">有效期</span>
      <a-select
        v-model="expiresInHours"
        :options="ttlOptions"
        style="width: 140px"
      />
      <a-button
        type="primary"
        :loading="creating"
        @click="create"
      >
        生成链接
      </a-button>
    </div>

    <div
      v-if="created"
      class="share-created"
    >
      <a-input
        :model-value="shareLink"
        readonly
      />
      <a-button @click="copy">
        复制
      </a-button>
      <div class="share-hint">
        链接只显示一次（服务端只保存哈希）；关闭后无法再次查看，只能重新生成。
      </div>
    </div>

    <a-divider style="margin: 14px 0 10px" />

    <div class="share-list">
      <div
        v-for="share in shares"
        :key="share.share_id"
        class="share-item"
      >
        <a-tag
          :color="isActive(share) ? 'green' : 'gray'"
          size="small"
        >
          {{ statusLabel(share) }}
        </a-tag>
        <span class="share-expire">
          {{ isActive(share) ? `${formatTime(share.expires_at)} 到期` : `创建于 ${formatTime(share.created_at)}` }}
        </span>
        <a-popconfirm
          content="撤销后链接立即失效，确定撤销？"
          @ok="revoke(share.share_id)"
        >
          <a-button
            size="mini"
            type="text"
            status="danger"
            :disabled="!isActive(share)"
          >
            撤销
          </a-button>
        </a-popconfirm>
      </div>
      <a-empty
        v-if="!loading && !shares.length"
        description="暂无分享记录"
      />
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import { createChatShare, listChatShares, revokeChatShare, type ChatShare } from '@/api/ai'

const props = defineProps<{
  visible: boolean
  sessionId: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const ttlOptions = [
  { label: '1 天', value: 24 },
  { label: '7 天（默认）', value: 168 },
  { label: '30 天', value: 720 }
]

const expiresInHours = ref(168)
const shares = ref<ChatShare[]>([])
const loading = ref(false)
const creating = ref(false)
const created = ref<{ token: string; shareUrl: string } | null>(null)

const shareLink = computed(() => {
  if (!created.value) return ''
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  return `${origin}${created.value.shareUrl}`
})

watch(
  () => [props.visible, props.sessionId],
  async ([visible]) => {
    if (!visible || !props.sessionId) return
    created.value = null
    expiresInHours.value = 168
    await load()
  }
)

async function load() {
  if (!props.sessionId) return
  loading.value = true
  try {
    const result = await listChatShares(props.sessionId)
    shares.value = result.shares || []
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    loading.value = false
  }
}

async function create() {
  if (!props.sessionId || creating.value) return
  creating.value = true
  try {
    const result = await createChatShare(props.sessionId, expiresInHours.value)
    created.value = { token: result.token, shareUrl: result.share_url }
    Message.success('分享链接已生成，请立即复制')
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    creating.value = false
  }
}

async function revoke(shareId: string) {
  if (!props.sessionId) return
  try {
    await revokeChatShare(props.sessionId, shareId)
    Message.success('分享链接已撤销')
    await load()
  } catch {
    /* 错误已由拦截器提示。 */
  }
}

async function copy() {
  if (!shareLink.value) return
  try {
    await navigator.clipboard.writeText(shareLink.value)
    Message.success('链接已复制')
  } catch {
    Message.warning('复制失败，请手动选中链接复制')
  }
}

function isActive(share: ChatShare) {
  return share.status === 'active' && new Date(share.expires_at).getTime() > Date.now()
}

function statusLabel(share: ChatShare) {
  if (share.status === 'revoked') return '已撤销'
  return isActive(share) ? '有效' : '已过期'
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function close() {
  emit('update:visible', false)
}
</script>

<style scoped>
.share-tip {
  margin-bottom: 14px;
}

.share-create {
  display: flex;
  align-items: center;
  gap: 8px;
}

.share-label {
  color: var(--color-text-2);
  font-size: 12px;
}

.share-created {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
}

.share-created :deep(.arco-input-wrapper) {
  flex: 1 1 280px;
}

.share-hint {
  width: 100%;
  color: var(--color-text-3);
  font-size: 11px;
}

.share-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px dashed rgba(81, 102, 63, 0.12);
}

.share-expire {
  flex: 1;
  color: var(--color-text-3);
  font-size: 11px;
}
</style>
