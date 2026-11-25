<script setup lang="ts">
import { computed } from 'vue';

import { formatFileSize } from '@/utils/files';
import ParseStatusBadge from './ParseStatusBadge.vue';
import {
  resolveParseStatus,
  summarizeParseError,
  type ParseStatusItem,
} from '../utils/parseStatus';

export interface ParseDetailItem extends ParseStatusItem {
  id?: string;
  file_name?: string;
  original_file_name?: string;
  title?: string;
  created_at?: string;
  file_size?: number | string;
  file_type?: string;
  type?: string;
}

const props = withDefaults(defineProps<{
  visible: boolean;
  item: ParseDetailItem | null;
  /** Retry mutates the document; read-only callers must not see the action. */
  canRetry?: boolean;
}>(), {
  canRetry: true,
});

const emit = defineEmits<{
  (e: 'update:visible', visible: boolean): void;
  (e: 'reparse'): void;
  (e: 'cancel-parse'): void;
}>();

const drawerVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
});

const view = computed(() => resolveParseStatus(props.item));
const error = computed(() => summarizeParseError(props.item?.error_message));
const fileSize = computed(() => formatFileSize(props.item?.file_size));
const fileName = computed(
  () => props.item?.file_name || props.item?.original_file_name || props.item?.title || '未命名文档',
);

const formatTime = (time?: string): string => {
  if (!time) return '--';
  const date = new Date(time);
  if (Number.isNaN(date.getTime())) return time;
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
};

const close = () => emit('update:visible', false);

const retry = () => {
  if (!props.item) return;
  // Emit before closing: the parent clears its selected item when the drawer
  // closes, so this must happen first for the handler to see the document.
  emit('reparse');
  close();
};

const cancelParse = () => {
  if (!props.item) return;
  emit('cancel-parse');
  close();
};
</script>

<template>
  <t-drawer
    v-model:visible="drawerVisible"
    size="420px"
    placement="right"
    :destroy-on-close="true"
    :close-btn="false"
    attach="body"
    :z-index="3000"
    class="parse-detail-drawer"
  >
    <template #header>
      <div class="pd-header">
        <span class="pd-heading">解析详情</span>
        <button type="button" class="pd-close" aria-label="关闭解析详情" @click="drawerVisible = false">
          <t-icon name="close" />
        </button>
      </div>
    </template>

    <div v-if="item" class="pd-body">
      <div class="pd-file">
        <h3 class="pd-file-name" :title="fileName">{{ fileName }}</h3>
        <div class="pd-status-line">
          <ParseStatusBadge :item="item" size="md" :interactive="false" />
          <span v-if="view.inFlight" class="pd-live">
            <span class="pd-live-dot" aria-hidden="true"></span>
            状态自动刷新
          </span>
        </div>
      </div>

      <dl class="pd-meta">
        <div class="pd-meta-row">
          <dt>更新时间</dt>
          <dd>{{ formatTime(item.updated_at) }}</dd>
        </div>
        <div v-if="item.created_at" class="pd-meta-row">
          <dt>创建时间</dt>
          <dd>{{ formatTime(item.created_at) }}</dd>
        </div>
        <div v-if="fileSize" class="pd-meta-row">
          <dt>文件大小</dt>
          <dd>{{ fileSize }}</dd>
        </div>
        <div v-if="item.file_type" class="pd-meta-row">
          <dt>文件类型</dt>
          <dd>{{ String(item.file_type).toUpperCase() }}</dd>
        </div>
      </dl>

      <section v-if="view.kind === 'failed'" class="pd-section pd-section-error">
        <h4 class="pd-section-title">失败原因</h4>
        <p class="pd-summary">{{ error.summary }}</p>
        <details v-if="error.detail" class="pd-details">
          <summary>查看原始错误信息</summary>
          <pre class="pd-error-detail">{{ error.detail }}</pre>
        </details>
        <p class="pd-tip">可点击下方「重新解析」再试一次；若多次失败，请联系管理员。</p>
      </section>

      <section v-else-if="view.kind === 'stalled'" class="pd-section pd-section-warning">
        <h4 class="pd-section-title">疑似卡住</h4>
        <p class="pd-summary">{{ view.hint }}</p>
        <p class="pd-tip">解析服务可能繁忙。建议停止当前解析后重新发起；停止不会删除已上传的文件。</p>
      </section>

      <section v-else-if="view.inFlight" class="pd-section">
        <h4 class="pd-section-title">处理进度</h4>
        <p class="pd-summary">{{ view.hint }}</p>
        <p class="pd-tip">无需手动刷新，页面会在后台自动更新状态。</p>
      </section>

      <section v-else class="pd-section">
        <h4 class="pd-section-title">状态说明</h4>
        <p class="pd-summary">{{ view.hint }}</p>
      </section>
    </div>

    <template #footer>
      <div class="pd-actions">
        <t-button v-if="view.kind === 'stalled'" theme="warning" @click="cancelParse">停止解析</t-button>
        <t-button v-else-if="view.canRetry && canRetry" theme="primary" @click="retry">重新解析</t-button>
        <t-button variant="outline" @click="close">关闭</t-button>
      </div>
    </template>
  </t-drawer>
</template>

<style scoped lang="less">
.pd-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.pd-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: var(--app-radius-sm);
  background: transparent;
  color: var(--td-text-color-secondary);
  font-size: 18px;
  cursor: pointer;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
  }

  &:focus-visible {
    outline: 2px solid var(--app-focus-border);
    outline-offset: 1px;
  }
}

.pd-heading {
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
  font-size: var(--app-text-lg);
  font-weight: 600;
}

.pd-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 4px 2px 20px;
  color: var(--td-text-color-primary);
  font-family: var(--app-font-family);
}

.pd-file {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.pd-file-name {
  margin: 0;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-lg);
  font-weight: 600;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.pd-status-line {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.pd-live {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--td-text-color-placeholder);
  font-size: var(--app-text-xs);
}

.pd-live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--td-brand-color);
  animation: pd-live-pulse 1.8s ease-in-out infinite;
}

.pd-meta {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
}

.pd-meta-row {
  display: flex;
  align-items: baseline;
  gap: 16px;
  font-size: var(--app-text-sm);

  dt {
    flex: 0 0 68px;
    margin: 0;
    color: var(--td-text-color-placeholder);
  }

  dd {
    flex: 1;
    min-width: 0;
    margin: 0;
    color: var(--td-text-color-secondary);
    overflow-wrap: anywhere;
  }
}

.pd-section {
  padding: 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-md);
  background: var(--td-bg-color-secondarycontainer);
}

.pd-section-error {
  border-color: var(--td-error-color-3);
  background: var(--td-error-color-1);
}

.pd-section-warning {
  border-color: var(--td-warning-color-3);
  background: var(--td-warning-color-1);
}

.pd-section-title {
  margin: 0 0 8px;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-sm);
  font-weight: 600;
}

.pd-summary {
  margin: 0;
  color: var(--td-text-color-primary);
  font-size: var(--app-text-base);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.pd-section-error .pd-summary {
  color: var(--td-error-color);
}

.pd-section-warning .pd-summary {
  color: var(--td-warning-color);
}

.pd-tip {
  margin: 10px 0 0;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-sm);
  line-height: 1.6;
}

.pd-details {
  margin-top: 10px;

  summary {
    color: var(--td-text-color-secondary);
    font-size: var(--app-text-sm);
    cursor: pointer;
    user-select: none;
  }

  &[open] summary {
    margin-bottom: 8px;
  }
}

.pd-error-detail {
  max-height: 220px;
  margin: 0;
  padding: 10px;
  overflow: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-sm);
  background: var(--td-bg-color-container);
  color: var(--td-text-color-secondary);
  font-family: var(--app-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: var(--app-text-xs);
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.pd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@keyframes pd-live-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pd-live-dot {
    animation: none;
  }
}
</style>
