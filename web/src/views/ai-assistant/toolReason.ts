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
  'from and to must be provided together': 'from 与 to 必须同时提供',
  'invalid time range': '指标时间范围不合法',
  'time range exceeds 7 days; period must be at least 86400': '时间窗口超过 7 天，日聚合 period 需至少 86400',
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
  'alert rule query is not configured': '告警规则查询能力未配置',
  'empty alert rule result': '告警规则查询结果为空',
  'alert rules query failed': '告警规则查询失败，已降级',
  'alert rules are not supported by the provider': '当前观测 provider 不支持告警规则查询'
}

export function toolReasonLabel(reason?: string): string {
  const key = (reason ?? '').trim()
  if (!key) return ''
  return TOOL_REASON_LABELS[key] ?? key
}