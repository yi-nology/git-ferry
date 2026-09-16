<template>
  <a-drawer :open="open" :title="`执行记录 #${run?.id ?? ''}`" width="560" @close="emit('update:open', false)">
    <template v-if="run">
      <a-descriptions size="small" bordered :column="2">
        <a-descriptions-item label="状态">
          <a-tag :color="statusColor(run.status)">{{ statusText(run.status) }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="耗时">{{ formatDuration(run.durationMs) }}</a-descriptions-item>
        <a-descriptions-item label="类别">{{ run.kind === 'verify' ? '验证' : '发布' }}</a-descriptions-item>
        <a-descriptions-item label="Tags">{{ parseTags(run.tags).join(', ') || '—' }}</a-descriptions-item>
        <a-descriptions-item label="开始" :span="2">{{ run.startedAt || '—' }}</a-descriptions-item>
      </a-descriptions>

      <a-alert
        v-if="run.error"
        type="error"
        show-icon
        :message="run.error"
        class="block"
      />

      <h4 class="section">步骤</h4>
      <a-timeline class="steps">
        <a-timeline-item
          v-for="(s, i) in steps"
          :key="i"
          :color="stepColor(s.status)"
        >
          <span :class="{ failed: s.status === 'failed' }">{{ s.name }}</span>
          <div v-if="s.detail" class="detail">{{ s.detail }}</div>
        </a-timeline-item>
      </a-timeline>

      <template v-if="tagStatuses.length">
        <h4 class="section">行级结果</h4>
        <a-table
          :data-source="tagStatuses"
          :pagination="false"
          row-key="tag"
          size="small"
          :columns="tagColumns"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="tagColor(record.status)">{{ statusText(record.status) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'detail'">
              <span class="detail">{{ record.detail }}</span>
            </template>
          </template>
        </a-table>
      </template>
    </template>
    <a-spin v-else />
  </a-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { MirrorRun, MirrorStep, MirrorTagStatus } from '@/api/mirror'

const props = defineProps<{ open: boolean; run?: MirrorRun }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

function parseJSON<T>(s: string, fallback: T): T {
  try {
    return JSON.parse(s) as T
  } catch {
    return fallback
  }
}

const steps = computed<MirrorStep[]>(() => parseJSON<MirrorStep[]>(props.run?.steps || '[]', []))
const tagStatuses = computed<MirrorTagStatus[]>(() =>
  Object.values(parseJSON<Record<string, MirrorTagStatus>>(props.run?.tagStatuses || '{}', {})),
)
function parseTags(s: string): string[] {
  try {
    return JSON.parse(`[${s || ''}]`) as string[]
  } catch {
    return s ? s.split(',') : []
  }
}

const tagColumns = [
  { title: 'Tag', dataIndex: 'tag', key: 'tag' },
  { title: '状态', key: 'status', width: 100 },
  { title: 'Commit', dataIndex: 'commit', key: 'commit', width: 110 },
  { title: '说明', key: 'detail' },
]

function statusColor(s: string) {
  return s === 'success' ? 'green' : s === 'divergent' ? 'orange' : s === 'running' ? 'blue' : s === 'failed' ? 'red' : 'default'
}
function statusText(s: string) {
  const m: Record<string, string> = {
    success: '成功', failed: '失败', divergent: '分歧拒绝',
    running: '执行中', pending: '排队中',
  }
  return m[s] || s
}
function tagColor(s: string) {
  return s === 'success' ? 'green' : s === 'divergent' ? 'orange' : 'red'
}
function stepColor(s: string) {
  return s === 'success' ? 'green' : s === 'running' ? 'blue' : s === 'failed' ? 'red' : 'gray'
}
function formatDuration(ms: number) {
  if (!ms) return '—'
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`
}
</script>

<style scoped lang="scss">
.block {
  margin-top: 12px;
}
.section {
  margin: 16px 0 8px;
  font-weight: 600;
}
.steps {
  .failed {
    color: #cf1322;
  }
  .detail {
    font-size: 12px;
    color: rgba(0, 0, 0, 0.45);
    word-break: break-all;
  }
}
</style>
