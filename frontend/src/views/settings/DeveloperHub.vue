<template>
  <div class="page-container">
    <PageHeader
      title="CLI / Agent 入口"
      subtitle="命令行客户端、AI Agent Skills 与 API 对照——脚本化运维与外部 Agent 接入"
    >
      <template #actions>
        <a-button :loading="probing" @click="probe">
          <template #icon><ReloadOutlined /></template>
          检测连通
        </a-button>
        <a-button @click="openDocs">
          <template #icon><BookOutlined /></template>
          API 文档
        </a-button>
        <a-button type="primary" @click="copyEnv">
          <template #icon><CopyOutlined /></template>
          复制环境变量
        </a-button>
      </template>
    </PageHeader>

    <!-- 概览条 -->
    <div class="content-card status-card">
      <div class="card-header">
        <span class="card-title">接入概览</span>
        <div class="header-tags">
          <a-tag :color="reachable ? 'green' : 'red'">
            {{ reachable ? '服务可达' : '服务不可达' }}
          </a-tag>
          <a-tag color="blue">Agent-Native</a-tag>
        </div>
      </div>
      <div class="card-body is-padded">
        <div class="status-row">
          <div class="status-item">
            <span class="status-label">服务地址</span>
            <span class="status-value mono">{{ baseUrl }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">版本</span>
            <span class="status-value mono">{{ sysStatus?.version || '—' }}</span>
          </div>
          <div class="status-item">
            <span class="status-label">鉴权</span>
            <span class="status-value">X-API-Key / <code>GITFERRY_TOKEN</code></span>
          </div>
          <div class="status-item">
            <span class="status-label">输出契约</span>
            <span class="status-value mono">{`{ok, data, error, meta}`}</span>
          </div>
          <div class="status-item">
            <span class="status-label">当前会话 Key</span>
            <span class="status-value mono">{{ keyHint }}</span>
          </div>
        </div>
        <a-alert
          v-if="probeError"
          type="warning"
          show-icon
          :message="probeError"
          style="margin-top: 12px"
        />
      </div>
    </div>

    <div class="dev-grid">
      <!-- 1. 安装 CLI -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">1. 安装 gitferry CLI</span>
          <a-tag>{{ installCmds.length }} 种方式</a-tag>
        </div>
        <div class="card-body is-padded">
          <p class="section-desc">
            三选一。npm 会自动下载对应平台二进制，并附带 <code>gitferry-install-skills</code>。
          </p>
          <div class="cmd-block" v-for="c in installCmds" :key="c.label">
            <div class="cmd-label">{{ c.label }}</div>
            <div class="cmd-line">
              <code>{{ c.cmd }}</code>
              <a-button size="small" type="text" @click="copy(c.cmd)">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>
        </div>
      </section>

      <!-- 2. 配置认证 -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">2. 配置认证</span>
        </div>
        <div class="card-body is-padded">
          <p class="section-desc">
            使用登录时的同一把 API Key（服务端 <code>GIT_SYNC_API_KEY</code> / 界面登录密钥）。
            密钥只放环境变量或本地配置，不要提交进脚本仓库。
          </p>

          <div class="cmd-block">
            <div class="cmd-label">环境变量（推荐 / CI）</div>
            <div class="cmd-line">
              <code>{{ envSnippet }}</code>
              <a-button size="small" type="text" @click="copy(envSnippet)">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>

          <div class="cmd-block">
            <div class="cmd-label">或写入本地配置（0600）</div>
            <div class="cmd-line">
              <code>{{ loginSnippet }}</code>
              <a-button size="small" type="text" @click="copy(loginSnippet)">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>

          <div class="cmd-block">
            <div class="cmd-label">验证连通</div>
            <div class="cmd-line">
              <code>gitferry auth status --format json</code>
              <a-button size="small" type="text" @click="copy('gitferry auth status --format json')">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>
        </div>
      </section>

      <!-- 3. Agent Skills -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">3. 安装 Agent Skills</span>
          <a-tag>{{ skills.length }} 个 Skill</a-tag>
        </div>
        <div class="card-body is-padded">
          <p class="section-desc">
            让 Claude Code / MiMo / Cursor 等外部 Agent 直接运维 GitFerry：查任务、看失败、诊断、受控重试。
          </p>

          <div class="cmd-block">
            <div class="cmd-label">仓库内一键安装</div>
            <div class="cmd-line">
              <code>make install-skills</code>
              <a-button size="small" type="text" @click="copy('make install-skills')">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>

          <div class="cmd-block">
            <div class="cmd-label">npm 安装后</div>
            <div class="cmd-line">
              <code>gitferry-install-skills</code>
              <a-button size="small" type="text" @click="copy('gitferry-install-skills')">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>

          <a-divider style="margin: 12px 0" />
          <div class="skill-grid">
            <div v-for="s in skills" :key="s.name" class="skill-item">
              <div class="skill-name">{{ s.name }}</div>
              <div class="skill-desc">{{ s.desc }}</div>
            </div>
          </div>
        </div>
      </section>

      <!-- 4. 常用命令 -->
      <section class="content-card">
        <div class="card-header">
          <span class="card-title">4. 常用命令速查</span>
          <a-radio-group v-model:value="cmdTab" size="small" button-style="solid">
            <a-radio-button value="task">任务</a-radio-button>
            <a-radio-button value="history">历史</a-radio-button>
            <a-radio-button value="ops">运维</a-radio-button>
            <a-radio-button value="api">Raw API</a-radio-button>
          </a-radio-group>
        </div>
        <div class="card-body is-padded">
          <div class="cmd-block" v-for="c in currentCmds" :key="c.cmd">
            <div class="cmd-label">
              {{ c.label }}
              <a-tag v-if="c.danger" color="orange" style="margin-left: 6px">危险</a-tag>
            </div>
            <div class="cmd-line">
              <code>{{ c.cmd }}</code>
              <a-button size="small" type="text" @click="copy(c.cmd)">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </div>
          </div>
          <a-alert
            type="info"
            show-icon
            message="危险操作默认拒绝并返回 ok=false；仅脚本/自动化场景显式加 --yes。"
            style="margin-top: 8px"
          />
        </div>
      </section>
    </div>

    <!-- 安全与文档 -->
    <section class="content-card links-card">
      <div class="card-header">
        <span class="card-title">安全约定与文档</span>
      </div>
      <div class="card-body is-padded">
        <a-descriptions :column="2" size="small">
          <a-descriptions-item label="密钥存放">
            <code>GITFERRY_TOKEN</code> 环境变量或 <code>~/.config/gitferry/config.yaml</code>（0600）
          </a-descriptions-item>
          <a-descriptions-item label="危险操作">
            默认需确认；仅脚本/自动化加 <code>--yes</code>
          </a-descriptions-item>
          <a-descriptions-item label="输出解析">
            先看 <code>ok</code>；失败读 <code>error.suggestion</code>
          </a-descriptions-item>
          <a-descriptions-item label="完整 API 面">
            <a @click="openDocs">/swagger/</a> · Raw 层
            <code>gitferry api GET /api/v1/...</code>
          </a-descriptions-item>
        </a-descriptions>
        <div class="link-row">
          <a href="https://github.com/yi-nology/git-ferry" target="_blank" rel="noopener">GitHub 仓库</a>
          <a href="https://github.com/yi-nology/git-ferry/blob/main/README.zh-CN.md" target="_blank" rel="noopener">中文 README</a>
          <a href="https://github.com/yi-nology/git-ferry/blob/main/CONTRIBUTING.zh-CN.md" target="_blank" rel="noopener">贡献指南</a>
          <a href="https://github.com/yi-nology/git-ferry/blob/main/SECURITY.md" target="_blank" rel="noopener">安全披露</a>
          <a href="https://github.com/yi-nology/git-ferry/tree/main/skills" target="_blank" rel="noopener">Skills 源码</a>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { BookOutlined, CopyOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import { copyToClipboard } from '@/utils'
import { systemApi, ApiError } from '@/api'
import type { SystemStatusData } from '@/types/api'
import { useAuthStore } from '@/stores/auth'

const baseUrl = window.location.origin
const auth = useAuthStore()

const cmdTab = ref<'task' | 'history' | 'ops' | 'api'>('task')
const probing = ref(false)
const reachable = ref(false)
const probeError = ref('')
const sysStatus = ref<SystemStatusData | null>(null)

const keyHint = computed(() => (auth.getApiKey() ? '已配置（不回显）' : '未配置'))

const envSnippet = `export GITFERRY_BASE_URL="${baseUrl}"
export GITFERRY_TOKEN="<YOUR_API_KEY>"`
const loginSnippet = `gitferry auth login --base-url ${baseUrl} --token <YOUR_API_KEY>`

const installCmds = [
  { label: 'npm（推荐）', cmd: 'npm install -g gitferry-cli' },
  { label: '源码构建', cmd: 'make build-cli && sudo cp output/gitferry /usr/local/bin/' },
  { label: 'Release 二进制', cmd: 'curl -fsSL https://github.com/yi-nology/git-ferry/releases/latest/download/gitferry.tgz | tar -xz' },
]

const skills = [
  { name: 'gitferry-shared', desc: '认证、Envelope、安全规则（必读）' },
  { name: 'gitferry-repo', desc: '仓库列表 / 详情 / 分支 / 连通测试' },
  { name: 'gitferry-task', desc: '同步任务 CRUD、手动触发、预览' },
  { name: 'gitferry-history', desc: '执行历史、失败诊断、重试' },
  { name: 'gitferry-ops', desc: '健康、盘点、RPO、漂移、审计、灾备' },
  { name: 'gitferry-workflow', desc: '失败排查 / 平台接入 / 灾备演练工作流' },
]

type Cmd = { label: string; cmd: string; danger?: boolean }

const cmdGroups: Record<string, Cmd[]> = {
  task: [
    { label: '任务列表', cmd: 'gitferry task +list --format json' },
    { label: '任务详情', cmd: 'gitferry task +info --key <TASK_KEY> --format json' },
    { label: '创建任务', cmd: 'gitferry task +create --name <NAME> --source-repo <SRC> --target-repo <DST> --cron "0 2 * * *"', danger: true },
    { label: '立即同步', cmd: 'gitferry task +run --key <TASK_KEY> --yes', danger: true },
    { label: '同步预览', cmd: 'gitferry task +preview --source-repo <SRC> --target-repo <DST> --format json' },
  ],
  history: [
    { label: '最近执行', cmd: 'gitferry history +list --task <TASK_KEY> --limit 10 --format json' },
    { label: '单次详情', cmd: 'gitferry history +detail --task <TASK_KEY> --run-id <RUN_ID> --format json' },
    { label: '失败诊断', cmd: 'gitferry history +diagnose --run-id <RUN_ID> --format json' },
    { label: '重试执行', cmd: 'gitferry history +retry --run-id <RUN_ID> --yes', danger: true },
  ],
  ops: [
    { label: '系统概览', cmd: 'gitferry ops +overview --format json' },
    { label: '健康评分', cmd: 'gitferry ops +health --format json' },
    { label: '资产盘点', cmd: 'gitferry ops +inventory --format json' },
    { label: '漂移检测', cmd: 'gitferry ops +drift --format json' },
    { label: '批量重试', cmd: 'gitferry ops +retry-batch --limit 10 --yes', danger: true },
  ],
  api: [
    { label: '系统状态', cmd: 'gitferry api GET /api/v1/system/status --format json' },
    { label: '仓库列表', cmd: 'gitferry api GET /api/v1/repos --query page=1 --format json' },
    { label: '触发同步', cmd: `gitferry api POST '/api/v1/sync/task/run?key=<TASK_KEY>'`, danger: true },
    { label: '任意端点', cmd: 'gitferry api GET /api/v1/ops/health-score --format json' },
  ],
}

const currentCmds = computed(() => cmdGroups[cmdTab.value] ?? [])

async function copy(text: string) {
  try {
    await copyToClipboard(text)
    message.success('已复制')
  } catch {
    message.error('复制失败，请手动选择')
  }
}

function copyEnv() {
  void copy(envSnippet)
}

function openDocs() {
  window.open(`${baseUrl}/swagger/`, '_blank')
}

async function probe() {
  probing.value = true
  probeError.value = ''
  try {
    sysStatus.value = await systemApi.status()
    reachable.value = true
    message.success('服务可达')
  } catch (e) {
    reachable.value = false
    sysStatus.value = null
    probeError.value = e instanceof ApiError ? e.message : '无法连接服务，请检查地址与网络'
  } finally {
    probing.value = false
  }
}

onMounted(probe)
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.status-card {
  margin-bottom: $spacing-md;
}

.header-tags {
  display: flex;
  gap: 8px;
  align-items: center;
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

  &.mono {
    font-family: $font-mono;
    font-size: 12px;
  }

  code {
    font-family: $font-mono;
    font-size: 12px;
    background: $bg-canvas;
    padding: 1px 6px;
    border-radius: 4px;
  }
}

.dev-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: $spacing-md;
  align-items: start;
  margin-bottom: $spacing-md;
}

.section-desc {
  font-size: $fs-body;
  color: $text-secondary;
  margin: 0 0 12px 0;
  line-height: 1.6;

  code {
    font-family: $font-mono;
    font-size: 12px;
    background: $bg-canvas;
    padding: 1px 6px;
    border-radius: 4px;
  }
}

.cmd-block {
  margin-bottom: 10px;

  &:last-child {
    margin-bottom: 0;
  }
}

.cmd-label {
  font-size: $fs-caption;
  color: $text-tertiary;
  font-weight: 500;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
}

.cmd-line {
  display: flex;
  align-items: center;
  gap: 4px;
  background: $bg-canvas;
  border: 1px solid $border-muted;
  border-radius: $radius-md;
  padding: 6px 8px 6px 12px;

  code {
    flex: 1;
    min-width: 0;
    font-family: $font-mono;
    font-size: 12px;
    color: $text-primary;
    white-space: nowrap;
    overflow-x: auto;
    line-height: 1.5;
  }
}

.skill-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}

.skill-item {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px solid $border-muted;

  &:last-child {
    border-bottom: none;
  }
}

.skill-name {
  font-family: $font-mono;
  font-size: 12px;
  font-weight: 600;
  color: $primary;
  flex-shrink: 0;
  min-width: 130px;
}

.skill-desc {
  font-size: $fs-caption;
  color: $text-secondary;
}

.link-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-top: 12px;
  font-size: $fs-body;
}

@media (max-width: 960px) {
  .dev-grid {
    grid-template-columns: 1fr;
  }

  .skill-name {
    min-width: 110px;
  }
}
</style>
