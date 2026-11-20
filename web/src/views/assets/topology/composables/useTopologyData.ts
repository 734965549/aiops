import { computed, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import { getResourceFilterOptions, listApplications, type Application, type ResourceFilterOptions } from '@/api/asset'
import {
  createAssetResourceRelation,
  deleteAssetResourceRelation,
  getAssetTopology,
  listAssetResourceRelations,
  type AssetResourceRelation,
  type AssetTopologyQuery,
  type AssetTopologySnapshot,
  type CreateAssetResourceRelationInput,
  type ResourceRelationType
} from '@/api/asset-topology'
import { topologyResourceNodes } from './topologyPresentation'

export function useTopologyData() {
  const applications = ref<Application[]>([])
  const applicationsLoading = ref(false)
  const selectedApplicationId = ref('')
  const rootResourceId = ref('')
  const depth = ref(2)
  const relationDirection = ref<'both' | 'upstream' | 'downstream'>('both')
  const topology = ref<AssetTopologySnapshot | null>(null)
  const relations = ref<AssetResourceRelation[]>([])
  const loading = ref(false)
  const relationsLoading = ref(false)
  const saving = ref(false)
  const error = ref('')
  const filterOptions = ref<ResourceFilterOptions>({ cloud_resource_types: [], regions: [] })
  const filters = reactive<{
    keyword: string
    region: string
    cloudResourceType: string
    relationType: '' | ResourceRelationType
    includeStale: boolean
  }>({
    keyword: '',
    region: '',
    cloudResourceType: '',
    relationType: '',
    includeStale: false
  })

  const resourceNodes = computed(() => topologyResourceNodes(topology.value?.nodes ?? []))

  async function loadApplications(preferredApplicationId = '') {
    applicationsLoading.value = true
    try {
      const result = await listApplications({ page: 1, page_size: 100 })
      applications.value = result.items
      const preferred = preferredApplicationId || selectedApplicationId.value
      selectedApplicationId.value = applications.value.some((app) => app.id === preferred)
        ? preferred
        : applications.value[0]?.id || ''
      if (selectedApplicationId.value) await selectApplication(selectedApplicationId.value)
    } catch {
      applications.value = []
      selectedApplicationId.value = ''
      topology.value = null
      error.value = '应用列表加载失败，请检查权限或稍后重试。'
    } finally {
      applicationsLoading.value = false
    }
  }

  function topologyQuery(): AssetTopologyQuery {
    if (rootResourceId.value) {
      return {
        root_resource_id: rootResourceId.value,
        depth: depth.value,
        direction: relationDirection.value,
        keyword: filters.keyword || undefined,
        region: filters.region || undefined,
        cloud_resource_type: filters.cloudResourceType || undefined,
        relation_type: filters.relationType || undefined,
        include_stale: filters.includeStale
      }
    }
    return {
      application_id: selectedApplicationId.value,
      keyword: filters.keyword || undefined,
      region: filters.region || undefined,
      cloud_resource_type: filters.cloudResourceType || undefined,
      relation_type: filters.relationType || undefined,
      include_stale: filters.includeStale
    }
  }

  async function loadTopology() {
    if (!selectedApplicationId.value) {
      topology.value = null
      return
    }
    loading.value = true
    error.value = ''
    try {
      topology.value = await getAssetTopology(topologyQuery())
    } catch {
      topology.value = null
      error.value = '拓扑加载失败，请检查权限或稍后重试。'
    } finally {
      loading.value = false
    }
  }

  async function loadRelations() {
    if (!selectedApplicationId.value) {
      relations.value = []
      return
    }
    relationsLoading.value = true
    try {
      const result = await listAssetResourceRelations({
        application_id: selectedApplicationId.value,
        relation_type: filters.relationType || undefined
      })
      relations.value = result.items
    } catch {
      relations.value = []
    } finally {
      relationsLoading.value = false
    }
  }

  async function loadFilterOptions() {
    if (!selectedApplicationId.value) return
    try {
      filterOptions.value = await getResourceFilterOptions(selectedApplicationId.value)
    } catch {
      filterOptions.value = { cloud_resource_types: [], regions: [] }
    }
  }

  async function refreshAll() {
    await Promise.all([loadTopology(), loadRelations(), loadFilterOptions()])
  }

  async function selectApplication(applicationId: string) {
    selectedApplicationId.value = applicationId
    rootResourceId.value = ''
    topology.value = null
    relations.value = []
    await refreshAll()
    if (resourceNodes.value.length) {
      rootResourceId.value = resourceNodes.value[0].entity_id
      await loadTopology()
    }
  }

  async function focusResource(resourceId: string) {
    rootResourceId.value = resourceId
    await loadTopology()
  }

  async function clearRoot() {
    rootResourceId.value = ''
    await loadTopology()
  }

  function resetFilters() {
    filters.keyword = ''
    filters.region = ''
    filters.cloudResourceType = ''
    filters.relationType = ''
    filters.includeStale = false
    return refreshAll()
  }

  async function createRelation(input: CreateAssetResourceRelationInput) {
    saving.value = true
    try {
      await createAssetResourceRelation(input)
      Message.success('资源关系已创建')
      await refreshAll()
      return true
    } catch {
      return false
    } finally {
      saving.value = false
    }
  }

  async function removeRelation(relationId: string) {
    try {
      await deleteAssetResourceRelation(relationId)
      Message.success('资源关系已删除')
      await refreshAll()
    } catch {
      // 请求层已展示错误。
    }
  }

  return {
    applications,
    applicationsLoading,
    selectedApplicationId,
    rootResourceId,
    depth,
    relationDirection,
    topology,
    relations,
    resourceNodes,
    loading,
    relationsLoading,
    saving,
    error,
    filters,
    filterOptions,
    loadApplications,
    loadTopology,
    refreshAll,
    selectApplication,
    focusResource,
    clearRoot,
    resetFilters,
    createRelation,
    removeRelation
  }
}
