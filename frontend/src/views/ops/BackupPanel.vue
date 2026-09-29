<template>
  <div>
    <a-space class="toolbar">
      <a-input v-model:value="taskFilter" placeholder="按 task_key 过滤" style="width: 220px" allow-clear />
      <a-button type="primary" :loading="loading" @click="load">刷新</a-button>
      <a-button @click="metaOpen = true">元数据快照</a-button>
      <a-statistic title="备份数" :value="items.length" />
    </a-space>

    <a-table :data-source="items" :columns="columns" row-key="name" :loading="loading" size="middle">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'size'">{{ formatSize(record.size) }}</template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-button size="small" @click="verify(record.name)">校验</a-button>
            <a-button size="small" type="primary" @click="openRestore(record.name)">恢复</a-button>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-modal v-model:open="restoreOpen" title="从冷备恢复" @ok="doRestore">
      <a-form layout="vertical">
        <a-form-item label="Bundle">{{ restoreName }}</a-form-item>
        <a-form-item label="恢复到目录（须为空）">
          <a-input v-model:value="destDir" placeholder="/tmp/restore-target" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:open="verifyOpen" title="校验结果" :footer="null">
      <pre style="max-height: 320px; overflow: auto">{{ verifyResult }}</pre>
    </a-modal>

    <a-modal v-model:open="metaOpen" title="元数据 / 资产快照" :confirm-loading="metaLoading" @ok="doMetaBackup">
      <a-form layout="vertical">
        <a-form-item label="仓库 key" required>
          <a-input v-model:value="metaRepoKey" placeholder="repo-key" />
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="metaOpts.with_archives">source archive（各 tag 源码包）</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="metaOpts.with_assets">Release 二进制附件（GitHub）</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="metaOpts.with_gists">附带 Gists（GitHub）</a-checkbox>
        </a-form-item>
      </a-form>
      <a-typography-paragraph type="secondary" style="margin: 0">
        快照含 issues/PR/labels/milestones/releases 元数据；附件与 gists 落在冷备 metadata/ 目录。
      </a-typography-paragraph>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { opsApi, type BundleInfo } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'BackupPanel' })

const items = ref<BundleInfo[]>([])
const loading = ref(false)
const taskFilter = ref('')
const restoreOpen = ref(false)
const restoreName = ref('')
const destDir = ref('/tmp/gitferry-restore')
const verifyOpen = ref(false)
const verifyResult = ref('')
const metaOpen = ref(false)
const metaLoading = ref(false)
const metaRepoKey = ref('')
const metaOpts = ref({ with_archives: true, with_assets: true, with_gists: true })

const columns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: '大小', key: 'size', width: 100 },
  { title: '时间', dataIndex: 'mod_time', width: 180 },
  { title: '操作', key: 'action', width: 160 },
]

function formatSize(n: number) {
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  if (n > 1 << 10) return (n / (1 << 10)).toFixed(1) + ' KB'
  return n + ' B'
}

async function load() {
  loading.value = true
  try {
    const d = await opsApi.listBundles(taskFilter.value || undefined)
    items.value = d.items || []
  } catch (e) {
    notifyError(e, '加载冷备列表失败')
  } finally {
    loading.value = false
  }
}

async function verify(name: string) {
  try {
    const info = await opsApi.verifyBundle(name)
    verifyResult.value = JSON.stringify(info, null, 2)
    verifyOpen.value = true
  } catch (e) {
    notifyError(e, '校验失败')
  }
}

function openRestore(name: string) {
  restoreName.value = name
  restoreOpen.value = true
}

async function doRestore() {
  try {
    await opsApi.restoreBundle(restoreName.value, destDir.value)
    notifySuccess('恢复完成')
    restoreOpen.value = false
  } catch (e) {
    message.error((e as Error)?.message || '恢复失败')
  }
}

async function doMetaBackup() {
  if (!metaRepoKey.value) {
    notifyError(new Error('missing repo_key'), '请填写仓库 key')
    return
  }
  metaLoading.value = true
  try {
    const snap = await opsApi.metadataBackup(metaRepoKey.value, metaOpts.value)
    const counts = snap.counts || {}
    notifySuccess(
      `快照完成: issues=${counts.issues || 0} prs=${counts.pull_requests || 0} assets=${counts.release_assets || 0} gists=${counts.gists || 0}`,
    )
    metaOpen.value = false
  } catch (e) {
    notifyError(e, '元数据快照失败')
  } finally {
    metaLoading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; gap: 16px; display: flex; align-items: center; }
</style>
