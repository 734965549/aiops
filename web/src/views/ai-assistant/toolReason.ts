/**
 * AI 助手工具拒绝原因的中文映射（ops/ai-contract.md §10.6.3）。
 *
 * 服务端返回的 reason 是稳定机器码；前端只做展示映射，未识别的原因原样透出，
 * 避免新增原因码时被静默吞掉。
 */
const TOOL_REASON_LABELS: Record<string, string> = {
  data_scope_denied: '对象不在当前账号的数据范围内',
  alert_not_found: '告警不存在或已被清理',
  alert_load_failed: '告警快照读取失败，已降级',
  resource_not_found: '资源不存在或尚未同步到资产',
  resource_not_cloud_linked: '该资源未关联云账号，无法推导查询参数',
  resource_load_failed: '资源上下文读取失败，已降级',
  'empty resource context': '资源上下文为空',
  'account list failed': '集成账号查询失败，已降级',
  'empty account list result': '集成账号清单为空',
  'authorize integration account failed': '集成账号权限校验失败',
  'resource discovery is not configured': '资源发现能力未配置',
  'resource discovery failed': '资源发现失败，已降级',
  'empty resource list result': '资源查询结果为空',
  'invalid resources.list params': 'resources.list 参数格式不合法',
  'invalid resource_type': '云资源类型格式不合法',
  'account_id or region is too long': '账号或区域字段过长',
  'no access to resource': '资源不在当前数据范围内',
  'no access to integration account': '该集成账号不在当前数据范围内',
  account_resolver_unavailable: '账号解析能力不可用',
  'resolve integration account failed': '集成账号解析失败',
  'integration account not found': '集成账号不存在或已删除',
  no_resource: '缺少资源上下文',
  'tool permission denied': '当前账号未开通该工具权限',
  'missing operation permission': '当前账号缺少对应操作权限',
  'authorization failed': '权限校验失败，请稍后重试',
  'tool gateway is not configured': '工具网关未配置',
  'tool is not allowed in agent loop': '该工具不允许在 Agent Loop 中调用',
  'duplicate tool request: identical parameters already executed in this turn': '相同参数的查询已执行过，结果已在观测中',
  'no access to integration account for evidence': '该集成账号不在当前数据范围内',
  evidence_collect_failed: '指标证据查询失败，已降级',
  'empty evidence result': '指标证据为空',
  'metric query timeout': '指标查询超时，已降级',
  'metric evidence collector is not configured': '指标证据能力未配置',
  no_alert: '当前页面没有告警上下文',
  no_account: '缺少集成账号上下文',
  no_region: '缺少区域上下文',
  no_namespace: '缺少命名空间上下文',
  no_metric: '缺少指标名',
  no_dimensions: '缺少指标维度',
  'invalid metrics.query params': 'metrics.query 参数格式不合法',
  'invalid alert.rules.list params': 'alert.rules.list 参数格式不合法',
  'invalid alert.rules.health params': 'alert.rules.health 参数格式不合法',
  'invalid rds.backup.query params': 'rds.backup.query 参数格式不合法',
  'invalid rds backup instance_id': 'RDS 实例 ID 格式不合法',
  'backup days must be at most 30': '备份记录查询窗口最多 30 天',
  'backup limit must be at most 20': '单次备份记录最多 20 条',
  no_instance_id: '缺少 RDS 实例 ID，无法查询备份策略',
  'rds backup collector is not configured': 'RDS 备份查询能力未配置',
  'rds backup query failed': 'RDS 备份查询失败，已降级',
  'empty rds backup result': 'RDS 备份查询结果为空',
  'rds backup query requires ak_sk auth': '委托账号鉴权暂不支持 RDS 备份查询',
  'no access to integration account for rds backup': '该集成账号不在当前数据范围内',
  'project_id is required': '缺少项目 ID，无法查询 RDS 备份',
  'invalid aggregator': '聚合方式不支持，可选 avg/max/min/sum/variance',
  'from and to must be provided together': 'from 与 to 必须同时提供',
  'invalid time range': '指标时间范围不合法',
  'time range exceeds 7 days; period must be at least 3600': '时间窗口超过 7 天，period 需至少 3600（小时级）',
  'time range exceeds maximum 30 days': '时间窗口超过 30 天上限',
  'too many data points': '数据点超过 1440 上限，请缩小窗口或增大 period',
  'period below minimum 10 seconds': 'period 不能小于 10 秒',
  'invalid namespace': '命名空间格式不合法',
  'metric identity fields are too long': '账号、区域或指标字段过长',
  'too many dimensions': '维度数量超过 20 个',
  'dimensions contain empty key or value': '维度 key/value 不能为空',
  'dimension key or value is too long': '维度 key/value 过长',
  'keyword exceeds 100 characters': '关键词超过 100 字符',
  'limit must be at most 50': '单次规则查询最多 50 条',
  'limit must be at most 20': '单次规则体检最多 20 条',
  'alert rule query is not configured': '告警规则查询能力未配置',
  'empty alert rule result': '告警规则查询结果为空',
  'alert rules query failed': '告警规则查询失败，已降级',
  'alert rules are not supported by the provider': '当前观测 provider 不支持告警规则查询',
  'rule health collector is not configured': '规则体检能力未配置',
  'rule health check failed': '规则体检失败，已降级',
  'empty rule health result': '规则体检结果为空',
  'empty metric query result': '该规则指标查询结果为空',
  no_resource_scope: '规则未返回关联资源，无法确定取数维度',
  'rule health query timeout': '规则体检总预算耗尽，剩余规则未取数',
  'agency auth does not support metrics query': '委托账号鉴权暂不支持指标取数'
}

export function toolReasonLabel(reason?: string): string {
  const key = (reason ?? '').trim()
  if (!key) return ''
  return TOOL_REASON_LABELS[key] ?? key
}

const TOOL_LABELS: Record<string, string> = {
  'alarm.analyze': '读取告警快照',
  'metrics.query': '查询指标证据',
  'alert.rules.list': '查询告警规则',
  'alert.rules.health': '批量规则体检',
  'rds.backup.query': '查询 RDS 备份策略与记录',
  'accounts.list': '列出集成账号',
  'resource.analyze': '读取资源上下文',
  'resources.list': '发现云同步资源',
  'execution.tasks.propose': '生成执行计划权限'
}

/** 工具编码的中文名；未登记的编码原样透出。 */
export function toolLabel(code?: string): string {
  const key = (code ?? '').trim()
  if (!key) return ''
  return TOOL_LABELS[key] ?? key
}

const TOOL_STATUS_LABELS: Record<string, string> = {
  ok: '完成',
  skipped: '跳过',
  denied: '被拒绝',
  error: '失败'
}

/** 工具观测状态的展示文案。 */
export function toolStatusLabel(status?: string): string {
  const key = (status ?? '').trim()
  if (!key) return '完成'
  return TOOL_STATUS_LABELS[key] ?? key
}

const INCOMPLETE_REASON_LABELS: Record<string, string> = {
  stream_idle_timeout: '流式输出空闲超时',
  stream_first_chunk_timeout: '首块响应超时',
  stream_read_error: '与模型服务的连接中断',
  empty_model_response: '模型未返回最终正文',
  output_truncated: '模型输出达到长度上限',
  stream_interrupted: '流式输出中断'
}

/** 不完整回答的具体中断原因；未识别时原样透出，避免静默吞掉。 */
export function incompleteReasonLabel(reason?: string): string {
  const key = (reason ?? '').trim()
  if (!key) return ''
  return INCOMPLETE_REASON_LABELS[key] ?? key
}