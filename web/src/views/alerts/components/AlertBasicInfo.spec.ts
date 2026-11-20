// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AlertBasicInfo from './AlertBasicInfo.vue'
import type { Alert } from '@/api/alert'

function makeAlert(overrides: Partial<Alert> = {}): Alert {
  return {
    id: 'alert-1', source: 'prometheus_alertmanager', fingerprint: 'fp-1', dedup_key: 'dk-1',
    name: 'HighCPU', severity: 'p0', status: 'processing', labels: {}, annotations: {},
    occurrence_count: 3, first_seen_at: 1, last_seen_at: 2, created_at: 1, updated_at: 2,
    ...overrides
  }
}

function render(alert: Alert) {
  return mount(AlertBasicInfo, {
    props: { alert },
    global: {
      stubs: {
        'a-descriptions': { template: '<div><slot /></div>' },
        'a-descriptions-item': { template: '<div><slot /></div>' },
        'a-tag': { template: '<span><slot /></span>' },
        'a-tooltip': { template: '<span><slot /></span>' },
        'a-link': { template: '<button><slot /></button>' },
        AlertCopyButton: { template: '<button />' },
        RouterLink: { template: '<a><slot /></a>' }
      }
    }
  })
}

beforeEach(() => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false,
    media: '',
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn()
  })))
})

describe('AlertBasicInfo', () => {
  it('shows friendly environment and explicit unmatched states', async () => {
    const wrapper = render(makeAlert({ environment: 'prod', resource_name: 'payment-pod-1' }))

    expect(wrapper.text()).toContain('生产环境（prod）')
    expect(wrapper.text()).toContain('未匹配应用')
    expect(wrapper.text()).toContain('未注册资源')
    const matchLink = wrapper.findAll('button, a').find((element) => element.text() === '检查匹配规则')
    expect(matchLink).toBeDefined()
    await matchLink!.trigger('click')
    expect(wrapper.emitted('check-match-rules')).toHaveLength(1)
  })

  it('keeps matched asset names and raw business IDs visible', () => {
    const wrapper = render(makeAlert({
      application_id: 'app-1', application_name: '支付服务',
      resource_id: 'res-1', resource_name: 'payment-db', resource_type: 'rds'
    }))

    expect(wrapper.text()).toContain('支付服务')
    expect(wrapper.text()).toContain('app-1')
    expect(wrapper.text()).toContain('payment-db')
    expect(wrapper.text()).toContain('res-1')
    expect(wrapper.text()).toContain('云数据库（RDS）（rds）')
  })
})
