<script setup lang="ts">
import { computed } from 'vue';

import {
  resolveParseStatus,
  type ParseStatusItem,
} from '../utils/parseStatus';

const props = withDefaults(
  defineProps<{
    item: ParseStatusItem;
    size?: 'sm' | 'md';
    /** Clicking the badge opens the parse-detail drawer. */
    interactive?: boolean;
    /** Hide the badge for completed rows where a label would be noise. */
    showCompleted?: boolean;
  }>(),
  { size: 'sm', interactive: true, showCompleted: true },
);

const emit = defineEmits<{
  (e: 'open-detail'): void;
}>();

const view = computed(() => resolveParseStatus(props.item));
const visible = computed(
  () => view.value.kind !== 'unknown' && (props.showCompleted || view.value.kind !== 'completed'),
);

const onActivate = () => {
  if (!props.interactive) return;
  emit('open-detail');
};
</script>

<template>
  <button
    v-if="visible"
    type="button"
    class="parse-status-badge"
    :class="[
      `is-${view.tone}`,
      `size-${size}`,
      {
        'is-quiet': view.quiet,
        'is-pulsing': view.pulse,
        'is-static': !interactive,
      },
    ]"
    :title="view.hint"
    :aria-label="`${view.label}：${view.hint}`"
    :tabindex="interactive ? 0 : -1"
    :disabled="!interactive"
    @click.stop="onActivate"
  >
    <t-icon
      :name="view.icon"
      class="ps-icon"
      :class="{ 'ps-spin': view.spin, 'ps-pulse': view.pulse }"
      aria-hidden="true"
    />
    <span class="ps-label">{{ view.label }}</span>
    <span v-if="view.kind === 'stalled' && view.stalledMinutes > 0" class="ps-minutes">
      {{ view.stalledMinutes }} 分钟
    </span>
  </button>
</template>

<style scoped lang="less">
.parse-status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  max-width: 100%;
  height: 22px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: var(--app-radius-pill);
  background: transparent;
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-family);
  font-size: var(--app-text-xs);
  font-weight: 400;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition:
    background-color var(--app-motion-fast) ease,
    border-color var(--app-motion-fast) ease,
    color var(--app-motion-fast) ease;

  &.size-md {
    height: 24px;
    padding: 0 9px;
    font-size: var(--app-text-sm);
  }

  &.is-active {
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  &.is-danger {
    color: var(--td-error-color);
    background: var(--td-error-color-1);
  }

  &.is-warning {
    color: var(--td-warning-color);
    background: var(--td-warning-color-1);
  }

  &.is-success,
  &.is-neutral {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-secondarycontainer);
  }

  &.is-success.is-quiet {
    padding: 0 2px;
    background: transparent;
    color: var(--td-text-color-placeholder);
  }

  &.is-static {
    cursor: default;
  }

  &:not(.is-static):hover {
    border-color: currentColor;
  }

  &:focus-visible {
    outline: 2px solid var(--app-focus-border);
    outline-offset: 2px;
  }

  &:disabled {
    opacity: 1;
  }
}

.ps-icon {
  flex-shrink: 0;
  font-size: 13px;
}

.size-md .ps-icon {
  font-size: 14px;
}

.ps-label {
  overflow: hidden;
  text-overflow: ellipsis;
}

.ps-minutes {
  padding-left: 3px;
  border-left: 1px solid currentColor;
  opacity: 0.72;
  font-variant-numeric: tabular-nums;
}

.ps-spin {
  animation: parse-status-spin 0.9s linear infinite;
}

.ps-pulse {
  animation: parse-status-pulse 1.8s ease-in-out infinite;
}

@keyframes parse-status-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes parse-status-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ps-spin,
  .ps-pulse {
    animation: none;
  }
}
</style>
