<template>
  <div class="page-container">
    <a-spin :spinning="loading" tip="加载中...">
      <template v-if="repo">
        <!-- Repo Header -->
        <div class="repo-header">
          <div class="repo-left">
            <div class="repo-icon-lg">
              <FolderOutlined style="font-size: 24px;" />
            </div>
            <div class="repo-info-lg">
              <div class="repo-name-lg">{{ repo.name }}</div>
              <div class="repo-url-lg">
                <a-tooltip :title="repo.clone_url">{{ repo.clone_url }}</a-tooltip>
              </div>
            </div>
            <a-badge :status="repo.status === 'active' ? 'success' : 'default'" :text="repo.status === 'active' ? '活跃' : '停用'" />
          </div>
          <div class="header-actions">
            <a-button @click="refresh">
              <template #icon><ReloadOutlined /></template>
              刷新
            </a-button>
            <a-button type="primary" @click="syncNow">
              <template #icon><SyncOutlined /></template>
              立即同步
            </a-button>
            <a-button @click="router.push(`/repos/config/${repoKey}`)">
              <template #icon><SettingOutlined /></template>
              配置
            </a-button>
          </div>
        </div>

        <!-- Tabs -->
        <a-tabs v-model:activeKey="activeTab" class="detail-tabs" @change="handleTabChange">
          <a-tab-pane key="tasks" tab="同步任务" />
          <a-tab-pane key="history" tab="同步历史" />
          <a-tab-pane key="webhook" tab="Webhook" />
        </a-tabs>

        <!-- Tasks Tab -->
        <div v-if="activeTab === 'tasks'">
          <div v-if="tasks.length === 0" class="empty-card">
            <a-empty description="暂无同步任务">
              <a-button type="primary" @click="router.push('/sync/new')">
                <template #icon><PlusOutlined /></template>
                创建同步任务
              </a-button>
            </a-empty>
          </div>
          <div v-else class="task-cards">
            <div class="task-card-item" v-for="task in tasks" :key="task.key">
              <div class="task-card-header">
                <span class="task-card-name">{{ task.name }}</span>
                <StatusBadge :status="task.last_status || 'idle'" />
              </div>
              <div class="task-card-body">
                <div class="task-card-meta">
                  <span class="branch-tag">{{ task.source_branch }}</span>
                  <ArrowRightOutlined style="color: #BFBFBF; font-size: 12px; margin: 0 6px;" />
                  <span class="branch-tag">{{ task.target_branch }}</span>
                </div>
                <div class="task-card-info">
                  <span v-if="task.cron" class="info-item">
                    <ClockCircleOutlined /> {{ task.cron }}
                  </span>
                  <span v-if="task.last_run_at" class="info-item">
                    <HistoryOutlined /> {{ task.last_run_at }}
                  </span>
                </div>
              </div>
              <div class="task-card-actions">
                <a-button size="small" type="primary" @click="runTask(task.key)">
                  <template #icon><PlayCircleOutlined /></template>
                  运行
                </a-button>
                <a-button size="small" @click="editTask(task.key)">
                  <template #icon><EditOutlined /></template>
                  编辑
                </a-button>
              </div>
            </div>
          </div>
        </div>

        <!-- History Tab -->
        <div v-if="activeTab === 'history'">
          <div v-if="history.length === 0" class="empty-card">
            <a-empty description="暂无同步历史" />
          </div>
          <a-timeline v-else class="history-timeline">
            <a-timeline-item
              v-for="run in history"
              :key="run.id"
              :color="run.status === 'success' ? 'green' : run.status === 'failed' ? 'red' : 'blue'"
            >
              <div class="timeline-item">
                <div class="timeline-header">
                  <span class="timeline-task">{{ getTaskName(run.task_key) }}</span>
                  <StatusBadge :status="run.status" />
                </div>
                <div class="timeline-meta">
                  <a-tag size="small" :color="triggerColor(run.trigger_source)">
                    {{ triggerLabel(run.trigger_source) }}
                  </a-tag>
                  <span class="timeline-time">{{ run.start_time }}</span>
                </div>
                <div v-if="run.error_message" class="timeline-error">
                  <ExclamationCircleOutlined /> {{ run.error_message }}
                </div>
              </div>
            </a-timeline-item>
          </a-timeline>
        </div>

        <!-- Webhook Tab -->
        <div v-if="activeTab === 'webhook'">
          <div class="webhook-card">
            <h4 style="margin-bottom: 12px;">Webhook 地址</h4>
            <a-input-group compact>
              <a-input
                :value="webhookUrl"
                readonly
                style="width: calc(100% - 80px);"
              />
              <a-button type="primary" @click="copyUrl" style="width: 80px;">
                <template #icon><CopyOutlined /></template>
                复制
              </a-button>
            </a-input-group>
            <div class="form-tip" style="margin-top: 8px;">
              <InfoCircleOutlined /> 将此地址配置到源仓库的 Webhook 设置中，即可实现推送自动同步
            </div>
          </div>
        </div>
      </template>

      <a-empty v-else-if="!loading" description="仓库不存在">
        <a-button @click="router.push('/repos')">返回仓库列表</a-button>
      </a-empty>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  FolderOutlined,
  ReloadOutlined,
  SyncOutlined,
  SettingOutlined,
  PlusOutlined,
  ArrowRightOutlined,
  ClockCircleOutlined,
  HistoryOutlined,
  PlayCircleOutlined,
  EditOutlined,
  CopyOutlined,
  InfoCircleOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons-vue'
import { useRepoStore } from '@/stores/repo'
import { useSyncTaskStore } from '@/stores/syncTask'
import { copyToClipboard } from '@/utils'
import { buildWebhookReceiveUrl } from '@/constants/webhook'
import { triggerColor, triggerLabel } from '@/utils/dictionaries'
import { notifySuccess, notifyError, notifyWarning } from '@/utils/notify'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { Repo, SyncTask, SyncRun } from '@/types'

const route = useRoute()
const router = useRouter()
const repoStore = useRepoStore()
const taskStore = useSyncTaskStore()

const loading = ref(true)
const repo = ref<Repo | null>(null)
const tasks = ref<SyncTask[]>([])
const history = ref<SyncRun[]>([])
const activeTab = ref('tasks')

const repoKey = computed(() => route.params.id as string)
const webhookUrl = computed(() => buildWebhookReceiveUrl(repoKey.value))

const getTaskName = (taskKey: string) => {
  const task = tasks.value.find(t => t.key === taskKey)
  return task?.name || taskKey.substring(0, 8) + '...'
}

onMounted(async () => {
  try {
    repo.value = await repoStore.getRepo(repoKey.value)
    if (repo.value) {
      await taskStore.fetchTasks({ repo_key: repoKey.value })
      tasks.value = taskStore.tasks
    }
    if (route.query.tab) {
      activeTab.value = route.query.tab as string
      // 如果直接打开 history tab，加载历史数据
      if (activeTab.value === 'history') {
        await loadHistory()
      }
    }
  } catch (e) {
    notifyError(e, '加载仓库详情失败')
  } finally {
    loading.value = false
  }
})

function handleTabChange(tab: string) {
  router.replace({ query: { tab } })
  if (tab === 'history' && history.value.length === 0) {
    loadHistory()
  }
}

async function loadHistory() {
  // 确保 tasks 已经加载
  if (tasks.value.length === 0) {
    await taskStore.fetchTasks({ repo_key: repoKey.value })
    tasks.value = taskStore.tasks
  }

  // 并行请求所有任务的执行记录,使用返回值而非共享 store ref
  const results = await Promise.allSettled(
    tasks.value.map((task) => taskStore.fetchHistory(task.key, 20)),
  )
  const allRuns: SyncRun[] = []
  for (const r of results) {
    if (r.status === 'fulfilled') {
      allRuns.push(...r.value)
    }
  }
  history.value = allRuns.sort((a, b) => new Date(b.start_time || 0).getTime() - new Date(a.start_time || 0).getTime())
}

async function refresh() {
  try {
    repo.value = await repoStore.getRepo(repoKey.value)
    await taskStore.fetchTasks({ repo_key: repoKey.value })
    tasks.value = taskStore.tasks
    if (activeTab.value === 'history') {
      await loadHistory()
    }
    notifySuccess('刷新成功')
  } catch (e) {
    notifyError(e, '刷新失败')
  }
}

async function syncNow() {
  if (tasks.value.length === 0) {
    notifyWarning('暂无同步任务')
    return
  }
  try {
    await taskStore.runTask(tasks.value[0].key)
    notifySuccess('任务已启动')
  } catch (e) {
    notifyError(e, '启动任务失败')
  }
}

async function runTask(key: string) {
  try {
    await taskStore.runTask(key)
    notifySuccess('任务已启动')
  } catch (e) {
    notifyError(e, '启动任务失败')
  }
}

function editTask(key: string) {
  router.push(`/sync?edit=${key}`)
}

function copyUrl() {
  copyToClipboard(webhookUrl.value)
  notifySuccess('已复制到剪贴板')
}
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

.page-container {
  background: $background-color;
  min-height: 100%;
}

// Repo Header
.repo-header {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 20px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: $spacing-md;
}

.repo-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.repo-icon-lg {
  width: 52px;
  height: 52px;
  border-radius: $border-radius-md;
  background: #E6F7FF;
  color: $primary-color;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.repo-info-lg {
  .repo-name-lg {
    font-size: 18px;
    font-weight: 600;
    color: $text-primary;
  }

  .repo-url-lg {
    font-size: 13px;
    color: $text-secondary;
    margin-top: 4px;
    max-width: 400px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.header-actions {
  display: flex;
  gap: 8px;
}

// Tabs
.detail-tabs {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 4px 16px 0;
  margin-bottom: $spacing-md;
}

// Tasks
.task-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: $spacing-md;
}

.task-card-item {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 18px 20px;
  border: 1px solid $border-color;
  transition: all 0.2s;

  &:hover {
    box-shadow: $shadow-card-hover;
    border-color: #E6F7FF;
  }
}

.task-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.task-card-name {
  font-size: 15px;
  font-weight: 600;
  color: $text-primary;
}

.task-card-body {
  margin-bottom: 14px;
}

.task-card-meta {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
}

.task-card-info {
  display: flex;
  gap: 16px;

  .info-item {
    font-size: 12px;
    color: $text-secondary;
    display: flex;
    align-items: center;
    gap: 4px;
  }
}

.task-card-actions {
  display: flex;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid $border-color;
}

// History
.empty-card {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 48px 24px;
}

.history-timeline {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 24px;
}

.timeline-item {
  .timeline-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;
  }

  .timeline-task {
    font-weight: 500;
    color: $text-primary;
  }

  .timeline-meta {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .timeline-time {
    font-size: 12px;
    color: $text-secondary;
  }

  .timeline-error {
    font-size: 12px;
    color: $error-color;
    margin-top: 6px;
    padding: 6px 10px;
    background: #FFF2F0;
    border-radius: $border-radius-sm;
  }
}

// Webhook
.webhook-card {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 24px;
  max-width: 600px;
}

.form-tip {
  font-size: 12px;
  color: $text-secondary;
  margin-top: 4px;

  .anticon {
    margin-right: 4px;
  }
}
</style>
