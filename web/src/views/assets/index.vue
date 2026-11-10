<template>
  <div class="page-shell">
    <a-tabs
      v-model:active-key="activeTab"
      class="assets-tabs page-tabs"
    >
      <a-tab-pane
        key="registry"
        title="注册表"
      >
        <AssetRegistryTab
          :applications="registry.applications"
          :apps-loading="registry.appsLoading"
          :app-columns="registry.appColumns"
          :app-pagination="registry.appPagination"
          :app-row-class="registry.appRowClass"
          :selected-app-id="registry.selectedAppId"
          :resource-card-title="registry.resourceCardTitle"
          :resources="registry.resources"
          :resources-loading="registry.resourcesLoading"
          :resource-columns="registry.resourceColumns"
          :resource-pagination="registry.resourcePagination"
          :resource-table-scroll="registry.resourceTableScroll"
          :resource-row-class="registry.resourceRowClass"
          :resource-filters="registry.resourceFilters"
          :visible-extra-columns="registry.visibleExtraColumns"
          @update:resource-filters="(f) => Object.assign(registry.resourceFilters, f)"
          @column-visibility-change="registry.setVisibleExtraColumns"
          @refresh-apps="registry.loadApplications"
          @create-app="registry.openCreateApplication"
          @edit-app="registry.openEditApplication"
          @delete-app="registry.confirmDeleteApplication"
          @refresh-resources="registry.loadResources"
          @create-resource="registry.openCreateResource"
          @edit-resource="registry.openEditResource"
          @delete-resource="registry.confirmDeleteResource"
          @apply-filters="registry.applyResourceFilters"
          @reset-filters="registry.resetResourceFilters"
          @select-app="registry.onSelectApplication"
          @app-page-change="registry.onAppPageChange"
          @app-page-size-change="registry.onAppPageSizeChange"
          @resource-page-change="registry.onResourcePageChange"
          @resource-page-size-change="registry.onResourcePageSizeChange"
        />
      </a-tab-pane>

      <a-tab-pane
        key="cloud-sync"
        title="云同步"
      >
        <CloudSyncTab
          v-model="sync.syncAccountId"
          :sync-batches="sync.syncBatches"
          :sync-batches-loading="sync.syncBatchesLoading"
          :sync-batch-columns="sync.syncBatchColumns"
          :sync-pagination="sync.syncPagination"
          :sync-loading="sync.syncLoading"
          @trigger-sync="sync.runCloudSync"
          @refresh="sync.loadSyncBatches"
          @open-detail="sync.openSyncBatchDetail"
          @page-change="sync.onSyncPageChange"
          @page-size-change="sync.onSyncPageSizeChange"
        />
      </a-tab-pane>

      <a-tab-pane
        key="match-rules"
        title="匹配规则"
      >
        <MatchRuleTab
          :match-rules="rules.matchRules"
          :rules-loading="rules.rulesLoading"
          :rule-columns="rules.ruleColumns"
          :rule-pagination="rules.rulePagination"
          @refresh="rules.loadMatchRules"
          @create="rules.openCreateMatchRule"
          @edit="rules.openEditMatchRule"
          @delete="rules.confirmDeleteMatchRule"
          @page-change="rules.onRulePageChange"
          @page-size-change="rules.onRulePageSizeChange"
        />
      </a-tab-pane>
    </a-tabs>

    <a-modal
      v-model:visible="registry.appModalVisible"
      :title="registry.appModalMode === 'edit' ? '编辑应用' : '新建应用'"
      :ok-loading="registry.appSaving"
      @ok="registry.submitApplication"
    >
      <a-form
        :model="registry.appForm"
        layout="vertical"
      >
        <a-form-item
          label="应用名"
          required
        >
          <a-input
            v-model="registry.appForm.name"
            placeholder="如 payment-service"
          />
        </a-form-item>
        <a-form-item label="环境">
          <a-select
            v-model="registry.appForm.environment"
            allow-clear
            placeholder="prod / staging / dev"
          >
            <a-option value="prod">
              prod
            </a-option>
            <a-option value="staging">
              staging
            </a-option>
            <a-option value="dev">
              dev
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="默认 Namespace">
          <a-input
            v-model="registry.appForm.namespace"
            placeholder="K8s namespace（可选）"
          />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea
            v-model="registry.appForm.description"
            placeholder="业务线、负责人等备注（可选）"
            :auto-size="{ minRows: 2, maxRows: 4 }"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="registry.resourceModalVisible"
      :title="registry.resourceModalMode === 'edit' ? '编辑资源' : '新建资源'"
      :ok-loading="registry.resourceSaving"
      @ok="registry.submitResource"
    >
      <a-form
        :model="registry.resourceForm"
        layout="vertical"
      >
        <a-form-item label="所属应用">
          <a-input
            :model-value="registry.selectedAppName"
            disabled
          />
        </a-form-item>
        <a-form-item label="资源名">
          <a-input
            v-model="registry.resourceForm.name"
            placeholder="显示名称（可选）"
          />
        </a-form-item>
        <a-form-item label="类型">
          <a-select
            v-model="registry.resourceForm.resource_type"
            allow-clear
            placeholder="pod / node / host / service"
          >
            <a-option value="pod">
              pod
            </a-option>
            <a-option value="node">
              node
            </a-option>
            <a-option value="host">
              host
            </a-option>
            <a-option value="service">
              service
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item label="Namespace">
          <a-input v-model="registry.resourceForm.namespace" />
        </a-form-item>
        <a-form-item label="Pod">
          <a-input v-model="registry.resourceForm.pod" />
        </a-form-item>
        <a-form-item label="Node">
          <a-input v-model="registry.resourceForm.node" />
        </a-form-item>
        <a-form-item label="Instance">
          <a-input
            v-model="registry.resourceForm.instance"
            placeholder="Prometheus instance 等"
          />
        </a-form-item>
        <a-form-item
          v-if="registry.resourceModalMode === 'edit'"
          label="云资源 Labels"
        >
          <a-empty
            v-if="!registry.editingResourceLabelEntries.length"
            description="暂无 labels"
          />
          <div
            v-else
            class="asset-label-panel"
          >
            <div
              v-for="item in registry.editingResourceLabelEntries"
              :key="item.key"
              class="asset-label-row"
            >
              <span class="asset-label-key">{{ item.key }}</span>
              <span class="asset-label-value">{{ item.displayValue }}</span>
            </div>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>

    <SyncBatchDetailDrawer
      v-model:visible="sync.syncBatchDetailVisible"
      :batch-detail="sync.syncBatchDetail"
      :loading="sync.syncBatchDetailLoading"
      :message-summary="sync.syncBatchMessageSummary"
      :scope-cards="sync.syncBatchScopeCards"
      :signal-tags="sync.syncBatchSignalTags"
      :selected-scope-key="sync.selectedSyncScopeKey"
      :selected-scope-trace="sync.selectedScopeTrace"
      :selected-scope-trace-cards="sync.selectedScopeTraceCards"
      :selected-scope-trace-tags="sync.selectedScopeTraceTags"
      :selected-scope-trace-snippet="sync.selectedScopeTraceSnippet"
      :selected-scope-signal-key="sync.selectedScopeSignalKey"
      :scope-diagnostic-columns="sync.syncBatchScopeDiagnosticColumns"
      @toggle-scope="sync.toggleScopeByKey"
      @open-scope="sync.openScopeTrace"
      @open-signal="sync.openScopeSignal"
      @copy-snippet="sync.copyScopeSnippet"
    />

    <a-modal
      v-model:visible="rules.ruleModalVisible"
      :title="rules.ruleModalMode === 'edit' ? '编辑匹配规则' : '新建匹配规则'"
      :ok-loading="rules.ruleSaving"
      @ok="rules.submitMatchRule"
    >
      <a-form
        :model="rules.ruleForm"
        layout="vertical"
      >
        <a-form-item
          label="规则名"
          required
        >
          <a-input
            v-model="rules.ruleForm.name"
            placeholder="如 payment 服务匹配"
          />
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="8">
            <a-form-item label="优先级">
              <a-input-number
                v-model="rules.ruleForm.priority"
                :min="0"
                :max="9999"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="启用">
              <a-switch v-model="rules.ruleForm.enabled" />
            </a-form-item>
          </a-col>
          <a-col :span="8">
            <a-form-item label="目标类型">
              <a-select v-model="rules.ruleForm.target_type">
                <a-option value="application">
                  application
                </a-option>
                <a-option value="resource">
                  resource
                </a-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="接入源">
          <a-select v-model="rules.ruleForm.source_type">
            <a-option value="all">
              all
            </a-option>
            <a-option value="prometheus_alertmanager">
              prometheus_alertmanager
            </a-option>
            <a-option value="huawei_ces">
              huawei_ces
            </a-option>
            <a-option value="signoz">
              signoz
            </a-option>
          </a-select>
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="10">
            <a-form-item
              label="Label Key"
              required
            >
              <a-input
                v-model="rules.ruleForm.label_key"
                placeholder="service"
              />
            </a-form-item>
          </a-col>
          <a-col :span="14">
            <a-form-item
              label="Label 匹配模式"
              required
            >
              <a-input
                v-model="rules.ruleForm.label_value_pattern"
                placeholder="payment-*"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item
          label="绑定应用"
          required
        >
          <a-select
            v-model="rules.ruleForm.application_id"
            allow-search
            placeholder="选择应用"
            @change="rules.onRuleAppChange"
          >
            <a-option
              v-for="app in rules.ruleApplicationOptions"
              :key="app.id"
              :value="app.id"
            >
              {{ app.name }} ({{ app.environment || 'default' }})
            </a-option>
          </a-select>
        </a-form-item>
        <a-form-item
          v-if="rules.ruleForm.target_type === 'resource'"
          label="绑定资源"
          required
        >
          <a-select
            v-model="rules.ruleForm.resource_id"
            allow-search
            placeholder="选择资源"
          >
            <a-option
              v-for="res in rules.ruleResourceOptions"
              :key="res.id"
              :value="res.id"
            >
              {{ res.name || res.pod || res.id }}
            </a-option>
          </a-select>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { Application, Resource } from '@/api/asset'
import AssetRegistryTab from './components/AssetRegistryTab.vue'
import CloudSyncTab from './components/CloudSyncTab.vue'
import MatchRuleTab from './components/MatchRuleTab.vue'
import SyncBatchDetailDrawer from './components/SyncBatchDetailDrawer.vue'
import { useAssetRegistry } from './composables/useAssetRegistry'
import { useCloudSync } from './composables/useCloudSync'
import { useMatchRules } from './composables/useMatchRules'

const route = useRoute()

const activeTab = ref('registry')

const selectedAppId = ref('')
const ruleApplicationOptions = ref<Application[]>([])
const ruleResourceOptions = ref<Resource[]>([])

const registry = reactive(useAssetRegistry({ selectedAppId, ruleApplicationOptions }))
const rules = reactive(useMatchRules({ selectedAppId, ruleApplicationOptions, ruleResourceOptions }))
const sync = reactive(useCloudSync({
  onSyncComplete: async (batch) => {
    await registry.loadApplications()
    if (batch.application_id) {
      selectedAppId.value = batch.application_id
      registry.resourcePagination.current = 1
      await registry.loadResources()
    }
  }
}))

async function applyRouteSelection() {
  const q = typeof route.query.application_id === 'string' ? route.query.application_id : ''
  const resourceId = registry.routeResourceId()
  if (!q && selectedAppId.value) {
    selectedAppId.value = ''
    registry.highlightedResourceId = ''
    registry.resources = []
    registry.resourcePagination.total = 0
    return
  }
  if (q && q !== selectedAppId.value) {
    selectedAppId.value = q
    registry.resourcePagination.current = 1
    await registry.loadResources()
    return
  }
  if (resourceId && resourceId !== registry.highlightedResourceId && selectedAppId.value) {
    await registry.loadResources()
  }
}

watch(
  () => [route.query.application_id, route.query.resource_id],
  () => {
    void applyRouteSelection()
  }
)

onMounted(async () => {
  await registry.loadApplications()
  await rules.loadMatchRules()
  await sync.loadSyncBatches()
  const q = typeof route.query.application_id === 'string' ? route.query.application_id : ''
  if (q) {
    selectedAppId.value = q
    registry.resourcePagination.current = 1
    await registry.loadResources()
  } else if (registry.applications.length === 1) {
    selectedAppId.value = registry.applications[0].id
    registry.resourcePagination.current = 1
    await registry.loadResources()
  }
})
</script>

<style scoped>
/* 容器与页签填充链由全局 .page-shell / .page-tabs 提供 */

:deep(.assets-tabs > .arco-tabs-nav) {
  margin-bottom: 12px;
}

/* 左右分栏：左栏是应用选择器（字段短、条数少），固定 300px；
   右栏是资源宽表，占满剩余空间。用 grid 而非 a-row，避免 gutter 负 margin
   把内容撑出 .main、在页面底部多出一条横向滚动条。 */
:deep(.registry-layout) {
  display: grid;
  grid-template-columns: minmax(0, 300px) minmax(0, 1fr);
  gap: var(--space-4);
  height: 100%;
  min-height: 0;
}

:deep(.registry-panel) {
  display: flex;
  min-width: 0;
  min-height: 0;
}

/* 卡片填充：与全局 .page-card 同一套 flex 链（注册页卡片在列内，需在此声明）；
   表格滚动统一由表格自身的 .page-table 全局链负责，不再使用 scroll.y 第二套高度 */
:deep(.assets-card) {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* header 只放标题 + 三个操作按钮。筛选表单已移到卡片 body 的工具条，
   不再受 Arco 固定 header 高度裁切。 */
:deep(.assets-card > .arco-card-header) {
  /* Arco 默认给 header 固定 height + overflow:hidden，窄屏换行时会裁掉第二行 */
  height: auto;
  min-height: 50px;
  align-items: center;
  gap: var(--space-3);
}

:deep(.assets-card > .arco-card-header > .arco-card-header-extra) {
  flex-shrink: 0;
  overflow: visible;
  white-space: normal;
}

:deep(.assets-card > .arco-card-body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 筛选工具条独立成行，窄屏自动换行；右侧给出结果总数，不占用表头空间 */
:deep(.registry-toolbar) {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-3);
  padding-bottom: var(--space-3);
  margin-bottom: var(--space-3);
  border-bottom: 1px solid var(--aiops-border-soft);
}

:deep(.registry-toolbar-count) {
  flex-shrink: 0;
  color: var(--aiops-text-3);
  font-size: 12px;
  white-space: nowrap;
}

/* 应用单元格：主名 + 环境/Namespace 副行，替代被压扁的多列 */
:deep(.app-cell) {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.4;
  text-align: left;
}

:deep(.app-cell-name),
:deep(.app-cell-meta) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.app-cell-name) {
  color: var(--aiops-text-1);
  font-weight: 600;
}

:deep(.app-cell-meta) {
  color: var(--aiops-text-3);
  font-size: 11px;
}

:deep(.assets-id),
:deep(.assets-text-ellipsis) {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

/* 长 ID 一律显示短码，等宽字体避免列宽被 UUID 撑开 */
:deep(.assets-id) {
  color: var(--aiops-text-2);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}

/* 窄屏两栏堆叠，高度交回内容流，由页签 pane 负责滚动 */
@media (max-width: 1180px) {
  :deep(.registry-layout) {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto minmax(420px, 1fr);
    height: auto;
    min-height: 100%;
  }
}

:deep(.assets-empty) {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

:deep(.assets-row-selected .arco-table-td) {
  background-color: var(--color-fill-2);
}

:deep(.assets-row-highlight .arco-table-td) {
  background-color: rgb(var(--primary-1));
}

:deep(.asset-label-tags) {
  display: flex;
  gap: 4px;
  max-width: 100%;
  overflow: hidden;
  flex-wrap: nowrap;
}

:deep(.asset-label-popover) {
  max-width: 440px;
  max-height: 320px;
  overflow: auto;
}

.asset-label-panel {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid var(--color-border-2);
  border-radius: var(--radius-sm);
  background: var(--color-fill-1);
}

.asset-label-panel,
:deep(.asset-label-panel) {
  max-width: 440px;
  max-height: 320px;
  overflow: auto;
}

.asset-label-row,
:deep(.asset-label-row) {
  display: grid;
  grid-template-columns: minmax(110px, 180px) minmax(0, 1fr);
  gap: 8px;
  line-height: 24px;
}

.asset-label-key,
:deep(.asset-label-key) {
  color: var(--color-text-2);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-label-value,
:deep(.asset-label-value) {
  color: var(--color-text-1);
  overflow-wrap: anywhere;
  word-break: break-word;
}
</style>

<style>
/* 列设置浮层挂载到 body，scoped 样式无法命中，使用带前缀的全局类名 */
.assets-column-settings {
  min-width: 190px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.assets-column-settings-title {
  color: var(--aiops-text-1);
  font-size: 12px;
  font-weight: 600;
}

.assets-column-settings-hint {
  padding-top: 8px;
  border-top: 1px solid var(--aiops-border-soft);
  color: var(--aiops-text-3);
  font-size: 11px;
  line-height: 1.5;
}
</style>
