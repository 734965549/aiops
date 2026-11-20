/**
 * 聊天图片输入工具：格式校验、压缩与 data URL 生成。
 * 与后端 normalizeChatImages 的规则保持一致（ops/ai-contract.md §10.20）：
 * 最多 3 张，png/jpeg/webp，单张解码后不超过 2MB。
 */
export const MAX_CHAT_IMAGES = 3
export const MAX_CHAT_IMAGE_BYTES = 2 * 1024 * 1024
export const IMAGE_ONLY_PROMPT = '请分析这张图片。'

const SUPPORTED_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/webp']
const COMPRESS_THRESHOLD_BYTES = 300 * 1024
const MAX_IMAGE_SIDE = 1280
const JPEG_QUALITY = 0.85

export function isSupportedImageType(type: string): boolean {
  return SUPPORTED_IMAGE_TYPES.includes((type || '').toLowerCase())
}

/** data URL 解码后的字节数；非法输入返回 -1。 */
export function dataURLBytes(dataURL: string): number {
  const match = /^data:([^;]+);base64,(.*)$/.exec(dataURL.trim())
  if (!match) return -1
  const payload = match[2]
  const padding = payload.endsWith('==') ? 2 : payload.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor((payload.length * 3) / 4) - padding)
}

export function isSupportedImageDataURL(dataURL: string): boolean {
  const match = /^data:([^;]+);base64,/.exec(dataURL.trim())
  if (!match || !isSupportedImageType(match[1])) return false
  const bytes = dataURLBytes(dataURL)
  return bytes > 0 && bytes <= MAX_CHAT_IMAGE_BYTES
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new Error('read image failed'))
    reader.readAsDataURL(file)
  })
}

function loadImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('decode image failed'))
    image.src = dataURL
  })
}

/**
 * 压缩图片为 data URL：小图（<=300KB）直接读取原图；
 * 大图缩放到最长边 1280 后按 JPEG 0.85 重编码，控制进入 Prompt 的体积。
 */
export async function compressImageToDataURL(file: File): Promise<string> {
  const raw = await readFileAsDataURL(file)
  if (file.size <= COMPRESS_THRESHOLD_BYTES && isSupportedImageDataURL(raw)) {
    return raw
  }
  const image = await loadImage(raw)
  const scale = Math.min(1, MAX_IMAGE_SIDE / Math.max(image.naturalWidth, image.naturalHeight))
  const width = Math.max(1, Math.round(image.naturalWidth * scale))
  const height = Math.max(1, Math.round(image.naturalHeight * scale))
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas is not supported')
  ctx.drawImage(image, 0, 0, width, height)
  return canvas.toDataURL('image/jpeg', JPEG_QUALITY)
}
