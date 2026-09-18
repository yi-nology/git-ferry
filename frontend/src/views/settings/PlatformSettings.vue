<template>
  <div class="page-container">
    <PageHeader title="平台管理">
      <template #actions>
        <a-button type="primary" @click="openAdd">
          <template #icon><PlusOutlined /></template>
          添加平台
        </a-button>
      </template>
    </PageHeader>
    <p class="page-subtitle">管理 Git 平台配置，用于连接远程仓库</p>

    <!-- 平台列表 -->
    <div class="platform-grid">
      <a-card
        v-for="platform in platforms"
        :key="platform.key"
        class="platform-card"
        :class="{ 'is-default': getIsDefault(platform) }"
        hoverable
      >
        <template #cover>
          <div class="platform-cover" :style="{ background: getPlatformColor(platform.type) }">
            <div class="platform-icon">
              <component :is="getPlatformIcon(platform.type)" style="font-size: 48px; color: white" />
            </div>
            <a-tag v-if="getIsDefault(platform)" color="white" class="default-tag">默认</a-tag>
          </div>
        </template>

        <a-card-meta :title="platform.name" :description="platform.type.toUpperCase()">
          <template #avatar>
            <a-badge :status="platform.status === 'active' ? 'success' : 'error'" />
          </template>
        </a-card-meta>

        <div class="platform-info">
          <div class="info-item">
            <GlobalOutlined class="info-icon" />
            <span class="info-label">实例地址</span>
            <span class="info-value">{{ getInstanceUrl(platform) || platformPresets[platform.type]?.defaultInstance || '—' }}</span>
          </div>
          <div class="info-item">
            <ApiOutlined class="info-icon" />
            <span class="info-label">API 地址</span>
            <span class="info-value">{{ getApiUrl(platform) }}</span>
          </div>
          <div class="info-item">
            <LockOutlined class="info-icon" />
            <span class="info-label">TLS 验证</span>
            <a-tag :color="getSkipTls(platform) ? 'warning' : 'success'" size="small">
              {{ getSkipTls(platform) ? '已跳过' : '已启用' }}
            </a-tag>
          </div>
          <div class="info-item">
            <KeyOutlined class="info-icon" />
            <span class="info-label">Token</span>
            <a-tag :color="platform.has_token ? 'success' : 'default'" size="small">
              {{ platform.has_token ? '已配置' : '未配置' }}
            </a-tag>
          </div>
          <div class="info-item">
            <DatabaseOutlined class="info-icon" />
            <span class="info-label">仓库数</span>
            <span class="info-value">{{ getRepoCount(platform) }}</span>
          </div>
          <div class="info-item" v-if="getLastTestAt(platform)">
            <ClockCircleOutlined class="info-icon" />
            <span class="info-label">最后测试</span>
            <span class="info-value">{{ formatTime(getLastTestAt(platform)) }}</span>
            <a-tag :color="platform.status === 'active' ? 'success' : 'error'" size="small" style="margin-left: 4px">
              {{ platform.status === 'active' ? '成功' : '失败' }}
            </a-tag>
          </div>
        </div>

        <template #actions>
          <a-tooltip title="测试连接">
            <a-button type="text" size="small" @click="testConnection(platform)">
              <ApiOutlined />
            </a-button>
          </a-tooltip>
          <a-tooltip title="同步仓库">
            <a-button type="text" size="small" @click="syncRepos(platform)">
              <SyncOutlined />
            </a-button>
          </a-tooltip>
          <a-tooltip title="设为默认">
            <a-button type="text" size="small" @click="setDefault(platform)" :disabled="getIsDefault(platform)">
              <StarOutlined />
            </a-button>
          </a-tooltip>
          <a-tooltip title="编辑">
            <a-button type="text" size="small" @click="openEdit(platform)">
              <EditOutlined />
            </a-button>
          </a-tooltip>
          <a-popconfirm
            title="确定要删除该平台配置吗？"
            ok-text="确定"
            cancel-text="取消"
            @confirm="handleDelete(platform.key)"
          >
            <a-tooltip title="删除">
              <a-button type="text" size="small" danger>
                <DeleteOutlined />
              </a-button>
            </a-tooltip>
          </a-popconfirm>
        </template>
      </a-card>

      <!-- 添加平台卡片 -->
      <a-card class="platform-card add-card" hoverable @click="openAdd">
        <div class="add-content">
          <PlusCircleOutlined style="font-size: 48px; color: #1677ff" />
          <p>添加新平台</p>
        </div>
      </a-card>
    </div>

    <!-- 使用提示 -->
    <a-alert class="usage-tips" type="info" show-icon>
      <template #message>使用提示</template>
      <template #description>
        <ul class="tips-list">
          <li>添加平台后，请先点击"测试连接"验证配置是否正确</li>
          <li>点击"同步仓库"可将远程仓库列表同步到本地</li>
          <li>设置默认平台后，新建同步任务时会自动选择该平台</li>
          <li>私有部署实例如使用自签名证书，请开启"跳过 TLS 验证"</li>
        </ul>
      </template>
    </a-alert>

    <!-- 添加/编辑对话框 -->
    <a-modal
      v-model:open="dialogVisible"
      :title="isEditing ? '编辑平台' : '添加平台'"
      :width="640"
      :footer="null"
      @cancel="handleClose"
    >
      <!-- 两步创建流程 -->
      <a-steps :current="currentStep" size="small" style="margin-bottom: 24px;" v-if="!isEditing">
        <a-step title="选择平台" />
        <a-step title="配置平台" />
      </a-steps>

      <!-- 步骤 1: 选择平台类型 -->
      <div v-if="currentStep === 0 && !isEditing" class="platform-type-grid">
        <div
          v-for="(preset, type) in platformPresets"
          :key="type"
          class="platform-type-item"
          :class="{ selected: formData.type === type }"
          @click="selectPlatformType(type as string)"
        >
          <div class="type-icon" :style="{ background: getPlatformColor(type as string) }">
            <component :is="getPlatformIcon(type as string)" style="font-size: 28px; color: white" />
          </div>
          <div class="type-info">
            <div class="type-name">{{ preset.name }}</div>
            <div class="type-desc">{{ preset.urlTip }}</div>
          </div>
          <CheckCircleFilled v-if="formData.type === type" class="check-icon" />
        </div>
      </div>

      <!-- 步骤 2: 配置平台 -->
      <a-form
        v-if="currentStep === 1 || isEditing"
        :model="formData"
        layout="vertical"
        class="platform-form"
      >
        <!-- 平台名称 -->
        <a-form-item label="平台名称" required>
          <a-input v-model:value="formData.name" placeholder="例如: 公司 GitLab、GitHub" />
          <div class="form-tip">
            <InfoCircleOutlined /> 给这个平台起一个容易识别的名称
          </div>
        </a-form-item>

        <!-- 实例地址 (对于私有部署) -->
        <a-form-item v-if="formData.type !== 'custom'" label="实例地址">
          <a-input
            v-model:value="formData.instance_url"
            :placeholder="instancePlaceholder"
            @change="handleInstanceUrlChange"
          />
          <div class="form-tip">
            <InfoCircleOutlined v-if="!formData.instance_url" />
            <CheckCircleOutlined v-else style="color: #52c41a" />
            {{ instanceTip }}
          </div>
        </a-form-item>

        <!-- API 地址 (可编辑) -->
        <a-form-item label="API 地址" required>
          <a-input v-model:value="formData.url" :placeholder="urlPlaceholder" />
          <div class="form-tip">
            <InfoCircleOutlined />
            {{ urlTip }}
            <a v-if="formData.type !== 'custom'" @click="resetApiUrl" style="margin-left: 8px; font-size: 12px;">
              恢复默认
            </a>
          </div>
        </a-form-item>

        <!-- 访问令牌 -->
        <a-form-item label="访问令牌" required>
          <a-input-password v-model:value="formData.token" placeholder="请输入 Personal Access Token" />
          <div class="form-tip">
            <InfoCircleOutlined /> {{ tokenTip }}
          </div>
        </a-form-item>

        <!-- 提示信息 -->
        <a-alert v-if="formData.type !== 'custom' && isPrivateInstance" type="info" show-icon style="margin-bottom: 16px;">
          <template #message>
            <span style="font-weight: 500;">私有部署实例</span>
          </template>
          <template #description>
            <div style="font-size: 12px; line-height: 1.8;">
              <p style="margin-bottom: 4px;">检测到您正在配置私有部署的 {{ platformPresets[formData.type]?.name }} 实例。</p>
              <p style="margin-bottom: 4px;">如果是自签名证书，请勾选下方的"跳过 TLS 证书验证"。</p>
              <p style="margin-bottom: 0;">如果是内部 CA 签发的证书，请提供 CA 证书路径。</p>
            </div>
          </template>
        </a-alert>

        <!-- 高级选项 -->
        <a-collapse ghost style="margin-bottom: 16px;">
          <a-collapse-panel key="advanced" header="高级选项">
            <!-- 跳过证书验证 -->
            <a-form-item>
              <a-checkbox v-model:checked="formData.skip_tls_verify">
                跳过 TLS 证书验证
              </a-checkbox>
              <div class="form-tip">
                <WarningOutlined style="color: #faad14" />
                适用于证书过期、自签名、Nginx代理等情况
              </div>
            </a-form-item>

            <!-- 自定义 CA 证书 -->
            <a-form-item label="自定义 CA 证书路径">
              <a-input
                v-model:value="formData.ca_cert_path"
                placeholder="/path/to/ca-bundle.crt (可选)"
                allow-clear
              />
            </a-form-item>

            <!-- HTTP 代理 -->
            <a-form-item label="HTTP 代理">
              <a-input
                v-model:value="formData.proxy_url"
                placeholder="http://proxy:8080 (可选)"
                allow-clear
              />
            </a-form-item>

            <!-- 设为默认 -->
            <a-form-item>
              <a-checkbox v-model:checked="formData.is_default">
                设为默认平台
              </a-checkbox>
            </a-form-item>
          </a-collapse-panel>
        </a-collapse>

        <!-- 操作按钮 -->
        <div class="form-actions">
          <a-button v-if="!isEditing" @click="prevStep">上一步</a-button>
          <div style="flex: 1;" />
          <a-button @click="handleClose">取消</a-button>
          <a-button type="primary" :loading="submitting" @click="handleSubmit">
            {{ isEditing ? '保存' : '添加' }}
          </a-button>
        </div>
      </a-form>

      <!-- 步骤 1 底部按钮 -->
      <div v-if="currentStep === 0 && !isEditing" class="form-actions">
        <div style="flex: 1;" />
        <a-button @click="handleClose">取消</a-button>
        <a-button type="primary" @click="nextStep" :disabled="!formData.type">
          下一步
        </a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, markRaw } from 'vue'
import { message } from 'ant-design-vue'
import {
  PlusOutlined,
  PlusCircleOutlined,
  ApiOutlined,
  StarOutlined,
  EditOutlined,
  DeleteOutlined,
  GithubOutlined,
  GitlabOutlined,
  CloudOutlined,
  CodeOutlined,
  AppstoreOutlined,
  QqOutlined,
  SettingOutlined,
  InfoCircleOutlined,
  WarningOutlined,
  ReloadOutlined,
  SyncOutlined,
  GlobalOutlined,
  LockOutlined,
  KeyOutlined,
  DatabaseOutlined,
  ClockCircleOutlined,
  CheckCircleOutlined,
  CheckCircleFilled,
} from '@ant-design/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { platformApi, type Platform, type CreatePlatformRequest, type UpdatePlatformRequest } from '@/api/platform'
import { PLATFORM_COLOR } from '@/utils/platform'
import { formatTime } from '@/utils'

const platforms = ref<Platform[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingId = ref('')
const currentStep = ref(0)
const submitting = ref(false)

const formData = reactive({
  type: 'github',
  name: '',
  instance_url: '',
  url: '',
  token: '',
  skip_tls_verify: false,
  ca_cert_path: '',
  proxy_url: '',
  is_default: false,
})

// 平台配置
const platformPresets: Record<string, {
  name: string;
  defaultInstance: string;
  apiPath: string;
  urlTip: string;
  tokenTip: string;
  instancePlaceholder: string;
}> = {
  github: {
    name: 'GitHub',
    defaultInstance: 'github.com',
    apiPath: '/api/v3',
    urlTip: '企业版地址格式: https://github.company.com/api/v3',
    tokenTip: 'GitHub Personal Access Token (需要 repo 权限)',
    instancePlaceholder: 'github.com (默认) 或 github.company.com',
  },
  gitlab: {
    name: 'GitLab',
    defaultInstance: 'gitlab.com',
    apiPath: '/api/v4',
    urlTip: '自建地址格式: https://gitlab.company.com/api/v4',
    tokenTip: 'GitLab Personal Access Token (需要 read_repository 权限)',
    instancePlaceholder: 'gitlab.com (默认) 或 gitlab.company.com',
  },
  gitea: {
    name: 'Gitea',
    defaultInstance: 'gitea.com',
    apiPath: '/api/v1',
    urlTip: '自建地址格式: https://gitea.company.com/api/v1',
    tokenTip: 'Gitea Access Token (需要 repo 权限)',
    instancePlaceholder: 'gitea.com (默认) 或 gitea.company.com',
  },
  gitee: {
    name: 'Gitee',
    defaultInstance: 'gitee.com',
    apiPath: '/api/v5',
    urlTip: '默认 API 地址',
    tokenTip: 'Gitee 私人令牌 (需要 projects 权限)',
    instancePlaceholder: 'gitee.com',
  },
  gitcode: {
    name: 'GitCode',
    defaultInstance: 'gitcode.com',
    apiPath: '/api/v5',
    urlTip: '默认 API 地址',
    tokenTip: 'GitCode Access Token (需要 repo 权限)',
    instancePlaceholder: 'gitcode.com',
  },
  atomgit: {
    name: 'AtomGit',
    defaultInstance: 'atomgit.com',
    apiPath: '/api/v1',
    urlTip: '默认 API 地址',
    tokenTip: 'AtomGit Access Token (需要 repo 权限)',
    instancePlaceholder: 'atomgit.com',
  },
  tencent_code: {
    name: '腾讯工蜂',
    defaultInstance: 'git.code.tencent.com',
    apiPath: '/api/v3',
    urlTip: '默认 API 地址',
    tokenTip: '腾讯工蜂 Access Token (需要 repo 权限)',
    instancePlaceholder: 'git.code.tencent.com',
  },
  custom: {
    name: '自定义平台',
    defaultInstance: '',
    apiPath: '',
    urlTip: '请输入完整的 API 地址',
    tokenTip: '平台对应的访问令牌',
    instancePlaceholder: '',
  },
}

const urlPlaceholder = ref('')
const urlTip = ref('')
const tokenTip = ref('')
const instancePlaceholder = ref('')
const instanceTip = ref('')

// 是否是私有实例
const isPrivateInstance = computed(() => {
  const preset = platformPresets[formData.type]
  if (!preset || formData.type === 'custom') return false
  return formData.instance_url && formData.instance_url !== preset.defaultInstance
})

// 从后端 API 加载
async function loadPlatforms() {
  loading.value = true
  try {
    const result = await platformApi.list()
    platforms.value = result.platforms || []
  } catch (e) {
    message.error('加载平台列表失败')
    platforms.value = []
  } finally {
    loading.value = false
  }
}

// 获取平台颜色(统一使用全局常量)
function getPlatformColor(type: string): string {
  return PLATFORM_COLOR[type] || '#666666'
}

// 获取平台图标
function getPlatformIcon(type: string) {
  const icons: Record<string, any> = {
    github: GithubOutlined,
    gitlab: GitlabOutlined,
    gitea: CloudOutlined,
    gitee: CloudOutlined,
    gitcode: CodeOutlined,
    atomgit: AppstoreOutlined,
    tencent_code: QqOutlined,
    custom: SettingOutlined,
  }
  return markRaw(icons[type] || SettingOutlined)
}

// Helper functions to handle both camelCase and snake_case
function getInstanceUrl(p: Platform): string {
  return p.instanceUrl || p.instance_url || ''
}
function getApiUrl(p: Platform): string {
  return p.apiUrl || p.api_url || ''
}
function getSkipTls(p: Platform): boolean {
  return p.skip_tls_verify || p.skipTlsVerify || false
}
function getIsDefault(p: Platform): boolean {
  return p.is_default || p.isDefault || false
}
function getRepoCount(p: Platform): number {
  return p.repo_count || p.repoCount || 0
}
function getLastTestAt(p: Platform): string {
  return p.last_test_at || p.lastTestAt || ''
}


// 平台类型变化
function handleTypeChange(type: string) {
  const preset = platformPresets[type]
  if (preset) {
    formData.name = preset.name
    formData.instance_url = ''
    formData.url = `https://${preset.defaultInstance}${preset.apiPath}`
    urlPlaceholder.value = `https://${preset.defaultInstance}${preset.apiPath}`
    urlTip.value = preset.urlTip
    tokenTip.value = preset.tokenTip
    instancePlaceholder.value = preset.instancePlaceholder
    instanceTip.value = `留空使用默认: ${preset.defaultInstance}`
  }
}

// 选择平台类型
function selectPlatformType(type: string) {
  formData.type = type
  handleTypeChange(type)
}

// 实例地址变化
function handleInstanceUrlChange() {
  const preset = platformPresets[formData.type]
  if (preset && formData.instance_url) {
    // 根据实例地址生成 API 地址
    const protocol = formData.instance_url.startsWith('http') ? '' : 'https://'
    formData.url = `${protocol}${formData.instance_url}${preset.apiPath}`
    urlTip.value = `自动生成: ${formData.url}`
  }
}

// 重置 API 地址
function resetApiUrl() {
  const preset = platformPresets[formData.type]
  if (preset) {
    const instance = formData.instance_url || preset.defaultInstance
    formData.url = `https://${instance}${preset.apiPath}`
    urlTip.value = preset.urlTip
  }
}

// 下一步
function nextStep() {
  if (!formData.type) {
    message.warning('请选择平台类型')
    return
  }
  currentStep.value = 1
}

// 上一步
function prevStep() {
  currentStep.value = 0
}

// 打开添加对话框
function openAdd() {
  isEditing.value = false
  currentStep.value = 0
  formData.type = 'github'
  formData.name = 'GitHub'
  formData.instance_url = ''
  formData.url = 'https://github.com/api/v3'
  formData.token = ''
  formData.skip_tls_verify = false
  formData.ca_cert_path = ''
  formData.proxy_url = ''
  formData.is_default = false
  urlPlaceholder.value = 'https://github.com/api/v3'
  urlTip.value = platformPresets.github.urlTip
  tokenTip.value = platformPresets.github.tokenTip
  instancePlaceholder.value = platformPresets.github.instancePlaceholder
  instanceTip.value = '留空使用默认: github.com'
  dialogVisible.value = true
}

// 打开编辑对话框
function openEdit(platform: Platform) {
  isEditing.value = true
  currentStep.value = 1
  editingId.value = platform.key
  formData.type = platform.type
  formData.name = platform.name
  formData.instance_url = platform.instance_url || ''
  formData.url = platform.api_url || ''
  formData.token = '' // 不回显 token
  formData.skip_tls_verify = platform.skip_tls_verify || false
  formData.ca_cert_path = platform.ca_cert_path || ''
  formData.proxy_url = platform.proxy_url || ''
  formData.is_default = platform.is_default || false

  const preset = platformPresets[platform.type]
  if (preset) {
    urlPlaceholder.value = `https://${preset.defaultInstance}${preset.apiPath}`
    urlTip.value = preset.urlTip
    tokenTip.value = preset.tokenTip
    instancePlaceholder.value = preset.instancePlaceholder
    instanceTip.value = formData.instance_url ? '私有部署实例' : `留空使用默认: ${preset.defaultInstance}`
  }

  dialogVisible.value = true
}

// 关闭对话框
function handleClose() {
  dialogVisible.value = false
  currentStep.value = 0
  isEditing.value = false
  editingId.value = ''
}

// 提交表单
async function handleSubmit() {
  if (!formData.name || !formData.url) {
    message.warning('请填写必填项')
    return
  }
  if (!isEditing.value && !formData.token) {
    message.warning('请填写访问令牌')
    return
  }

  const platformData: CreatePlatformRequest = {
    type: formData.type,
    name: formData.name,
    instance_url: formData.instance_url,
    api_url: formData.url,
    skip_tls_verify: formData.skip_tls_verify,
    ca_cert_path: formData.ca_cert_path,
    proxy_url: formData.proxy_url,
    is_default: formData.is_default,
  }

  // 只有填写了 token 才传递
  if (formData.token) {
    platformData.access_token = formData.token
  }

  submitting.value = true
  try {
    if (isEditing.value) {
      // 编辑:不含 type(后端不支持改平台类型),令牌仅在重填时更新
      const updateData: UpdatePlatformRequest = {
        key: editingId.value,
        name: platformData.name,
        instance_url: platformData.instance_url,
        api_url: platformData.api_url,
        skip_tls_verify: platformData.skip_tls_verify,
        ca_cert_path: platformData.ca_cert_path,
        proxy_url: platformData.proxy_url,
        is_default: platformData.is_default,
      }
      if (formData.token) updateData.access_token = formData.token
      await platformApi.update(updateData)
    } else {
      // 添加
      await platformApi.create(platformData)
    }

    handleClose()
    message.success('保存成功')
    await loadPlatforms()
  } catch (e: any) {
    message.error(e?.message || '保存失败')
  } finally {
    submitting.value = false
  }
}

// 删除平台
async function handleDelete(key: string) {
  try {
    await platformApi.delete(key)
    message.success('删除成功')
    await loadPlatforms()
  } catch (e: any) {
    message.error(e?.message || '删除失败')
  }
}

// 设为默认
async function setDefault(platform: Platform) {
  try {
    await platformApi.setDefault(platform.key)
    message.success('已设为默认平台')
    await loadPlatforms()
  } catch (e: any) {
    message.error(e?.message || '设置失败')
  }
}

// 测试连接
async function testConnection(platform: Platform) {
  message.loading({ content: '正在测试连接...', key: 'test' })
  try {
    const result = await platformApi.test(platform.key)
    if (result.result?.connected) {
      message.success({ content: '连接成功', key: 'test' })
    } else {
      message.error({ content: result.result?.message || '连接失败', key: 'test' })
    }
    await loadPlatforms()
  } catch (e: any) {
    message.error({ content: e?.message || '测试失败', key: 'test' })
  }
}

// 同步仓库
async function syncRepos(platform: Platform) {
  message.loading({ content: '正在同步仓库...', key: 'sync' })
  try {
    const result = await platformApi.syncRepos(platform.key)
    message.success({ content: `同步成功，共 ${result.synced_count || 0} 个仓库`, key: 'sync' })
    await loadPlatforms()
  } catch (e: any) {
    message.error({ content: e?.message || '同步失败', key: 'sync' })
  }
}

onMounted(() => {
  loadPlatforms()
})
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

.page-container {
  background: $bg-secondary;
  min-height: 100%;
  padding: $spacing-lg;
}

.page-subtitle {
  color: $text-secondary;
  margin: -$spacing-md 0 $spacing-lg 0;
  font-size: 14px;
}

.platform-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: $spacing-md;
  margin-top: $spacing-md;
}

.platform-card {
  border-radius: $radius-lg;
  overflow: hidden;
  transition: all 0.3s ease;
  box-shadow: $shadow-card;

  &:hover {
    transform: translateY(-4px);
    box-shadow: $shadow-card-hover;
  }

  &.is-default {
    border: 2px solid $primary;
  }

  &.add-card {
    cursor: pointer;
    border: 2px dashed $border;
    background: $bg-primary;
    min-height: 400px;
    display: flex;
    align-items: center;
    justify-content: center;

    &:hover {
      border-color: $primary;
      background: lighten($primary, 45%);
    }

    .add-content {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: $spacing-xl 0;

      p {
        margin-top: $spacing-md;
        font-size: 16px;
        color: $text-secondary;
      }
    }
  }
}

.platform-cover {
  height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;

  .platform-icon {
    width: 80px;
    height: 80px;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.2);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .default-tag {
    position: absolute;
    top: $spacing-sm;
    right: $spacing-sm;
  }
}

.platform-info {
  padding: $spacing-md 0;

  .info-item {
    display: flex;
    align-items: center;
    margin-bottom: $spacing-sm;
    font-size: 13px;

    .info-icon {
      color: $text-tertiary;
      margin-right: $spacing-sm;
      font-size: 14px;
    }

    .info-label {
      color: $text-secondary;
      margin-right: $spacing-sm;
      min-width: 70px;
    }

    .info-value {
      color: $text-primary;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
}

.form-tip {
  font-size: 12px;
  color: $text-secondary;
  margin-top: $spacing-xs;

  .anticon {
    margin-right: $spacing-xs;
  }
}

.form-actions {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  margin-top: $spacing-lg;
  padding-top: $spacing-md;
  border-top: 1px solid $border;
}

.usage-tips {
  margin-top: $spacing-lg;

  .tips-list {
    margin: 0;
    padding-left: $spacing-lg;

    li {
      margin-bottom: $spacing-xs;
      font-size: 13px;
      color: $text-secondary;

      &:last-child {
        margin-bottom: 0;
      }
    }
  }
}

.platform-type-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: $spacing-md;
  margin-bottom: $spacing-md;
}

.platform-type-item {
  display: flex;
  align-items: center;
  padding: $spacing-md;
  border: 2px solid $border;
  border-radius: $radius-md;
  cursor: pointer;
  transition: all 0.3s ease;
  position: relative;

  &:hover {
    border-color: $primary;
    background: lighten($primary, 48%);
  }

  &.selected {
    border-color: $primary;
    background: lighten($primary, 45%);
  }

  .type-icon {
    width: 48px;
    height: 48px;
    border-radius: $radius-md;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .type-info {
    margin-left: $spacing-md;
    flex: 1;
    min-width: 0;

    .type-name {
      font-weight: 500;
      color: $text-primary;
      margin-bottom: $spacing-xs;
    }

    .type-desc {
      font-size: 12px;
      color: $text-secondary;
      // 允许描述换行(最多 2 行),避免长 URL 示例被省略号截断
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      word-break: break-all;
      line-height: 1.5;
    }
  }

  .check-icon {
    position: absolute;
    top: $spacing-sm;
    right: $spacing-sm;
    color: $primary;
    font-size: 18px;
  }
}
</style>
