// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Resource } from '@/api/asset'
import { clearAssetAIContext, syncAssetAIContext } from './assetAIContext'
import { useAIContextStore } from '@/stores/aiContext'

function resource(overrides: Partial<Resource> = {}): Resource {
  return {
    id: 'i-1',
    application_id: 'app-1',
    name: 'ecs-prod-01',
    resource_type: 'host',
    cloud_resource_type: 'ecs',
    created_at: 1,
    updated_at: 1,
    ...overrides
  }
}

describe('syncAssetAIContext', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('prefers cloud resource type over registry type', () => {
    syncAssetAIContext({ applicationId: 'app-1', applicationName: 'payment', resource: resource() })
    const context = useAIContextStore().current
    expect(context.page).toBe('assets')
    expect(context.application_id).toBe('app-1')
    expect(context.resource_id).toBe('i-1')
    expect(context.resource_type).toBe('ecs')
    expect(context.page_title).toBe('资源 · ecs-prod-01')
  })

  it('falls back to registry resource type when cloud type is missing', () => {
    syncAssetAIContext({ applicationId: 'app-1', resource: resource({ cloud_resource_type: undefined }) })
    expect(useAIContextStore().current.resource_type).toBe('host')
  })

  it('writes application context when no resource is highlighted', () => {
    syncAssetAIContext({ applicationId: 'app-1', applicationName: 'payment-service' })
    const context = useAIContextStore().current
    expect(context.application_id).toBe('app-1')
    expect(context.resource_id).toBeUndefined()
    expect(context.page_title).toBe('应用 · payment-service')
  })

  it('clears resource context when nothing is selected', () => {
    syncAssetAIContext({ applicationId: 'app-1', resource: resource() })
    clearAssetAIContext()
    const context = useAIContextStore().current
    expect(context.page).toBe('assets')
    expect(context.application_id).toBeUndefined()
    expect(context.resource_id).toBeUndefined()
    expect(useAIContextStore().hasResourceContext).toBe(false)
  })
})