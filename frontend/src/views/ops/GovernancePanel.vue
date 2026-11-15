<template>
  <div class="gov-panel">
    <!-- ===== 对账 ===== -->
    <a-card size="small" class="gov-card">
      <template #title>
        <span>跨平台对账</span>
        <a-typography-text type="secondary" class="card-hint">
          纯平台 API 只读:分支 HEAD 一致 / 漂移量化 / 两端缺失
        </a-typography-text>
      </template>
      <a-space wrap>
        <a-input
          v-model:value="reconcileTaskKey"
          placeholder="任务 key"
          style="width: 260px"
          @press-enter="doReconcile"
        />
        <a-button type="primary" :loading="reconcileLoading" @click="doReconcile">查询对账</a-button>
        <a-button :loading="resultsLoading" @click="loadResults">漂移列表</a-button>
      </a-space>

      <div v-if="reconcileResult" class="result-block">
        <a-tag :color="statusColor(reconcileResult.status)">{{ statusText(reconcileResult.status) }}</a-tag>
        <span class="mono">{{ reconcileResult.source_branch }} → {{ reconcileResult.target_branch }}</span>
        <span class="mono dim">src {{ shortSha(reconcileResult.source_sha) }}</span>
        <span class="mono dim">tgt {{ shortSha(reconcileResult.target_sha) }}</span>
        <span v-if="reconcileResult.source_ahead" class="ahead">
          目标缺 {{ reconcileResult.source_ahead }} 个提交
        </span>
        <span v-if="reconcileResult.target_ahead" class="ahead warn">
          目标多出 {{ reconcileResult.target_ahead }} 个提交(旁路写入?)
        </span>
        <div class="dim">{{ reconcileResult.message }}</div>
      </div>

      <a-table
        v-if="driftItems.length"
        :data-source="driftItems"
        :columns="driftColumns"
        :loading="resultsLoading"
        row-key="task_key"
        :pagination="{ pageSize: 8 }"
        size="small"
        class="gov-table"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'ahead'">
            <span v-if="record.source_ahead">缺 {{ record.source_ahead }}</span>
            <span v-if="record.target_ahead" class="ahead warn">多 {{ record.target_ahead }}</span>
            <span v-if="!record.source_ahead && !record.target_ahead" class="dim">—</span>
          </template>
          <template v-else-if="column.key === 'sha'">
            <span class="mono dim">{{ shortSha(record.source_sha) }} / {{ shortSha(record.target_sha) }}</span>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- ===== 恢复点 ===== -->
    <a-card size="small" class="gov-card">
      <template #title>
        <span>恢复点索引</span>
        <a-typography-text type="secondary" class="card-hint">
          一次成功冷备 = 一个可检索恢复点(文件 + 源 HEAD 状态锚)
        </a-typography-text>
      </template>
      <a-space wrap>
        <a-input
          v-model:value="recoveryTaskKey"
          placeholder="任务 key"
          style="width: 260px"
          @press-enter="loadRecovery"
        />
        <a-button type="primary" :loading="recoveryLoading" @click="loadRecovery">查询恢复点</a-button>
      </a-space>
      <a-table
        v-if="recoveryItems.length"
        :data-source="recoveryItems"
        :columns="recoveryColumns"
        :loading="recoveryLoading"
        row-key="id"
        :pagination="{ pageSize: 8 }"
        size="small"
        class="gov-table"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'sha'">
            <span class="mono">{{ shortSha(record.source_sha) }}</span>
          </template>
          <template v-else-if="column.key === 'size'">
            {{ formatSize(record.size_bytes) }}
          </template>
        </template>
      </a-table>
      <a-empty v-else-if="recoveryLoaded" description="该任务暂无恢复点(或文件已被轮转清理)" />
    </a-card>

    <!-- ===== 策略即代码 ===== -->
    <a-card size="small" class="gov-card">
      <template #title>
        <span>策略即代码</span>
        <a-typography-text type="secondary" class="card-hint">
          导出确定性 JSON 进 git 走 PR;回放 = 新建/更新任务策略(部分失败完整报告)
        </a-typography-text>
      </template>
      <a-space wrap>
        <a-button :loading="exportLoading" @click="doExport">导出策略 JSON</a-button>
        <a-button @click="fileInput?.click()">从文件回放…</a-button>
        <a-button
          type="primary"
          danger
          :loading="applyLoading"
          :disabled="!policyDoc"
          @click="doApply"
        >
          应用当前文档
        </a-button>
        <a-button v-if="policyDoc" @click="policyDoc = ''">清空</a-button>
      </a-space>
      <input
        ref="fileInput"
        type="file"
        accept=".json,application/json"
        style="display: none"
        @change="onFile"
      />
      <a-textarea
        v-model:value="policyDoc"
        :rows="10"
        class="policy-doc"
        placeholder='策略文件内容(与导出同 schema),或点击「从文件回放」选择'
      />
      <div v-if="applyResult" class="result-block">
        <a-tag color="blue">新建 {{ applyResult.created }}</a-tag>
        <a-tag color="green">更新 {{ applyResult.updated }}</a-tag>
        <template v-if="applyResult.errors?.length">
          <a-tag color="red">失败 {{ applyResult.errors.length }}</a-tag>
          <ul class="err-list">
            <li v-for="(e, i) in applyResult.errors" :key="i">{{ e }}</li>
          </ul>
        </template>
      </div>
    </a-card>

    <!-- ===== 元数据迁移 ===== -->
    <a-card size="small" class="gov-card">
      <template #title>
        <span>元数据迁移(Issue/PR)</span>
        <a-typography-text type="secondary" class="card-hint">
          幂等收敛:目标已有同标题不重复创建;dry-run 先盘点再执行;评论仅对新建重放
        </a-typography-text>
      </template>
      <a-space wrap>
        <a-input v-model:value="migTaskKey" placeholder="任务 key" style="width: 260px" />
        <a-checkbox v-model:checked="migIssues">Issue</a-checkbox>
        <a-checkbox v-model:checked="migComments">评论</a-checkbox>
        <a-checkbox v-model:checked="migPRs">PR/MR</a-checkbox>
        <a-button :loading="migLoading" @click="doMigrate(true)">dry-run 盘点</a-button>
        <a-button type="primary" danger :loading="migLoading" @click="doMigrate(false)">
          执行迁移
        </a-button>
      </a-space>
      <div v-if="migReport" class="result-block">
        <a-tag v-if="migReport.dryRun" color="blue">dry-run</a-tag>
        <a-tag color="green">新建 Issue {{ migReport.createdIssues }}</a-tag>
        <a-tag v-if="migReport.updatedIssues" color="orange">收敛更新 {{ migReport.updatedIssues }}</a-tag>
        <a-tag v-if="migReport.unchangedIssues" color="default">已达标 {{ migReport.unchangedIssues }}</a-tag>
        <a-tag v-if="migReport.createdComments">评论 {{ migReport.createdComments }}</a-tag>
        <a-tag v-if="migReport.createdPrs">PR {{ migReport.createdPrs }}</a-tag>
        <a-tag v-if="migReport.errors?.length" color="red">失败 {{ migReport.errors.length }}</a-tag>
        <ul v-if="migReport.errors?.length" class="err-list">
          <li v-for="(e, i) in migReport.errors" :key="i">{{ e }}</li>
        </ul>
        <ul v-if="migReport.dryRunPlanned?.length" class="plan-list">
          <li v-for="(pl, i) in migReport.dryRunPlanned" :key="i">{{ pl }}</li>
        </ul>
      </div>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  governanceApi,
  metadataApi,
  type ReconcileResult,
  type ReconcileRecord,
  type RecoveryPoint,
  type ApplyPoliciesResult,
  type MetadataMigrateReport,
} from '@/api/ops'

// ===== 对账 =====
const reconcileTaskKey = ref('')
const reconcileLoading = ref(false)
const reconcileResult = ref<ReconcileResult | null>(null)
const resultsLoading = ref(false)
const driftItems = ref<ReconcileRecord[]>([])
const driftColumns = [
  { title: '任务', dataIndex: 'task_key', key: 'task_key', width: 200 },
  { title: '状态', key: 'status', width: 110 },
  { title: '两端 SHA', key: 'sha', width: 170 },
  { title: '量化', key: 'ahead', width: 140 },
  { title: '说明', dataIndex: 'message', key: 'message', ellipsis: true },
]

async function doReconcile() {
  const key = reconcileTaskKey.value.trim()
  if (!key) {
    message.warning('请输入任务 key')
    return
  }
  reconcileLoading.value = true
  try {
    reconcileResult.value = await governanceApi.reconcile(key)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '对账查询失败')
  } finally {
    reconcileLoading.value = false
  }
}

async function loadResults() {
  resultsLoading.value = true
  try {
    const resp = await governanceApi.reconcileResults(false)
    driftItems.value = resp.items || []
    if (!driftItems.value.length) message.success('无漂移:全部对账通过')
  } catch (e) {
    message.error(e instanceof Error ? e.message : '漂移列表加载失败')
  } finally {
    resultsLoading.value = false
  }
}

// ===== 恢复点 =====
const recoveryTaskKey = ref('')
const recoveryLoading = ref(false)
const recoveryLoaded = ref(false)
const recoveryItems = ref<RecoveryPoint[]>([])
const recoveryColumns = [
  { title: '文件', dataIndex: 'file_name', key: 'file_name', ellipsis: true },
  { title: '格式', dataIndex: 'format', key: 'format', width: 80 },
  { title: '源 HEAD', key: 'sha', width: 110 },
  { title: '大小', key: 'size', width: 100 },
  { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 170 },
]

async function loadRecovery() {
  const key = recoveryTaskKey.value.trim()
  if (!key) {
    message.warning('请输入任务 key')
    return
  }
  recoveryLoading.value = true
  recoveryLoaded.value = true
  try {
    const resp = await governanceApi.recoveryPoints(key)
    recoveryItems.value = resp.items || []
  } catch (e) {
    message.error(e instanceof Error ? e.message : '恢复点查询失败')
  } finally {
    recoveryLoading.value = false
  }
}

// ===== 策略 =====
const policyDoc = ref('')
const exportLoading = ref(false)
const applyLoading = ref(false)
const applyResult = ref<ApplyPoliciesResult | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

async function doExport() {
  exportLoading.value = true
  try {
    policyDoc.value = await governanceApi.exportPolicies()
    message.success('策略已导出(可复制保存进 git)')
  } catch (e) {
    message.error(e instanceof Error ? e.message : '导出失败')
  } finally {
    exportLoading.value = false
  }
}

function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    policyDoc.value = String(reader.result || '')
    applyResult.value = null
  }
  reader.readAsText(file)
  input.value = ''
}

async function doApply() {
  if (!policyDoc.value.trim()) return
  applyLoading.value = true
  applyResult.value = null
  try {
    applyResult.value = await governanceApi.applyPolicies(policyDoc.value)
    const r = applyResult.value
    if (r.errors?.length) {
      message.warning(`回放完成:新建 ${r.created} / 更新 ${r.updated},失败 ${r.errors.length}`)
    } else {
      message.success(`回放完成:新建 ${r.created} / 更新 ${r.updated}`)
    }
  } catch (e) {
    message.error(e instanceof Error ? e.message : '回放失败')
  } finally {
    applyLoading.value = false
  }
}

// ===== 元数据迁移 =====
const migTaskKey = ref('')
const migIssues = ref(true)
const migComments = ref(true)
const migPRs = ref(false)
const migLoading = ref(false)
const migReport = ref<MetadataMigrateReport | null>(null)

async function doMigrate(dryRun: boolean) {
  const key = migTaskKey.value.trim()
  if (!key) {
    message.warning('请输入任务 key')
    return
  }
  if (!migIssues.value && !migPRs.value) {
    message.warning('至少选择 Issue 或 PR')
    return
  }
  if (!dryRun) {
    migReport.value = null
  }
  migLoading.value = true
  try {
    migReport.value = await metadataApi.migrate({
      task_key: key,
      issues: migIssues.value,
      comments: migComments.value,
      prs: migPRs.value,
      dry_run: dryRun,
    })
    const r = migReport.value
    if (dryRun) {
      message.success(`盘点完成:源 Issue ${r.sourceIssues} / PR ${r.sourcePrs}`)
    } else if (r.errors?.length) {
      message.warning(`迁移完成,失败 ${r.errors.length} 条(见下方)`)
    } else {
      message.success('迁移完成')
    }
  } catch (e) {
    message.error(e instanceof Error ? e.message : '迁移失败')
  } finally {
    migLoading.value = false
  }
}

// ===== 展示辅助 =====
function statusColor(s: string) {
  return s === 'in_sync' ? 'green' : s === 'drift' ? 'red' : 'orange'
}
function statusText(s: string) {
  const m: Record<string, string> = {
    in_sync: '一致',
    drift: '漂移',
    source_missing: '源分支缺失',
    target_missing: '目标分支缺失',
  }
  return m[s] || s
}
function shortSha(sha: string) {
  return sha ? sha.slice(0, 8) : '—'
}
function formatSize(n: number) {
  if (!n) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
</script>

<style scoped>
.gov-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.gov-card :deep(.ant-card-head-title) {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.card-hint {
  font-size: 12px;
  font-weight: normal;
}
.result-block {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.gov-table {
  margin-top: 10px;
}
.policy-doc {
  margin-top: 10px;
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.mono {
  font-family: ui-monospace, monospace;
}
.dim {
  color: var(--ant-color-text-tertiary, #999);
}
.ahead {
  color: var(--ant-color-error, #cf1322);
}
.ahead.warn {
  color: #d46b08;
}
.err-list {
  margin: 4px 0 0;
  padding-left: 18px;
  color: var(--ant-color-error, #cf1322);
  font-size: 12px;
}
</style>
