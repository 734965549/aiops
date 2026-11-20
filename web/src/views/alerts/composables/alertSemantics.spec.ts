import { describe, expect, it } from 'vitest'
import type { Alert } from '@/api/alert'
import { buildAlertSemantics, formatThresholdValue } from './alertSemantics'

function rdsAlert(): Alert {
  return {
    id: 'alert-rds', source: 'huawei_ces', fingerprint: 'fp', dedup_key: 'dk', name: 'pg-alert',
    summary: '30715ce08c524a33b4fe8a203c56f84in03',
    description: 'SYS.RDS oldest_transaction_duration > 600000ms (average, 1s x1) · 30715ce08c524a33b4fe8a203c56f84in03',
    severity: 'p1', status: 'recovered', environment: '', labels: {
      cloud_resource_id: '30715ce08c524a33b4fe8a203c56f84in03',
      postgresql_cluster_id: '30715ce08c524a33b4fe8a203c56f84in03',
      resource_type: 'rds', region: 'cn-south-1', ces_namespace: 'SYS.RDS', metric_name: 'oldest_transaction_duration'
    },
    annotations: { ces_namespace: 'SYS.RDS', ces_metric_name: 'oldest_transaction_duration', ces_condition: '> 600000ms (average, 1s x1)' },
    occurrence_count: 1, first_seen_at: 1, last_seen_at: 2, created_at: 1, updated_at: 2
  }
}

describe('alert semantics', () => {
  it('explains the RDS long transaction alert without using the ID as the headline', () => {
    const semantic = buildAlertSemantics(rdsAlert())
    expect(semantic.headline).toBe('最长事务存活时长超过 10 分钟（600000ms）')
    expect(semantic.description).toContain('统计当前数据库中存在的最长事务存活时长')
    expect(semantic.suggestion).toContain('未提交事务')
    expect(semantic.resourceKind).toBe('RDS PostgreSQL 集群')
    expect(semantic.namespace).toBe('云数据库 RDS')
    expect(semantic.evaluationText).toContain('1 秒')
  })

  it('keeps unknown metrics on their original content', () => {
    const alert = rdsAlert()
    alert.labels.metric_name = 'unknown_metric'
    alert.annotations.ces_metric_name = 'unknown_metric'
    const semantic = buildAlertSemantics(alert)
    expect(semantic.hasDefinition).toBe(false)
    expect(semantic.headline).toBe(alert.summary)
    expect(semantic.description).toBe(alert.description)
  })

  it('formats millisecond thresholds only when the conversion is exact', () => {
    expect(formatThresholdValue('600000', 'ms')).toBe('10 分钟（600000ms）')
    expect(formatThresholdValue('30000', 'ms')).toBe('30 秒（30000ms）')
    expect(formatThresholdValue('1500', 'ms')).toBe('1500ms')
    expect(formatThresholdValue('90', '%')).toBe('90%')
  })
})
