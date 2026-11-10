// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { getSyncBatchNotice, type SyncBatch } from './asset'

function makeBatch(overrides: Partial<SyncBatch> = {}): SyncBatch {
  return {
    batch_id: 'sync-1',
    integration_account_id: 'acc-1',
    provider: 'huawei_cloud',
    status: 'failed',
    created_count: 0,
    updated_count: 0,
    completed_count: 0,
    stale_count: 0,
    failed_count: 1,
    started_at: 1700000000,
    created_at: 1700000000,
    updated_at: 1700000000,
    ...overrides
  }
}

describe('getSyncBatchNotice', () => {
  it('never renders undefined for batch counters', () => {
    // 回归用例：后端曾不返回 completed_count，导致提示出现「完成 undefined」。
    const notice = getSyncBatchNotice(makeBatch())
    expect(notice.content).not.toContain('undefined')
    expect(notice.content).toContain('完成 0')
  })

  it('renders counters for a failed batch and keeps the message detail', () => {
    const notice = getSyncBatchNotice(
      makeBatch({
        message: 'cn-south-1: no CES resource group matched (default_candidates=全部资源)'
      })
    )
    expect(notice.type).toBe('error')
    expect(notice.content).toContain('新建 0')
    expect(notice.content).toContain('失败 1')
    expect(notice.content).toContain('no CES resource group matched')
  })

  it('renders success counters without undefined', () => {
    const notice = getSyncBatchNotice(
      makeBatch({ status: 'success', created_count: 7, updated_count: 5, completed_count: 12, failed_count: 0 })
    )
    expect(notice.type).toBe('success')
    expect(notice.content).toContain('完成 12')
    expect(notice.content).not.toContain('undefined')
  })
})
