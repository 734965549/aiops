import { computed, nextTick, reactive, ref } from 'vue'
import type { Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { LocationQueryRaw } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import * as assetApi from '@/api/asset'
import type { Application, Resource } from '@/api/asset'
import {
  OPTIONAL_RESOURCE_COLUMNS,
  SORTABLE_RESOURCE_FIELDS,
  labelEntries,
  resourceFiltersFromQuery,
  resourceSortFromQuery,
  toResourceSortParams,
  withResourceQuery
} from './assetUtils'
import type { OptionalResourceColumnKey, ResourceFilterState, ResourceSortState } from './assetUtils'

type TableColumn = NonNullable<TableInstance['columns']>[number]

export function useAssetRegistry(options: {
  selectedAppId: Ref<string>
  ruleApplicationOptions: Ref<Application[]>
}) {
  const { selectedAppId, ruleApplicationOptions } = options
  const route = useRoute()
  const router = useRouter()

  const applications = ref<Application[]>([])
  const resources = ref<Resource[]>([])
  const highlightedResourceId = ref('')
  const appsLoading = ref(false)
  const resourcesLoading = ref(false)
  const filterOptionsLoading = ref(false)
  const appModalVisible = ref(false)
  const resourceModalVisible = ref(false)
  const appModalMode = ref<'create' | 'edit'>('create')
  const resourceModalMode = ref<'create' | 'edit'>('create')
  const editingAppId = ref('')
  const editingResourceId = ref('')
  const appSaving = ref(false)
  const resourceSaving = ref(false)

  const appPagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })
  const resourcePagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })

  // 初始筛选/排序从 URL 还原，保证刷新与分享链接可复现同一视图。
  const resourceFilters = reactive<ResourceFilterState>(
    resourceFiltersFromQuery(route.query as Record<string, unknown>)
  )

  // 排序只记录状态，真正排序由后端按 sort_by / sort_order 完成，保证跨页正确。
  const resourceSorter = reactive<ResourceSortState>(
    resourceSortFromQuery(route.query as Record<string, unknown>)
  )

  // 云类型 / Region 的候选值来自后端字典接口，只用于下拉建议，不参与本地过滤。
  const resourceFilterOptions = reactive<assetApi.ResourceFilterOptions>({
    cloud_resource_types: [],
    regions: []
  })
  let filterOptionsAppId = ''

  const appForm = reactive({
    name: '',
    environment: 'prod',
    namespace: '',
    description: ''
  })

  const resourceForm = reactive({
    name: '',
    resource_type: 'pod',
    namespace: '',
    pod: '',
    node: '',
    instance: ''
  })

  // 左栏只有 1~2 列可用：应用名 + 环境/Namespace/描述合并到同一单元格，
  // 编辑/删除改为图标按钮，避免窄栏里再出现被压扁的文字列和横向滚动条。
  const appColumns: TableInstance['columns'] = [
    { title: '应用', slotName: 'name', width: 200 },
    { title: '操作', slotName: 'actions', width: 88 }
  ]

  // 「附加列」默认全部关闭。验收要求可见的「来源 / 云资源 ID / Region / 同步状态」
  // 都在核心列里；勾选附加列后表格才切回横向滚动。
  const visibleExtraColumns = ref<OptionalResourceColumnKey[]>([])

  // 核心列宽度只用于「附加列开启」时的 scroll.x 计算；
  // 默认视图下由 .page-table-fit 的 !important 百分比列宽覆盖。
  const baseResourceColumns: TableColumn[] = [
    { title: '资源名', dataIndex: 'name', width: 150, ellipsis: true, tooltip: true },
    { title: '来源', dataIndex: 'source', slotName: 'source', width: 80 },
    { title: '类型', dataIndex: 'resource_type', width: 80 },
    { title: '云类型', dataIndex: 'cloud_resource_type', slotName: 'cloud_resource_type', width: 90 },
    { title: '云账号', dataIndex: 'integration_account_id', slotName: 'integration_account_id', width: 150 },
    { title: '云资源 ID', dataIndex: 'cloud_resource_id', slotName: 'cloud_resource_id', width: 150, ellipsis: true },
    { title: 'Region', dataIndex: 'region', slotName: 'region', width: 100 },
    { title: '同步状态', dataIndex: 'sync_status', slotName: 'sync_status', width: 90 }
  ]
  const baseResourceWidth = baseResourceColumns.reduce((sum, col) => sum + (col.width || 0), 0) + 110

  const sortableFields: readonly string[] = SORTABLE_RESOURCE_FIELDS

  // sorter:true 让 Arco 只发出 sorterChange、不做本地排序（本地排序只对当前页生效）。
  // defaultSortOrder 保证「列设置」改变列定义后表头箭头不丢。
  function sortableFor(field: string): TableColumn['sortable'] {
    const active = resourceSorter.field === field && resourceSorter.direction !== ''
    return {
      sortDirections: ['ascend', 'descend'] as ('ascend' | 'descend')[],
      sorter: true,
      ...(active ? { defaultSortOrder: resourceSorter.direction as 'ascend' | 'descend' } : {})
    }
  }

  const resourceColumns = computed<TableInstance['columns']>(() => {
    const core: TableColumn[] = baseResourceColumns.map((col) => {
      const field = typeof col.dataIndex === 'string' ? col.dataIndex : ''
      return field && sortableFields.includes(field)
        ? { ...col, sortable: sortableFor(field) }
        : { ...col }
    })
    const extras: TableColumn[] = OPTIONAL_RESOURCE_COLUMNS
      .filter((col) => visibleExtraColumns.value.includes(col.key))
      .map((col) => {
        const field = col.dataIndex || col.slotName || ''
        const column: TableColumn = {
          title: col.label,
          width: col.width,
          ...(col.slotName
            ? { slotName: col.slotName }
            : { dataIndex: col.dataIndex || col.key, ellipsis: true, tooltip: true })
        }
        if (field && field !== 'labels' && sortableFields.includes(field)) {
          return { ...column, dataIndex: field, sortable: sortableFor(field) }
        }
        return column
      })
    return [
      ...core,
      ...extras,
      {
        title: '操作',
        slotName: 'actions',
        width: 110,
        // 默认视图不横向滚动，无需固定列；fixed + 百分比列宽会让 sticky 计算错位。
        ...(extras.length ? { fixed: 'right' as const } : {})
      }
    ]
  })

  const resourceTableScroll = computed<Record<string, unknown>>(() => {
    if (!visibleExtraColumns.value.length) return {}
    const extraWidth = OPTIONAL_RESOURCE_COLUMNS
      .filter((col) => visibleExtraColumns.value.includes(col.key))
      .reduce((sum, col) => sum + col.width, 0)
    return { x: baseResourceWidth + extraWidth }
  })

  const selectedApplication = computed(() => {
    return applications.value.find((a) => a.id === selectedAppId.value)
      || ruleApplicationOptions.value.find((a) => a.id === selectedAppId.value)
  })

  const selectedAppName = computed(() => {
    return selectedApplication.value?.name || selectedAppId.value
  })

  const resourceCardTitle = computed(() => {
    if (!selectedAppId.value) return '资源'
    return `资源 · ${selectedAppName.value}`
  })

  const editingResource = computed(() => {
    if (!editingResourceId.value) return undefined
    return resources.value.find((r) => r.id === editingResourceId.value)
  })

  const editingResourceLabelEntries = computed(() => labelEntries(editingResource.value?.labels))

  function appRowClass(record: TableData) {
    const app = record as Application
    return app.id === selectedAppId.value ? 'assets-row-selected' : ''
  }

  function resourceRowClass(record: TableData) {
    const res = record as Resource
    return res.id === highlightedResourceId.value ? 'assets-row-highlight' : ''
  }

  function routeResourceId(): string {
    return typeof route.query.resource_id === 'string' ? route.query.resource_id : ''
  }

  // 筛选与排序进入 URL：刷新、复制链接、浏览器前进/后退都能复原同一组条件。
  function syncFiltersToRoute() {
    const query = withResourceQuery(
      route.query as Record<string, unknown>,
      resourceFilters,
      resourceSorter
    )
    void router.replace({ query: query as LocationQueryRaw })
  }

  async function scrollToHighlightedResource() {
    if (!highlightedResourceId.value) return
    await nextTick()
    const row = document.querySelector('.assets-row-highlight')
    row?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }

  function formatTs(ts?: number) {
    if (!ts) return '-'
    return new Date(ts * 1000).toLocaleString()
  }

  function labelPreviewEntries(resource: Resource) {
    return labelEntries(resource.labels).slice(0, 3)
  }

  async function loadApplications() {
    appsLoading.value = true
    try {
      const res = await assetApi.listApplications({
        page: appPagination.current,
        page_size: appPagination.pageSize
      })
      applications.value = res.items ?? []
      appPagination.total = res.total ?? 0
    } finally {
      appsLoading.value = false
    }
  }

  async function loadResourceFilterOptions(force = false) {
    const appId = selectedAppId.value
    if (!appId) {
      resourceFilterOptions.cloud_resource_types = []
      resourceFilterOptions.regions = []
      filterOptionsAppId = ''
      return
    }
    if (!force && filterOptionsAppId === appId) return
    filterOptionsLoading.value = true
    try {
      const options = await assetApi.getResourceFilterOptions(appId)
      // 应用切换期间旧请求可能后返回，避免覆盖新应用的字典。
      if (selectedAppId.value !== appId) return
      resourceFilterOptions.cloud_resource_types = options.cloud_resource_types ?? []
      resourceFilterOptions.regions = options.regions ?? []
      filterOptionsAppId = appId
    } catch {
      // 字典失败不阻塞列表，保留已有候选值。
    } finally {
      filterOptionsLoading.value = false
    }
  }

  async function loadResources() {
    if (!selectedAppId.value) return
    void loadResourceFilterOptions()
    resourcesLoading.value = true
    try {
      const res = await assetApi.listResources(selectedAppId.value, {
        page: resourcePagination.current,
        page_size: resourcePagination.pageSize,
        keyword: resourceFilters.keyword.trim() || undefined,
        source: resourceFilters.source || undefined,
        cloud_resource_type: resourceFilters.cloud_resource_type.trim() || undefined,
        region: resourceFilters.region.trim() || undefined,
        sync_status: resourceFilters.sync_status || undefined,
        ...toResourceSortParams(resourceSorter.field, resourceSorter.direction)
      })
      resources.value = res.items ?? []
      resourcePagination.total = res.total ?? 0
      const resourceId = routeResourceId()
      if (resourceId && resources.value.some((r) => r.id === resourceId)) {
        highlightedResourceId.value = resourceId
        await scrollToHighlightedResource()
      } else {
        highlightedResourceId.value = ''
      }
    } finally {
      resourcesLoading.value = false
    }
  }

  function applyResourceFilters() {
    resourcePagination.current = 1
    syncFiltersToRoute()
    loadResources()
  }

  function resetResourceFilters() {
    resourceFilters.keyword = ''
    resourceFilters.source = ''
    resourceFilters.cloud_resource_type = ''
    resourceFilters.region = ''
    resourceFilters.sync_status = ''
    applyResourceFilters()
  }

  function onResourceSorterChange(dataIndex: string, direction: string) {
    const params = toResourceSortParams(dataIndex, direction)
    resourceSorter.field = params.sort_by || ''
    resourceSorter.direction = !params.sort_by
      ? ''
      : params.sort_order === 'desc' ? 'descend' : 'ascend'
    // 排序是服务端行为，换排序后必须回到第 1 页，否则停留在越界页码。
    resourcePagination.current = 1
    syncFiltersToRoute()
    loadResources()
  }

  function setVisibleExtraColumns(keys: OptionalResourceColumnKey[]) {
    visibleExtraColumns.value = keys
  }

  function onSelectApplication(record: TableData) {
    const app = record as Application
    if (selectedAppId.value === app.id) return
    selectedAppId.value = app.id
    resourcePagination.current = 1
    router.replace({ query: { ...route.query, application_id: app.id } })
    loadResources()
  }

  function openCreateApplication() {
    appModalMode.value = 'create'
    editingAppId.value = ''
    appForm.name = ''
    appForm.environment = 'prod'
    appForm.namespace = ''
    appForm.description = ''
    appModalVisible.value = true
  }

  function openEditApplication(app: Application) {
    appModalMode.value = 'edit'
    editingAppId.value = app.id
    appForm.name = app.name
    appForm.environment = app.environment || ''
    appForm.namespace = app.namespace || ''
    appForm.description = app.description || ''
    appModalVisible.value = true
  }

  function openCreateResource() {
    resourceModalMode.value = 'create'
    editingResourceId.value = ''
    resourceForm.name = ''
    resourceForm.resource_type = 'pod'
    resourceForm.namespace = selectedApplication.value?.namespace || ''
    resourceForm.pod = ''
    resourceForm.node = ''
    resourceForm.instance = ''
    resourceModalVisible.value = true
  }

  function openEditResource(res: Resource) {
    resourceModalMode.value = 'edit'
    editingResourceId.value = res.id
    resourceForm.name = res.name || ''
    resourceForm.resource_type = res.resource_type || 'pod'
    resourceForm.namespace = res.namespace || ''
    resourceForm.pod = res.pod || ''
    resourceForm.node = res.node || ''
    resourceForm.instance = res.instance || ''
    resourceModalVisible.value = true
  }

  async function submitApplication() {
    if (!appForm.name.trim()) {
      Message.warning('请填写应用名')
      return
    }
    appSaving.value = true
    try {
      if (appModalMode.value === 'edit' && editingAppId.value) {
        await assetApi.updateApplication(editingAppId.value, {
          name: appForm.name.trim(),
          environment: appForm.environment || undefined,
          namespace: appForm.namespace.trim() || undefined,
          description: appForm.description.trim() || undefined
        })
        Message.success('应用已更新')
      } else {
        const created = await assetApi.createApplication({
          name: appForm.name.trim(),
          environment: appForm.environment || undefined,
          namespace: appForm.namespace.trim() || undefined,
          description: appForm.description.trim() || undefined
        })
        Message.success('应用已创建')
        selectedAppId.value = created.id
        resourcePagination.current = 1
        router.replace({ query: { ...route.query, application_id: created.id } })
        await loadResources()
      }
      appModalVisible.value = false
      await loadApplications()
    } finally {
      appSaving.value = false
    }
  }

  async function confirmDeleteApplication(app: Application) {
    try {
      await assetApi.deleteApplication(app.id)
      Message.success('应用已删除')
      if (selectedAppId.value === app.id) {
        selectedAppId.value = ''
        resources.value = []
        const query = { ...route.query }
        delete query.application_id
        delete query.resource_id
        router.replace({ query })
      }
      await loadApplications()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : '删除失败'
      Message.error(msg.includes('resource') ? '请先删除该应用下的所有资源' : msg)
    }
  }

  async function submitResource() {
    if (!selectedAppId.value) return
    resourceSaving.value = true
    try {
      if (resourceModalMode.value === 'edit' && editingResourceId.value) {
        await assetApi.updateResource(editingResourceId.value, {
          name: resourceForm.name.trim() || undefined,
          resource_type: resourceForm.resource_type || undefined,
          namespace: resourceForm.namespace.trim() || undefined,
          pod: resourceForm.pod.trim() || undefined,
          node: resourceForm.node.trim() || undefined,
          instance: resourceForm.instance.trim() || undefined
        })
        Message.success('资源已更新')
      } else {
        await assetApi.createResource({
          application_id: selectedAppId.value,
          name: resourceForm.name.trim() || undefined,
          resource_type: resourceForm.resource_type || undefined,
          namespace: resourceForm.namespace.trim() || undefined,
          pod: resourceForm.pod.trim() || undefined,
          node: resourceForm.node.trim() || undefined,
          instance: resourceForm.instance.trim() || undefined
        })
        Message.success('资源已创建')
      }
      resourceModalVisible.value = false
      resourcePagination.current = resourceModalMode.value === 'create' ? 1 : resourcePagination.current
      // 资源变更可能产生新的云类型/Region，强制刷新字典。
      filterOptionsAppId = ''
      await loadResources()
    } finally {
      resourceSaving.value = false
    }
  }

  async function confirmDeleteResource(res: Resource) {
    try {
      await assetApi.deleteResource(res.id)
      Message.success('资源已删除')
      if (highlightedResourceId.value === res.id) {
        highlightedResourceId.value = ''
        const query = { ...route.query }
        delete query.resource_id
        router.replace({ query })
      }
      filterOptionsAppId = ''
      await loadResources()
    } catch (e: unknown) {
      Message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  function onAppPageChange(page: number) {
    appPagination.current = page
    loadApplications()
  }

  function onAppPageSizeChange(size: number) {
    appPagination.pageSize = size
    appPagination.current = 1
    loadApplications()
  }

  function onResourcePageChange(page: number) {
    resourcePagination.current = page
    loadResources()
  }

  function onResourcePageSizeChange(size: number) {
    resourcePagination.pageSize = size
    resourcePagination.current = 1
    loadResources()
  }

  return {
    applications,
    resources,
    selectedAppId,
    highlightedResourceId,
    appsLoading,
    resourcesLoading,
    appModalVisible,
    resourceModalVisible,
    appModalMode,
    resourceModalMode,
    editingAppId,
    editingResourceId,
    appSaving,
    resourceSaving,
    appForm,
    resourceForm,
    resourceFilters,
    resourceSorter,
    resourceFilterOptions,
    filterOptionsLoading,
    visibleExtraColumns,
    appPagination,
    resourcePagination,
    resourceTableScroll,
    appColumns,
    resourceColumns,
    selectedApplication,
    selectedAppName,
    resourceCardTitle,
    editingResource,
    editingResourceLabelEntries,
    appRowClass,
    resourceRowClass,
    routeResourceId,
    scrollToHighlightedResource,
    formatTs,
    labelPreviewEntries,
    loadApplications,
    loadResources,
    loadResourceFilterOptions,
    applyResourceFilters,
    resetResourceFilters,
    onResourceSorterChange,
    setVisibleExtraColumns,
    onSelectApplication,
    openCreateApplication,
    openEditApplication,
    openCreateResource,
    openEditResource,
    submitApplication,
    confirmDeleteApplication,
    submitResource,
    confirmDeleteResource,
    onAppPageChange,
    onAppPageSizeChange,
    onResourcePageChange,
    onResourcePageSizeChange
  }
}
