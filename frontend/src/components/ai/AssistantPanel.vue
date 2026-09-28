<template>
  <Teleport to="body">
    <a-float-button
      v-if="enabled !== false"
      class="ai-fab"
      tooltip="AI 运维助手"
      @click="toggle"
    >
      <template #icon><RobotOutlined /></template>
    </a-float-button>

    <a-drawer
      v-model:open="open"
      placement="right"
      :width="440"
      :closable="false"
      :body-style="{ padding: 0, display: 'flex', flexDirection: 'column', height: '100%' }"
      class="ai-drawer"
    >
      <!-- 头部 -->
      <header class="ai-header">
        <div class="ai-header-left">
          <span class="ai-avatar"><RobotOutlined /></span>
          <div class="ai-header-text">
            <div class="ai-title">AI 运维助手</div>
            <div class="ai-sub">
              <span v-if="model" class="model-chip">{{ model }}</span>
              <span v-else class="model-chip muted">只读查询 · 危险操作需确认</span>
            </div>
          </div>
        </div>
        <div class="ai-header-actions">
          <a-tooltip title="清空会话">
            <a-button type="text" class="icon-btn" @click="reset">
              <DeleteOutlined />
            </a-button>
          </a-tooltip>
          <a-tooltip title="关闭">
            <a-button type="text" class="icon-btn" @click="toggle">
              <CloseOutlined />
            </a-button>
          </a-tooltip>
        </div>
      </header>

      <!-- 未启用 -->
      <div v-if="enabled === false" class="ai-disabled">
        <a-alert
          type="info"
          show-icon
          message="AI 助手未启用"
          description="在服务端打开 ai 配置段并设置 GIT_SYNC_AI_API_KEY 后重启即可使用。"
        />
      </div>

      <!-- 消息区 -->
      <div v-else ref="listRef" class="msg-list">
        <div v-if="!messages.length" class="empty-state">
          <div class="empty-icon"><RobotOutlined /></div>
          <div class="empty-title">需要查什么？</div>
          <p class="empty-desc">只读查询直接执行；立即同步等危险操作会在界面弹确认。</p>
          <div class="suggest-list">
            <button
              v-for="s in suggestions"
              :key="s"
              type="button"
              class="suggest-chip"
              @click="useSuggestion(s)"
            >
              {{ s }}
            </button>
          </div>
        </div>

        <template v-for="(m, i) in messages" :key="i">
          <!-- 工具活动 -->
          <div v-if="m.role === 'tool'" class="tool-row">
            <span class="tool-dot" />
            <span class="tool-name">{{ m.tool || 'tool' }}</span>
            <span class="tool-text">{{ m.content }}</span>
          </div>

          <!-- 错误 -->
          <div v-else-if="m.role === 'error'" class="msg error">
            <div class="bubble error-bubble">{{ m.content }}</div>
          </div>

          <!-- 对话气泡 -->
          <div v-else :class="['msg', m.role]">
            <div class="bubble">
              {{ m.content }}<span v-if="m.streaming" class="cursor">▍</span>
            </div>
          </div>
        </template>

        <!-- 危险操作确认 -->
        <div v-if="pendingConfirm" class="confirm-card">
          <div class="confirm-head">
            <WarningOutlined class="confirm-icon" />
            <span>确认执行</span>
            <code class="confirm-tool">{{ pendingConfirm.tool }}</code>
          </div>
          <pre v-if="prettyArgs" class="args">{{ prettyArgs }}</pre>
          <div class="confirm-actions">
            <a-button size="small" @click="deny">取消</a-button>
            <a-button danger size="small" :loading="streaming" @click="confirm">
              确认执行
            </a-button>
          </div>
        </div>
      </div>

      <!-- 输入区 -->
      <footer v-if="enabled !== false" class="composer">
        <div class="composer-box">
          <a-textarea
            v-model:value="draft"
            :auto-size="{ minRows: 1, maxRows: 4 }"
            :disabled="streaming"
            :bordered="false"
            placeholder="询问仓库、任务、同步状态…（Enter 发送）"
            class="composer-input"
            @keydown.enter.exact.prevent="submit"
          />
          <div class="composer-bar">
            <span class="composer-hint">Shift+Enter 换行</span>
            <a-button
              v-if="streaming"
              size="small"
              @click="cancelStream"
            >
              停止
            </a-button>
            <a-button
              v-else
              type="primary"
              size="small"
              :disabled="!draft.trim()"
              @click="submit"
            >
              发送
            </a-button>
          </div>
        </div>
      </footer>
    </a-drawer>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  RobotOutlined,
  DeleteOutlined,
  CloseOutlined,
  WarningOutlined,
} from '@ant-design/icons-vue'
import { useAIChat } from '@/composables/useAIChat'

const {
  open,
  enabled,
  model,
  messages,
  streaming,
  pendingConfirm,
  send,
  confirm,
  deny,
  cancelStream,
  reset,
  toggle,
} = useAIChat()

const draft = ref('')
const listRef = ref<HTMLElement>()

const suggestions = [
  '有哪些仓库？',
  '查看同步任务',
  '最近执行记录怎么样？',
  '系统健康评分',
]

const prettyArgs = computed(() => {
  if (!pendingConfirm.value?.args) return ''
  try {
    return JSON.stringify(JSON.parse(pendingConfirm.value.args), null, 2)
  } catch {
    return pendingConfirm.value.args
  }
})

function useSuggestion(text: string) {
  if (streaming.value) return
  void send(text)
}

function submit() {
  const text = draft.value.trim()
  if (!text || streaming.value) return
  draft.value = ''
  void send(text)
}

// 新消息与流式增量都要滚动:流式时最后一条 content 增长但列表长度不变,
// 只 watch length 会导致输出期间停在上次位置
watch(
  () => [messages.value.length, messages.value[messages.value.length - 1]?.content] as const,
  async () => {
    await nextTick()
    listRef.value?.scrollTo({ top: listRef.value.scrollHeight })
  },
)
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.ai-fab {
  right: 24px;
  bottom: 24px;
}

.ai-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid $border-muted;
  background: $bg-primary;
  flex-shrink: 0;
}

.ai-header-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ai-avatar {
  width: 32px;
  height: 32px;
  border-radius: $radius-md;
  background: $primary-soft;
  color: $primary;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}

.ai-header-text {
  min-width: 0;
}

.ai-title {
  font-size: $fs-md;
  font-weight: 600;
  color: $text-primary;
  line-height: 1.2;
}

.ai-sub {
  margin-top: 2px;
}

.model-chip {
  display: inline-block;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: $primary;
  background: $primary-soft;
  border: 1px solid $primary-border;
  border-radius: 999px;
  padding: 0 8px;
  line-height: 18px;

  &.muted {
    color: $text-tertiary;
    background: $bg-subtle;
    border-color: $border-light;
  }
}

.ai-header-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.icon-btn {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: $text-secondary;
  border-radius: $radius-md;

  &:hover {
    color: $text-primary;
    background: $bg-hover;
  }
}

.ai-disabled {
  padding: 16px;
}

.msg-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px 14px;
  background: $bg-canvas;
  min-height: 0;
}

/* 空态 */
.empty-state {
  margin: auto 0;
  text-align: center;
  padding: 12px 8px 24px;
}

.empty-icon {
  width: 48px;
  height: 48px;
  margin: 0 auto 12px;
  border-radius: $radius-xl;
  background: $primary-soft;
  color: $primary;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
}

.empty-title {
  font-size: $fs-lg;
  font-weight: 600;
  color: $text-primary;
  margin-bottom: 6px;
}

.empty-desc {
  font-size: $fs-body;
  color: $text-secondary;
  margin: 0 0 16px;
  line-height: 1.6;
}

.suggest-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: stretch;
}

.suggest-chip {
  appearance: none;
  border: 1px solid $border-light;
  background: $bg-primary;
  color: $text-primary;
  border-radius: $radius-lg;
  padding: 10px 12px;
  font-size: $fs-body;
  text-align: left;
  cursor: pointer;
  transition: border-color $duration-fast $ease, background $duration-fast $ease;

  &:hover {
    border-color: $primary-border;
    background: $primary-soft;
    color: $primary;
  }
}

/* 消息 */
.msg {
  display: flex;

  &.user {
    justify-content: flex-end;
  }

  &.assistant {
    justify-content: flex-start;
  }

  .bubble {
    max-width: 88%;
    padding: 8px 12px;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: $fs-md;
    line-height: 1.6;
  }

  &.user .bubble {
    background: $primary;
    color: #fff;
    border-radius: 10px 10px 2px 10px;
  }

  &.assistant .bubble {
    background: $bg-primary;
    border: 1px solid $border-light;
    color: $text-primary;
    border-radius: 10px 10px 10px 2px;
  }

  &.error .bubble {
    background: $error-soft;
    border: 1px solid $error-border;
    color: $error;
    border-radius: $radius-lg;
    max-width: 100%;
  }
}

.cursor {
  animation: blink 1s step-start infinite;
  margin-left: 1px;
}

@keyframes blink {
  50% {
    opacity: 0;
  }
}

/* 工具活动 */
.tool-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: $fs-caption;
  color: $text-tertiary;
  padding: 2px 4px;
  min-width: 0;
}

.tool-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: $text-tertiary;
  flex-shrink: 0;
}

.tool-name {
  font-family: $font-mono;
  font-size: 11px;
  color: $text-secondary;
  background: $bg-subtle;
  border: 1px solid $border-light;
  border-radius: $radius-sm;
  padding: 0 5px;
  line-height: 18px;
  flex-shrink: 0;
}

.tool-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 确认卡 */
.confirm-card {
  background: $warning-soft;
  border: 1px solid $warning-border;
  border-radius: $radius-lg;
  padding: 12px;
}

.confirm-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: $fs-md;
  font-weight: 600;
  color: $warning;
  margin-bottom: 8px;
}

.confirm-icon {
  font-size: 14px;
}

.confirm-tool {
  font-family: $font-mono;
  font-size: 12px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid $warning-border;
  border-radius: $radius-sm;
  padding: 0 6px;
  color: $text-primary;
  margin-left: 2px;
}

.args {
  font-family: $font-mono;
  font-size: 11px;
  line-height: 1.5;
  margin: 0 0 10px;
  padding: 8px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid $warning-border;
  border-radius: $radius-md;
  white-space: pre-wrap;
  word-break: break-all;
  color: $text-primary;
  max-height: 160px;
  overflow: auto;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

/* 输入 */
.composer {
  padding: 12px 14px 14px;
  border-top: 1px solid $border-muted;
  background: $bg-primary;
  flex-shrink: 0;
}

.composer-box {
  border: 1px solid $border;
  border-radius: $radius-lg;
  background: $bg-primary;
  padding: 8px 10px 6px;

  &:focus-within {
    border-color: $primary;
    box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.12);
  }
}

.composer-input {
  padding: 0;
  resize: none;
  font-size: $fs-md;

  :deep(textarea) {
    padding: 0;
  }
}

.composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 6px;
  gap: 8px;
}

.composer-hint {
  font-size: 11px;
  color: $text-tertiary;
}
</style>
