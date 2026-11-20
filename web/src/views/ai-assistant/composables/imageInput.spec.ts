import { describe, expect, it } from 'vitest'
import {
  dataURLBytes,
  IMAGE_ONLY_PROMPT,
  isSupportedImageDataURL,
  isSupportedImageType,
  MAX_CHAT_IMAGE_BYTES,
  MAX_CHAT_IMAGES
} from './imageInput'

const tinyPNG = 'data:image/png;base64,iVBORw0KGgo='

describe('imageInput helpers', () => {
  it('accepts png/jpeg/webp mime types only', () => {
    expect(isSupportedImageType('image/png')).toBe(true)
    expect(isSupportedImageType('image/jpeg')).toBe(true)
    expect(isSupportedImageType('image/webp')).toBe(true)
    expect(isSupportedImageType('IMAGE/PNG')).toBe(true)
    expect(isSupportedImageType('image/gif')).toBe(false)
    expect(isSupportedImageType('image/svg+xml')).toBe(false)
    expect(isSupportedImageType('')).toBe(false)
  })

  it('computes decoded bytes of a data url', () => {
    // PNG 文件头 8 字节。
    expect(dataURLBytes(tinyPNG)).toBe(8)
    expect(dataURLBytes('data:image/png;base64,')).toBe(0)
    expect(dataURLBytes('not-a-data-url')).toBe(-1)
  })

  it('validates image data urls against mime and size rules', () => {
    expect(isSupportedImageDataURL(tinyPNG)).toBe(true)
    expect(isSupportedImageDataURL('data:image/gif;base64,R0lGODlh')).toBe(false)
    expect(isSupportedImageDataURL('https://example.com/a.png')).toBe(false)
    expect(isSupportedImageDataURL('data:image/png;base64,')).toBe(false)
    const oversizePayload = 'A'.repeat(Math.ceil((MAX_CHAT_IMAGE_BYTES + 1) * 4 / 3) + 4)
    expect(isSupportedImageDataURL(`data:image/png;base64,${oversizePayload}`)).toBe(false)
  })

  it('keeps limits aligned with the backend contract', () => {
    expect(MAX_CHAT_IMAGES).toBe(3)
    expect(MAX_CHAT_IMAGE_BYTES).toBe(2 * 1024 * 1024)
    expect(IMAGE_ONLY_PROMPT.length).toBeGreaterThan(0)
  })
})
