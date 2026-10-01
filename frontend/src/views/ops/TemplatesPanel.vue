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
        <template v-else-if="column.key === 'extends'">
          <a-tag v-if="record.extends" color="blue">→ {{ record.extends }}</a-tag>
          <a-typography-text v-else type="secondary">—</a-typography-text>
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
        <a-form-item label="继承基础模板">
          <a-select
            v-model:value="form.extends"
            allow-clear
            placeholder="可选：从已有模板继承 Spec"
            :options="inheritOptions"
          />
          <div class="form-tip">子模板非空字段覆盖父模板（Renovate preset 模式）</div>
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

    <a-modal v-model:open="applyOpen" title="套用结果" :footer="null">
      <div v-if="applyResult">
        <div v-if="applyResult.extends_chain?.length" class="chain-row">
          继承链：
          <a-tag v-for="(id, i) in applyResult.extends_chain" :key="id" :color="i === 0 ? 'blue' : 'default'">
            {{ id }}
          </a-tag>
        </div>
        <div v-if="applyResult.effective_spec" class="chain-row">
          生效 Spec：
          <code>{{ JSON.stringify(applyResult.effective_spec) }}</code>
        </div>
        <a-typography-text>
          {{ applyResult.dry_run ? 'Dry-run' : '已写库' }}：变更 {{ applyResult.total }} 条
        </a-typography-text>
        <a-list :data-source="applyResult.changed" size="small" :pagination="{ pageSize: 5 }">
          <template #renderItem="{ item }">
            <a-list-item>
              {{ item.name }}
              <a-typography-text type="secondary">
                {{ JSON.stringify(item.before) }} → {{ JSON.stringify(item.after) }}
              </a-typography-text>
            </a-list-item>
          </template>
        </a-list>
      </div>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Modal } from 'ant-design-vue'
import { opsApi, type SyncTemplate, type TemplateApplyResult } from '@/api/ops'
import { notifyError, notifySuccess } from '@/utils/notify'

defineOptions({ name: 'TemplatesPanel' })

const items = ref<SyncTemplate[]>([])
const loading = ref(false)
const createOpen = ref(false)
const previewOpen = ref(false)
const applyOpen = ref(false)
const previewResult = ref<{ matched: Array<{ key: string; name: string }>; total: number }>({ matched: [], total: 0 })
const applyResult = ref<TemplateApplyResult | null>(null)

const form = reactive<{ name: string; extends?: string; spec: SyncTemplate['spec'] }>({
  name: '',
  extends: undefined,
  spec: { cron: '' },
})
const includeGlobs = ref('')
const exclude = ref('')
const tags = ref('')

const columns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: 'ID', dataIndex: 'id', width: 160, ellipsis: true },
  { title: '继承', key: 'extends', width: 140 },
  { title: 'Cron', key: 'cron', width: 120 },
  { title: '标签', key: 'tags' },
  { title: '操作', key: 'action', width: 280 },
]

const inheritOptions = computed(() =>
  items.value.map((t) => ({ label: `${t.name} (${t.id})`, value: t.id })),
)

function openCreate() {
  form.name = ''
  form.extends = undefined
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
      extends: form.extends || undefined,
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
        applyResult.value = r
        applyOpen.value = true
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
.form-tip {
  color: #9ca3af;
  font-size: 12px;
  margin-top: 4px;
}
.chain-row {
  margin-bottom: 8px;
  font-size: 12px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
</style>
