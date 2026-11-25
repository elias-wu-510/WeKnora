<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  visible: boolean;
  kbName?: string;
  maxFileSizeMb: number;
}>();

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void;
  (e: 'drop', event: DragEvent): void;
  (e: 'pick'): void;
}>();

const visible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
});
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    header="上传资料"
    :footer="false"
    width="520px"
    placement="center"
  >
    <div class="ck-upload-entry">
      <p class="ck-upload-entry__target">
        目标知识库：<strong>{{ kbName || '当前知识库' }}</strong>
      </p>
      <div
        class="ck-upload-entry__dropzone"
        @dragover.prevent
        @drop.stop.prevent="emit('drop', $event)"
      >
        <span class="ck-upload-entry__icon">
          <t-icon name="upload" size="28px" />
        </span>
        <p class="ck-upload-entry__title">把文件拖到这里</p>
        <t-button theme="primary" @click="emit('pick')">选择文件</t-button>
      </div>
      <p class="ck-upload-entry__tip">
        支持 PDF、Word、Excel、PPT、Markdown、纯文本等格式，单个文件不超过 {{ maxFileSizeMb }}MB。
        选好后会先打开上传确认，可设置标签与解析方式。
      </p>
    </div>
  </t-dialog>
</template>

<style scoped lang="less">
// 上传投递面板（从首页带 ?upload=1 进入时先落到这里）
.ck-upload-entry__target {
  margin: 0 0 12px;
  font-size: 14px;
  color: var(--td-text-color-secondary);
}

.ck-upload-entry__target strong {
  color: var(--td-text-color-primary);
  font-weight: 600;
}

.ck-upload-entry__dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 28px 16px;
  border: 1px dashed var(--td-brand-color-4);
  border-radius: 10px;
  background: var(--td-brand-color-1);
  text-align: center;
}

.ck-upload-entry__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background: var(--td-bg-color-container);
  color: var(--td-brand-color);
}

.ck-upload-entry__title {
  margin: 0;
  font-size: 14px;
  color: var(--td-text-color-primary);
}

.ck-upload-entry__tip {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--td-text-color-placeholder);
}
</style>