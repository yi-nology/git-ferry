<template>
  <Teleport to="body">
    <a-float-button
      v-if="enabled !== false"
      tooltip="AI 助手"
      class="ai-fab"
      @click="toggle"
    >
      <template #icon><RobotOutlined /></template>
    </a-float-button>

    <a-drawer
      v-model:open="open"
      title="AI 助手"
      placement="right"
      :width="420"
      :body-style="{ display: 'flex', flexDirection: 'column', gap: '12px', padding: '16px' }"
    >
      <template #extra>
        <a-space>
          <a-tag v-if="model" color="blue">{{ model }}</a-tag>
          <a-button size="small" type="text" @click="reset">清空</a-button>
        </a-space>
      </template>

      <a-alert
        v-if="enabled === false"
        type="info"
        show-icon
        message="AI 助手未启用"
        description="请在服务端配置 ai 配置段与 GIT_SYNC_AI_API_KEY 后重启服务。"
      />

      <div ref="listRef" class="msg-list">
        <div v-if="!messages.length && enabled !== false" class="empty-tip">
          试试:『有哪些仓库』『查看任务列表』『帮我同步 demo-task』
        </div>

        <div v-for="(m, i) in messages" :key="i" :class="['msg', m.role]">
          <span v-if="m.role === 'tool'" class="tool-line">🔧 {{ m.content }}</span>
          <span v-else class="bubble">{{ m.content }}<span v-if="m.streaming" class="cursor">▍</span></span>
        </div>

        <a-alert
          v-if="pendingConfirm"
          type="warning"
          show-icon
          class="confirm-card"
        >
          <template #message>确认执行 {{ pendingConfirm.tool }}</template>
          <template #description>
            <pre class="args">{{ prettyArgs }}</pre>
            <a-space>
              <a-button type="primary" danger size="small" :loading="streaming" @click="confirm">
                确认执行
              </a-button>
              <a-button size="small" @click="deny">取消</a-button>
            </a-space>
          </template>
        </a-alert>

        <a-alert v-if="error" type="error" show-icon :message="error" closable @close="error = ''" />
      </div>

      <div class="input-row">
        <a-textarea
          v-model:value="draft"
          :auto-size="{ minRows: 1, maxRows: 4 }"
          :disabled="streaming"
          placeholder="询问同步状态、仓库、任务…(Enter 发送,Shift+Enter 换行)"
          @keydown.enter.exact.prevent="submit"
        />
        <a-button v-if="streaming" @click="cancelStream">停止</a-button>
        <a-button v-else type="primary" :disabled="!draft.trim()" @click="submit">发送</a-button>
      </div>
    </a-drawer>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { RobotOutlined } from '@ant-design/icons-vue'
import { useAIChat } from '@/composables/useAIChat'

const {
  open,
  enabled,
  model,
  messages,
  streaming,
  error,
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

const prettyArgs = computed(() => {
  if (!pendingConfirm.value?.args) return ''
  try {
    return JSON.stringify(JSON.parse(pendingConfirm.value.args), null, 2)
  } catch {
    return pendingConfirm.value.args
  }
})

function submit() {
  const text = draft.value.trim()
  if (!text || streaming.value) return
  draft.value = ''
  void send(text)
}

watch(
  () => messages.value.length,
  async () => {
    await nextTick()
    listRef.value?.scrollTo({ top: listRef.value.scrollHeight })
  },
)
</script>

<style scoped>
.ai-fab {
  right: 24px;
  bottom: 24px;
}

.msg-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 4px;
}

.empty-tip {
  color: var(--ant-color-text-tertiary, #999);
  font-size: 13px;
  text-align: center;
  margin-top: 24px;
}

.msg {
  display: flex;
}

.msg.user {
  justify-content: flex-end;
}

.msg.user .bubble {
  background: #1677ff;
  color: #fff;
  border-radius: 8px 8px 2px 8px;
}

.msg.assistant {
  justify-content: flex-start;
}

.msg.assistant .bubble {
  background: rgba(0, 0, 0, 0.06);
  color: inherit;
  border-radius: 8px 8px 8px 2px;
}

.msg.assistant .bubble,
.msg.user .bubble {
  max-width: 85%;
  padding: 8px 12px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 14px;
  line-height: 1.6;
}

.cursor {
  animation: blink 1s step-start infinite;
}

@keyframes blink {
  50% {
    opacity: 0;
  }
}

.tool-line {
  font-size: 12px;
  color: #999;
  font-style: italic;
}

.confirm-card {
  margin-top: 4px;
}

.args {
  font-size: 12px;
  margin: 0 0 8px;
  white-space: pre-wrap;
  word-break: break-all;
}

.input-row {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}
</style>
