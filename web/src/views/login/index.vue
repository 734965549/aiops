<template>
  <div
    class="login-page"
    :class="pageStateClass"
  >
    <div class="login-bg-grid" />
    <div class="login-orbit orbit-one" />
    <div class="login-orbit orbit-two" />

    <section class="brand-panel">
      <div class="brand-kicker">
        AIOps Neural Console
      </div>
      <h1>让告警、资产与自动化执行形成闭环</h1>
      <p>
        统一接入身份源、Runbook、告警与 AI 工具网关，把运维动作沉淀成可追踪、可审计的智能控制台。
      </p>

      <div class="signal-card tech-scanline">
        <div class="signal-header">
          <span class="pulse-dot" />
          实时态势感知
        </div>
        <div class="signal-lines">
          <span />
          <span />
          <span />
        </div>
      </div>
    </section>

    <a-card
      class="login-card"
      :bordered="false"
    >
      <div class="sentinel-stage">
        <div
          v-for="idx in 3"
          :key="idx"
          class="sentinel"
          :class="sentinelClass"
        >
          <span class="sentinel-ear left" />
          <span class="sentinel-ear right" />
          <span class="sentinel-head">
            <span class="sentinel-eye left" />
            <span class="sentinel-eye right" />
            <span class="sentinel-visor" />
          </span>
          <span class="sentinel-body" />
          <span class="sentinel-hand left" />
          <span class="sentinel-hand right" />
        </div>
      </div>

      <div class="login-title">
        AI 运维平台
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  fetchLoginProviders,
  fetchOAuthAuthorizeURL,
  type IdentityProviderInfo
} from '@/api/identity'
import { useAuthStore } from '@/stores/auth'

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
  'is-password': activeField.value === 'password'
}))
const sentinelClass = computed(() => ({
  'sentinel-watch': activeField.value === 'username',
  'sentinel-hide': activeField.value === 'password'
}))

onMounted(async () => {
  await handleOAuthCallbackIfNeeded()
  try {
    const resp = await fetchLoginProviders()
    providers.value = resp.providers ?? []
  } catch {
    providers.value = []
  }
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
  position: relative;
  display: grid;
  grid-template-columns: minmax(320px, 560px) 440px;
  align-items: center;
  justify-content: center;
  gap: 54px;
  padding: 48px;
  overflow: hidden;
  color: #eaf7ff;
  background:
    radial-gradient(circle at 16% 20%, rgba(22, 93, 255, 0.38), transparent 30%),
    radial-gradient(circle at 82% 16%, rgba(0, 220, 197, 0.26), transparent 28%),
    linear-gradient(135deg, #061126 0%, #0a1731 52%, #071022 100%);
}

.login-bg-grid {
  position: absolute;
  inset: 0;
  opacity: 0.55;
  background-image:
    linear-gradient(rgba(125, 211, 252, 0.11) 1px, transparent 1px),
    linear-gradient(90deg, rgba(125, 211, 252, 0.11) 1px, transparent 1px);
  background-size: 44px 44px;
  transform: perspective(700px) rotateX(58deg) translateY(18%);
  transform-origin: bottom;
  animation: grid-drift 18s linear infinite;
}

.login-page::before,
.login-page::after {
  content: '';
  position: absolute;
  width: 430px;
  height: 430px;
  border-radius: 999px;
  filter: blur(36px);
  opacity: 0.42;
  pointer-events: none;
}

.login-page::before {
  left: -140px;
  bottom: -120px;
  background: rgba(22, 93, 255, 0.55);
}

.login-page::after {
  right: -120px;
  top: -90px;
  background: rgba(0, 220, 197, 0.38);
}

.login-orbit {
  position: absolute;
  border: 1px solid rgba(125, 211, 252, 0.16);
  border-radius: 999px;
  pointer-events: none;
}

.orbit-one {
  width: 560px;
  height: 560px;
  right: 5%;
  top: 12%;
  animation: spin 28s linear infinite;
}

.orbit-two {
  width: 300px;
  height: 300px;
  left: 13%;
  bottom: 14%;
  animation: spin 18s linear infinite reverse;
}

.brand-panel,
.login-card {
  position: relative;
  z-index: 1;
}

.brand-panel {
  max-width: 560px;
}

.brand-kicker {
  display: inline-flex;
  margin-bottom: 18px;
  padding: 6px 12px;
  border: 1px solid rgba(0, 220, 197, 0.28);
  border-radius: 999px;
  color: #8ff8ee;
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  background: rgba(0, 220, 197, 0.1);
}

.brand-panel h1 {
  margin: 0;
  max-width: 540px;
  font-size: 42px;
  line-height: 1.16;
  letter-spacing: 0;
}

.brand-panel p {
  margin: 18px 0 28px;
  max-width: 520px;
  color: rgba(226, 246, 255, 0.76);
  font-size: 15px;
  line-height: 1.8;
}

.signal-card {
  width: min(430px, 100%);
  padding: 18px;
  border: 1px solid rgba(125, 211, 252, 0.2);
  border-radius: 8px;
  background: rgba(5, 17, 38, 0.58);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.24);
  backdrop-filter: blur(16px);
}

.signal-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  font-size: 13px;
  color: #dffcff;
}

.pulse-dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #00dcc5;
  box-shadow: 0 0 18px rgba(0, 220, 197, 0.92);
}

.signal-lines {
  display: grid;
  gap: 10px;
}

.signal-lines span {
  height: 8px;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(0, 220, 197, 0.75), rgba(22, 93, 255, 0.06));
}

.signal-lines span:nth-child(2) {
  width: 72%;
}

.signal-lines span:nth-child(3) {
  width: 84%;
}

.login-card {
  width: 440px;
  padding: 8px;
  overflow: hidden;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(233, 246, 255, 0.88)),
    radial-gradient(circle at 50% 0, rgba(0, 220, 197, 0.22), transparent 42%) !important;
}

.login-card::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(120deg, transparent 0%, rgba(255, 255, 255, 0.32) 36%, transparent 58%);
  transform: translateX(-110%);
  animation: card-shine 7s ease-in-out infinite;
}

.sentinel-stage {
  height: 118px;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  gap: 14px;
  margin-bottom: 8px;
}

.sentinel {
  position: relative;
  width: 74px;
  height: 92px;
  transform: translateY(0);
  transition: transform 0.28s ease;
}

.sentinel:nth-child(1),
.sentinel:nth-child(3) {
  transform: translateY(10px) scale(0.88);
  opacity: 0.82;
}

.sentinel-head {
  position: absolute;
  top: 2px;
  left: 50%;
  width: 62px;
  height: 58px;
  border: 1px solid rgba(22, 93, 255, 0.2);
  border-radius: 18px;
  background: linear-gradient(180deg, #f8fdff, #c9f3ff);
  box-shadow: inset 0 -8px 16px rgba(22, 93, 255, 0.12), 0 14px 28px rgba(22, 93, 255, 0.14);
  transform: translateX(-50%);
  transition: transform 0.28s ease;
}

.sentinel-ear {
  position: absolute;
  top: 26px;
  width: 11px;
  height: 20px;
  border-radius: 8px;
  background: #9feaff;
}

.sentinel-ear.left {
  left: 2px;
}

.sentinel-ear.right {
  right: 2px;
}

.sentinel-eye {
  position: absolute;
  top: 24px;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #07223c;
  box-shadow: 0 0 10px rgba(0, 220, 197, 0.85);
  transition: transform 0.22s ease, opacity 0.18s ease;
}

.sentinel-eye.left {
  left: 18px;
}

.sentinel-eye.right {
  right: 18px;
}

.sentinel-visor {
  position: absolute;
  left: 13px;
  right: 13px;
  bottom: 14px;
  height: 4px;
  border-radius: 999px;
  background: linear-gradient(90deg, #165dff, #00dcc5);
  opacity: 0.8;
}

.sentinel-body {
  position: absolute;
  left: 50%;
  bottom: 0;
  width: 46px;
  height: 36px;
  border-radius: 14px 14px 10px 10px;
  background: linear-gradient(180deg, #3a7bff, #00c9b7);
  transform: translateX(-50%);
  box-shadow: 0 14px 28px rgba(0, 220, 197, 0.24);
}

.sentinel-hand {
  position: absolute;
  top: 53px;
  width: 13px;
  height: 32px;
  border-radius: 999px;
  background: #b9f2ff;
  transform-origin: top center;
  transition: transform 0.24s ease;
}

.sentinel-hand.left {
  left: 8px;
  transform: rotate(16deg);
}

.sentinel-hand.right {
  right: 8px;
  transform: rotate(-16deg);
}

.sentinel-watch .sentinel-head {
  transform: translateX(-50%) rotate(4deg);
}

.sentinel-watch .sentinel-eye {
  transform: translate(4px, 1px);
}

.sentinel-hide .sentinel-head {
  transform: translateX(-50%) rotate(-14deg);
}

.sentinel-hide .sentinel-eye {
  opacity: 0.25;
  transform: translate(-7px, 0);
}

.sentinel-hide .sentinel-hand.left {
  transform: translate(14px, -22px) rotate(76deg);
}

.sentinel-hide .sentinel-hand.right {
  transform: translate(-14px, -22px) rotate(-76deg);
}

.login-title {
  color: #111d33;
  font-size: 22px;
  font-weight: 800;
  text-align: center;
}

.login-sub {
  margin-top: 6px;
  color: var(--aiops-text-soft);
  font-size: 13px;
  text-align: center;
}

.login-tabs {
  margin: 22px 0 8px;
}

.login-form {
  margin-top: 20px;
}

.login-submit {
  height: 42px;
  font-weight: 700;
}

.oauth-section {
  margin-top: 8px;
}

.login-tip {
  margin-top: 16px;
  border-radius: 8px;
}

.is-username .orbit-one {
  border-color: rgba(0, 220, 197, 0.34);
}

.is-password .login-card {
  box-shadow: 0 24px 70px rgba(22, 93, 255, 0.28);
}

@keyframes grid-drift {
  to {
    background-position: 0 44px, 44px 0;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes card-shine {
  0%,
  48% {
    transform: translateX(-110%);
  }
  64%,
  100% {
    transform: translateX(110%);
  }
}

@media (max-width: 980px) {
  .login-page {
    grid-template-columns: 1fr;
    gap: 28px;
    padding: 28px 18px;
  }

  .brand-panel {
    max-width: 680px;
    text-align: center;
  }

  .brand-panel h1 {
    max-width: none;
    font-size: 30px;
  }

  .brand-panel p {
    margin-left: auto;
    margin-right: auto;
  }

  .signal-card {
    display: none;
  }

  .login-card {
    width: min(440px, 100%);
    margin: 0 auto;
  }
}
</style>
