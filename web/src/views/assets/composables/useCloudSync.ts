import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import * as assetApi from '@/api/asset'
import { formatSyncBatchSummary } from './assetUtils'

export function useCloudSync(options?: {
  onSyncComplete?: (batch: assetApi.SyncBatch) => Promise<void> | void
}) {
  const syncAccountId = ref('')
  const syncLoading = ref(false)
  const syncBatchesLoading = ref(false)
  const syncBatches = ref<assetApi.SyncBatch[]>([])
  const syncBatchDetailVisible = ref(false)
  const syncBatchDetailLoading = ref(false)
  const syncBatchDetail = ref<assetApi.SyncBatch | null>(null)
  const selectedSyncScopeKey = ref('')
  const selectedScopeSignalKey = ref('')

  const tableScroll = { y: 'calc(100vh - 330px)' }

  const syncPagination = reactive({ current: 1, pageSize: 10, total: 0, showTotal: true, showPageSize: true })

  // 请求序号：快速切换批次/翻页时丢弃旧响应，避免旧请求覆盖新结果。
  let batchListReqSeq = 0
  let batchDetailReqSeq = 0

  const syncBatchColumns: TableInstance['columns'] = [
    { title: '批次 ID', dataIndex: 'batch_id', ellipsis: true, tooltip: true },
    { title: '账号', dataIndex: 'integration_account_id', width: 220, ellipsis: true, tooltip: true },
    { title: '状态', slotName: 'status', width: 90 },
    { title: '警告', slotName: 'signal', width: 92 },
    { title: '新建', dataIndex: 'created_count', width: 70 },
    { title: '更新', dataIndex: 'updated_count', width: 70 },
    { title: 'Stale', dataIndex: 'stale_count', width: 70 },
    { title: '失败', dataIndex: 'failed_count', width: 70 },
    { title: '摘要', slotName: 'message', ellipsis: true, tooltip: true },
    { title: '操作', slotName: 'actions', width: 80, fixed: 'right' }
  ]

  const syncBatchScopeDiagnosticColumns: TableInstance['columns'] = [
    { title: 'Region', dataIndex: 'region', width: 120, ellipsis: true, tooltip: true },
    { title: 'Failed scopes', slotName: 'failed_scopes', width: 160, ellipsis: true, tooltip: true },
    { title: 'Query failed', slotName: 'query_failed_types', width: 130, ellipsis: true, tooltip: true },
    { title: 'Conversion failed', slotName: 'conversion_failed_types', width: 140, ellipsis: true, tooltip: true },
    { title: 'Enriched', dataIndex: 'enriched_count', width: 90 },
    { title: 'Enrichment failed', slotName: 'enrichment_failed_types', width: 140, ellipsis: true, tooltip: true },
    { title: 'Enrichment warnings', slotName: 'enrichment_warnings', width: 150, ellipsis: true, tooltip: true },
    { title: 'Max reached', slotName: 'max_resources_reached', width: 100 }
  ]

  // 契约 scopes[] 按 region/project_id/resource_group 逐 scope 明细，
  // 同 region 下可能存在多 project/resource_group，需用复合 key 唯一定位，避免选错明细
  function scopeKey(scope: assetApi.SyncBatchScopeSummary): string {
    return [scope.region, scope.project_id || '', scope.resource_group_id || '', scope.resource_group_name || ''].join('|')
  }

  function toggleScopeByKey(key: string) {
    selectedSyncScopeKey.value = selectedSyncScopeKey.value === key ? '' : key
    selectedScopeSignalKey.value = ''
  }

  function getScopeTraceLabel(scope: assetApi.SyncBatchScopeSummary) {
    const parts = [scope.region]
    if (scope.project_id) parts.push(scope.project_id)
    if (scope.resource_group_name) parts.push(scope.resource_group_name)
    return parts.join(' · ')
  }

  function openScopeTrace(scope: assetApi.SyncBatchScopeSummary) {
    toggleScopeByKey(scopeKey(scope))
  }

  function openScopeSignal(key: string) {
    selectedScopeSignalKey.value = key
  }

  async function copyScopeSnippet() {
    if (!selectedScopeTraceSnippet.value) return
    try {
      await navigator.clipboard.writeText(selectedScopeTraceSnippet.value)
      Message.success('已复制定位块')
    } catch {
      Message.warning('复制失败，请手动选中复制')
    }
  }

  const syncBatchMessageSummary = computed(() => formatSyncBatchSummary(syncBatchDetail.value?.summary, syncBatchDetail.value?.message || ''))

  const syncBatchScopeCards = computed(() => {
    const summary = syncBatchDetail.value?.summary
    if (!summary) return []
    return (summary.scopes || []).map((scope) => ({
      key: scopeKey(scope),
      label: getScopeTraceLabel(scope),
      region: scope.region,
      value: scope.persisted_count ?? 0,
      hint: `CES ${scope.ces_total ?? 0} · discovered ${scope.discovered_count ?? 0} · persisted ${scope.persisted_count ?? 0}`
    }))
  })

  const syncBatchSignalTags = computed(() => {
    const summary = syncBatchDetail.value?.summary
    if (!summary) return []
    const tags: { label: string; value: string; color: string }[] = []
    if (summary.max_resources_reached) tags.push({ label: 'Max reached', value: '是', color: 'orange' })
    if (summary.product_names_empty) tags.push({ label: 'Product names empty', value: '是；仅为兜底白名单结果，不保证完整性', color: 'gold' })
    if ((summary.query_failed_types?.length ?? 0) > 0) tags.push({ label: 'Query failed types', value: summary.query_failed_types!.join(', '), color: 'red' })
    if ((summary.conversion_failed_types?.length ?? 0) > 0) tags.push({ label: 'Conversion failed types', value: summary.conversion_failed_types!.join(', '), color: 'red' })
    if ((summary.enrichment_failed_types?.length ?? 0) > 0) tags.push({ label: 'Enrichment failed types', value: summary.enrichment_failed_types!.join(', '), color: 'red' })
    if ((summary.enrichment_warnings?.length ?? 0) > 0) tags.push({ label: 'Enrichment warnings', value: summary.enrichment_warnings!.join(', '), color: 'orange' })
    if (summary.enrichment_stage_error) tags.push({ label: 'Enrichment stage error', value: summary.enrichment_stage_error, color: 'red' })
    if ((summary.writeback_failed_count ?? 0) > 0) tags.push({ label: 'Writeback failed', value: String(summary.writeback_failed_count), color: 'red' })
    if ((summary.failed_scopes?.length ?? 0) > 0) tags.push({ label: 'Failed scopes', value: summary.failed_scopes!.join(', '), color: 'gray' })
    return tags
  })

  const selectedScopeTrace = computed(() => {
    const summary = syncBatchDetail.value?.summary
    if (!summary || !selectedSyncScopeKey.value) return null
    return summary.scopes?.find((item) => scopeKey(item) === selectedSyncScopeKey.value) || null
  })

  const selectedScopeTraceCards = computed(() => {
    const scope = selectedScopeTrace.value
    if (!scope) return []
    return [
      { key: 'ces_total', label: 'CES total', value: scope.ces_total ?? 0 },
      { key: 'discovered', label: 'Discovered', value: scope.discovered_count ?? 0 },
      { key: 'persisted', label: 'Persisted', value: scope.persisted_count ?? 0 },
      { key: 'raw_fetched', label: 'Raw fetched', value: scope.raw_fetched_count ?? 0 },
      { key: 'mapped', label: 'Mapped', value: scope.mapped_count ?? 0 }
    ]
  })

  const selectedScopeTraceSignal = computed(() => {
    const scope = selectedScopeTrace.value
    if (!scope || !selectedScopeSignalKey.value) return null
    switch (selectedScopeSignalKey.value) {
      case 'query':
        return { label: 'Query failed', value: scope.query_failed_types?.join(', ') || '-' }
      case 'conversion':
        return { label: 'Conversion failed', value: scope.conversion_failed_types?.join(', ') || '-' }
      case 'enrichment':
        return { label: 'Enrichment failed', value: scope.enrichment_failed_types?.join(', ') || '-' }
      case 'warnings':
        return { label: 'Enrichment warnings', value: scope.enrichment_warnings?.join(', ') || '-' }
      case 'stage_error':
        return { label: 'Enrichment stage error', value: scope.enrichment_stage_error || '-' }
      case 'writeback':
        return { label: 'Writeback failed', value: String(scope.writeback_failed_count ?? 0) }
      case 'failed':
        return { label: 'Failed scopes', value: scope.failed_scopes?.join(', ') || '-' }
      case 'max':
        return { label: 'Max reached', value: scope.max_resources_reached ? '是' : '否' }
      default:
        return null
    }
  })

  const selectedScopeTraceSnippet = computed(() => {
    const scope = selectedScopeTrace.value
    const signal = selectedScopeTraceSignal.value
    if (!scope) return ''
    const alertLevel = (() => {
      if (scope.max_resources_reached) return 'warning'
      if ((scope.failed_scopes?.length ?? 0) > 0) return 'warning'
      if ((scope.query_failed_types?.length ?? 0) > 0 || (scope.conversion_failed_types?.length ?? 0) > 0 || (scope.enrichment_failed_types?.length ?? 0) > 0 || scope.enrichment_stage_error || (scope.writeback_failed_count ?? 0) > 0) return 'warning'
      return 'info'
    })()
    const rootCause = (() => {
      if (scope.max_resources_reached) return '查询上限截断，导致 scope 覆盖不完整'
      if ((scope.query_failed_types?.length ?? 0) > 0) return `查询失败类型：${scope.query_failed_types!.join(', ')}`
      if ((scope.conversion_failed_types?.length ?? 0) > 0) return `资源转换失败类型：${scope.conversion_failed_types!.join(', ')}`
      if ((scope.enrichment_failed_types?.length ?? 0) > 0) return `增强失败类型：${scope.enrichment_failed_types!.join(', ')}`
      if (scope.enrichment_stage_error) return `增强阶段整体失败：${scope.enrichment_stage_error}`
      if ((scope.writeback_failed_count ?? 0) > 0) return `label 回写失败 ${scope.writeback_failed_count} 次`
      if ((scope.failed_scopes?.length ?? 0) > 0) return `失败 scope：${scope.failed_scopes!.join(', ')}`
      if (scope.product_names_empty) return 'product_names 为空，走了兜底白名单，需人工复核'
      return '暂无明显异常信号，建议结合原始 message 继续排查'
    })()
    const nextStep = (() => {
      if (scope.max_resources_reached) return '先提高 max_resources，重新触发同步并对照 region / project 差异。'
      if ((scope.query_failed_types?.length ?? 0) > 0) return '先检查 query 端凭据、区域连通性以及该类型的 discovery 实现。'
      if ((scope.conversion_failed_types?.length ?? 0) > 0) return '先核对转换映射规则与字段兼容性，再重跑当前 region。'
      if ((scope.enrichment_failed_types?.length ?? 0) > 0) return '先检查增强接口可用性与失败类型对应的 enrichment 处理链路。'
      if (scope.enrichment_stage_error) return '增强阶段整体失败，先检查 enrichment 端口连通性与凭据，再重跑当前 region。'
      if ((scope.writeback_failed_count ?? 0) > 0) return 'label 回写失败，先检查数据库连通性、租约状态与 batch fencing token。'
      if ((scope.failed_scopes?.length ?? 0) > 0) return '先按 failed_scopes 逐项回溯下游返回和鉴权结果。'
      return '若仍无法定位，复制定位块到工单并附上原始 message 与 summary。'
    })()
    const summaryLines = [
      `摘要原文`,
      `region=${scope.region}`,
      scope.project_id ? `project_id=${scope.project_id}` : '',
      scope.sync_mode ? `sync_mode=${scope.sync_mode}` : '',
      scope.resource_group_name ? `resource_group_name=${scope.resource_group_name}` : '',
      scope.resource_group_id ? `resource_group_id=${scope.resource_group_id}` : '',
      scope.resource_group_selection ? `resource_group_selection=${scope.resource_group_selection}` : '',
      scope.ces_total !== undefined ? `ces_total=${scope.ces_total}` : '',
      scope.discovered_count !== undefined ? `discovered_count=${scope.discovered_count}` : '',
      scope.persisted_count !== undefined ? `persisted_count=${scope.persisted_count}` : '',
      scope.persist_failed_count !== undefined ? `persist_failed_count=${scope.persist_failed_count}` : '',
      scope.raw_fetched_count !== undefined ? `raw_fetched_count=${scope.raw_fetched_count}` : '',
      scope.mapped_count !== undefined ? `mapped_count=${scope.mapped_count}` : ''
    ].filter(Boolean)
    const signalLines = [
      '',
      `错误信号`,
      `high_signal=${signal ? signal.label : '-'}`,
      `signal_value=${signal ? signal.value : '-'}`,
      `failed_scopes=${scope.failed_scopes?.length ? scope.failed_scopes.join(', ') : '-'}`,
      `query_failed_types=${scope.query_failed_types?.length ? scope.query_failed_types.join(', ') : '-'}`,
      `conversion_failed_types=${scope.conversion_failed_types?.length ? scope.conversion_failed_types.join(', ') : '-'}`,
      `enrichment_failed_types=${scope.enrichment_failed_types?.length ? scope.enrichment_failed_types.join(', ') : '-'}`,
      `enrichment_warnings=${scope.enrichment_warnings?.length ? scope.enrichment_warnings.join(', ') : '-'}`,
      `enrichment_stage_error=${scope.enrichment_stage_error || '-'}`,
      `writeback_failed_count=${scope.writeback_failed_count ?? 0}`,
      `max_resources_reached=${scope.max_resources_reached ? 'true' : 'false'}`,
      `product_names_empty=${scope.product_names_empty ? 'true' : 'false'}`
    ]
    const actionLines = [
      '',
      `告警级别`,
      alertLevel,
      '',
      `根因候选`,
      rootCause,
      '',
      `下一步动作`,
      nextStep,
      '',
      `当前锚定`,
      signal ? `${signal.label}：${signal.value}` : '未选中具体信号，可点击下方标签进一步锚定。'
    ]
    return [...summaryLines, ...signalLines, ...actionLines].join('\n')
  })

  const selectedScopeTraceTags = computed(() => {
    const scope = selectedScopeTrace.value
    if (!scope) return []
    const tags: { label: string; value: string; color: string; key: string }[] = []
    if (scope.max_resources_reached) tags.push({ key: 'max', label: 'Max reached', value: '是', color: 'orange' })
    if ((scope.failed_scopes?.length ?? 0) > 0) tags.push({ key: 'failed', label: 'Failed scopes', value: scope.failed_scopes!.join(', '), color: 'gray' })
    if ((scope.query_failed_types?.length ?? 0) > 0) tags.push({ key: 'query', label: 'Query failed', value: scope.query_failed_types!.join(', '), color: 'red' })
    if ((scope.conversion_failed_types?.length ?? 0) > 0) tags.push({ key: 'conversion', label: 'Conversion failed', value: scope.conversion_failed_types!.join(', '), color: 'red' })
    if ((scope.enrichment_failed_types?.length ?? 0) > 0) tags.push({ key: 'enrichment', label: 'Enrichment failed', value: scope.enrichment_failed_types!.join(', '), color: 'red' })
    if ((scope.enrichment_warnings?.length ?? 0) > 0) tags.push({ key: 'warnings', label: 'Enrichment warnings', value: scope.enrichment_warnings!.join(', '), color: 'orange' })
    if (scope.enrichment_stage_error) tags.push({ key: 'stage_error', label: 'Stage error', value: scope.enrichment_stage_error, color: 'red' })
    if ((scope.writeback_failed_count ?? 0) > 0) tags.push({ key: 'writeback', label: 'Writeback failed', value: String(scope.writeback_failed_count), color: 'red' })
    return tags
  })

  function syncStatusColor(status: string) {
    switch (status) {
      case 'success':
        return 'green'
      case 'partial':
        return 'orange'
      case 'failed':
        return 'red'
      default:
        return 'blue'
    }
  }

  function hasSyncWarning(batch?: assetApi.SyncBatch | null) {
    return Boolean(batch?.summary?.product_names_empty || batch?.summary?.max_resources_reached || (batch?.summary?.query_failed_types?.length ?? 0) > 0 || (batch?.summary?.conversion_failed_types?.length ?? 0) > 0)
  }

  function formatTs(ts?: number) {
    if (!ts) return '-'
    return new Date(ts * 1000).toLocaleString()
  }

  async function loadSyncBatches() {
    const seq = ++batchListReqSeq
    syncBatchesLoading.value = true
    try {
      const res = await assetApi.listSyncBatches({
        account_id: syncAccountId.value.trim() || undefined,
        page: syncPagination.current,
        page_size: syncPagination.pageSize
      })
      if (seq !== batchListReqSeq) return
      syncBatches.value = res.items ?? []
      syncPagination.total = res.total ?? 0
    } finally {
      if (seq === batchListReqSeq) {
        syncBatchesLoading.value = false
      }
    }
  }

  let syncPollingStopped = false
  onBeforeUnmount(() => {
    syncPollingStopped = true
  })

  async function runCloudSync() {
    const accountId = syncAccountId.value.trim()
    if (!accountId) {
      Message.warning('请填写接入账号 ID')
      return
    }
    syncLoading.value = true
    try {
      const running = await assetApi.triggerAssetSync(accountId)
      const batch = await assetApi.pollSyncBatch(running.batch_id, { shouldStop: () => syncPollingStopped })
      const notice = assetApi.getSyncBatchNotice(batch)
      Message[notice.type](notice.content)
      await loadSyncBatches()
      if (options?.onSyncComplete) {
        await options.onSyncComplete(batch)
      }
    } catch (e: unknown) {
      if (assetApi.isAssetSyncInProgressError(e)) {
        Message.warning('该账号正在同步，请稍后重试')
        return
      }
      if (e instanceof assetApi.SyncStillRunningError) {
        Message.info(e.message)
        return
      }
      if (e instanceof Error && e.message === 'polling cancelled') {
        return
      }
      Message.error(e instanceof Error ? e.message : '同步失败')
    } finally {
      syncLoading.value = false
    }
  }

  async function openSyncBatchDetail(batch: assetApi.SyncBatch) {
    const seq = ++batchDetailReqSeq
    // 打开新批次时重置 scope/signal 选择，避免残留上一批次的选中状态。
    selectedSyncScopeKey.value = ''
    selectedScopeSignalKey.value = ''
    syncBatchDetailVisible.value = true
    syncBatchDetail.value = batch
    syncBatchDetailLoading.value = true
    try {
      const detail = await assetApi.getSyncBatch(batch.batch_id)
      if (seq !== batchDetailReqSeq) return
      syncBatchDetail.value = detail
    } catch (e: unknown) {
      if (seq !== batchDetailReqSeq) return
      Message.error(e instanceof Error ? e.message : '加载批次详情失败')
    } finally {
      if (seq === batchDetailReqSeq) {
        syncBatchDetailLoading.value = false
      }
    }
  }

  function onSyncPageChange(page: number) {
    syncPagination.current = page
    loadSyncBatches()
  }

  function onSyncPageSizeChange(size: number) {
    syncPagination.pageSize = size
    syncPagination.current = 1
    loadSyncBatches()
  }

  return {
    syncAccountId,
    syncLoading,
    syncBatchesLoading,
    syncBatches,
    syncBatchDetailVisible,
    syncBatchDetailLoading,
    syncBatchDetail,
    selectedSyncScopeKey,
    selectedScopeSignalKey,
    tableScroll,
    syncPagination,
    syncBatchColumns,
    syncBatchScopeDiagnosticColumns,
    syncBatchMessageSummary,
    syncBatchScopeCards,
    syncBatchSignalTags,
    hasSyncWarning,
    selectedScopeTrace,
    selectedScopeTraceCards,
    selectedScopeTraceTags,
    selectedScopeTraceSnippet,
    scopeKey,
    toggleScopeByKey,
    openScopeTrace,
    openScopeSignal,
    copyScopeSnippet,
    syncStatusColor,
    formatTs,
    loadSyncBatches,
    runCloudSync,
    openSyncBatchDetail,
    onSyncPageChange,
    onSyncPageSizeChange
  }
}
