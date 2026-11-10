<template>
  <div class="registry-layout">
    <!-- 左栏：应用选择器。内容少、字段短，固定为窄栏，不再与右侧宽表平分宽度 -->
    <section class="registry-panel registry-panel--apps">
      <a-card
        title="应用"
        :bordered="false"
        class="assets-card"
      >
        <template #extra>
          <a-space :size="6">
            <a-button
              size="small"
              aria-label="刷新应用"
              @click="emit('refresh-apps')"
            >
              <template #icon>
                <IconRefresh />
              </template>
            </a-button>
            <a-button
              type="primary"
              size="small"
              @click="emit('create-app')"
            >
              新建应用
            </a-button>
          </a-space>
        </template>

        <a-table
          class="page-table page-table-fit"
          :style="appFitStyle"
          :table-layout-fixed="true"
          :columns="appColumns"
          :data="applications"
          :loading="appsLoading"
          row-key="id"
          :pagination="appPagination"
          :bordered="false"
          :row-class="appRowClass"
          @page-change="(page: number) => emit('app-page-change', page)"
          @page-size-change="(size: number) => emit('app-page-size-change', size)"
          @row-click="(record: TableData) => emit('select-app', record)"
        >
          <template #name="{ record }">
            <a-tooltip :content="appDetail(record as Application)">
              <div class="app-cell">
                <span class="app-cell-name">{{ record.name }}</span>
                <span class="app-cell-meta">{{ appMeta(record as Application) }}</span>
              </div>
            </a-tooltip>
          </template>
          <template #actions="{ record }">
            <a-space
              :size="0"
              @click.stop
            >
              <a-tooltip content="编辑应用">
                <a-button
                  type="text"
                  size="mini"
                  aria-label="编辑应用"
                  @click="emit('edit-app', record as Application)"
                >
                  <template #icon>
                    <IconEdit />
                  </template>
                </a-button>
              </a-tooltip>
              <a-popconfirm
                content="删除应用前需先清空其下所有资源，确定删除？"
                @ok="emit('delete-app', record as Application)"
              >
                <a-button
                  type="text"
                  size="mini"
                  status="danger"
                  aria-label="删除应用"
                >
                  <template #icon>
                    <IconDelete />
                  </template>
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </a-table>
      </a-card>
    </section>

    <!-- 右栏：资源清单。占满剩余宽度，筛选与操作分层，默认视图不产生横向滚动 -->
    <section class="registry-panel registry-panel--resources">
      <a-card
        :title="resourceCardTitle"
        :bordered="false"
        class="assets-card"
      >
        <template #extra>
          <a-space :size="6">
            <a-popover
              trigger="click"
              position="br"
            >
              <a-button size="small">
                <template #icon>
                  <IconSettings />
                </template>
                列设置
              </a-button>
              <template #content>
                <div class="assets-column-settings">
                  <div class="assets-column-settings-title">
                    附加列
                  </div>
                  <a-checkbox-group
                    :model-value="visibleExtraColumns"
                    direction="vertical"
                    :options="optionalColumnOptions"
                    @change="onVisibleColumnsChange"
                  />
                  <div class="assets-column-settings-hint">
                    勾选后表格转为横向滚动模式
                  </div>
                </div>
              </template>
            </a-popover>
            <a-button
              size="small"
              :disabled="!selectedAppId"
              @click="emit('refresh-resources')"
            >
              <template #icon>
                <IconRefresh />
              </template>
              刷新
            </a-button>
            <a-button
              type="primary"
              size="small"
              :disabled="!selectedAppId"
              @click="emit('create-resource')"
            >
              新建资源
            </a-button>
          </a-space>
        </template>

        <!-- 筛选独立成行：不再挤在 Arco 卡片 header（固定高度会裁掉换行内容） -->
        <div class="registry-toolbar">
          <a-space
            wrap
            :size="8"
          >
            <a-input
              v-model="filterCloudResourceType"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="云类型"
              size="small"
              style="width: 106px"
              @press-enter="emit('apply-filters')"
              @clear="emit('apply-filters')"
            />
            <a-input
              v-model="filterRegion"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="Region"
              size="small"
              style="width: 118px"
              @press-enter="emit('apply-filters')"
              @clear="emit('apply-filters')"
            />
            <a-select
              v-model="filterSyncStatus"
              :disabled="!selectedAppId"
              allow-clear
              placeholder="同步状态"
              size="small"
              style="width: 118px"
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
              type="primary"
              size="small"
              :disabled="!selectedAppId"
              @click="emit('apply-filters')"
            >
              筛选
            </a-button>
            <a-button
              size="small"
              :disabled="!selectedAppId"
              @click="emit('reset-filters')"
            >
              重置
            </a-button>
          </a-space>
          <span class="registry-toolbar-count">共 {{ resourcePagination.total }} 条资源</span>
        </div>

        <a-empty
          v-if="!selectedAppId"
          class="assets-empty"
          description="请先在左侧选择应用"
        />
        <a-table
          v-else
          :class="['page-table', { 'page-table-fit': fitResourceTable }]"
          :style="fitResourceTable ? resourceFitStyle : undefined"
          :table-layout-fixed="true"
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
              <span class="assets-id">{{ shortId((record as Resource).cloud_resource_id) }}</span>
            </a-tooltip>
            <span v-else>-</span>
          </template>
          <template #integration_account_id="{ record }">
            <a-tooltip
              v-if="(record as Resource).integration_account_id"
              :content="(record as Resource).integration_account_id"
            >
              <span class="assets-id">{{ shortId((record as Resource).integration_account_id) }}</span>
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
              <span class="assets-id">{{ (record as Resource).region }}</span>
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
              <span class="assets-id">{{ shortId((record as Resource).sync_batch_id) }}</span>
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
            <a-space :size="2">
              <a-button
                type="text"
                size="mini"
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
                  size="mini"
                  status="danger"
                >
                  删除
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </a-table>
      </a-card>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { CSSProperties } from 'vue'
import {
  IconDelete,
  IconEdit,
  IconRefresh,
  IconSettings
} from '@arco-design/web-vue/es/icon'
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { TableData } from '@arco-design/web-vue/es/table/interface'
import type { Application, Resource } from '@/api/asset'
import {
  OPTIONAL_RESOURCE_COLUMNS,
  labelEntries,
  shortId
} from '../composables/assetUtils'
import type { OptionalResourceColumnKey } from '../composables/assetUtils'

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
  visibleExtraColumns: OptionalResourceColumnKey[]
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
  'reset-filters': []
  'column-visibility-change': [keys: OptionalResourceColumnKey[]]
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

// 列宽按百分比交给 .page-table-fit，表格随容器缩放，始终 100% 铺满。
// 顺序与 appColumns / 默认 resourceColumns 一一对应；勾选附加列后不再套用 fit。
const appFitStyle: CSSProperties = {
  '--aiops-col-1': '76%',
  '--aiops-col-2': '24%'
}

const resourceFitStyle: CSSProperties = {
  '--aiops-col-1': '16%',
  '--aiops-col-2': '8%',
  '--aiops-col-3': '8%',
  '--aiops-col-4': '9%',
  '--aiops-col-5': '14%',
  '--aiops-col-6': '15%',
  '--aiops-col-7': '10%',
  '--aiops-col-8': '9%',
  '--aiops-col-9': '11%'
}

const fitResourceTable = computed(() => !('x' in props.resourceTableScroll))

const optionalColumnOptions = OPTIONAL_RESOURCE_COLUMNS.map((col) => ({
  label: col.label,
  value: col.key
}))

const optionalColumnKeys = OPTIONAL_RESOURCE_COLUMNS.map((col) => col.key)

function onVisibleColumnsChange(values: unknown[]) {
  // 只回传白名单内的 key，保持列定义与勾选状态严格对齐。
  emit('column-visibility-change', optionalColumnKeys.filter((key) => values.includes(key)))
}

function appMeta(app: Application) {
  const env = app.environment || 'default'
  return app.namespace ? `${env} · ${app.namespace}` : env
}

function appDetail(app: Application) {
  return [app.name, appMeta(app), app.description].filter(Boolean).join(' · ')
}

function formatTs(ts?: number) {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString()
}

function labelPreviewEntries(resource: Resource) {
  return labelEntries(resource.labels).slice(0, 3)
}
</script>
