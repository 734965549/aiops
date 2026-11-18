import { describe, expect, it } from 'vitest'
import { SSEParser, isAbortError } from './sse'

describe('SSEParser', () => {
  it('跨分片拼接 JSON 后产出完整帧', () => {
    const parser = new SSEParser()
    expect(parser.push('data: {"type":"stage"')).toEqual([])
    const frames = parser.push(',"stage":"thinking"}\n\n')
    expect(frames).toEqual([{ event: '', data: '{"type":"stage","stage":"thinking"}' }])
  })

  it('一次推入多帧，兼容 CRLF 与注释行', () => {
    const parser = new SSEParser()
    const frames = parser.push(': keep-alive\r\ndata: {"a":1}\r\n\r\ndata: {"b":2}\r\n\r\n')
    expect(frames.map((frame) => frame.data)).toEqual(['{"a":1}', '{"b":2}'])
  })

  it('合并多行 data 并保留 event 字段', () => {
    const parser = new SSEParser()
    const frames = parser.push('event: message\ndata: line1\ndata: line2\n\n')
    expect(frames).toEqual([{ event: 'message', data: 'line1\nline2' }])
  })

  it('flush 兜底解析缺少结尾空行的最后一帧', () => {
    const parser = new SSEParser()
    expect(parser.push('data: tail')).toEqual([])
    expect(parser.flush()).toEqual([{ event: '', data: 'tail' }])
    expect(parser.flush()).toEqual([])
  })

  it('忽略无 data 的帧', () => {
    const parser = new SSEParser()
    expect(parser.push('event: ping\n\n')).toEqual([])
  })
})

describe('isAbortError', () => {
  it('识别主动取消错误', () => {
    expect(isAbortError({ name: 'AbortError' })).toBe(true)
    expect(isAbortError({ name: 'CanceledError' })).toBe(true)
  })

  it('普通错误与空值不算取消', () => {
    expect(isAbortError(new Error('boom'))).toBe(false)
    expect(isAbortError(null)).toBe(false)
    expect(isAbortError('aborted')).toBe(false)
  })
})