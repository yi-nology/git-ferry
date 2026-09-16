<template>
  <a-app>
    <AppRegistrar />
    <router-view />
    <AssistantPanel />
  </a-app>
</template>

<script setup lang="ts">
import AppRegistrar from '@/components/AppRegistrar.vue'
import AssistantPanel from '@/components/ai/AssistantPanel.vue'
</script>
