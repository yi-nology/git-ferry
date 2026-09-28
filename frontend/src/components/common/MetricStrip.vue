<template>
  <div class="metric-strip">
    <div
      v-for="item in items"
      :key="item.label"
      class="metric-item"
      :class="{ clickable: !!item.path || !!onClick }"
      role="button"
      :tabindex="item.path || onClick ? 0 : undefined"
      @click="handleClick(item)"
      @keydown.enter="handleClick(item)"
    >
      <div class="metric-label">{{ item.label }}</div>
      <div class="metric-value" :class="item.tone">{{ item.value }}</div>
      <div v-if="item.hint" class="metric-hint">{{ item.hint }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

export interface MetricItem {
  label: string
  value: string | number
  hint?: string
  tone?: 'default' | 'success' | 'warning' | 'danger' | 'info'
  path?: string
}

const props = defineProps<{
  items: MetricItem[]
  onClick?: (item: MetricItem) => void
}>()

const router = useRouter()

function handleClick(item: MetricItem) {
  if (props.onClick) {
    props.onClick(item)
    return
  }
  if (item.path) router.push(item.path)
}
</script>

<style scoped lang="scss">
@use '@/styles/variables.scss' as *;

.metric-strip {
  display: grid;
  gap: $spacing-md;
  margin-bottom: $spacing-lg;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
}

.metric-item {
  background: $bg-primary;
  border: 1px solid $border-light;
  border-radius: $radius-lg;
  padding: 14px 16px;
  transition: border-color $duration-fast $ease;

  &.clickable {
    cursor: pointer;

    &:hover {
      border-color: $primary-border;
      background: #fcfdff;
    }
  }
}

.metric-label {
  font-size: $fs-caption;
  font-weight: 500;
  color: $text-secondary;
  margin-bottom: 6px;
}

.metric-value {
  font-size: 22px;
  font-weight: 600;
  color: $text-primary;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.3px;

  &.success { color: $success; }
  &.warning { color: $warning; }
  &.danger { color: $error; }
  &.info { color: $primary; }
}

.metric-hint {
  margin-top: 4px;
  font-size: $fs-caption;
  color: $text-tertiary;
}

@media (max-width: 560px) {
  .metric-strip {
    grid-template-columns: 1fr;
  }
}
</style>
