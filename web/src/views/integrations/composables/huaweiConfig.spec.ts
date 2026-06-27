// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import {
  HUAWEI_KNOWN_EXTRA_KEYS,
  extractUnknownExtraConfig,
  formatRegionProjects,
  mergeHuaweiExtraConfig,
  parseRegionProjects,
  parseRegions
} from './huaweiConfig'

describe('parseRegions', () => {
  it('splits by comma, whitespace and Chinese comma', () => {
    expect(parseRegions('cn-north-4,cn-south-1')).toEqual(['cn-north-4', 'cn-south-1'])
    expect(parseRegions('cn-north-4 cn-south-1')).toEqual(['cn-north-4', 'cn-south-1'])
    expect(parseRegions('cn-north-4，cn-south-1')).toEqual(['cn-north-4', 'cn-south-1'])
    expect(parseRegions('cn-north-4, cn-south-1 , cn-east-3')).toEqual([
      'cn-north-4',
      'cn-south-1',
      'cn-east-3'
    ])
  })

  it('returns empty array for blank input', () => {
    expect(parseRegions('')).toEqual([])
    expect(parseRegions('  ')).toEqual([])
  })
})

describe('parseRegionProjects', () => {
  it('parses valid multi-line mapping', () => {
    const { items, errors } = parseRegionProjects('cn-north-4=proj-a\ncn-south-1=proj-b')
    expect(items).toEqual([
      { region: 'cn-north-4', project_id: 'proj-a' },
      { region: 'cn-south-1', project_id: 'proj-b' }
    ])
    expect(errors).toEqual([])
  })

  it('allows project_id to contain = characters', () => {
    const { items, errors } = parseRegionProjects('cn-north-4=a=b=c')
    expect(items).toEqual([{ region: 'cn-north-4', project_id: 'a=b=c' }])
    expect(errors).toEqual([])
  })

  it('reports format error for missing project_id', () => {
    const { items, errors } = parseRegionProjects('cn-north-4=')
    expect(items).toEqual([])
    expect(errors).toHaveLength(1)
    expect(errors[0]).toContain('第 1 行格式错误')
  })

  it('reports duplicate region error (case-insensitive)', () => {
    const { items, errors } = parseRegionProjects('cn-north-4=proj-a\nCN-NORTH-4=proj-b')
    expect(items).toEqual([{ region: 'cn-north-4', project_id: 'proj-a' }])
    expect(errors).toHaveLength(1)
    expect(errors[0]).toContain('region 重复')
  })

  it('skips blank lines without error', () => {
    const { items, errors } = parseRegionProjects('\ncn-north-4=proj-a\n\n')
    expect(items).toEqual([{ region: 'cn-north-4', project_id: 'proj-a' }])
    expect(errors).toEqual([])
  })
})

describe('formatRegionProjects', () => {
  it('formats items back to region=project_id lines', () => {
    expect(
      formatRegionProjects([
        { region: 'cn-north-4', project_id: 'proj-a' },
        { region: 'cn-south-1', project_id: 'proj-b' }
      ])
    ).toBe('cn-north-4=proj-a\ncn-south-1=proj-b')
  })

  it('returns empty string for nil/empty', () => {
    expect(formatRegionProjects(undefined)).toBe('')
    expect(formatRegionProjects([])).toBe('')
  })

  it('roundtrips with parseRegionProjects', () => {
    const text = 'cn-north-4=proj-a\ncn-south-1=proj-b'
    expect(formatRegionProjects(parseRegionProjects(text).items)).toBe(text)
  })
})

describe('extractUnknownExtraConfig', () => {
  it('strips known keys and retains unknown keys', () => {
    const config = {
      sync_mode: 'ces',
      resource_group_name: '全部资源',
      max_resources: 20000,
      custom_tag: 'team-foo',
      owner_email: 'ops@example.com'
    }
    const preserved = extractUnknownExtraConfig(config)
    expect(preserved).toEqual({ custom_tag: 'team-foo', owner_email: 'ops@example.com' })
    expect(preserved).not.toHaveProperty('sync_mode')
    expect(preserved).not.toHaveProperty('max_resources')
  })

  it('returns empty object when only known keys present', () => {
    const preserved = extractUnknownExtraConfig({ sync_mode: 'hybrid', max_resources: 10 })
    expect(preserved).toEqual({})
  })

  it('HUAWEI_KNOWN_EXTRA_KEYS covers all form fields', () => {
    expect(HUAWEI_KNOWN_EXTRA_KEYS).toEqual([
      'sync_mode',
      'resource_group_name',
      'resource_group_id',
      'enterprise_project_id',
      'max_resources',
      'region_projects'
    ])
  })
})

describe('mergeHuaweiExtraConfig', () => {
  const baseFields = {
    sync_mode: 'ces' as const,
    resource_group_name: '全部资源',
    resource_group_id: '',
    enterprise_project_id: '',
    max_resources: 20000,
    region_projects: undefined
  }

  it('retains preserved unknown keys alongside known form fields', () => {
    const config = mergeHuaweiExtraConfig({ custom_tag: 'team-foo' }, baseFields)
    expect(config.custom_tag).toBe('team-foo')
    expect(config.sync_mode).toBe('ces')
    expect(config.resource_group_name).toBe('全部资源')
    expect(config.max_resources).toBe(20000)
  })

  it('trims whitespace on string fields', () => {
    const config = mergeHuaweiExtraConfig({}, {
      ...baseFields,
      resource_group_name: '  全部资源  ',
      resource_group_id: '  rg001  '
    })
    expect(config.resource_group_name).toBe('全部资源')
    expect(config.resource_group_id).toBe('rg001')
  })

  it('omits empty string fields (does not write blank values)', () => {
    const config = mergeHuaweiExtraConfig({}, {
      ...baseFields,
      resource_group_name: '   ',
      resource_group_id: ''
    })
    expect(config.resource_group_name).toBeUndefined()
    expect(config.resource_group_id).toBeUndefined()
  })

  it('skips non-positive max_resources', () => {
    const config = mergeHuaweiExtraConfig({}, { ...baseFields, max_resources: 0 })
    expect(config.max_resources).toBeUndefined()
  })

  it('includes region_projects when provided non-empty', () => {
    const config = mergeHuaweiExtraConfig({}, {
      ...baseFields,
      region_projects: [{ region: 'cn-north-4', project_id: 'proj-a' }]
    })
    expect(config.region_projects).toEqual([{ region: 'cn-north-4', project_id: 'proj-a' }])
  })

  it('omits region_projects when empty', () => {
    const config = mergeHuaweiExtraConfig({}, { ...baseFields, region_projects: [] })
    expect(config.region_projects).toBeUndefined()
  })

  it('preserved unknown keys survive even when form omits them', () => {
    const config = mergeHuaweiExtraConfig({ owner_email: 'ops@example.com', custom: 42 }, baseFields)
    expect(config.owner_email).toBe('ops@example.com')
    expect(config.custom).toBe(42)
  })
})
