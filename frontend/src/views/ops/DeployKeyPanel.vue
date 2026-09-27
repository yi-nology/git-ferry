<template>
  <div>
    <a-alert
      type="info"
      show-icon
      message="镜像部署密钥"
      description="生成 Ed25519 密钥对：私钥交给同步任务/密钥库，公钥粘贴到目标平台的 Deploy Keys。私钥仅本次返回，服务端不保存。"
      style="margin-bottom: 16px"
    />

    <a-space>
      <a-input
        v-model:value="comment"
        placeholder="备注，如 mirror-to-github"
        style="width: 260px"
        allow-clear
      />
      <a-button type="primary" :loading="loading" @click="generate">生成密钥</a-button>
    </a-space>

    <a-card v-if="result" title="生成结果" style="margin-top: 16px">
      <a-descriptions :column="1" bordered size="small">
        <a-descriptions-item label="指纹">
          <code>{{ result.fingerprint }}</code>
        </a-descriptions-item>
        <a-descriptions-item label="公钥 (authorized_keys)">
          <a-typography-paragraph copyable :content="result.public_key" style="margin: 0">
            <code style="word-break: break-all">{{ result.public_key }}</code>
          </a-typography-paragraph>
        </a-descriptions-item>
        <a-descriptions-item label="私钥 (PEM)">
          <a-typography-paragraph copyable :content="result.private_key_pem" style="margin: 0">
            <a-typography-text type="danger" style="word-break: break-all; font-family: monospace">
              {{ result.private_key_pem.slice(0, 80) }}…
            </a-typography-text>
            <a-typography-text type="secondary">（点复制获取完整私钥）</a-typography-text>
          </a-typography-paragraph>
        </a-descriptions-item>
      </a-descriptions>
      <a-alert type="warning" show-icon message="请立即保存私钥，离开本页后无法再次查看" style="margin-top: 12px" />
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { opsApi, type DeployKeyResult } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'DeployKeyPanel' })

const comment = ref('gitferry-mirror')
const loading = ref(false)
const result = ref<DeployKeyResult | null>(null)

async function generate() {
  loading.value = true
  try {
    result.value = await opsApi.generateDeployKey(comment.value || 'gitferry-mirror')
    notifySuccess('密钥已生成，请立即保存私钥')
  } catch (e) {
    notifyError(e, '生成部署密钥失败')
  } finally {
    loading.value = false
  }
}
</script>
