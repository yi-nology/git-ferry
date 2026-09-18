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
            <a-button @click="testConn">
              <template #icon><ApiOutlined /></template>
              测试连接
            </a-button>
            <a-button type="primary" @click="runSync">
              <template #icon><SyncOutlined /></template>
              立即同步
            </a-button>
            <a-button @click="router.push('/repos')">
              <template #icon><ArrowLeftOutlined /></template>
              返回
            </a-button>
          </div>
        </div>

        <!-- Config Sections -->
        <div class="config-sections">
          <!-- Repo Info -->
          <div class="config-card">
            <div class="card-title">
              <InfoCircleOutlined style="margin-right: 8px; color: #1677FF;" />
              仓库信息
            </div>
            <a-descriptions :column="2" bordered size="small">
              <a-descriptions-item label="平台">
                <a-tag :color="platformColor(repo.platform)">{{ platformLabel(repo.platform) }}</a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="默认分支">
                <span class="branch-tag">{{ repo.default_branch }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="所有者">{{ repo.platform_owner }}</a-descriptions-item>
              <a-descriptions-item label="仓库名">{{ repo.platform_repo }}</a-descriptions-item>
              <a-descriptions-item label="HTTPS URL">
                <span style="font-family: monospace; font-size: 12px;">{{ repo.clone_url }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="SSH URL">
                <span style="font-family: monospace; font-size: 12px;">{{ repo.ssh_url || '-' }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="创建时间" :span="2">{{ repo.created_at }}</a-descriptions-item>
            </a-descriptions>
          </div>

          <!-- Associated Tasks -->
          <div class="config-card">
            <div class="card-title">
              <SyncOutlined style="margin-right: 8px; color: #52C41A;" />
              关联同步任务
            </div>
            <div v-if="tasks.length === 0">
              <a-empty description="暂无关联任务">
                <a-button type="primary" size="small" @click="router.push('/sync/new')">创建任务</a-button>
              </a-empty>
            </div>
            <div v-else class="task-list">
              <div class="task-item" v-for="task in tasks" :key="task.key">
                <div class="task-info">
                  <div class="task-name">{{ task.name }}</div>
                  <div class="task-meta">
                    <span class="branch-tag">{{ task.source_branch }}</span>
                    <ArrowRightOutlined style="color: #BFBFBF; font-size: 12px; margin: 0 4px;" />
                    <span class="branch-tag">{{ task.target_branch }}</span>
                  </div>
                </div>
                <StatusBadge :status="task.last_status || 'idle'" />
              </div>
            </div>
          </div>

          <!-- Webhook Config -->
          <div class="config-card">
            <div class="card-title">
              <ApiOutlined style="margin-right: 8px; color: #722ED1;" />
              Webhook 配置
            </div>
            <a-form layout="vertical">
              <a-form-item label="Webhook 地址">
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
                <div class="form-tip">
                  <InfoCircleOutlined /> 将此地址配置到源仓库的 Webhook 设置中
                </div>
              </a-form-item>
            </a-form>
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
  ApiOutlined,
  SyncOutlined,
  ArrowLeftOutlined,
  ArrowRightOutlined,
  InfoCircleOutlined,
  CopyOutlined,
} from '@ant-design/icons-vue'
import { useRepoStore } from '@/stores/repo'
import { useSyncTaskStore } from '@/stores/syncTask'
import { copyToClipboard } from '@/utils'
import { buildWebhookReceiveUrl } from '@/constants/webhook'
import { platformColor, platformLabel } from '@/utils/platform'
import { notifySuccess, notifyError, notifyWarning } from '@/utils/notify'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { Repo, SyncTask } from '@/types'

const route = useRoute()
const router = useRouter()
const repoStore = useRepoStore()
const taskStore = useSyncTaskStore()

const loading = ref(true)
const repo = ref<Repo | null>(null)
const tasks = ref<SyncTask[]>([])

const repoKey = computed(() => route.params.id as string)
const webhookUrl = computed(() => buildWebhookReceiveUrl(repoKey.value))

onMounted(async () => {
  try {
    repo.value = await repoStore.getRepo(repoKey.value)
    if (repo.value) {
      await taskStore.fetchTasks({ repo_key: repoKey.value })
      tasks.value = taskStore.tasks
    }
  } catch (e) {
    notifyError(e, '加载仓库详情失败')
  } finally {
    loading.value = false
  }
})

async function testConn() {
  try {
    const result = await repoStore.testConnection(repoKey.value)
    if (result.success) notifySuccess(result.message)
    else notifyWarning(result.message)
  } catch (e) {
    notifyError(e, '测试连接失败')
  }
}

async function runSync() {
  if (tasks.value.length === 0) {
    notifyWarning('暂无关联任务')
    return
  }
  try {
    await taskStore.runTask(tasks.value[0].key)
    notifySuccess('任务已启动')
  } catch (e) {
    notifyError(e, '启动任务失败')
  }
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

// Config Sections
.config-sections {
  display: flex;
  flex-direction: column;
  gap: $spacing-md;
}

.config-card {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  padding: 20px 24px;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: $text-primary;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid $border-color;
  display: flex;
  align-items: center;
}

// Task List
.task-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #FAFAFA;
  border-radius: $border-radius-md;
  border: 1px solid $border-color;
  transition: all 0.2s;

  &:hover {
    border-color: #E6F7FF;
    background: #F9FCFF;
  }
}

.task-info {
  .task-name {
    font-size: 14px;
    font-weight: 500;
    color: $text-primary;
  }

  .task-meta {
    font-size: 12px;
    color: $text-secondary;
    margin-top: 4px;
    display: flex;
    align-items: center;
  }
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
