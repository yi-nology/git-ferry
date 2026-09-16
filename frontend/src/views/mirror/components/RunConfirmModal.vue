<template>
  <a-modal
    :open="open"
    title="发布确认"
    :confirm-loading="executing"
    ok-text="确认发布"
    cancel-text="取消"
    @ok="submit"
    @cancel="emit('update:open', false)"
  >
    <a-spin :spinning="previewing">
      <a-alert
        v-if="previewError"
        type="error"
        show-icon
        :message="'预检失败,不能发布'"
        :description="previewError"
        class="block"
      />

      <template v-else-if="previews.length">
        <div class="mapping" v-for="p in previews" :key="p.tag">
          <code>{{ sourceModule }}</code>
          <ArrowRightOutlined class="arrow" />
          <code>{{ targetModule }}</code>
          <a-tag>{{ p.tag }}</a-tag>
        </div>

        <a-descriptions size="small" :column="1" bordered class="block">
          <a-descriptions-item label="改写文件数">
            {{ previews.reduce((s, p) => s + (p.replacedFiles?.length || 0), 0) }} 个
            <a-typography-text type="secondary">(go.mod、import 前缀、文档引用等)</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="合成方式">
            孤儿快照 commit(继承源 tag 作者与时间,重跑幂等)
          </a-descriptions-item>
          <a-descriptions-item v-for="p in previews" :key="'st-' + p.tag" :label="`远端 ${p.tag}`">
            <template v-if="!p.remoteTagExisted">
              <a-tag color="green">无同名 tag,全新发布</a-tag>
            </template>
            <template v-else-if="p.treeMatchedRemote">
              <a-tag color="blue">内容一致(幂等重跑,无副作用)</a-tag>
            </template>
            <template v-else>
              <a-tag color="red">内容不一致</a-tag>
            </template>
          </a-descriptions-item>
        </a-descriptions>

        <a-collapse class="block">
          <a-collapse-panel header="查看将被改写的文件清单">
            <ul class="files">
              <li v-for="f in allReplacedFiles" :key="f"><code>{{ f }}</code></li>
            </ul>
          </a-collapse-panel>
        </a-collapse>

        <a-alert
          v-if="hasDivergent"
          type="warning"
          show-icon
          message="远端已存在内容不一致的同名 tag"
          class="block"
        >
          <template #description>
            覆盖将替换已发布版本(内容以本次合成结果为准)。请确认双方 tree:
            <div class="trees">
              <div>远端现有:<code>{{ divergentTree || '—' }}</code></div>
              <div>本次合成:<code>{{ previews.find((p) => !p.treeMatchedRemote)?.tree || '—' }}</code></div>
            </div>
            <a-checkbox v-model:checked="allowOverwrite">我确认覆盖远端已发布的版本</a-checkbox>
          </template>
        </a-alert>
      </template>
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRightOutlined } from '@ant-design/icons-vue'
import type { MirrorPreviewReport } from '@/api/mirror'

const props = defineProps<{
  open: boolean
  sourceModule: string
  targetModule: string
  previews: MirrorPreviewReport[]
  previewing: boolean
  previewError: string
  executing: boolean
}>()

const emit = defineEmits<{
  'update:open': [boolean]
  confirm: [{ allowOverwrite: boolean }]
}>()

const allowOverwrite = ref(false)

watch(
  () => props.open,
  (o) => {
    if (o) allowOverwrite.value = false
  },
)

const hasDivergent = computed(() => props.previews.some((p) => p.remoteTagExisted && !p.treeMatchedRemote))
const divergentTree = computed(() => '见版本矩阵(远端 tree 未比对到本地)')
const allReplacedFiles = computed(() => {
  const set = new Set<string>()
  props.previews.forEach((p) => (p.replacedFiles || []).forEach((f) => set.add(f)))
  return [...set].sort()
})

function submit() {
  if (hasDivergent.value && !allowOverwrite.value) return
  emit('confirm', { allowOverwrite: allowOverwrite.value })
}
</script>

<style scoped lang="scss">
.block {
  margin-top: 12px;
}
.mapping {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0;
  flex-wrap: wrap;
  code {
    font-size: 12px;
    word-break: break-all;
  }
  .arrow {
    color: #1677ff;
  }
}
.files {
  margin: 0;
  padding-left: 18px;
  max-height: 200px;
  overflow: auto;
}
.trees {
  margin: 8px 0;
  font-size: 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
</style>
