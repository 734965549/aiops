<template>
  <a-layout class="layout">
    <div class="shell-glow shell-glow-a" />
    <div class="shell-glow shell-glow-b" />

    <a-layout-sider
      :width="232"
      theme="dark"
      breakpoint="lg"
      class="sider"
    >
      <div class="logo">
        <div class="logo-mark">
          AI
        </div>
        <div>
          <span class="logo-text">AI 运维平台</span>
          <span class="logo-sub">AIOps Command</span>
        </div>
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
    </a-layout-sider>

    <a-layout class="content-shell">
      <a-layout-header class="header">
        <div class="header-left">
          <span class="header-kicker">OPS CONTROL</span>
          <span class="header-title">{{ currentTitle }}</span>
        </div>
        <div class="header-right">
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
import { getApiError } from '@/api/request'

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
  } catch (err) {
    const apiErr = getApiError(err)
    if (apiErr) {
      console.warn('[identity/me]', {
        status: apiErr.status,
        code: apiErr.code,
        message: apiErr.message,
        trace_id: apiErr.traceId
      })
    }
  }
})
</script>

<style scoped lang="scss">
.layout {
  min-height: 100vh;
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(circle at 16% 12%, rgba(22, 93, 255, 0.28), transparent 28%),
    radial-gradient(circle at 82% 8%, rgba(0, 255, 226, 0.22), transparent 24%),
    linear-gradient(135deg, #081224 0%, #0c1730 46%, #0d1c35 100%);
}

.layout::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(125, 211, 252, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(125, 211, 252, 0.08) 1px, transparent 1px);
  background-size: 42px 42px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.9), transparent 90%);
}

.shell-glow {
  position: absolute;
  width: 360px;
  height: 360px;
  border-radius: 999px;
  filter: blur(28px);
  opacity: 0.45;
  pointer-events: none;
  animation: float-glow 12s ease-in-out infinite;
}

.shell-glow-a {
  left: 14%;
  bottom: 6%;
  background: rgba(22, 93, 255, 0.2);
}

.shell-glow-b {
  right: 8%;
  top: 16%;
  background: rgba(0, 255, 204, 0.16);
  animation-delay: -5s;
}

.sider {
  position: relative;
  z-index: 2;
  background:
    linear-gradient(180deg, rgba(7, 18, 38, 0.98), rgba(7, 22, 47, 0.92)) !important;
  border-right: 1px solid rgba(125, 211, 252, 0.18);
  box-shadow: 12px 0 34px rgba(0, 0, 0, 0.22);
}

.logo {
  height: 72px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 20px;
  color: #fff;
  border-bottom: 1px solid rgba(125, 211, 252, 0.16);
}

.logo-mark {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  color: #dffcff;
  font-weight: 800;
  letter-spacing: 0;
  background: linear-gradient(135deg, #165dff, #00dcc5);
  box-shadow: 0 0 24px rgba(0, 220, 197, 0.42);
}

.logo-text,
.logo-sub {
  display: block;
}

.logo-text {
  font-weight: 700;
  font-size: 16px;
}

.logo-sub {
  margin-top: 2px;
  color: rgba(191, 219, 254, 0.68);
  font-size: 11px;
  text-transform: uppercase;
}

.content-shell {
  position: relative;
  z-index: 1;
  background: transparent;
}

.header {
  height: 64px;
  margin: 12px 16px 0;
  padding: 0 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #e5f7ff;
  border: 1px solid rgba(125, 211, 252, 0.18);
  border-radius: 8px;
  background: rgba(8, 22, 44, 0.68);
  backdrop-filter: blur(18px);
  box-shadow: 0 18px 60px rgba(1, 7, 19, 0.3);

  .header-left,
  .header-right {
    display: flex;
    align-items: center;
  }

  .header-left {
    gap: 12px;
  }

  .header-right {
    gap: 10px;
    color: rgba(226, 246, 255, 0.78);
  }
}

.header-kicker {
  padding: 4px 8px;
  border-radius: 999px;
  color: #8ee7ff;
  font-size: 11px;
  font-weight: 700;
  background: rgba(56, 189, 248, 0.12);
  border: 1px solid rgba(56, 189, 248, 0.24);
}

.header-title {
  font-size: 18px;
  font-weight: 700;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #22c55e;
  box-shadow: 0 0 16px rgba(34, 197, 94, 0.8);
}

.main {
  min-height: calc(100vh - 88px);
  margin: 16px;
  padding: 18px;
  border: 1px solid rgba(125, 211, 252, 0.14);
  border-radius: 8px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.9), rgba(236, 248, 255, 0.82)),
    radial-gradient(circle at top right, rgba(22, 93, 255, 0.12), transparent 34%);
  box-shadow: 0 18px 60px rgba(0, 8, 24, 0.24);
  overflow: auto;
}

@keyframes float-glow {
  0%,
  100% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  50% {
    transform: translate3d(26px, -18px, 0) scale(1.08);
  }
}

@media (max-width: 900px) {
  .header {
    margin: 8px 10px 0;
    padding: 0 14px;
  }

  .header-kicker {
    display: none;
  }

  .main {
    margin: 10px;
    padding: 12px;
  }
}
</style>
