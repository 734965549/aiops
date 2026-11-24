import { describe, expect, it } from 'vitest'
import { incompleteReasonLabel } from './toolReason'

describe('incompleteReasonLabel', () => {
  it('maps known stream interruption reasons', () => {
    expect(incompleteReasonLabel('stream_idle_timeout')).toBe('流式输出空闲超时')
    expect(incompleteReasonLabel('stream_read_error')).toBe('与模型服务的连接中断')
    expect(incompleteReasonLabel('empty_model_response')).toBe('模型未返回最终正文')
    expect(incompleteReasonLabel('output_truncated')).toBe('模型输出达到长度上限')
  })

  it('passes through unknown reasons and empty values', () => {
    expect(incompleteReasonLabel('custom_reason')).toBe('custom_reason')
    expect(incompleteReasonLabel('')).toBe('')
    expect(incompleteReasonLabel()).toBe('')
  })
})