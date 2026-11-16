<template>
  <div class="page-shell ai-config">
    <a-tabs
      default-active-key="providers"
      class="page-tabs"
    >
      <a-tab-pane
        key="providers"
        title="Provider 管理"
      >
        <ProviderManager />
      </a-tab-pane>
      <a-tab-pane
        key="console"
        title="工具调用调试"
      >
        <ToolConsole />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<script setup lang="ts">
import ProviderManager from './components/ProviderManager.vue'
import ToolConsole from './components/ToolConsole.vue'
</script>

<style scoped>
.ai-config :deep(.arco-tabs-content) {
  padding-top: 10px;
}

/* 页签头吸顶兜底：与其它配置页保持一致的胶囊化页签。 */
.ai-config :deep(.arco-tabs-nav) {
  position: sticky;
  top: 0;
  z-index: 10;
  margin: 0 0 10px;
  padding: 6px 0 10px;
  background: transparent;
}

.ai-config :deep(.arco-tabs-nav-type-line .arco-tabs-tab) {
  margin: 0 10px 0 0;
  padding: 0 18px;
  height: 34px;
  border: 1px solid rgba(81, 102, 63, 0.16);
  border-radius: var(--radius-md);
  color: #4a544c;
  font-weight: 500;
  background: rgba(81, 102, 63, 0.06);
  transition: background-color 160ms ease, color 160ms ease, border-color 160ms ease,
    box-shadow 160ms ease;
}

.ai-config :deep(.arco-tabs-tab-title:before) {
  background-color: transparent !important;
}

.ai-config :deep(.arco-tabs-nav-type-line .arco-tabs-tab:hover) {
  color: #263328;
  background: rgba(81, 102, 63, 0.14);
  border-color: rgba(81, 102, 63, 0.28);
}

.ai-config :deep(.arco-tabs-nav-type-line .arco-tabs-tab-active),
.ai-config :deep(.arco-tabs-nav-type-line .arco-tabs-tab-active:hover) {
  color: #ffffff;
  border-color: #51663f;
  background: #51663f;
  box-shadow: 0 2px 10px rgba(81, 102, 63, 0.35);
}

.ai-config :deep(.arco-tabs-tab-active .arco-tabs-tab-title) {
  color: #ffffff !important;
}

.ai-config :deep(.arco-tabs-nav-ink) {
  display: none !important;
}
</style>
