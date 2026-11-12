import { describe, expect, it } from 'vitest'
import {
  buildAlertPresentation,
  columns,
  COLLAPSE_THRESHOLD,
  eventTypeLabel,
  KNOWN_LABEL_KEYS,
  parseCondition,
  severityOptions,
  statusOptions,
  type EnhancementAlert
} from './alertPresentation'

function makeAlert(
  labels: Record<string, string> = {},
  annotations: Record<string, string> = {},
  source = 'huawei_ces'
): EnhancementAlert {
  return { labels, annotations, source }
}

function successAlert(): EnhancementAlert {
  return makeAlert(
    {
      cloud_resource_id: 'res-001',
      resource_type: 'ecs',
      region: 'cn-north-4',
      project_id: 'proj-001',
      account_id: 'acc-001',
      instance_id: 'i-001',
      ces_namespace: 'SYS.ECS',
      rule_name: 'cpu-high',
      alarm_id: 'alarm-001',
      record_id: 'rec-001',
      metric_name: 'cpu_util'
    },
    {
      ces_namespace: 'SYS.ECS',
      ces_level: '2',
      ces_rule_description: 'CPU 使用率连续超标',
      ces_rule_type: 'MULTI_INSTANCE',
      ces_metric_name: 'cpu_util',
      ces_condition: '> 90% (average, 300s x3)'
    }
  )
}

describe('parseCondition', () => {
  it('parses the AlarmCondition.Describe() format completely', () => {
    const parsed = parseCondition('> 90% (average, 300s x3)')
    expect(parsed.ok).toBe(true)
    expect(parsed.op).toBe('>')
    expect(parsed.opText).toBe('大于')
    expect(parsed.threshold).toBe('90')
    expect(parsed.unit).toBe('%')
    expect(parsed.filter).toBe('average')
    expect(parsed.period).toBe('300')
    expect(parsed.count).toBe('3')
    expect(parsed.windowText).toBe('平均值 · 300 秒 · 连续 3 次')
  })

  it('omits the window text entirely when there is no bracket clause', () => {
    const parsed = parseCondition('> 90%')
    expect(parsed.ok).toBe(true)
    expect(parsed.op).toBe('>')
    expect(parsed.threshold).toBe('90')
    expect(parsed.unit).toBe('%')
    expect(parsed.windowText).toBe('')
  })

  it('maps every operator and filter word and only joins present fragments', () => {
    expect(parseCondition('<= -3.5 (max, 60s x1)')).toMatchObject({
      ok: true,
      op: '<=',
      opText: '小于等于',
      threshold: '-3.5',
      windowText: '最大值 · 60 秒 · 连续 1 次'
    })
    expect(parseCondition('!= 0 (variance)').windowText).toBe('方差')
    expect(parseCondition('>= 1 (min)').windowText).toBe('最小值')
    expect(parseCondition('= 5 (sum, 120s)').windowText).toBe('求和 · 120 秒')
    expect(parseCondition('> 1 (unknown_filter)').windowText).toBe('unknown_filter')
  })

  it.each(['>= (custom)', 'weird', '', '90% (average, 300s x3)', '> NaN%', '> 90% (average']) (
    'degrades the whole expression for %j without partial parsing',
    (raw) => {
      const parsed = parseCondition(raw)
      expect(parsed.ok).toBe(false)
      expect(parsed.raw).toBe(raw)
      expect(parsed.threshold).toBe('')
      expect(parsed.windowText).toBe('')
      expect(`${parsed.op}${parsed.threshold}${parsed.unit}`).not.toContain('undefined')
      expect(`${parsed.op}${parsed.threshold}${parsed.unit}`).not.toContain('NaN')
    }
  )
})

describe('buildAlertPresentation: region visibility and CES detection', () => {
  it('silently omits all three sections when labels and annotations are both empty', () => {
    const vm = buildAlertPresentation(makeAlert())
    expect(vm.showRegion).toBe(false)
    expect(vm.showConditionCard).toBe(false)
    expect(vm.showLabelsPanel).toBe(false)
    expect(vm.showAnnotations).toBe(false)
    expect(vm.labels).toEqual([])
  })

  it.each([
    ['source contains ces', makeAlert({ x: '1' }, {}, 'huawei_CES')],
    ['annotations have ces_ prefix', makeAlert({}, { ces_level: '1' }, 'webhook')],
    ['legacy record_id', makeAlert({ record_id: 'r' }, {}, 'webhook')],
    ['legacy alarm_id', makeAlert({ alarm_id: 'a' }, {}, 'webhook')],
    ['legacy ces_namespace', makeAlert({ ces_namespace: 'SYS.ECS' }, {}, 'webhook')]
  ])('detects CES via %s', (_name, alert) => {
    expect(buildAlertPresentation(alert).isCES).toBe(true)
  })

  it('never renders the condition card for non-CES but still shows labels', () => {
    const vm = buildAlertPresentation(makeAlert({ instance_id: 'i-1', future_key: 'v' }, {}, 'webhook'))
    expect(vm.isCES).toBe(false)
    expect(vm.showConditionCard).toBe(false)
    expect(vm.showLabelsPanel).toBe(true)
    expect(vm.labels.map((item) => item.key)).toContain('future_key')
  })

  it('omits section ③ entirely when annotations are empty', () => {
    const vm = buildAlertPresentation(makeAlert({ instance_id: 'i-1' }, {}))
    expect(vm.showLabelsPanel).toBe(true)
    expect(vm.showAnnotations).toBe(false)
  })
})

describe('buildAlertPresentation: dedup and label ordering', () => {
  it('consumes metric name / condition / namespace exactly once', () => {
    const vm = buildAlertPresentation(successAlert())
    expect(vm.condition.metricName).toBe('cpu_util')
    expect(vm.labels.some((item) => item.key === 'metric_name')).toBe(false)
    expect(vm.labels.filter((item) => item.key === 'ces_namespace')).toHaveLength(1)
    expect(vm.annotationKVs.some((kv) => kv.key === 'ces_namespace')).toBe(false)
    expect(vm.annotationKVs.some((kv) => kv.key === 'ces_metric_name')).toBe(false)
    expect(vm.annotationKVs.some((kv) => kv.key === 'ces_condition')).toBe(false)
    expect(vm.annotationKVs.some((kv) => kv.key === 'ces_rule_lookup')).toBe(false)
  })

  it('keeps ces_namespace in the labels grid even when only annotations carry it', () => {
    const vm = buildAlertPresentation(makeAlert({}, { ces_namespace: 'SYS.ECS' }))
    expect(vm.labels.find((item) => item.key === 'ces_namespace')?.value).toBe('SYS.ECS')
    expect(vm.annotationKVs).toHaveLength(0)
  })

  it('places the first five keys in a constant order and sorts dimension keys', () => {
    const vm = buildAlertPresentation(
      makeAlert({
        record_id: 'r',
        alarm_id: 'a',
        rule_name: 'rule',
        ces_namespace: 'SYS.ECS',
        account_id: 'acc',
        project_id: 'proj',
        region: 'cn',
        resource_type: 'ecs',
        cloud_resource_id: 'cr',
        instance_id: 'i1',
        namespace: 'ns',
        bucket_name: 'b',
        vpc_id: 'v',
        pod: 'p',
        disk_name: 'd'
      })
    )
    expect(vm.labelCount).toBe(15)
    expect(vm.labels.slice(0, 5).map((item) => item.key)).toEqual([
      'cloud_resource_id', 'resource_type', 'region', 'project_id', 'account_id'
    ])
    expect(vm.labels.slice(5, 11).map((item) => item.key)).toEqual([
      'bucket_name', 'disk_name', 'instance_id', 'namespace', 'pod', 'vpc_id'
    ])
    expect(vm.labels.slice(11).map((item) => item.key)).toEqual([
      'ces_namespace', 'rule_name', 'alarm_id', 'record_id'
    ])
  })

  it('collapses only above the 12-key threshold', () => {
    const keys = [
      'cloud_resource_id', 'resource_type', 'region', 'project_id', 'account_id',
      'bucket_name', 'disk_name', 'instance_id', 'namespace', 'pod', 'vpc_id', 'node_id'
    ]
    const labels = Object.fromEntries(keys.map((key) => [key, 'v']))
    const vm = buildAlertPresentation(makeAlert(labels))
    expect(vm.labelCount).toBe(COLLAPSE_THRESHOLD)
    expect(vm.needsCollapse).toBe(false)

    const more = buildAlertPresentation(makeAlert({ ...labels, cluster_id: 'v' }))
    expect(more.labelCount).toBe(13)
    expect(more.needsCollapse).toBe(true)
  })

  it('never uses the whitelist as a display filter and never splits values', () => {
    const vm = buildAlertPresentation(makeAlert({ future_dim: 'a,b,c' }))
    expect(KNOWN_LABEL_KEYS.has('future_dim')).toBe(false)
    expect(vm.labels[0].key).toBe('future_dim')
    expect(vm.labels[0].label).toBe('future_dim')
    expect(vm.labels[0].value).toBe('a,b,c')
    expect(vm.labels[0].hint).toBe('· 维度')
    expect(vm.labels[0].copyable).toBe(true)
  })

  it('maps composite dimension keys segment by segment and preserves the raw value', () => {
    const vm = buildAlertPresentation(makeAlert({ 'lbaas_instance_id,loadbalancer_id': 'id-a,id-b' }))
    expect(vm.labels[0].label).toBe('LB 实例 ID / 负载均衡器 ID')
    expect(vm.labels[0].hint).toBe('· 维度')
    expect(vm.labels[0].value).toBe('id-a,id-b')
  })

  it('masks sensitive values and does not offer a copy control', () => {
    const vm = buildAlertPresentation(makeAlert({ api_key: 'super-secret-plain' }))
    expect(vm.labels[0].value).toBe('****')
    expect(vm.labels[0].sensitive).toBe(true)
    expect(vm.labels[0].copyable).toBe(false)
  })

  it('masks sensitive annotation keys too', () => {
    const vm = buildAlertPresentation(makeAlert({}, { access_token: 'plain' }))
    expect(vm.annotationKVs[0].value).toBe('****')
    expect(vm.annotationKVs[0].hint).toBe('· 注解')
  })

  it('masks only by key name: business IDs stay plaintext and values are never scanned', () => {
    const vm = buildAlertPresentation(
      makeAlert(
        {
          account_id: 'acc-plain',
          region: 'cn-north-4',
          project_id: 'proj-plain',
          record_id: 'rec-plain',
          cloud_resource_id: 'res-plain',
          api_key: 'must-not-leak'
        },
        { note: 'this value contains a secret word' }
      )
    )
    for (const key of ['account_id', 'region', 'project_id', 'record_id', 'cloud_resource_id']) {
      const item = vm.labels.find((label) => label.key === key)
      expect(item?.value).toBeTruthy()
      expect(item?.value).not.toBe('****')
      expect(item?.sensitive).toBe(false)
    }
    expect(vm.labels.find((label) => label.key === 'api_key')?.value).toBe('****')
    expect(vm.annotationKVs.find((kv) => kv.key === 'note')?.value).toBe(
      'this value contains a secret word'
    )
  })

  it('adds the product-level dimension-only note', () => {
    expect(buildAlertPresentation(makeAlert({ dim_name: 'instance_id' })).dimOnlyNote).toBe(true)
    expect(
      buildAlertPresentation(makeAlert({ dim_name: 'instance_id', instance_id: 'i-1' })).dimOnlyNote
    ).toBe(false)
  })
})

describe('buildAlertPresentation: condition states', () => {
  it('state A keeps the full conclusion and no foot', () => {
    const condition = buildAlertPresentation(successAlert()).condition
    expect(condition.state).toBe('A')
    expect(condition.showConclusion).toBe(true)
    expect(condition.parsed.ok).toBe(true)
    expect(condition.foot).toBe('')
    expect(condition.ariaLabel).toContain('大于')
    expect(condition.ariaLabel).toContain('90%')
    expect(condition.ariaLabel).toContain('平均值')
  })

  it('state A-raw keeps the original condition verbatim', () => {
    const condition = buildAlertPresentation(
      makeAlert({ metric_name: 'cpu_util' }, { ces_condition: '>= (custom)' })
    ).condition
    expect(condition.state).toBe('A-raw')
    expect(condition.rawText).toBe('>= (custom)')
    expect(condition.showConclusion).toBe(false)
    expect(condition.foot).toBe('条件按云端原文展示。')
  })

  it('state B (event rule) has no threshold and the exact foot text', () => {
    const condition = buildAlertPresentation(
      makeAlert({}, { ces_rule_type: 'EVENT_SYS', ces_level: '1' })
    ).condition
    expect(condition.state).toBe('B')
    expect(condition.headLabel).toBe('事件触发')
    expect(condition.showMetric).toBe(false)
    expect(condition.showConclusion).toBe(false)
    expect(condition.foot).toBe('事件类规则按云端事件触发，不含指标与阈值条件。')
  })

  it.each(['not_found', 'error'])(
    'state D (%s) never parses and never shows a conclusion row',
    (lookup) => {
      const vm = buildAlertPresentation(
        makeAlert(
          { metric_name: 'cpu_util' },
          {
            ces_rule_lookup: lookup,
            ces_condition: '> 90% (average, 300s x3)',
            ces_rule_description: 'CPU 规则 · ecs-001'
          }
        )
      )
      expect(vm.condition.state).toBe('D')
      expect(vm.condition.headLabel).toBe('指标信息未获取')
      expect(vm.condition.showConclusion).toBe(false)
      expect(vm.condition.parsed.ok).toBe(false)
      expect(vm.condition.rawText).toBe('')
      expect(vm.condition.rawFoot).toBe('> 90% (average, 300s x3)')
      expect(vm.condition.foot).toBe('未获取到触发条件表达式。')
      expect(vm.condition.showMetric).toBe(true)
      expect(vm.mutedNote).toBe(true)
      expect(vm.lookupValue).toBe(lookup)
      expect(vm.showAnnotations).toBe(true)
      expect(vm.description).toBe('CPU 规则 · ecs-001')
    }
  )

  it('state C covers missing fields and partial fields with distinct foot text', () => {
    expect(buildAlertPresentation(makeAlert({ record_id: 'r' })).condition.foot).toBe(
      '未收到触发条件表达式，已按可用字段展示。'
    )
    expect(buildAlertPresentation(makeAlert({ metric_name: 'cpu_util' })).condition.foot).toBe(
      '部分规则字段未返回，已按可用信息展示。'
    )
  })
})

describe('buildAlertPresentation: annotation presentation', () => {
  it('maps rule type and keeps unknown values dim/raw', () => {
    expect(buildAlertPresentation(makeAlert({}, { ces_rule_type: 'MULTI_INSTANCE' }))).toMatchObject({
      ruleTypeText: '多实例规则',
      ruleTypeDim: false
    })
    expect(buildAlertPresentation(makeAlert({}, { ces_rule_type: 'EVENT_CUSTOM' }))).toMatchObject({
      ruleTypeText: '事件规则',
      ruleTypeDim: false
    })
    expect(buildAlertPresentation(makeAlert({}, { ces_rule_type: 'SINGLE_INSTANCE' }))).toMatchObject({
      ruleTypeText: 'SINGLE_INSTANCE',
      ruleTypeDim: true
    })
  })

  it('renders the cloud level neutrally and handles unknown levels', () => {
    const known = buildAlertPresentation(makeAlert({}, { ces_level: '2' }))
    expect(known.levelCode).toBe('2')
    expect(known.levelSuffix).toBe('（云端：重要）· 平台定级以基础信息「级别」为准')
    expect(buildAlertPresentation(makeAlert({}, { ces_level: '9' })).levelSuffix).toContain(
      '（未在默认映射内）'
    )
    expect(buildAlertPresentation(makeAlert({})).levelCode).toBe('')
  })
})

describe('migrated view constants (unchanged behaviour)', () => {
  it('keeps the same option / column / event label contracts', () => {
    expect(statusOptions).toHaveLength(6)
    expect(statusOptions[0]).toEqual({ label: '新建', value: 'new' })
    expect(severityOptions.map((option) => option.value)).toEqual(['p0', 'p1', 'p2', 'p3', 'info'])
    expect(columns[0]).toMatchObject({ title: '告警名称', dataIndex: 'name', ellipsis: true })
    expect(columns[5].align).toBe('right')
    expect(eventTypeLabel('triggered')).toBe('首次触发')
    expect(eventTypeLabel('execution_finished')).toBe('执行完成')
    expect(eventTypeLabel('brand_new_event')).toBe('brand_new_event')
  })
})
