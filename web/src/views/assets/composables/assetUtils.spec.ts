// @vitest-environment jsdom
import type { Resource, SyncBatchSummary } from '@/api/asset'
import { describe, expect, it } from 'vitest'
import {
  OPTIONAL_RESOURCE_COLUMNS,
  buildResourceCsv,
  buildSyncBatchSignalTags,
  csvCell,
  sanitizeVisibleColumns,
  formatSyncBatchSummary,
  humanizeSyncPartialReason,
  isSensitiveLabelKey,
  labelEntries,
  maskLabelValue,
  parseSyncBatchMessage,
  preferredLabelKeys,
  resourceFiltersFromQuery,
  resourceSortFromQuery,
  shortId,
  toResourceSortParams,
  withResourceQuery
} from './assetUtils'

describe('shortId', () => {
  it('returns short or empty values unchanged', () => {
    expect(shortId('')).toBe('')
    expect(shortId(undefined)).toBe('')
    expect(shortId('cn-south-1')).toBe('cn-south-1')
  })

  it('keeps head and tail of long ids', () => {
    expect(shortId('acc-50063711-8bfe-4d3e-9b05-1234abcd')).toBe('acc-5006…abcd')
    expect(shortId('0123456789abcdef', 4, 2)).toBe('0123…ef')
  })

  it('trims surrounding whitespace before shortening', () => {
    expect(shortId('  0123456789abcdef  ', 4, 2)).toBe('0123…ef')
  })
})

describe('OPTIONAL_RESOURCE_COLUMNS', () => {
  it('keeps keys unique and every column renderable', () => {
    const keys = OPTIONAL_RESOURCE_COLUMNS.map((col) => col.key)
    expect(new Set(keys).size).toBe(keys.length)
    expect(OPTIONAL_RESOURCE_COLUMNS.every((col) => col.slotName || col.dataIndex)).toBe(true)
    expect(OPTIONAL_RESOURCE_COLUMNS.every((col) => col.width > 0)).toBe(true)
  })
})

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

describe('buildSyncBatchSignalTags', () => {
  it('shows aggregate resource group count when scope covers multiple groups', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      resource_group_count: 10,
      ces_total: 100,
      discovered_count: 90
    }
    const tags = buildSyncBatchSignalTags(summary)
    const groups = tags.find((t) => t.label === 'Resource groups')
    expect(groups).toBeTruthy()
    expect(groups?.value).toContain('10')
    expect(tags.some((t) => t.label === 'Scope incomplete')).toBe(false)
  })

  it('flags incomplete aggregate scope so operators know stale was skipped', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      resource_group_count: 10,
      scope_incomplete: true,
      ces_total: 100,
      discovered_count: 90
    }
    const tags = buildSyncBatchSignalTags(summary)
    const incomplete = tags.find((t) => t.label === 'Scope incomplete')
    expect(incomplete).toBeTruthy()
    expect(incomplete?.value).toContain('不标记 stale')
  })

  it('omits group count tag for single group scope', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '生产ecs',
      resource_group_id: 'rg-001',
      resource_group_count: 1,
      ces_total: 5,
      discovered_count: 5
    }
    const tags = buildSyncBatchSignalTags(summary)
    expect(tags.some((t) => t.label === 'Resource groups')).toBe(false)
    expect(tags.some((t) => t.label === 'Scope incomplete')).toBe(false)
  })

  it('shows dimension group count tag when dimension groups covered', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      resource_group_count: 10,
      dimension_group_count: 4,
      ces_total: 1826,
      discovered_count: 1800
    }
    const tags = buildSyncBatchSignalTags(summary)
    const dim = tags.find((t) => t.label === 'Dimension groups')
    expect(dim).toBeTruthy()
    expect(dim?.value).toContain('4')
    expect(dim?.value).toContain('按资源实际维度发现')
  })

  it('omits dimension group tag when no dimension group covered', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '生产ecs',
      resource_group_id: 'rg-001',
      resource_group_count: 1,
      dimension_group_count: 0,
      ces_total: 5,
      discovered_count: 5
    }
    const tags = buildSyncBatchSignalTags(summary)
    expect(tags.some((t) => t.label === 'Dimension groups')).toBe(false)
  })

  it('flags dimension group coverage mismatch so operators know scope may be incomplete', () => {
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      resource_group_count: 10,
      dimension_group_count: 4,
      group_coverage_mismatches: ['大数据', '商用电站'],
      scope_incomplete: true,
      ces_total: 1826,
      discovered_count: 1800
    }
    const tags = buildSyncBatchSignalTags(summary)
    const mismatch = tags.find((t) => t.label === 'Coverage mismatch')
    expect(mismatch).toBeTruthy()
    expect(mismatch?.value).toContain('大数据')
    expect(mismatch?.value).toContain('商用电站')
    // 现有 Scope incomplete 标签语义不变。
    expect(tags.find((t) => t.label === 'Scope incomplete')?.value).toContain('不标记 stale')
  })

  it('drops dimension-skip hint when failed_scopes is empty', () => {
    // dimension 组不再因层级被跳过记 failed_scopes：无失败 scope 时不得出现 Failed scopes 标签。
    const summary: SyncBatchSummary = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      resource_group_count: 10,
      dimension_group_count: 4,
      failed_scopes: [],
      ces_total: 1826,
      discovered_count: 1800
    }
    const tags = buildSyncBatchSignalTags(summary)
    expect(tags.some((t) => t.label === 'Failed scopes')).toBe(false)
  })

  it('returns empty array without summary', () => {
    expect(buildSyncBatchSignalTags(undefined)).toEqual([])
  })
})

describe('toResourceSortParams', () => {
  it('maps Arco directions to server-side sort params', () => {
    expect(toResourceSortParams('name', 'ascend')).toEqual({ sort_by: 'name', sort_order: 'asc' })
    expect(toResourceSortParams('region', 'descend')).toEqual({ sort_by: 'region', sort_order: 'desc' })
  })

  it('drops empty direction and unknown fields', () => {
    expect(toResourceSortParams('name', '')).toEqual({})
    expect(toResourceSortParams('id; DROP TABLE', 'ascend')).toEqual({})
    expect(toResourceSortParams('labels', 'ascend')).toEqual({})
  })
})

describe('resource filter/sort URL state', () => {
  it('restores filters and sort from query', () => {
    expect(resourceFiltersFromQuery({
      keyword: 'elb',
      source: 'cloud_sync',
      cloud_resource_type: 'ecs',
      region: 'cn-north-4',
      sync_status: 'active'
    })).toEqual({
      keyword: 'elb',
      source: 'cloud_sync',
      cloud_resource_type: 'ecs',
      region: 'cn-north-4',
      sync_status: 'active'
    })

    expect(resourceSortFromQuery({ sort_by: 'name', sort_order: 'desc' })).toEqual({
      field: 'name',
      direction: 'descend'
    })
    expect(resourceSortFromQuery({ sort_by: 'region' })).toEqual({ field: 'region', direction: 'ascend' })
  })

  it('ignores malformed query values', () => {
    expect(resourceFiltersFromQuery({ keyword: 42, source: null })).toEqual({
      keyword: '',
      source: '',
      cloud_resource_type: '',
      region: '',
      sync_status: ''
    })
    expect(resourceSortFromQuery({ sort_by: 'labels', sort_order: 'desc' })).toEqual({ field: '', direction: '' })
    expect(resourceSortFromQuery({})).toEqual({ field: '', direction: '' })
  })

  it('writes state back without dropping unrelated query keys', () => {
    const query = withResourceQuery(
      { application_id: 'app-1', resource_id: 'res-1', keyword: 'old' },
      {
        keyword: ' elb ',
        source: '',
        cloud_resource_type: 'ecs',
        region: '',
        sync_status: ''
      },
      { field: 'name', direction: 'descend' }
    )
    expect(query).toEqual({
      application_id: 'app-1',
      resource_id: 'res-1',
      keyword: 'elb',
      cloud_resource_type: 'ecs',
      sort_by: 'name',
      sort_order: 'desc'
    })
  })

  it('removes filter keys when cleared', () => {
    const query = withResourceQuery(
      { application_id: 'app-1', keyword: 'elb', region: 'cn-north-4', sort_by: 'name', sort_order: 'asc' },
      { keyword: '', source: '', cloud_resource_type: '', region: '', sync_status: '' },
      { field: '', direction: '' }
    )
    expect(query).toEqual({ application_id: 'app-1' })
  })
})

describe('sanitizeVisibleColumns', () => {
  it('drops unknown keys, deduplicates and keeps definition order', () => {
    expect(sanitizeVisibleColumns(['pod', 'labels', 'pod', 'gone', 'created_at']))
      .toEqual(['labels', 'pod', 'created_at'])
  })

  it('tolerates corrupt localStorage values', () => {
    expect(sanitizeVisibleColumns(null)).toEqual([])
    expect(sanitizeVisibleColumns('labels')).toEqual([])
    expect(sanitizeVisibleColumns([{ key: 'labels' }])).toEqual([])
  })
})

describe('buildResourceCsv', () => {
  const row = {
    id: 'res-1',
    application_id: 'app-1',
    name: 'elb,prod"01',
    cloud_resource_type: 'elb',
    region: 'cn-south-1',
    labels: { env: 'prod' },
    created_at: 1700000000,
    updated_at: 1700000000
  } as Resource

  it('quotes and escapes commas, quotes and JSON labels', () => {
    expect(csvCell('a,b')).toBe('"a,b"')
    expect(csvCell('a"b')).toBe('"a""b"')
    expect(csvCell(undefined)).toBe('""')
    const lines = buildResourceCsv([row]).split('\r\n')
    expect(lines).toHaveLength(2)
    expect(lines[1]).toContain('"elb,prod""01"')
    expect(lines[1]).toContain('"{""env"":""prod""}"')
  })

  it('always emits a header even for an empty result', () => {
    expect(buildResourceCsv([]).split('\r\n')).toHaveLength(1)
    expect(buildResourceCsv([])).toContain('"资源 ID"')
  })
})
