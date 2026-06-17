import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ArcoVue from '@arco-design/web-vue'
import ArcoVueIcon from '@arco-design/web-vue/es/icon'
import '@arco-design/web-vue/dist/arco.css'
import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn'

import App from './App.vue'
import router from './router'
import './styles/index.scss'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ArcoVue, { locale: zhCN })
app.use(ArcoVueIcon)
app.mount('#app')
