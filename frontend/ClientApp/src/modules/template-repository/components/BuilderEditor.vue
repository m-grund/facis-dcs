<template>
  <div class="flex flex-col gap-4">
    <EmptyBlockCreator v-if="!hasBlocks" />
    <EditorBlocks v-else />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import EmptyBlockCreator from '@template-repository/components/builder-editor/EmptyBlockCreator.vue'
import EditorBlocks from '@template-repository/components/builder-editor/EditorBlocks.vue'

const { layout } = storeToRefs(useTemplateDraftStore())

const rootBlock = computed(() => layout.value.find((n) => n['dcs:isRoot']))
const hasBlocks = computed(() => (rootBlock.value?.['dcs:children']['@list'].length ?? 0) > 0)
</script>
