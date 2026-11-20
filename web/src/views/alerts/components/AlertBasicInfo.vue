<template>
  <div class="alert-basic-info">
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
        <strong class="semantic-headline">{{ semantic.headline }}</strong>
        <div class="semantic-namespace">
          {{ semantic.namespace }}<span v-if="semantic.metricName"> · {{ semantic.metricName }}</span>
        </div>
      </a-descriptions-item>
      <a-descriptions-item
        label="描述"
        :span="2"
      >
        <div>{{ semantic.description }}</div>
        <div
          v-if="semantic.suggestion"
          class="semantic-suggestion"
        >{{ semantic.suggestion }}</div>
      </a-descriptions-item>
      <a-descriptions-item label="环境">
        {{ alert.environment ? environmentText(alert.environment) : '未配置环境' }}
      </a-descriptions-item>
      <a-descriptions-item label="应用">
        <RouterLink
          v-if="applicationView.link"
          :to="applicationView.link"
          class="asset-reference"
        >
          {{ applicationView.name }}
        </RouterLink>
        <span
          v-else-if="applicationView.name"
          class="reference-state"
        >
          {{ applicationView.name }}
          <a-tag
            v-if="applicationView.unassociated"
            size="small"
          >未关联注册表</a-tag>
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
          v-if="applicationView.id"
          class="reference-id"
        >
          <code>{{ applicationView.id }}</code>
          <AlertCopyButton
            :value="applicationView.id"
            label="应用 ID"
          />
        </div>
      </a-descriptions-item>
      <a-descriptions-item label="资源">
        <RouterLink
          v-if="resourceView.link"
          :to="resourceView.link"
          class="asset-reference"
        >
          {{ resourceView.name }}
        </RouterLink>
        <span
          v-else
          class="reference-state"
        >
          <span>{{ resourceView.name }}</span>
          <a-tag
            v-if="resourceView.status === 'unregistered'"
            size="small"
            color="orange"
          >未注册资源</a-tag>
          <a-tag
            v-else-if="resourceView.status === 'ambiguous'"
            size="small"
            color="orange"
          >多个候选</a-tag>
          <a-tag
            v-else-if="resourceView.status === 'loading'"
            size="small"
          >解析中</a-tag>
        </span>
        <div class="reference-meta">
          <span>{{ resourceView.type || semantic.resourceKind }}</span>
          <a-tag
            v-if="resourceView.syncStatus === 'stale'"
            size="small"
            color="orange"
          >stale</a-tag>
        </div>
        <div
          v-if="resourceView.rawId"
          class="reference-id"
        >
          <code>{{ resourceView.rawId }}</code>
          <AlertCopyButton
            :value="resourceView.rawId"
            label="资源 ID"
          />
        </div>
        <div
          v-if="resolutionError"
          class="resolution-note"
        >{{ resolutionError }}</div>
      </a-descriptions-item>
      <a-descriptions-item label="首次触发">{{ formatTime(alert.first_seen_at) }}</a-descriptions-item>
      <a-descriptions-item label="最近更新">{{ formatTime(alert.last_seen_at) }}</a-descriptions-item>
    </a-descriptions>

    <details class="raw-alert-content">
      <summary>原始摘要与描述</summary>
      <dl>
        <div>
          <dt>原始摘要</dt>
          <dd>{{ alert.summary || '未提供' }}</dd>
        </div>
        <div>
          <dt>原始描述</dt>
          <dd>{{ alert.description || '未提供' }}</dd>
        </div>
      </dl>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Alert } from '@/api/alert'
import { getAssetResourceResolution, type AssetResourceResolution, type ResolvedAssetResource } from '@/api/asset-topology'
import AlertCopyButton from './AlertCopyButton.vue'
import { buildAlertSemantics } from '../composables/alertSemantics'
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

const props = defineProps<{
  alert: Alert
}>()

const emit = defineEmits<{
  'check-match-rules': []
}>()

const semantic = computed(() => buildAlertSemantics(props.alert))
const resolution = ref<AssetResourceResolution | null>(null)
const resolving = ref(false)
const resolutionError = ref('')
let resolutionVersion = 0

const cloudResourceId = computed(() => {
  const labels = props.alert.labels || {}
  return labels.cloud_resource_id || labels.postgresql_cluster_id || labels.rds_cluster_id || ''
})

const matchedResource = computed<ResolvedAssetResource | null>(() => {
  return resolution.value?.status === 'matched' ? resolution.value.candidates[0] || null : null
})

const applicationView = computed(() => {
  if (props.alert.application_id) {
    return {
      id: props.alert.application_id,
      name: props.alert.application_name || '未命名应用',
      link: assetLink(props.alert.application_id),
      unassociated: false
    }
  }
  const resolved = matchedResource.value
  if (resolved?.application_id) {
    return {
      id: resolved.application_id,
      name: resolved.application_name || '未命名应用',
      link: assetLink(resolved.application_id),
      unassociated: false
    }
  }
  if (props.alert.application_name) {
    return { id: '', name: props.alert.application_name, link: null, unassociated: true }
  }
  return { id: '', name: '', link: null, unassociated: false }
})

const resourceView = computed(() => {
  const resolved = matchedResource.value
  if (resolved) {
    return {
      name: resolved.name,
      type: resourceTypeText(resolved.cloud_resource_type || resolved.resource_type),
      rawId: cloudResourceId.value || resolved.resource_id,
      link: assetLink(resolved.application_id, resolved.resource_id),
      status: 'matched',
      syncStatus: resolved.sync_status || ''
    }
  }
  if (props.alert.resource_id) {
    return {
      name: props.alert.resource_name || '未命名资源',
      type: resourceTypeText(props.alert.resource_type),
      rawId: props.alert.resource_id,
      link: assetLink(props.alert.application_id, props.alert.resource_id),
      status: 'registered',
      syncStatus: ''
    }
  }
  if (resolution.value?.status === 'ambiguous') {
    return {
      name: `发现 ${resolution.value.total} 个候选资源，请确认后选择`,
      type: semantic.value.resourceKind,
      rawId: cloudResourceId.value,
      link: null,
      status: 'ambiguous',
      syncStatus: ''
    }
  }
  if (resolving.value) {
    return {
      name: '正在解析资源名称…', type: semantic.value.resourceKind, rawId: cloudResourceId.value,
      link: null, status: 'loading', syncStatus: ''
    }
  }
  if (resolution.value?.status === 'unregistered' || cloudResourceId.value) {
    return {
      name: props.alert.resource_name || `未同步到 CMDB 的 ${semantic.value.resourceKind}`,
      type: semantic.value.resourceKind,
      rawId: cloudResourceId.value,
      link: null,
      status: 'unregistered',
      syncStatus: ''
    }
  }
  return {
    name: props.alert.resource_name || '未提供资源标识',
    type: resourceTypeText(props.alert.resource_type),
    rawId: '',
    link: null,
    status: props.alert.resource_name ? 'unregistered' : 'missing',
    syncStatus: ''
  }
})

watch(
  () => [props.alert.id, cloudResourceId.value, props.alert.labels?.account_id, props.alert.labels?.region],
  async () => {
    const version = ++resolutionVersion
    resolution.value = null
    resolutionError.value = ''
    const accountId = props.alert.labels?.account_id || ''
    if (!cloudResourceId.value || !accountId) return
    resolving.value = true
    try {
      const result = await getAssetResourceResolution({
        cloud_resource_id: cloudResourceId.value,
        account_id: accountId,
        region: props.alert.labels?.region || undefined,
        cloud_resource_type: props.alert.labels?.resource_type || props.alert.resource_type || undefined
      })
      if (version === resolutionVersion) resolution.value = result
    } catch {
      if (version === resolutionVersion) resolutionError.value = '资源名称解析失败，已保留原始资源标识。'
    } finally {
      if (version === resolutionVersion) resolving.value = false
    }
  },
  { immediate: true }
)

function assetLink(applicationId?: string, resourceId?: string) {
  const query: Record<string, string> = {}
  if (applicationId) query.application_id = applicationId
  if (resourceId) query.resource_id = resourceId
  return { path: '/assets', query }
}
</script>

<style scoped>
.semantic-headline {
  display: inline-block;
  color: var(--color-text-1);
  font-size: 14px;
  font-weight: 600;
  line-height: 1.5;
}

.semantic-namespace,
.semantic-suggestion,
.resolution-note {
  margin-top: 4px;
  color: var(--color-text-2);
  font-size: 11px;
  line-height: 1.5;
}

.semantic-suggestion {
  padding-left: 8px;
  border-left: 2px solid var(--color-border-2);
}

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

.raw-alert-content {
  margin-top: 12px;
  color: var(--color-text-2);
  font-size: 12px;
}

.raw-alert-content summary {
  cursor: pointer;
  user-select: none;
}

.raw-alert-content dl {
  margin-top: 8px;
  padding: 8px 12px;
  border: 1px solid var(--color-border-2);
  border-radius: var(--radius-sm);
  background: var(--color-fill-1);
}

.raw-alert-content dl > div {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  gap: 8px;
  margin-top: 4px;
}

.raw-alert-content dd {
  margin: 0;
  overflow-wrap: anywhere;
}
</style>
