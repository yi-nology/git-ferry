import axios from 'axios'
import type { ApiResponse } from '@/types/api'
import { useAuthStore } from '@/stores/auth'

/**
 * 统一业务错误。拦截器把所有失败(网络/HTTP/认证/业务)都包装成 ApiError,
 * 业务层只需 `catch (e)` 后读取 `e.message`。
 */
export class ApiError extends Error {
  code: number
  constructor(message: string, code = 0) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

// 请求拦截器:注入 API Key
http.interceptors.request.use((config) => {
  const authStore = useAuthStore()
  const apiKey = authStore.getApiKey()
  if (apiKey) {
    config.headers['X-API-Key'] = apiKey
  }
  return config
})

/**
 * 响应拦截器:统一解包 + 5xx 自动重试(最多 2 次,指数退避)。
 */
http.interceptors.response.use(
  (response) => {
    // 204 No Content(删除类操作,空 body)
    if (response.status === 204 || response.data == null || response.data === '') {
      return { success: true }
    }
    const body = response.data
    // 标准包装 → 解包 data
    if (body && typeof body === 'object' && 'code' in body && 'data' in body) {
      const env = body as ApiResponse
      if (env.code >= 200 && env.code < 300) {
        return env.data
      }
      return Promise.reject(new ApiError(env.message || '请求失败', env.code))
    }
    // 非标准包装,原样返回
    return body
  },
  async (error) => {
    const config = error.config
    const status = error.response?.status

    // 5xx 自动重试(最多 2 次,指数退避)
    if (status >= 500 && status < 600) {
      const retryCount = config.__retryCount || 0
      if (retryCount < 2) {
        config.__retryCount = retryCount + 1
        await new Promise((r) => setTimeout(r, 1000 * config.__retryCount))
        return http(config)
      }
    }

    if (status === 401) {
      const authStore = useAuthStore()
      authStore.clearApiKey()
      // 动态 import router 规避循环依赖;已位于 /login 时不再重复跳转
      if (window.location.pathname !== '/login') {
        import('@/router').then((m) => m.default.push('/login'))
      }
      return Promise.reject(new ApiError(error.response?.data?.error || '未授权,请重新登录', 401))
    }
    const body = error.response?.data
    const msg = body?.error || body?.message || error.message || '网络错误,请稍后重试'
    return Promise.reject(new ApiError(msg, status || 0))
  },
)

export default http
