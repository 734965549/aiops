import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { useLocale } from '@arco-design/web-vue/es/locale'
import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn'

import App from './App.vue'
import router from './router'
import './styles/index.scss'

useLocale(zhCN.locale)

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
