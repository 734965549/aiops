<template>
  <div
    v-if="modelValue.length"
    class="image-preview-strip"
  >
    <div
      v-for="(img, index) in modelValue"
      :key="index"
      class="image-preview-item"
    >
      <img
        :src="img"
        :alt="`待发送图片 ${index + 1}`"
      >
      <button
        type="button"
        class="image-preview-remove"
        :disabled="sending"
        aria-label="移除图片"
        @click="removeImage(index)"
      >
        <IconClose :size="10" />
      </button>
    </div>
  </div>
  <input
    ref="fileInput"
    type="file"
    accept="image/png,image/jpeg,image/webp"
    multiple
    hidden
    @change="onFileChange"
  >
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { IconClose } from '@arco-design/web-vue/es/icon'
import Message from '@arco-design/web-vue/es/message'
import {
  compressImageToDataURL,
  isSupportedImageDataURL,
  isSupportedImageType,
  MAX_CHAT_IMAGES
} from '../composables/imageInput'

const props = defineProps<{
  modelValue: string[]
  sending: boolean
  visionEnabled?: boolean
  attaching: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string[]): void
  (e: 'update:attaching', value: boolean): void
}>()

const fileInput = ref<HTMLInputElement | null>(null)

async function addImageFiles(files: ArrayLike<File> | null) {
  if (!files || files.length === 0 || props.sending) return
  if (!props.visionEnabled) {
    Message.warning('当前模型未开启图片输入，请先在 AI 配置开启视觉能力')
    return
  }
  emit('update:attaching', true)
  try {
    for (const file of Array.from(files)) {
      if (props.modelValue.length >= MAX_CHAT_IMAGES) {
        Message.warning(`每条消息最多 ${MAX_CHAT_IMAGES} 张图片`)
        break
      }
      if (!isSupportedImageType(file.type)) {
        Message.warning('仅支持 PNG / JPEG / WebP 图片')
        continue
      }
      try {
        const dataURL = await compressImageToDataURL(file)
        if (!isSupportedImageDataURL(dataURL)) {
          Message.warning('图片压缩后仍超过 2MB，请缩小后再发送')
          continue
        }
        emit('update:modelValue', [...props.modelValue, dataURL])
      } catch {
        Message.warning('图片读取失败，请更换图片重试')
      }
    }
  } finally {
    emit('update:attaching', false)
  }
}

function onPickImages() {
  fileInput.value?.click()
}

function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  void addImageFiles(target.files)
  target.value = ''
}

function removeImage(index: number) {
  emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
}

defineExpose({ addImageFiles, onPickImages })
</script>

<style scoped>
.image-preview-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.image-preview-item {
  position: relative;
  width: 72px;
  height: 72px;
  overflow: hidden;
  border: 1px solid rgba(81, 102, 63, 0.2);
  border-radius: 6px;
}

.image-preview-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.image-preview-remove {
  position: absolute;
  top: 2px;
  right: 2px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  border-radius: 50%;
  color: #ffffff;
  background: rgba(0, 0, 0, 0.55);
  cursor: pointer;
}
</style>
