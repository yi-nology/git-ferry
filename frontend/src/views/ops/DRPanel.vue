<template>
  <div>
    <a-space class="toolbar" wrap>
      <a-button type="primary" :loading="drilling" @click="runBatchDrill">批量灾备演练</a-button>
      <a-button @click="buildManifest">生成完整性清单</a-button>
      <a-button @click="verifyManifest">校验完整性</a-button>
      <a-button @click="loadRpo">刷新 RPO</a-button>
      <a-button :loading="driftLoading" @click="runDrift">漂移检测</a-button>
      <a-button @click="verifyAuditChain">审计链校验</a-button>
      <a-button @click="exportReport('json')">导出 JSON</a-button>
      <a-button @click="exportReport('csv')">导出 CSV</a-button>
    </a-space>

    <a-row :gutter="16" style="margin-top: 16px">
      <a-col :span="8">
        <a-card size="small" title="RPO / RTO">
          <a-statistic title="最差 RPO" :value="rpo?.overall_rpo_human || '-'" />
          <a-statistic title="超标任务" :value="rpo?.violations ?? 0" style="margin-top: 12px" />
          <a-statistic title="最差任务" :value="rpo?.worst_task || '-'" style="margin-top: 12px" />
        </a-card>
      </a-col>
      <a-col :span="8">
        <a-card size="small" title="备份完整性">
          <p v-if="manifestResult">
            <a-tag :color="manifestResult.ok ? 'green' : 'red'">
              {{ manifestResult.ok ? '通过' : '失败' }}
            </a-tag>
            {{ manifestResult.message }}
          </p>
          <p v-else-if="manifest">
            Merkle Root: <code>{{ manifest.merkle_root?.slice(0, 16) }}…</code>
            <br />条目 {{ manifest.entry_count }} ·
            {{ formatSize(manifest.total_size) }}
          </p>
          <p v-else>尚未生成清单</p>
        </a-card>
      </a-col>
      <a-col :span="8">
        <a-card size="small" title="审计哈希链">
          <p v-if="auditChain">
            <a-tag :color="auditChain.ok ? 'green' : 'red'">
              {{ auditChain.ok ? '完整' : '已破坏' }}
            </a-tag>
            已校验 {{ auditChain.checked }} 条
            <span v-if="!auditChain.ok">· {{ auditChain.message }}</span>
          </p>
          <p v-else>点击右上角校验</p>
        </a-card>
      </a-col>
    </a-row>

    <a-divider>RPO 明细</a-divider>
    <a-table
      :data-source="rpo?.metrics || []"
      :columns="rpoColumns"
      row-key="task_key"
      size="middle"
      :pagination="{ pageSize: 10 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'rpo'">
          <a-tag :color="record.rpo_violated ? 'red' : 'green'">{{ record.rpo_human }}</a-tag>
        </template>
        <template v-else-if="column.key === 'size'">{{ formatSize(record.total_size) }}</template>
      </template>
    </a-table>

    <a-divider>漂移检测</a-divider>
    <a-alert
      v-if="drift"
      :type="drift.drifted > 0 ? 'warning' : 'success'"
      show-icon
      style="margin-bottom: 12px"
      :message="`已检查 ${drift.checked} 个任务，漂移 ${drift.drifted} 个`"
    />
    <a-table
      v-if="drift && drift.items?.length"
      :data-source="drift.items"
      :columns="driftColumns"
      row-key="task_key"
      size="middle"
      :pagination="{ pageSize: 8 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'status'">
          <a-tag :color="record.drifted ? 'red' : 'green'">{{ record.message }}</a-tag>
        </template>
      </template>
    </a-table>

    <a-divider>演练历史</a-divider>
    <a-table
      :data-source="history"
      :columns="drillColumns"
      row-key="hash"
      size="middle"
      :pagination="{ pageSize: 8 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'status'">
          <a-tag :color="record.report?.success ? 'green' : 'red'">
            {{ record.report?.success ? '成功' : '失败' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'rto'">{{ record.report?.est_rto }}</template>
        <template v-else-if="column.key === 'bundle'">{{ record.report?.bundle_name }}</template>
        <template v-else-if="column.key === 'time'">
          {{ record.report?.started_at?.replace('T', ' ').slice(0, 19) }}
        </template>
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  opsApi,
  type BackupManifest,
  type DrillHistoryEntry,
  type DriftReport,
  type ManifestVerifyResult,
  type RPOReport,
} from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'DRPanel' })

const drilling = ref(false)
const driftLoading = ref(false)
const rpo = ref<RPOReport | null>(null)
const manifest = ref<BackupManifest | null>(null)
const manifestResult = ref<ManifestVerifyResult | null>(null)
const history = ref<DrillHistoryEntry[]>([])
const auditChain = ref<{ ok: boolean; checked: number; message?: string } | null>(null)
const drift = ref<DriftReport | null>(null)

const rpoColumns = [
  { title: '任务', dataIndex: 'task_key', ellipsis: true },
  { title: '最近备份', dataIndex: 'last_backup_at', width: 180 },
  { title: 'RPO', key: 'rpo', width: 100 },
  { title: '估算 RTO', dataIndex: 'est_rto', width: 100 },
  { title: '份数', dataIndex: 'bundle_count', width: 80 },
  { title: '总大小', key: 'size', width: 100 },
]

const drillColumns = [
  { title: '状态', key: 'status', width: 80 },
  { title: 'Bundle', key: 'bundle', ellipsis: true },
  { title: 'RTO', key: 'rto', width: 100 },
  { title: '时间', key: 'time', width: 170 },
]

const driftColumns = [
  { title: '任务', dataIndex: 'task_key', ellipsis: true },
  { title: '分支', dataIndex: 'branch', width: 120 },
  { title: '状态', key: 'status', width: 160 },
  { title: '本地领先', dataIndex: 'local_ahead', width: 90 },
  { title: '远端领先', dataIndex: 'remote_ahead', width: 90 },
]

function formatSize(n: number) {
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n > 1 << 10) return (n / (1 << 10)).toFixed(1) + ' KB'
  return (n || 0) + ' B'
}

async function runBatchDrill() {
  drilling.value = true
  try {
    const d = await opsApi.runDRDrill(undefined, true, 3)
    notifySuccess(`演练完成: ${d.summary?.success ?? 0}/${d.summary?.total ?? 0} 成功`)
    await loadHistory()
    await loadRpo()
  } catch (e) {
    notifyError(e, '灾备演练失败')
  } finally {
    drilling.value = false
  }
}

async function buildManifest() {
  try {
    manifest.value = await opsApi.buildBackupManifest()
    manifestResult.value = null
    notifySuccess('清单已生成')
  } catch (e) {
    notifyError(e, '生成清单失败')
  }
}

async function verifyManifest() {
  try {
    manifestResult.value = await opsApi.verifyBackupManifest()
    if (manifestResult.value?.ok) notifySuccess('完整性校验通过')
  } catch (e) {
    notifyError(e, '完整性校验失败')
  }
}

async function loadRpo() {
  try {
    rpo.value = await opsApi.rpoReport()
  } catch (e) {
    notifyError(e, '加载 RPO 失败')
  }
}

async function loadHistory() {
  try {
    const d = await opsApi.drillHistory(20)
    history.value = d.items || []
  } catch (e) {
    notifyError(e, '加载演练历史失败')
  }
}

async function verifyAuditChain() {
  try {
    auditChain.value = await opsApi.verifyAuditChain()
  } catch (e) {
    notifyError(e, '审计链校验失败')
  }
}

async function runDrift() {
  driftLoading.value = true
  try {
    drift.value = await opsApi.detectDrift()
    if ((drift.value?.drifted ?? 0) > 0) {
      notifyError(new Error(`${drift.value.drifted} drifted`), `检测到 ${drift.value.drifted} 个任务漂移`)
    } else {
      notifySuccess(`全部 ${drift.value?.checked ?? 0} 个任务一致`)
    }
  } catch (e) {
    notifyError(e, '漂移检测失败')
  } finally {
    driftLoading.value = false
  }
}

async function exportReport(format: 'json' | 'csv') {
  try {
    const blob = await opsApi.exportDrillHistory(format)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `dr-drill-report.${format}`
    a.click()
    URL.revokeObjectURL(url)
    notifySuccess(`已导出 ${format.toUpperCase()}`)
  } catch (e) {
    notifyError(e, '导出演练报告失败')
  }
}

onMounted(() => {
  loadRpo()
  loadHistory()
})
</script>

<style scoped>
.toolbar {
  margin-bottom: 8px;
}
</style>
