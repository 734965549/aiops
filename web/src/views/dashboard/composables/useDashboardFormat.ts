export function formatTime(ts?: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

export function severityLabel(s: string) {
  const map: Record<string, string> = { p0: 'P0', p1: 'P1', p2: 'P2', p3: 'P3', info: 'Info' }
  return map[s] || s
}

export function severityColor(s: string) {
  const map: Record<string, string> = {
    p0: 'red',
    p1: 'orangered',
    p2: 'orange',
    p3: 'gold',
    info: 'arcoblue'
  }
  return map[s] || 'gray'
}

export function executionStatusLabel(s: string) {
  const map: Record<string, string> = {
    pending_confirm: '待确认',
    pending_execute: '待执行',
    running: '执行中',
    success: '成功',
    failed: '失败',
    cancelled: '已取消'
  }
  return map[s] || s
}

export function executionStatusColor(s: string) {
  const map: Record<string, string> = {
    pending_confirm: 'orangered',
    pending_execute: 'orange',
    running: 'arcoblue',
    success: 'green',
    failed: 'red',
    cancelled: 'gray'
  }
  return map[s] || 'gray'
}

export function checkStatusLabel(s: string) {
  const map: Record<string, string> = { ok: '正常', down: '不可用', degraded: '降级' }
  return map[s] || s
}

export function checkStatusColor(s: string) {
  const map: Record<string, string> = { ok: 'green', down: 'red', degraded: 'orange' }
  return map[s] || 'gray'
}
