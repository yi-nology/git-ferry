<template>
  <div class="mirror-list">
    <div class="page-header">
      <h2>镜像中心</h2>
      <p class="subtitle">开源公开发布与仓库备份:把源仓库以独立快照推送到镜像远端</p>
    </div>

    <div class="toolbar">
      <a-radio-group v-model:value="modeFilter" button-style="solid" size="small">
        <a-radio-button value="">全部</a-radio-button>
        <a-radio-button value="publish">开源发布</a-radio-button>
        <a-radio-button value="backup">仓库备份</a-radio-button>
      </a-radio-group>
      <a-button type="primary" @click="showCreate = true">
        <template #icon><PlusOutlined /></template>
        新建通道
      </a-button>
    </div>

    <a-spin :spinning="isLoading">
      <a-empty v-if="!isLoading && channels.length === 0" description="还没有镜像通道">
        <a-button type="primary" @click="showCreate = true">创建第一个通道</a-button>
      </a-empty>

      <a-row :gutter="16">
        <a-col v-for="ch in channels" :key="ch.id" :xs="24" :md="12" :xl="8">
          <a-card class="channel-card" hoverable @click="goDetail(ch.id)">
            <template #title>
              <span class="card-title">
                <a-tag :color="ch.mode === 'publish' ? 'blue' : 'purple'">
                  {{ ch.mode === 'publish' ? '🚀 开源发布' : '🗄 仓库备份' }}
                </a-tag>
                {{ ch.name }}
              </span>
            </template>
            <p class="line"><span class="label">源仓库</span>{{ ch.repoKey }}</p>
            <p v-if="ch.module" class="line module">{{ ch.module }}</p>
            <div v-if="ch.targets?.length" class="targets">
              <div v-for="t in ch.targets" :key="t.id" class="target-line">
                <ArrowRightOutlined class="arrow" />
                <span class="target-module">{{ t.targetModule || t.repoUrl }}</span>
              </div>
            </div>
          </a-card>
        </a-col>
      </a-row>
    </a-spin>

    <ChannelFormDrawer v-model:open="showCreate" @created="onCreated" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { PlusOutlined, ArrowRightOutlined } from '@ant-design/icons-vue'
import { useMirrorChannelsQuery } from '@/composables/useMirror'
import type { MirrorChannel } from '@/api/mirror'
import ChannelFormDrawer from './components/ChannelFormDrawer.vue'

const router = useRouter()
const modeFilter = ref('')
const showCreate = ref(false)

const { data, isLoading } = useMirrorChannelsQuery()
const channels = computed(() =>
  (data.value || []).filter((ch: MirrorChannel) => !modeFilter.value || ch.mode === modeFilter.value),
)

function goDetail(id: number) {
  router.push(`/mirror/${id}`)
}

function onCreated(ch: MirrorChannel) {
  showCreate.value = false
  message.success(`通道「${ch.name}」创建成功`)
  router.push(`/mirror/${ch.id}`)
}
</script>

<style scoped lang="scss">
.mirror-list {
  padding: 20px;
}
.page-header {
  margin-bottom: 16px;
  h2 {
    margin: 0 0 4px;
    font-size: 20px;
  }
  .subtitle {
    margin: 0;
    color: rgba(0, 0, 0, 0.45);
    font-size: 13px;
  }
}
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.channel-card {
  margin-bottom: 16px;
  .card-title {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  .line {
    margin: 0 0 6px;
    font-size: 13px;
    .label {
      display: inline-block;
      width: 56px;
      color: rgba(0, 0, 0, 0.45);
    }
    &.module {
      font-family: monospace;
      color: rgba(0, 0, 0, 0.65);
      word-break: break-all;
    }
  }
  .targets {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px dashed rgba(0, 0, 0, 0.06);
    .target-line {
      display: flex;
      align-items: center;
      gap: 6px;
      font-family: monospace;
      font-size: 12px;
      color: rgba(0, 0, 0, 0.65);
      word-break: break-all;
      .arrow {
        color: #1677ff;
        flex-shrink: 0;
      }
    }
  }
}
</style>
