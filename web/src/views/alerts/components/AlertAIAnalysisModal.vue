<template>
  <a-modal
    v-model:visible="visible"
    title="AI 告警分析"
    :width="640"
    :ok-loading="loading"
    @before-ok="runAnalysis"
    @cancel="reset"
  >
    <a-form
      :model="form"
      layout="vertical"
    >
      <a-form-item label="时间范围">
        <a-input
          v-model="form.time_range"
          placeholder="30m"
        />
      </a-form-item>
      <a-form-item>
        <a-space direction="vertical">
          <a-checkbox v-model="form.include_logs">
            包含日志
          </a-checkbox>
          <a-checkbox v-model="form.include_metrics">
            包含指标
          </a-checkbox>
          <a-checkbox v-model="form.include_changes">
            包含变更
          </a-checkbox>
        </a-space>
      </a-form-item>
    </a-form>
    <AlertAIAnalysisResult
      v-if="result"
      :result="result"
    />
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import { analyzeAlert, type AnalyzeAlertResult } from '@/api/ai'
import * as alertApi from '@/api/alert'
import AlertAIAnalysisResult from './AlertAIAnalysisResult.vue'

const props = defineProps<{
  visible: boolean
  alertId: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'analyzed'): void
}>()

const visible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value)
})

const loading = ref(false)
const result = ref<AnalyzeAlertResult | null>(null)

const form = reactive({
  time_range: '30m',
  include_logs: false,
  include_metrics: false,
  include_changes: false
})

function reset() {
  result.value = null
}

/** 先写告警时间线与审计，再取分析结果展示。 */
async function runAnalysis(): Promise<boolean> {
  if (!props.alertId) return false
  loading.value = true
  try {
    await alertApi.requestAlertAIAnalysis(props.alertId, {
      time_range: form.time_range,
      include_logs: form.include_logs,
      include_metrics: form.include_metrics,
      include_changes: form.include_changes
    })
    result.value = await analyzeAlert({
      alert_id: props.alertId,
      time_range: form.time_range,
      include_logs: form.include_logs,
      include_metrics: form.include_metrics,
      include_changes: form.include_changes
    })
    Message.success('AI 分析完成')
    emit('analyzed')
    // 保留弹窗以展示分析结果，由用户手动关闭。
    return false
  } catch {
    return false
  } finally {
    loading.value = false
  }
}
</script>
