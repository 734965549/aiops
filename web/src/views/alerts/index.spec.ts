// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AlertsView from './index.vue'
import type { Alert, AlertDetail } from '@/api/alert'
import {
  getAlert,
  listAlerts,
  listAlertSources
} from '@/api/alert'
import { getApiError } from '@/api/request'

vi.mock('@/api/alert', () => ({
  listAlerts: vi.fn(),
  listAlertSources: vi.fn(),
  getAlert: vi.fn(),
  acknowledgeAlert: vi.fn(),
  startProcessingAlert: vi.fn(),
  recoverAlert: vi.fn(),
  closeAlert: vi.fn(),
  silenceAlert: vi.fn(),
  unsilenceAlert: vi.fn(),
  assignAlert: vi.fn(),
  commentAlert: vi.fn(),
  requestAlertAIAnalysis: vi.fn()
}))
vi.mock('@/api/ai', () => ({ analyzeAlert: vi.fn() }))
vi.mock('@/api/execution', () => ({ createExecutionTask: vi.fn() }))
vi.mock('@/api/runbook', () => ({ listRunbookRecommendations: vi.fn() }))
vi.mock('@/api/request', () => ({
  getApiError: vi.fn(() => null),
  http: vi.fn(),
  isApiHttpError: vi.fn(() => false)
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: vi.fn(() => ({ user: { id: 'user-1' }, token: '', refreshToken: '' }))
}))
vi.mock('vue-router', () => ({
  useRouter: vi.fn(() => ({ push: vi.fn(), currentRoute: { value: {} } }))
}))
vi.mock('./components/AlertSourceManager.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@arco-design/web-vue/es/message', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn()
  }
}))
// unplugin-vue-components 把 <a-*> 编译成 @arco-design/web-vue 的直接 import，
// 因此 VTU 的按名 stub 不生效：把自定义 Table / Drawer stub 放进模块 mock。
vi.mock('@arco-design/web-vue', () => {
  const stub = { template: '<div><slot /></div>' }
  const tableStub = {
    emits: ['row-click'],
    template:
      '<div class="table-root">' +
      '<button class="row-open" @click="$emit(\'row-click\', { id: \'alert-1\' })">open</button>' +
      '</div>'
  }
  const drawerStub = {
    props: ['title', 'width'],
    emits: ['cancel'],
    template:
      '<div class="drawer-root" :data-width="String(width)">' +
      '<header class="drawer-title">{{ title }}</header>' +
      '<button class="drawer-cancel" @click="$emit(\'cancel\')">close</button>' +
      '<slot /></div>'
  }
  return {
    Alert: stub,
    Button: { template: '<button><slot /></button>' },
    Card: stub,
    Checkbox: stub,
    Descriptions: stub,
    DescriptionsItem: stub,
    Divider: stub,
    Drawer: drawerStub,
    Empty: stub,
    Form: stub,
    FormItem: stub,
    Input: stub,
    InputNumber: stub,
    Link: stub,
    Modal: stub,
    Radio: stub,
    RadioGroup: stub,
    Select: stub,
    Space: stub,
    Spin: stub,
    Switch: stub,
    Table: tableStub,
    Tag: { template: '<span><slot /></span>' },
    Textarea: stub,
    Timeline: stub,
    TimelineItem: stub,
    Tooltip: stub,
    Message: { error: vi.fn(), warning: vi.fn(), success: vi.fn() }
  }
})

function mountView() {
  return mount(AlertsView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        AlertSourceManager: { template: '<div />' }
      }
    }
  })
}

function makeAlert(overrides: Partial<Alert> = {}): Alert {
  return {
    id: 'alert-1',
    source: 'huawei_ces',
    fingerprint: '',
    dedup_key: '',
    name: 'CPU 告警',
    summary: 'ecs-资源-001',
    description: 'SYS.ECS cpu_util > 90% (average, 300s x3) · ecs-资源-001',
    severity: 'p1',
    status: 'new',
    labels: {},
    annotations: {},
    occurrence_count: 1,
    first_seen_at: 0,
    last_seen_at: 0,
    created_at: 0,
    updated_at: 0,
    ...overrides
  }
}

function makeDetail(overrides: Partial<Alert> = {}): AlertDetail {
  return { alert: makeAlert(overrides), events: [], related: {} }
}

const richLabels: Record<string, string> = {
  cloud_resource_id: 'res-001',
  resource_type: 'ecs',
  region: 'cn-north-4',
  project_id: 'proj-001',
  account_id: 'acc-001',
  instance_id: 'i-001',
  ces_namespace: 'SYS.ECS',
  rule_name: 'cpu-high',
  record_id: 'rec-001',
  metric_name: 'cpu_util'
}

const richAnnotations: Record<string, string> = {
  ces_namespace: 'SYS.ECS',
  ces_level: '2',
  ces_rule_description: 'CPU 使用率连续超标',
  ces_rule_type: 'MULTI_INSTANCE',
  ces_metric_name: 'cpu_util',
  ces_condition: '> 90% (average, 300s x3)'
}

function stubMatchMedia(matches: boolean) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches,
    media: '(max-width: 900px)',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn()
  })))
}

function isBefore(el: Element, a: string, b: string) {
  const first = el.querySelector(a)
  const second = el.querySelector(b)
  expect(first, `missing ${a}`).not.toBeNull()
  expect(second, `missing ${b}`).not.toBeNull()
  return !!(first!.compareDocumentPosition(second!) & Node.DOCUMENT_POSITION_FOLLOWING)
}

async function openRow(wrapper: VueWrapper) {
  await wrapper.find('.row-open').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  stubMatchMedia(false)
  vi.mocked(listAlerts).mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
  vi.mocked(listAlertSources).mockResolvedValue({ items: [] })
  vi.mocked(getApiError).mockReturnValue(null)
  vi.mocked(getAlert).mockResolvedValue(makeDetail())
})

describe('alert detail: block order and baseline regions', () => {
  it('renders ① → ② → ③ → action-bar after the basic descriptions', async () => {
    vi.mocked(getAlert).mockResolvedValue(
      makeDetail({ labels: richLabels, annotations: richAnnotations })
    )
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    const el = wrapper.element as Element

    expect(isBefore(el, '.detail-desc', '.alert-condition-card')).toBe(true)
    expect(isBefore(el, '.alert-condition-card', '.alert-labels-panel')).toBe(true)
    expect(isBefore(el, '.alert-labels-panel', '.alert-anno')).toBe(true)
    expect(isBefore(el, '.alert-anno', '.action-bar')).toBe(true)
    expect(isBefore(el, '.action-bar', '.timeline-card')).toBe(true)
    expect(el.querySelector('.detail-desc')).not.toBeNull()
    expect(el.querySelector('.timeline-card')).not.toBeNull()
  })

  it('keeps basic info / action-bar / timeline when there is no enhancement data', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    const el = wrapper.element as Element

    expect(wrapper.find('.alert-enhancement').exists()).toBe(false)
    expect(wrapper.findAll('.alert-section')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('暂无数据')
    expect(el.querySelector('.detail-desc')).not.toBeNull()
    expect(el.querySelector('.action-bar')).not.toBeNull()
    expect(el.querySelector('.timeline-card')).not.toBeNull()
  })
})

describe('alert detail: request failure state', () => {
  it('shows an error alert with retry inside the drawer instead of a blank spin', async () => {
    vi.mocked(getAlert).mockRejectedValue(new Error('boom'))
    vi.mocked(getApiError).mockReturnValue({ status: 500, message: '服务异常' } as never)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)

    expect(wrapper.find('.detail-error').exists()).toBe(true)
    expect(wrapper.find('.detail-error').attributes('type')).toBe('error')
    expect(wrapper.find('.detail-error').text()).toContain('服务异常')
    expect(wrapper.find('.detail-retry').exists()).toBe(true)
    expect((wrapper.element as Element).querySelector('.detail-desc')).toBeNull()
    expect((wrapper.element as Element).querySelector('.drawer-root')).not.toBeNull()
    expect((wrapper.element as Element).querySelector('.drawer-title')?.textContent).toContain('告警详情')
  })

  it('uses the 403 wording, keeps the drawer shell and still offers retry', async () => {
    vi.mocked(getAlert).mockRejectedValue(new Error('forbidden'))
    vi.mocked(getApiError).mockReturnValue({ status: 403, message: '权限不足' } as never)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)

    expect(wrapper.find('.detail-error').text()).toContain('无权限查看该告警详情')
    expect(wrapper.find('.detail-retry').exists()).toBe(true)
    expect((wrapper.element as Element).querySelector('.drawer-root')).not.toBeNull()
    expect((wrapper.element as Element).querySelector('.drawer-title')?.textContent).toContain('告警详情')
  })

  it('falls back to a generic message when getApiError returns null', async () => {
    vi.mocked(getAlert).mockRejectedValue(new Error('network'))
    vi.mocked(getApiError).mockReturnValue(null)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    expect(wrapper.find('.detail-error').text()).toContain('加载告警详情失败')
  })

  it('retry re-calls getAlert with selectedAlertId and recovers the detail', async () => {
    vi.mocked(getAlert).mockRejectedValueOnce(new Error('boom'))
    vi.mocked(getApiError).mockReturnValue({ status: 500, message: '服务异常' } as never)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    expect(wrapper.find('.detail-error').exists()).toBe(true)

    await wrapper.find('.detail-retry').trigger('click')
    await flushPromises()

    expect(getAlert).toHaveBeenCalledTimes(2)
    expect(getAlert).toHaveBeenNthCalledWith(2, 'alert-1')
    expect(wrapper.find('.detail-error').exists()).toBe(false)
    expect((wrapper.element as Element).querySelector('.detail-desc')).not.toBeNull()
  })

  it('closeDetail clears the error so reopening does not leak stale state', async () => {
    vi.mocked(getAlert).mockRejectedValueOnce(new Error('boom'))
    vi.mocked(getApiError).mockReturnValue({ status: 500, message: '服务异常' } as never)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    expect(wrapper.find('.detail-error').exists()).toBe(true)

    await wrapper.find('.drawer-cancel').trigger('click')
    expect(wrapper.find('.detail-error').exists()).toBe(false)

    await openRow(wrapper)
    expect(wrapper.find('.detail-error').exists()).toBe(false)
    expect((wrapper.element as Element).querySelector('.detail-desc')).not.toBeNull()
  })
})

describe('alert detail: error colour mutual-exclusion regression', () => {
  it('allows error colour for a failed request', async () => {
    vi.mocked(getAlert).mockRejectedValue(new Error('boom'))
    vi.mocked(getApiError).mockReturnValue({ status: 500, message: '服务异常' } as never)
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)
    expect(wrapper.find('.detail-error[type="error"]').exists()).toBe(true)
  })

  it('never uses error colour for a field-level enrichment downgrade', async () => {
    vi.mocked(getAlert).mockResolvedValue(
      makeDetail({
        labels: { metric_name: 'cpu_util' },
        annotations: {
          ces_rule_lookup: 'not_found',
          ces_condition: '> 90% (average, 300s x3)',
          ces_rule_description: 'CPU 规则 · ecs-资源-001'
        }
      })
    )
    const wrapper = mountView()
    await flushPromises()
    await openRow(wrapper)

    const enhancement = wrapper.find('.alert-enhancement')
    expect(enhancement.exists()).toBe(true)
    expect(enhancement.text()).toContain('告警数据完整')
    expect(enhancement.find('.alert-muted-code').text()).toBe('not_found')
    expect(enhancement.html()).not.toContain('type="error"')
    expect(enhancement.html()).not.toContain('type="warning"')
    expect(enhancement.html()).not.toContain('arco-alert-warning')
    expect(enhancement.html()).not.toContain('status="danger"')
    expect(enhancement.findAll('[role="alert"]')).toHaveLength(0)
    expect(enhancement.findAll('[role="status"]')).toHaveLength(0)
    expect(enhancement.findAll('[aria-live]')).toHaveLength(0)
    expect(enhancement.html()).not.toMatch(/color:\s*(red|#f|rgb\(2\d\d)/i)
    // 48/50：区块级弱化容器是 info，且不带 live role
    expect(enhancement.find('.alert-condition-foot').attributes('type')).toBe('info')
    expect(enhancement.text()).not.toContain('加载失败')
    expect(enhancement.text()).not.toContain('数据错误')
    // 51/52：新增三区块无占位符、规则类型仅一次、无新 Tag
    expect(enhancement.text()).not.toContain('—')
    expect(enhancement.text()).not.toContain('N/A')
    expect(enhancement.html()).not.toContain('<a-tag')
    expect(enhancement.text()).not.toContain('ces_rule_type')
    // 基础信息区既有 '—' 占位保持原样（作用域仅限新增三区块）
    expect(wrapper.find('.detail-desc').text()).toContain('—')
    // 基础信息区状态/级别 Tag 与资源名不受降级态影响
    expect(wrapper.find('.detail-desc').text()).toContain('新建')
    expect(wrapper.find('.detail-desc').text()).toContain('P1')
    expect(wrapper.find('.detail-desc').text()).toContain('ecs-资源-001')
  })
})

describe('alert detail: responsive drawer width', () => {
  it('uses 720px on wide viewports and falls back safely without matchMedia', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('.drawer-root').attributes('data-width')).toBe('720')
  })

  it('switches to 100% on narrow viewports', async () => {
    stubMatchMedia(true)
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('.drawer-root').attributes('data-width')).toBe('100%')
  })

  it('does not throw when matchMedia is unavailable (SSR/jsdom defensive path)', async () => {
    vi.stubGlobal('matchMedia', undefined)
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('.drawer-root').attributes('data-width')).toBe('720')
  })
})
