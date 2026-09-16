import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin, QueryClient } from '@tanstack/vue-query'
import 'ant-design-vue/dist/reset.css'
import App from './App.vue'
import router from './router'
import './styles/global.scss'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,        // 30 秒内认为数据新鲜,不重新请求
      gcTime: 5 * 60_000,      // 5 分钟后清理未使用的缓存
      refetchOnWindowFocus: true, // 切回窗口时自动刷新
      retry: 1,                  // 失败重试 1 次
    },
  },
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(VueQueryPlugin, { queryClient })

// 全局错误处理:渲染错误不静默吞掉,方便定位生产问题
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Error]', info, err)
}

// 未捕获的 Promise rejection
window.addEventListener('unhandledrejection', (event) => {
  console.error('[Unhandled Rejection]', event.reason)
})

app.mount('#app')
