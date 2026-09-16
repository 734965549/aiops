// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ObservabilityView from './index.vue'
import { listIntegrationAccounts } from '@/api/integration'
import { queryMetrics } from '@/api/observability'

vi.mock('@/api/integration', () => ({
  listIntegrationAccounts: vi.fn()
}))

vi.mock('@/api/observability', () => ({
  queryMetrics: vi.fn(),
  queryTopology: vi.fn(),
  queryTraces: vi.fn(),
  searchLogs: vi.fn()
}))

vi.mock('@/api/request', () => ({
  getApiError: vi.fn(() => null)
}))

vi.mock('@arco-design/web-vue', () => {
  const stub = { template: '<div><slot /></div>' }
  return {
    Alert: stub,
    Button: { emits: ['click'], template: '<button @click="$emit(\'click\')"><slot /></button>' },
    Card: stub,
    Col: stub,
    Empty: stub,
    Form: stub,
    FormItem: stub,
    Input: stub,
    InputNumber: stub,
    Option: stub,
    RangePicker: stub,
    Row: stub,
    Select: stub,
    Switch: stub,
    TabPane: stub,
    Table: stub,
    Tabs: stub,
    Message: {
      error: vi.fn(),
      warning: vi.fn()
    }
  }
})

const passthrough = { template: '<div><slot /></div>' }
const buttonStub = { emits: ['click'], template: '<button @click="$emit(\'click\')"><slot /></button>' }

function mountView() {
  return mount(ObservabilityView, {
    global: {
      stubs: {
        'a-alert': passthrough,
        'a-button': buttonStub,
        'a-card': passthrough,
        'a-col': passthrough,
        'a-empty': passthrough,
        'a-form': passthrough,
        'a-form-item': passthrough,
        'a-input': passthrough,
        'a-input-number': passthrough,
        'a-option': passthrough,
        'a-range-picker': passthrough,
        'a-row': passthrough,
        'a-select': passthrough,
        'a-switch': passthrough,
        'a-tab-pane': passthrough,
        'a-table': passthrough,
        'a-tabs': passthrough
      }
    }
  })
}

describe('observability metrics query', () => {
  beforeEach(() => {
    vi.mocked(listIntegrationAccounts).mockResolvedValue({
      items: [
        {
          account_id: 'acc-huawei-real',
          name: 'Huawei Real',
          provider: 'huawei_cloud',
          auth_type: 'ak_sk',
          regions: ['cn-north-4'],
          project_id: 'project-1',
          has_credential: true,
          enabled: true,
          capabilities: ['metrics'],
          created_at: 1710000000,
          updated_at: 1710000000
        }
      ],
      total: 1,
      page: 1,
      page_size: 100
    })
    vi.mocked(queryMetrics).mockResolvedValue({
      evidence_id: 'ev-test',
      series: [{ metric: 'cpu_util', unit: '%', labels: {}, points: [] }]
    })
  })

  it('sends huawei CES required metric parameters from the page', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button').trigger('click')

    expect(queryMetrics).toHaveBeenCalledWith({
      account_id: 'acc-huawei-real',
      region: 'cn-north-4',
      namespace: 'SYS.ECS',
      metric: 'cpu_util',
      dimensions: { instance_id: 'ecs-xxx' },
      from: expect.any(Number),
      to: expect.any(Number),
      period: 60,
      aggregator: 'avg'
    })
  })
})
