<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import EmptyBlockCreator from '@template-repository/components/builder-editor/EmptyBlockCreator.vue'
import EditorBlocks from '@template-repository/components/builder-editor/EditorBlocks.vue'

const { documentOutline } = storeToRefs(useTemplateDraftStore())

const rootBlock = computed(() => documentOutline.value.find((b) => b.isRoot))
const hasBlocks = computed(() => (rootBlock.value?.children?.length ?? 0) > 0)
</script>

<template>
  <div class="flex flex-col gap-4">
    <EmptyBlockCreator v-if="!hasBlocks" />
    <EditorBlocks v-else />
  </div>
</template>
