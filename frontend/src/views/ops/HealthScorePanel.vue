<template>
  <div>
    <a-space class="toolbar" wrap>
      <a-button type="primary" :loading="loading" @click="load(false)">刷新</a-button>
      <a-button :loading="loading" @click="load(true)">
        <template #icon><SafetyOutlined /></template>
        含漂移检测
      </a-button>
      <a-typography-text type="secondary">
        维度: reliability35 · freshness20 · schedule15 · safety15 · completeness15
      </a-typography-text>
    </a-space>

    <!-- 待办：需要关注 + 建议动作 -->
    <div v-if="attention.length || topActions.length" class="attention-card">
      <div class="attention-grid">
        <div class="attention-block">
          <div class="block-title">
            需要关注
            <a-tag color="red">{{ attention.length }}</a-tag>
          </div>
          <div v-if="!attention.length" class="block-empty">全部 ≥60 分</div>
          <div v-for="t in attention" :key="t.key" class="attention-row">
            <a-tag :color="levelColor(t.level)">{{ t.score }}</a-tag>
            <span class="att-name">{{ t.name || t.key }}</span>
            <span class="att-issues">{{ (t.issues || []).slice(0, 2).join(' · ') }}</span>
          </div>
        </div>
        <div class="attention-block">
          <div class="block-title">
            建议动作
            <a-tag>{{ topActions.length }}</a-tag>
          </div>
          <div v-if="!topActions.length" class="block-empty">暂无</div>
          <div v-for="(a, i) in topActions" :key="i" class="action-row">
            <span class="action-text">{{ a }}</span>
          </div>
        </div>
      </div>
    </div>

    <a-table
      :data-source="items"
      :columns="columns"
      :loading="loading"
      row-key="key"
      :pagination="{ pageSize: 10 }"
      size="middle"
      :expand-column-width="60"
    >
      <template #expandedRowRender="{ record }">
        <div class="dim-list">
          <div v-for="d in record.dimensions || []" :key="d.name" class="dim-item">
            <div class="dim-head">
              <span class="dim-name">{{ d.name }}</span>
              <a-progress
                :percent="d.score"
                :status="d.score >= 80 ? 'success' : d.score >= 50 ? 'normal' : 'exception'"
                size="small"
                style="width: 120px"
              />
              <span class="dim-reason">{{ d.reason }}</span>
            </div>
            <div v-if="(d.detail || []).length" class="dim-detail">
              <a-typography-text v-for="(x, i) in d.detail" :key="i" type="secondary">· {{ x }}</a-typography-text>
            </div>
            <div v-if="d.action" class="dim-action">
              <BulbOutlined /> {{ d.action }}
            </div>
          </div>
          <div v-if="(record.action_items || record.actions || []).length" class="row-actions">
            <a-typography-text strong>建议动作：</a-typography-text>
            <template v-if="record.action_items?.length">
              <div v-for="(a, i) in record.action_items" :key="i" class="act-line">
                <a-tag :color="a.priority === 1 ? 'red' : 'default'">P{{ a.priority ?? 3 }}</a-tag>
                <span class="act-title">{{ a.title }}</span>
                <a-button
                  v-if="a.command"
                  size="small"
                  type="link"
                  :danger="!!a.danger"
                  @click="copyCommand(a.command || '')"
                >
                  <CopyOutlined /> 复制命令
                </a-button>
              </div>
            </template>
            <template v-else>
              <div v-for="(a, i) in record.actions" :key="i" class="dim-action">{{ a }}</div>
            </template>
          </div>
        </div>
      </template>
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
import { BulbOutlined, CopyOutlined, SafetyOutlined } from '@ant-design/icons-vue'
import { opsApi, type HealthScoreItem } from '@/api/ops'
import { copyToClipboard } from '@/utils'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'HealthScorePanel' })

const items = ref<HealthScoreItem[]>([])
const attention = ref<HealthScoreItem[]>([])
const topActions = ref<string[]>([])
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

async function copyCommand(cmd: string) {
  try {
    await copyToClipboard(cmd)
    notifySuccess('命令已复制')
  } catch {
    notifyError('复制失败')
  }
}

async function load(withDrift: boolean) {
  loading.value = true
  try {
    const data = await opsApi.healthScore(50, { withDrift })
    items.value = data.items || []
    attention.value = (data.attention || items.value.filter((i) => i.score < 60))
      .slice()
      .sort((a, b) => a.score - b.score)
    topActions.value = data.summary?.top_actions || []
  } catch (e) {
    notifyError(e, '加载健康评分失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => load(false))
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.toolbar {
  margin-bottom: $spacing-md;
}

.attention-card {
  background: $bg-primary;
  border: 1px solid $border-light;
  border-radius: $radius-lg;
  padding: $spacing-md $spacing-lg;
  margin-bottom: $spacing-md;
}

.attention-grid {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: $spacing-lg;
}

@media (max-width: 900px) {
  .attention-grid {
    grid-template-columns: 1fr;
  }
}

.block-title {
  font-weight: 600;
  margin-bottom: $spacing-sm;
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  color: $text-primary;
}

.block-empty {
  color: $text-tertiary;
  font-size: $fs-caption;
}

.attention-row {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  padding: 4px 0;
  border-bottom: 1px dashed $border-muted;
  font-size: $fs-caption;
}

.att-name {
  font-weight: 500;
  color: $text-primary;
}

.att-issues {
  color: $warning;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.action-row {
  font-size: $fs-caption;
  padding: 3px 0;
  color: $text-secondary;
}

.dim-list {
  padding: $spacing-sm $spacing-lg;
  background: $bg-canvas;
  border-radius: $radius-md;
}

.dim-item {
  margin-bottom: 10px;
}

.dim-head {
  display: flex;
  align-items: center;
  gap: $spacing-md;
}

.dim-name {
  width: 110px;
  font-family: $font-mono;
  font-size: $fs-caption;
  font-weight: 600;
  color: $primary;
}

.dim-reason {
  color: $text-secondary;
  font-size: $fs-caption;
}

.dim-detail {
  margin: 2px 0 0 122px;
  display: flex;
  flex-wrap: wrap;
  gap: $spacing-sm;
  font-size: $fs-caption;
  color: $text-tertiary;
}

.dim-action {
  margin: 2px 0 0 122px;
  color: $warning;
  font-size: $fs-caption;
}

.row-actions {
  margin-top: $spacing-sm;
  padding-top: $spacing-sm;
  border-top: 1px dashed $border-muted;
}

.act-line {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  margin-top: 4px;
  font-size: $fs-caption;
}

.act-title {
  color: $text-secondary;
  flex: 1;
  min-width: 0;
}
</style>
