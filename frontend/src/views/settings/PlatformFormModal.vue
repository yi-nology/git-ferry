<template>
  <a-modal
    :open="open"
    :title="isEditing ? '编辑平台' : '添加平台'"
    :width="640"
    :footer="null"
    @cancel="handleClose"
  >
    <a-steps v-if="!isEditing" :current="currentStep" size="small" style="margin-bottom: 24px">
      <a-step title="选择平台" />
      <a-step title="配置平台" />
    </a-steps>

    <div v-if="currentStep === 0 && !isEditing" class="platform-type-grid">
      <div
        v-for="(preset, type) in PLATFORM_PRESETS"
        :key="type"
        class="platform-type-item"
        :class="{ selected: formData.type === type }"
        @click="selectPlatformType(String(type))"
      >
        <div class="type-icon" :style="{ background: platformColor(String(type)) }">
          <component :is="platformIcon(String(type))" style="font-size: 28px; color: white" />
        </div>
        <div class="type-info">
          <div class="type-name">{{ preset.name }}</div>
          <div class="type-desc">{{ preset.urlTip }}</div>
        </div>
        <CheckCircleFilled v-if="formData.type === type" class="check-icon" />
      </div>
    </div>

    <a-form v-if="currentStep === 1 || isEditing" :model="formData" layout="vertical">
      <a-form-item label="平台名称" required>
        <a-input v-model:value="formData.name" placeholder="例如: 公司 GitLab、GitHub" />
      </a-form-item>

      <a-form-item v-if="formData.type !== 'custom'" label="实例地址">
        <a-input
          v-model:value="formData.instance_url"
          :placeholder="instancePlaceholder"
          @change="handleInstanceUrlChange"
        />
        <div class="form-tip">{{ instanceTip }}</div>
      </a-form-item>

      <a-form-item label="API 地址" required>
        <a-input v-model:value="formData.url" :placeholder="urlPlaceholder" />
        <div class="form-tip">
          {{ urlTip }}
          <a v-if="formData.type !== 'custom'" style="margin-left: 8px; font-size: 12px" @click="resetApiUrl">
            恢复默认
          </a>
        </div>
      </a-form-item>

      <a-form-item label="访问令牌" required>
        <a-input-password v-model:value="formData.token" placeholder="请输入 Personal Access Token" />
        <div class="form-tip">{{ tokenTip }}</div>
      </a-form-item>

      <a-alert
        v-if="formData.type !== 'custom' && isPrivateInstance"
        type="info"
        show-icon
        style="margin-bottom: 16px"
      >
        <template #message>私有部署实例</template>
        <template #description>
          自签名证书请勾选「跳过 TLS 证书验证」;内部 CA 请提供证书路径。
        </template>
      </a-alert>

      <a-collapse ghost style="margin-bottom: 16px">
        <a-collapse-panel key="advanced" header="高级选项">
          <a-form-item>
            <a-checkbox v-model:checked="formData.skip_tls_verify">跳过 TLS 证书验证</a-checkbox>
          </a-form-item>
          <a-form-item label="自定义 CA 证书路径">
            <a-input v-model:value="formData.ca_cert_path" placeholder="/path/to/ca-bundle.crt" allow-clear />
          </a-form-item>
          <a-form-item label="HTTP 代理">
            <a-input v-model:value="formData.proxy_url" placeholder="http://proxy:8080" allow-clear />
          </a-form-item>
          <a-form-item label="SSH 主机指纹">
            <a-input
              v-model:value="formData.ssh_host_key_fingerprint"
              placeholder="SHA256:xxx (SSH 仓库必填可防 MITM)"
              allow-clear
            />
          </a-form-item>
          <a-form-item label="SSH known_hosts 路径">
            <a-input
              v-model:value="formData.ssh_known_hosts_path"
              placeholder="/etc/gitferry/known_hosts (可选)"
              allow-clear
            />
          </a-form-item>
          <a-form-item>
            <a-checkbox v-model:checked="formData.is_default">设为默认平台</a-checkbox>
          </a-form-item>
        </a-collapse-panel>
      </a-collapse>

      <div class="form-actions">
        <a-button v-if="!isEditing" @click="currentStep = 0">上一步</a-button>
        <div style="flex: 1" />
        <a-button @click="handleClose">取消</a-button>
        <a-button type="primary" :loading="submitting" @click="handleSubmit">
          {{ isEditing ? '保存' : '添加' }}
        </a-button>
      </div>
    </a-form>

    <div v-if="currentStep === 0 && !isEditing" class="form-actions">
      <div style="flex: 1" />
      <a-button @click="handleClose">取消</a-button>
      <a-button type="primary" :disabled="!formData.type" @click="currentStep = 1">下一步</a-button>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { CheckCircleFilled } from '@ant-design/icons-vue'
import { platformApi, type Platform, type CreatePlatformRequest, type UpdatePlatformRequest } from '@/api/platform'
import { PLATFORM_PRESETS } from '@/constants/platformPresets'
import { platformColor, platformIcon, getInstanceUrl, getApiUrl, getSkipTls, getIsDefault } from '@/utils/platformDisplay'

defineOptions({ name: 'PlatformFormModal' })

const props = defineProps<{
  open: boolean
  platform?: Platform | null
}>()

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'saved'): void
}>()

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
  ssh_host_key_fingerprint: '',
  ssh_known_hosts_path: '',
})

const urlPlaceholder = ref('')
const urlTip = ref('')
const tokenTip = ref('')
const instancePlaceholder = ref('')
const instanceTip = ref('')

const isEditing = computed(() => !!props.platform)

const isPrivateInstance = computed(() => {
  const preset = PLATFORM_PRESETS[formData.type]
  if (!preset || formData.type === 'custom') return false
  return !!formData.instance_url && formData.instance_url !== preset.defaultInstance
})

function applyPreset(type: string) {
  const preset = PLATFORM_PRESETS[type]
  if (!preset) return
  formData.name = preset.name
  formData.instance_url = ''
  formData.url = `https://${preset.defaultInstance}${preset.apiPath}`
  urlPlaceholder.value = formData.url
  urlTip.value = preset.urlTip
  tokenTip.value = preset.tokenTip
  instancePlaceholder.value = preset.instancePlaceholder
  instanceTip.value = `留空使用默认: ${preset.defaultInstance}`
}

function selectPlatformType(type: string) {
  formData.type = type
  applyPreset(type)
}

function handleInstanceUrlChange() {
  const preset = PLATFORM_PRESETS[formData.type]
  if (preset && formData.instance_url) {
    const protocol = formData.instance_url.startsWith('http') ? '' : 'https://'
    formData.url = `${protocol}${formData.instance_url}${preset.apiPath}`
    urlTip.value = `自动生成: ${formData.url}`
  }
}

function resetApiUrl() {
  const preset = PLATFORM_PRESETS[formData.type]
  if (!preset) return
  const instance = formData.instance_url || preset.defaultInstance
  formData.url = `https://${instance}${preset.apiPath}`
  urlTip.value = preset.urlTip
}

watch(
  () => props.open,
  (v) => {
    if (!v) return
    if (props.platform) {
      const p = props.platform
      currentStep.value = 1
      formData.type = p.type
      formData.name = p.name
      formData.instance_url = getInstanceUrl(p)
      formData.url = getApiUrl(p)
      formData.token = ''
      formData.skip_tls_verify = getSkipTls(p)
      formData.ca_cert_path = p.ca_cert_path || ''
      formData.proxy_url = p.proxy_url || ''
      formData.is_default = getIsDefault(p)
      formData.ssh_host_key_fingerprint = p.ssh_host_key_fingerprint || ''
      formData.ssh_known_hosts_path = p.ssh_known_hosts_path || ''
      const preset = PLATFORM_PRESETS[p.type]
      if (preset) {
        urlPlaceholder.value = `https://${preset.defaultInstance}${preset.apiPath}`
        urlTip.value = preset.urlTip
        tokenTip.value = preset.tokenTip
        instancePlaceholder.value = preset.instancePlaceholder
        instanceTip.value = formData.instance_url ? '私有部署实例' : `留空使用默认: ${preset.defaultInstance}`
      }
    } else {
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
      formData.ssh_host_key_fingerprint = ''
      formData.ssh_known_hosts_path = ''
      applyPreset('github')
    }
  },
  { immediate: true },
)

function handleClose() {
  emit('update:open', false)
}

async function handleSubmit() {
  if (!formData.name || !formData.url) {
    message.warning('请填写必填项')
    return
  }
  if (!isEditing.value && !formData.token) {
    message.warning('请填写访问令牌')
    return
  }
  submitting.value = true
  try {
    const base: CreatePlatformRequest = {
      type: formData.type,
      name: formData.name,
      instance_url: formData.instance_url,
      api_url: formData.url,
      skip_tls_verify: formData.skip_tls_verify,
      ca_cert_path: formData.ca_cert_path,
      proxy_url: formData.proxy_url,
      is_default: formData.is_default,
      ssh_host_key_fingerprint: formData.ssh_host_key_fingerprint,
      ssh_known_hosts_path: formData.ssh_known_hosts_path,
    }
    if (formData.token) base.access_token = formData.token

    if (isEditing.value && props.platform) {
      const upd: UpdatePlatformRequest = { key: props.platform.key, ...base }
      await platformApi.update(upd)
    } else {
      await platformApi.create(base)
    }
    handleClose()
    message.success('保存成功')
    emit('saved')
  } catch (e) {
    message.error((e as Error)?.message || '保存失败')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped lang="scss">
.platform-type-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}
.platform-type-item {
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  padding: 12px;
  display: flex;
  gap: 12px;
  align-items: center;
  cursor: pointer;
  position: relative;
  &:hover { border-color: #1677ff; }
  &.selected { border-color: #1677ff; background: #e6f4ff; }
}
.type-icon {
  width: 48px; height: 48px; border-radius: 8px;
  display: flex; align-items: center; justify-content: center;
}
.type-name { font-weight: 500; }
.type-desc { font-size: 12px; color: #888; margin-top: 2px; }
.check-icon { position: absolute; right: 8px; top: 8px; color: #1677ff; }
.form-tip { font-size: 12px; color: #888; margin-top: 4px; }
.form-actions { display: flex; gap: 8px; margin-top: 8px; }
</style>
