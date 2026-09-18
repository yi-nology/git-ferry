<template>
  <div class="mirror-detail" v-if="channel">
    <a-page-header
      :title="channel.name"
      :sub-title="channel.module"
      @back="router.push('/mirror')"
    >
      <template #tags>
        <a-tag :color="channel.mode === 'publish' ? 'blue' : 'purple'">
          {{ channel.mode === 'publish' ? '🚀 开源发布' : '🗄 仓库备份' }}
        </a-tag>
      </template>
      <template #extra>
        <a-button danger @click="showDelete = true">删除通道</a-button>
      </template>
    </a-page-header>

    <!-- ② 发布目标 -->
    <a-card title="发布目标" size="small" class="block">
      <template #extra>
        <a-button
          v-for="t in channel.targets"
          :key="'test-' + t.id"
          size="small"
          :loading="testingId === t.id"
          class="test-btn"
          @click="testTarget(t.id)"
        >
          测试 {{ t.remote }}
        </a-button>
      </template>
      <a-table
        :data-source="channel.targets"
        :pagination="false"
        row-key="id"
        size="small"
        :columns="targetColumns"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'module'">
            <code>{{ record.targetModule || '—' }}</code>
          </template>
          <template v-else-if="column.key === 'cred'">
            <a-tag>{{ credText(record) }}</a-tag>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- ③ 版本矩阵 -->
    <a-card title="版本矩阵" size="small" class="block">
      <template #extra>
        <a-space>
          <a-select
            v-model:value="selectedTargetId"
            size="small"
            style="min-width: 220px"
            :options="targetOptions"
            placeholder="选择目标"
          />
          <a-button
            type="primary"
            size="small"
            :disabled="selectedRowKeys.length === 0 || !selectedTargetId"
            :loading="previewing || executing"
            @click="startPublish"
          >
            发布选中({{ selectedRowKeys.length }})
          </a-button>
          <a-button size="small" @click="refetchVersions">刷新</a-button>
        </a-space>
      </template>

      <a-table
        :data-source="versions"
        :pagination="{ pageSize: 15 }"
        row-key="tag"
        size="small"
        :columns="versionColumns"
        :row-selection="{ selectedRowKeys, onChange: onSelectionChange }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key?.startsWith('target-')">
            <a-space :size="4">
              <a-tag :color="stateColor(cellOf(record, column).state)">
                {{ stateText(cellOf(record, column).state) }}
              </a-tag>
              <a-tag v-if="cellOf(record, column).verify === 'passed'" color="success">已验证</a-tag>
            </a-space>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-space :size="4">
              <a-button
                size="small"
                type="link"
                :disabled="!isPublished(record)"
                @click="verifyTag(record.tag)"
              >
                验证
              </a-button>
              <a-button
                v-if="channel?.mode === 'publish'"
                size="small"
                type="link"
                @click="copyGoGet(record.tag)"
              >
                复制 go get
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- ④ 执行记录 -->
    <a-card title="执行记录" size="small" class="block">
      <a-table
        :data-source="runs"
        :pagination="{ pageSize: 10 }"
        row-key="id"
        size="small"
        :columns="runColumns"
        :loading="runsLoading"
        :custom-row="(r: MirrorRun) => ({ onClick: () => openRun(r), style: { cursor: 'pointer' } })"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'tags'">
            {{ shortTags(record.tags) }}
          </template>
        </template>
      </a-table>
    </a-card>

    <RunConfirmModal
      v-model:open="showConfirm"
      :source-module="channel.module || ''"
      :target-module="selectedTarget?.targetModule || ''"
      :previews="previews"
      :previewing="previewing"
      :preview-error="previewError"
      :executing="executing"
      @confirm="doPublish"
    />

    <MirrorRunDrawer v-model:open="showRunDrawer" :run="activeRun" />

    <a-modal
      v-model:open="showDelete"
      title="删除通道"
      ok-text="确认删除"
      :ok-button-props="{ danger: true }"
      @ok="doDelete"
    >
      <p>此操作将删除通道及其目标配置(执行记录保留)。请输入通道名 <b>{{ channel.name }}</b> 确认:</p>
      <a-input v-model:value="deleteConfirm" placeholder="通道名" />
    </a-modal>
  </div>
  <a-spin v-else style="display: block; margin: 80px auto" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  useMirrorChannelQuery,
  useMirrorVersionsQuery,
  useMirrorRunsQuery,
  useMirrorRunQuery,
  useMirrorPreviewMutation,
  useExecuteMirrorRunMutation,
  useMirrorVerifyMutation,
  useTestMirrorTargetMutation,
  useDeleteMirrorChannelMutation,
} from '@/composables/useMirror'
import type { MirrorRun, MirrorVersion, MirrorPreviewReport } from '@/api/mirror'
import RunConfirmModal from './components/RunConfirmModal.vue'
import MirrorRunDrawer from './components/MirrorRunDrawer.vue'

const route = useRoute()
const router = useRouter()
const channelId = computed(() => Number(route.params.id) || 0)

const { data: channel } = useMirrorChannelQuery(channelId)

// ---- 目标 ----
const targetColumns = [
  { title: '远端名', dataIndex: 'remote', key: 'remote', width: 120 },
  { title: '目标仓库', dataIndex: 'repoUrl', key: 'repoUrl' },
  { title: '目标 module', key: 'module' },
  { title: '凭据', key: 'cred', width: 140 },
]
const testingId = ref(0)
const testMutation = useTestMirrorTargetMutation()
async function testTarget(id: number) {
  testingId.value = id
  try {
    await testMutation.mutateAsync(id)
    message.success('连接成功')
  } catch (e) {
    message.error(`连接失败: ${(e as Error).message}`)
  } finally {
    testingId.value = 0
  }
}
function credText(t: { credType: string; hasCredential: boolean }) {
  const names: Record<string, string> = { none: '本机凭据', token: 'Token', ssh_key: 'SSH 私钥' }
  return `${names[t.credType] || t.credType}${t.hasCredential ? '(已配置)' : ''}`
}

const targetOptions = computed(() =>
  (channel.value?.targets || []).map((t) => ({
    label: `${t.remote} → ${t.targetModule || t.repoUrl}`,
    value: t.id,
  })),
)
const selectedTargetId = ref<number>()
watch(
  () => channel.value?.targets,
  (ts) => {
    if (ts?.length && !selectedTargetId.value) selectedTargetId.value = ts[0].id
  },
  { immediate: true },
)
const selectedTarget = computed(() =>
  channel.value?.targets?.find((t) => t.id === selectedTargetId.value),
)

// ---- 版本矩阵 ----
const anyRunning = computed(() =>
  Object.values(runsMap.value).some((r) => r && ['pending', 'running'].includes(r.status)),
)
const { data: versionsData, refetch: refetchVersions } = useMirrorVersionsQuery(channelId, anyRunning)
const versions = computed(() => versionsData.value?.versions || [])

const versionColumns = computed(() => {
  const cols: { title: string; dataIndex?: string; key: string; width?: number }[] = [
    { title: 'Tag', dataIndex: 'tag', key: 'tag', width: 120 },
    { title: '源 commit', dataIndex: 'commit', key: 'commit', width: 110 },
  ]
  for (const t of channel.value?.targets || []) {
    cols.push({ title: t.remote, key: `target-${t.id}` })
  }
  cols.push({ title: '操作', key: 'actions', width: 180 })
  return cols
})

function cellOf(record: MirrorVersion, col: { key?: string }) {
  const tid = Number(col.key?.replace('target-', '')) || selectedTargetId.value || 0
  return (
    record.targets.find((t) => t.targetId === tid) || {
      state: 'unpublished',
      verify: 'unverified',
      targetId: tid,
      target: '',
    }
  )
}
function column0(record: MirrorVersion) {
  return { key: `target-${selectedTargetId.value || record.targets[0]?.targetId || 0}` }
}
function isPublished(record: MirrorVersion) {
  const cell = cellOf(record, column0(record))
  return cell.state === 'published' || cell.state === 'backedup'
}
function stateColor(s: string) {
  return s === 'published' || s === 'backedup'
    ? 'green'
    : s === 'divergent'
      ? 'orange'
      : s === 'failed'
        ? 'red'
        : 'default'
}
function stateText(s: string) {
  const m: Record<string, string> = {
    published: '已发布', backedup: '已备份', divergent: '分歧', failed: '失败',
    unpublished: '未发布', unbackedup: '未备份',
  }
  return m[s] || s
}

const selectedRowKeys = ref<string[]>([])
function onSelectionChange(keys: (string | number)[]) {
  selectedRowKeys.value = keys as string[]
}

// ---- 发布流:预检 → 确认 → 执行 ----
const showConfirm = ref(false)
const previews = ref<MirrorPreviewReport[]>([])
const previewing = ref(false)
const previewError = ref('')
const executing = ref(false)
const previewMutation = useMirrorPreviewMutation()
const executeMutation = useExecuteMirrorRunMutation()
const activeRunId = ref(0)
const { data: activeRun } = useMirrorRunQuery(activeRunId)

watch(activeRun, (r) => {
  if (r && ['success', 'failed', 'divergent'].includes(r.status)) {
    executing.value = false
    refetchVersions()
    if (r.status === 'success') message.success('发布完成')
    else if (r.status === 'divergent') message.warning('远端存在分歧,已拒绝覆盖')
    else message.error(`发布失败: ${r.error || ''}`)
  }
})

async function startPublish() {
  if (!selectedTargetId.value || !channel.value) return
  const tags = [...selectedRowKeys.value]
  previews.value = []
  previewError.value = ''
  previewing.value = true
  showConfirm.value = true
  try {
    for (const tag of tags) {
      const p = await previewMutation.mutateAsync({
        channelId: channelId.value,
        targetId: selectedTargetId.value,
        tag,
      })
      previews.value.push(p)
    }
  } catch (e) {
    previewError.value = (e as Error).message
  } finally {
    previewing.value = false
  }
}

async function doPublish({ allowOverwrite }: { allowOverwrite: boolean }) {
  if (!selectedTargetId.value) return
  executing.value = true
  try {
    const run = await executeMutation.mutateAsync({
      channelId: channelId.value,
      targetId: selectedTargetId.value,
      tags: [...selectedRowKeys.value],
      allowOverwrite,
    })
    activeRunId.value = run.id
    showConfirm.value = false
    showRunDrawer.value = true
    selectedRowKeys.value = []
  } catch (e) {
    executing.value = false
    message.error((e as Error).message)
  }
}

// ---- 验证 / 复制 ----
const verifyMutation = useMirrorVerifyMutation()
async function verifyTag(tag: string) {
  if (!selectedTargetId.value) return
  try {
    await verifyMutation.mutateAsync({ channelId: channelId.value, targetId: selectedTargetId.value, tag })
    message.success(`${tag} 验证通过(仓库可达/版本存在/zip 可拉/身份匹配)`)
  } catch (e) {
    message.error(`验证失败: ${(e as Error).message}`)
  }
}
async function copyGoGet(tag: string) {
  const t = selectedTarget.value
  if (!t) return
  await navigator.clipboard.writeText(`go get ${t.targetModule}@${tag}`)
  message.success('已复制')
}

// ---- 执行记录 ----
const { data: runsData, isLoading: runsLoading } = useMirrorRunsQuery(channelId)
const runs = computed(() => runsData.value?.list || [])
const runsMap = computed<Record<number, MirrorRun>>(() =>
  Object.fromEntries(runs.value.map((r) => [r.id, r])),
)
const showRunDrawer = ref(false)
const openRunId = ref(0)
function openRun(r: MirrorRun) {
  openRunId.value = r.id
  activeRunId.value = 0
  showRunDrawer.value = true
  setTimeout(() => (activeRunId.value = r.id), 0)
}

const runColumns = [
  { title: 'ID', dataIndex: 'id', width: 60 },
  { title: '状态', key: 'status', width: 100 },
  { title: 'Tags', key: 'tags' },
  { title: '耗时', dataIndex: 'durationMs', key: 'durationMs', width: 90 },
  { title: '开始时间', dataIndex: 'startedAt', width: 170 },
]
function shortTags(s: string) {
  return s.replace(/"/g, '')
}
function statusColor(s: string) {
  return s === 'success' ? 'green' : s === 'divergent' ? 'orange' : s === 'running' ? 'blue' : s === 'failed' ? 'red' : 'default'
}
function statusText(s: string) {
  const m: Record<string, string> = { success: '成功', failed: '失败', divergent: '分歧拒绝', running: '执行中', pending: '排队中' }
  return m[s] || s
}

// ---- 删除 ----
const showDelete = ref(false)
const deleteConfirm = ref('')
const deleteMutation = useDeleteMirrorChannelMutation()
async function doDelete() {
  if (!channel.value) return
  if (deleteConfirm.value !== channel.value.name) return message.error('通道名不匹配')
  try {
    await deleteMutation.mutateAsync({ id: channel.value.id, confirm: deleteConfirm.value })
    message.success('已删除')
    router.push('/mirror')
  } catch (e) {
    message.error((e as Error)?.message || '删除失败')
  }
}
</script>

<style scoped lang="scss">
.mirror-detail {
  padding: 4px 20px 20px;
}
.block {
  margin-top: 16px;
}
.test-btn {
  margin-left: 8px;
}
:deep(.ant-table-row) {
  cursor: pointer;
}
</style>
