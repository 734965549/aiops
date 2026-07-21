<template>
  <a-layout class="layout">
    <OceanBackdrop />
    <a-layout-sider
      :width="228"
      theme="dark"
      breakpoint="lg"
      class="sider"
    >
      <div class="logo">
        <div class="logo-mark">
          AI
        </div>
        <div class="logo-copy">
          <span class="logo-text">AI 运维平台</span>
          <span class="logo-sub">Safe operations</span>
        </div>
      </div>

      <div class="sider-label">
        Control plane / 01
      </div>
      <a-menu
        theme="dark"
        :selected-keys="[activeMenuKey]"
        :default-open-keys="[]"
        @menu-item-click="onMenuClick"
      >
        <a-menu-item
          v-for="item in menuItems"
          :key="item.path"
        >
          <template #icon>
            <component :is="item.icon" />
          </template>
          {{ item.title }}
        </a-menu-item>
      </a-menu>

      <div class="sider-foot">
        <span class="sider-foot-dot" />
        <div>
          <strong>Human in the loop</strong>
          <span>执行始终受权限、确认与审计约束</span>
        </div>
      </div>
    </a-layout-sider>

    <a-layout class="content-shell">
      <a-layout-header class="header">
        <div class="header-left">
          <span class="header-index">AIOPS / 02</span>
          <span class="header-title">{{ currentTitle }}</span>
        </div>
        <div class="header-right">
          <span class="header-principle">SAFE · CONTROLLED · AUDITABLE</span>
          <span class="status-dot" />
          <span class="user-name">{{ auth.user?.display_name || auth.user?.username || '未登录' }}</span>
          <a-button
            type="text"
            size="small"
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
  </a-layout>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  IconDashboard,
  IconRobot,
  IconNotification,
  IconStorage,
  IconCommand,
  IconBook,
  IconFile,
  IconUserGroup,
  IconCloud,
  IconBarChart,
  IconBulb
} from '@arco-design/web-vue/es/icon'
import { useAuthStore } from '@/stores/auth'
import { fetchCurrentUser } from '@/api/system'
import OceanBackdrop from './components/OceanBackdrop.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

interface MenuItem { path: string; title: string; icon: unknown }

const menuItems: MenuItem[] = [
  { path: '/identity/access-control', title: '权限管理', icon: IconUserGroup },
  { path: '/dashboard', title: '首页驾驶舱', icon: IconDashboard },
  { path: '/ai-assistant', title: 'AI 运维助手', icon: IconRobot },
  { path: '/alerts', title: '告警中心', icon: IconNotification },
  { path: '/assets', title: '资源与应用', icon: IconStorage },
  { path: '/integrations', title: '云账号接入', icon: IconCloud },
  { path: '/observability', title: '观测查询', icon: IconBarChart },
  { path: '/inspections', title: '智能巡检', icon: IconBulb },
  { path: '/executions', title: '自动化执行', icon: IconCommand },
  { path: '/runbooks', title: 'Runbook 预案', icon: IconBook },
  { path: '/audits', title: '审计中心', icon: IconFile },
  { path: '/identity/ldap-import', title: '域账号导入', icon: IconUserGroup }
]

const activeMenuKey = computed(() => {
  const metaMenu = route.meta.activeMenu as string | undefined
  if (metaMenu) return metaMenu
  const hit = menuItems.find(
    (item) => route.path === item.path || route.path.startsWith(`${item.path}/`)
  )
  return hit?.path ?? route.path
})

const currentTitle = computed(() => {
  const m = menuItems.find((i) => i.path === activeMenuKey.value)
  return m?.title || (route.meta.title as string) || ''
})

function onMenuClick(key: string) {
  if (key && key !== route.path) {
    router.push(key)
  }
}

async function onLogout() {
  await auth.logout()
  router.replace('/login')
}

onMounted(async () => {
  if (!auth.token) return
  try {
    const u = await fetchCurrentUser()
    auth.setUser(u)
  } catch {
    /* 错误已由拦截器提示；用户信息刷新失败不阻断布局渲染。 */
  }
})
</script>

<style scoped lang="scss">
.layout {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: #063b45;
}

.sider {
  position: relative;
  z-index: 3;
  display: flex;
  flex-direction: column;
  background: rgba(3, 29, 36, 0.84) !important;
  border-right: 1px solid rgba(193, 239, 230, 0.17);
  box-shadow: 18px 0 44px rgba(0, 21, 29, 0.18);
  backdrop-filter: blur(20px) saturate(1.08);
}

.logo {
  height: 82px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 20px;
  color: #effbf7;
  border-bottom: 1px solid rgba(193, 239, 230, 0.14);
}

.logo-mark {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border: 1px solid rgba(214, 255, 246, 0.5);
  border-radius: 50%;
  color: #f6f5ef;
  font-family: var(--aiops-display);
  font-size: 13px;
  font-style: italic;
  font-weight: 700;
  background: rgba(14, 78, 86, 0.78);
  box-shadow: inset 0 0 18px rgba(163, 242, 226, 0.12);
}

.logo-text,
.logo-sub {
  display: block;
}

.logo-text {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.logo-sub {
  margin-top: 3px;
  color: rgba(210, 238, 232, 0.62);
  font-family: var(--aiops-display);
  font-size: 11px;
  font-style: italic;
}

.sider-label {
  padding: 22px 20px 8px;
  color: rgba(200, 235, 228, 0.48);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.sider :deep(.arco-menu) {
  flex: 1;
  padding-bottom: 154px;
  background: transparent !important;
}

.sider :deep(.arco-menu-inner) {
  background: transparent !important;
}

.sider :deep(.arco-menu-item) {
  width: calc(100% - 24px);
  height: 40px;
  margin: 3px 12px;
  border-radius: 0;
  color: rgba(232, 248, 244, 0.82) !important;
  background: transparent !important;
  font-size: 13px;
  transition: color 180ms ease, background 180ms ease, padding 180ms ease;
}

.sider :deep(.arco-menu-item:hover) {
  color: #ffffff !important;
  background: rgba(113, 194, 187, 0.13) !important;
}

.sider :deep(.arco-menu-selected) {
  padding-left: 20px;
  color: #f4fffc !important;
  background: rgba(97, 184, 177, 0.24) !important;
  box-shadow: inset 2px 0 #a4e5da;
}

.sider :deep(.arco-menu-icon) {
  color: rgba(190, 234, 226, 0.76) !important;
}

.sider :deep(.arco-menu-selected .arco-menu-icon) {
  color: #9af0df !important;
}

.sider-foot {
  position: absolute;
  right: 16px;
  bottom: 18px;
  left: 16px;
  display: flex;
  gap: 10px;
  padding: 14px;
  border-top: 1px solid rgba(193, 239, 230, 0.14);
  color: rgba(212, 239, 233, 0.56);
}

.sider-foot-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  margin-top: 4px;
  border-radius: 50%;
  background: #7de0cb;
  box-shadow: 0 0 0 5px rgba(125, 224, 203, 0.11), 0 0 14px rgba(125, 224, 203, 0.42);
}

.sider-foot strong,
.sider-foot span {
  display: block;
}

.sider-foot strong {
  color: rgba(236, 252, 248, 0.82);
  font-size: 11px;
  letter-spacing: 0.04em;
}

.sider-foot span {
  margin-top: 4px;
  font-size: 10px;
  line-height: 1.55;
}

.content-shell {
  position: relative;
  z-index: 2;
  min-width: 0;
  background: transparent;
}

.header {
  height: 66px;
  margin: 12px 14px 0;
  padding: 0 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #effbf8;
  border: 1px solid rgba(190, 239, 231, 0.2);
  border-radius: 16px 16px 0 0;
  background: rgba(4, 38, 46, 0.68);
  box-shadow: 0 18px 50px rgba(0, 25, 33, 0.13);
  backdrop-filter: blur(20px) saturate(1.1);

  .header-left,
  .header-right {
    display: flex;
    align-items: center;
  }

  .header-left {
    gap: 14px;
  }

  .header-right {
    gap: 10px;
    color: rgba(228, 247, 242, 0.7);
  }
}

.header-index {
  color: rgba(202, 237, 230, 0.55);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.header-title {
  font-family: var(--aiops-display);
  font-size: 19px;
  font-style: italic;
  font-weight: 500;
}

.header-principle {
  margin-right: 8px;
  color: rgba(205, 238, 232, 0.52);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #7de0cb;
  box-shadow: 0 0 0 4px rgba(125, 224, 203, 0.11), 0 0 12px rgba(125, 224, 203, 0.38);
}

.user-name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main {
  position: relative;
  min-height: calc(100vh - 90px);
  margin: 0 14px 14px;
  padding: 18px;
  border: 1px solid rgba(190, 239, 231, 0.2);
  border-top: 0;
  border-radius: 0 0 16px 16px;
  background: radial-gradient(
    ellipse 58% 54% at 50% 42%,
    rgba(246, 250, 248, 0.94) 0%,
    rgba(241, 248, 245, 0.86) 58%,
    rgba(231, 244, 241, 0.68) 100%
  );
  box-shadow: 0 26px 70px rgba(0, 23, 31, 0.18);
  backdrop-filter: blur(18px) saturate(0.9);
  overflow: auto;
}

.header :deep(.arco-btn-text) {
  color: rgba(236, 252, 248, 0.78);
}

.header :deep(.arco-btn-text:hover) {
  color: #ffffff;
  background: rgba(161, 226, 215, 0.12);
}

@media (max-width: 1080px) {
  .header-principle {
    display: none;
  }
}

@media (max-width: 900px) {
  .header {
    height: 60px;
    margin: 8px 8px 0;
    padding: 0 14px;
  }

  .header-index,
  .user-name {
    display: none;
  }

  .main {
    margin: 0 8px 8px;
    padding: 10px;
  }
}
</style>
