<template>
  <div class="page-header">
    <div class="page-header-text">
      <h1 class="page-header-title">{{ title }}</h1>
      <p v-if="subtitle" class="page-header-subtitle">{{ subtitle }}</p>
    </div>
    <div class="page-header-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: $spacing-lg;
  gap: $spacing-md;
}

.page-header-title {
  font-size: $fs-xl;
  font-weight: 600;
  color: $text-primary;
  margin: 0;
  line-height: 1.3;
  letter-spacing: -0.2px;
}

.page-header-subtitle {
  font-size: $fs-body;
  color: $text-secondary;
  margin: 4px 0 0 0;
}

.page-header-actions {
  display: flex;
  gap: $spacing-sm;
  align-items: center;
  flex-shrink: 0;
}
</style>
