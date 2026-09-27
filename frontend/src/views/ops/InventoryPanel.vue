<template>
  <div>
    <a-space class="toolbar" wrap>
      <a-statistic title="仓库总数" :value="summary.total" />
      <a-statistic title="已覆盖" :value="summary.covered" :value-style="{ color: '#3f8600' }" />
      <a-statistic title="孤儿仓库" :value="summary.orphan" :value-style="{ color: '#cf1322' }" />
      <a-statistic title="失败覆盖" :value="summary.failing" :value-style="{ color: '#d46b08' }" />
      <a-button type="primary" :loading="loading" @click="load">刷新</a-button>
      <a-button danger :loading="retrying" @click="batchRetry">批量重试失败</a-button>
    </a-space>

    <a-table
      :data-source="items"
      :columns="columns"
      :loading="loading"
      row-key="repo_key"
      :pagination="{ pageSize: 10 }"
      size="middle"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'coverage'">
          <a-tag :color="covColor(record.coverage)">{{ covLabel(record.coverage) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'last_run_status'">
          <StatusBadge v-if="record.last_run_status" :status="record.last_run_status" />
          <a-typography-text v-else type="secondary">—</a-typography-text>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-button
            v-if="record.coverage === 'failing' || record.coverage === 'no_task'"
            size="small"
            @click="batchRetry(record.task_keys?.[0])"
          >
            重试/关注
          </a-button>
        </template>
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { opsApi, type InventoryItem } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'
import StatusBadge from '@/components/common/StatusBadge.vue'

defineOptions({ name: 'InventoryPanel' })

const items = ref<InventoryItem[]>([])
const loading = ref(false)
const retrying = ref(false)
const meta = ref({ total: 0, covered: 0, orphan: 0, failing: 0 })

const summary = computed(() => meta.value)

const columns = [
  { title: '仓库', dataIndex: 'repo_name', key: 'repo_name', ellipsis: true },
  { title: 'Key', dataIndex: 'repo_key', key: 'repo_key', ellipsis: true, width: 150 },
  { title: '平台', dataIndex: 'platform', key: 'platform', width: 100 },
  { title: '覆盖', key: 'coverage', width: 110 },
  { title: '最近执行', key: 'last_run_status', width: 110 },
  { title: '时间', dataIndex: 'last_run_at', key: 'last_run_at', width: 160 },
  { title: '操作', key: 'action', width: 120 },
]

function covColor(c: string) {
  return { covered: 'green', no_task: 'red', failing: 'orange', stale: 'gold' }[c] || 'default'
}
function covLabel(c: string) {
  return { covered: '已覆盖', no_task: '孤儿', failing: '失败', stale: '久未运行' }[c] || c
}

async function load() {
  loading.value = true
  try {
    const data = await opsApi.inventory()
    items.value = data.items || []
    meta.value = {
      total: data.total || 0,
      covered: data.covered || 0,
      orphan: data.orphan_repos || 0,
      failing: data.failing || 0,
    }
  } catch (e) {
    notifyError(e, '加载资产盘点失败')
  } finally {
    loading.value = false
  }
}

async function batchRetry(taskKey?: string) {
  retrying.value = true
  try {
    const r = await opsApi.retryBatch(10, taskKey)
    notifySuccess(`已重试 ${r.retried?.length || 0} 条(跳过 ${r.skipped})`)
    await load()
  } catch (e) {
    notifyError(e, '批量重试失败')
  } finally {
    retrying.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; gap: 24px; display: flex; align-items: center; }
</style>
