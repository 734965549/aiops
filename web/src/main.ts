import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { useLocale } from '@arco-design/web-vue/es/locale'
import zhCN from '@arco-design/web-vue/es/locale/lang/zh-cn'
// Message/Modal 以函数方式调用，unplugin-vue-components 的 ArcoResolver
// 不会为其注入样式，必须手动引入，否则提示浮层会渲染在视口之外。
import '@arco-design/web-vue/es/message/style/index.css'
import '@arco-design/web-vue/es/modal/style/index.css'

import App from './App.vue'
import router from './router'
import './styles/index.scss'

useLocale(zhCN.locale)

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
