import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/login/index.vue'),
    meta: { public: true, title: '登录' }
  },
  {
    path: '/',
    component: () => import('@/layouts/BasicLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/dashboard/index.vue'),
        meta: { title: '首页驾驶舱', icon: 'Odometer' }
      },
      {
        path: 'ai-assistant',
        name: 'ai-assistant',
        component: () => import('@/views/ai-assistant/index.vue'),
        meta: { title: 'AI 运维助手', icon: 'ChatDotRound' }
      },
      {
        path: 'alerts',
        name: 'alerts',
        component: () => import('@/views/alerts/index.vue'),
        meta: { title: '告警中心', icon: 'Bell' }
      },
      {
        path: 'assets',
        name: 'assets',
        component: () => import('@/views/assets/index.vue'),
        meta: { title: '资源与应用', icon: 'Cpu' }
      },
      {
        path: 'executions',
        name: 'executions',
        component: () => import('@/views/executions/index.vue'),
        meta: { title: '自动化执行', icon: 'Operation' }
      },
      {
        path: 'runbooks',
        name: 'runbooks',
        component: () => import('@/views/runbooks/index.vue'),
        meta: { title: 'Runbook 预案', icon: 'Collection' }
      },
      {
        path: 'integrations',
        name: 'integrations',
        component: () => import('@/views/integrations/index.vue'),
        meta: { title: '云账号接入', icon: 'Cloud' }
      },
      {
        path: 'observability',
        name: 'observability',
        component: () => import('@/views/observability/index.vue'),
        meta: { title: '观测查询', icon: 'BarChart' }
      },
      {
        path: 'inspections',
        name: 'inspections',
        component: () => import('@/views/inspections/index.vue'),
        meta: { title: '智能巡检', icon: 'Robot' }
      },
      {
        path: 'audits',
        name: 'audits',
        component: () => import('@/views/audits/index.vue'),
        meta: { title: '审计中心', icon: 'Document' }
      },
      {
        path: 'identity/ldap-import',
        name: 'identity-ldap-import',
        component: () => import('@/views/identity/ldap-import/index.vue'),
        meta: { title: '域账号导入', icon: 'UserGroup' }
      },
      {
        path: 'identity/access-control',
        name: 'identity-access-control',
        component: () => import('@/views/identity/access-control/index.vue'),
        meta: { title: '权限管理', icon: 'UserGroup' }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/error/404.vue'),
    meta: { public: true, title: '页面未找到' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, _from, next) => {
  const auth = useAuthStore()
  if (!auth.token) {
    auth.loadFromStorage()
  }
  if (to.path === '/login' && auth.token) {
    const redirect = typeof to.query.redirect === 'string' ? to.query.redirect : ''
    next({ path: redirect && redirect.startsWith('/') ? redirect : '/dashboard' })
    return
  }
  if (to.meta.public) {
    next()
    return
  }
  if (auth.token) {
    next()
    return
  }
  next({ path: '/login', query: { redirect: to.fullPath } })
})

router.afterEach((to) => {
  const base = 'AI 运维平台'
  document.title = to.meta.title ? `${to.meta.title} - ${base}` : base
})

export default router
