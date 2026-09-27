<template>
  <div>
    <a-space class="toolbar">
      <a-button type="primary" :loading="loading" @click="load">刷新</a-button>
      <a-typography-text type="secondary">
        等级: gold ≥80 · silver ≥60 · bronze ≥40 · basic
      </a-typography-text>
    </a-space>

    <a-table
      :data-source="items"
      :columns="columns"
      :loading="loading"
      row-key="key"
      :pagination="{ pageSize: 10 }"
      size="middle"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'level'">
          <a-tag :color="levelColor(record.level)">{{ levelLabel(record.level) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'score'">
          <a-progress
            :percent="record.score"
            :status="record.score >= 80 ? 'success' : record.score >= 40 ? 'normal' : 'exception'"
            size="small"
            style="width: 120px"
          />
        </template>
        <template v-else-if="column.key === 'issues'">
          <a-space direction="vertical" :size="2">
            <a-typography-text v-for="(iss, i) in record.issues || []" :key="i" type="warning">
              · {{ iss }}
            </a-typography-text>
            <a-typography-text v-if="!(record.issues || []).length" type="success">无</a-typography-text>
          </a-space>
        </template>
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { opsApi, type HealthScoreItem } from '@/api/ops'
import { notifyError } from '@/utils/notify'

defineOptions({ name: 'HealthScorePanel' })

const items = ref<HealthScoreItem[]>([])
const loading = ref(false)

const columns = [
  { title: '任务', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: 'Key', dataIndex: 'key', key: 'key', ellipsis: true, width: 160 },
  { title: '等级', key: 'level', width: 100 },
  { title: '得分', key: 'score', width: 140 },
  { title: '问题', key: 'issues' },
]

function levelColor(l: string) {
  return { gold: 'gold', silver: 'silver', bronze: 'orange', basic: 'default' }[l] || 'default'
}
function levelLabel(l: string) {
  return { gold: 'Gold', silver: 'Silver', bronze: 'Bronze', basic: 'Basic' }[l] || l
}

async function load() {
  loading.value = true
  try {
    const data = await opsApi.healthScore(50)
    items.value = data.items || []
  } catch (e) {
    notifyError(e, '加载健康评分失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; }
</style>
