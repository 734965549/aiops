<template>
  <div class="assets-page">
    <a-tabs
      v-model:active-key="activeTab"
      class="assets-tabs"
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
          :table-scroll="registry.tableScroll"
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
          @update:resource-filters="(f) => Object.assign(registry.resourceFilters, f)"
          @refresh-apps="registry.loadApplications"
          @create-app="registry.openCreateApplication"
          @edit-app="registry.openEditApplication"
          @delete-app="registry.confirmDeleteApplication"
          @refresh-resources="registry.loadResources"
          @create-resource="registry.openCreateResource"
          @edit-resource="registry.openEditResource"
          @delete-resource="registry.confirmDeleteResource"
          @apply-filters="registry.applyResourceFilters"
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
          :table-scroll="sync.tableScroll"
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
          :table-scroll="registry.tableScroll"
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
.assets-page {
  height: calc(100vh - 144px);
  min-height: 560px;
  overflow: hidden;
}

.assets-tabs,
:deep(.assets-tabs > .arco-tabs-content),
:deep(.assets-tabs > .arco-tabs-content > .arco-tabs-content-list),
:deep(.assets-tabs > .arco-tabs-content > .arco-tabs-content-list > .arco-tabs-pane) {
  height: 100%;
}

:deep(.assets-tabs > .arco-tabs-nav) {
  margin-bottom: 12px;
}

:deep(.registry-layout),
:deep(.registry-layout > .arco-col) {
  height: 100%;
}

:deep(.assets-card) {
  overflow: hidden;
}

:deep(.assets-card-fixed) {
  height: calc(100% - 44px);
}

:deep(.assets-card .arco-card-body) {
  height: calc(100% - 50px);
  padding: 0 16px 12px;
  overflow: hidden;
}

:deep(.assets-card .arco-table) {
  height: 100%;
}

:deep(.assets-card .arco-table-container) {
  height: calc(100% - 44px);
}

:deep(.assets-card .arco-table-pagination) {
  margin-top: 10px;
}

:deep(.assets-text-ellipsis) {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
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
  border-radius: 4px;
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
