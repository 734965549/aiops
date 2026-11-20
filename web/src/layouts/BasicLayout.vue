<template>
  <a-layout class="layout">
    <a-layout-sider
      :width="208"
      theme="dark"
      breakpoint="lg"
      class="sider"
    >
      <div class="logo">
        <div class="logo-mark">AI</div>
        <div class="logo-copy">
          <span class="logo-text">AI 运维平台</span>
          <span class="logo-sub">OPERATIONS CONSOLE</span>
        </div>
      </div>

      <nav class="navigation" aria-label="主导航">
        <section
          v-for="group in navigationGroups"
          :key="group.title"
          class="nav-group"
        >
          <div class="nav-group-label">{{ group.title }}</div>
          <a-menu
            theme="dark"
            :selected-keys="[activeMenuKey]"
            @menu-item-click="onMenuClick"
          >
            <a-menu-item
              v-for="item in group.items"
              :key="item.path"
            >
              <template #icon>
                <component :is="item.icon" />
              </template>
              {{ item.title }}
            </a-menu-item>
          </a-menu>
        </section>
      </nav>

      <div class="sider-foot">
        <span class="sider-foot-dot" />
        <div>
          <strong>安全执行受控</strong>
          <span>权限 · 确认 · 审计</span>
        </div>
      </div>
    </a-layout-sider>

    <a-layout class="content-shell">
      <a-layout-header class="header">
        <div class="header-left">
          <span class="header-section">{{ currentGroup }}</span>
          <span class="header-separator">/</span>
          <span class="header-title">{{ currentTitle }}</span>
        </div>
        <div class="header-right">
          <a-tooltip content="AI 助手（Alt + A）">
            <a-button
              type="text"
              size="small"
              class="ai-entry"
              @click="aiContext.openOverlay()"
            >
              <template #icon>
                <IconRobot />
              </template>
              AI 助手
            </a-button>
          </a-tooltip>
          <span class="header-divider" />
          <span class="user-avatar">{{ userInitial }}</span>
          <span class="user-name">{{ auth.user?.display_name || auth.user?.username || '未登录' }}</span>
          <a-button
            type="text"
            size="small"
            class="logout-button"
            @click="onLogout"
          >
            退出
          </a-button>
        </div>
      </a-layout-header>

      <a-layout-content class="main">
        <RouterView />
      </a-layout-content>
    </a-layout>

    <AIAssistantOverlay />
  </a-layout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { IconRobot } from '@arco-design/web-vue/es/icon'
import { useAuthStore } from '@/stores/auth'
import { useAIContextStore } from '@/stores/aiContext'
import { fetchCurrentUser } from '@/api/system'
import AIAssistantOverlay from './components/AIAssistantOverlay.vue'
import { navigationGroups, navigationItems } from './navigation'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const aiContext = useAIContextStore()

const activeMenuKey = computed(() => {
  const metaMenu = route.meta.activeMenu as string | undefined
  if (metaMenu) return metaMenu
  const hit = [...navigationItems]
    .filter((item) => route.path === item.path || route.path.startsWith(`${item.path}/`))
    .sort((a, b) => b.path.length - a.path.length)[0]
  return hit?.path ?? route.path
})

const currentTitle = computed(() => {
  const metaTitle = typeof route.meta.title === 'string' ? route.meta.title : ''
  if (metaTitle) return metaTitle
  return navigationItems.find((item) => item.path === activeMenuKey.value)?.title ?? ''
})

const currentGroup = computed(() => {
  return navigationGroups.find((group) => group.items.some((item) => item.path === activeMenuKey.value))?.title ?? '控制台'
})

const userInitial = computed(() => {
  const name = auth.user?.display_name || auth.user?.username || 'U'
  return name.trim().slice(0, 1).toUpperCase()
})

function onMenuClick(key: string) {
  if (key && key !== route.path) router.push(key)
}

watch(
  () => route.path,
  (path) => {
    const title = typeof route.meta.title === 'string' ? route.meta.title : ''
    if (path === '/ai-assistant') {
      if (!aiContext.hasResourceContext) aiContext.setPageBase('ai-assistant', title)
      return
    }
    aiContext.setPageBase(path.replace(/^\//, ''), title)
  },
  { immediate: true }
)

function onKeydown(event: KeyboardEvent) {
  if (event.altKey && !event.ctrlKey && !event.metaKey && event.key.toLowerCase() === 'a') {
    event.preventDefault()
    aiContext.toggleOverlay()
  }
}

async function onLogout() {
  await auth.logout()
  router.replace('/login')
}

onMounted(async () => {
  window.addEventListener('keydown', onKeydown)
  if (!auth.token) return
  try {
    const user = await fetchCurrentUser()
    auth.setUser(user)
  } catch {
    // 请求层已提示，用户信息刷新失败不阻断布局。
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<style scoped>
.layout {
  height: 100vh;
  overflow: hidden;
  background: var(--aiops-paper);
}

.sider {
  z-index: 3;
  background: var(--aiops-nav) !important;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
}

.sider :deep(.arco-layout-sider-children) {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.logo {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 10px;
  height: 64px;
  padding: 0 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.logo-mark {
  display: grid;
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
  place-items: center;
  border-radius: 8px;
  color: #ffffff;
  background: var(--aiops-accent);
  font-size: 12px;
  font-weight: 700;
}

.logo-text,
.logo-sub {
  display: block;
  white-space: nowrap;
}

.logo-text {
  color: #f2f4f7;
  font-size: 14px;
  font-weight: 600;
}

.logo-sub {
  margin-top: 3px;
  color: #667085;
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.navigation {
  flex: 1;
  min-height: 0;
  padding: 12px 0;
  overflow-y: auto;
}

.nav-group {
  margin-bottom: 12px;
}

.nav-group-label {
  padding: 7px 18px 5px;
  color: #667085;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.sider :deep(.arco-menu) {
  background: transparent !important;
}

.sider :deep(.arco-menu-inner) {
  background: transparent !important;
}

.sider :deep(.arco-menu-item) {
  width: calc(100% - 16px);
  height: 38px;
  margin: 2px 8px;
  border-radius: 6px;
  color: #98a2b3 !important;
  background: transparent !important;
  font-size: 13px;
}

.sider :deep(.arco-menu-item:hover) {
  color: #f2f4f7 !important;
  background: rgba(255, 255, 255, 0.06) !important;
}

.sider :deep(.arco-menu-selected) {
  color: #ffffff !important;
  background: rgba(47, 111, 237, 0.24) !important;
  box-shadow: inset 2px 0 #7aa7ff;
}

.sider :deep(.arco-menu-icon),
.sider :deep(.arco-menu-selected .arco-menu-icon) {
  color: currentColor !important;
}

.sider-foot {
  display: flex;
  flex-shrink: 0;
  gap: 9px;
  align-items: flex-start;
  margin: 8px 14px 16px;
  padding-top: 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.sider-foot-dot {
  flex: 0 0 6px;
  width: 6px;
  height: 6px;
  margin-top: 5px;
  border-radius: 50%;
  background: #36cfc9;
}

.sider-foot strong,
.sider-foot span {
  display: block;
}

.sider-foot strong {
  color: #d0d5dd;
  font-size: 11px;
  font-weight: 500;
}

.sider-foot span {
  margin-top: 4px;
  color: #667085;
  font-size: 10px;
}

.sider.arco-layout-sider-collapsed .logo-copy,
.sider.arco-layout-sider-collapsed .nav-group-label,
.sider.arco-layout-sider-collapsed .sider-foot div {
  display: none;
}

.content-shell {
  min-width: 0;
  background: var(--aiops-paper);
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 56px;
  padding: 0 20px;
  color: var(--aiops-text-1);
  background: var(--aiops-surface);
  border-bottom: 1px solid var(--aiops-border);
}

.header-left,
.header-right {
  display: flex;
  align-items: center;
}

.header-left {
  gap: 9px;
  min-width: 0;
}

.header-right {
  gap: 8px;
}

.header-section {
  color: var(--aiops-text-3);
  font-size: 12px;
}

.header-separator {
  color: #c4ccd6;
}

.header-title {
  overflow: hidden;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-entry {
  color: var(--aiops-accent) !important;
}

.header-divider {
  width: 1px;
  height: 20px;
  margin: 0 4px;
  background: var(--aiops-border);
}

.user-avatar {
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 50%;
  color: var(--aiops-accent-strong);
  background: var(--aiops-accent-soft);
  font-size: 12px;
  font-weight: 600;
}

.user-name {
  max-width: 140px;
  overflow: hidden;
  color: var(--aiops-text-2);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logout-button {
  color: var(--aiops-text-3) !important;
}

.main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  padding: 16px;
  overflow: auto;
  background: var(--aiops-paper);
}

@media (max-width: 900px) {
  .header {
    padding: 0 12px;
  }

  .header-section,
  .header-separator,
  .user-name {
    display: none;
  }

  .main {
    padding: 12px;
  }
}
</style>
