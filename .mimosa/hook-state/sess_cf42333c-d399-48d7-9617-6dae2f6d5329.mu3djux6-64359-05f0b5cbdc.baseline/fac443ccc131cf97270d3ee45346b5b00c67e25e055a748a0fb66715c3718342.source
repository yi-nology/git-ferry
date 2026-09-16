import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/Login.vue'),
    meta: { requiresAuth: false, title: '登录' },
  },
  {
    path: '/',
    component: AppLayout,
    redirect: '/dashboard',
    meta: { requiresAuth: true },
    children: [
      { path: 'dashboard', name: 'Dashboard', component: () => import('@/views/dashboard/Dashboard.vue'), meta: { title: '仪表盘' } },
      { path: 'sync', name: 'SyncTasks', component: () => import('@/views/sync/SyncTaskList.vue'), meta: { title: '同步任务' } },
      { path: 'sync/records', name: 'SyncRecords', component: () => import('@/views/sync/SyncRecords.vue'), meta: { title: '执行记录' } },
      { path: 'sync/new', name: 'SyncNew', component: () => import('@/views/sync/NewSyncTask.vue'), meta: { title: '新建同步任务' } },
      { path: 'webhook/rules', name: 'WebhookRules', component: () => import('@/views/webhook/WebhookRules.vue'), meta: { title: 'Webhook 规则' } },
      { path: 'logs/webhook-events', name: 'WebhookEvents', component: () => import('@/views/webhook/WebhookEvents.vue'), meta: { title: 'Webhook 事件' } },
      { path: 'repos', name: 'Repos', component: () => import('@/views/repos/RepoList.vue'), meta: { title: '仓库管理' } },
      { path: 'repos/config/:id', name: 'RepoConfig', component: () => import('@/views/repos/RepoUnifiedConfig.vue'), meta: { title: '仓库配置' } },
      { path: 'local-repos/:id', name: 'LocalRepoDetail', component: () => import('@/views/repos/LocalRepoDetail.vue'), meta: { title: '本地仓库' } },
      { path: 'logs/operations', name: 'OperationLogs', component: () => import('@/views/logs/OperationLogs.vue'), meta: { title: '操作日志' } },
      { path: 'settings/platforms', name: 'PlatformSettings', component: () => import('@/views/settings/PlatformSettings.vue'), meta: { title: '平台管理' } },
      { path: ':pathMatch(.*)*', name: 'NotFound', component: () => import('@/views/NotFound.vue'), meta: { title: '页面不存在' } },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: (_to, _from, savedPosition) => {
    if (savedPosition) return savedPosition
    return { top: 0 }
  },
})

// 鉴权守卫
router.beforeEach((to) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some((r) => r.meta.requiresAuth)

  if (requiresAuth && !authStore.isAuthenticated) {
    return '/login'
  }
  if (to.path === '/login' && authStore.isAuthenticated) {
    return '/dashboard'
  }
  return true
})

// 页面标题联动
router.afterEach((to) => {
  const title = to.meta.title as string | undefined
  document.title = title ? `${title} · Git Sync` : 'Git Sync'
})

export default router
