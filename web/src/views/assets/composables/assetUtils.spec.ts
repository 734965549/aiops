// @vitest-environment jsdom
import type { SyncBatchSummary } from '@/api/asset'
import { describe, expect, it } from 'vitest'
import {
  formatSyncBatchSummary,
  humanizeSyncPartialReason,
  isSensitiveLabelKey,
  labelEntries,
  maskLabelValue,
  parseSyncBatchMessage,
  preferredLabelKeys
} from './assetUtils'

describe('maskLabelValue', () => {
  it('returns original value for non-sensitive keys', () => {
    expect(maskLabelValue('private_ip', '10.0.0.1')).toBe('10.0.0.1')
    expect(maskLabelValue('namespace', 'SYS.ECS')).toBe('SYS.ECS')
  })

  it('masks sensitive keys with ******', () => {
    expect(maskLabelValue('secret', 's3cr3t')).toBe('******')
    expect(maskLabelValue('api_key', 'AKIDxxxx')).toBe('******')
    expect(maskLabelValue('authorization', 'Bearer abc')).toBe('******')
    expect(maskLabelValue('password', 'p@ss')).toBe('******')
    expect(maskLabelValue('token', 'tk')).toBe('******')
    expect(maskLabelValue('credential', 'c')).toBe('******')
  })

  it('returns empty string for sensitive key with empty value', () => {
    expect(maskLabelValue('api_key', '')).toBe('')
  })

  it('sensitive pattern is case-insensitive', () => {
    expect(maskLabelValue('API_KEY', 'x')).toBe('******')
    expect(maskLabelValue('Token', 'x')).toBe('******')
  })

  it('masks camelCase sensitive keys (clientSecret, accessToken)', () => {
    expect(maskLabelValue('clientSecret', 's3cr3t')).toBe('******')
    expect(maskLabelValue('accessToken', 'tk-xxx')).toBe('******')
    expect(isSensitiveLabelKey('clientSecret')).toBe(true)
    expect(isSensitiveLabelKey('accessToken')).toBe(true)
  })

  it('does not match plain resource fields', () => {
    expect(isSensitiveLabelKey('private_ip')).toBe(false)
    expect(isSensitiveLabelKey('flavor')).toBe(false)
    expect(isSensitiveLabelKey('resource_group_name')).toBe(false)
  })

  it('does not falsely mask keys containing sensitive substrings as part of a larger token', () => {
    // keypair_name 含 "key" 子串但 "keypair" 是独立 token，不应误伤
    expect(isSensitiveLabelKey('keypair_name')).toBe(false)
    expect(maskLabelValue('keypair_name', 'kp-001')).toBe('kp-001')
    // keyword 含 "key" 但 "keyword" 是独立 token
    expect(isSensitiveLabelKey('keyword')).toBe(false)
    // monkey 含 "key" 但 "monkey" 是独立 token
    expect(isSensitiveLabelKey('monkey')).toBe(false)
  })

  it('masks denylist exact matches (ak/sk)', () => {
    expect(isSensitiveLabelKey('ak')).toBe(true)
    expect(isSensitiveLabelKey('sk')).toBe(true)
    expect(maskLabelValue('ak', 'AKIDxxxx')).toBe('******')
  })
})

describe('labelEntries', () => {
  it('returns empty array for nil/undefined', () => {
    expect(labelEntries(undefined)).toEqual([])
    expect(labelEntries(null as unknown as undefined)).toEqual([])
  })

  it('places preferred keys first in defined order, rest sorted alphabetically', () => {
    const entries = labelEntries({
      zeta: 'z',
      flavor: 's6.large.2',
      namespace: 'SYS.ECS',
      alpha: 'a',
      private_ip: '10.0.0.1'
    })
    const keys = entries.map((e) => e.key)
    // preferred (namespace, private_ip, flavor) first, then alpha, zeta
    expect(keys).toEqual(['namespace', 'private_ip', 'flavor', 'alpha', 'zeta'])
  })

  it('filters out empty/undefined values', () => {
    const entries = labelEntries({ namespace: 'SYS.ECS', empty: '', gone: undefined as unknown as string })
    expect(entries.map((e) => e.key)).toEqual(['namespace'])
  })

  it('masks sensitive values in displayValue', () => {
    const entries = labelEntries({ api_key: 'AKIDxxxx', private_ip: '10.0.0.1' })
    const byKey = Object.fromEntries(entries.map((e) => [e.key, e.displayValue]))
    expect(byKey['api_key']).toBe('******')
    expect(byKey['private_ip']).toBe('10.0.0.1')
  })

  it('preferredLabelKeys covers core CES + enrichment fields', () => {
    expect(preferredLabelKeys).toContain('namespace')
    expect(preferredLabelKeys).toContain('dim_name')
    expect(preferredLabelKeys).toContain('resource_group_id')
    expect(preferredLabelKeys).toContain('private_ip')
    expect(preferredLabelKeys).toContain('flavor')
  })
})

describe('parseSyncBatchMessage', () => {
  it('parses known fields from a CES summary message', () => {
    const msg = 'sync_mode=hybrid resource_group_name=全部资源 resource_group_id=rg-001 config_mode_fallback=false resource_group_selection=explicit ces_total=1614 discovered=1600 upserted=1600 failed_scopes=1; enriched=10 enrichment_failed=rds'
    const summary = parseSyncBatchMessage(msg)
    expect(summary.sync_mode).toBe('hybrid')
    expect(summary.resource_group_name).toBe('全部资源')
    expect(summary.resource_group_id).toBe('rg-001')
    expect(summary.config_mode_fallback).toBe('false')
    expect(summary.resource_group_selection).toBe('explicit')
    expect(summary.ces_total).toBe('1614')
    expect(summary.discovered).toBe('1600')
    expect(summary.failed_scopes).toBe('1')
    expect(summary.enriched).toBe('10')
    expect(summary.enrichment_failed).toBe('rds')
  })

  it('omits absent fields', () => {
    const summary = parseSyncBatchMessage('region=cn-north-4 ces_total=5 discovered=5')
    expect(summary.ces_total).toBe('5')
    expect(summary.discovered).toBe('5')
    expect(summary.failed_scopes).toBeUndefined()
    expect(summary.enriched).toBeUndefined()
    expect(summary.sync_mode).toBeUndefined()
  })

  it('returns empty summary for empty message', () => {
    expect(parseSyncBatchMessage('')).toEqual({})
    expect(parseSyncBatchMessage('   ')).toEqual({})
  })

  it('joins multiple matching values with comma', () => {
    const msg = 'ces_total=1; ces_total=2'
    const summary = parseSyncBatchMessage(msg)
    expect(summary.ces_total).toBe('1, 2')
  })
})

describe('humanizeSyncPartialReason', () => {
  it('converts internal markers to readable text', () => {
    const reason = humanizeSyncPartialReason('product_names_empty=true; max_resources_reached=true; query_failed_types=ecs,rds; conversion_failed_types=elb')
    expect(reason).toContain('兜底白名单结果')
    expect(reason).toContain('查询上限截断')
    expect(reason).toContain('查询失败类型：ecs,rds')
    expect(reason).toContain('转换失败类型：elb')
  })

  it('returns empty string for blank input', () => {
    expect(humanizeSyncPartialReason('')).toBe('')
    expect(humanizeSyncPartialReason('   ')).toBe('')
  })
})

describe('formatSyncBatchSummary', () => {
  it('maps only diagnostic fields from structured summary', () => {
    const summary: SyncBatchSummary = {
      ces_total: 1614,
      discovered_count: 1600,
      completed_count: 1600,
      failed_scopes: ['cn-north-4/rds'],
      enriched_count: 10,
      enrichment_failed_types: ['rds'],
      enrichment_warnings: ['dms.kafka', 'vpc.subnet_count'],
      // 诊断字段：不得被映射成 created/updated/stale/failed
      persisted_count: 1590,
      mapped_count: 1600,
      regions: ['cn-north-4', 'cn-east-3'],
      persist_failed_count: 10
    }
    const result = formatSyncBatchSummary(summary, 'ignored')
    expect(result.ces_total).toBe('1614')
    expect(result.discovered).toBe('1600')
    expect(result.completed).toBe('1600')
    expect(result.failed_scopes).toBe('cn-north-4/rds')
    expect(result.enriched).toBe('10')
    expect(result.enrichment_failed).toBe('rds')
  })

  it('maps enrichment_warnings as joined string', () => {
    const summary: SyncBatchSummary = {
      enrichment_warnings: ['dms.kafka', 'vpc.subnet_count']
    }
    const result = formatSyncBatchSummary(summary)
    expect(result.enrichment_warnings).toBe('dms.kafka, vpc.subnet_count')
  })

  it('omits enrichment_warnings when absent', () => {
    const result = formatSyncBatchSummary({ ces_total: 5 })
    expect(result.enrichment_warnings).toBeUndefined()
  })

  it('does not return created/updated/stale/failed from summary diagnostics', () => {
    // 即使 summary 带有 persisted_count/mapped_count/regions/persist_failed_count，
    // 也不应映射到 created/updated/stale/failed，避免契约歧义。
    const summary: SyncBatchSummary = {
      persisted_count: 1590,
      mapped_count: 1600,
      regions: ['cn-north-4', 'cn-east-3'],
      persist_failed_count: 10
    }
    const result = formatSyncBatchSummary(summary)
    expect(result.created).toBeUndefined()
    expect(result.updated).toBeUndefined()
    expect(result.stale).toBeUndefined()
    expect(result.failed).toBeUndefined()
  })

  it('omits absent diagnostic fields', () => {
    const result = formatSyncBatchSummary({ ces_total: 5 })
    expect(result.ces_total).toBe('5')
    expect(result.discovered).toBeUndefined()
    expect(result.completed).toBeUndefined()
    expect(result.failed_scopes).toBeUndefined()
    expect(result.enriched).toBeUndefined()
    expect(result.enrichment_failed).toBeUndefined()
  })

  it('falls back to parseSyncBatchMessage when summary absent', () => {
    const msg = 'ces_total=5 discovered=5 created=3 updated=2 stale=1 failed=0'
    const result = formatSyncBatchSummary(undefined, msg)
    expect(result.ces_total).toBe('5')
    expect(result.discovered).toBe('5')
    // message 分支仍保留 created/updated/stale/failed 文本解析
    expect(result.created).toBe('3')
    expect(result.updated).toBe('2')
    expect(result.stale).toBe('1')
    expect(result.failed).toBe('0')
  })
})
