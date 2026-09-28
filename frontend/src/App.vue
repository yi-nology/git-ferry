<template>
  <a-config-provider :theme="antdTheme" :locale="zhCN">
    <a-app>
      <AppRegistrar />
      <router-view />
      <AssistantPanel />
    </a-app>
  </a-config-provider>
</template>

<script setup lang="ts">
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import AppRegistrar from '@/components/AppRegistrar.vue'
import AssistantPanel from '@/components/ai/AssistantPanel.vue'
import { antdTheme } from '@/styles/theme'
</script>
