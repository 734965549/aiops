/**
 * AI 助手工具拒绝原因的中文映射（ops/ai-contract.md §10.6.3）。
 *
 * 服务端返回的 reason 是稳定机器码；前端只做展示映射，未识别的原因原样透出，
 * 避免新增原因码时被静默吞掉。
 */
const TOOL_REASON_LABELS: Record<string, string> = {
  data_scope_denied: '告警不在当前账号的数据范围内',
  alert_not_found: '告警不存在或已被清理',
  alert_load_failed: '告警快照读取失败，已降级',
  'tool permission denied': '当前账号未开通该工具权限',
  'missing operation permission': '当前账号缺少对应操作权限',
  'authorization failed': '权限校验失败，请稍后重试',
  'tool gateway is not configured': '工具网关未配置',
  'evidence_collect_failed': '指标证据查询失败，已降级',
  'empty evidence result': '指标证据为空'
}

export function toolReasonLabel(reason?: string): string {
  const key = (reason ?? '').trim()
  if (!key) return ''
  return TOOL_REASON_LABELS[key] ?? key
}