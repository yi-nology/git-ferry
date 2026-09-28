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
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: true,
      retry: 1,
    },
  },
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(VueQueryPlugin, { queryClient })

app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Error]', info, err)
}

window.addEventListener('unhandledrejection', (event) => {
  console.error('[Unhandled Rejection]', event.reason)
})

app.mount('#app')
