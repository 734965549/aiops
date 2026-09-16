// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SyncBatchDetailDrawer from './SyncBatchDetailDrawer.vue'

const passthrough = { template: '<div><slot /></div>' }
const tagStub = { template: '<span><slot /></span>' }

beforeEach(() => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })))
})

function mountDrawer() {
  return mount(SyncBatchDetailDrawer, {
    props: {
      visible: true,
      batchDetail: {
        batch_id: 'batch-001',
        integration_account_id: 'acc-001',
        provider: 'huawei_cloud',
        status: 'success',
        created_count: 1,
        updated_count: 2,
        completed_count: 3,
        stale_count: 0,
        failed_count: 0,
        triggered_by: 'tester',
        message: 'sync_mode=native resource_group_name=全部资源 resource_group_id=rg-001 config_mode_fallback=true resource_group_selection=fallback',
        summary: {
          sync_mode: 'native',
          resource_group_name: '全部资源',
          resource_group_id: 'rg-001',
          config_mode_fallback: true,
          resource_group_selection: 'fallback',
          ces_total: 5,
          discovered_count: 5,
          completed_count: 5,
          scopes: []
        },
        started_at: 1710000000,
        created_at: 1710000000,
        updated_at: 1710000100
      },
      loading: false,
      messageSummary: {
        sync_mode: 'native',
        resource_group_name: '全部资源',
        resource_group_id: 'rg-001',
        config_mode_fallback: 'true',
        resource_group_selection: 'fallback',
        ces_total: '5',
        discovered: '5',
        completed: '5',
        failed_scopes: '0'
      },
      scopeCards: [],
      signalTags: [],
      selectedScopeKey: '',
      selectedScopeTrace: null,
      selectedScopeTraceCards: [],
      selectedScopeTraceTags: [],
      selectedScopeTraceSnippet: '',
      selectedScopeSignalKey: '',
      scopeDiagnosticColumns: []
    },
    global: {
      stubs: {
        ClientOnly: passthrough,
        Teleport: passthrough,
        'a-alert': passthrough,
        'a-button': { template: '<button><slot /></button>' },
        'a-card': passthrough,
        'a-col': passthrough,
        'a-descriptions': passthrough,
        'a-descriptions-item': passthrough,
        'a-drawer': passthrough,
        'a-empty': passthrough,
        'a-row': passthrough,
        'a-spin': passthrough,
        'a-table': passthrough,
        'a-tag': tagStub,
        'a-tooltip': passthrough,
        IconQuestionCircle: passthrough
      }
    }
  })
}

describe('SyncBatchDetailDrawer', () => {
  it('shows native fallback metadata in the header area', () => {
    const wrapper = mountDrawer()
    const text = wrapper.text()
    expect(text).toContain('同步模式')
    expect(text).toContain('资源组')
    expect(text).toContain('模式回退true')
    expect(text).toContain('资源组使用回退值')
  })
})
