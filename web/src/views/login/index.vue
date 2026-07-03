<template>
  <div
    class="login-page"
    :class="pageStateClass"
  >
    <main
      ref="sceneRef"
      class="login-frame"
      @pointermove="onScenePointerMove"
      @pointerleave="onScenePointerLeave"
    >
      <section class="login-visual">
        <img
          class="login-visual-image login-visual-image-organic"
          :src="organicHeroImage"
          alt="覆有青苔的树木形态"
        >
        <img
          class="login-visual-image login-visual-image-mechanical"
          :src="mechanicalHeroImage"
          alt=""
          aria-hidden="true"
        >
        <MorphCanvas
          ref="morphCanvasRef"
          class="login-morph-canvas"
          :organic-src="organicHeroImage"
          :mechanical-src="mechanicalHeroImage"
          @ready="morphReady = true"
        />

        <div
          class="login-morph-lens"
          aria-hidden="true"
        >
          <img
            class="login-visual-image login-morph-lens-image"
            :src="organicHeroImage"
            alt=""
          >
        </div>
        <div
          class="moss-burst"
          aria-hidden="true"
        />
        <div
          class="morph-interaction-zone"
          aria-hidden="true"
        />

        <div
          v-if="isCollapsed"
          class="collapsed-login-card"
          @click="isCollapsed = false"
        >
          <div class="collapsed-login-header">ENTERPRISE ACCESS</div>
          <div class="collapsed-login-content">
            <span class="collapsed-login-text">展开登录</span>
            <svg
              class="collapsed-login-arrow"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
            >
              <path d="M7 17L17 7M17 7H7M17 7V17" />
            </svg>
          </div>
        </div>

        <header class="brand-bar">
          <div class="brand-sign">
            <span class="brand-star">✦</span>
            <span>AI 运维平台</span>
          </div>
          <span class="brand-index">AIOPS / ACCESS / 01</span>
        </header>

        <div class="visual-copy">
          <span class="visual-kicker">Intelligent operations, under control.</span>
          <h1>让复杂的运维，<br><em>回到可控的秩序。</em></h1>
          <p>
            AI 提供分析、证据与计划；权限、确认、状态机和审计负责守住每一次真实执行。
          </p>
        </div>

        <div
          class="workflow-line"
          aria-label="平台核心闭环"
        >
          <span><b>01</b> 告警接入</span>
          <span><b>02</b> 资产匹配</span>
          <span><b>03</b> Runbook 推荐</span>
          <span><b>04</b> 安全执行</span>
        </div>

        <div class="visual-caption">
          Nature of reliability / Human in the loop
        </div>

        <div
          class="morph-guide"
          aria-hidden="true"
        >
          <span>Organic</span>
          <i><b /></i>
          <span>System</span>
        </div>
      </section>

      <section class="login-access" :class="{ 'is-collapsed': isCollapsed }">
        <div class="access-meta">
          <span>Enterprise access</span>
          <span class="access-state"><i /> Protected</span>
          <button
            class="collapse-toggle"
            @click="isCollapsed = !isCollapsed"
            title="折叠登录面板"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M18 15l-6-6-6 6" />
            </svg>
          </button>
        </div>

        <a-card
          class="login-card"
          :bordered="false"
        >
          <div class="login-index">
            02 / 身份验证
          </div>
          <div class="login-title">
            登录控制台
          </div>
          <div class="login-sub">
            {{ loginModeLabel }}
          </div>

          <a-tabs
            v-if="passwordProviders.length > 0"
            v-model:active-key="activeProviderId"
            type="rounded"
            class="login-tabs"
          >
            <a-tab-pane
              key="local"
              title="本地账号"
            />
            <a-tab-pane
              v-for="p in passwordProviders"
              :key="p.id"
              :title="p.name"
            />
          </a-tabs>

          <a-form
            :model="form"
            layout="vertical"
            class="login-form"
            @submit="onSubmit"
          >
            <a-form-item
              field="username"
              label="用户名"
              :rules="[{ required: true, message: '请输入用户名' }]"
            >
              <a-input
                v-model="form.username"
                :placeholder="isLocalLogin ? 'admin' : '域账号'"
                allow-clear
                @focus="activeField = 'username'"
                @blur="activeField = ''"
              />
            </a-form-item>
            <a-form-item
              field="password"
              label="密码"
              :rules="[{ required: true, message: '请输入密码' }]"
            >
              <a-input-password
                v-model="form.password"
                placeholder="请输入密码"
                allow-clear
                @focus="activeField = 'password'"
                @blur="activeField = ''"
              />
            </a-form-item>
            <a-button
              type="primary"
              long
              :loading="loading"
              html-type="submit"
              class="login-submit"
            >
              登录控制台
            </a-button>
          </a-form>

          <div
            v-if="oauthProviders.length > 0"
            class="oauth-section"
          >
            <a-divider>或使用企业 SSO</a-divider>
            <a-space
              direction="vertical"
              fill
            >
              <a-button
                v-for="p in oauthProviders"
                :key="p.id"
                long
                :loading="oauthLoadingId === p.id"
                @click="onOAuthLogin(p.id)"
              >
                {{ p.name }}
              </a-button>
            </a-space>
          </div>

          <a-alert
            type="info"
            show-icon
            class="login-tip"
            :closable="false"
          >
            本地联调默认账号见 configs/config.example.yaml；企业 LDAP/AD 需在 identity.providers 中启用。
          </a-alert>
        </a-card>

        <footer class="access-footer">
          <span>Bearer Token</span>
          <span>RBAC</span>
          <span>Trace &amp; Audit</span>
        </footer>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Message from '@arco-design/web-vue/es/message'
import {
  fetchLoginProviders,
  fetchOAuthAuthorizeURL,
  type IdentityProviderInfo
} from '@/api/identity'
import { useAuthStore } from '@/stores/auth'
import organicHeroImage from '@/assets/aiops-hero-organic-dense.webp'
import mechanicalHeroImage from '@/assets/aiops-hero-mechanical.webp'
import MorphCanvas from './components/MorphCanvas.vue'

type MorphCanvasExpose = {
  setMorphState: (state: {
    progress: number
    x: number
    y: number
    active: boolean
    energy: number
  }) => void
}

const OAUTH_STATE_PREFIX = 'aiops_oauth_state:'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const oauthLoadingId = ref('')
const providers = ref<IdentityProviderInfo[]>([])
const activeProviderId = ref('local')
const activeField = ref<'username' | 'password' | ''>('')
const form = reactive({ username: '', password: '' })
const sceneRef = ref<HTMLElement | null>(null)
const morphCanvasRef = ref<MorphCanvasExpose | null>(null)
const morphReady = ref(false)
const isCollapsed = ref(true)
let pointerFrame = 0
let pointerX = 0.5
let pointerY = 0.64
let renderedPointerX = 0.5
let renderedPointerY = 0.64
let morphActive = false

const passwordProviders = computed(() =>
  providers.value.filter((p) => p.type === 'ldap' || p.type === 'ad')
)
const oauthProviders = computed(() =>
  providers.value.filter((p) => p.type === 'oauth2' || p.type === 'oidc' || p.type === 'sso')
)
const isLocalLogin = computed(() => activeProviderId.value === 'local')
const loginModeLabel = computed(() => {
  if (isLocalLogin.value) return '使用本地账号密码登录'
  const p = passwordProviders.value.find((item) => item.id === activeProviderId.value)
  return p ? `使用 ${p.name} 登录` : '使用企业域账号登录'
})
const pageStateClass = computed(() => ({
  'is-username': activeField.value === 'username',
  'is-password': activeField.value === 'password',
  'is-morph-ready': morphReady.value
}))

function applyScenePointer() {
  pointerFrame = 0
  const scene = sceneRef.value
  if (!scene) return

  const deltaX = pointerX - renderedPointerX
  const deltaY = pointerY - renderedPointerY
  renderedPointerX += deltaX * 0.18
  renderedPointerY += deltaY * 0.18

  const normalizedPosition = Math.max(0, Math.min(1, (renderedPointerX - 0.12) / 0.7))
  const guidePosition = Math.pow(normalizedPosition, 1.25) * 100
  const morphPosition = -16 + (guidePosition / 100) * 132
  const movement = Math.min(1, Math.hypot(deltaX, deltaY) * 10)
  const morphEnergy = morphActive ? Math.max(0.28, movement) : 0

  scene.style.setProperty('--morph-progress', String(guidePosition / 100))
  scene.style.setProperty('--morph-position', `${morphPosition}%`)
  scene.style.setProperty('--morph-guide-position', `${guidePosition}%`)
  scene.style.setProperty('--morph-x', `${renderedPointerX * 100}%`)
  scene.style.setProperty('--morph-y', `${renderedPointerY * 100}%`)
  scene.style.setProperty('--morph-active', morphActive ? '1' : '0')
  scene.style.setProperty('--morph-energy', String(morphEnergy))
  morphCanvasRef.value?.setMorphState({
    progress: guidePosition / 100,
    x: renderedPointerX,
    y: renderedPointerY,
    active: morphActive,
    energy: morphEnergy
  })

  if (Math.abs(deltaX) > 0.0008 || Math.abs(deltaY) > 0.0008) {
    pointerFrame = window.requestAnimationFrame(applyScenePointer)
  }
}

function scheduleScenePointer() {
  if (!pointerFrame) {
    pointerFrame = window.requestAnimationFrame(applyScenePointer)
  }
}

function onScenePointerMove(event: PointerEvent) {
  if (event.pointerType === 'touch') return
  const scene = sceneRef.value
  const target = event.target as HTMLElement | null
  if (!scene) return

  const rect = scene.getBoundingClientRect()
  const nextPointerX = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
  const nextPointerY = Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height))
  const subjectTop = 0.15 - nextPointerX * 0.15 - nextPointerX * nextPointerX * 0.25
  const subjectBottom = 1.15 - nextPointerX * 0.08 - nextPointerX * nextPointerX * 0.15
  const isOverSubject = nextPointerY >= subjectTop && nextPointerY <= subjectBottom

  if (!isOverSubject || target?.closest('.login-access')) {
    if (morphActive) {
      morphActive = false
      scheduleScenePointer()
    }
    return
  }

  morphActive = true
  pointerX = nextPointerX
  pointerY = nextPointerY
  scheduleScenePointer()
}

function onScenePointerLeave() {
  morphActive = false
  scheduleScenePointer()
}

onMounted(async () => {
  await handleOAuthCallbackIfNeeded()
  try {
    const resp = await fetchLoginProviders()
    providers.value = resp.providers ?? []
  } catch {
    providers.value = []
  }
})

onBeforeUnmount(() => {
  if (pointerFrame) window.cancelAnimationFrame(pointerFrame)
})

async function handleOAuthCallbackIfNeeded() {
  const code = typeof route.query.code === 'string' ? route.query.code : ''
  const state = typeof route.query.state === 'string' ? route.query.state : ''
  if (!code || !state) return

  const providerId = sessionStorage.getItem(`${OAUTH_STATE_PREFIX}${state}`)
  if (!providerId) {
    Message.error('无法识别 SSO 身份源，请重新登录')
    return
  }

  loading.value = true
  try {
    await auth.loginWithOAuthCallback(providerId, code, state)
    sessionStorage.removeItem(`${OAUTH_STATE_PREFIX}${state}`)
    Message.success('登录成功')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.replace(redirect || '/dashboard')
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'SSO 登录失败'
    Message.error(msg)
  } finally {
    loading.value = false
  }
}

async function onSubmit() {
  if (!form.username.trim() || !form.password) {
    Message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const providerId = isLocalLogin.value ? undefined : activeProviderId.value
    await auth.login(form.username.trim(), form.password, providerId)
    Message.success('登录成功')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    router.replace(redirect || '/dashboard')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '登录失败'
    Message.error(msg)
  } finally {
    loading.value = false
  }
}

async function onOAuthLogin(providerId: string) {
  oauthLoadingId.value = providerId
  try {
    const resp = await fetchOAuthAuthorizeURL(providerId)
    sessionStorage.setItem(`${OAUTH_STATE_PREFIX}${resp.state}`, providerId)
    window.location.href = resp.authorization_url
  } catch (err) {
    const msg = err instanceof Error ? err.message : '无法发起 SSO 登录'
    Message.error(msg)
  } finally {
    oauthLoadingId.value = ''
  }
}
</script>

<style scoped lang="scss">
.login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 20px;
  color: #171b17;
  background: #deded6;
}

.login-frame {
  width: min(1500px, 100%);
  min-height: min(900px, calc(100vh - 40px));
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(390px, 0.72fr);
  overflow: hidden;
  border: 1px solid rgba(24, 27, 24, 0.42);
  border-radius: 18px;
  background: #f6f5ef;
  box-shadow: 0 30px 80px rgba(37, 41, 34, 0.12);
}

.login-visual {
  position: relative;
  min-height: 680px;
  overflow: hidden;
  border-right: 1px solid rgba(24, 27, 24, 0.22);
  background: #f1efe8;
  isolation: isolate;
}

.login-visual::before {
  content: '';
  position: absolute;
  inset: 14px;
  z-index: 2;
  pointer-events: none;
  border: 1px solid rgba(24, 27, 24, 0.3);
  border-radius: 10px;
}

.login-visual::after {
  content: '';
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
  opacity: 0.25;
  background-image: radial-gradient(rgba(27, 31, 25, 0.28) 0.6px, transparent 0.7px);
  background-size: 7px 7px;
  mix-blend-mode: multiply;
}

.login-visual-image {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: 58% center;
  transform: scale(1.025);
  animation: visual-drift 18s ease-in-out infinite alternate;
}

.brand-bar {
  position: absolute;
  top: 34px;
  right: 36px;
  left: 36px;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #262b25;
  font-size: 11px;
}

.brand-sign {
  display: flex;
  align-items: center;
  gap: 8px;
  font-family: var(--aiops-display);
  font-style: italic;
  font-weight: 600;
}

.brand-star {
  font-size: 16px;
}

.brand-index {
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.14em;
}

.visual-copy {
  position: absolute;
  top: 18%;
  left: clamp(38px, 7vw, 108px);
  z-index: 3;
  max-width: 660px;
}

.visual-kicker {
  display: block;
  margin-bottom: 20px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.visual-copy h1 {
  margin: 0;
  font-family: var(--aiops-display);
  font-size: clamp(42px, 5vw, 76px);
  font-weight: 400;
  letter-spacing: -0.05em;
  line-height: 1.06;
}

.visual-copy h1 em {
  font-weight: 400;
}

.visual-copy p {
  max-width: 500px;
  margin: 26px 0 0;
  color: #565c53;
  font-size: 14px;
  line-height: 1.8;
}

.workflow-line {
  position: absolute;
  right: 40px;
  bottom: 42px;
  left: 40px;
  z-index: 3;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  border-top: 1px solid rgba(24, 27, 24, 0.25);
}

.workflow-line span {
  padding: 12px 8px 0;
  color: #4f554d;
  font-size: 11px;
}

.workflow-line b {
  margin-right: 6px;
  color: #252b24;
  font-family: var(--aiops-display);
  font-style: italic;
}

.visual-caption {
  position: absolute;
  top: 50%;
  right: 26px;
  z-index: 3;
  color: rgba(33, 38, 32, 0.72);
  font-family: var(--aiops-display);
  font-size: 10px;
  font-style: italic;
  writing-mode: vertical-rl;
}

.login-access {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 64px clamp(28px, 4vw, 64px) 42px;
  background: #f7f6f0;
  transition: background 220ms ease;
}

.is-username .login-access,
.is-password .login-access {
  background: #fbfaf5;
}

.access-meta {
  position: absolute;
  top: 28px;
  right: 34px;
  left: 34px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #747970;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.collapse-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  color: #3a4237;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.1em;
  cursor: pointer;
  transition: all 250ms ease;
}

.collapse-toggle:hover {
  color: #171b17;
  transform: scale(1.03);
}

.collapse-toggle span {
  position: relative;
}

.collapse-toggle span::after {
  content: '';
  position: absolute;
  bottom: -4px;
  left: 50%;
  width: 0;
  height: 2px;
  background: linear-gradient(90deg, #5d7648, #8ba872);
  border-radius: 1px;
  transform: translateX(-50%);
  transition: width 250ms ease;
}

.collapse-toggle:hover span::after {
  width: 100%;
}

.collapsed-login-card {
  position: absolute;
  top: 48%;
  right: 5%;
  transform: translateY(-50%);
  z-index: 10;
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 220px;
  padding: 12px 18px;
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(12px);
  border-radius: 12px;
  box-shadow:
    0 4px 20px rgba(33, 38, 32, 0.06),
    0 1px 4px rgba(33, 38, 32, 0.03);
  cursor: pointer;
  transition: all 300ms ease;
  user-select: none;
}

.collapsed-login-card:hover {
  background: rgba(255, 255, 255, 0.98);
  box-shadow:
    0 12px 48px rgba(33, 38, 32, 0.12),
    0 4px 12px rgba(33, 38, 32, 0.06);
  transform: translateY(-3px);
}

.collapsed-login-header {
  text-align: left;
  color: #747970;
  font-size: 9px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.collapsed-login-content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}

.collapsed-login-text {
  text-align: left;
  color: #171b17;
  font-family: var(--aiops-display);
  font-size: 18px;
  font-weight: 400;
  letter-spacing: -0.01em;
}

.collapsed-login-arrow {
  color: #5d7648;
  flex-shrink: 0;
  transition: transform 250ms ease;
}

.collapsed-login-card:hover .collapsed-login-arrow {
  transform: translate(4px, -4px);
}

.login-access.is-collapsed {
  display: none;
}

.login-access.is-collapsed:hover {
  background: transparent !important;
}

.access-state {
  display: flex;
  align-items: center;
  gap: 7px;
}

.access-state i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #5d7648;
  box-shadow: 0 0 0 4px rgba(93, 118, 72, 0.1);
}

.login-card {
  width: 100%;
  max-width: 430px;
  max-height: 600px;
  margin: 0 auto;
  border: 0 !important;
  background: transparent !important;
  backdrop-filter: none;
  overflow: hidden;
  opacity: 1;
  transform: translateY(0);
  transition:
    max-height 350ms cubic-bezier(0.4, 0, 0.2, 1),
    opacity 280ms ease,
    transform 280ms ease;
}

.login-card :deep(.arco-card-body) {
  padding: 0;
}

.login-index {
  margin-bottom: 18px;
  color: #747970;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.login-title {
  color: #171b17;
  font-family: var(--aiops-display);
  font-size: 42px;
  font-style: italic;
  font-weight: 400;
  letter-spacing: -0.04em;
}

.login-sub {
  margin-top: 9px;
  color: var(--aiops-text-soft);
  font-size: 13px;
}

.login-tabs {
  margin: 26px 0 4px;
}

.login-form {
  margin-top: 28px;
}

.login-form :deep(.arco-form-item-label-col) {
  padding-bottom: 7px;
  color: #4e544c;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.login-form :deep(.arco-input-wrapper) {
  height: 44px;
  border-radius: 0;
  border-width: 0 0 1px;
  background: transparent;
  box-shadow: none;
}

.login-submit {
  height: 46px;
  margin-top: 8px;
  border-radius: 999px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.oauth-section {
  margin-top: 10px;
}

.login-tip {
  margin-top: 20px;
  border: 1px solid rgba(49, 90, 99, 0.14);
  border-radius: 8px;
  background: rgba(236, 241, 237, 0.72);
  font-size: 11px;
  line-height: 1.6;
}

.access-footer {
  position: absolute;
  right: 34px;
  bottom: 22px;
  left: 34px;
  display: flex;
  justify-content: space-between;
  color: #8a8e86;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  opacity: 1;
  transition: opacity 250ms ease;
}

.login-access {
  transition: background 220ms ease, min-width 300ms ease;
}

.login-access.is-collapsed {
  min-width: auto;
  background: transparent !important;
}

.is-collapsed .login-access {
  background: transparent !important;
}

@keyframes visual-drift {
  from {
    transform: scale(1.025) translate3d(-0.8%, 0, 0);
  }
  to {
    transform: scale(1.075) translate3d(1.2%, -0.8%, 0);
  }
}

@media (max-width: 1050px) {
  .login-page {
    padding: 0;
  }

  .login-frame {
    min-height: 100vh;
    grid-template-columns: 1fr;
    border: 0;
    border-radius: 0;
  }

  .login-visual {
    min-height: 330px;
    border-right: 0;
    border-bottom: 1px solid rgba(24, 27, 24, 0.2);
  }

  .visual-copy {
    top: 26%;
    left: 38px;
  }

  .visual-copy h1 {
    font-size: clamp(36px, 8vw, 58px);
  }

  .visual-copy p,
  .workflow-line,
  .visual-caption {
    display: none;
  }

  .login-access {
    min-height: 600px;
  }
}

@media (max-width: 560px) {
  .login-visual {
    min-height: 270px;
  }

  .brand-bar {
    top: 26px;
    right: 26px;
    left: 26px;
  }

  .brand-index {
    display: none;
  }

  .visual-copy {
    top: 32%;
    left: 26px;
  }

  .visual-kicker {
    margin-bottom: 12px;
    font-size: 8px;
  }

  .visual-copy h1 {
    font-size: 35px;
  }

  .login-access {
    min-height: 610px;
    padding: 64px 24px 54px;
  }

  .login-title {
    font-size: 36px;
  }
}
</style>

<style scoped lang="scss">
/* Cursor-controlled material morph and collapsible authentication surface. */
.login-frame {
  --morph-progress: 0.5;
  --morph-position: 50%;
  --morph-guide-position: 50%;
  --morph-x: 50%;
  --morph-y: 64%;
  --morph-active: 0;
  --morph-energy: 0;
}

.login-visual-image-organic,
.login-visual-image-mechanical {
  inset: -3%;
  width: 106%;
  height: 106%;
  transform: scale(1.035);
  transform-origin: center;
}

.login-visual-image-organic {
  z-index: 0;
  animation: organic-breathe 18s ease-in-out infinite alternate;
}

.login-visual-image-mechanical {
  z-index: 1;
  opacity: 0.99;
  filter: saturate(0.92) contrast(1.02);
  mask-image: linear-gradient(
    90deg,
    transparent 0%,
    transparent calc(var(--morph-position) - 32%),
    rgba(0, 0, 0, 0.08) calc(var(--morph-position) - 22%),
    rgba(0, 0, 0, 0.5) var(--morph-position),
    rgba(0, 0, 0, 0.92) calc(var(--morph-position) + 22%),
    #000 calc(var(--morph-position) + 32%),
    #000 100%
  );
  -webkit-mask-image: linear-gradient(
    90deg,
    transparent 0%,
    transparent calc(var(--morph-position) - 32%),
    rgba(0, 0, 0, 0.08) calc(var(--morph-position) - 22%),
    rgba(0, 0, 0, 0.5) var(--morph-position),
    rgba(0, 0, 0, 0.92) calc(var(--morph-position) + 22%),
    #000 calc(var(--morph-position) + 32%),
    #000 100%
  );
  transition: mask-image 60ms linear, -webkit-mask-image 60ms linear;
  will-change: mask-image;
}

.login-morph-canvas {
  z-index: 2;
  opacity: 0;
  transition: opacity 320ms ease;
}

.is-morph-ready .login-morph-canvas {
  opacity: 1;
}

.is-morph-ready .login-morph-lens {
  display: none;
}

.login-morph-lens {
  position: absolute;
  inset: 0;
  z-index: 3;
  overflow: hidden;
  opacity: var(--morph-active);
  clip-path: polygon(0 64%, 23% 63%, 47% 56%, 100% 23%, 100% 62%, 72% 80%, 48% 92%, 31% 100%, 0 100%);
  filter: saturate(1.12) contrast(1.04) drop-shadow(0 7px 12px rgba(42, 53, 27, 0.12));
  mask-image: radial-gradient(
    ellipse 16% 22% at var(--morph-x) var(--morph-y),
    #000 0%,
    rgba(0, 0, 0, 0.96) 34%,
    rgba(0, 0, 0, 0.62) 57%,
    rgba(0, 0, 0, 0.18) 76%,
    transparent 100%
  );
  -webkit-mask-image: radial-gradient(
    ellipse 16% 22% at var(--morph-x) var(--morph-y),
    #000 0%,
    rgba(0, 0, 0, 0.96) 34%,
    rgba(0, 0, 0, 0.62) 57%,
    rgba(0, 0, 0, 0.18) 76%,
    transparent 100%
  );
  pointer-events: none;
  transition: opacity 160ms ease;
  will-change: opacity, mask-image;
}

.login-morph-lens::after {
  content: '';
  width: clamp(150px, 19vw, 280px);
  aspect-ratio: 1.45;
  position: absolute;
  top: var(--morph-y);
  left: var(--morph-x);
  border: 1px solid rgba(219, 225, 198, 0.18);
  border-radius: 50%;
  box-shadow: inset 0 0 34px rgba(221, 230, 198, 0.09);
  transform: translate(-50%, -50%) rotate(-8deg);
}

.login-morph-lens-image {
  z-index: 0;
  transform: scale(1.085);
  transform-origin: var(--morph-x) var(--morph-y);
  transition: transform 180ms ease-out;
  will-change: transform;
}

.moss-burst {
  position: absolute;
  inset: 0;
  z-index: 5;
  opacity: var(--morph-energy);
  pointer-events: none;
  transition: opacity 140ms ease;
}

.moss-burst::before,
.moss-burst::after {
  content: '';
  position: absolute;
  top: var(--morph-y);
  left: var(--morph-x);
  border-radius: 50%;
  background:
    radial-gradient(circle at 9% 52%, rgba(72, 88, 39, 0.85) 0 2px, transparent 3px),
    radial-gradient(circle at 22% 24%, rgba(93, 107, 53, 0.78) 0 1.5px, transparent 3px),
    radial-gradient(circle at 36% 72%, rgba(58, 73, 32, 0.72) 0 2.5px, transparent 4px),
    radial-gradient(circle at 58% 13%, rgba(108, 120, 64, 0.72) 0 2px, transparent 3.5px),
    radial-gradient(circle at 71% 69%, rgba(62, 78, 35, 0.82) 0 1.5px, transparent 3px),
    radial-gradient(circle at 88% 38%, rgba(85, 98, 48, 0.78) 0 2.5px, transparent 4px);
  mix-blend-mode: multiply;
  filter: blur(0.2px) drop-shadow(0 2px 3px rgba(42, 51, 24, 0.2));
  transform: translate(-50%, -50%) rotate(-10deg);
}

.moss-burst::before {
  width: clamp(170px, 24vw, 360px);
  height: clamp(90px, 14vw, 210px);
  animation: moss-scatter-primary 720ms ease-in-out infinite alternate;
}

.moss-burst::after {
  width: clamp(120px, 17vw, 250px);
  height: clamp(70px, 11vw, 160px);
  opacity: 0.65;
  animation: moss-scatter-secondary 940ms ease-in-out infinite alternate-reverse;
}

.morph-interaction-zone {
  position: absolute;
  inset: 0;
  z-index: 5;
  clip-path: polygon(0 64%, 23% 63%, 47% 56%, 100% 23%, 100% 62%, 72% 80%, 48% 92%, 31% 100%, 0 100%);
  cursor: ew-resize;
}

.morph-guide {
  position: absolute;
  right: 42px;
  bottom: 42px;
  z-index: 7;
  width: min(320px, 32vw);
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  color: rgba(33, 38, 32, 0.7);
  font-size: 8px;
  font-weight: 700;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.morph-guide i {
  height: 1px;
  position: relative;
  display: block;
  background: rgba(24, 27, 24, 0.24);
}

.morph-guide b {
  width: 8px;
  height: 8px;
  position: absolute;
  top: 50%;
  left: var(--morph-guide-position);
  border: 1px solid rgba(24, 27, 24, 0.46);
  border-radius: 50%;
  background: #f4f2ea;
  box-shadow: 0 0 0 4px rgba(244, 242, 234, 0.5);
  transform: translate(-50%, -50%);
  transition: left 80ms linear;
}

@keyframes organic-breathe {
  from {
    transform: scale(1.035) translate3d(-0.35%, 0, 0);
  }
  to {
    transform: scale(1.055) translate3d(0.35%, -0.25%, 0);
  }
}

@keyframes moss-scatter-primary {
  from {
    opacity: 0.42;
    transform: translate(-50%, -50%) rotate(-11deg) scale(0.92);
  }
  to {
    opacity: 0.78;
    transform: translate(-50%, -50%) rotate(-7deg) scale(1.06);
  }
}

@keyframes moss-scatter-secondary {
  from {
    opacity: 0.24;
    transform: translate(-50%, -50%) rotate(8deg) scale(0.9);
  }
  to {
    opacity: 0.58;
    transform: translate(-50%, -50%) rotate(13deg) scale(1.1);
  }
}

@media (max-width: 1120px) {
  .morph-guide {
    width: 250px;
  }
}

@media (max-width: 820px) {
  .morph-guide {
    right: 24px;
    bottom: 28px;
    left: 24px;
    width: auto;
  }

}

@media (hover: none), (pointer: coarse), (prefers-reduced-motion: reduce) {
  .login-visual-image-organic {
    animation: none;
  }

  .login-visual-image-mechanical {
    transition: none;
  }

  .login-morph-lens,
  .moss-burst {
    display: none;
  }

  .morph-interaction-zone {
    pointer-events: none;
  }
}
</style>

<style scoped lang="scss">
/* Layered login composition: background at the bottom, authentication panel above it. */
.login-page {
  display: block;
  min-height: 100vh;
  padding: 14px;
  overflow: auto;
  background: #dcdcd4;
}

.login-frame {
  --cursor-x: 50%;
  --cursor-y: 50%;
  --moss-shift-x: 0px;
  --moss-shift-y: 0px;
  --moss-opacity: 0;
  width: min(1600px, 100%);
  min-height: calc(100vh - 28px);
  position: relative;
  display: block;
  margin: 0 auto;
  overflow: hidden;
  border: 1px solid rgba(24, 27, 24, 0.42);
  border-radius: 18px;
  background: #eeeae0;
  box-shadow: 0 30px 80px rgba(37, 41, 34, 0.14);
  isolation: isolate;
}

.login-visual {
  position: absolute;
  inset: 0;
  min-height: 0;
  overflow: hidden;
  border: 0;
  background: #eeeae0;
}

.login-visual::before {
  inset: 14px;
  z-index: 5;
  border-color: rgba(24, 27, 24, 0.3);
}

.login-visual::after {
  z-index: 2;
  opacity: 1;
  background:
    linear-gradient(90deg, rgba(245, 242, 233, 0.02) 0%, rgba(245, 242, 233, 0.03) 45%, rgba(245, 242, 233, 0.48) 100%),
    radial-gradient(rgba(27, 31, 25, 0.2) 0.55px, transparent 0.7px);
  background-size: 100% 100%, 8px 8px;
  mix-blend-mode: normal;
}

.login-visual-image {
  position: absolute;
  inset: -3%;
  width: 106%;
  height: 106%;
  object-fit: cover;
  object-position: 58% center;
}

.login-visual-image-base {
  z-index: 0;
  transform: scale(1.035);
  animation: background-drift 20s ease-in-out infinite alternate;
}

.login-visual-image-reactive {
  z-index: 1;
  opacity: var(--moss-opacity);
  transform: translate3d(var(--moss-shift-x), var(--moss-shift-y), 0) scale(1.065);
  filter: url('#moss-cursor-distort') saturate(1.18) contrast(1.04);
  mask-image: radial-gradient(
    circle 190px at var(--cursor-x) var(--cursor-y),
    #000 0%,
    rgba(0, 0, 0, 0.94) 42%,
    rgba(0, 0, 0, 0.36) 68%,
    transparent 82%
  );
  -webkit-mask-image: radial-gradient(
    circle 190px at var(--cursor-x) var(--cursor-y),
    #000 0%,
    rgba(0, 0, 0, 0.94) 42%,
    rgba(0, 0, 0, 0.36) 68%,
    transparent 82%
  );
  pointer-events: none;
  transition: opacity 180ms ease;
  will-change: transform, opacity, mask-image;
}

.effect-definitions {
  position: absolute;
  pointer-events: none;
}

.brand-bar {
  top: 36px;
  right: 38px;
  left: 38px;
  z-index: 6;
}

.visual-copy {
  top: 50%;
  left: clamp(42px, 6vw, 104px);
  z-index: 6;
  width: min(48vw, 690px);
  max-width: none;
  transform: translateY(-58%);
}

.visual-copy h1 {
  font-size: clamp(48px, 5.2vw, 82px);
}

.visual-copy p {
  display: block;
  max-width: 480px;
}

.workflow-line {
  right: min(47vw, 600px);
  bottom: 30px;
  left: 42px;
  z-index: 7;
  padding: 0 12px;
  border: 1px solid rgba(24, 27, 24, 0.2);
  border-radius: 8px;
  background: rgba(249, 248, 242, 0.82);
  box-shadow: 0 10px 30px rgba(28, 33, 26, 0.08);
  backdrop-filter: blur(14px) saturate(0.9);
}

.workflow-line span {
  padding: 12px 8px;
  color: #41473f;
}

.workflow-line b {
  color: #20251f;
}

.visual-caption {
  display: none;
}

.login-access {
  position: absolute;
  top: 50%;
  right: clamp(30px, 5vw, 78px);
  z-index: 8;
  width: min(430px, 36vw);
  min-height: 0;
  display: block;
  padding: 28px 32px 24px;
  border: 1px solid rgba(24, 27, 24, 0.28);
  border-radius: 14px;
  background: rgba(248, 247, 241, 0.86);
  box-shadow: 0 30px 70px rgba(30, 35, 28, 0.18);
  backdrop-filter: blur(20px) saturate(0.9);
  transform: translateY(-50%);
  transition: background 220ms ease, box-shadow 220ms ease, transform 220ms ease;
}

.login-access::before {
  content: '';
  position: absolute;
  top: 0;
  right: 28px;
  left: 28px;
  height: 2px;
  background: #4e6540;
  transform: scaleX(0.22);
  transform-origin: left;
  transition: transform 260ms ease;
}

.is-username .login-access,
.is-password .login-access {
  background: rgba(255, 254, 249, 0.94);
  box-shadow: 0 34px 80px rgba(30, 35, 28, 0.23);
  transform: translateY(-50%) translateY(-3px);
}

.is-username .login-access::before,
.is-password .login-access::before {
  transform: scaleX(1);
}

.access-meta {
  position: static;
  margin-bottom: 28px;
  padding-right: 34px;
}

.login-card {
  width: 100%;
  max-width: none;
}

.login-card :deep(.arco-card-body) {
  padding: 0;
}

.login-title {
  font-size: clamp(34px, 3.2vw, 44px);
}

.login-form {
  margin-top: 24px;
}

.login-submit,
.login-submit:not(.arco-btn-disabled) {
  border-color: #263328 !important;
  color: #f8fbf7 !important;
  background: #263328 !important;
}

.login-submit:not(.arco-btn-disabled):hover {
  border-color: #3f5135 !important;
  background: #3f5135 !important;
}

.login-tip {
  margin-top: 18px;
}

.access-footer {
  position: static;
  margin-top: 22px;
  padding-top: 16px;
  border-top: 1px solid rgba(24, 27, 24, 0.1);
}

@keyframes background-drift {
  from {
    transform: scale(1.035) translate3d(-0.5%, 0, 0);
  }
  to {
    transform: scale(1.075) translate3d(0.8%, -0.6%, 0);
  }
}

@media (max-width: 1120px) {
  .visual-copy {
    left: 38px;
    width: 45vw;
  }

  .visual-copy h1 {
    font-size: clamp(46px, 5.7vw, 64px);
  }

  .workflow-line,
  .visual-caption {
    display: none;
  }

  .login-access {
    right: 28px;
    width: min(400px, 43vw);
  }
}

@media (max-width: 820px) {
  .login-page {
    padding: 0;
  }

  .login-frame {
    min-height: 820px;
    border: 0;
    border-radius: 0;
  }

  .login-visual {
    position: absolute;
    min-height: 0;
    border: 0;
  }

  .visual-copy,
  .workflow-line,
  .visual-caption {
    display: none;
  }

  .login-visual-image {
    object-position: 64% center;
  }

  .login-access {
    top: 50%;
    right: auto;
    left: 50%;
    width: min(470px, calc(100% - 40px));
    min-height: 0;
    transform: translate(-50%, -47%);
  }

  .is-username .login-access,
  .is-password .login-access {
    transform: translate(-50%, -47%) translateY(-3px);
  }
}

@media (max-width: 560px) {
  .brand-bar {
    top: 24px;
    right: 24px;
    left: 24px;
  }

  .login-visual {
    min-height: 0;
  }

  .login-access {
    width: calc(100% - 30px);
    padding: 26px 22px 22px;
  }

  .login-title {
    font-size: 35px;
  }

  .access-footer {
    gap: 12px;
  }
}

@media (hover: none), (pointer: coarse) {
  .login-visual-image-reactive {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .login-visual-image-base {
    animation: none;
  }

  .login-visual-image-reactive {
    display: none;
  }
}
</style>
