import type { Alert } from '@/api/alert'

const ENVIRONMENT_LABELS: Record<string, string> = {
  prod: '生产环境',
  production: '生产环境',
  staging: '预发环境',
  test: '测试环境',
  dev: '开发环境',
  development: '开发环境'
}

const SEVERITY_VALUE_LABELS: Record<string, string> = {
  critical: '严重',
  fatal: '致命',
  warning: '警告',
  warn: '警告',
  info: '提示',
  information: '提示'
}

const RESOURCE_TYPE_LABELS: Record<string, string> = {
  host: '主机',
  pod: 'Pod',
  service: '服务',
  container: '容器',
  ecs: '云服务器（ECS）',
  rds: '云数据库（RDS）',
  elb: '负载均衡（ELB）',
  evs: '云硬盘（EVS）',
  vpc: '虚拟私有云（VPC）',
  subnet: '子网',
  dcs: '分布式缓存（DCS）',
  dms: '分布式消息（DMS）',
  obs: '对象存储（OBS）',
  bandwidth: '带宽',
  eip: '弹性公网 IP',
  peering: 'VPC 对等连接'
}

const SOURCE_TYPE_LABELS: Record<string, string> = {
  prometheus_alertmanager: 'Prometheus Alertmanager',
  huawei_ces: '华为云 CES',
  webhook: 'Webhook 告警源'
}

const REGION_LABELS: Record<string, string> = {
  'cn-north-1': '华北-北京一',
  'cn-north-4': '华北-北京四',
  'cn-east-3': '华东-上海一',
  'cn-south-1': '华南-广州'
}

const ANNOTATION_LABELS: Record<string, string> = {
  summary: '规则摘要',
  description: '规则描述',
  alertname: '告警名称',
  severity: '严重程度',
  message: '规则消息',
  runbook_url: 'Runbook 链接'
}

function dualLabel(label: string | undefined, raw: string) {
  if (!raw) return '—'
  return label && label !== raw ? `${label}（${raw}）` : raw
}

export function environmentText(value?: string) {
  const raw = (value || '').trim()
  return dualLabel(ENVIRONMENT_LABELS[raw.toLowerCase()], raw)
}

export function severityValueText(value?: string) {
  const raw = (value || '').trim()
  return dualLabel(SEVERITY_VALUE_LABELS[raw.toLowerCase()], raw)
}

export function resourceTypeText(value?: string) {
  const raw = (value || '').trim()
  return dualLabel(RESOURCE_TYPE_LABELS[raw.toLowerCase()], raw)
}

export function sourceTypeText(value?: string) {
  const raw = (value || '').trim()
  return SOURCE_TYPE_LABELS[raw.toLowerCase()] || raw || '未知来源'
}

export function regionText(value?: string) {
  const raw = (value || '').trim()
  return dualLabel(REGION_LABELS[raw.toLowerCase()], raw)
}

export function annotationLabel(key: string) {
  return ANNOTATION_LABELS[key] || key
}

export function labelValueText(key: string, value: string) {
  switch (key) {
    case 'environment':
      return environmentText(value)
    case 'severity':
      return severityValueText(value)
    case 'resource_type':
      return resourceTypeText(value)
    case 'region':
      return regionText(value)
    default:
      return value
  }
}

export function severityLabel(value: string) {
  const map: Record<string, string> = {
    p0: 'P0 · 紧急',
    p1: 'P1 · 高',
    p2: 'P2 · 中',
    p3: 'P3 · 低',
    info: 'Info · 提示'
  }
  return map[value] || severityValueText(value)
}

export function severityColor(value: string) {
  const map: Record<string, string> = {
    p0: 'red',
    p1: 'orangered',
    p2: 'orange',
    p3: 'gold',
    info: 'arcoblue'
  }
  return map[value] || 'gray'
}

export function statusLabel(value: string) {
  const map: Record<string, string> = {
    new: '新建',
    acknowledged: '已认领',
    processing: '处理中',
    recovered: '已恢复',
    closed: '已关闭',
    silenced: '已静默'
  }
  return map[value] || value
}

export function statusColor(value: string) {
  const map: Record<string, string> = {
    new: 'red',
    acknowledged: 'orangered',
    processing: 'orange',
    recovered: 'green',
    closed: 'gray',
    silenced: 'purple'
  }
  return map[value] || 'gray'
}

export function sourceLabel(alert: Alert) {
  return alert.source_name || alert.source_id || alert.source || '未知来源'
}

export function formatTime(timestamp?: number) {
  if (!timestamp) return '—'
  return new Date(timestamp * 1000).toLocaleString('zh-CN', { hour12: false })
}
