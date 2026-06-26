<template>
  <div class="runbooks-page">
    <a-card
      title="Runbook 预案"
      :bordered="false"
    >
      <template #extra>
        <a-button
          :loading="loading"
          @click="loadTemplates"
        >
          刷新
        </a-button>
      </template>

      <a-table
        :columns="columns"
        :data="templates"
        :loading="loading"
        row-key="template_id"
        :pagination="pagination"
        @page-change="onPageChange"
      >
        <template #enabled="{ record }">
          <a-switch
            :model-value="record.enabled"
            :loading="togglingId === record.template_id"
            @change="(v: string | number | boolean) => onToggleEnabled(record, Boolean(v))"
          />
        </template>
        <template #risk_level="{ record }">
          <a-tag>{{ record.risk_level }}</a-tag>
        </template>
        <template #actions="{ record }">
          <a-link @click="openDetail(record.template_id)">
            详情
          </a-link>
        </template>
      </a-table>
    </a-card>

    <a-drawer
      v-model:visible="detailVisible"
      :width="640"
      :title="detail?.template.name || '预案详情'"
      unmount-on-close
    >
      <template v-if="detail">
        <a-descriptions
          :column="1"
          bordered
          size="small"
        >
          <a-descriptions-item label="描述">
            {{ detail.template.description || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="匹配">
            告警 {{ detail.template.match_alert_name || '*' }} /
            资源 {{ detail.template.match_resource_type || '*' }} /
            环境 {{ detail.template.match_environment || '*' }}
          </a-descriptions-item>
        </a-descriptions>
        <a-table
          :columns="stepColumns"
          :data="detail.steps"
          :pagination="false"
          row-key="step_id"
          size="small"
          style="margin-top: 16px"
        />
      </template>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Message from '@arco-design/web-vue/es/message'
import * as runbookApi from '@/api/runbook'
import type { RunbookTemplate, RunbookTemplateDetail } from '@/api/runbook'

const templates = ref<RunbookTemplate[]>([])
const detail = ref<RunbookTemplateDetail | null>(null)
const loading = ref(false)
const detailVisible = ref(false)
const togglingId = ref('')

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showTotal: true
})

const columns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: '风险', slotName: 'risk_level', width: 90 },
  { title: '操作类型', dataIndex: 'operation_type', width: 100 },
  { title: '启用', slotName: 'enabled', width: 90 },
  { title: '匹配告警', dataIndex: 'match_alert_name', width: 120 },
  { title: '环境', dataIndex: 'match_environment', width: 80 },
  { title: '操作', slotName: 'actions', width: 80 }
]

const stepColumns = [
  { title: '#', dataIndex: 'step_order', width: 50 },
  { title: '步骤', dataIndex: 'name' },
  { title: '动作', dataIndex: 'action_type', width: 100 },
  { title: '风险', dataIndex: 'risk_level', width: 80 }
]

async function loadTemplates() {
  loading.value = true
  try {
    const res = await runbookApi.listRunbookTemplates({
      page: pagination.current,
      page_size: pagination.pageSize
    })
    templates.value = res.items
    pagination.total = res.total
  } finally {
    loading.value = false
  }
}

function onPageChange(page: number) {
  pagination.current = page
  loadTemplates()
}

async function openDetail(templateId: string) {
  detailVisible.value = true
  detail.value = await runbookApi.getRunbookTemplate(templateId)
}

async function onToggleEnabled(record: RunbookTemplate, enabled: boolean) {
  togglingId.value = record.template_id
  try {
    const full = await runbookApi.getRunbookTemplate(record.template_id)
    await runbookApi.updateRunbookTemplate(record.template_id, {
      ...full.template,
      enabled,
      steps: full.steps.map((s) => ({
        step_order: s.step_order,
        name: s.name,
        action_type: s.action_type,
        risk_level: s.risk_level,
        dry_run_supported: s.dry_run_supported,
        default_dry_run: s.default_dry_run,
        parameter_schema: s.parameter_schema,
        default_parameters: s.default_parameters,
        rollback_plan: s.rollback_plan,
        timeout_seconds: s.timeout_seconds
      }))
    })
    Message.success(enabled ? '已启用' : '已停用')
    await loadTemplates()
  } finally {
    togglingId.value = ''
  }
}

onMounted(loadTemplates)
</script>

<style scoped>
.runbooks-page {
  min-height: 100%;
}
</style>
