<template>
  <div class="page-container">
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">新建同步任务</h1>
        <p class="page-subtitle">配置源仓库和目标仓库之间的同步规则</p>
      </div>
      <a-button @click="router.push('/sync')">返回列表</a-button>
    </div>

    <!-- Steps -->
    <a-steps :current="step" class="steps-bar" size="small">
      <a-step title="基本配置" description="选择仓库和模式" />
      <a-step title="同步规则" description="分支和定时设置" />
      <a-step title="高级选项" description="Git 同步参数" />
    </a-steps>

    <!-- Form Card -->
    <div class="form-card">
      <div class="form-body">
        <!-- Step 1: Basic Config -->
        <div v-show="step === 1">
          <a-form layout="vertical">
            <a-form-item label="任务名称" required>
              <a-input
                v-model:value="form.name"
                placeholder="请输入任务名称，例如: prod-sync"
                :maxlength="64"
                show-count
              />
            </a-form-item>

            <a-form-item label="同步模式" required>
              <a-radio-group v-model:value="form.sync_mode">
                <a-radio-button value="single">单分支同步</a-radio-button>
                <a-radio-button value="all">全分支同步</a-radio-button>
              </a-radio-group>
              <div class="form-tip">
                <InfoCircleOutlined />
                单分支同步：仅同步指定分支；全分支同步：同步所有分支
              </div>
            </a-form-item>

            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="源仓库" required>
                  <a-select
                    v-model:value="form.source_repo_key"
                    placeholder="输入名称搜索,滚动加载更多"
                    show-search
                    :filter-option="false"
                    :loading="repoLoading"
                    style="width: 100%"
                    size="large"
                    @search="handleRepoSearch"
                    @popupScroll="handleRepoPopupScroll"
                  >
                    <a-select-option v-for="repo in sourceRepoOptions" :key="repo.key" :value="repo.key">
                      <div style="display: flex; justify-content: space-between; align-items: center;">
                        <span>{{ repo.name }}</span>
                        <span style="color: #8C8C8C; font-size: 12px;">{{ repo.platform }}</span>
                      </div>
                    </a-select-option>
                    <a-select-option v-if="repoLoading" key="__loading" disabled>
                      <span style="color: #8C8C8C; font-size: 12px;">加载中...</span>
                    </a-select-option>
                    <a-select-option v-else-if="!sourceRepoOptions.length" key="__empty" disabled>
                      <span style="color: #8C8C8C; font-size: 12px;">无匹配仓库</span>
                    </a-select-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="目标仓库" required>
                  <a-select
                    v-model:value="form.target_repo_key"
                    placeholder="输入名称搜索,滚动加载更多"
                    show-search
                    :filter-option="false"
                    :loading="repoLoading"
                    style="width: 100%"
                    size="large"
                    @search="handleRepoSearch"
                    @popupScroll="handleRepoPopupScroll"
                  >
                    <a-select-option v-for="repo in targetRepoOptions" :key="repo.key" :value="repo.key">
                      <div style="display: flex; justify-content: space-between; align-items: center;">
                        <span>{{ repo.name }}</span>
                        <span style="color: #8C8C8C; font-size: 12px;">{{ repo.platform }}</span>
                      </div>
                    </a-select-option>
                    <a-select-option v-if="repoLoading" key="__loading" disabled>
                      <span style="color: #8C8C8C; font-size: 12px;">加载中...</span>
                    </a-select-option>
                    <a-select-option v-else-if="!targetRepoOptions.length" key="__empty" disabled>
                      <span style="color: #8C8C8C; font-size: 12px;">无匹配仓库</span>
                    </a-select-option>
                  </a-select>
                </a-form-item>
              </a-col>
            </a-row>

            <!-- Preview -->
            <a-alert
              v-if="form.source_repo_key && form.target_repo_key"
              type="info"
              show-icon
              style="margin-top: 8px;"
            >
              <template #message>
                <span>
                  将从 <strong>{{ getRepoName(form.source_repo_key) }}</strong>
                  同步到 <strong>{{ getRepoName(form.target_repo_key) }}</strong>
                  （{{ form.sync_mode === 'all' ? '全分支' : '单分支' }}模式）
                </span>
              </template>
            </a-alert>
          </a-form>
        </div>

        <!-- Step 2: Sync Rules -->
        <div v-show="step === 2">
          <a-form layout="vertical">
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="源分支">
                  <a-input v-model:value="form.source_branch" placeholder="main" />
                  <div class="form-tip">
                    <InfoCircleOutlined /> 仅在单分支模式下生效
                  </div>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="目标分支">
                  <a-input v-model:value="form.target_branch" placeholder="main" />
                  <div class="form-tip">
                    <InfoCircleOutlined /> 仅在单分支模式下生效
                  </div>
                </a-form-item>
              </a-col>
            </a-row>

            <a-form-item label="定时同步 (Cron 表达式)">
              <a-input v-model:value="form.cron" placeholder="可选，如 0 */5 * * * *" />
              <div class="cron-presets">
                <span class="preset-label">常用表达式:</span>
                <a-tag
                  v-for="preset in cronPresets"
                  :key="preset.value"
                  class="cron-preset-tag"
                  @click="form.cron = preset.value"
                >
                  {{ preset.label }}
                </a-tag>
              </div>
              <div class="form-tip">
                <InfoCircleOutlined /> 留空表示仅通过 Webhook 或手动触发同步
              </div>
            </a-form-item>
          </a-form>
        </div>

        <!-- Step 3: Advanced Options -->
        <div v-show="step === 3">
          <a-form layout="vertical">
            <a-form-item label="Git 同步选项">
              <a-space direction="vertical" :size="12" style="width: 100%">
                <div class="option-item">
                  <a-checkbox v-model:checked="form.git_tags">同步 Tags</a-checkbox>
                  <div class="option-desc">同时推送 git tags 到目标仓库</div>
                </div>
                <div class="option-item">
                  <a-checkbox v-model:checked="form.git_force">强制推送</a-checkbox>
                  <div class="option-desc">使用 --force 推送，会覆盖目标分支的历史（谨慎使用）</div>
                </div>
                <div class="option-item">
                  <a-checkbox v-model:checked="form.git_prune">Prune 本地跟踪</a-checkbox>
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model:checked="form.git_lfs">同步 LFS 对象</a-checkbox>
                </a-form-item>
                <a-form-item>
                  <a-checkbox v-model:checked="form.git_push_prune">删除目标多余分支</a-checkbox>
                </a-form-item>
                <a-form-item>
                  <a-tooltip title="关闭后允许 force 覆盖目标独有提交(危险)">
                    <a-checkbox v-model:checked="form.keep_divergent">分歧保护</a-checkbox>
                  </a-tooltip>
                  <div class="option-desc">清理目标仓库中已被源仓库删除的远程分支</div>
                </div>
              </a-space>
            </a-form-item>

            <!-- Summary -->
            <a-divider />
            <div class="summary-section">
              <h4 style="margin-bottom: 12px;">任务摘要</h4>
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item label="任务名称">{{ form.name || '-' }}</a-descriptions-item>
                <a-descriptions-item label="源仓库">{{ getRepoName(form.source_repo_key) || '-' }}</a-descriptions-item>
                <a-descriptions-item label="目标仓库">{{ getRepoName(form.target_repo_key) || '-' }}</a-descriptions-item>
                <a-descriptions-item label="同步模式">{{ form.sync_mode === 'all' ? '全分支同步' : '单分支同步' }}</a-descriptions-item>
                <a-descriptions-item label="分支">{{ form.source_branch }} → {{ form.target_branch }}</a-descriptions-item>
                <a-descriptions-item label="定时">{{ form.cron || '手动/Webhook 触发' }}</a-descriptions-item>
              </a-descriptions>
            </div>
          </a-form>
        </div>
      </div>

      <!-- Footer -->
      <div class="form-footer">
        <a-button v-if="step > 1" @click="step--">
          <template #icon><LeftOutlined /></template>
          上一步
        </a-button>
        <div style="flex: 1;" />
        <a-button @click="router.push('/sync')">取消</a-button>
        <a-button v-if="step < 3" type="primary" @click="nextStep">
          下一步
          <template #icon><RightOutlined /></template>
        </a-button>
        <a-button v-if="step === 3" type="primary" @click="submit" :loading="submitting">
          <template #icon><CheckOutlined /></template>
          创建任务
        </a-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  InfoCircleOutlined,
  LeftOutlined,
  RightOutlined,
  CheckOutlined,
} from '@ant-design/icons-vue'
import { useSyncTaskStore } from '@/stores/syncTask'
import { repoApi } from '@/api'
import { notifySuccess, notifyError, notifyWarning } from '@/utils/notify'
import type { Repo } from '@/types'

const router = useRouter()
const taskStore = useSyncTaskStore()
const step = ref(1)
const submitting = ref(false)

const form = reactive({
  name: '',
  source_repo_key: '',
  target_repo_key: '',
  sync_mode: 'single',
  source_branch: 'main',
  target_branch: 'main',
  cron: '',
  git_tags: false,
  git_force: false,
  git_prune: false,
  git_lfs: false,
  git_push_prune: false,
  keep_divergent: true,
})

const cronPresets = [
  { label: '每5分钟', value: '*/5 * * * *' },
  { label: '每小时', value: '0 * * * *' },
  { label: '每天凌晨', value: '0 0 * * *' },
  { label: '每周一', value: '0 0 * * 1' },
]

// ---- 仓库下拉:远程搜索 + 滚动翻页加载 ----
// 仓库可能很多,不再一次性拉全量(原 page_size=500 仍有截断风险):
// 输入搜索走后端 search,滚动触底追加下一页。
const REPO_PAGE_SIZE = 50
const repoOptions = ref<Repo[]>([])
const repoTotal = ref(0)
const repoPage = ref(1)
const repoSearch = ref('')
const repoLoading = ref(false)
const repoLoadedKeys = new Set<string>()
// 已选中的仓库对象缓存:搜索/翻页替换 options 后保证回显有据可查
const selectedRepoCache = new Map<string, Repo>()
let repoSearchTimer: ReturnType<typeof setTimeout> | null = null

const repoHasMore = computed(() => repoOptions.value.length < repoTotal.value)

// 已选项可能不在当前页 options 里,合并进各自下拉保证显示名称而非裸 key;
// 源下拉只补源仓库、目标下拉只补目标仓库,避免把源仓库混入目标列表造成误选
function mergeSelected(options: Repo[], selectedKey: string): Repo[] {
  if (!selectedKey || options.some(r => r.key === selectedKey)) return options
  const r = selectedRepoCache.get(selectedKey)
  return r ? [r, ...options] : options
}

const sourceRepoOptions = computed(() => mergeSelected(repoOptions.value, form.source_repo_key))
const targetRepoOptions = computed(() => mergeSelected(repoOptions.value, form.target_repo_key))

// 请求序号:慢响应不得覆盖新搜索结果
let repoRequestSeq = 0

async function loadRepoPage(page: number, search: string) {
  const seq = ++repoRequestSeq
  repoLoading.value = true
  try {
    const data = await repoApi.list({ page, page_size: REPO_PAGE_SIZE, search: search || undefined })
    if (seq !== repoRequestSeq) return // 过期响应,丢弃
    repoTotal.value = data.pagination?.total ?? data.list.length
    if (page === 1) {
      repoLoadedKeys.clear()
      repoOptions.value = data.list
    } else {
      const merged = [...repoOptions.value]
      for (const r of data.list) {
        if (!repoLoadedKeys.has(r.key)) merged.push(r)
      }
      repoOptions.value = merged
    }
    for (const r of data.list) repoLoadedKeys.add(r.key)
    repoPage.value = page
    repoSearch.value = search
  } catch (e) {
    if (seq !== repoRequestSeq) return
    notifyError(e, '加载仓库失败')
  } finally {
    if (seq === repoRequestSeq) repoLoading.value = false
  }
}

function handleRepoSearch(val: string) {
  if (repoSearchTimer) clearTimeout(repoSearchTimer)
  repoSearchTimer = setTimeout(() => {
    loadRepoPage(1, val.trim())
  }, 300)
}

async function handleRepoPopupScroll(e: Event) {
  const target = e.target as HTMLElement
  const nearBottom = target.scrollHeight - target.scrollTop - target.clientHeight < 60
  if (nearBottom && repoHasMore.value && !repoLoading.value) {
    await loadRepoPage(repoPage.value + 1, repoSearch.value)
  }
}

function cacheSelectedRepos() {
  for (const r of repoOptions.value) {
    if (r.key === form.source_repo_key || r.key === form.target_repo_key) {
      selectedRepoCache.set(r.key, r)
    }
  }
}

// 选中变化时缓存选中对象(在 options 被搜索替换前抓取,保证回显)
watch(() => [form.source_repo_key, form.target_repo_key], cacheSelectedRepos)

function getRepoName(key: string) {
  return (
    selectedRepoCache.get(key)?.name
    || repoOptions.value.find(r => r.key === key)?.name
    || key
  )
}

function nextStep() {
  // 校验当前步骤
  if (step.value === 1) {
    if (!form.name) {
      notifyWarning('请输入任务名称')
      return
    }
    if (!form.source_repo_key) {
      notifyWarning('请选择源仓库')
      return
    }
    if (!form.target_repo_key) {
      notifyWarning('请选择目标仓库')
      return
    }
    if (form.source_repo_key === form.target_repo_key) {
      notifyWarning('源仓库和目标仓库不能相同')
      return
    }
  }
  step.value++
}

async function submit() {
  if (!form.name || !form.source_repo_key || !form.target_repo_key) {
    notifyWarning('请填写必填项')
    return
  }
  submitting.value = true
  try {
    await taskStore.createTask(form)
    notifySuccess('创建任务成功')
    router.push('/sync')
  } catch (e) {
    notifyError(e, '创建任务失败')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadRepoPage(1, '').catch((e) => notifyError(e, '加载仓库失败'))
})
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

// 公共样式(page-container/header/title/subtitle/form-tip)统一使用 global.scss

.steps-bar {
  margin-bottom: $spacing-lg;
  padding: $spacing-lg $spacing-xl;
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
}

.form-card {
  background: $card-background;
  border-radius: $border-radius-md;
  box-shadow: $shadow-card;
  overflow: hidden;
}

.form-body {
  padding: $spacing-lg $spacing-xl;
  min-height: 320px;
}

.form-footer {
  padding: 16px $spacing-xl;
  border-top: 1px solid $border-color;
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-tip {
  font-size: 12px;
  color: $text-secondary;
  margin-top: 4px;

  .anticon {
    margin-right: 4px;
  }
}

.option-item {
  padding: 12px 16px;
  background: #FAFAFA;
  border-radius: $border-radius-md;
  border: 1px solid $border-color;

  .ant-checkbox-wrapper {
    font-weight: 500;
  }
}

.option-desc {
  font-size: 12px;
  color: $text-secondary;
  margin-top: 4px;
  padding-left: 24px;
}

// Cron presets
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
    color: $primary-color;
    border-color: $primary-color;
  }
}

.summary-section {
  :deep(.ant-descriptions-item-label) {
    width: 100px;
  }
}
</style>
