<template>
  <div class="page-container">
    <PageHeader title="AI 助手配置" subtitle="选择模型、配置 OpenAI 兼容端点与访问密钥">
      <template #actions>
        <a-button :loading="testing" @click="handleTest">
          <template #icon><ApiOutlined /></template>
          测试连接
        </a-button>
        <a-button type="primary" :loading="saving" @click="handleSave">
          <template #icon><SaveOutlined /></template>
          保存并生效
        </a-button>
      </template>
    </PageHeader>

    <!-- 当前状态 -->
    <div class="content-card status-card">
      <div class="card-header">
        <span class="card-title">当前状态</span>
        <a-tag :color="form.enabled ? 'green' : 'default'">
          {{ form.enabled ? '已启用' : '未启用' }}
        </a-tag>
      </div>
      <div class="card-body is-padded">
        <div class="status-row">
          <div class="status-item">
            <span class="status-label">模型</span>
            <span class="status-value mono">{{ form.model || '—' }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">端点</span>
            <span class="status-value mono">{{ form.base_url || '—' }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">API Key</span>
            <span class="status-value mono">
              {{ form.has_api_key ? form.api_key_masked || '已配置' : '未配置' }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <div class="config-grid">
      <!-- 基础配置 -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">基础配置</span>
        </div>
        <div class="card-body is-padded">
          <a-form layout="vertical">
            <a-form-item label="启用 AI 助手">
              <a-switch v-model:checked="form.enabled" />
              <div class="form-tip">
                关闭后聊天入口隐藏，/ai/* 返回 501；配置仍会保存。
              </div>
            </a-form-item>

            <a-form-item label="服务预设">
              <a-select v-model:value="preset" @change="applyPreset">
                <a-select-option value="openai">OpenAI</a-select-option>
                <a-select-option value="dashscope">阿里百炼 DashScope</a-select-option>
                <a-select-option value="deepseek">DeepSeek</a-select-option>
                <a-select-option value="ollama">Ollama（本地）</a-select-option>
                <a-select-option value="vllm">vLLM（内网）</a-select-option>
                <a-select-option value="custom">自定义</a-select-option>
              </a-select>
              <div class="form-tip">预设会填入常见 base_url，模型名仍可自由修改。</div>
            </a-form-item>

            <a-form-item label="API Base URL" required>
              <a-input
                v-model:value="form.base_url"
                placeholder="https://api.openai.com/v1"
                class="mono-input"
              />
              <div class="form-tip">
                OpenAI 兼容端点，需以 <code>/v1</code> 结尾。内网可指 vLLM / Ollama。
              </div>
            </a-form-item>

            <a-form-item label="模型" required>
              <a-select
                v-model:value="form.model"
                show-search
                allow-clear
                :options="modelOptions"
                placeholder="选择或输入模型名"
                mode="combobox"
                @change="onModelChange"
              />
              <div class="form-tip">
                也可直接输入任意模型 ID，例如 <code>gpt-4o-mini</code>、<code>qwen2.5:14b</code>。
              </div>
            </a-form-item>

            <a-form-item label="API Key" :required="form.enabled">
              <a-input-password
                v-model:value="form.api_key"
                :placeholder="keyPlaceholder"
                autocomplete="new-password"
              />
              <div class="form-tip">
                仅保存在服务端 <code>data/ai-settings.json</code>，不会写入 yaml，也不会回传明文。
                留空表示沿用已保存密钥。
              </div>
            </a-form-item>
          </a-form>
        </div>
      </section>

      <!-- 高级参数 -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">高级参数</span>
        </div>
        <div class="card-body is-padded">
          <a-form layout="vertical">
            <a-form-item label="Temperature">
              <a-slider
                v-model:value="form.temperature"
                :min="0"
                :max="2"
                :step="0.1"
                :tooltip-formatter="(v: number) => v.toFixed(1)"
              />
              <div class="form-tip">越低越稳定。默认 0.3。</div>
            </a-form-item>

            <a-form-item label="Max Tokens">
              <a-input-number
                v-model:value="form.max_tokens"
                :min="256"
                :max="32768"
                :step="256"
                style="width: 100%"
              />
            </a-form-item>

            <a-form-item label="超时（秒）">
              <a-input-number
                v-model:value="form.timeout_seconds"
                :min="10"
                :max="600"
                :step="10"
                style="width: 100%"
              />
            </a-form-item>

            <a-form-item label="最大并发会话">
              <a-input-number
                v-model:value="form.max_concurrent_chats"
                :min="1"
                :max="16"
                style="width: 100%"
              />
            </a-form-item>

            <a-alert
              v-if="testResult"
              :type="testResult.ok ? 'success' : 'error'"
              show-icon
              :message="testResult.ok ? '连接正常' : '连接失败'"
              :description="testResult.message"
              class="test-result"
            />
          </a-form>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { ApiOutlined, SaveOutlined } from '@ant-design/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { aiConfigApi, type AIConfig, type AITestResult } from '@/api/aiConfig'
import { useAIChat } from '@/composables/useAIChat'

defineOptions({ name: 'AISettings' })

type Preset = 'openai' | 'dashscope' | 'deepseek' | 'ollama' | 'vllm' | 'custom'

const preset = ref<Preset>('custom')
const saving = ref(false)
const testing = ref(false)
const testResult = ref<AITestResult | null>(null)

const form = reactive({
  enabled: false,
  base_url: '',
  model: '',
  api_key: '',
  temperature: 0.3,
  max_tokens: 2048,
  timeout_seconds: 60,
  max_concurrent_chats: 4,
  has_api_key: false,
  api_key_masked: '',
})

const PRESETS: Record<Preset, { base_url: string; models: string[] }> = {
  openai: {
    base_url: 'https://api.openai.com/v1',
    models: ['gpt-4o-mini', 'gpt-4o', 'gpt-4.1-mini', 'o4-mini'],
  },
  dashscope: {
    base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    models: ['qwen-plus', 'qwen-max', 'qwen-turbo', 'qwen2.5-72b-instruct'],
  },
  deepseek: {
    base_url: 'https://api.deepseek.com/v1',
    models: ['deepseek-chat', 'deepseek-reasoner'],
  },
  ollama: {
    base_url: 'http://127.0.0.1:11434/v1',
    models: ['qwen2.5:14b', 'qwen2.5:7b', 'llama3.1:8b', 'deepseek-r1:14b'],
  },
  vllm: {
    base_url: 'http://127.0.0.1:8000/v1',
    models: ['Qwen/Qwen2.5-14B-Instruct', 'meta-llama/Llama-3.1-8B-Instruct'],
  },
  custom: {
    base_url: '',
    models: [],
  },
}

const modelOptions = computed(() => {
  const list = PRESETS[preset.value]?.models ?? []
  return list.map((m) => ({ label: m, value: m }))
})

const keyPlaceholder = computed(() => {
  if (form.has_api_key && form.api_key_masked) {
    return `已保存（${form.api_key_masked}），留空则不修改`
  }
  return 'sk-... 或本地端点可留空'
})

function applyPreset(p: Preset) {
  const conf = PRESETS[p]
  if (conf?.base_url) form.base_url = conf.base_url
  if (conf?.models.length && !conf.models.includes(form.model)) {
    form.model = conf.models[0]
  }
}

function detectPreset(url: string): Preset {
  if (!url) return 'custom'
  if (url.includes('openai.com')) return 'openai'
  if (url.includes('dashscope')) return 'dashscope'
  if (url.includes('deepseek')) return 'deepseek'
  if (url.includes('11434') || url.includes('ollama')) return 'ollama'
  if (url.includes(':8000') || url.includes('vllm')) return 'vllm'
  return 'custom'
}

function onModelChange() {
  /* combobox 允许自由输入 */
}

function applyConfig(cfg: AIConfig) {
  form.enabled = cfg.enabled
  form.base_url = cfg.base_url || ''
  form.model = cfg.model || ''
  form.temperature = cfg.temperature || 0.3
  form.max_tokens = cfg.max_tokens || 2048
  form.timeout_seconds = cfg.timeout_seconds || 60
  form.max_concurrent_chats = cfg.max_concurrent_chats || 4
  form.has_api_key = !!cfg.has_api_key
  form.api_key_masked = cfg.api_key_masked || ''
  form.api_key = ''
  preset.value = detectPreset(form.base_url)
}

async function load() {
  try {
    applyConfig(await aiConfigApi.get())
  } catch {
    // 未启用/旧后端：给空表单，仍可填写
  }
}

async function handleSave() {
  saving.value = true
  try {
    const cfg = await aiConfigApi.update({
      enabled: form.enabled,
      base_url: form.base_url,
      model: form.model,
      temperature: form.temperature,
      max_tokens: form.max_tokens,
      timeout_seconds: form.timeout_seconds,
      max_concurrent_chats: form.max_concurrent_chats,
      api_key: form.api_key || undefined,
    })
    applyConfig(cfg)
    message.success('已保存并生效')
    // 刷新助手入口可用性
    await useAIChat().loadStatus()
  } catch (e) {
    message.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  if (!form.base_url) {
    message.warning('请先填写 API Base URL')
    return
  }
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await aiConfigApi.test({
      base_url: form.base_url,
      model: form.model,
      api_key: form.api_key || undefined,
    })
    if (testResult.value.ok) message.success(testResult.value.message)
    else message.error(testResult.value.message)
  } catch (e) {
    message.error((e as Error).message || '测试失败')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.status-card {
  margin-bottom: $spacing-md;
}

.status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 32px;
}

.status-item {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.status-label {
  font-size: $fs-caption;
  color: $text-tertiary;
  font-weight: 500;
  flex-shrink: 0;
}

.status-value {
  font-size: $fs-body;
  color: $text-primary;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 420px;

  &.mono {
    font-family: $font-mono;
    font-size: 12px;
  }
}

.config-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
  gap: $spacing-md;
  align-items: start;
}

.mono-input {
  font-family: $font-mono;
  font-size: 12px;
}

.test-result {
  margin-top: 8px;
}

@media (max-width: 960px) {
  .config-grid {
    grid-template-columns: 1fr;
  }
}
</style>
