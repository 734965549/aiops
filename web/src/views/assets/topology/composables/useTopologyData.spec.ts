// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  listApplications: vi.fn(),
  getResourceFilterOptions: vi.fn(),
  getAssetTopology: vi.fn(),
  listAssetResourceRelations: vi.fn(),
  createAssetResourceRelation: vi.fn(),
  deleteAssetResourceRelation: vi.fn()
}))

vi.mock('@/api/asset', () => ({
  listApplications: mocks.listApplications,
  getResourceFilterOptions: mocks.getResourceFilterOptions
}))

vi.mock('@/api/asset-topology', () => ({
  getAssetTopology: mocks.getAssetTopology,
  listAssetResourceRelations: mocks.listAssetResourceRelations,
  createAssetResourceRelation: mocks.createAssetResourceRelation,
  deleteAssetResourceRelation: mocks.deleteAssetResourceRelation
}))

import { useTopologyData } from './useTopologyData'

const applicationNode = {
  node_id: 'application:app-1', entity_type: 'application' as const, entity_id: 'app-1', name: 'payment'
}
const resourceNode = {
  node_id: 'resource:res-1', entity_type: 'resource' as const, entity_id: 'res-1', name: 'gateway'
}

describe('useTopologyData', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listApplications.mockResolvedValue({
      items: [{ id: 'app-1', name: 'payment', environment: 'prod' }], total: 1, page: 1, page_size: 100
    })
    mocks.getResourceFilterOptions.mockResolvedValue({ cloud_resource_types: ['ecs'], regions: ['cn-north-4'] })
    mocks.getAssetTopology.mockResolvedValue({
      nodes: [applicationNode, resourceNode], edges: [],
      meta: { generated_at: 1, truncated: false, node_count: 2, edge_count: 0 }
    })
    mocks.listAssetResourceRelations.mockResolvedValue({ items: [], total: 0 })
  })

  it('loads the first application and its real topology', async () => {
    const data = useTopologyData()
    await data.loadApplications()

    expect(data.selectedApplicationId.value).toBe('app-1')
    expect(data.resourceNodes.value).toEqual([resourceNode])
    expect(mocks.getAssetTopology).toHaveBeenCalledWith({
      application_id: 'app-1', keyword: undefined, region: undefined,
      cloud_resource_type: undefined, relation_type: undefined, include_stale: false
    })
    expect(mocks.getAssetTopology).toHaveBeenLastCalledWith({
      root_resource_id: 'res-1', depth: 2, direction: 'both', keyword: undefined,
      region: undefined, cloud_resource_type: undefined, relation_type: undefined, include_stale: false
    })
  })

  it('contains application list failures without an unhandled mounted error', async () => {
    mocks.listApplications.mockRejectedValueOnce(new Error('api unavailable'))
    const data = useTopologyData()

    await expect(data.loadApplications()).resolves.toBeUndefined()
    expect(data.applications.value).toEqual([])
    expect(data.selectedApplicationId.value).toBe('')
    expect(data.error.value).toContain('应用列表加载失败')
  })
})
