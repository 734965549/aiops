// text/event-stream 增量解析与读取。
//
// 对话流式接口（ops/ai-contract.md §10.17）使用 POST + Bearer Token，
// 不能用浏览器原生 EventSource，因此这里基于 fetch 的 ReadableStream 自行解析。

/** 一条完整 SSE 帧（已合并多行 data）。 */
export interface SSEFrame {
  event: string
  data: string
}

/**
 * 增量解析 SSE：push 接收任意切片，返回已闭合的帧；未闭合的尾部留在缓冲区。
 * 兼容 \n / \r\n / \r 三种换行，忽略注释行与 id/retry 字段。
 */
export class SSEParser {
  private buffer = ''

  push(chunk: string): SSEFrame[] {
    // 先拼接再归一化，避免 \r\n 被切在两个分片之间。
    this.buffer = (this.buffer + chunk).replace(/\r\n?/g, '\n')
    const frames: SSEFrame[] = []
    let boundary = this.buffer.indexOf('\n\n')
    while (boundary >= 0) {
      const block = this.buffer.slice(0, boundary)
      this.buffer = this.buffer.slice(boundary + 2)
      const frame = parseSSEBlock(block)
      if (frame) frames.push(frame)
      boundary = this.buffer.indexOf('\n\n')
    }
    return frames
  }

  /** 读取结束时兜底：服务端最后一帧缺少空行时仍能解析。 */
  flush(): SSEFrame[] {
    const rest = this.buffer
    this.buffer = ''
    if (!rest.trim()) return []
    const frame = parseSSEBlock(rest)
    return frame ? [frame] : []
  }
}

function parseSSEBlock(block: string): SSEFrame | null {
  const data: string[] = []
  let event = ''
  for (const rawLine of block.split('\n')) {
    const line = rawLine.replace(/\s+$/, '')
    if (!line || line.startsWith(':')) continue
    const colon = line.indexOf(':')
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'data') {
      data.push(value)
    } else if (field === 'event') {
      event = value
    }
  }
  if (!data.length) return null
  return { event, data: data.join('\n') }
}

/**
 * 逐帧消费 SSE 响应。onFrame 抛出的错误会中断读取并取消响应体，
 * 便于收到 error 事件后立即停止。
 */
export async function consumeSSE(
  response: Response,
  onFrame: (frame: SSEFrame) => void,
  signal?: AbortSignal
): Promise<void> {
  const body = response.body
  if (!body) throw new Error('SSE response has no body')
  const reader = body.getReader()
  const decoder = new TextDecoder()
  const parser = new SSEParser()
  try {
    for (;;) {
      if (signal?.aborted) throw abortError()
      const { value, done } = await reader.read()
      if (done) break
      const text = decoder.decode(value, { stream: true })
      for (const frame of parser.push(text)) onFrame(frame)
    }
    const tail = decoder.decode()
    if (tail) {
      for (const frame of parser.push(tail)) onFrame(frame)
    }
    for (const frame of parser.flush()) onFrame(frame)
  } catch (error) {
    await reader.cancel().catch(() => undefined)
    throw error
  } finally {
    reader.releaseLock()
  }
}

/** 判断错误是否来自主动取消（AbortController / fetch abort）。 */
export function isAbortError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const name = (error as { name?: string }).name
  return name === 'AbortError' || name === 'CanceledError'
}

function abortError(): Error {
  const error = new Error('the operation was aborted')
  error.name = 'AbortError'
  return error
}
