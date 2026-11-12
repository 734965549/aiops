<template>
  <div
    v-if="vm.showRegion"
    class="alert-enhancement"
  >
    <!-- ① 触发条件卡：浅底全框卡片 + 横向结论行；所有降级态都保留外壳 -->
    <section
      v-if="vm.showConditionCard"
      class="alert-section"
      aria-labelledby="alert-condition-title"
    >
      <h3
        id="alert-condition-title"
        class="alert-section-title"
      >
        触发条件
      </h3>
      <div class="alert-condition-card">
        <div class="alert-condition-head">
          <span
            v-if="condition.headLabel"
            class="alert-condition-metric"
          >{{ condition.headLabel }}</span>
          <span
            v-if="condition.showMetric && condition.metricName"
            class="alert-condition-metric-value"
          >{{ condition.metricName }}</span>
        </div>

        <!-- 结论行整体读作一句自然语言，拆分的视觉片段对读屏隐藏 -->
        <div
          v-if="condition.showConclusion"
          class="alert-condition-body"
          role="img"
          :aria-label="condition.ariaLabel"
        >
          <span
            class="cond-op"
            aria-hidden="true"
          >{{ condition.parsed.op }}</span>
          <span
            class="cond-threshold"
            aria-hidden="true"
          >{{ condition.parsed.threshold }}{{ condition.parsed.unit }}</span>
          <span
            v-if="condition.parsed.windowText"
            class="cond-window"
            aria-hidden="true"
          >{{ condition.parsed.windowText }}</span>
        </div>
        <!-- 态 A'：条件原文整体降级，文本与云端返回值一字不差 -->
        <div
          v-else-if="condition.showRaw"
          class="alert-condition-body"
        >
          <code class="cond-raw">{{ condition.rawText }}</code>
          <AlertCopyButton
            :value="condition.rawText"
            label="触发条件原文"
          />
        </div>

        <!-- 区块级弱化容器一律 info；Arco 硬编码的 live role 由 v-no-live-role 移除 -->
        <a-alert
          v-if="condition.foot || condition.rawFoot"
          v-no-live-role
          type="info"
          class="alert-condition-foot"
          :show-icon="false"
        >
          <span v-if="condition.foot">{{ condition.foot }}</span>
          <template v-if="condition.rawFoot">
            <span>收到的原始条件</span>
            <code class="cond-raw">{{ condition.rawFoot }}</code>
            <AlertCopyButton
              :value="condition.rawFoot"
              label="收到的原始条件"
            />
          </template>
        </a-alert>
      </div>
    </section>

    <!-- ② labels：两列键值网格；未知键一律兜底渲染，>12 项默认折叠 -->
    <section
      v-if="vm.showLabelsPanel"
      class="alert-section"
      aria-labelledby="alert-labels-title"
    >
      <h3
        id="alert-labels-title"
        class="alert-section-title"
      >
        标签与维度
        <span
          class="alert-section-title-count"
          :aria-label="`共 ${vm.labelCount} 项`"
        >{{ vm.labelCount }}</span>
      </h3>
      <div class="alert-labels-panel">
        <dl
          :id="gridId"
          ref="gridEl"
          class="alert-labels-grid"
        >
          <div
            v-for="item in visibleLabels"
            :key="item.key"
            :data-key="item.key"
            class="alert-kv"
            :class="{ 'is-dim-key': !!item.hint }"
          >
            <dt class="alert-kv-label">
              {{ item.label }}
              <code
                v-if="item.label !== item.key"
                class="alert-kv-hint is-mono"
              >{{ item.key }}</code>
              <span
                v-if="item.hint"
                class="alert-kv-hint"
              >{{ item.hint }}</span>
            </dt>
            <dd
              class="alert-kv-value"
              :class="{ 'is-mono': item.mono, 'is-dim': item.sensitive }"
            >
              <span class="alert-kv-text">{{ item.value }}</span>
              <AlertCopyButton
                v-if="item.copyable"
                :value="item.value"
                :label="item.label"
              />
            </dd>
          </div>
        </dl>
        <p
          v-if="vm.dimOnlyNote"
          class="alert-muted-note"
        >
          该规则关联多个资源，云端未指定具体实例，仅提供维度名。
        </p>
        <button
          v-if="vm.needsCollapse"
          ref="triggerBtn"
          type="button"
          class="alert-labels-more"
          :aria-expanded="expanded ? 'true' : 'false'"
          :aria-controls="gridId"
          @click="toggleCollapse"
        >
          {{ expanded ? '收起' : `展开全部（${vm.labelCount}）` }}
        </button>
      </div>
    </section>

    <!-- ③ annotations：无框开放段落 + 左引用线；富化失败说明保持中性 info 语气 -->
    <section
      v-if="vm.showAnnotations"
      class="alert-section"
      aria-labelledby="alert-anno-title"
    >
      <h3
        id="alert-anno-title"
        class="alert-section-title"
      >
        规则信息
      </h3>
      <p class="alert-subtitle">
        来自告警规则与注解
      </p>
      <div class="alert-anno">
        <p
          v-if="vm.mutedNote"
          class="alert-muted-note"
        >
          规则详情未获取（富化状态：<code class="alert-muted-code">{{ vm.lookupValue }}</code>），告警数据完整；以下为已收到的原始字段。
        </p>

        <div
          v-if="vm.description"
          class="alert-anno-desc-wrap"
        >
          <p class="alert-anno-desc">
            {{ vm.description }}
          </p>
          <AlertCopyButton
            :value="vm.description"
            label="规则原文"
          />
        </div>

        <div
          v-if="vm.ruleTypeText"
          class="alert-anno-meta"
        >
          <span class="alert-anno-meta-label">规则类型</span>
          <span
            class="alert-anno-meta-value"
            :class="{ 'is-dim': vm.ruleTypeDim }"
          >{{ vm.ruleTypeText }}</span>
        </div>

        <div
          v-if="vm.levelCode"
          class="alert-anno-meta"
        >
          <span class="alert-anno-meta-label">云端级别</span>
          <span class="alert-anno-meta-value">云端原始级别 <code class="alert-muted-code">{{ vm.levelCode }}</code>{{ vm.levelSuffix }}</span>
        </div>

        <div
          v-for="kv in vm.annotationKVs"
          :key="kv.key"
          class="alert-anno-kv"
        >
          <span class="alert-kv-label">
            {{ kv.label }}
            <span class="alert-kv-hint">{{ kv.hint }}</span>
          </span>
          <span class="alert-kv-value is-mono">{{ kv.value }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import {
  buildAlertPresentation,
  COLLAPSE_VISIBLE,
  type EnhancementAlert
} from '../composables/alertPresentation'
import AlertCopyButton from './AlertCopyButton.vue'

const props = defineProps<{ alert: EnhancementAlert }>()

// Arco 2.56 的 a-alert 根节点硬编码 role="alert"（assertive live region），
// 且根节点是 Transition，属性透传覆盖不到内层 div。弱化说明是随详情一次性
// 渲染的静态内容，用指令移除该 live role，避免读屏把中性说明当紧急播报。
const vNoLiveRole = {
  mounted: (el: HTMLElement) => el.removeAttribute('role'),
  updated: (el: HTMLElement) => el.removeAttribute('role')
}

const vm = computed(() => buildAlertPresentation(props.alert))
const condition = computed(() => vm.value.condition)

const gridId = `alert-labels-grid-${useId()}`
const expanded = ref(false)
const gridEl = ref<HTMLElement | null>(null)
const triggerBtn = ref<HTMLButtonElement | null>(null)

const visibleLabels = computed(() =>
  vm.value.needsCollapse && !expanded.value
    ? vm.value.labels.slice(0, COLLAPSE_VISIBLE)
    : vm.value.labels
)

function toggleCollapse() {
  // 收起会把折叠项从 DOM/可访问性树移除；焦点若在其中，先交还给触发按钮。
  if (expanded.value && gridEl.value?.contains(document.activeElement) && triggerBtn.value) {
    triggerBtn.value.focus()
  }
  expanded.value = !expanded.value
}
</script>

<style scoped lang="scss">
$mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
$label-col: 96px;

.alert-enhancement { --alert-row-h: 28px; }

/* 区块节奏：相邻区块用中性细线分隔 */
.alert-section { margin-top: 12px; }
.alert-section + .alert-section { padding-top: 12px; border-top: 1px solid var(--color-border-2); }
.alert-section-title { display:flex; align-items:baseline; gap:8px; margin:0 0 6px; font-size:14px; font-weight:500; line-height:20px; color:var(--color-text-1); }
.alert-section-title-count { font-size:12px; font-weight:400; line-height:20px; color:var(--color-text-2); }
.alert-subtitle { margin:0 0 4px; font-size:12px; font-weight:400; line-height:20px; color:var(--color-text-2); }
.alert-kv-hint { font-size:12px; font-weight:400; line-height:20px; white-space:nowrap; color:var(--color-text-2); }
.alert-kv-hint.is-mono { font-family:$mono; }

/* ① 触发条件卡：形态不变（浅底全框 8px） */
.alert-condition-card { padding:10px 14px; background:var(--color-fill-1); border:1px solid var(--color-border-2); border-radius:var(--radius-md); }
.alert-condition-head { display:flex; align-items:baseline; flex-wrap:wrap; gap:6px; min-height:20px; line-height:20px; }
.alert-condition-metric { font-size:12px; line-height:20px; color:var(--color-text-2); }
.alert-condition-metric-value { font-size:13px; font-weight:500; line-height:20px; color:var(--color-text-1); overflow-wrap:anywhere; word-break:break-word; }
.alert-condition-body { display:flex; align-items:baseline; flex-wrap:wrap; column-gap:0; row-gap:2px; margin-top:6px; line-height:28px; }
.cond-op,.cond-threshold,.cond-window { display:inline-block; font-family:$mono; line-height:28px; }
.cond-op { margin-right:4px; font-size:13px; font-weight:600; color:var(--color-text-2); }
.cond-threshold { margin-right:8px; font-size:20px; font-weight:600; letter-spacing:-0.2px; color:rgb(var(--primary-6)); }
.cond-window { font-size:12px; color:var(--color-text-2); }
.cond-raw { display:inline-block; max-width:100%; padding:4px 10px; font-family:$mono; font-size:13px; line-height:20px; color:var(--color-text-2); background:var(--color-fill-2); border:1px solid var(--color-border-2); border-radius:var(--radius-sm); overflow-wrap:anywhere; word-break:break-word; }
.alert-condition-foot { margin-top:6px; padding:8px 12px; font-size:12px; line-height:20px; color:var(--color-text-2); background:var(--color-fill-1); border:1px solid var(--color-border-2); border-radius:var(--radius-sm);
  :deep(.arco-alert-content) { display:flex; align-items:baseline; flex-wrap:wrap; gap:4px 8px; line-height:20px; color:var(--color-text-2); }
}

/* ② labels：单列定义列表（两列→单列是本次最核心修正） */
.alert-labels-panel { padding:10px 14px; background:var(--color-fill-1); border:1px solid var(--color-border-2); border-radius:var(--radius-sm); }
.alert-labels-grid { display:grid; grid-template-columns:minmax(0,1fr); gap:0; margin:0; }
.alert-kv { display:grid; grid-template-columns:$label-col minmax(0,1fr); align-items:baseline; column-gap:12px; min-width:0; min-height:var(--alert-row-h); padding:2px 0; border-bottom:1px solid var(--color-border-2); }
.alert-kv:last-child { border-bottom:0; }
.alert-kv.is-dim-key { grid-template-columns:minmax(0,1fr); row-gap:0; min-height:calc(var(--alert-row-h) * 2); }
.alert-kv.is-dim-key .alert-kv-label { display:flex; align-items:baseline; flex-wrap:wrap; gap:4px; font-family:$mono; }
.alert-kv.is-dim-key .alert-kv-label,.alert-kv.is-dim-key .alert-kv-value { line-height:20px; padding:4px 0; }
.alert-anno-kv { display:grid; grid-template-columns:$label-col minmax(0,1fr); align-items:baseline; column-gap:12px; min-width:0; margin-top:4px; padding:2px 0; }
.alert-kv-label { min-width:0; font-size:12px; font-weight:400; line-height:20px; color:var(--color-text-2); overflow-wrap:anywhere; word-break:break-word; }
.alert-kv-value { position:relative; min-width:0; margin:0; padding-right:28px; font-size:13px; font-weight:400; line-height:20px; color:var(--color-text-1); overflow-wrap:anywhere; word-break:break-word; }
.alert-kv-text { overflow-wrap:anywhere; word-break:break-word; }
.is-mono { font-family:$mono; }
.is-dim { color:var(--color-text-2); }
.alert-anno-kv .alert-kv-value { position:static; padding-right:0; }
/* 复制按钮脱离行内流：否则 24px 按钮仍会让带按钮的行比不带按钮的行高 4px */
.alert-kv-value :deep(.alert-copy-wrap) { position:absolute; top:0; right:0; }
/* 剪贴板降级时把容器放回正常流：完整原值独占一行、位于值下方且不遮挡后续行 */
.alert-kv-value :deep(.alert-copy-wrap.is-fallback) { position:static; }
.alert-labels-more { min-height:28px; margin-top:8px; padding:0 12px; font-size:13px; line-height:20px; cursor:pointer; background:transparent; border:1px solid var(--color-border-2); border-radius:var(--radius-sm); color:var(--color-text-1); }
.alert-labels-more:hover { background:var(--color-fill-2); }
.alert-labels-more:focus-visible { outline:2px solid rgb(var(--primary-6)); outline-offset:2px; }

/* 弱化说明：中性，保留 --color-text-4 仅作装饰圆点 background */
.alert-muted-note { position:relative; margin:6px 0 0; padding:8px 12px 8px 22px; font-size:12px; line-height:20px; color:var(--color-text-2); background:var(--color-fill-1); border:1px solid var(--color-border-2); border-radius:var(--radius-sm); overflow-wrap:anywhere; word-break:break-word; }
.alert-muted-note::before { content:''; position:absolute; top:15px; left:10px; width:6px; height:6px; border-radius:50%; background:var(--color-text-4); }
.alert-muted-code { font-family:$mono; font-size:12px; color:var(--color-text-2); }

/* ③ annotations：透明无框 + 左 2px 中性引用线，形态与 ② 严格区分 */
.alert-anno { padding:2px 0 2px 12px; border-left:2px solid var(--color-border-2); background:transparent; }
.alert-anno-desc-wrap { display:flex; align-items:baseline; gap:4px; }
.alert-anno-desc { flex:1 1 auto; min-width:0; margin:0; font-size:13px; font-weight:400; line-height:20px; color:var(--color-text-1); overflow-wrap:anywhere; word-break:break-word; }
.alert-anno-meta { display:flex; align-items:baseline; flex-wrap:wrap; gap:4px 12px; margin-top:4px; font-size:12px; line-height:20px; }
.alert-anno-meta-label,.alert-anno-meta-value { color:var(--color-text-2); overflow-wrap:anywhere; word-break:break-word; }
.alert-anno-meta-label { flex:0 0 auto; }
.alert-anno-meta-value.is-dim { display:inline-block; padding:0 8px; background:var(--color-fill-2); border:1px solid var(--color-border-2); border-radius:var(--radius-sm); }

/* 响应式：三个 media 块都必须保留（静态断言要命中） */
@media (max-width:900px) { :deep(.alert-copy-btn) { opacity:1; } }
@media (max-width:480px) {
  .alert-kv,.alert-anno-kv,.alert-kv.is-dim-key { grid-template-columns:minmax(0,1fr); row-gap:0; }
}
@media (hover:none) {
  :deep(.alert-copy-btn) { opacity:1; }
  .alert-kv { min-height:44px; }
  .alert-kv.is-dim-key { min-height:88px; }
}
</style>
