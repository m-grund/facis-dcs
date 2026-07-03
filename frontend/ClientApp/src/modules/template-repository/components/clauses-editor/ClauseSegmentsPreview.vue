<script setup lang="ts">
import type { Segment } from '@template-repository/composables/useClauseTextChips'
import { isText, isPlaceholder, isNewline } from '@template-repository/composables/useClauseTextChips'
import ClausePlaceholderSpan from '@template-repository/components/clauses-editor/ClausePlaceholderSpan.vue'

const props = defineProps<{
  segments: Segment[]
  getPlaceholderLabel: (seg: Segment) => string
}>()

const segments = props.segments
const getPlaceholderLabel = props.getPlaceholderLabel
</script>

<template>
  <span>
    <template v-for="(seg, i) in segments" :key="i">
      <template v-if="isText(seg)">
        {{ seg.value }}
      </template>
      <ClausePlaceholderSpan v-else-if="isPlaceholder(seg)" :label="getPlaceholderLabel(seg)" />
      <br v-else-if="isNewline(seg)" />
    </template>
  </span>
</template>
