<template>
  <div>
    <a-space class="toolbar">
      <a-button type="primary" :loading="loading" @click="load">刷新</a-button>
      <a-button danger :loading="retrying" @click="batchRetry">批量重试最近失败</a-button>
    </a-space>

    <a-row :gutter="16" class="stats">
      <a-col :span="6">
        <a-card><a-statistic title="仓库数" :value="data?.repo_count ?? 0" /></a-card>
      </a-col>
      <a-col :span="6">
        <a-card><a-statistic title="运行中" :value="data?.tasks_by_status?.running ?? 0" /></a-card>
      </a-col>
      <a-col :span="6">
        <a-card>
          <a-statistic
            title="失败任务"
            :value="data?.tasks_by_status?.failed ?? 0"
            :value-style="{ color: '#cf1322' }"
          />
        </a-card>
      </a-col>
      <a-col :span="6">
        <a-card><a-statistic title="成功任务" :value="data?.tasks_by_status?.success ?? 0" /></a-card>
      </a-col>
    </a-row>

    <a-card title="最近失败执行" class="fail-card">
      <a-table
        :data-source="data?.recent_failed || []"
        :columns="failColumns"
        row-key="run_id"
        size="small"
        :pagination="false"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'error'">
            <a-typography-text type="danger" ellipsis>
              {{ record.error_msg || record.err_type || '—' }}
            </a-typography-text>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button size="small" type="primary" @click="retryOne(record.run_id)">重试</a-button>
          </template>
        </template>
      </a-table>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { opsApi, type OverviewData } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'OverviewPanel' })

const data = ref<OverviewData | null>(null)
const loading = ref(false)
const retrying = ref(false)

const failColumns = [
  { title: 'Run ID', dataIndex: 'run_id', width: 90 },
  { title: '任务', dataIndex: 'task_key', ellipsis: true },
  { title: '错误', key: 'error' },
  { title: '时间', dataIndex: 'end_at', width: 170 },
  { title: '操作', key: 'action', width: 90 },
]

async function load() {
  loading.value = true
  try {
    data.value = await opsApi.overview()
  } catch (e) {
    notifyError(e, '加载同步概览失败')
  } finally {
    loading.value = false
  }
}

async function retryOne(runId: number) {
  try {
    const r = await opsApi.retryRun(runId)
    notifySuccess(`已重新触发 ${r.task_key}`)
    await load()
  } catch (e) {
    notifyError(e, '重试失败')
  }
}

async function batchRetry() {
  retrying.value = true
  try {
    const r = await opsApi.retryBatch(10)
    notifySuccess(`已重试 ${r.retried?.length || 0} 条`)
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
.toolbar { margin-bottom: 12px; }
.stats { margin-bottom: 16px; }
.fail-card { margin-top: 8px; }
</style>
