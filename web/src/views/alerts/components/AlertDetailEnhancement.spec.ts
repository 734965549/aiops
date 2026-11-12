// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AlertDetailEnhancement from './AlertDetailEnhancement.vue'
import type { EnhancementAlert } from '../composables/alertPresentation'

vi.mock('@arco-design/web-vue/es/message', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn()
  }
}))
// unplugin-vue-components 把 <a-alert> 编译成 @arco-design/web-vue 的直接 import。
vi.mock('@arco-design/web-vue', () => ({
  Alert: { template: '<div><slot /></div>' }
}))

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

function render(alert: EnhancementAlert) {
  return mount(AlertDetailEnhancement, { props: { alert } })
}

function isBefore(el: Element, a: string, b: string) {
  const first = el.querySelector(a)
  const second = el.querySelector(b)
  expect(first, `missing ${a}`).not.toBeNull()
  expect(second, `missing ${b}`).not.toBeNull()
  return !!(first!.compareDocumentPosition(second!) & Node.DOCUMENT_POSITION_FOLLOWING)
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn()
  })))
})

describe('success state (A)', () => {
  it('renders the split conclusion row with one accessible sentence', () => {
    const wrapper = render(successAlert())
    expect(wrapper.find('.alert-condition-card').exists()).toBe(true)
    expect(wrapper.find('.cond-op').text()).toBe('>')
    expect(wrapper.find('.cond-threshold').text()).toContain('90')
    expect(wrapper.find('.cond-threshold').text()).toContain('%')
    expect(wrapper.find('.cond-window').text()).toContain('平均值')
    expect(wrapper.find('.cond-window').text()).toContain('300')
    expect(wrapper.find('.cond-window').text()).toContain('3')

    const body = wrapper.find('.alert-condition-body')
    expect(body.attributes('aria-label')).toContain('大于')
    expect(body.attributes('aria-label')).toContain('90%')
    expect(body.attributes('aria-label')).toContain('平均值')
    body.findAll('span').forEach((span) => expect(span.attributes('aria-hidden')).toBe('true'))
  })

  it('shows the metric name and ces_namespace exactly once in the whole region', () => {
    const text = render(successAlert()).text()
    expect(text.split('cpu_util').length - 1).toBe(1)
    expect(text.split('SYS.ECS').length - 1).toBe(1)
    expect(text).toContain('云端原始级别')
    expect(text).toContain('（云端：重要）')
    expect(text).toContain('平台定级以基础信息「级别」为准')
    expect(wrapperHasTag(render(successAlert()), 'a-tag')).toBe(false)
  })

  it('orders the three sections and keeps their distinct shapes', () => {
    const wrapper = render(successAlert())
    const el = wrapper.element as Element
    expect(isBefore(el, '.alert-condition-card', '.alert-labels-panel')).toBe(true)
    expect(isBefore(el, '.alert-labels-panel', '.alert-anno')).toBe(true)
    expect(wrapper.findAll('section[aria-labelledby]')).toHaveLength(3)
    expect(wrapper.find('#alert-condition-title').exists()).toBe(true)
    expect(wrapper.find('#alert-labels-title').exists()).toBe(true)
    expect(wrapper.find('#alert-anno-title').exists()).toBe(true)
  })
})

function wrapperHasTag(wrapper: ReturnType<typeof render>, tag: string) {
  return wrapper.html().includes(`<${tag}`)
}

describe('degraded states', () => {
  it('event rule keeps the card but has no threshold and no undefined/NaN', () => {
    const wrapper = render(makeAlert({}, { ces_rule_type: 'EVENT_SYS', ces_level: '1' }))
    expect(wrapper.find('.alert-condition-card').exists()).toBe(true)
    expect(wrapper.find('.cond-threshold').exists()).toBe(false)
    expect(wrapper.find('.alert-condition-body').exists()).toBe(false)
    expect(wrapper.find('.alert-condition-foot').text()).toBe(
      '事件类规则按云端事件触发，不含指标与阈值条件。'
    )
    expect(wrapper.text()).not.toContain('undefined')
    expect(wrapper.text()).not.toContain('NaN')
  })

  it.each(['not_found', 'error'])('enrichment failure %s is neutral and Complete', (lookup) => {
    const wrapper = render(
      makeAlert(
        { metric_name: 'cpu_util' },
        {
          ces_rule_lookup: lookup,
          ces_condition: '> 90% (average, 300s x3)',
          ces_rule_description: 'CPU 规则 · ecs-资源-001',
          ces_level: '3'
        }
      )
    )
    expect(wrapper.find('.alert-condition-card').exists()).toBe(true)
    expect(wrapper.find('.cond-threshold').exists()).toBe(false)
    expect(wrapper.find('.alert-condition-body').exists()).toBe(false)
    expect(wrapper.find('.alert-condition-foot').text()).toContain('未获取到触发条件表达式。')
    expect(wrapper.find('.alert-condition-foot .cond-raw').text()).toBe('> 90% (average, 300s x3)')
    expect(wrapper.text()).toContain('告警数据完整')
    expect(wrapper.text()).toContain('以下为已收到的原始字段。')
    expect(wrapper.find('.alert-muted-code').text()).toBe(lookup)
    expect(wrapper.find('.alert-anno-desc').text()).toContain('ecs-资源-001')

    const html = wrapper.html()
    expect(html).not.toContain('type="error"')
    expect(html).not.toContain('status="danger"')
    expect(html).not.toMatch(/color:\s*(red|#f|rgb\(2\d\d)/i)
    expect(wrapper.text()).not.toContain('加载失败')
    expect(wrapper.text()).not.toContain('数据错误')
    expect(wrapper.text()).not.toContain('ces_rule_lookup')
    expect(wrapper.text()).not.toContain('规则详情未获取（富化状态：undefined')
  })

  it.each(['>= (custom)', 'weird', '', '90% (average, 300s x3)'])(
    'condition %j degrades to a verbatim raw code block',
    (raw) => {
      const wrapper = render(makeAlert({ metric_name: 'cpu_util' }, { ces_condition: raw }))
      expect(wrapper.find('.cond-raw').text()).toBe(raw)
      expect(wrapper.find('.alert-condition-foot').text()).toBe('条件按云端原文展示。')
      expect(wrapper.find('.cond-threshold').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('undefined')
      expect(wrapper.text()).not.toContain('NaN')
    }
  )

  it('omits the window span when the condition has no bracket clause', () => {
    const wrapper = render(makeAlert({ metric_name: 'cpu_util' }, { ces_condition: '> 90%' }))
    expect(wrapper.find('.cond-op').text()).toBe('>')
    expect(wrapper.find('.cond-threshold').text()).toBe('90%')
    expect(wrapper.find('.cond-window').exists()).toBe(false)
    expect(wrapper.find('.alert-condition-foot').exists()).toBe(false)
  })

  it('field-missing state keeps the card and the dimension-only note', () => {
    const wrapper = render(makeAlert({ dim_name: 'instance_id' }, {}))
    expect(wrapper.find('.alert-condition-card').exists()).toBe(true)
    expect(wrapper.text()).toContain('维度名')
    expect(wrapper.text()).toContain('该规则关联多个资源，云端未指定具体实例，仅提供维度名。')
    expect(wrapper.find('.alert-condition-foot').text()).toBe('未收到触发条件表达式，已按可用字段展示。')
    expect(wrapper.find('.alert-anno').exists()).toBe(false)
  })
})

describe('empty and partial data', () => {
  it('renders nothing (no empty / no "暂无数据") when both maps are empty', () => {
    const wrapper = render(makeAlert())
    expect(wrapper.find('.alert-enhancement').exists()).toBe(false)
    expect(wrapper.findAll('.alert-section')).toHaveLength(0)
    expect(wrapper.html()).not.toContain('a-empty')
    expect(wrapper.text()).not.toContain('暂无数据')
  })

  it('omits section ③ when annotations are empty', () => {
    const wrapper = render(makeAlert({ instance_id: 'i-1' }, {}))
    expect(wrapper.find('.alert-labels-panel').exists()).toBe(true)
    expect(wrapper.find('.alert-anno').exists()).toBe(false)
    expect(wrapper.find('.alert-subtitle').exists()).toBe(false)
  })

  it('non-CES source never renders the condition card but still shows labels', () => {
    const wrapper = render(makeAlert({ instance_id: 'i-1' }, {}, 'webhook'))
    expect(wrapper.find('.alert-condition-card').exists()).toBe(false)
    expect(wrapper.find('.alert-labels-panel').exists()).toBe(true)
    expect(wrapper.find('[data-key="instance_id"]').exists()).toBe(true)
  })
})

describe('long IDs, unknown keys and masking', () => {
  it('renders an 80-char ID fully without truncation', () => {
    const longId = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890-' + '0123456789abcdef'.repeat(4)
    const wrapper = render(makeAlert({ cloud_resource_id: longId }))
    expect(longId.length).toBeGreaterThanOrEqual(80)
    expect(wrapper.text()).toContain(longId)
    expect(wrapper.html()).not.toContain('…')
    expect(wrapper.find('[data-key="cloud_resource_id"] .is-mono').exists()).toBe(true)
  })

  it('maps composite keys, keeps raw values and never drops unknown keys', () => {
    const wrapper = render(makeAlert({ 'lbaas_instance_id,loadbalancer_id': 'id-a,id-b', future_dim: 'v' }))
    expect(wrapper.text()).toContain('LB 实例 ID / 负载均衡器 ID')
    expect(wrapper.text()).toContain('· 维度')
    expect(wrapper.text()).toContain('id-a,id-b')
    expect(wrapper.find('[data-key="future_dim"]').exists()).toBe(true)
    expect(wrapper.find('[data-key="future_dim"]').text()).toContain('future_dim')
  })

  it('masks sensitive keys', () => {
    const wrapper = render(makeAlert({ api_key: 'plain-secret' }, {}))
    expect(wrapper.text()).toContain('****')
    expect(wrapper.text()).not.toContain('plain-secret')
    expect(wrapper.find('[data-key="api_key"] .alert-copy-btn').exists()).toBe(false)
  })
})

describe('collapse and ARIA', () => {
  const manyLabels: Record<string, string> = {
    cloud_resource_id: 'cr', resource_type: 'ecs', region: 'cn', project_id: 'p', account_id: 'a',
    instance_id: 'i1', namespace: 'ns', bucket_name: 'b', vpc_id: 'v', pod: 'p',
    disk_name: 'd', cluster_id: 'c', node_id: 'n', publicip_id: 'pub', bandwidth_id: 'bw'
  }

  it('collapses 15 keys to 8 and toggles with aria-expanded / aria-controls', async () => {
    const wrapper = render(makeAlert(manyLabels))
    expect(wrapper.findAll('.alert-kv')).toHaveLength(8)
    const button = wrapper.find('.alert-labels-more')
    expect(button.text()).toContain('展开全部（15）')
    expect(button.attributes('aria-expanded')).toBe('false')
    const gridId = button.attributes('aria-controls')
    expect(gridId).toBeTruthy()
    expect(wrapper.find(`#${gridId}`).exists()).toBe(true)
    expect(wrapper.find('dl.alert-labels-grid').exists()).toBe(true)
    expect(wrapper.find('dt.alert-kv-label').exists()).toBe(true)
    expect(wrapper.find('dd.alert-kv-value').exists()).toBe(true)
    expect(wrapper.findAll('.alert-kv').slice(0, 5).map((item) => item.attributes('data-key'))).toEqual([
      'cloud_resource_id', 'resource_type', 'region', 'project_id', 'account_id'
    ])

    await button.trigger('click')
    expect(wrapper.findAll('.alert-kv')).toHaveLength(15)
    expect(wrapper.find('.alert-labels-more').text()).toContain('收起')
    expect(wrapper.find('.alert-labels-more').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.alert-labels-more').attributes('aria-controls')).toBe(gridId)
  })

  it('shows no toggle at exactly 12 keys', () => {
    const keys = Object.keys(manyLabels).slice(0, 12)
    const labels = Object.fromEntries(keys.map((key) => [key, 'v']))
    const wrapper = render(makeAlert(labels))
    expect(wrapper.findAll('.alert-kv')).toHaveLength(12)
    expect(wrapper.find('.alert-labels-more').exists()).toBe(false)
  })

  it('returns focus to the trigger button when collapsing from inside the grid', async () => {
    // attachTo 让 jsdom 的 focus() 真正更新 document.activeElement。
    const wrapper = mount(AlertDetailEnhancement, {
      props: { alert: makeAlert(manyLabels) },
      attachTo: document.body
    })
    await wrapper.find('.alert-labels-more').trigger('click')
    const copyBtn = wrapper.find('dl.alert-labels-grid').findAll('.alert-copy-btn')[0]
    const target = copyBtn.element as HTMLElement
    target.focus()
    expect(document.activeElement).toBe(target)
    await wrapper.find('.alert-labels-more').trigger('click')
    expect(document.activeElement).toBe(wrapper.find('.alert-labels-more').element)
    wrapper.unmount()
  })
})

describe('copy controls', () => {
  it('copies the original full value via the clipboard API', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    })
    const longId = 'id-with,commas-and-a-very-long-value-0123456789'
    const wrapper = render(makeAlert({ cloud_resource_id: longId }))
    const button = wrapper.find('.alert-copy-btn')
    expect(button.attributes('aria-label')).toContain('复制')
    await button.trigger('click')
    expect(writeText).toHaveBeenCalledWith(longId)
  })

  it('falls back to a selectable full value without clipboard support', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true
    })
    const wrapper = render(makeAlert({ cloud_resource_id: 'raw-id-001' }))
    await wrapper.find('.alert-copy-btn').trigger('click')
    expect(wrapper.find('.alert-copy-fallback').text()).toBe('raw-id-001')
  })
})

describe('supplemental rulings: neutral degradation containers', () => {
  const degraded: Array<[string, EnhancementAlert]> = [
    ['enrichment failure not_found', makeAlert(
      { metric_name: 'cpu_util' },
      {
        ces_rule_lookup: 'not_found',
        ces_condition: '> 90% (average, 300s x3)',
        ces_rule_description: 'CPU 规则 · ecs-001',
        ces_rule_type: 'MULTI_INSTANCE',
        ces_level: '2'
      }
    )],
    ['enrichment failure error', makeAlert({}, { ces_rule_lookup: 'error' })],
    ['event rule', makeAlert({}, { ces_rule_type: 'EVENT_SYS', ces_level: '1' })],
    ['field missing', makeAlert({ dim_name: 'instance_id' }, {})],
    ['raw condition', makeAlert({ metric_name: 'cpu_util' }, { ces_condition: 'weird' })]
  ]

  // 48 / 50：降级态只用 info 弱化容器，禁止 warning/error 与任何 live region。
  it.each(degraded)('%s uses an info-level container with no warning/error and no live role', (_name, alert) => {
    const wrapper = render(alert)
    const html = wrapper.find('.alert-enhancement').html()
    expect(html).not.toContain('arco-alert-warning')
    expect(html).not.toContain('type="warning"')
    expect(html).not.toContain('type="error"')
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0)
    expect(wrapper.findAll('[role="status"]')).toHaveLength(0)
    expect(wrapper.findAll('[aria-live]')).toHaveLength(0)
    const foot = wrapper.find('.alert-condition-foot')
    expect(foot.exists()).toBe(true)
    expect(foot.attributes('type')).toBe('info')
    expect(foot.attributes('show-icon')).toBe('false')
  })

  it('48/50 also applies to the ③ block-level muted note', () => {
    const wrapper = render(degraded[0][1])
    const note = wrapper.find('.alert-muted-note')
    expect(note.exists()).toBe(true)
    expect(note.element.tagName).toBe('P')
    expect(note.attributes('role')).toBeUndefined()
    expect(note.attributes('aria-live')).toBeUndefined()
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0)
    expect(wrapper.findAll('[aria-live]')).toHaveLength(0)
  })

  // 51：「—」/「N/A」占位仅限新增三区块；head 语义说明文案保留。
  it('51. no em dash / N/A placeholders in the three new blocks', () => {
    const cases: EnhancementAlert[] = [...degraded.map(([, alert]) => alert), successAlert()]
    cases.forEach((alert) => {
      const text = render(alert).find('.alert-enhancement').text()
      expect(text).not.toContain('—')
      expect(text).not.toContain('N/A')
    })
    expect(render(degraded[2][1]).text()).toContain('事件触发')
    expect(render(degraded[3][1]).text()).toContain('触发条件')
    expect(render(degraded[0][1]).text()).toContain('指标信息未获取')
  })

  // 52 + B1 + B2：规则类型全页一次、无新 Tag、云端级别中性。
  it('52/B1/B2. rule type once, no tags, ces_level stays neutral', () => {
    const region = render(successAlert()).find('.alert-enhancement')
    const text = region.text()
    expect(text.split('多实例规则').length - 1).toBe(1)
    expect(text).not.toContain('MULTI_INSTANCE')
    expect(text).not.toContain('ces_rule_type')
    expect(region.html()).not.toContain('<a-tag')
    expect(region.html()).not.toMatch(/\scolor="(?:red|orange|gold|green|purple|arcoblue)/)
    expect(region.findAll('.alert-anno-meta')).toHaveLength(2)
    expect(region.find('.alert-muted-code').text()).toBe('2')
    expect(text).toContain('云端原始级别')
    expect(text).toContain('平台定级以基础信息「级别」为准')

    const eventRegion = render(degraded[2][1]).find('.alert-enhancement')
    expect(eventRegion.text().split('事件规则').length - 1).toBe(1)
    expect(eventRegion.html()).not.toContain('<a-tag')
  })

  it('event rule head is semantic copy, not a placeholder or a tag', () => {
    const wrapper = render(degraded[2][1])
    expect(wrapper.find('.alert-condition-head').text()).toBe('事件触发')
    expect(wrapper.find('.alert-condition-card').exists()).toBe(true)
    expect(wrapper.find('.alert-enhancement').html()).not.toContain('<a-tag')
  })

  // D1：不挂 tooltip / title，长值靠等宽完整换行 + 复制控件。
  it('D1. no tooltip or title fallback; long values stay visible and copyable', () => {
    const longId = 'a1b2c3d4-e5f6-7890-abcd-ef1234567890-' + '0123456789abcdef'.repeat(4)
    const wrapper = render(makeAlert({ cloud_resource_id: longId }))
    const html = wrapper.find('.alert-enhancement').html()
    expect(html).not.toContain('a-tooltip')
    expect(html).not.toMatch(/\stitle=/)
    expect(wrapper.text()).toContain(longId)
    expect(wrapper.find('[data-key="cloud_resource_id"] .alert-copy-btn').exists()).toBe(true)
    expect(wrapper.find('[data-key="cloud_resource_id"] .is-mono').exists()).toBe(true)
  })
})

describe('static SCSS contract', () => {
  const files = ['./AlertDetailEnhancement.vue', './AlertCopyButton.vue']
  const scss = files
    .map((name) => {
      const source = readFileSync(fileURLToPath(new URL(name, import.meta.url)), 'utf-8')
      return /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? ''
    })
    .join('\n')

  it('has no hex colours, shadows, gradients or ellipsis truncation', () => {
    expect(scss).not.toMatch(/#[0-9a-fA-F]{3,8}\b/)
    expect(scss).not.toContain('box-shadow')
    expect(scss).not.toContain('linear-gradient')
    expect(scss).not.toMatch(/text-overflow\s*:\s*ellipsis/)
    expect(scss).not.toContain('outline: none')
  })

  it('uses only text-1 / text-2 for text colours (threshold uses primary-6)', () => {
    const declarations = [...scss.matchAll(/(?:^|[;{\s])color\s*:\s*([^;}]+)/gm)].map((m) => m[1].trim())
    expect(declarations.length).toBeGreaterThan(5)
    expect(declarations.filter((value) => value.includes('--color-text-3'))).toHaveLength(0)
    expect(declarations.filter((value) => value.includes('--color-text-4'))).toHaveLength(0)
    declarations.forEach((value) => {
      expect(/--color-text-1|--color-text-2|--primary-6/.test(value)).toBe(true)
    })
    expect(scss).toMatch(/\.cond-threshold\s*\{[^}]*color:\s*rgb\(var\(--primary-6\)\)/s)
  })

  // 49：a-alert type="info" 与 .alert-muted-note 的文字色都必须是 text-2。
  it('49. info containers explicitly override the Arco default to --color-text-2', () => {
    expect(scss).not.toContain('--color-text-3')
    expect(scss).toMatch(/\.alert-condition-foot\s*\{[^}]*color:\s*var\(--color-text-2\)/s)
    expect(scss).toMatch(/\.alert-muted-note\s*\{[^}]*color:\s*var\(--color-text-2\)/s)
    expect(scss).toMatch(/:deep\(\.arco-alert-content\)\s*\{[^}]*color:\s*var\(--color-text-2\)/s)
  })

  // 把 @media 块与基础作用域分离，用于「40px 仅存在于 hover:none」的双向断言。
  function splitMedia(source: string) {
    const media: string[] = []
    let base = ''
    const re = /@media[^{]*\{/g
    let last = 0
    let match: RegExpExecArray | null
    while ((match = re.exec(source)) !== null) {
      base += source.slice(last, match.index)
      let depth = 1
      let cursor = match.index + match[0].length
      while (cursor < source.length && depth > 0) {
        if (source[cursor] === '{') depth += 1
        else if (source[cursor] === '}') depth -= 1
        cursor += 1
      }
      media.push(source.slice(match.index, cursor))
      last = cursor
      re.lastIndex = cursor
    }
    base += source.slice(last)
    return { base, media }
  }

  const { base, media } = splitMedia(scss)
  const hoverNone = media
    .filter((block) => /@media\s*\(hover:\s*none\)/.test(block))
    .join('\n')

  it('single-column grid replaces the two-column auto-fit layout', () => {
    expect(scss).toMatch(/\.alert-labels-grid\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/s)
    expect(scss).not.toMatch(/repeat\(auto-fit,\s*minmax\(280px,\s*1fr\)\)/)
    expect(scss).not.toMatch(/gap:\s*8px\s+16px/)
    expect(scss).toMatch(/outline:\s*2px solid rgb\(var\(--primary-6\)\)/)
  })

  it('40px touch target exists only inside @media (hover:none)', () => {
    expect(base).not.toMatch(/min-height:\s*40px/)
    expect(base).not.toMatch(/min-width:\s*40px/)
    expect(base).toMatch(/min-height:\s*24px/)
    expect(hoverNone).toMatch(/min-width:\s*40px/)
    expect(hoverNone).toMatch(/min-height:\s*40px/)
  })

  it('keeps all three responsive media blocks', () => {
    expect(media.some((block) => /@media\s*\(max-width:\s*900px\)/.test(block))).toBe(true)
    expect(media.some((block) => /@media\s*\(max-width:\s*480px\)/.test(block))).toBe(true)
    expect(hoverNone.length).toBeGreaterThan(0)
  })

  it('28px row grid and the 2x dim-key variant', () => {
    expect(scss).toMatch(/--alert-row-h:\s*28px/)
    expect(scss).toMatch(/calc\(var\(--alert-row-h\)\s*\*\s*2\)/)
    expect(scss).toMatch(/\.alert-kv\.is-dim-key\s*\{[^}]*calc\(var\(--alert-row-h\)/s)
    expect(hoverNone).toMatch(/min-height:\s*44px/)
    expect(hoverNone).toMatch(/min-height:\s*88px/)
  })

  it('96px label column via $label-col token and separated grid rows', () => {
    expect(scss).toMatch(/\$label-col:\s*96px/)
    const kvRule = /\.alert-kv\s*\{([^}]*)\}/s.exec(scss)?.[1] ?? ''
    expect(kvRule).toMatch(/display:\s*grid/)
    expect(kvRule).toMatch(/grid-template-columns:\s*\$label-col\s+minmax\(0,\s*1fr\)/)
    expect(kvRule).toMatch(/column-gap:\s*12px/)
    expect(kvRule).toMatch(/border-bottom:\s*1px solid var\(--color-border-2\)/)
    expect(scss).toMatch(/\.alert-kv:last-child\s*\{[^}]*border-bottom:\s*0/s)
  })

  it('removes the legacy typography that caused ragged row heights', () => {
    expect(scss).not.toContain('display:contents')
    expect(scss).not.toMatch(/flex:\s*0\s+0\s+128px/)
    expect(scss).not.toMatch(/font-size:\s*15px/)
    expect(scss).not.toMatch(/line-height:\s*22px/)
    expect(scss).not.toMatch(/flex-direction:\s*column/)
  })

  it('is-mono carries no font-size; dd reserves the copy button gutter', () => {
    const monoRule = /(?:^|\n)\.is-mono\s*\{([^}]*)\}/s.exec(scss)?.[1] ?? ''
    expect(monoRule).not.toMatch(/font-size/)
    const ddRule = /(?:^|\n)\.alert-kv-value\s*\{([^}]*)\}/s.exec(scss)?.[1] ?? ''
    expect(ddRule).toMatch(/position:\s*relative/)
    expect(ddRule).toMatch(/padding-right:\s*28px/)
    expect(scss).toMatch(/:deep\(\.alert-copy-wrap\)\s*\{[^}]*position:\s*absolute/s)
    expect(scss).toMatch(/:deep\(\.alert-copy-wrap\.is-fallback\)\s*\{[^}]*position:\s*static/s)
  })

  it('.cond-op is 13px/600 and the condition row uses 28px line-height', () => {
    const opRule = /\.cond-op\s*\{([^}]*)\}/s.exec(scss)?.[1] ?? ''
    expect(opRule).toMatch(/font-size:\s*13px/)
    expect(opRule).toMatch(/font-weight:\s*600/)
    expect(scss).toMatch(/\.alert-condition-body\s*\{[^}]*line-height:\s*28px/s)
  })

  it('text-4 only as a decorative background, never as a text colour', () => {
    expect(scss).not.toMatch(/color:\s*var\(--color-text-4\)/)
    expect(scss).toMatch(/background:\s*var\(--color-text-4\)/)
    expect(scss).not.toContain('--color-text-3')
  })
})

describe('Phase 3 typography DOM contract', () => {
  it('A1. mapped dimension keys expose Chinese name + raw key + · 维度', () => {
    const wrapper = render(makeAlert({
      'lbaas_instance_id,loadbalancer_id': 'id-a,id-b',
      future_dim: 'v'
    }))
    const composite = wrapper.find('[data-key="lbaas_instance_id,loadbalancer_id"]')
    expect(composite.classes()).toContain('is-dim-key')
    const dt = composite.find('dt')
    expect(dt.text()).toContain('LB 实例 ID / 负载均衡器 ID')
    expect(dt.text()).toContain('lbaas_instance_id,loadbalancer_id')
    expect(dt.text()).toContain('· 维度')
    expect(dt.find('code.alert-kv-hint.is-mono').exists()).toBe(true)
    expect(composite.find('dd').text()).toContain('id-a,id-b')

    const unknown = wrapper.find('[data-key="future_dim"]')
    expect(unknown.find('dt').text()).toContain('future_dim')
    expect(unknown.find('dt code.alert-kv-hint.is-mono').exists()).toBe(false)
    expect(unknown.classes()).toContain('is-dim-key')
  })

  it('A1. known mapped keys show the raw key once without is-dim-key', () => {
    const wrapper = render(makeAlert({ instance_id: 'i-001' }))
    const row = wrapper.find('[data-key="instance_id"]')
    expect(row.classes()).not.toContain('is-dim-key')
    expect(row.find('dt').text()).toContain('实例 ID')
    expect(row.find('dt code.alert-kv-hint.is-mono').text()).toBe('instance_id')
    expect(row.find('dt .alert-kv-hint:not(.is-mono)').exists()).toBe(false)
  })

  it('fallback keeps the complete value inside a static (non-absolute) wrapper', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true
    })
    const wrapper = render(makeAlert({ cloud_resource_id: 'raw-id-001' }))
    await wrapper.find('.alert-copy-btn').trigger('click')
    expect(wrapper.find('.alert-copy-wrap').classes()).toContain('is-fallback')
    const fallback = wrapper.find('.alert-copy-fallback')
    expect(fallback.exists()).toBe(true)
    expect(fallback.text()).toBe('raw-id-001')
    expect(wrapper.find('dd').text()).toContain('raw-id-001')
  })
})
