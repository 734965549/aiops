// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import {
  labelEntries,
  maskLabelValue,
  parseSyncBatchMessage,
  preferredLabelKeys,
  sensitiveLabelKeyPattern
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

  it('sensitiveLabelKeyPattern does not match plain resource fields', () => {
    expect(sensitiveLabelKeyPattern.test('private_ip')).toBe(false)
    expect(sensitiveLabelKeyPattern.test('flavor')).toBe(false)
    expect(sensitiveLabelKeyPattern.test('resource_group_name')).toBe(false)
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
    const msg = 'region=cn-north-4 project=p ces_total=1614 discovered=1600 upserted=1600 failed_scopes=1; enriched=10 enrichment_failed=rds'
    const summary = parseSyncBatchMessage(msg)
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
