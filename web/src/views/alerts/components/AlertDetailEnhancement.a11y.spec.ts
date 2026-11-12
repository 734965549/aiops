// @vitest-environment jsdom
// 本文件刻意不 mock @arco-design/web-vue：用真实 Arco Alert + 真实 Transition
// 验证生产环境下的弱化容器确实是 type="info" 且不带 role="alert" live region。
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
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

function renderReal(alert: EnhancementAlert) {
  return mount(AlertDetailEnhancement, {
    props: { alert },
    global: { stubs: { transition: false } }
  })
}

describe('real Arco a-alert type="info" in production markup', () => {
  it('strips the hardcoded role="alert" and never renders warning/error', () => {
    const wrapper = renderReal({
      labels: { dim_name: 'instance_id' },
      annotations: {},
      source: 'huawei_ces'
    })
    const html = wrapper.html()

    expect(html).toContain('arco-alert-info')
    expect(html).not.toContain('arco-alert-warning')
    expect(html).not.toContain('arco-alert-error')
    expect(html).not.toContain('type="error"')
    expect(html).not.toContain('type="warning"')
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0)
    expect(wrapper.findAll('[role="status"]')).toHaveLength(0)
    expect(wrapper.findAll('[aria-live]')).toHaveLength(0)
    expect(wrapper.find('.alert-condition-foot').text()).toContain('未收到触发条件表达式')
  })

  it('does the same for the enrichment-failure state', () => {
    const wrapper = renderReal({
      labels: { metric_name: 'cpu_util' },
      annotations: { ces_rule_lookup: 'not_found', ces_condition: '> 90% (average, 300s x3)' },
      source: 'huawei_ces'
    })
    const html = wrapper.find('.alert-enhancement').html()
    expect(html).toContain('arco-alert-info')
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(0)
    expect(wrapper.findAll('[aria-live]')).toHaveLength(0)
    expect(html).not.toContain('type="warning"')
    expect(html).not.toContain('type="error"')
    expect(wrapper.text()).toContain('告警数据完整')
  })
})
