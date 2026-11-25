<script setup lang="ts">
import { watch } from "vue"

const props = defineProps(['reviewImg', 'reviewUrl'])
const emit = defineEmits(['closePreImg'])
const close = () => {
    emit('closePreImg')
}
</script>
<template>
    <!-- TDesign 的 ImageViewer 即使 visible=false 也会渲染默认 trigger；没有图片时
         这个 trigger 就是一张“图片无法显示 / 预览”的空卡。这里只在确实有预览地址
         且需要打开时挂载，并用空的 trigger 槽避免在消息流里重复插入缩略图。 -->
    <t-image-viewer v-if="reviewImg && reviewUrl" :visible="reviewImg" closeOnOverlay closeOnEscKeydown
        @close="close" :images="[reviewUrl]">
        <template #trigger><span /></template>
    </t-image-viewer>
</template>
<style scoped lang="less"></style>
