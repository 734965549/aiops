<template>
  <span
    class="alert-copy-wrap"
    :class="{ 'is-fallback': fallback }"
  >
    <button
      type="button"
      class="alert-copy-btn"
      :aria-label="`复制${label}`"
      @click="onCopy"
    >
      <svg
        class="alert-copy-icon"
        viewBox="0 0 16 16"
        width="14"
        height="14"
        aria-hidden="true"
        focusable="false"
      >
        <rect
          x="5.5"
          y="5.5"
          width="8.5"
          height="8.5"
          rx="1.5"
          fill="none"
          stroke="currentColor"
          stroke-width="1.4"
        />
        <path
          d="M11 3.2H3.7A1.7 1.7 0 0 0 2 4.9V11"
          fill="none"
          stroke="currentColor"
          stroke-width="1.4"
          stroke-linecap="round"
        />
      </svg>
    </button>
    <!-- 非安全上下文 / 无剪贴板 API 时降级：展示可整段选中的完整原始值。 -->
    <code
      v-if="fallback"
      class="alert-copy-fallback"
    >{{ value }}</code>
  </span>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'

const props = defineProps<{ value: string; label: string }>()
const fallback = ref(false)

async function onCopy() {
  fallback.value = false
  try {
    if (typeof navigator !== 'undefined' && typeof navigator.clipboard?.writeText === 'function') {
      // 复制内容始终是原始完整值，不受中文映射 / 换行 / 掩码影响。
      await navigator.clipboard.writeText(props.value)
      Message.success('已复制')
      return
    }
  } catch {
    // 剪贴板被拒绝时同样走完整值展示，不弹错误提示。
  }
  fallback.value = true
}
</script>

<style scoped lang="scss">
.alert-copy-wrap {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  vertical-align: middle;
  max-width: 100%;
}

.alert-copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  min-height: 24px;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
  color: var(--color-text-2);
  opacity: 0;
  transition: opacity 0.15s;

  &:hover,
  &:focus-visible {
    opacity: 1;
  }

  &:focus-visible {
    outline: 2px solid rgb(var(--primary-6));
    outline-offset: 2px;
  }
}

.alert-copy-wrap:hover .alert-copy-btn,
.alert-copy-wrap:focus-within .alert-copy-btn {
  opacity: 1;
}

.alert-copy-fallback {
  display: block;
  flex: 1 1 100%;
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 20px;
  color: var(--color-text-2);
  background: var(--color-fill-2);
  border: 1px solid var(--color-border-2);
  border-radius: var(--radius-sm);
  padding: 4px 8px;
  overflow-wrap: anywhere;
  word-break: break-word;
  user-select: all;
}

@media (hover: none) {
  .alert-copy-btn {
    opacity: 1;
    min-width: 40px;
    min-height: 40px;
  }
}
</style>
