<template>
  <a-descriptions
    :column="2"
    bordered
    size="small"
    class="detail-desc"
  >
    <a-descriptions-item label="状态">
      <a-tag :color="statusColor(alert.status)">{{ statusLabel(alert.status) }}</a-tag>
    </a-descriptions-item>
    <a-descriptions-item label="级别">
      <a-tag :color="severityColor(alert.severity)">{{ severityLabel(alert.severity) }}</a-tag>
    </a-descriptions-item>
    <a-descriptions-item label="来源">
      <a-tooltip :content="`来源类型：${sourceTypeText(alert.source)}`">
        <span>{{ sourceLabel(alert) }}</span>
      </a-tooltip>
    </a-descriptions-item>
    <a-descriptions-item label="触发次数">{{ alert.occurrence_count }}</a-descriptions-item>
    <a-descriptions-item
      label="摘要"
      :span="2"
    >
      {{ alert.summary || '未提供摘要' }}
    </a-descriptions-item>
    <a-descriptions-item
      label="描述"
      :span="2"
    >
      {{ alert.description || '未提供描述' }}
    </a-descriptions-item>
    <a-descriptions-item label="环境">
      {{ environmentText(alert.environment) }}
    </a-descriptions-item>
    <a-descriptions-item label="应用">
      <RouterLink
        v-if="alert.application_id"
        :to="assetLink(alert.application_id)"
        class="asset-reference"
      >
        {{ alert.application_name || '未命名应用' }}
      </RouterLink>
      <span
        v-else-if="alert.application_name"
        class="reference-state"
      >
        {{ alert.application_name }} <a-tag size="small">未关联注册表</a-tag>
      </span>
      <span
        v-else
        class="reference-state"
      >
        <a-tag
          size="small"
          color="orange"
        >未匹配应用</a-tag>
        <a-link @click="emit('check-match-rules')">检查匹配规则</a-link>
      </span>
      <div
        v-if="alert.application_id"
        class="reference-id"
      >
        <code>{{ alert.application_id }}</code>
        <AlertCopyButton
          :value="alert.application_id"
          label="应用 ID"
        />
      </div>
    </a-descriptions-item>
    <a-descriptions-item label="资源">
      <RouterLink
        v-if="alert.resource_id"
        :to="assetLink(alert.application_id, alert.resource_id)"
        class="asset-reference"
      >
        {{ alert.resource_name || '未命名资源' }}
      </RouterLink>
      <span
        v-else
        class="reference-state"
      >
        <span>{{ alert.resource_name || '未提供资源标识' }}</span>
        <a-tag
          v-if="alert.resource_name"
          size="small"
          color="orange"
        >未注册资源</a-tag>
      </span>
      <div
        v-if="alert.resource_id || alert.resource_type"
        class="reference-meta"
      >
        <span v-if="alert.resource_type">{{ resourceTypeText(alert.resource_type) }}</span>
        <code v-if="alert.resource_id">{{ alert.resource_id }}</code>
        <AlertCopyButton
          v-if="alert.resource_id"
          :value="alert.resource_id"
          label="资源 ID"
        />
      </div>
    </a-descriptions-item>
    <a-descriptions-item label="首次触发">{{ formatTime(alert.first_seen_at) }}</a-descriptions-item>
    <a-descriptions-item label="最近更新">{{ formatTime(alert.last_seen_at) }}</a-descriptions-item>
  </a-descriptions>
</template>

<script setup lang="ts">
import type { Alert } from '@/api/alert'
import AlertCopyButton from './AlertCopyButton.vue'
import {
  environmentText,
  formatTime,
  resourceTypeText,
  severityColor,
  severityLabel,
  sourceLabel,
  sourceTypeText,
  statusColor,
  statusLabel
} from '../composables/alertDisplay'

defineProps<{
  alert: Alert
}>()

const emit = defineEmits<{
  'check-match-rules': []
}>()

function assetLink(applicationId?: string, resourceId?: string) {
  const query: Record<string, string> = {}
  if (applicationId) query.application_id = applicationId
  if (resourceId) query.resource_id = resourceId
  return { path: '/assets', query }
}

</script>

<style scoped>
.asset-reference {
  display: inline-block;
  max-width: 100%;
  overflow-wrap: anywhere;
  font-weight: 500;
}

.reference-state {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.reference-id,
.reference-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
  color: var(--color-text-2);
  font-size: 11px;
}

.reference-id code,
.reference-meta code {
  overflow-wrap: anywhere;
}
</style>
