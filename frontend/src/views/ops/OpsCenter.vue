<template>
  <div class="page-container ops-center">
    <PageHeader title="运维中心" subtitle="健康评分 · 资产盘点 · 策略模板 · 失败重试" />

    <div class="content-card ops-card">
      <a-tabs v-model:activeKey="tab" class="ops-tabs">
        <a-tab-pane key="overview" tab="概览">
          <OverviewPanel />
        </a-tab-pane>
        <a-tab-pane key="health" tab="健康评分">
          <HealthScorePanel />
        </a-tab-pane>
        <a-tab-pane key="inventory" tab="资产盘点">
          <InventoryPanel />
        </a-tab-pane>
        <a-tab-pane key="templates" tab="策略模板">
          <TemplatesPanel />
        </a-tab-pane>
        <a-tab-pane key="deploykey" tab="部署密钥">
          <DeployKeyPanel />
        </a-tab-pane>
        <a-tab-pane key="backup" tab="冷备恢复">
          <BackupPanel />
        </a-tab-pane>
      </a-tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import PageHeader from '@/components/common/PageHeader.vue'
import OverviewPanel from './OverviewPanel.vue'
import HealthScorePanel from './HealthScorePanel.vue'
import InventoryPanel from './InventoryPanel.vue'
import TemplatesPanel from './TemplatesPanel.vue'
import DeployKeyPanel from './DeployKeyPanel.vue'
import BackupPanel from './BackupPanel.vue'

defineOptions({ name: 'OpsCenter' })

const tab = ref('overview')
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.ops-center {
  max-width: none;
}

.ops-card {
  padding: 0 8px;
}

.ops-tabs {
  :deep(.ant-tabs-nav) {
    margin: 0;
    padding: 0 8px;
  }

  :deep(.ant-tabs-content-holder) {
    padding: 16px 8px 20px;
  }
}
</style>
