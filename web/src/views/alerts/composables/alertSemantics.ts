import type { Alert } from '@/api/alert'
import { metricDefinition, namespaceText, resourceKindText } from './alertMetrics'
import { parseCondition } from './alertPresentation'

export type SemanticAlert = Pick<Alert, 'labels' | 'annotations' | 'source'> & Partial<Pick<Alert, 'name' | 'summary' | 'description' | 'resource_type'>>

export interface AlertSemantics {
  headline: string
  description: string
  suggestion: string
  metricTitle: string
  metricName: string
  namespace: string
  thresholdText: string
  evaluationText: string
  conditionText: string
  resourceKind: string
  hasDefinition: boolean
}

const COMPARISON_TEXT: Record<string, string> = {
  '>': '超过',
  '>=': '不低于',
  '<': '低于',
  '<=': '不超过',
  '=': '等于',
  '!=': '不等于'
}

export function formatThresholdValue(value: string, unit: string) {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return `${value}${unit}`
  if (unit === 'ms') {
    if (numeric % 60000 === 0) return `${numeric / 60000} 分钟（${value}ms）`
    if (numeric % 1000 === 0) return `${numeric / 1000} 秒（${value}ms）`
  }
  if (unit === 's' && numeric % 60 === 0) return `${numeric / 60} 分钟（${value}s）`
  return `${value}${unit}`
}

export function buildAlertSemantics(alert: SemanticAlert): AlertSemantics {
  const labels = alert.labels || {}
  const annotations = alert.annotations || {}
  const metricName = labels.metric_name || annotations.ces_metric_name || ''
  const namespace = labels.ces_namespace || annotations.ces_namespace || ''
  const definition = metricDefinition(namespace, metricName)
  const condition = parseCondition(annotations.ces_condition || '')
  const thresholdText = condition.ok ? formatThresholdValue(condition.threshold, condition.unit) : ''
  const comparison = COMPARISON_TEXT[condition.op] || condition.opText || '触发阈值'
  const conditionText = definition && condition.ok
    ? `${definition.title}${comparison} ${thresholdText}`
    : ''
  return {
    headline: conditionText || definition?.title || alert.summary || alert.name || '告警详情',
    description: definition?.meaning || alert.description || '未提供描述',
    suggestion: definition?.suggestion || '',
    metricTitle: definition?.title || metricName,
    metricName,
    namespace: namespaceText(namespace),
    thresholdText,
    evaluationText: condition.ok ? condition.windowText : '',
    conditionText,
    resourceKind: resourceKindText(alert),
    hasDefinition: Boolean(definition)
  }
}

export function alertHeadline(alert: SemanticAlert) {
  return buildAlertSemantics(alert).headline
}
