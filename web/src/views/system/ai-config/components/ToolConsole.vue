<template>
  <a-row
    :gutter="16"
    class="equal-row"
  >
    <a-col
      :xs="24"
      :lg="12"
    >
      <a-card title="调用参数">
        <a-alert
          type="info"
          :show-icon="true"
          class="console-alert"
        >
          联调工具专用：AI 助手与告警分析由服务端固定编排，用户无需手填工具编码。
        </a-alert>
        <a-form
          :model="form"
          layout="vertical"
          size="small"
        >
          <a-form-item
            label="Provider"
            required
          >
            <a-select
              v-model="form.provider_id"
              placeholder="选择 Provider"
              :options="providerOptions"
              allow-search
            />
          </a-form-item>
          <a-form-item
            label="工具编码"
            required
          >
            <a-input
              v-model="form.tool_code"
              placeholder="alarm.analyze"
            />
          </a-form-item>
          <a-form-item label="Resource">
            <a-input
              v-model="form.resource"
              placeholder="alarm"
            />
          </a-form-item>
          <a-form-item label="Action">
            <a-input
              v-model="form.action"
              placeholder="analyze"
            />
          </a-form-item>
          <a-form-item label="Payload (JSON)">
            <a-textarea
              v-model="payloadText"
              :auto-size="{ minRows: 6, maxRows: 12 }"
              placeholder="{&quot;alarm_id&quot;:&quot;a-123&quot;}"
            />
          </a-form-item>
          <a-form-item>
            <a-checkbox v-model="form.confirmed">
              已人工确认（require_confirm 类工具必填）
            </a-checkbox>
          </a-form-item>
          <a-button
            type="primary"
            :loading="invoking"
            @click="onInvoke"
          >
            调用
          </a-button>
        </a-form>
      </a-card>
    </a-col>
    <a-col
      :xs="24"
      :lg="12"
    >
      <a-card title="调用结果">
        <pre
          v-if="result"
          class="result-json"
        >{{ formattedResult }}</pre>
        <a-empty
          v-else
          description="提交后将在此展示 allowed / reason / data"
        />
      </a-card>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import { invokeTool, listProviders, type AIProvider, type InvokeToolResult } from '@/api/ai'

const providers = ref<AIProvider[]>([])
const invoking = ref(false)
const result = ref<InvokeToolResult | null>(null)
const payloadText = ref('{}')

const form = reactive({
  provider_id: '',
  tool_code: '',
  resource: '',
  action: '',
  confirmed: false
})

const providerOptions = computed(() =>
  providers.value.map((provider) => ({ label: `${provider.name} (${provider.id})`, value: provider.id }))
)

const formattedResult = computed(() => (result.value ? JSON.stringify(result.value, null, 2) : ''))

function parsePayload(): Record<string, unknown> | undefined {
  const text = payloadText.value.trim()
  if (!text) return undefined
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
    throw new Error('payload must be a JSON object')
  } catch {
    Message.error('Payload 必须是合法 JSON 对象')
    throw new Error('invalid payload')
  }
}

async function onInvoke() {
  if (!form.provider_id || !form.tool_code.trim()) {
    Message.warning('请选择 Provider 并填写工具编码')
    return
  }
  let payload: Record<string, unknown> | undefined
  try {
    payload = parsePayload()
  } catch {
    return
  }
  invoking.value = true
  result.value = null
  try {
    result.value = await invokeTool({
      provider_id: form.provider_id,
      tool_code: form.tool_code.trim(),
      resource: form.resource.trim() || undefined,
      action: form.action.trim() || undefined,
      confirmed: form.confirmed,
      payload
    })
    if (result.value && !result.value.allowed) {
      Message.warning(result.value.reason || '工具调用被拒绝（allowed=false）')
    }
  } catch {
    /* 错误已由拦截器提示。 */
  } finally {
    invoking.value = false
  }
}

onMounted(async () => {
  try {
    providers.value = await listProviders()
  } catch {
    /* 错误已由拦截器提示。 */
  }
})
</script>

<style scoped>
.console-alert {
  margin-bottom: 10px;
  font-size: 12px;
}

.result-json {
  margin: 0;
  padding: 12px;
  background: var(--color-fill-1);
  border-radius: var(--radius-sm);
  font-size: 12px;
  line-height: 1.5;
  overflow: auto;
}

.equal-row {
  flex: 1;
  min-height: 0;
}

.equal-row > .arco-col > .arco-card {
  min-height: 0;
}

:deep(.arco-card-body) {
  overflow: auto;
}

:deep(.arco-form-item) {
  margin-bottom: 10px;
}
</style>