<template>
  <div class="page-container">
    <!-- Page Header -->
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">同步任务</h1>
        <p class="page-subtitle">管理代码仓库间的同步任务</p>
      </div>
      <a-button type="primary" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        创建任务
      </a-button>
    </div>

    <!-- Stats Cards -->
    <div class="stats-row">
      <div class="stat-card">
        <div class="stat-icon blue"><UnorderedListOutlined /></div>
        <div class="stat-content">
          <div class="stat-num">{{ taskStore.total }}</div>
          <div class="stat-name">总任务数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon green"><CheckCircleOutlined /></div>
        <div class="stat-content">
          <div class="stat-num">{{ successCount }}</div>
          <div class="stat-name">成功</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon orange"><SyncOutlined /></div>
        <div class="stat-content">
          <div class="stat-num">{{ runningCount }}</div>
          <div class="stat-name">运行中</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon red"><CloseCircleOutlined /></div>
        <div class="stat-content">
          <div class="stat-num">{{ failedCount }}</div>
          <div class="stat-name">失败</div>
        </div>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar">
      <a-input
        v-model:value="filters.search"
        placeholder="搜索任务名称..."
        allow-clear
        class="filter-input"
        @pressEnter="handleSearch"
      >
        <template #prefix><SearchOutlined /></template>
      </a-input>
      <a-select
        v-model:value="filters.status"
        placeholder="全部状态"
        allow-clear
        class="filter-select"
        @change="handleSearch"
      >
        <a-select-option value="success">成功</a-select-option>
        <a-select-option value="running">运行中</a-select-option>
        <a-select-option value="failed">失败</a-select-option>
      </a-select>
      <a-select
        v-model:value="filters.repo_key"
        placeholder="全部仓库"
        allow-clear
        show-search
        :filter-option="filterRepoOption"
        class="filter-select"
        @change="handleSearch"
      >
        <a-select-option v-for="repo in repoStore.repos" :key="repo.key" :value="repo.key">
          {{ repo.name }}
        </a-select-option>
      </a-select>
      <a-button @click="handleRefresh" :loading="taskStore.loading">
        <template #icon><ReloadOutlined /></template>
        刷新
      </a-button>
    </div>

    <!-- Batch Actions Bar -->
    <div v-if="selectedRowKeys.length > 0" class="batch-bar">
      <div class="batch-info">
        已选择 <strong>{{ selectedRowKeys.length }}</strong> 项
        <a-button type="link" size="small" @click="clearSelection">取消选择</a-button>
      </div>
      <a-popconfirm
        title="确定要删除选中的任务吗？此操作不可恢复。"
        ok-text="确定"
        cancel-text="取消"
        @confirm="handleBatchDelete"
      >
        <a-button danger size="small">
          <template #icon><DeleteOutlined /></template>
          批量删除
        </a-button>
      </a-popconfirm>
    </div>

    <!-- Task Table -->
    <a-table
      :columns="columns"
      :data-source="taskStore.tasks"
      :loading="taskStore.loading"
      :pagination="paginationConfig"
      :row-selection="{ selectedRowKeys, onChange: onSelectChange }"
      row-key="key"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <span class="task-name">{{ record.name }}</span>
        </template>
        <template v-if="column.key === 'repos'">
          <div class="repo-pair">
            <span class="repo-key">{{ getRepoName(record.source_repo_key) }}</span>
            <ArrowRightOutlined style="color: #BFBFBF; font-size: 12px; margin: 0 4px;" />
            <span class="repo-key">{{ getRepoName(record.target_repo_key) }}</span>
          </div>
        </template>
        <template v-if="column.key === 'branches'">
          <a-space :size="4">
            <span class="branch-tag">{{ record.source_branch }}</span>
            <ArrowRightOutlined style="color: #BFBFBF; font-size: 12px;" />
            <span class="branch-tag">{{ record.target_branch }}</span>
          </a-space>
        </template>
        <template v-if="column.key === 'mode'">
          <a-tag :color="record.sync_mode === 'all' ? 'blue' : 'default'">
            {{ record.sync_mode === 'all' ? '全分支' : '单分支' }}
          </a-tag>
        </template>
        <template v-if="column.key === 'status'">
          <StatusBadge :status="record.last_status" />
        </template>
        <template v-if="column.key === 'last_run'">
          <span class="time-text">{{ record.last_run_at || '未运行' }}</span>
        </template>
        <template v-if="column.key === 'actions'">
          <a-space :size="4">
            <a-tooltip title="立即运行">
              <a-button type="link" size="small" @click="handleRun(record.key)">
                <template #icon><PlayCircleOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑任务">
              <a-button type="link" size="small" @click="openEdit(record)">
                <template #icon><EditOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-popconfirm
              title="确定要删除该任务吗？"
              ok-text="确定"
              cancel-text="取消"
              @confirm="handleDelete(record.key)"
            >
              <a-tooltip title="删除任务">
                <a-button type="link" danger size="small">
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </a-tooltip>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
      <template #emptyText>
        <a-empty description="暂无同步任务">
          <a-button type="primary" @click="openCreate">
            <template #icon><PlusOutlined /></template>
            创建第一个任务
          </a-button>
        </a-empty>
      </template>
    </a-table>

    <!-- Create/Edit Modal -->
    <a-modal
      v-model:open="dialogVisible"
      :title="dialogTitle"
      :width="640"
      @ok="handleSubmit"
      ok-text="确定"
      cancel-text="取消"
    >
      <a-form :model="formData" layout="vertical">
        <a-form-item label="任务名称" required>
          <a-input v-model:value="formData.name" placeholder="请输入任务名称，例如: prod-sync" />
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="源仓库" required>
              <a-select
                v-model:value="formData.source_repo_key"
                placeholder="选择源仓库"
                show-search
                :filter-option="filterRepoOption"
                style="width: 100%"
              >
                <a-select-option v-for="repo in repoStore.repos" :key="repo.key" :value="repo.key">
                  <span>{{ repo.name }}</span>
                  <span style="color: #8C8C8C; font-size: 12px; margin-left: 8px;">{{ repo.key }}</span>
                </a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="目标仓库" required>
              <a-select
                v-model:value="formData.target_repo_key"
                placeholder="选择目标仓库"
                show-search
                :filter-option="filterRepoOption"
                style="width: 100%"
              >
                <a-select-option v-for="repo in repoStore.repos" :key="repo.key" :value="repo.key">
                  <span>{{ repo.name }}</span>
                  <span style="color: #8C8C8C; font-size: 12px; margin-left: 8px;">{{ repo.key }}</span>
                </a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="同步模式">
          <a-radio-group v-model:value="formData.sync_mode">
            <a-radio-button value="single">单分支同步</a-radio-button>
            <a-radio-button value="all">全分支同步</a-radio-button>
          </a-radio-group>
        </a-form-item>

        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="源分支">
              <a-input v-model:value="formData.source_branch" placeholder="main" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="目标分支">
              <a-input v-model:value="formData.target_branch" placeholder="main" />
            </a-form-item>
          </a-col>
        </a-row>

        <a-form-item label="定时表达式 (Cron)">
          <a-input v-model:value="formData.cron" placeholder="可选，如 */5 * * * *" />
          <div class="cron-presets">
            <span class="preset-label">常用:</span>
            <a-tag
              v-for="preset in cronPresets"
              :key="preset.value"
              class="cron-preset-tag"
              @click="formData.cron = preset.value"
            >
              {{ preset.label }}
            </a-tag>
          </div>
        </a-form-item>

        <a-form-item label="同步选项">
          <a-space direction="vertical" :size="8" style="width: 100%">
            <a-checkbox v-model:checked="formData.git_tags">
              <span style="font-weight: 500;">同步 Tags</span>
              <span style="color: #8C8C8C; font-size: 12px; margin-left: 8px;">同时推送 git tags 到目标仓库</span>
            </a-checkbox>
            <a-checkbox v-model:checked="formData.git_force">
              <span style="font-weight: 500;">强制推送</span>
              <span style="color: #8C8C8C; font-size: 12px; margin-left: 8px;">使用 --force 推送（谨慎使用）</span>
            </a-checkbox>
            <a-checkbox v-model:checked="formData.git_prune">
              <span style="font-weight: 500;">Prune</span>
              <span style="color: #8C8C8C; font-size: 12px; margin-left: 8px;">清理远程已删除的分支</span>
            </a-checkbox>
          </a-space>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: 'SyncTaskList' })

import { onMounted, onActivated, ref, reactive, computed } from 'vue'
import {
  PlusOutlined,
  PlayCircleOutlined,
  EditOutlined,
  DeleteOutlined,
  ArrowRightOutlined,
  UnorderedListOutlined,
  CheckCircleOutlined,
  SyncOutlined,
  CloseCircleOutlined,
  SearchOutlined,
  ReloadOutlined,
} from '@ant-design/icons-vue'
import { useSyncTaskStore } from '@/stores/syncTask'
import { useRepoStore } from '@/stores/repo'
import type { SyncTask } from '@/types'
import type { TablePaginationConfig } from 'ant-design-vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import { notifySuccess, notifyError, notifyWarning } from '@/utils/notify'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const taskStore = useSyncTaskStore()
const repoStore = useRepoStore()
const dialogVisible = ref(false)
const dialogTitle = ref('')
const editingKey = ref('')

// -- Filters --
const filters = reactive({
  search: '',
  status: undefined as string | undefined,
  repo_key: undefined as string | undefined,
})

// -- Pagination state --
const currentPage = ref(1)
const pageSize = ref(10)

// -- Row selection --
const selectedRowKeys = ref<string[]>([])

function onSelectChange(keys: string[]) {
  selectedRowKeys.value = keys
}

function clearSelection() {
  selectedRowKeys.value = []
}

// -- Computed stats (单次遍历) --
const taskStats = computed(() => {
  let success = 0, running = 0, failed = 0
  for (const t of taskStore.tasks) {
    if (t.last_status === 'success') success++
    else if (t.last_status === 'running') running++
    else if (t.last_status === 'failed') failed++
  }
  return { success, running, failed }
})
const successCount = computed(() => taskStats.value.success)
const runningCount = computed(() => taskStats.value.running)
const failedCount = computed(() => taskStats.value.failed)

// -- Columns --
const columns = [
  { title: '任务名称', key: 'name', dataIndex: 'name', width: 180, ellipsis: true },
  { title: '仓库', key: 'repos', width: 260 },
  { title: '分支', key: 'branches', width: 180 },
  { title: '模式', key: 'mode', width: 90, align: 'center' as const },
  { title: '状态', key: 'status', width: 90, align: 'center' as const },
  { title: '最后运行', key: 'last_run', width: 140 },
  { title: '操作', key: 'actions', width: 120, fixed: 'right' as const },
]

// -- Pagination config --
const paginationConfig = computed(() => ({
  current: currentPage.value,
  pageSize: pageSize.value,
  total: taskStore.total,
  showSizeChanger: true,
  showTotal: (total: number) => `共 ${total} 条`,
}))

// -- Filter / search helpers --
function filterRepoOption(input: string, option: any) {
  const repo = repoStore.repos.find(r => r.key === option.value)
  if (!repo) return false
  const search = input.toLowerCase()
  return repo.name.toLowerCase().includes(search) || repo.key.toLowerCase().includes(search)
}

// 预计算 repoKey → repoName 的 Map,避免模板循环中 O(n×m) 逐次 find
const repoNameMap = computed(() => {
  const map = new Map<string, string>()
  for (const r of repoStore.repos) {
    map.set(r.key, r.name)
  }
  return map
})
function getRepoName(key: string): string {
  return repoNameMap.value.get(key) || key.substring(0, 8) + '...'
}

function buildParams() {
  return {
    page: currentPage.value,
    page_size: pageSize.value,
    ...(filters.search ? { search: filters.search } : {}),
    ...(filters.status ? { status: filters.status } : {}),
    ...(filters.repo_key ? { repo_key: filters.repo_key } : {}),
  }
}

async function loadTasks() {
  try {
    await taskStore.fetchTasks(buildParams())
  } catch (e) {
    notifyError(e, '加载任务列表失败')
  }
}

function handleSearch() {
  currentPage.value = 1
  loadTasks()
  clearSelection()
}

function handleRefresh() {
  loadTasks()
  clearSelection()
}

function handleTableChange(pagination: TablePaginationConfig) {
  currentPage.value = pagination.current || 1
  pageSize.value = pagination.pageSize || 10
  loadTasks()
  clearSelection()
}

// -- Form data --
const formData = reactive({
  name: '',
  source_repo_key: '',
  source_branch: 'main',
  target_repo_key: '',
  target_branch: 'main',
  sync_mode: 'single',
  cron: '',
  git_tags: false,
  git_force: false,
  git_prune: false,
})

const cronPresets = [
  { label: '每5分钟', value: '*/5 * * * *' },
  { label: '每小时', value: '0 * * * *' },
  { label: '每天凌晨', value: '0 0 * * *' },
  { label: '每周一', value: '0 0 * * 1' },
  { label: '每30分钟', value: '*/30 * * * *' },
]

// -- Lifecycle --
onMounted(async () => {
  await loadTasks()
  repoStore.fetchRepos().catch((e) => notifyError(e, '加载仓库失败'))
  // 支持 /sync?edit=<taskKey> 深链(仓库详情页"编辑"按钮跳转目标)
  const editKey = route.query.edit as string | undefined
  if (editKey) {
    const task = taskStore.tasks.find((t) => t.key === editKey)
    if (task) openEdit(task)
    else notifyWarning(`未找到任务 ${editKey}`)
    router.replace({ query: {} }) // 清掉参数,避免刷新重复弹窗
  }
})

// keep-alive 缓存页:每次切回刷新任务列表
onActivated(() => {
  loadTasks()
})

// -- CRUD actions --
function openCreate() {
  dialogTitle.value = '创建任务'
  editingKey.value = ''
  Object.assign(formData, {
    name: '', source_repo_key: '', source_branch: 'main',
    target_repo_key: '', target_branch: 'main', sync_mode: 'single',
    cron: '', git_tags: false, git_force: false, git_prune: false,
  })
  dialogVisible.value = true
}

function openEdit(task: SyncTask) {
  dialogTitle.value = '编辑任务'
  editingKey.value = task.key
  Object.assign(formData, {
    name: task.name,
    source_repo_key: task.source_repo_key,
    source_branch: task.source_branch,
    target_repo_key: task.target_repo_key,
    target_branch: task.target_branch,
    sync_mode: task.sync_mode,
    cron: task.cron,
    git_tags: task.git_tags,
    git_force: task.git_force,
    git_prune: task.git_prune,
  })
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!formData.name || !formData.source_repo_key || !formData.target_repo_key) {
    notifyWarning('请填写必填字段')
    return
  }
  try {
    if (editingKey.value) {
      await taskStore.updateTask({ key: editingKey.value, ...formData })
      notifySuccess('更新任务成功')
    } else {
      await taskStore.createTask(formData)
      notifySuccess('创建任务成功')
    }
    dialogVisible.value = false
    clearSelection()
  } catch (e) {
    notifyError(e, '保存任务失败')
  }
}

async function handleDelete(key: string) {
  try {
    await taskStore.deleteTask(key)
    notifySuccess('删除任务成功')
  } catch (e) {
    notifyError(e, '删除任务失败')
  }
  clearSelection()
}

async function handleRun(key: string) {
  try {
    await taskStore.runTask(key)
    notifySuccess('任务已启动')
  } catch (e) {
    notifyError(e, '启动任务失败')
  }
}

async function handleBatchDelete() {
  try {
    const { succeeded, failed } = await taskStore.batchDelete(selectedRowKeys.value)
    if (succeeded > 0) notifySuccess(`成功删除 ${succeeded} 个任务`)
    if (failed > 0) notifyError(`有 ${failed} 个任务删除失败`)
  } catch (e) {
    notifyError(e, '批量删除失败')
  }
  selectedRowKeys.value = []
}
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

// 公共样式(page-container/header/stats/filter-bar/batch-bar 等)统一使用 global.scss

.batch-info {
  font-size: 14px;
  color: $text-primary;

  strong {
    color: $primary;
  }
}

// -- Table cell styles --
.task-name {
  font-weight: 500;
  color: $text-primary;
}

.repo-pair {
  display: flex;
  align-items: center;
}

.repo-key {
  display: inline-block;
  padding: 1px 6px;
  background: #F5F5F5;
  border-radius: $radius-sm;
  font-size: 12px;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  color: $text-primary;
}

.branch-tag {
  font-size: 12px;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
}

.time-text {
  color: $text-secondary;
  font-size: 13px;
}

// -- Cron presets --
.cron-presets {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.preset-label {
  font-size: 12px;
  color: $text-secondary;
}

.cron-preset-tag {
  cursor: pointer;
  font-size: 12px;
  transition: all 0.2s;

  &:hover {
    color: $primary;
    border-color: $primary;
  }
}

// -- Responsive --
@media (max-width: 1200px) {
  .stats-row {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 768px) {
  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-input,
  .filter-select {
    width: 100%;
  }
}
</style>
