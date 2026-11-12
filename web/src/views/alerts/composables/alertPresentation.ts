/**
 * 告警详情「增强区」展示模型。
 *
 * 纯 TypeScript：只依赖 @/api/alert 的类型，不引入 Vue / Arco，便于单测。
 * CES 富化字段的权威来源：internal/alert/application/alert_sync_service.go
 * normalizeAlarms（labels / annotations 写入）与 alert_pull_port.go
 * AlarmCondition.Describe()（ces_condition 文本格式）。
 *
 * 设计要点：
 * - 白名单只用于取中文名 / 定排序 / 定归属，绝不当「是否显示」的过滤器；
 *   任何不在白名单内的 labels 键都按动态维度键兜底渲染，后端新增字段不丢信息。
 * - 去重优先级：触发条件卡 > labels > annotations，高优先级消费过的键低优先级静默丢弃。
 * - 富化失败（ces_rule_lookup）只影响规则详情展示，不改变告警数据 Complete 语义。
 */
import type { Alert } from '@/api/alert'

// ---------------------------------------------------------------------------
// 从 index.vue 迁出的静态映射（变量名保持不变，模板无需改动）
// ---------------------------------------------------------------------------

export const statusOptions = [
  { label: '新建', value: 'new' },
  { label: '已认领', value: 'acknowledged' },
  { label: '处理中', value: 'processing' },
  { label: '已恢复', value: 'recovered' },
  { label: '已关闭', value: 'closed' },
  { label: '已静默', value: 'silenced' }
]

export const severityOptions = [
  { label: 'P0', value: 'p0' },
  { label: 'P1', value: 'p1' },
  { label: 'P2', value: 'p2' },
  { label: 'P3', value: 'p3' },
  { label: 'Info', value: 'info' }
]

export const columns = [
  { title: '告警名称', dataIndex: 'name', ellipsis: true },
  { title: '级别', slotName: 'severity', width: 80 },
  { title: '状态', slotName: 'status', width: 100 },
  { title: '来源', slotName: 'source', width: 140, ellipsis: true },
  { title: '环境', dataIndex: 'environment', width: 80 },
  { title: '次数', dataIndex: 'occurrence_count', width: 80, align: 'right' as const },
  { title: '最近更新', slotName: 'last_seen_at', width: 170 },
  { title: '操作', slotName: 'actions', width: 80 }
]

export function eventTypeLabel(t: string) {
  const map: Record<string, string> = {
    triggered: '首次触发',
    updated: '重复触发',
    recovered: '已恢复',
    acknowledged: '认领',
    assigned: '转派',
    processing_started: '开始处理',
    closed: '关闭',
    silenced: '静默',
    unsilenced: '取消静默',
    commented: '备注',
    ai_analysis_requested: 'AI 分析',
    execution_created: '创建执行任务',
    execution_started: '开始执行',
    execution_finished: '执行完成'
  }
  return map[t] || t
}

// ---------------------------------------------------------------------------
// 字段契约
// ---------------------------------------------------------------------------

/** 已知 labels 键：仅用于中文名 / 排序 / 归属，不用于过滤显示。 */
export const KNOWN_LABEL_KEYS: ReadonlySet<string> = new Set([
  'account_id',
  'region',
  'project_id',
  'record_id',
  'alarm_id',
  'ces_namespace',
  'cloud_resource_id',
  'resource_type',
  'rule_name',
  'metric_name'
])

const LABEL_CN: Record<string, string> = {
  cloud_resource_id: '云资源 ID',
  resource_type: '资源类型',
  region: '区域',
  project_id: '项目 ID',
  account_id: '云账号 ID',
  ces_namespace: '指标命名空间',
  rule_name: '规则名称',
  alarm_id: '云规则 ID',
  record_id: '告警流水 ID',
  metric_name: '指标名',
  instance_id: '实例 ID',
  publicip_id: '公网 IP ID',
  dim_name: '维度名',
  bandwidth_id: '带宽 ID',
  namespace: '命名空间',
  vpc_id: 'VPC ID',
  bucket_name: '桶名称',
  cluster_id: '集群 ID',
  loadbalancer_id: '负载均衡器 ID',
  node_id: '节点 ID',
  lbaas_instance_id: 'LB 实例 ID',
  disk_name: '磁盘名',
  rds_instance_id: 'RDS 实例 ID',
  dcs_instance_id: 'DCS 实例 ID',
  dms_instance_id: 'DMS 实例 ID',
  pod: 'Pod',
  container: '容器'
}

/** 前 5 项位置恒定，便于跨告警比对。 */
const FIXED_FIRST = ['cloud_resource_id', 'resource_type', 'region', 'project_id', 'account_id']
/** 维度键之后的固定尾部（alarm_id / record_id 默认落在折叠区）。 */
const FIXED_TAIL = ['ces_namespace', 'rule_name', 'alarm_id', 'record_id']

const MONO_KEYS: ReadonlySet<string> = new Set([
  'cloud_resource_id', 'project_id', 'account_id', 'alarm_id', 'record_id', 'ces_namespace'
])

/** 可复制键；动态维度键值在运行时统一追加。 */
const COPY_KEYS: ReadonlySet<string> = new Set([
  'cloud_resource_id', 'project_id', 'account_id', 'alarm_id', 'record_id', 'ces_namespace'
])

/** 只用于展示防御，命中即掩码，明文不进入 DOM / 剪贴板。 */
const SENSITIVE_KEY_RE = /secret|token|password|api_key|credential|ak_sk/i

/** 被条件卡 / 控制键消费、不作为 annotations KV 展示的键。 */
const ANNO_STANDARD_KEYS: ReadonlySet<string> = new Set([
  'ces_rule_description', 'ces_rule_type', 'ces_level'
])

const CES_LEVEL_CN: Record<string, string> = {
  '1': '紧急',
  '2': '重要',
  '3': '次要',
  '4': '提示'
}

const FILTER_CN: Record<string, string> = {
  average: '平均值',
  max: '最大值',
  min: '最小值',
  sum: '求和',
  variance: '方差'
}

const OP_TEXT: Record<string, string> = {
  '>': '大于',
  '>=': '大于等于',
  '<': '小于',
  '<=': '小于等于',
  '=': '等于',
  '!=': '不等于'
}

export const OPS = ['>=', '<=', '!=', '>', '<', '='] as const

// 严格全量匹配：op + 数值阈值 + 单位 + 可选括号窗口；任何异常形状整体降级。
const CONDITION_RE = /^(>=|<=|!=|>|<|=)\s+(-?\d+(?:\.\d+)?)([^\s(]*)\s*(?:\(([^)]*)\))?\s*$/

export const COLLAPSE_THRESHOLD = 12
export const COLLAPSE_VISIBLE = 8

export interface ParsedCondition {
  ok: boolean
  raw: string
  op: string
  opText: string
  threshold: string
  unit: string
  filter: string
  period: string
  count: string
  /** 「平均值 · 300 秒 · 连续 3 次」，只拼存在的片段；全空为空串。 */
  windowText: string
}

/** 解析 Describe() 输出，失败时 ok=false，调用方必须整体展示原始串。 */
export function parseCondition(raw: string): ParsedCondition {
  const fallback: ParsedCondition = {
    ok: false,
    raw,
    op: '',
    opText: '',
    threshold: '',
    unit: '',
    filter: '',
    period: '',
    count: '',
    windowText: ''
  }
  if (typeof raw !== 'string' || !raw) return fallback
  const matched = CONDITION_RE.exec(raw)
  if (!matched) return fallback

  const [, op, threshold, unit, windowRaw = ''] = matched
  const parts = windowRaw.split(',').map((part) => part.trim()).filter(Boolean)
  const filter = parts[0] ?? ''
  const period = /(\d+)\s*s\b/.exec(windowRaw)?.[1] ?? ''
  const count = /x\s*(\d+)/.exec(windowRaw)?.[1] ?? ''
  const segments: string[] = []
  if (filter) segments.push(FILTER_CN[filter.toLowerCase()] ?? filter)
  if (period) segments.push(`${period} 秒`)
  if (count) segments.push(`连续 ${count} 次`)

  return {
    ok: true,
    raw,
    op,
    opText: OP_TEXT[op] ?? op,
    threshold,
    unit,
    filter,
    period,
    count,
    windowText: segments.join(' · ')
  }
}

// ---------------------------------------------------------------------------
// View model
// ---------------------------------------------------------------------------

export type EnhancementAlert = Pick<Alert, 'labels' | 'annotations' | 'source'>
export type ConditionState = 'A' | 'A-raw' | 'B' | 'C' | 'D'

export interface LabelItem {
  key: string
  label: string
  value: string
  mono: boolean
  copyable: boolean
  hint: string
  sensitive: boolean
}

export interface AnnotationKV {
  key: string
  label: string
  value: string
  hint: string
}

export interface ConditionVM {
  state: ConditionState
  headLabel: string
  metricName: string
  showMetric: boolean
  showConclusion: boolean
  /** 态 A'：body 整体展示原文（即使原文为空串也保留 .cond-raw）。 */
  showRaw: boolean
  parsed: ParsedCondition
  /** 态 A'：body 整体展示的原文。 */
  rawText: string
  foot: string
  /** 态 D：只在 foot 弱化展示收到的原始条件。 */
  rawFoot: string
  ariaLabel: string
}

export interface AlertPresentationVM {
  isCES: boolean
  hasEnhancement: boolean
  showRegion: boolean
  showConditionCard: boolean
  showLabelsPanel: boolean
  showAnnotations: boolean
  labels: LabelItem[]
  labelCount: number
  needsCollapse: boolean
  dimOnlyNote: boolean
  condition: ConditionVM
  description: string
  ruleTypeText: string
  ruleTypeDim: boolean
  levelCode: string
  levelSuffix: string
  mutedNote: boolean
  lookupValue: string
  annotationKVs: AnnotationKV[]
}

function asRecord(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const out: Record<string, string> = {}
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    out[key] = typeof item === 'string' ? item : String(item ?? '')
  }
  return out
}

/** 复合维度键按 `,` 拆段逐段查中文映射，未命中的段保留原样。值永不拆分。 */
function dimensionLabel(key: string): string {
  if (LABEL_CN[key]) return LABEL_CN[key]
  if (key.includes(',')) {
    return key
      .split(',')
      .map((part) => {
        const trimmed = part.trim()
        return LABEL_CN[trimmed] ?? trimmed
      })
      .join(' / ')
  }
  return key
}

function buildLabelItem(key: string, value: string): LabelItem {
  const dynamic = !KNOWN_LABEL_KEYS.has(key)
  const sensitive = SENSITIVE_KEY_RE.test(key)
  const knownLabel = LABEL_CN[key]
  return {
    key,
    label: dynamic ? dimensionLabel(key) : knownLabel || key,
    value: sensitive ? '****' : value,
    // 未命中完整中文映射（含复合键）时给出「· 维度」提示并等宽完整换行
    hint: dynamic && !knownLabel ? '· 维度' : '',
    mono: dynamic || MONO_KEYS.has(key),
    copyable: !sensitive && (dynamic || COPY_KEYS.has(key)),
    sensitive
  }
}

const EMPTY_PARSED: ParsedCondition = {
  ok: false,
  raw: '',
  op: '',
  opText: '',
  threshold: '',
  unit: '',
  filter: '',
  period: '',
  count: '',
  windowText: ''
}

function buildCondition(
  conditionRaw: string | null,
  metricName: string,
  ruleType: string,
  ruleLookup: string
): ConditionVM {
  // 态 B/C/D 完全不调用解析器，避免异常条件串被部分解析。
  // null = 键不存在；'' = 键存在但内容为空，仍按「原文降级」处理。
  const base: ConditionVM = {
    state: 'C',
    headLabel: '触发条件',
    metricName,
    showMetric: !!metricName,
    showConclusion: false,
    showRaw: false,
    parsed: EMPTY_PARSED,
    rawText: '',
    foot: '',
    rawFoot: '',
    ariaLabel: ''
  }

  // 优先级不可换：富化失败 > 事件规则 > 字段缺失 > 解析成功 > 原文降级 > 字段缺失。
  if (ruleLookup) {
    return {
      ...base,
      state: 'D',
      headLabel: '指标信息未获取',
      foot: '未获取到触发条件表达式。',
      rawFoot: conditionRaw ?? ''
    }
  }
  if (/^EVENT/i.test(ruleType)) {
    return {
      ...base,
      state: 'B',
      headLabel: '事件触发',
      showMetric: false,
      foot: '事件类规则按云端事件触发，不含指标与阈值条件。'
    }
  }
  if (conditionRaw === null) {
    // 键缺失：完全没有条件表达式 vs. 部分规则字段未返回。
    return {
      ...base,
      foot: !metricName && !ruleType
        ? '未收到触发条件表达式，已按可用字段展示。'
        : '部分规则字段未返回，已按可用信息展示。'
    }
  }
  const parsed = parseCondition(conditionRaw)
  if (parsed.ok) {
    const tail = parsed.windowText ? ` ${parsed.windowText}` : ''
    return {
      ...base,
      state: 'A',
      headLabel: metricName ? '' : '触发条件',
      showConclusion: true,
      parsed,
      ariaLabel: `触发条件：${metricName} ${parsed.opText} ${parsed.threshold}${parsed.unit}${tail}`
    }
  }
  return {
    ...base,
    state: 'A-raw',
    headLabel: metricName ? '' : '触发条件',
    showRaw: true,
    rawText: conditionRaw,
    foot: '条件按云端原文展示。'
  }
}

/**
 * 构建增强区 view model。
 * 所有区块独立判定；labels={} && annotations={} 时三区块全部静默缺席。
 */
export function buildAlertPresentation(alert: EnhancementAlert): AlertPresentationVM {
  const labels = asRecord(alert.labels)
  const annotations = asRecord(alert.annotations)
  const labelKeys = Object.keys(labels)
  const annoKeys = Object.keys(annotations)
  const hasEnhancement = labelKeys.length > 0 || annoKeys.length > 0

  const isCES =
    /ces/i.test(alert.source ?? '') ||
    annoKeys.some((key) => key.startsWith('ces_')) ||
    labelKeys.some((key) => key === 'record_id' || key === 'alarm_id' || key === 'ces_namespace')

  const showConditionCard = isCES && hasEnhancement
  // 条件卡不渲染（非 CES）时不得吞掉字段，避免信息丢失。
  const consumeConditionKeys = showConditionCard

  const metricName = labels.metric_name?.trim() || annotations.ces_metric_name?.trim() || ''
  const hasConditionKey = Object.prototype.hasOwnProperty.call(annotations, 'ces_condition')
  const conditionRaw: string | null = hasConditionKey ? annotations.ces_condition : null
  const ruleType = annotations.ces_rule_type ?? ''
  const ruleLookup = annotations.ces_rule_lookup ?? ''
  const description = annotations.ces_rule_description ?? ''

  // --- labels：合并 annotations.ces_namespace，去重后按固定顺序 + 字典序排列 ---
  const mergedLabels: Record<string, string> = { ...labels }
  if (mergedLabels.ces_namespace === undefined && annotations.ces_namespace !== undefined) {
    mergedLabels.ces_namespace = annotations.ces_namespace
  }
  if (consumeConditionKeys) delete mergedLabels.metric_name

  const allKeys = Object.keys(mergedLabels)
  const dynamicKeys = allKeys.filter((key) => !KNOWN_LABEL_KEYS.has(key)).sort()
  const first = FIXED_FIRST.filter((key) => allKeys.includes(key))
  const tail = FIXED_TAIL.filter((key) => allKeys.includes(key))
  const consumed = new Set([...first, ...dynamicKeys, ...tail])
  const leftovers = allKeys.filter((key) => !consumed.has(key)).sort()
  const orderedKeys = [...first, ...dynamicKeys, ...tail, ...leftovers]

  const labelItems = orderedKeys.map((key) => buildLabelItem(key, mergedLabels[key]))
  const labelCount = labelItems.length
  const dimOnlyNote = dynamicKeys.length === 1 && dynamicKeys[0] === 'dim_name'

  // --- annotations：标准字段特殊呈现，其余键原样兜底 ---
  const annoConsumed = new Set<string>(['ces_namespace', 'ces_rule_lookup'])
  if (consumeConditionKeys) {
    annoConsumed.add('ces_condition')
    annoConsumed.add('ces_metric_name')
  }
  const annotationKVs: AnnotationKV[] = annoKeys
    .filter((key) => !annoConsumed.has(key) && !ANNO_STANDARD_KEYS.has(key))
    .sort()
    .map((key) => ({
      key,
      label: key,
      value: SENSITIVE_KEY_RE.test(key) ? '****' : annotations[key],
      hint: '· 注解'
    }))

  const ruleTypeMapped = /^MULTI_INSTANCE$/i.test(ruleType)
    ? '多实例规则'
    : /^EVENT/i.test(ruleType)
      ? '事件规则'
      : ''
  const ruleTypeText = ruleTypeMapped || ruleType
  const ruleTypeDim = !!ruleType && !ruleTypeMapped

  const levelCode = annotations.ces_level ?? ''
  const levelCN = CES_LEVEL_CN[levelCode]
  const levelSuffix = levelCode
    ? `${levelCN ? `（云端：${levelCN}）` : '（未在默认映射内）'}· 平台定级以基础信息「级别」为准`
    : ''

  const mutedNote = !!ruleLookup
  const showAnnotations = !!(
    description ||
    ruleTypeText ||
    levelCode ||
    mutedNote ||
    annotationKVs.length > 0
  )

  return {
    isCES,
    hasEnhancement,
    showRegion: hasEnhancement,
    showConditionCard,
    showLabelsPanel: labelCount > 0,
    showAnnotations,
    labels: labelItems,
    labelCount,
    needsCollapse: labelCount > COLLAPSE_THRESHOLD,
    dimOnlyNote,
    condition: buildCondition(conditionRaw, metricName, ruleType, ruleLookup),
    description,
    ruleTypeText,
    ruleTypeDim,
    levelCode,
    levelSuffix,
    mutedNote,
    lookupValue: ruleLookup,
    annotationKVs
  }
}
