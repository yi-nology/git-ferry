<template>
  <a-drawer
    :open="open"
    title="新建镜像通道"
    width="640"
    :mask-closable="false"
    @close="emit('update:open', false)"
  >
    <a-form layout="vertical">
      <a-form-item label="通道名称" required>
        <a-input v-model:value="form.name" placeholder="如:agentkit GitHub 开源镜像" />
      </a-form-item>

      <a-form-item label="通道模式" required>
        <a-radio-group v-model:value="form.mode" :disabled="modeFixed">
          <a-radio-button value="publish">🚀 开源公开发布</a-radio-button>
          <a-radio-button value="backup" title="P1 提供">🗄 仓库备份(P1)</a-radio-button>
        </a-radio-group>
        <p class="hint">
          {{ form.mode === 'publish' ? '改写 module 身份生成快照,公开可 go get;推送前过编译门禁。' : '原样推送 tag 指针到备份远端,不改代码。' }}
        </p>
      </a-form-item>

      <a-form-item label="源仓库" required>
        <a-select
          v-model:value="form.repoKey"
          show-search
          placeholder="选择 internal 源仓库"
          :options="repoOptions"
          :filter-option="filterRepo"
        />
        <p class="hint">创建时会先克隆源仓库读取 go.mod 的 module 身份,作为映射左侧。</p>
      </a-form-item>

      <a-divider>发布目标(可多个,多仓镜像)</a-divider>

      <div v-for="(t, idx) in form.targets" :key="idx" class="target-block">
        <div class="target-head">
          <span>目标 #{{ idx + 1 }}</span>
          <a-button
            v-if="form.targets.length > 1"
            type="text"
            danger
            size="small"
            @click="form.targets.splice(idx, 1)"
          >
            移除
          </a-button>
        </div>
        <a-form-item label="远端名" required>
          <a-input v-model:value="t.remote" placeholder="github(仓库中 remote 的名字)" />
        </a-form-item>
        <a-form-item label="目标仓库 Git URL" required>
          <a-input v-model:value="t.repoUrl" placeholder="https://github.com/org/repo.git 或 ssh://" />
        </a-form-item>
        <a-form-item v-if="form.mode === 'publish'" label="目标 module" required>
          <a-input v-model:value="t.targetModule" placeholder="github.com/org/repo" />
        </a-form-item>
        <a-row :gutter="8">
          <a-col :span="8">
            <a-form-item label="凭据类型">
              <a-select v-model:value="t.credType">
                <a-select-option value="none">本机凭据</a-select-option>
                <a-select-option value="token">Token(Basic)</a-select-option>
                <a-select-option value="ssh_key">SSH 私钥</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="8" v-if="t.credType === 'token'">
            <a-form-item label="用户名">
              <a-input v-model:value="t.username" placeholder="oauth2" />
            </a-form-item>
          </a-col>
          <a-col :span="16" v-if="t.credType !== 'none'">
            <a-form-item :label="t.credType === 'token' ? 'Token' : '私钥 PEM'">
              <a-input-password v-model:value="t.credential" placeholder="仅提交,不回显" />
            </a-form-item>
          </a-col>
        </a-row>
      </div>

      <a-button type="dashed" block @click="addTarget">
        <template #icon><PlusOutlined /></template>
        添加目标
      </a-button>
    </a-form>

    <template #footer>
      <a-space>
        <a-button @click="emit('update:open', false)">取消</a-button>
        <a-button type="primary" :loading="createMutation.isPending.value" @click="submit">
          创建(将克隆源仓库校验)
        </a-button>
      </a-space>
    </template>
  </a-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { useReposQuery } from '@/composables/useRepos'
import {
  useCreateMirrorChannelMutation,
} from '@/composables/useMirror'
import type { MirrorChannel } from '@/api/mirror'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [boolean]; created: [MirrorChannel] }>()

const defaultTarget = () => ({
  remote: 'github',
  repoUrl: '',
  targetModule: '',
  credType: 'none',
  credential: '',
  username: '',
})

const form = reactive({
  name: '',
  mode: 'publish',
  repoKey: '' as string,
  targets: [defaultTarget()],
})
const modeFixed = ref(false)

const { data: reposData } = useReposQuery()
const repoOptions = computed(() =>
  (reposData.value?.repos || []).map((r: { key: string; name: string }) => ({
    label: `${r.key}(${r.name})`,
    value: r.key,
  })),
)
function filterRepo(input: string, option: { label: string }) {
  return option.label.toLowerCase().includes(input.toLowerCase())
}

watch(
  () => form.repoKey,
  (key) => {
    // 目标 module 简单预填建议:与源仓库 key 同名
    if (key && form.mode === 'publish' && !form.targets[0].targetModule) {
      const repo = (reposData.value?.repos || []).find((r: { key: string }) => r.key === key)
      if (repo) form.targets[0].targetModule = `github.com/yi-nology/${repo.key}`
    }
  },
)

function addTarget() {
  form.targets.push(defaultTarget())
}

const createMutation = useCreateMirrorChannelMutation()

async function submit() {
  if (!form.name.trim()) return message.error('请填写通道名称')
  if (!form.repoKey) return message.error('请选择源仓库')
  for (const t of form.targets) {
    if (!t.remote.trim() || !t.repoUrl.trim()) return message.error('目标远端名与 URL 必填')
    if (form.mode === 'publish' && !t.targetModule.trim())
      return message.error('publish 模式需要填写目标 module')
  }
  const ch = await createMutation.mutateAsync({
    name: form.name.trim(),
    mode: form.mode,
    repoKey: form.repoKey,
    targets: form.targets.map((t) => ({ ...t })),
  })
  emit('created', ch)
}
</script>

<style scoped lang="scss">
.hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.45);
}
.target-block {
  padding: 12px;
  margin-bottom: 12px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 8px;
  .target-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
    font-weight: 500;
  }
}
</style>
