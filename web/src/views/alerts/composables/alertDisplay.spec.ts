import { describe, expect, it } from 'vitest'
import { annotationLabel, environmentText, labelValueText, regionText, resourceTypeText, severityValueText } from './alertDisplay'

describe('alert display dictionary', () => {
  it('maps common environment values with the raw value retained', () => {
    expect(environmentText('prod')).toBe('生产环境（prod）')
    expect(environmentText('staging')).toBe('预发环境（staging）')
    expect(environmentText('custom-env')).toBe('custom-env')
    expect(environmentText('')).toBe('—')
  })

  it('maps source severity and cloud region without losing traceability', () => {
    expect(severityValueText('critical')).toBe('严重（critical）')
    expect(regionText('cn-north-4')).toBe('华北-北京四（cn-north-4）')
    expect(regionText('unknown-region')).toBe('unknown-region')
  })

  it('maps resource types and annotation keys', () => {
    expect(resourceTypeText('rds')).toBe('云数据库（RDS）（rds）')
    expect(resourceTypeText('pod')).toBe('Pod（pod）')
    expect(annotationLabel('summary')).toBe('规则摘要')
    expect(annotationLabel('description')).toBe('规则描述')
  })

  it('only maps values for their declared field semantics', () => {
    expect(labelValueText('severity', 'critical')).toBe('严重（critical）')
    expect(labelValueText('namespace', 'critical')).toBe('critical')
    expect(labelValueText('resource_type', 'ecs')).toBe('云服务器（ECS）（ecs）')
  })
})
