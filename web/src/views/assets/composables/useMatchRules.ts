import { reactive, ref } from 'vue'
import type { Ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import * as assetApi from '@/api/asset'
import type { Application, MatchRule, Resource } from '@/api/asset'

export function useMatchRules(options: {
  selectedAppId: Ref<string>
  ruleApplicationOptions: Ref<Application[]>
  ruleResourceOptions: Ref<Resource[]>
}) {
  const { selectedAppId, ruleApplicationOptions, ruleResourceOptions } = options

  const matchRules = ref<MatchRule[]>([])
  const rulesLoading = ref(false)
  const ruleModalVisible = ref(false)
  const ruleModalMode = ref<'create' | 'edit'>('create')
  const editingRuleId = ref('')
  const ruleSaving = ref(false)

  const rulePagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })

  const ruleForm = reactive({
    name: '',
    enabled: true,
    priority: 100,
    target_type: 'application',
    source_type: 'all',
    label_key: 'service',
    label_value_pattern: '',
    application_id: '',
    resource_id: ''
  })

  const ruleColumns: TableInstance['columns'] = [
    { title: '规则名', dataIndex: 'name', ellipsis: true, tooltip: true },
    { title: '优先级', dataIndex: 'priority', width: 80 },
    { title: '状态', slotName: 'enabled', width: 80 },
    { title: 'Label', dataIndex: 'label_key', width: 100, ellipsis: true, tooltip: true },
    { title: '模式', dataIndex: 'label_value_pattern', width: 140, ellipsis: true, tooltip: true },
    { title: '目标', slotName: 'target_type', width: 100 },
    { title: '接入源', dataIndex: 'source_type', width: 160, ellipsis: true, tooltip: true },
    { title: '应用 ID', dataIndex: 'application_id', width: 120, ellipsis: true, tooltip: true },
    { title: '操作', slotName: 'actions', width: 120 }
  ]

  async function loadMatchRules() {
    rulesLoading.value = true
    try {
      const res = await assetApi.listMatchRules({
        page: rulePagination.current,
        page_size: rulePagination.pageSize
      })
      matchRules.value = res.items ?? []
      rulePagination.total = res.total ?? 0
    } finally {
      rulesLoading.value = false
    }
  }

  async function loadAllPages<T>(loader: (page: number, pageSize: number) => Promise<{ items?: T[]; total?: number }>, pageSize = 100) {
    const items: T[] = []
    let page = 1
    let total = 0
    do {
      const res = await loader(page, pageSize)
      const pageItems = res.items ?? []
      items.push(...pageItems)
      total = res.total ?? items.length
      if (pageItems.length === 0) break
      page += 1
    } while (items.length < total)
    return items
  }

  async function loadRuleApplications() {
    ruleApplicationOptions.value = await loadAllPages<Application>((page, pageSize) => assetApi.listApplications({
      page,
      page_size: pageSize
    }))
  }

  async function loadRuleResources(appId: string) {
    if (!appId) {
      ruleResourceOptions.value = []
      return
    }
    ruleResourceOptions.value = await loadAllPages<Resource>((page, pageSize) => assetApi.listResources(appId, {
      page,
      page_size: pageSize
    }))
  }

  async function openCreateMatchRule() {
    ruleModalMode.value = 'create'
    editingRuleId.value = ''
    ruleForm.name = ''
    ruleForm.enabled = true
    ruleForm.priority = 100
    ruleForm.target_type = 'application'
    ruleForm.source_type = 'all'
    ruleForm.label_key = 'service'
    ruleForm.label_value_pattern = ''
    ruleForm.application_id = selectedAppId.value || ''
    ruleForm.resource_id = ''
    await loadRuleApplications()
    void loadRuleResources(ruleForm.application_id)
    ruleModalVisible.value = true
  }

  async function openEditMatchRule(rule: MatchRule) {
    ruleModalMode.value = 'edit'
    editingRuleId.value = rule.id
    ruleForm.name = rule.name
    ruleForm.enabled = rule.enabled
    ruleForm.priority = rule.priority
    ruleForm.target_type = rule.target_type
    ruleForm.source_type = rule.source_type
    ruleForm.label_key = rule.label_key
    ruleForm.label_value_pattern = rule.label_value_pattern
    ruleForm.application_id = rule.application_id
    ruleForm.resource_id = rule.resource_id || ''
    await loadRuleApplications()
    void loadRuleResources(rule.application_id)
    ruleModalVisible.value = true
  }

  async function onRuleAppChange(value: string | number | boolean | Record<string, unknown> | (string | number | boolean | Record<string, unknown>)[]) {
    if (typeof value !== 'string' || !value) {
      return
    }
    ruleForm.resource_id = ''
    await loadRuleResources(value)
  }

  async function submitMatchRule() {
    if (!ruleForm.name.trim() || !ruleForm.label_key.trim() || !ruleForm.label_value_pattern.trim() || !ruleForm.application_id) {
      Message.warning('请填写规则名、Label、匹配模式并选择应用')
      return
    }
    if (ruleForm.target_type === 'resource' && !ruleForm.resource_id) {
      Message.warning('目标类型为 resource 时必须选择资源')
      return
    }
    ruleSaving.value = true
    try {
      const payload = {
        name: ruleForm.name.trim(),
        enabled: ruleForm.enabled,
        priority: ruleForm.priority,
        target_type: ruleForm.target_type,
        source_type: ruleForm.source_type,
        label_key: ruleForm.label_key.trim(),
        label_value_pattern: ruleForm.label_value_pattern.trim(),
        application_id: ruleForm.application_id,
        resource_id: ruleForm.target_type === 'resource' ? ruleForm.resource_id : undefined
      }
      if (ruleModalMode.value === 'edit' && editingRuleId.value) {
        await assetApi.updateMatchRule(editingRuleId.value, payload)
        Message.success('规则已更新')
      } else {
        await assetApi.createMatchRule(payload)
        Message.success('规则已创建')
      }
      ruleModalVisible.value = false
      rulePagination.current = 1
      await loadMatchRules()
    } finally {
      ruleSaving.value = false
    }
  }

  async function confirmDeleteMatchRule(rule: MatchRule) {
    try {
      await assetApi.deleteMatchRule(rule.id)
      Message.success('规则已删除')
      await loadMatchRules()
    } catch (e: unknown) {
      Message.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  function onRulePageChange(page: number) {
    rulePagination.current = page
    loadMatchRules()
  }

  function onRulePageSizeChange(size: number) {
    rulePagination.pageSize = size
    rulePagination.current = 1
    loadMatchRules()
  }

  return {
    matchRules,
    rulesLoading,
    ruleModalVisible,
    ruleModalMode,
    editingRuleId,
    ruleSaving,
    ruleForm,
    ruleApplicationOptions,
    ruleResourceOptions,
    rulePagination,
    ruleColumns,
    loadMatchRules,
    loadRuleApplications,
    loadRuleResources,
    openCreateMatchRule,
    openEditMatchRule,
    onRuleAppChange,
    submitMatchRule,
    confirmDeleteMatchRule,
    onRulePageChange,
    onRulePageSizeChange
  }
}
