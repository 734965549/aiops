<template>
  <a-row
    :gutter="16"
    class="registry-layout"
  >
    <a-col :span="10">
      <a-card
        title="应用"
        :bordered="false"
        class="assets-card assets-card-fixed"
      >
        <template #extra>
          <a-space>
            <a-button @click="emit('refresh-apps')">
              刷新
            </a-button>
            <a-button
              type="primary"
              @click="emit('create-app')"
            >
              新建应用
            </a-button>
          </a-space>
        </template>

        <a-table
          :columns="appColumns"
          :data="applications"
          :loading="appsLoading"
          row-key="id"
          :pagination="appPagination"
          :scroll="tableScroll"
          :bordered="false"
          :row-class="appRowClass"
          @page-change="(page: number) => emit('app-page-change', page)"
          @page-size-change="(size: number) => emit('app-page-size-change', size)"
          @row-click="(record: TableData) => emit('select-app', record)"
        >
          <template #environment="{ record }">
            {{ record.environment || '-' }}
          </template>
          <template #namespace="{ record }">
            <a-tooltip
              v-if="record.namespace"
              :content="record.namespace"
            >
              <span class="assets-text-ellipsis">{{ record.namespace }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #actions="{ record }">
            <a-space @click.stop>
              <a-button
                type="text"
                size="small"
                @click="emit('edit-app', record as Application)"
              >
                编辑
              </a-button>
              <a-popconfirm
                content="删除应用前需先清空其下所有资源，确定删除？"
                @ok="emit('delete-app', record as Application)"
              >
                <a-button
                  type="text"
                  size="small"
                  status="danger"
                >
                  删除
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </a-table>
      </a-card>
    </a-col>

    <a-col :span="14">
      <a-card
        :title="resourceCardTitle"
        :bordered="false"
        class="assets-card assets-card-fixed"
      >
        <template #extra>
          <a-space wrap>
            <a-input
              v-model="filterCloudResourceType"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="云类型"
              size="small"
              style="width: 96px"
              @press-enter="emit('apply-filters')"
              @clear="emit('apply-filters')"
            />
            <a-input
              v-model="filterRegion"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="Region"
              size="small"
              style="width: 110px"
              @press-enter="emit('apply-filters')"
              @clear="emit('apply-filters')"
            />
            <a-select
              v-model="filterSyncStatus"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="同步状态"
              size="small"
              style="width: 110px"
              @change="emit('apply-filters')"
              @clear="emit('apply-filters')"
            >
              <a-option value="active">
                active
              </a-option>
              <a-option value="stale">
                stale
              </a-option>
            </a-select>
            <a-button
              :disabled="!selectedAppId"
              size="small"
              @click="emit('apply-filters')"
            >
              筛选
            </a-button>
            <a-button
              :disabled="!selectedAppId"
              @click="emit('refresh-resources')"
            >
              刷新
            </a-button>
            <a-button
              type="primary"
              :disabled="!selectedAppId"
              @click="emit('create-resource')"
            >
              新建资源
            </a-button>
          </a-space>
        </template>

        <a-empty
          v-if="!selectedAppId"
          class="assets-empty"
          description="请先选择左侧应用"
        />
        <a-table
          v-else
          :columns="resourceColumns"
          :data="resources"
          :loading="resourcesLoading"
          row-key="id"
          :pagination="resourcePagination"
          :scroll="resourceTableScroll"
          :bordered="false"
          :row-class="resourceRowClass"
          @page-change="(page: number) => emit('resource-page-change', page)"
          @page-size-change="(size: number) => emit('resource-page-size-change', size)"
        >
          <template #source="{ record }">
            <a-tag
              v-if="(record as Resource).source === 'cloud_sync'"
              color="arcoblue"
              size="small"
            >
              云同步
            </a-tag>
            <a-tag
              v-else
              size="small"
            >
              手工
            </a-tag>
          </template>
          <template #cloud_resource_id="{ record }">
            <a-tooltip
              v-if="(record as Resource).cloud_resource_id"
              :content="(record as Resource).cloud_resource_id"
            >
              <span class="assets-text-ellipsis">{{ (record as Resource).cloud_resource_id }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #integration_account_id="{ record }">
            <a-tooltip
              v-if="(record as Resource).integration_account_id"
              :content="(record as Resource).integration_account_id"
            >
              <span class="assets-text-ellipsis">{{ (record as Resource).integration_account_id }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #cloud_resource_type="{ record }">
            {{ (record as Resource).cloud_resource_type || '-' }}
          </template>
          <template #region="{ record }">
            <a-tooltip
              v-if="(record as Resource).region"
              :content="(record as Resource).region"
            >
              <span class="assets-text-ellipsis">{{ (record as Resource).region }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #sync_status="{ record }">
            <a-tag
              v-if="(record as Resource).sync_status"
              :color="(record as Resource).sync_status === 'stale' ? 'orangered' : 'green'"
              size="small"
            >
              {{ (record as Resource).sync_status }}
            </a-tag>
            <span v-else>-</span>
          </template>
          <template #last_synced_at="{ record }">
            {{ formatTs((record as Resource).last_synced_at) }}
          </template>
          <template #sync_batch_id="{ record }">
            <a-tooltip
              v-if="(record as Resource).sync_batch_id"
              :content="(record as Resource).sync_batch_id"
            >
              <span class="assets-text-ellipsis">{{ (record as Resource).sync_batch_id }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #labels="{ record }">
            <a-popover
              v-if="labelEntries((record as Resource).labels).length"
              title="Labels"
            >
              <template #content>
                <div class="asset-label-popover">
                  <div
                    v-for="item in labelEntries((record as Resource).labels)"
                    :key="item.key"
                    class="asset-label-row"
                  >
                    <span class="asset-label-key">{{ item.key }}</span>
                    <span class="asset-label-value">{{ item.displayValue }}</span>
                  </div>
                </div>
              </template>
              <div class="asset-label-tags">
                <a-tag
                  v-for="item in labelPreviewEntries(record as Resource)"
                  :key="item.key"
                  size="small"
                >
                  {{ item.key }}={{ item.displayValue }}
                </a-tag>
                <a-tag
                  v-if="labelEntries((record as Resource).labels).length > 3"
                  size="small"
                >
                  +{{ labelEntries((record as Resource).labels).length - 3 }}
                </a-tag>
              </div>
            </a-popover>
            <span v-else>-</span>
          </template>
          <template #actions="{ record }">
            <a-space>
              <a-button
                type="text"
                size="small"
                @click="emit('edit-resource', record as Resource)"
              >
                编辑
              </a-button>
              <a-popconfirm
                content="删除后历史告警仍可能引用该资源 ID，确定删除？"
                @ok="emit('delete-resource', record as Resource)"
              >
                <a-button
                  type="text"
                  size="small"
                  status="danger"
                >
                  删除
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </a-table>
      </a-card>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { Application, Resource } from '@/api/asset'
import { labelEntries } from '../composables/assetUtils'

interface PaginationConfig {
  current: number
  pageSize: number
  total: number
  showTotal: boolean
  showPageSize: boolean
}

interface ResourceFilters {
  cloud_resource_type: string
  region: string
  sync_status: string
}

const props = defineProps<{
  applications: Application[]
  appsLoading: boolean
  appColumns: TableInstance['columns']
  appPagination: PaginationConfig
  tableScroll: Record<string, unknown>
  appRowClass: (record: TableData) => string
  selectedAppId: string
  resourceCardTitle: string
  resources: Resource[]
  resourcesLoading: boolean
  resourceColumns: TableInstance['columns']
  resourcePagination: PaginationConfig
  resourceTableScroll: Record<string, unknown>
  resourceRowClass: (record: TableData) => string
  resourceFilters: ResourceFilters
}>()

const emit = defineEmits<{
  'refresh-apps': []
  'create-app': []
  'edit-app': [app: Application]
  'delete-app': [app: Application]
  'refresh-resources': []
  'create-resource': []
  'edit-resource': [res: Resource]
  'delete-resource': [res: Resource]
  'apply-filters': []
  'select-app': [record: TableData]
  'app-page-change': [page: number]
  'app-page-size-change': [size: number]
  'resource-page-change': [page: number]
  'resource-page-size-change': [size: number]
  'update:resourceFilters': [filters: ResourceFilters]
}>()

const filterCloudResourceType = computed({
  get: () => props.resourceFilters.cloud_resource_type,
  set: (val: string) => emit('update:resourceFilters', { ...props.resourceFilters, cloud_resource_type: val })
})

const filterRegion = computed({
  get: () => props.resourceFilters.region,
  set: (val: string) => emit('update:resourceFilters', { ...props.resourceFilters, region: val })
})

const filterSyncStatus = computed({
  get: () => props.resourceFilters.sync_status,
  set: (val: string) => emit('update:resourceFilters', { ...props.resourceFilters, sync_status: val })
})

function formatTs(ts?: number) {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

function labelPreviewEntries(resource: Resource) {
  return labelEntries(resource.labels).slice(0, 3)
}
</script>
