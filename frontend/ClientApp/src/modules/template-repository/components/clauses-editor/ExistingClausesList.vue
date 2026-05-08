<template>
  <div class="space-y-2">
    <p
      v-if="!clauseBlocks.length"
      class="text-center py-6 text-xs text-base-content/40 italic"
    >
      No clauses defined yet.
    </p>
    <div
      v-for="clause in clauseBlocks"
      :key="clause.blockId"
      class="flex items-start gap-3 p-3 rounded-lg border border-base-300 bg-base-200/30 group hover:shadow-sm transition-all"
    >
      <div class="flex-1 min-w-0">
        <div v-if="editingBlockId === clause.blockId">
          <ClauseEditorForm
            :mode="'edit'"
            :initial-title="clause.title ?? ''"
            :initial-text="clause.text ?? ''"
            :semantic-conditions="semanticConditions"
            @submit="(payload) => $emit('save', { blockId: clause.blockId, ...payload })"
            @cancel="$emit('cancel-edit')"
          />
        </div>
        <div v-else>
          <div class="font-semibold text-sm text-base-content">
            {{ clause.title ?? "" }}
            <span
              v-if="outlineBlockIds.has(clause.blockId)"
              class="font-normal text-base-content/60 ml-1"
            >
              (used in builder)
            </span>
          </div>
          <p class="text-xs text-base-content/70 mt-1 leading-relaxed whitespace-pre-wrap">
            <ClauseSegmentsPreview
              :segments="getSegments(clause)"
              :get-placeholder-label="getPlaceholderLabel"
            />
          </p>
        </div>
      </div>
      <!-- Actions -->
      <div
        class="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
        v-if="editable && editingBlockId !== clause.blockId"
      >
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          aria-label="Edit clause"
          @click="$emit('edit', clause.blockId)"
        >
          <IconEdit class="w-4 h-4" />
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-xs text-error"
          aria-label="Delete clause"
          @click="$emit('delete', clause.blockId)"
        >
          <IconRemove class="w-4 h-4" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ClauseBlock, SemanticCondition } from '@/modules/template-repository/models/contract-template'
import { parseSegments, getPlaceholderLabelFromConditions, type Segment } from '@template-repository/composables/useClauseTextChips'
import ClauseSegmentsPreview from '@template-repository/components/clauses-editor/ClauseSegmentsPreview.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import IconEdit from '@/core/components/icons/IconEdit.vue'
import IconRemove from '@/core/components/icons/IconRemove.vue'

const props =
  withDefaults(
    defineProps<{
      clauseBlocks: ClauseBlock[]
      semanticConditions: SemanticCondition[]
      blockIdsInOutline: Set<string>
      editingBlockId: string | null
      editable?: boolean
    }>(),
    { editable: true }
  )

const outlineBlockIds = computed(() => props.blockIdsInOutline)

defineEmits<{
  delete: [blockId: string]
  edit: [blockId: string]
  save: [payload: { blockId: string; title: string; text: string }]
  'cancel-edit': []
}>()

function getSegments(clause: ClauseBlock): Segment[] {
  return parseSegments(clause.text ?? '', props.semanticConditions)
}

function getPlaceholderLabel(seg: Segment): string {
  return getPlaceholderLabelFromConditions(seg, props.semanticConditions)
}
</script>
