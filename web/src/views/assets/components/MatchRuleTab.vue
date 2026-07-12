<template>
  <a-card
    title="告警匹配规则"
    :bordered="false"
    class="assets-card assets-card-fixed"
  >
    <template #extra>
      <a-space>
        <a-button @click="emit('refresh')">
          刷新
        </a-button>
        <a-button
          type="primary"
          @click="emit('create')"
        >
          新建规则
        </a-button>
      </a-space>
    </template>
    <a-table
      :columns="ruleColumns"
      :data="matchRules"
      :loading="rulesLoading"
      row-key="id"
      :pagination="rulePagination"
      :scroll="tableScroll"
      :bordered="false"
      @page-change="(page: number) => emit('page-change', page)"
      @page-size-change="(size: number) => emit('page-size-change', size)"
    >
      <template #enabled="{ record }">
        <a-tag :color="(record as MatchRule).enabled ? 'green' : 'gray'">
          {{ (record as MatchRule).enabled ? '启用' : '禁用' }}
        </a-tag>
      </template>
      <template #target_type="{ record }">
        {{ (record as MatchRule).target_type }}
      </template>
      <template #actions="{ record }">
        <a-space>
          <a-button
            type="text"
            size="small"
            @click="emit('edit', record as MatchRule)"
          >
            编辑
          </a-button>
          <a-popconfirm
            content="确定删除该匹配规则？"
            @ok="emit('delete', record as MatchRule)"
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
</template>

<script setup lang="ts">
import type { TableInstance } from '@arco-design/web-vue/es/table'
import type { MatchRule } from '@/api/asset'

interface PaginationConfig {
  current: number
  pageSize: number
  total: number
  showTotal: boolean
  showPageSize: boolean
}

defineProps<{
  matchRules: MatchRule[]
  rulesLoading: boolean
  ruleColumns: TableInstance['columns']
  rulePagination: PaginationConfig
  tableScroll: Record<string, unknown>
}>()

const emit = defineEmits<{
  'refresh': []
  'create': []
  'edit': [rule: MatchRule]
  'delete': [rule: MatchRule]
  'page-change': [page: number]
  'page-size-change': [size: number]
}>()
</script>
