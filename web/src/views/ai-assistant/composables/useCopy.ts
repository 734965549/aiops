import Message from '@arco-design/web-vue/es/message'

/**
 * 复制文本：优先 Clipboard API，非安全上下文/权限失败时回退 execCommand。
 * 只复制调用方传入的业务文本，不读取页面中选择区或隐藏敏感节点。
 */
export async function copyText(text: string, successMessage = '已复制'): Promise<void> {
  const value = (text ?? '').trim()
  if (!value) {
    Message.warning('没有可复制的内容')
    return
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      fallbackCopy(value)
    }
    Message.success(successMessage)
  } catch {
    try {
      fallbackCopy(value)
      Message.success(successMessage)
    } catch {
      Message.error('复制失败，请手动选择文本')
    }
  }
}

function fallbackCopy(text: string) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'readonly')
  textarea.style.position = 'fixed'
  textarea.style.top = '-1000px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  try {
    textarea.select()
    const copied = document.execCommand('copy')
    if (!copied) throw new Error('execCommand copy failed')
  } finally {
    document.body.removeChild(textarea)
  }
}