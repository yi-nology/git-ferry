<template>
  <div class="page-container">
    <!-- Page Header -->
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">仓库管理</h1>
        <p class="page-subtitle">管理您的代码仓库，配置同步源和目标</p>
      </div>
      <a-space>
        <a-button @click="handleSyncPlatform">
          <template #icon><SyncOutlined /></template>
          同步平台仓库
        </a-button>
        <a-button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          添加仓库
        </a-button>
      </a-space>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar">
      <a-input
        v-model:value="filters.search"
        placeholder="搜索仓库名称或地址..."
        allow-clear
        class="filter-input"
        @pressEnter="handleSearch"
      >
        <template #prefix><SearchOutlined /></template>
      </a-input>
      <a-select
        v-model:value="filters.platform"
        placeholder="全部平台"
        allow-clear
        class="filter-select"
        @change="handleSearch"
      >
        <a-select-option v-for="(config, key) in platformDisplayConfig" :key="key" :value="key">
          {{ config.name }}
        </a-select-option>
      </a-select>
      <a-select
        v-model:value="filters.status"
        placeholder="全部状态"
        allow-clear
        class="filter-select"
        @change="handleSearch"
      >
        <a-select-option value="active">正常</a-select-option>
        <a-select-option value="error">异常</a-select-option>
      </a-select>
      <a-button @click="handleRefresh" :loading="repoStore.loading">
        <template #icon><ReloadOutlined /></template>
        刷新
      </a-button>
    </div>

    <!-- Repo Cards Grid -->
    <div class="repo-grid" v-if="repoStore.repos.length > 0 || repoStore.loading">
      <div
        v-for="repo in repoStore.repos"
        :key="repo.key"
        v-memo="[repo.status, repo.name, repo.platform, testingKeys[repo.key]]"
        class="repo-card"
        :class="{ 'repo-card--error': repo.status === 'error' }"
      >
        <div class="repo-card__header">
          <div class="repo-card__icon" :style="{ background: platformDisplayConfig[repo.platform]?.color || '#8C8C8C' }">
            <component :is="platformDisplayConfig[repo.platform]?.icon || MoreOutlined" />
          </div>
          <a-tag :color="platformDisplayConfig[repo.platform]?.color || '#8C8C8C'" class="repo-card__platform-tag">
            {{ platformDisplayConfig[repo.platform]?.name || repo.platform }}
          </a-tag>
        </div>

        <div class="repo-card__body">
          <a-tooltip :title="repo.name">
            <div class="repo-card__name" @click="router.push(`/local-repos/${repo.key}`)">
              {{ repo.name }}
            </div>
          </a-tooltip>
          <a-tooltip :title="repo.clone_url">
            <div class="repo-card__url">{{ repo.clone_url }}</div>
          </a-tooltip>

          <div class="repo-card__meta">
            <div class="repo-card__meta-item">
              <BranchesOutlined />
              <span>{{ repo.default_branch || 'main' }}</span>
            </div>
            <div class="repo-card__meta-item">
              <SyncOutlined />
              <span>{{ getTaskCount(repo.key) }} 个任务</span>
            </div>
          </div>
        </div>

        <div class="repo-card__footer">
          <StatusBadge :status="repo.status" />
          <a-space :size="4">
            <a-tooltip title="测试连接">
              <a-button
                type="text"
                size="small"
                :loading="testingKeys[repo.key]"
                @click="testConn(repo.key)"
              >
                <template #icon><ApiOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip title="编辑">
              <a-button type="text" size="small" @click="openEdit(repo)">
                <template #icon><EditOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-popconfirm
              title="确定要删除该仓库吗？删除后关联的同步任务也将受影响。"
              ok-text="确定删除"
              cancel-text="取消"
              @confirm="handleDelete(repo.key)"
            >
              <a-tooltip title="删除">
                <a-button type="text" danger size="small">
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </a-tooltip>
            </a-popconfirm>
          </a-space>
        </div>
      </div>

      <!-- Loading skeleton cards -->
      <template v-if="repoStore.loading">
        <div v-for="i in 3" :key="'skeleton-' + i" class="repo-card repo-card--skeleton">
          <a-skeleton active :paragraph="{ rows: 3 }" />
        </div>
      </template>
    </div>

    <!-- Empty State -->
    <div v-else class="empty-state">
      <a-empty description="暂无仓库数据">
        <a-button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          添加第一个仓库
        </a-button>
      </a-empty>
    </div>

    <!-- Pagination -->
    <div class="pagination-wrapper" v-if="repoStore.total > 0">
      <a-pagination
        v-model:current="currentPage"
        v-model:pageSize="pageSize"
        :total="repoStore.total"
        :show-size-changer="true"
        :show-total="(total: number) => `共 ${total} 个仓库`"
        @change="handlePageChange"
        @showSizeChange="handlePageChange"
      />
    </div>

    <!-- Create/Edit Modal -->
    <a-modal
      v-model:open="dialogVisible"
      :title="dialogTitle"
      :width="600"
      @ok="handleSubmit"
      ok-text="确定"
      cancel-text="取消"
      :ok-button-props="{ loading: submitting }"
    >
      <a-form :model="formData" layout="vertical">
        <!-- 步骤1: 选择已配置的平台 -->
        <a-form-item label="选择平台" required>
          <a-select
            v-model:value="formData.platform_id"
            placeholder="请先在平台管理中配置平台"
            size="large"
            @change="handlePlatformConfigSelect"
          >
            <a-select-option
              v-for="p in configuredPlatforms"
              :key="p.id"
              :value="p.id"
            >
              <div style="display: flex; align-items: center; gap: 8px;">
                <a-badge :status="p.status === 'active' ? 'success' : 'error'" />
                <span style="font-weight: 500;">{{ p.name }}</span>
                <a-tag size="small" :color="platformDisplayConfig[p.type]?.color">
                  {{ platformDisplayConfig[p.type]?.name || p.type }}
                </a-tag>
                <span v-if="p.isDefault" style="color: #1890ff; font-size: 12px;">(默认)</span>
              </div>
            </a-select-option>
          </a-select>
          <div class="form-tip" v-if="configuredPlatforms.length === 0">
            <WarningOutlined style="color: #faad14" />
            还没有配置平台，请先
            <a @click="goToPlatformSettings" style="color: #1890ff; cursor: pointer; font-weight: 500;">
              去配置平台
            </a>
          </div>
          <div class="form-tip" v-else>
            <InfoCircleOutlined />
            选择已配置的平台，平台包含 API 地址、访问令牌等配置
          </div>
        </a-form-item>

        <!-- 选择平台后的表单 -->
        <template v-if="selectedPlatformConfig">
          <!-- 仓库地址 -->
          <a-form-item label="仓库地址" required>
            <a-input
              v-model:value="formData.remote_url"
              :placeholder="urlPlaceholder"
              size="large"
            />
            <div class="form-tip">
              <InfoCircleOutlined /> {{ urlTip }}
            </div>
          </a-form-item>

          <!-- 仓库名称 -->
          <a-form-item label="仓库名称" required>
            <a-input
              v-model:value="formData.name"
              placeholder="例如: my-project（留空将自动从地址提取）"
              :maxlength="64"
              show-count
            />
          </a-form-item>

          <!-- 平台信息展示 -->
          <a-alert type="info" show-icon style="margin-top: 16px;">
            <template #message>
              <span style="font-weight: 500;">平台信息: {{ selectedPlatformConfig.name }}</span>
            </template>
            <template #description>
              <div style="font-size: 12px; line-height: 1.8;">
                <p style="margin-bottom: 4px;">
                  <strong>API 地址:</strong> {{ selectedPlatformConfig.url }}
                </p>
                <p style="margin-bottom: 4px;">
                  <strong>证书验证:</strong>
                  <a-tag :color="selectedPlatformConfig.skip_tls_verify ? 'warning' : 'success'" size="small">
                    {{ selectedPlatformConfig.skip_tls_verify ? '已跳过' : '正常' }}
                  </a-tag>
                  <a-tag v-if="selectedPlatformConfig.proxy_url" color="blue" size="small" style="margin-left: 4px;">
                    使用代理
                  </a-tag>
                </p>
                <p style="margin-bottom: 0;">
                  <strong>访问令牌:</strong>
                  <a-tag :color="selectedPlatformConfig.has_token ? 'success' : 'error'" size="small">
                    {{ selectedPlatformConfig.has_token ? '已配置' : '未配置' }}
                  </a-tag>
                </p>
              </div>
            </template>
          </a-alert>

          <!-- 常见问题 -->
          <a-collapse v-model:activeKey="faqExpanded" ghost style="margin-top: 16px;">
            <a-collapse-panel key="faq" header="常见问题">
              <div class="faq-content">
                <div class="faq-item">
                  <h4>证书相关问题</h4>
                  <p><strong>Q: 提示 "certificate has expired" 或 "certificate revoked"</strong></p>
                  <p>A: 证书已过期或被吊销，请在平台管理中勾选"跳过 TLS 证书验证"。</p>
                  <p><strong>Q: 提示 "certificate signed by unknown authority"</strong></p>
                  <p>A: 使用了自签名证书或私有 CA，请在平台管理中配置 CA 证书路径或跳过验证。</p>
                </div>
                <div class="faq-item">
                  <h4>认证相关问题</h4>
                  <p><strong>Q: 提示 "authentication failed"</strong></p>
                  <p>A: 请在平台管理中检查访问令牌是否正确、是否过期、是否有足够权限。</p>
                </div>
              </div>
            </a-collapse-panel>
          </a-collapse>
        </template>

        <!-- 没有平台时的提示 -->
        <a-result
          v-else-if="configuredPlatforms.length === 0"
          status="warning"
          title="需要先配置平台"
          sub-title="在添加仓库之前，请先配置一个 Git 平台（如 GitHub、GitLab、私有部署等）"
        >
          <template #extra>
            <a-button type="primary" size="large" @click="goToPlatformSettings">
              <SettingOutlined /> 去配置平台
            </a-button>
          </template>
        </a-result>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
defineOptions({ name: 'RepoList' })

import { onMounted, onActivated, ref, shallowRef, reactive, computed, markRaw } from 'vue'
import { useRouter } from 'vue-router'
import {
  PlusOutlined,
  SearchOutlined,
  GithubOutlined,
  GitlabOutlined,
  CloudOutlined,
  MoreOutlined,
  InfoCircleOutlined,
  CodeOutlined,
  QqOutlined,
  WarningOutlined,
  ApiOutlined,
  EditOutlined,
  DeleteOutlined,
  SettingOutlined,
  ReloadOutlined,
  SyncOutlined,
  BranchesOutlined,
} from '@ant-design/icons-vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import { useRepoStore } from '@/stores/repo'
import { useSyncTaskStore } from '@/stores/syncTask'
import { platformApi } from '@/api/platform'
import type { Repo } from '@/types'
import { notifySuccess, notifyError, notifyWarning, notifyInfo } from '@/utils/notify'

interface PlatformConfig {
  id: string
  key: string
  type: string
  name: string
  url: string
  has_token: boolean
  skip_tls_verify: boolean
  ca_cert_path: string
  proxy_url: string
  isDefault: boolean
  status: 'active' | 'error'
}

const router = useRouter()
const repoStore = useRepoStore()
const taskStore = useSyncTaskStore()
const dialogVisible = ref(false)
const dialogTitle = ref('')
const editingKey = ref('')
const selectedPlatform = ref('github')
const selectedPlatformConfig = ref<PlatformConfig | null>(null)
const submitting = ref(false)
const testingKeys = shallowRef<Record<string, boolean>>({})
const faqExpanded = ref<string[]>([])

// Pagination state
const currentPage = ref(1)
const pageSize = ref(12)

// Filters
const filters = reactive({
  search: '',
  platform: undefined as string | undefined,
  status: undefined as string | undefined,
})

// 从后端加载已配置的平台(平台管理页与数据库同源)
const configuredPlatforms = ref<PlatformConfig[]>([])

async function loadConfiguredPlatforms() {
  try {
    const data = await platformApi.list()
    configuredPlatforms.value = (data.platforms || []).map(p => ({
      id: String(p.id),
      key: p.key,
      type: p.type,
      name: p.name,
      url: p.api_url || p.apiUrl,
      has_token: !!p.has_token,
      skip_tls_verify: !!(p.skip_tls_verify ?? p.skipTlsVerify),
      ca_cert_path: p.ca_cert_path || p.caCertPath || '',
      proxy_url: p.proxy_url || p.proxyUrl || '',
      isDefault: !!(p.is_default ?? p.isDefault),
      status: ((p.status === 'error') ? 'error' : 'active'),
    }))
  } catch (e: any) {
    configuredPlatforms.value = []
  }

  // 如果有默认平台，自动选中
  const defaultPlatform = configuredPlatforms.value.find(p => p.isDefault)
  if (defaultPlatform) {
    formData.platform_id = defaultPlatform.id
    selectedPlatformConfig.value = defaultPlatform
    selectedPlatform.value = defaultPlatform.type
  }
}

const formData = reactive({
  platform_id: '',
  name: '',
  remote_url: '',
  access_token: '',
  skip_tls_verify: false,
  ca_cert_path: '',
  proxy_url: '',
})

// Platform display config with colors and icons
const platformDisplayConfig: Record<string, {
  name: string
  color: string
  icon: any
}> = {
  github: { name: 'GitHub', color: '#24292E', icon: markRaw(GithubOutlined) },
  gitlab: { name: 'GitLab', color: '#FC6D26', icon: markRaw(GitlabOutlined) },
  gitea: { name: 'Gitea', color: '#609926', icon: markRaw(CloudOutlined) },
  gitee: { name: 'Gitee', color: '#C71D23', icon: markRaw(CloudOutlined) },
  gitcode: { name: 'GitCode', color: '#0066FF', icon: markRaw(CodeOutlined) },
  atomgit: { name: 'AtomGit', color: '#0084FF', icon: markRaw(CloudOutlined) },
  tencent_code: { name: '腾讯工蜂', color: '#00B4D8', icon: markRaw(QqOutlined) },
  other: { name: '其他', color: '#8C8C8C', icon: markRaw(MoreOutlined) },
}

// Platform form config
const platformConfig: Record<string, { urlPlaceholder: string; urlTip: string; tokenTip: string; tokenGuide: string; name: string }> = {
  github: {
    name: 'GitHub',
    urlPlaceholder: 'https://github.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://github.com/octocat/Hello-World.git',
    tokenTip: 'GitHub Personal Access Token (需要 repo 权限)',
    tokenGuide: '访问 GitHub → Settings → Developer settings → Personal access tokens → Generate new token',
  },
  gitlab: {
    name: 'GitLab',
    urlPlaceholder: 'https://gitlab.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://gitlab.com/gitlab-org/gitlab.git',
    tokenTip: 'GitLab Personal Access Token (需要 read_repository 权限)',
    tokenGuide: '访问 GitLab → User Settings → Access Tokens → Add personal access token',
  },
  gitea: {
    name: 'Gitea',
    urlPlaceholder: 'https://gitea.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://gitea.com/gitea/gitea.git',
    tokenTip: 'Gitea Access Token (需要 repo 权限)',
    tokenGuide: '访问 Gitea → Settings → Applications → Generate New Token',
  },
  gitee: {
    name: 'Gitee',
    urlPlaceholder: 'https://gitee.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://gitee.com/gitee/gitee.git',
    tokenTip: 'Gitee 私人令牌 (需要 projects 权限)',
    tokenGuide: '访问 Gitee → 设置 → 私人令牌 → 生成新令牌',
  },
  gitcode: {
    name: 'GitCode',
    urlPlaceholder: 'https://gitcode.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://gitcode.com/gitcode-org/gitcode.git',
    tokenTip: 'GitCode Access Token (需要 repo 权限)',
    tokenGuide: '访问 GitCode → 设置 → 访问令牌 → 新建访问令牌',
  },
  atomgit: {
    name: 'AtomGit',
    urlPlaceholder: 'https://atomgit.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://atomgit.com/atomgit/atomgit.git',
    tokenTip: 'AtomGit Access Token (需要 repo 权限)',
    tokenGuide: '访问 AtomGit → 设置 → 个人令牌 → 生成新令牌',
  },
  tencent_code: {
    name: '腾讯工蜂',
    urlPlaceholder: 'https://git.code.tencent.com/username/repository.git',
    urlTip: '支持 HTTPS 格式，例如: https://git.code.tencent.com/tencent/tdesign.git',
    tokenTip: '腾讯工蜂 Access Token (需要 repo 权限)',
    tokenGuide: '访问腾讯工蜂 → 设置 → 访问令牌 → 新建访问令牌',
  },
  other: {
    name: '其他平台',
    urlPlaceholder: 'https://git.example.com/username/repository.git',
    urlTip: '支持 HTTPS 和 SSH 格式',
    tokenTip: '平台对应的访问令牌',
    tokenGuide: '请参考对应平台的文档获取访问令牌',
  },
}

const platformName = computed(() => platformConfig[selectedPlatform.value]?.name || '其他平台')
const urlPlaceholder = computed(() => platformConfig[selectedPlatform.value]?.urlPlaceholder || '')
const urlTip = computed(() => platformConfig[selectedPlatform.value]?.urlTip || '')
const tokenTip = computed(() => platformConfig[selectedPlatform.value]?.tokenTip || '')
const tokenGuide = computed(() => platformConfig[selectedPlatform.value]?.tokenGuide || '')

// 预计算 repoKey → 任务数 的 Map,避免模板循环中 O(n×m) 逐次 filter
const taskCountMap = computed(() => {
  const map = new Map<string, number>()
  for (const t of taskStore.tasks) {
    map.set(t.source_repo_key, (map.get(t.source_repo_key) || 0) + 1)
    if (t.target_repo_key && t.target_repo_key !== t.source_repo_key) {
      map.set(t.target_repo_key, (map.get(t.target_repo_key) || 0) + 1)
    }
  }
  return map
})
function getTaskCount(repoKey: string): number {
  return taskCountMap.value.get(repoKey) || 0
}

function handlePlatformConfigSelect(platformId: string) {
  const platform = configuredPlatforms.value.find(p => p.id === platformId)
  if (platform) {
    selectedPlatformConfig.value = platform
    selectedPlatform.value = platform.type
    formData.remote_url = ''
    formData.name = ''
  }
}

function goToPlatformSettings() {
  dialogVisible.value = false
  router.push('/settings/platforms')
}

function buildParams() {
  return {
    page: currentPage.value,
    page_size: pageSize.value,
    ...(filters.search ? { search: filters.search } : {}),
    ...(filters.platform ? { platform: filters.platform } : {}),
    ...(filters.status ? { status: filters.status } : {}),
  }
}

async function loadRepos() {
  try {
    await repoStore.fetchRepos(buildParams())
  } catch (e) {
    notifyError(e, '加载仓库列表失败')
  }
}

function handleSearch() {
  currentPage.value = 1
  loadRepos()
}

function handleRefresh() {
  loadRepos()
}

function handlePageChange() {
  loadRepos()
}

async function handleSyncPlatform() {
  try {
    const data = await platformApi.list()
    const platforms = data.platforms || []
    if (!platforms.length) {
      notifyWarning('请先在系统-平台管理中配置平台')
      return
    }
    notifyInfo(`正在同步 ${platforms.length} 个平台的仓库...`)
    // 并行同步所有平台,而非逐个串行等待
    const results = await Promise.allSettled(
      platforms.map(async (p: any) => {
        const result = await platformApi.syncRepos(p.key)
        return { name: p.name, count: result.synced_count || 0 }
      }),
    )
    let total = 0
    const failed: string[] = []
    for (const r of results) {
      if (r.status === 'fulfilled') {
        total += r.value.count
      } else {
        const idx = results.indexOf(r)
        failed.push(platforms[idx]?.name || '未知')
      }
    }
    await loadRepos()
    if (failed.length) {
      notifyError(`部分平台同步失败: ${failed.join('、')}`)
    } else {
      notifySuccess(`同步完成，共导入 ${total} 个仓库`)
    }
  } catch (e: any) {
    notifyError(e, '同步平台仓库失败')
  }
}

onMounted(() => {
  loadRepos()
  taskStore.fetchTasks({ page: 1, page_size: 100 }).catch((e) => notifyError(e, '加载任务失败'))
  loadConfiguredPlatforms()
})

// keep-alive 缓存页:每次切回刷新,避免看到过期数据
onActivated(() => {
  loadRepos()
})

function openCreate() {
  dialogTitle.value = '添加仓库'
  editingKey.value = ''
  faqExpanded.value = []
  loadConfiguredPlatforms()

  // 选中默认平台
  const defaultPlatform = configuredPlatforms.value.find(p => p.isDefault)

  Object.assign(formData, {
    platform_id: defaultPlatform?.id || '',
    name: '', remote_url: '', access_token: '',
    skip_tls_verify: false, ca_cert_path: '', proxy_url: '',
  })

  selectedPlatformConfig.value = defaultPlatform || null
  selectedPlatform.value = defaultPlatform?.type || ''
  dialogVisible.value = true
}

function openEdit(repo: Repo) {
  dialogTitle.value = '编辑仓库'
  editingKey.value = repo.key
  faqExpanded.value = []
  loadConfiguredPlatforms()

  // 查找匹配的平台
  const matchingPlatform = configuredPlatforms.value.find(p => p.type === repo.platform) || null

  Object.assign(formData, {
    platform_id: matchingPlatform?.id || '',
    name: repo.name, remote_url: repo.clone_url, access_token: '',
    skip_tls_verify: false, ca_cert_path: '', proxy_url: '',
  })

  selectedPlatformConfig.value = matchingPlatform
  selectedPlatform.value = repo.platform || ''
  dialogVisible.value = true
}

async function handleSubmit() {
  if (!formData.name || !formData.remote_url) {
    notifyWarning('请填写仓库名称和地址')
    return
  }
  submitting.value = true
  try {
    if (editingKey.value) {
      await repoStore.updateRepo({ key: editingKey.value, name: formData.name, access_token: formData.access_token })
      notifySuccess('更新仓库成功')
    } else {
      await repoStore.createRepo(formData)
      notifySuccess('添加仓库成功')
    }
    dialogVisible.value = false
  } catch (e) {
    notifyError(e, '保存失败')
  } finally {
    submitting.value = false
  }
}

async function handleDelete(key: string) {
  try {
    await repoStore.deleteRepo(key)
    notifySuccess('删除仓库成功')
  } catch (e) {
    notifyError(e, '删除仓库失败')
  }
}

async function testConn(key: string) {
  testingKeys.value = { ...testingKeys.value, [key]: true }
  try {
    const result = await repoStore.testConnection(key)
    if (result.success) notifySuccess(result.message)
    else notifyWarning(result.message)
  } catch (e) {
    notifyError(e, '测试连接失败')
  } finally {
    testingKeys.value = { ...testingKeys.value, [key]: false }
  }
}
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

// 公共样式(page-container/header/title/subtitle/filter-bar 等)统一使用 global.scss
.filter-input {
  width: 260px;
}

// -- Repo grid --
.repo-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
}

// -- Repo card --
.repo-card {
  background: $bg-primary;
  border-radius: $radius-lg;
  box-shadow: $shadow-card;
  overflow: hidden;
  transition: all 0.3s ease;
  display: flex;
  flex-direction: column;
  border: 1px solid transparent;

  &:hover {
    box-shadow: $shadow-card-hover;
    transform: translateY(-2px);
  }

  &--error {
    border-color: $error;

    .repo-card__header {
      background: linear-gradient(135deg, #fff2f0 0%, #ffccc7 100%);
    }
  }

  &--skeleton {
    min-height: 200px;
    padding: $spacing-md;
  }
}

.repo-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: $spacing-md;
  background: linear-gradient(135deg, #f5f5f5 0%, #fafafa 100%);
  border-bottom: 1px solid $border;
}

.repo-card__icon {
  width: 40px;
  height: 40px;
  border-radius: $radius-md;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: #fff;
}

.repo-card__platform-tag {
  font-weight: 500;
  border: none;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

.repo-card__body {
  flex: 1;
  padding: $spacing-md;
}

.repo-card__name {
  font-size: 16px;
  font-weight: 600;
  color: $text-primary;
  margin-bottom: 4px;
  cursor: pointer;
  transition: color 0.2s;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:hover {
    color: $primary;
  }
}

.repo-card__url {
  font-size: 12px;
  color: $text-secondary;
  margin-bottom: $spacing-sm;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.repo-card__meta {
  display: flex;
  gap: $spacing-md;
  margin-top: $spacing-sm;
}

.repo-card__meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: $text-secondary;

  .anticon {
    font-size: 14px;
  }
}

.repo-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: $spacing-sm $spacing-md;
  border-top: 1px solid $border;
  background: #fafafa;
}

// -- Empty state --
.empty-state {
  background: $bg-primary;
  border-radius: $radius-lg;
  box-shadow: $shadow-card;
  padding: $spacing-xl * 2;
  text-align: center;
}

// -- Pagination --
.pagination-wrapper {
  display: flex;
  justify-content: center;
  padding: $spacing-md 0;
}

// -- Form tips --
.form-tip {
  font-size: 12px;
  color: $text-secondary;
  margin-top: 4px;

  .anticon {
    margin-right: 4px;
  }
}

// -- FAQ content --
.faq-content {
  .faq-item {
    margin-bottom: 16px;

    &:last-child {
      margin-bottom: 0;
    }

    h4 {
      margin-bottom: 8px;
      font-size: 14px;
      font-weight: 600;
    }

    p {
      margin-bottom: 4px;
      font-size: 13px;
      line-height: 1.6;
    }
  }
}

// -- Responsive --
@media (max-width: 768px) {
  .page-header-bar {
    flex-direction: column;
    gap: $spacing-md;
  }

  .filter-bar {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-input,
  .filter-select {
    width: 100%;
  }

  .repo-grid {
    grid-template-columns: 1fr;
  }
}
</style>
