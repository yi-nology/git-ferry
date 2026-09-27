<template>
  <div class="platform-settings">
    <PageHeader title="平台管理" subtitle="管理 Git 托管平台凭据与连接" />

    <a-space class="toolbar">
      <a-button type="primary" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        添加平台
      </a-button>
      <a-button :loading="loading" @click="loadPlatforms">刷新</a-button>
    </a-space>

    <a-row :gutter="[16, 16]">
      <a-col v-for="p in platforms" :key="p.key" :xs="24" :sm="12" :lg="8">
        <a-card hoverable>
          <template #cover>
            <div class="cover" :style="{ background: platformColor(p.type) }">
              <component :is="platformIcon(p.type)" class="cover-icon" />
              <a-tag v-if="getIsDefault(p)" color="white" class="default-tag">默认</a-tag>
            </div>
          </template>
          <a-card-meta :title="p.name">
            <template #description>
              <div class="meta">
                <div>{{ p.type }}</div>
                <div class="url">{{ getInstanceUrl(p) || getApiUrl(p) }}</div>
                <div>
                  <a-tag :color="p.status === 'active' ? 'green' : 'red'">
                    {{ p.status === 'active' ? '正常' : '异常' }}
                  </a-tag>
                  <span class="muted">仓库 {{ getRepoCount(p) }}</span>
                </div>
              </div>
            </template>
          </a-card-meta>
          <template #actions>
            <a-tooltip title="测试连接">
              <a-button type="text" @click="testConnection(p)"><ApiOutlined /></a-button>
            </a-tooltip>
            <a-tooltip title="同步仓库">
              <a-button type="text" @click="syncRepos(p)"><SyncOutlined /></a-button>
            </a-tooltip>
            <a-tooltip title="编辑">
              <a-button type="text" @click="openEdit(p)"><EditOutlined /></a-button>
            </a-tooltip>
            <a-popconfirm title="确认删除该平台?" @confirm="handleDelete(p.key)">
              <a-button type="text" danger><DeleteOutlined /></a-button>
            </a-popconfirm>
          </template>
        </a-card>
      </a-col>
    </a-row>

    <a-empty v-if="!loading && !platforms.length" description="暂无平台,点击右上角添加" />

    <PlatformFormModal v-model:open="dialogVisible" :platform="editing" @saved="loadPlatforms" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  PlusOutlined, ApiOutlined, EditOutlined, DeleteOutlined, SyncOutlined,
} from '@ant-design/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import PlatformFormModal from './PlatformFormModal.vue'
import { platformApi, type Platform } from '@/api/platform'
import {
  platformColor, platformIcon,
  getInstanceUrl, getApiUrl, getIsDefault, getRepoCount,
} from '@/utils/platformDisplay'

defineOptions({ name: 'PlatformSettings' })

const platforms = ref<Platform[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref<Platform | null>(null)

async function loadPlatforms() {
  loading.value = true
  try {
    const result = await platformApi.list()
    platforms.value = result.platforms || []
  } catch {
    message.error('加载平台列表失败')
    platforms.value = []
  } finally {
    loading.value = false
  }
}

function openAdd() {
  editing.value = null
  dialogVisible.value = true
}

function openEdit(p: Platform) {
  editing.value = p
  dialogVisible.value = true
}

async function handleDelete(key: string) {
  try {
    await platformApi.delete(key)
    message.success('删除成功')
    await loadPlatforms()
  } catch (e) {
    message.error((e as Error)?.message || '删除失败')
  }
}

async function testConnection(p: Platform) {
  message.loading({ content: '正在测试连接...', key: 'test' })
  try {
    const result = await platformApi.test(p.key)
    if (result.result?.connected ?? result.result?.success) {
      message.success({ content: '连接成功', key: 'test' })
    } else {
      message.error({ content: result.result?.message || '连接失败', key: 'test' })
    }
    await loadPlatforms()
  } catch (e) {
    message.error({ content: (e as Error)?.message || '测试失败', key: 'test' })
  }
}

async function syncRepos(p: Platform) {
  message.loading({ content: '正在同步仓库...', key: 'sync' })
  try {
    const result = await platformApi.syncRepos(p.key)
    message.success({ content: `同步成功，共 ${result.synced_count || 0} 个仓库`, key: 'sync' })
    await loadPlatforms()
  } catch (e) {
    message.error({ content: (e as Error)?.message || '同步失败', key: 'sync' })
  }
}

onMounted(loadPlatforms)
</script>

<style scoped lang="scss">
.platform-settings { padding: 16px 24px; }
.toolbar { margin-bottom: 16px; }
.cover {
  height: 100px; display: flex; align-items: center; justify-content: center;
  position: relative;
}
.cover-icon { font-size: 40px; color: #fff; }
.default-tag { position: absolute; right: 8px; top: 8px; }
.meta { font-size: 12px; color: #666; }
.url { margin: 4px 0; word-break: break-all; }
.muted { margin-left: 8px; color: #999; }
</style>
