<template>
  <div class="page-shell settings-shell">
    <aside class="settings-nav">
      <div class="settings-nav-head">
        <span class="settings-nav-eyebrow">Control plane / Settings</span>
        <strong class="settings-nav-title">系统设置</strong>
        <p class="settings-nav-desc">
          平台运行相关的配置与账号治理入口，点击后在右侧面板处理。
        </p>
      </div>

      <nav class="settings-nav-list">
        <RouterLink
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          class="settings-nav-item"
          :class="{ 'settings-nav-item--active': isActive(item.path) }"
        >
          <span class="settings-nav-icon">
            <component :is="item.icon" />
          </span>
          <span class="settings-nav-copy">
            <strong>{{ item.title }}</strong>
            <small>{{ item.desc }}</small>
          </span>
          <IconArrowRight class="settings-nav-arrow" />
        </RouterLink>
      </nav>
    </aside>

    <section class="settings-pane">
      <RouterView />
    </section>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import {
  IconArrowRight,
  IconSettings,
  IconUserAdd,
  IconUserGroup
} from '@arco-design/web-vue/es/icon'

interface SettingsNavItem {
  path: string
  title: string
  desc: string
  icon: unknown
}

const route = useRoute()

const navItems: SettingsNavItem[] = [
  {
    path: '/system/ai-config',
    title: 'AI 配置',
    desc: 'Provider、工具调试与知识沉淀',
    icon: IconSettings
  },
  {
    path: '/identity/access-control',
    title: '权限管理',
    desc: '用户角色绑定与授权校验',
    icon: IconUserGroup
  },
  {
    path: '/identity/ldap-import',
    title: '域账号导入',
    desc: 'LDAP / AD 目录浏览与批量导入',
    icon: IconUserAdd
  }
]

function isActive(path: string) {
  return route.path === path || route.path.startsWith(`${path}/`)
}
</script>

<style scoped lang="scss">
.settings-shell {
  flex-direction: row;
  gap: var(--space-4);
}

.settings-nav {
  flex: 0 0 248px;
  width: 248px;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 20px 14px;
  border: 1px solid var(--aiops-border-soft);
  border-radius: var(--radius-lg);
  background: var(--aiops-panel);
  box-shadow: 0 16px 40px rgba(24, 27, 24, 0.05);
  overflow-y: auto;
}

.settings-nav-head {
  padding: 2px 8px 16px;
  border-bottom: 1px solid var(--aiops-border-soft);
}

.settings-nav-eyebrow {
  display: block;
  color: var(--aiops-text-3);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.settings-nav-title {
  display: block;
  margin-top: 8px;
  color: var(--aiops-text-1);
  font-family: var(--aiops-display);
  font-size: 20px;
  font-weight: 600;
}

.settings-nav-desc {
  margin: 6px 0 0;
  color: var(--aiops-text-3);
  font-size: 11px;
  line-height: 1.6;
}

.settings-nav-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-top: var(--space-4);
}

.settings-nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 11px 12px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--aiops-text-2);
  transition: background-color 160ms ease, border-color 160ms ease, color 160ms ease;
}

.settings-nav-item:hover {
  color: var(--aiops-text-1);
  background: rgba(81, 102, 63, 0.07);
}

.settings-nav-item--active,
.settings-nav-item--active:hover {
  color: var(--aiops-accent-strong);
  border-color: rgba(81, 102, 63, 0.26);
  background: var(--aiops-accent-soft);
  box-shadow: inset 2px 0 var(--aiops-accent);
}

.settings-nav-icon {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-md);
  background: rgba(81, 102, 63, 0.08);
  font-size: 16px;
}

.settings-nav-item--active .settings-nav-icon {
  background: rgba(81, 102, 63, 0.16);
}

.settings-nav-copy {
  flex: 1;
  min-width: 0;
}

.settings-nav-copy strong,
.settings-nav-copy small {
  display: block;
}

.settings-nav-copy strong {
  font-size: 13px;
  font-weight: 600;
}

.settings-nav-copy small {
  margin-top: 3px;
  overflow: hidden;
  color: var(--aiops-text-3);
  font-size: 11px;
  line-height: 1.45;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-nav-arrow {
  flex: 0 0 auto;
  color: rgba(24, 27, 24, 0.28);
  font-size: 12px;
}

.settings-nav-item--active .settings-nav-arrow {
  color: var(--aiops-accent);
}

.settings-pane {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.settings-pane :deep(.page-shell) {
  flex: 1;
  min-height: 0;
}

@media (max-width: 960px) {
  .settings-shell {
    flex-direction: column;
  }

  .settings-nav {
    flex: 0 0 auto;
    width: 100%;
    padding: 14px;
  }

  .settings-nav-head {
    display: none;
  }

  .settings-nav-list {
    flex-direction: row;
    flex-wrap: wrap;
    margin-top: 0;
  }

  .settings-nav-item {
    flex: 1 1 180px;
  }

  .settings-nav-arrow {
    display: none;
  }
}
</style>
