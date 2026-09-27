<template>
  <div class="page-container">
    <div class="page-header-bar">
      <div>
        <h1 class="page-title">事件日志</h1>
        <p class="page-subtitle">查看 Webhook 接收和处理的事件记录</p>
      </div>
      <a-space>
        <a-select
          v-model:value="repoKey"
          placeholder="选择仓库"
          style="width: 220px"
          show-search
          :filter-option="filterRepoOption"
          @change="loadEvents"
        >
          <a-select-option v-for="repo in repoStore.repos" :key="repo.key" :value="repo.key">
            {{ repo.name }}
          </a-select-option>
        </a-select>
        <a-tooltip :title="autoRefresh ? '关闭自动刷新' : '开启自动刷新'">
          <a-button :type="autoRefresh ? 'primary' : 'default'" @click="toggleAutoRefresh">
            <template #icon><ClockCircleOutlined /></template>
            {{ autoRefresh ? '自动刷新中' : '自动刷新' }}
          </a-button>
        </a-tooltip>
        <a-button @click="loadEvents">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
      </a-space>
    </div>

    <!-- Tabs -->
    <a-tabs v-model:activeKey="activeTab" class="event-tabs">
      <a-tab-pane key="all">
        <template #tab>
          <span>全部 <a-badge :count="webhookStore.events.length" :overflow-count="99" :number-style="{ fontSize: '11px' }" /></span>
        </template>
      </a-tab-pane>
      <a-tab-pane key="received">
        <template #tab>
          <span>已接收 <a-badge :count="receivedCount" :overflow-count="99" :number-style="{ fontSize: '11px', backgroundColor: '#1677FF' }" /></span>
        </template>
      </a-tab-pane>
      <a-tab-pane key="processed">
        <template #tab>
          <span>已处理 <a-badge :count="processedCount" :overflow-count="99" :number-style="{ fontSize: '11px', backgroundColor: '#52C41A' }" /></span>
        </template>
      </a-tab-pane>
      <a-tab-pane key="failed">
        <template #tab>
          <span>失败 <a-badge :count="failedCount" :overflow-count="99" :number-style="{ fontSize: '11px', backgroundColor: '#FF4D4F' }" /></span>
        </template>
      </a-tab-pane>
    </a-tabs>

    <!-- Table -->
    <a-table
      :columns="columns"
      :data-source="filteredEvents"
      :loading="webhookStore.loading"
      row-key="id"
      :pagination="pagination"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'event_id'">
          <a-tooltip :title="record.event_id">
            <span class="truncated-id">{{ record.event_id }}</span>
          </a-tooltip>
        </template>
        <template v-if="column.dataIndex === 'event_type'">
          <a-tag :color="eventTypeColor(record.event_type)">{{ record.event_type }}</a-tag>
        </template>
        <template v-if="column.dataIndex === 'source'">
          <span>{{ record.source || record.repo_key || '-' }}</span>
        </template>
        <template v-if="column.dataIndex === 'actor_name'">
          <span>{{ record.actor_name || '-' }}</span>
        </template>
        <template v-if="column.dataIndex === 'branch'">
          <span class="branch-tag">{{ record.branch || '-' }}</span>
        </template>
        <template v-if="column.dataIndex === 'status'">
          <StatusBadge :status="record.status" />
        </template>
        <template v-if="column.dataIndex === 'processed_at'">
          <span class="time-text">{{ record.processed_at || record.created_at || '-' }}</span>
        </template>
        <template v-if="column.dataIndex === 'action'">
          <a-space :size="4">
            <a-button type="link" size="small" @click="showDetail(record)">详情</a-button>
            <a-button
              v-if="record.status === 'failed' || record.status === 'received'"
              type="link"
              size="small"
              @click="handleRetry(record.id)"
            >
              重试
            </a-button>
          </a-space>
        </template>
      </template>
      <template #emptyText>
        <a-empty :description="repoKey ? '该仓库暂无事件记录' : '请选择一个仓库查看事件'" />
      </template>
    </a-table>

    <!-- Detail Modal -->
    <a-modal
      v-model:open="detailVisible"
      title="事件详情"
      :width="640"
      :footer="null"
    >
      <a-descriptions
        v-if="currentEvent"
        :column="2"
        bordered
        size="small"
      >
        <a-descriptions-item label="事件 ID" :span="2">
          <span style="font-family: monospace; font-size: 12px;">{{ currentEvent.event_id }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="触发时间">{{ currentEvent.created_at }}</a-descriptions-item>
        <a-descriptions-item label="事件类型">
          <a-tag :color="eventTypeColor(currentEvent.event_type)">{{ currentEvent.event_type }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="仓库">{{ currentEvent.repo_key }}</a-descriptions-item>
        <a-descriptions-item label="来源">{{ currentEvent.source || '-' }}</a-descriptions-item>
        <a-descriptions-item label="分支">{{ currentEvent.branch || '-' }}</a-descriptions-item>
        <a-descriptions-item label="触发者">{{ currentEvent.actor_name || '-' }}</a-descriptions-item>
        <a-descriptions-item label="Commit">
          <span style="font-family: monospace; font-size: 12px;">{{ currentEvent.commit_sha || '-' }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="状态">
          <StatusBadge :status="currentEvent.status" />
        </a-descriptions-item>
        <a-descriptions-item label="处理时间">{{ currentEvent.processed_at || '-' }}</a-descriptions-item>
        <a-descriptions-item v-if="currentEvent.error_message" label="错误信息" :span="2">
          <a-typography-text type="danger">{{ currentEvent.error_message }}</a-typography-text>
        </a-descriptions-item>
      </a-descriptions>
      <div style="text-align: right; margin-top: 16px;">
        <a-button @click="detailVisible = false">关闭</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, computed, onUnmounted } from 'vue'
import { ReloadOutlined, ClockCircleOutlined } from '@ant-design/icons-vue'
import { useWebhookStore } from '@/stores/webhook'
import { useRepoStore } from '@/stores/repo'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { WebhookEvent } from '@/types'
import { eventTypeColor } from '@/utils/dictionaries'
import { notifySuccess, notifyError } from '@/utils/notify'
import { makeRepoFilter } from '@/utils/repoFilter'

const webhookStore = useWebhookStore()
const repoStore = useRepoStore()
const repoKey = ref('')
const activeTab = ref('all')
const detailVisible = ref(false)
const currentEvent = ref<WebhookEvent | null>(null)
const autoRefresh = ref(false)
let refreshTimer: ReturnType<typeof setInterval> | null = null

const columns = [
  { title: '事件 ID', dataIndex: 'event_id', width: 160, ellipsis: true },
  { title: '类型', dataIndex: 'event_type', width: 120, align: 'center' as const },
  { title: '来源', dataIndex: 'source', width: 120, ellipsis: true },
  { title: '操作者', dataIndex: 'actor_name', width: 100 },
  { title: '分支', dataIndex: 'branch', width: 120 },
  { title: '状态', dataIndex: 'status', width: 90, align: 'center' as const },
  { title: '处理时间', dataIndex: 'processed_at', width: 140 },
  { title: '操作', dataIndex: 'action', width: 100, align: 'center' as const },
]

const pagination = {
  pageSize: 20,
  showSizeChanger: true,
  showTotal: (total: number) => `共 ${total} 条事件`,
}

const receivedCount = computed(() => webhookStore.events.filter(e => e.status === 'received').length)
const processedCount = computed(() => webhookStore.events.filter(e => e.status === 'processed').length)
const failedCount = computed(() => webhookStore.events.filter(e => e.status === 'failed').length)

const filteredEvents = computed(() => {
  if (activeTab.value === 'all') return webhookStore.events
  return webhookStore.events.filter(e => e.status === activeTab.value)
})

const filterRepoOption = makeRepoFilter(() => repoStore.repos)

function loadEvents() {
  if (repoKey.value) {
    webhookStore.fetchEvents(repoKey.value).catch((e) => notifyError(e, '加载事件失败'))
  }
}

function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    refreshTimer = setInterval(loadEvents, 10000) // refresh every 10s
  } else {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  }
}

onMounted(async () => {
  try {
    await repoStore.fetchRepos()
    if (repoStore.repos.length > 0) {
      repoKey.value = repoStore.repos[0].key
      loadEvents()
    }
  } catch (e) {
    notifyError(e, '加载仓库失败')
  }
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})

function showDetail(e: WebhookEvent) {
  currentEvent.value = e
  detailVisible.value = true
}

async function handleRetry(id: number) {
  try {
    await webhookStore.retryEvent(id)
    notifySuccess('事件重试已触发')
    loadEvents()
  } catch (e) {
    notifyError(e, '重试失败')
  }
}
</script>

<style scoped lang="scss">
@import '@/styles/variables.scss';

.page-container {
  background: $background-color;
  min-height: 100%;
}

.page-header-bar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: $spacing-lg;
}

.page-title {
  font-size: 22px;
  font-weight: 600;
  color: $text-primary;
  margin: 0;
  line-height: 1.3;
}

.page-subtitle {
  font-size: 14px;
  color: $text-secondary;
  margin: 4px 0 0 0;
}

.event-tabs {
  margin-bottom: $spacing-md;
  background: $card-background;
  border-radius: $border-radius-md;
  padding: 4px $spacing-md 0;
  box-shadow: $shadow-card;
}

.truncated-id {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  vertical-align: middle;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 12px;
}

.time-text {
  color: $text-secondary;
  font-size: 13px;
}
</style>
