import { computed, nextTick, reactive, ref } from 'vue'
import type { Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import * as assetApi from '@/api/asset'
import type { Application, Resource } from '@/api/asset'
import { labelEntries } from './assetUtils'

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
  const appModalVisible = ref(false)
  const resourceModalVisible = ref(false)
  const appModalMode = ref<'create' | 'edit'>('create')
  const resourceModalMode = ref<'create' | 'edit'>('create')
  const editingAppId = ref('')
  const editingResourceId = ref('')
  const appSaving = ref(false)
  const resourceSaving = ref(false)

  const tableScroll = { y: 'calc(100vh - 330px)' }
  const resourceTableScroll = { x: 1880, y: 'calc(100vh - 330px)' }

  const appPagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })
  const resourcePagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })

  const resourceFilters = reactive({
    cloud_resource_type: '',
    region: '',
    sync_status: ''
  })

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

  const appColumns: TableInstance['columns'] = [
    { title: '应用名', dataIndex: 'name', ellipsis: true, tooltip: true, width: 120 },
    { title: '环境', slotName: 'environment', width: 80 },
    { title: 'Namespace', slotName: 'namespace', width: 110, ellipsis: true },
    { title: '描述', dataIndex: 'description', ellipsis: true, tooltip: true },
    { title: '操作', slotName: 'actions', width: 100 }
  ]

  const resourceColumns: TableInstance['columns'] = [
    { title: '资源名', dataIndex: 'name', ellipsis: true, tooltip: true, width: 140 },
    { title: '来源', slotName: 'source', width: 80 },
    { title: '类型', dataIndex: 'resource_type', width: 80 },
    { title: '账号', slotName: 'integration_account_id', width: 180 },
    { title: '云类型', slotName: 'cloud_resource_type', width: 90 },
    { title: '云资源 ID', slotName: 'cloud_resource_id', width: 150, ellipsis: true },
    { title: 'Region', slotName: 'region', width: 100 },
    { title: '同步状态', slotName: 'sync_status', width: 90 },
    { title: '最近同步', slotName: 'last_synced_at', width: 150 },
    { title: '最近成功批次', slotName: 'sync_batch_id', width: 180 },
    { title: 'Labels', slotName: 'labels', width: 220 },
    { title: 'Namespace', dataIndex: 'namespace', width: 110, ellipsis: true, tooltip: true },
    { title: 'Pod', dataIndex: 'pod', width: 110, ellipsis: true, tooltip: true },
    { title: 'Instance', dataIndex: 'instance', width: 120, ellipsis: true, tooltip: true },
    { title: '操作', slotName: 'actions', width: 120, fixed: 'right' }
  ]

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

  async function loadResources() {
    if (!selectedAppId.value) return
    resourcesLoading.value = true
    try {
      const res = await assetApi.listResources(selectedAppId.value, {
        page: resourcePagination.current,
        page_size: resourcePagination.pageSize,
        cloud_resource_type: resourceFilters.cloud_resource_type.trim() || undefined,
        region: resourceFilters.region.trim() || undefined,
        sync_status: resourceFilters.sync_status || undefined
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
    loadResources()
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
    appPagination,
    resourcePagination,
    tableScroll,
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
    applyResourceFilters,
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
