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
            <span class="status-label">执行后端</span>
            <span class="status-value mono">{{ backendLabel || form.backend || '—' }}</span>
            <a-tag v-if="form.backend !== 'internal'" :color="ready ? 'green' : 'orange'">
              {{ ready ? '就绪' : '未就绪' }}
            </a-tag>
          </div>
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
          <div v-if="form.backend !== 'internal' && !ready && readyMsg" class="status-item">
            <span class="status-label">未就绪原因</span>
            <span class="status-value mono">{{ readyMsg }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">外部 Agent</span>
            <span class="status-value">
              <router-link to="/settings/dev">CLI / Agent Skills →</router-link>
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

            <a-form-item label="执行后端">
              <a-radio-group v-model:value="form.backend">
                <a-radio-button value="internal">内置助手</a-radio-button>
                <a-radio-button value="acpx">外部 coding agent</a-radio-button>
                <a-radio-button value="stdio">外部 · 原生 ACP</a-radio-button>
              </a-radio-group>
              <div class="form-tip">
                内置助手走 OpenAI 兼容端点；外部后端把整段对话交给外部成熟的
                AI coding 工具（MiniMax Code / Claude Code / Codex / Gemini CLI…），
                通过 ACP 协议驱动，工具在 agent 侧执行。
              </div>
            </a-form-item>

            <template v-if="form.backend === 'internal'">
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
              <div class="model-row">
                <a-select
                  v-model:value="form.model"
                  show-search
                  allow-clear
                  :options="modelOptions"
                  placeholder="选择或输入模型名"
                  mode="combobox"
                  class="model-select"
                  @change="onModelChange"
                />
                <a-button :loading="loadingModels" @click="fetchModels">
                  <template #icon><CloudDownloadOutlined /></template>
                  获取模型
                </a-button>
              </div>
              <div class="form-tip">
                从端点拉取真实模型列表；也可手输
                <code>gpt-4o-mini</code>、<code>qwen2.5:14b</code> 等。
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
            </template>

            <!-- 外部 coding agent -->
            <template v-else>
            <a-form-item label="Agent" required>
              <a-select
                v-model:value="form.external.agent"
                show-search
                allow-clear
                :options="agentOptions"
                placeholder="选择或输入 agent 名"
                mode="combobox"
                class="model-select"
              />
              <div class="form-tip">
                acpx 内置的 agent 名。原生 ACP 后端请改填下方「命令」。
              </div>
            </a-form-item>

            <a-form-item label="命令">
              <a-input
                v-model:value="form.external.command"
                placeholder="mcode acp"
                class="mono-input"
              />
              <div class="form-tip">
                自定义命令。原生 ACP 后端填这个（如 <code>mcode acp</code>）；
                acpx 后端填了会用它覆盖 agent 名（等价 acpx 的 <code>--agent</code>）。
              </div>
            </a-form-item>

            <a-form-item label="工作目录">
              <a-input
                v-model:value="form.external.cwd"
                placeholder="var/ai-workspace"
                class="mono-input"
              />
              <div class="form-tip">
                外部 agent 的工作目录，同时是**沙箱边界**：agent 侧只能访问它下面的文件。
              </div>
            </a-form-item>

            <a-form-item label="授权档">
              <a-select v-model:value="form.external.permission">
                <a-select-option value="ask">每次都问（推荐）</a-select-option>
                <a-select-option value="approve_reads">只读自动放行</a-select-option>
                <a-select-option value="approve_all">全部放行</a-select-option>
                <a-select-option value="deny">全部拒绝</a-select-option>
              </a-select>
              <div class="form-tip">
                外部 agent 请求授权时的默认处置。「每次都问」会在界面上弹授权卡，
                由你逐次决定。
              </div>
            </a-form-item>

            <a-form-item label="挂载本服务 MCP">
              <a-switch v-model:checked="form.external.mcp_enabled" />
              <div class="form-tip">
                开启后，外部 agent 通过 MCP 客户端连回本服务，直接使用
                同步 / 镜像 / 运维等 {{ toolCountText }} 领域工具。
                该 MCP 专用 key 固定为**只读角色**（MCP 路径上危险工具不弹确认）。
              </div>
            </a-form-item>

            <a-form-item v-if="form.external.mcp_enabled" label="MCP Key 环境变量">
              <a-input
                v-model:value="form.external.mcp_api_key_env"
                placeholder="GITFERRY_AI_MCP_API_KEY"
                class="mono-input"
              />
              <div class="form-tip">
                填**环境变量名**，不填密钥本身。服务启动时从该变量取值并注册为只读 key。
              </div>
            </a-form-item>

            <a-form-item v-if="form.external.mcp_enabled" label="MCP 端点">
              <a-input
                v-model:value="form.external.mcp_url"
                placeholder="留空则用本机默认地址"
                class="mono-input"
              />
            </a-form-item>

            <a-form-item label="外部 agent 配置">
              <a-radio-group v-model:value="extMode" size="small">
                <a-radio-button value="default">默认单个</a-radio-button>
                <a-radio-button value="profiles">具名列表</a-radio-button>
              </a-radio-group>
              <div class="form-tip">
                「具名列表」可同时挂多个外部 agent（如 Claude Code + Codex）：
                对话用其中一个，委派时按名字选。
              </div>
            </a-form-item>

            <template v-if="extMode === 'profiles'">
              <a-form-item label="当前对话使用">
                <a-select
                  v-model:value="form.external.current"
                  placeholder="留空=列表第一个"
                  style="width: 100%"
                >
                  <a-select-option v-for="p in form.external.agents" :key="p.name" :value="p.name">
                    {{ p.name }}
                  </a-select-option>
                </a-select>
              </a-form-item>

              <a-form-item label="profile 列表">
                <div class="profile-list">
                  <div v-for="(p, i) in form.external.agents" :key="i" class="profile-item">
                    <div class="profile-head">
                      <span class="profile-title">{{ p.name || '未命名' }}</span>
                      <a-button size="small" type="text" danger @click="removeProfile(i)">
                        移除
                      </a-button>
                    </div>
                    <div class="profile-grid">
                      <a-input v-model:value="p.name" placeholder="名称" size="small" />
                      <a-select v-model:value="p.transport" size="small" style="width: 100px">
                        <a-select-option value="">跟随</a-select-option>
                        <a-select-option value="acpx">acpx</a-select-option>
                        <a-select-option value="stdio">stdio</a-select-option>
                      </a-select>
                      <a-input v-model:value="p.agent" placeholder="agent 名" size="small" />
                      <a-input v-model:value="p.command" placeholder="命令(stdio)" size="small" />
                      <a-input v-model:value="p.cwd" placeholder="工作目录" size="small" />
                      <a-select v-model:value="p.permission" size="small" style="width: 130px">
                        <a-select-option value="ask">每次都问</a-select-option>
                        <a-select-option value="approve_reads">只读放行</a-select-option>
                        <a-select-option value="approve_all">全部放行</a-select-option>
                        <a-select-option value="deny">全部拒绝</a-select-option>
                      </a-select>
                    </div>
                  </div>
                  <a-button size="small" @click="addProfile">
                    <template #icon><PlusOutlined /></template>
                    添加 profile
                  </a-button>
                </div>
              </a-form-item>
            </template>

            <a-form-item v-if="extMode === 'default'" label="额外 MCP 服务器">
              <div class="mcp-list">
                <div v-for="(srv, i) in form.external.mcp_servers" :key="i" class="mcp-item">
                  <div class="mcp-item-head">
                    <span class="mcp-item-title">{{ srv.name || '未命名' }}</span>
                    <a-button size="small" type="text" danger @click="removeMcpServer(i)">
                      移除
                    </a-button>
                  </div>
                  <div class="mcp-item-grid">
                    <a-input v-model:value="srv.name" placeholder="名称" size="small" />
                    <a-select v-model:value="srv.type" size="small" style="width: 110px">
                      <a-select-option value="http">http</a-select-option>
                      <a-select-option value="sse">sse</a-select-option>
                      <a-select-option value="stdio">stdio</a-select-option>
                    </a-select>
                    <a-input
                      v-if="srv.type !== 'stdio'"
                      v-model:value="srv.url"
                      placeholder="https://…/mcp"
                      size="small"
                    />
                    <a-input v-else v-model:value="srv.command" placeholder="/usr/bin/mcp-server" size="small" />
                    <a-input
                      v-model:value="srv.api_key_env"
                      placeholder="认证头环境变量名"
                      size="small"
                    />
                  </div>
                </div>
                <a-button size="small" @click="addMcpServer">
                  <template #icon><PlusOutlined /></template>
                  添加 MCP 服务器
                </a-button>
                <div class="form-tip">
                  额外挂给外部 agent 的 MCP 服务器（不止本服务自己）。认证密钥只填
                  <b>环境变量名</b>，变量里放完整头值（如 <code>Bearer xxxxx</code>）。
                </div>
              </div>
            </a-form-item>
            </template>
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
import {
  ApiOutlined,
  SaveOutlined,
  CloudDownloadOutlined,
  PlusOutlined,
} from '@ant-design/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import {
  aiConfigApi,
  type AIBackend,
  type AIConfig,
  type AIExternalConfig,
  type AITestResult,
  type AIModelItem,
} from '@/api/aiConfig'
import { useAIChat } from '@/composables/useAIChat'

defineOptions({ name: 'AISettings' })

type Preset = 'openai' | 'dashscope' | 'deepseek' | 'ollama' | 'vllm' | 'custom'

const preset = ref<Preset>('custom')
const saving = ref(false)
const testing = ref(false)
const loadingModels = ref(false)
const testResult = ref<AITestResult | null>(null)
const remoteModels = ref<AIModelItem[]>([])

const { backendLabel, ready, readyMsg, loadStatus } = useAIChat()

/** 外部 agent 配置的编辑形态:默认单个 | 具名列表。 */
const extMode = ref<'default' | 'profiles'>('default')

/** acpx 内置 agent（节选）。空值也允许手输。 */
const AGENT_NAMES = [
  'mcode',
  'codex',
  'claude',
  'gemini',
  'opencode',
  'qwen',
  'cursor',
  'copilot',
  'kimi',
  'openclaw',
  'pi',
  'iflow',
  'droid',
  'kilocode',
]

const agentOptions = AGENT_NAMES.map((a) => ({ label: a, value: a }))

/** 领域工具数量（展示用，来自 MCP 能力探测）。 */
const toolCountText = '41'

const emptyExternal = (): AIExternalConfig => ({
  agent: '',
  command: '',
  args: [],
  cwd: 'var/ai-workspace',
  timeout_seconds: 600,
  permission: 'ask',
  mcp_enabled: false,
  mcp_url: '',
  mcp_api_key_env: 'GITFERRY_AI_MCP_API_KEY',
  mcp_servers: [],
  agents: [],
  current: '',
})

function addMcpServer() {
  form.external.mcp_servers.push({
    name: '',
    type: 'http',
    url: '',
    api_key_env: '',
    api_key_header: '',
    command: '',
    args: [],
    env: [],
  })
}

function removeMcpServer(i: number) {
  form.external.mcp_servers.splice(i, 1)
}

function addProfile() {
  form.external.agents.push({
    name: '',
    transport: '',
    agent: '',
    command: '',
    args: [],
    cwd: 'var/ai-workspace',
    timeout_seconds: 600,
    permission: 'ask',
    mcp_enabled: false,
    mcp_url: '',
    mcp_api_key_env: '',
    mcp_servers: [],
  })
}

function removeProfile(i: number) {
  form.external.agents.splice(i, 1)
}

const form = reactive({
  enabled: false,
  backend: 'internal' as AIBackend,
  base_url: '',
  model: '',
  api_key: '',
  temperature: 0.3,
  max_tokens: 2048,
  timeout_seconds: 60,
  max_concurrent_chats: 4,
  has_api_key: false,
  api_key_masked: '',
  external: emptyExternal(),
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
  const presetModels = (PRESETS[preset.value]?.models ?? []).map((m) => ({ label: m, value: m }))
  const remote = remoteModels.value.map((m) => ({
    label: m.owned_by ? `${m.id} · ${m.owned_by}` : m.id,
    value: m.id,
  }))
  // 远端列表优先,预设作补充;按 value 去重
  const seen = new Set<string>()
  const out: { label: string; value: string }[] = []
  for (const opt of [...remote, ...presetModels]) {
    if (seen.has(opt.value)) continue
    seen.add(opt.value)
    out.push(opt)
  }
  return out
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

async function fetchModels() {
  if (!form.base_url) {
    message.warning('请先填写 API Base URL')
    return
  }
  loadingModels.value = true
  try {
    const data = await aiConfigApi.listModels({
      base_url: form.base_url,
      api_key: form.api_key || undefined,
    })
    remoteModels.value = data.models || []
    if (!remoteModels.value.length) {
      message.info('端点未返回模型列表')
    } else {
      message.success(`已获取 ${remoteModels.value.length} 个模型`)
      // 无当前模型时默认选第一个
      if (!form.model && remoteModels.value[0]) {
        form.model = remoteModels.value[0].id
      }
    }
  } catch (e) {
    message.error((e as Error).message || '获取模型失败')
  } finally {
    loadingModels.value = false
  }
}

function applyConfig(cfg: AIConfig) {
  form.enabled = cfg.enabled
  form.backend = (cfg.backend || 'internal') as AIBackend
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
      backend: form.backend,
      base_url: form.base_url,
      model: form.model,
      temperature: form.temperature,
      max_tokens: form.max_tokens,
      timeout_seconds: form.timeout_seconds,
      max_concurrent_chats: form.max_concurrent_chats,
      api_key: form.api_key || undefined,
      external: { ...form.external },
    })
    applyConfig(cfg)
    message.success('已保存并生效')
    // 刷新助手入口可用性与后端标识
    await loadStatus()
  } catch (e) {
    message.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  // **按后端分支**:外部 agent 没有 base_url/model,拿 OpenAI 探测去问它是问错对象。
  if (form.backend === 'internal' && !form.base_url) {
    message.warning('请先填写 API Base URL')
    return
  }
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await aiConfigApi.test({
      backend: form.backend,
      base_url: form.base_url,
      model: form.model,
      api_key: form.api_key || undefined,
      external: { ...form.external },
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

.model-row {
  display: flex;
  width: 100%;
  gap: 8px;
  align-items: center;
}

.model-select {
  flex: 1;
  min-width: 0;
}

.test-result {
  margin-top: 8px;
}

.profile-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.profile-item {
  border: 1px solid $border-light;
  border-radius: $radius-md;
  padding: 10px;
  background: $bg-subtle;
}

.profile-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.profile-title {
  font-size: $fs-caption;
  color: $text-secondary;
  font-family: $font-mono;
}

.profile-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 8px;
}

.mcp-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.mcp-item {
  border: 1px solid $border-light;
  border-radius: $radius-md;
  padding: 10px;
  background: $bg-subtle;
}

.mcp-item-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.mcp-item-title {
  font-size: $fs-caption;
  color: $text-secondary;
  font-family: $font-mono;
}

.mcp-item-grid {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 8px;
}

@media (max-width: 960px) {
  .config-grid {
    grid-template-columns: 1fr;
  }
}
</style>
