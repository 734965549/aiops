<template>
  <div class="dashboard">
    <section
      ref="heroRef"
      class="dashboard-hero"
    >
      <img
        class="hero-image"
        :src="heroImage"
        alt="从海底向上仰望水面的海洋景观"
      >
      <div class="hero-water-shade" />
      <SardineSchool
        class="hero-school"
        :count="220"
        :seed="20260703"
        :center-x="0.52"
        :center-y="0.48"
        :radius-x="0.42"
        :radius-y="0.39"
        :fish-scale="0.92"
      />

      <div class="hero-body">
        <div class="hero-topline">
          <span class="hero-brand">✦ AIOps Command</span>
          <span class="hero-status">
            <i :class="{ 'is-ready': readiness?.status === 'ready' }" />
            {{ readiness?.status === 'ready' ? 'Platform ready' : 'Readiness pending' }}
          </span>
        </div>

        <div class="hero-copy">
          <span class="hero-kicker">Operational intelligence / with evidence</span>
          <h1>将每一次信号，<br><em>转化为可追踪的行动。</em></h1>
          <p>
            从告警接入到审计追溯，让 AI 的建议始终运行在权限、风险与人工确认构成的安全边界内。
          </p>
          <div class="hero-actions">
            <button
              type="button"
              class="hero-link hero-link-primary"
              @click="go('/alerts')"
            >
              查看活跃告警 <span>↗</span>
            </button>
            <button
              type="button"
              class="hero-link"
              @click="go('/executions')"
            >
              进入执行中心 <span>→</span>
            </button>
          </div>
        </div>

        <div class="hero-flow">
          <span><b>01</b> 告警接入</span>
          <span><b>02</b> 资产匹配</span>
          <span><b>03</b> Runbook 推荐</span>
          <span><b>04</b> 执行确认</span>
          <span><b>05</b> 审计追溯</span>
        </div>
      </div>
    </section>

    <section class="dashboard-toolbar">
      <div>
        <span class="toolbar-kicker">LIVE OPERATIONS / 03</span>
        <h2>此刻的运行态势</h2>
        <p>数据来自告警、资产、执行与 Runbook 闭环。</p>
      </div>
      <a-button
        :loading="loading"
        @click="loadAll"
      >
        刷新数据
      </a-button>
    </section>

    <MetricCards
      :summary="summary"
      :readiness="readiness"
      @navigate="go"
    />

    <a-row
      :gutter="16"
      class="row equal-row"
    >
      <a-col
        :xs="24"
        :lg="14"
      >
        <RecentExecutions
          :executions="recentExecutions"
          :loading="loadingExecutions"
          @navigate="go"
          @open-execution="onExecutionOpen"
        />
      </a-col>

      <a-col
        :xs="24"
        :lg="10"
      >
        <RunbookRecommendations
          :executions="recentExecutions"
          :loading-executions="loadingExecutions"
          :loading-recommendations="loadingRecommendations"
          :total-count="runbookTotalCount"
          :enabled-count="runbookEnabledCount"
          :recommendations="runbookRecommendations"
          @navigate="go"
          @open-execution="onExecutionOpen"
        />
      </a-col>
    </a-row>

    <ReadinessPanel
      class="row"
      :version="version"
      :readiness="readiness"
      :loading="loadingHealth"
    />
  </div>
</template>

<script setup lang="ts">
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { ExecutionTask } from '@/api/execution'
import heroImage from '@/assets/aiops-ocean-depth.jpg'
import SardineSchool from '@/layouts/components/SardineSchool.vue'
import { useDashboardData } from './composables/useDashboardData'

const MetricCards = defineAsyncComponent(() => import('./components/MetricCards.vue'))
const RecentExecutions = defineAsyncComponent(() => import('./components/RecentExecutions.vue'))
const RunbookRecommendations = defineAsyncComponent(() => import('./components/RunbookRecommendations.vue'))
const ReadinessPanel = defineAsyncComponent(() => import('./components/ReadinessPanel.vue'))

const router = useRouter()
const heroRef = ref<HTMLElement | null>(null)
let scrollContainer: HTMLElement | null = null
let animationFrame = 0

const {
  loading,
  loadingExecutions,
  loadingRecommendations,
  loadingHealth,
  summary,
  recentExecutions,
  runbookTotalCount,
  runbookEnabledCount,
  runbookRecommendations,
  version,
  readiness,
  loadAll
} = useDashboardData()

function go(path: string) {
  if (!path) return
  router.push(path)
}

function onExecutionOpen(task: ExecutionTask) {
  router.push({ path: '/executions', query: { task_id: task.id } })
}

function updateHeroMotion() {
  animationFrame = 0
  const hero = heroRef.value
  if (!hero) return
  const rect = hero.getBoundingClientRect()
  const progress = Math.max(-1, Math.min(1, (window.innerHeight * 0.45 - rect.top) / window.innerHeight))
  hero.style.setProperty('--hero-shift', `${progress * 18}px`)
  hero.style.setProperty('--hero-scale', String(1.035 + Math.abs(progress) * 0.025))
}

function scheduleHeroMotion() {
  if (animationFrame) return
  animationFrame = window.requestAnimationFrame(updateHeroMotion)
}

onMounted(() => {
  loadAll()
  scrollContainer = heroRef.value?.closest('.main') as HTMLElement | null
  scrollContainer?.addEventListener('scroll', scheduleHeroMotion, { passive: true })
  updateHeroMotion()
})

onBeforeUnmount(() => {
  scrollContainer?.removeEventListener('scroll', scheduleHeroMotion)
  if (animationFrame) window.cancelAnimationFrame(animationFrame)
})
</script>

<style scoped>
.dashboard {
  --hero-shift: 0px;
  --hero-scale: 1.035;
}

.dashboard-hero {
  min-height: clamp(380px, 46vh, 540px);
  position: relative;
  display: flex;
  overflow: hidden;
  border: 1px solid rgba(173, 235, 226, 0.34);
  border-radius: var(--radius-lg);
  color: #f4fffc;
  background: #053946;
  isolation: isolate;
}

.dashboard-hero::before {
  content: '';
  position: absolute;
  inset: 12px;
  z-index: 3;
  pointer-events: none;
  border: 1px solid rgba(205, 249, 241, 0.24);
  border-radius: var(--radius-sm);
}

.hero-image {
  position: absolute;
  inset: -3%;
  width: 106%;
  height: 106%;
  object-fit: cover;
  z-index: 0;
  object-position: 50% 42%;
  filter: saturate(0.92) contrast(1.05) brightness(0.78);
  transform: translate3d(0, var(--hero-shift), 0) scale(var(--hero-scale));
  transform-origin: center;
  transition: transform 80ms linear;
  will-change: transform;
}

.hero-water-shade {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
  background:
    radial-gradient(ellipse 36% 54% at 52% 48%, rgba(66, 155, 170, 0.34), rgba(3, 45, 57, 0.16) 60%, transparent 100%),
    linear-gradient(180deg, rgba(0, 24, 33, 0.14), transparent 42%, rgba(0, 24, 33, 0.28));
}

.hero-school {
  position: absolute;
  inset: 0;
  z-index: 2;
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 3px 5px rgba(0, 21, 29, 0.38));
}

/* 内容层：flex 流式布局，topline / copy / flow 依次排列，
   小屏缩放下不会再出现绝对定位导致的重叠 */
.hero-body {
  position: relative;
  z-index: 4;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 26px 32px;
}

.hero-topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 10px;
}

.hero-brand {
  font-family: var(--aiops-display);
  font-style: italic;
  font-weight: 600;
}

.hero-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.hero-status i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #a58143;
  box-shadow: 0 0 0 4px rgba(165, 129, 67, 0.12);
}

.hero-status i.is-ready {
  background: #587443;
  box-shadow: 0 0 0 4px rgba(88, 116, 67, 0.12);
}

.hero-copy {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: min(680px, 100%);
  margin: 0 auto;
  text-align: center;
  text-shadow: 0 2px 18px rgba(0, 22, 31, 0.34);
}

.hero-kicker {
  display: block;
  margin-bottom: 14px;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.17em;
  text-transform: uppercase;
}

.hero-copy h1 {
  margin: 0;
  font-family: inherit;
  font-size: clamp(22px, 2.2vw, 30px);
  font-weight: 600;
  letter-spacing: 0;
  line-height: 1.25;
}

.hero-copy h1 em {
  font-style: normal;
}

.hero-copy p {
  max-width: 520px;
  margin: 12px auto 0;
  color: rgba(233, 250, 246, 0.8);
  font-size: 12px;
  line-height: 1.7;
}

.hero-actions {
  display: flex;
  justify-content: center;
  gap: 10px;
  margin-top: 18px;
}

.hero-link {
  height: 34px;
  padding: 0 16px;
  border: 1px solid rgba(214, 255, 247, 0.36);
  border-radius: var(--radius-full);
  color: #f4fffc;
  font: inherit;
  font-size: 12px;
  background: rgba(4, 48, 58, 0.48);
  backdrop-filter: blur(10px);
  cursor: pointer;
  transition: transform 180ms ease, background 180ms ease;
}

.hero-link:hover {
  transform: translateY(-2px);
  background: rgba(13, 80, 89, 0.72);
}

.hero-link span {
  margin-left: 8px;
}

.hero-link-primary {
  border-color: rgba(232, 255, 250, 0.82);
  color: #07343d;
  background: rgba(239, 255, 251, 0.9);
}

.hero-link-primary:hover {
  background: #ffffff;
}

.hero-flow {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 8px;
  padding: 12px 16px;
  border: 1px solid rgba(205, 249, 241, 0.22);
  border-radius: var(--radius-md);
  background: rgba(2, 35, 44, 0.6);
  backdrop-filter: blur(14px);
}

.hero-flow span {
  color: rgba(231, 249, 245, 0.76);
  font-size: 10px;
}

.hero-flow b {
  margin-right: 6px;
  color: #f4fffc;
  font-family: var(--aiops-display);
  font-style: italic;
}

.dashboard-toolbar {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
  margin: 24px 0 16px;
  padding: 0 2px 14px;
  border-bottom: 1px solid rgba(24, 27, 24, 0.16);
}

.dashboard-toolbar h2 {
  margin: 6px 0 0;
  color: var(--aiops-text-1);
  font-family: inherit;
  font-size: 20px;
  font-weight: 600;
  letter-spacing: 0;
}

.dashboard-toolbar p {
  margin: 7px 0 0;
  color: var(--aiops-text-3);
  font-size: 12px;
}

.toolbar-kicker {
  color: var(--aiops-text-3);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.16em;
}

.row {
  margin-top: 16px;
}

@media (max-width: 900px) {
  .hero-body {
    padding: 22px 24px;
  }
}

@media (max-width: 620px) {
  .hero-body {
    gap: 18px;
    padding: 20px;
  }

  .hero-image {
    object-position: 56% 42%;
  }

  .hero-brand {
    display: none;
  }

  .hero-status {
    margin-left: auto;
  }

  .hero-copy p {
    max-width: 320px;
  }

  .hero-actions {
    justify-content: center;
    flex-wrap: wrap;
  }

  .dashboard-toolbar {
    align-items: flex-start;
    flex-direction: column;
    margin-top: 20px;
  }
}
</style>
