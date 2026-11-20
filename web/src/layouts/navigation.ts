import {
  IconBarChart,
  IconBook,
  IconBranch,
  IconBulb,
  IconCloud,
  IconCommand,
  IconDashboard,
  IconFile,
  IconNotification,
  IconRobot,
  IconStorage,
  IconTool
} from '@arco-design/web-vue/es/icon'

export interface NavigationItem {
  path: string
  title: string
  icon: unknown
}

export interface NavigationGroup {
  title: string
  items: NavigationItem[]
}

export const navigationGroups: NavigationGroup[] = [
  {
    title: '工作台',
    items: [
      { path: '/dashboard', title: '运维总览', icon: IconDashboard },
      { path: '/ai-assistant', title: 'AI 运维助手', icon: IconRobot }
    ]
  },
  {
    title: '运维处置',
    items: [
      { path: '/alerts', title: '告警中心', icon: IconNotification },
      { path: '/inspections', title: '智能巡检', icon: IconBulb },
      { path: '/executions', title: '自动化执行', icon: IconCommand },
      { path: '/runbooks', title: 'Runbook 预案', icon: IconBook }
    ]
  },
  {
    title: '资源与观测',
    items: [
      { path: '/assets', title: 'CMDB 资产', icon: IconStorage },
      { path: '/assets/topology', title: 'CMDB 拓扑', icon: IconBranch },
      { path: '/observability', title: '观测查询', icon: IconBarChart }
    ]
  },
  {
    title: '平台治理',
    items: [
      { path: '/integrations', title: '云账号接入', icon: IconCloud },
      { path: '/audits', title: '审计中心', icon: IconFile },
      { path: '/system', title: '系统设置', icon: IconTool }
    ]
  }
]

export const navigationItems = navigationGroups.flatMap((group) => group.items)
