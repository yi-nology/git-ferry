<template>
  <div>
    <a-space class="toolbar">
      <a-button type="primary" @click="openCreate">新建模板</a-button>
      <a-button :loading="loading" @click="load">刷新</a-button>
      <a-typography-text type="secondary">
        套用前可预览命中任务;dry-run 不落库
      </a-typography-text>
    </a-space>

    <a-table
      :data-source="items"
      :columns="columns"
      :loading="loading"
      row-key="id"
      size="middle"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'tags'">
          <a-tag v-for="t in record.tags || []" :key="t">{{ t }}</a-tag>
        </template>
        <template v-else-if="column.key === 'cron'">
          <code>{{ record.spec?.cron || '—' }}</code>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-button size="small" @click="preview(record)">预览</a-button>
            <a-button size="small" type="primary" @click="apply(record, true)">Dry-run</a-button>
            <a-button size="small" type="primary" danger @click="apply(record, false)">套用</a-button>
            <a-button size="small" danger @click="remove(record)">删除</a-button>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-modal v-model:open="createOpen" title="新建策略模板" @ok="submitCreate">
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="form.name" placeholder="如 nightly-backup" />
        </a-form-item>
        <a-form-item label="Cron">
          <a-input v-model:value="form.spec.cron" placeholder="0 2 * * *" />
        </a-form-item>
        <a-form-item label="Include Globs(逗号分隔)">
          <a-input v-model:value="includeGlobs" placeholder="team-*,app-*" />
        </a-form-item>
        <a-form-item label="Exclude(逗号分隔)">
          <a-input v-model:value="exclude" placeholder="team-secret" />
        </a-form-item>
        <a-form-item label="标签(逗号分隔)">
          <a-input v-model:value="tags" placeholder="backup,nightly" />
        </a-form-item>
      </a-form>
    </a-modal>

    <a-modal v-model:open="previewOpen" :title="`预览: ${previewResult.total} 条命中`" :footer="null">
      <a-list :data-source="previewResult.matched" size="small">
        <template #renderItem="{ item }">
          <a-list-item>{{ item.name }} ({{ item.key }})</a-list-item>
        </template>
      </a-list>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Modal } from 'ant-design-vue'
import { opsApi, type SyncTemplate } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'TemplatesPanel' })

const items = ref<SyncTemplate[]>([])
const loading = ref(false)
const createOpen = ref(false)
const previewOpen = ref(false)
const previewResult = ref<{ matched: Array<{ key: string; name: string }>; total: number }>({ matched: [], total: 0 })

const form = reactive<{ name: string; spec: SyncTemplate['spec'] }>({
  name: '',
  spec: { cron: '' },
})
const includeGlobs = ref('')
const exclude = ref('')
const tags = ref('')

const columns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: 'ID', dataIndex: 'id', width: 180, ellipsis: true },
  { title: 'Cron', key: 'cron', width: 130 },
  { title: '标签', key: 'tags' },
  { title: '操作', key: 'action', width: 280 },
]

function openCreate() {
  form.name = ''
  form.spec = { cron: '' }
  includeGlobs.value = ''
  exclude.value = ''
  tags.value = ''
  createOpen.value = true
}

async function submitCreate() {
  try {
    const match: Record<string, string[]> = {}
    if (includeGlobs.value) match.include_globs = includeGlobs.value.split(',').map((s) => s.trim()).filter(Boolean)
    if (exclude.value) match.exclude = exclude.value.split(',').map((s) => s.trim()).filter(Boolean)
    await opsApi.createTemplate({
      name: form.name,
      spec: form.spec,
      match: Object.keys(match).length ? match : undefined,
      tags: tags.value ? tags.value.split(',').map((s) => s.trim()).filter(Boolean) : undefined,
    })
    notifySuccess('模板已创建')
    createOpen.value = false
    await load()
  } catch (e) {
    notifyError(e, '创建模板失败')
  }
}

async function preview(t: SyncTemplate) {
  try {
    previewResult.value = await opsApi.previewTemplate(t.id)
    previewOpen.value = true
  } catch (e) {
    notifyError(e, '预览失败')
  }
}

async function apply(t: SyncTemplate, dryRun: boolean) {
  Modal.confirm({
    title: dryRun ? 'Dry-run 套用?' : '确认套用模板?',
    content: dryRun
      ? '仅预览将变更的任务,不写库'
      : `将对命中任务批量更新 cron/启用位: ${t.name}`,
    onOk: async () => {
      try {
        const r = await opsApi.applyTemplate(t.id, dryRun)
        notifySuccess(`${dryRun ? 'Dry-run' : '套用'}完成: ${r.total} 条`)
        if (!dryRun) await load()
      } catch (e) {
        notifyError(e, '套用失败')
      }
    },
  })
}

async function remove(t: SyncTemplate) {
  Modal.confirm({
    title: '删除模板?',
    content: t.name,
    onOk: async () => {
      try {
        await opsApi.deleteTemplate(t.id)
        notifySuccess('已删除')
        await load()
      } catch (e) {
        notifyError(e, '删除失败')
      }
    },
  })
}

async function load() {
  loading.value = true
  try {
    const data = await opsApi.listTemplates()
    items.value = data.items || []
  } catch (e) {
    notifyError(e, '加载模板失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; }
</style>
