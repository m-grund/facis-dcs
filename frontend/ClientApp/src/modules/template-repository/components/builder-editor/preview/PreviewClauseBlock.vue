<script setup lang="ts">
import TypedClauseEditor from '@template-repository/components/clauses-editor/TypedClauseEditor.vue'
import {
  isNewline,
  isPlaceholder,
  isText,
  parseSegmentsFromContent,
} from '@template-repository/composables/useClauseTextChips'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'
import { isMergedBlockId } from '@template-repository/utils/template-data-ref'
import { computed, ref } from 'vue'
import { PREVIEW_NEWLINE_SPAN_CLASS } from './preview-classes'
import PreviewParamInput from './PreviewParamInput.vue'
import PreviewTextBlock from './PreviewTextBlock.vue'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { DcsContentSegment, DcsTypedClauseInstance } from '@/models/dcs-jsonld'
import type { VerificationResult } from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import type {
  SemanticCondition,
  SemanticParameterType,
  SemanticValueConstraint,
} from '@template-repository/models/contract-template'

const props = defineProps<{
  blockId: string
  content: DcsContentSegment[]
  semanticConditions: SemanticCondition[]
  semanticConditionValues?: SemanticConditionValue[]
  verificationResult?: VerificationResult | null
  setSemanticConditionValue?: SemanticConditionValueSetter
  /** The clause's nested typed instance, when it is a Semantic Hub typed clause. */
  typedClause?: DcsTypedClauseInstance
}>()

// Typed clause fill (hub shapes at contract time): editable exactly when the
// preview is in fill mode (a value setter is wired) and the clause lives in
// the document itself — sub-template snapshot clauses are immutable.
const typedClauseEditable = computed(
  () => !!props.typedClause && !!props.setSemanticConditionValue && !isMergedBlockId(props.blockId),
)
const editingTypedClause = ref(false)
const draftStore = useDcsDraftStore()

function saveTypedClause(payload: { title: string; instance: DcsTypedClauseInstance }) {
  draftStore.updateTypedClause(props.blockId, { title: payload.title, instance: payload.instance })
  editingTypedClause.value = false
}

type PreviewSegment =
  | { type: 'text'; value: string }
  | {
      type: 'param'
      conditionId: string
      parameterName: string
      paramType: SemanticParameterType
      label: string
      value?: string | number | boolean
      valueConstraint?: SemanticValueConstraint
      isInvalid?: boolean
      invalidTip?: string
    }
  | { type: 'newline' }

const previewNewlineSpanClass = PREVIEW_NEWLINE_SPAN_CLASS

const segments = computed<PreviewSegment[]>(() => {
  const baseSegments = parseSegmentsFromContent(props.content ?? [], props.semanticConditions)
  const result: PreviewSegment[] = []
  for (const seg of baseSegments) {
    if (isText(seg)) {
      result.push({ type: 'text', value: seg.value })
    } else if (isPlaceholder(seg)) {
      const cond = props.semanticConditions.find((c) => c.conditionId === seg.conditionId)
      const param = cond?.parameters.find((p) => p.parameterName === seg.parameterName)
      const paramType: SemanticParameterType = param?.type ?? 'string'
      result.push({
        type: 'param',
        conditionId: seg.conditionId,
        parameterName: seg.parameterName,
        paramType,
        label: param ? semanticParameterLabel(param) : seg.parameterName,
        value: findSemanticValue(seg.conditionId, seg.parameterName),
        valueConstraint: param?.valueConstraint,
        isInvalid: !!findVerificationError(seg.conditionId, seg.parameterName),
        invalidTip: findVerificationError(seg.conditionId, seg.parameterName)?.message,
      })
    } else if (isNewline(seg)) {
      result.push({ type: 'newline' })
    }
  }
  return result
})

function onParamValueChange(seg: PreviewSegment, value: string | number | boolean) {
  if (seg.type !== 'param') return
  props.setSemanticConditionValue?.(props.blockId, seg.conditionId, seg.parameterName, value)
}

function findSemanticValue(conditionId: string, parameterName: string): string | number | boolean | undefined {
  return props.semanticConditionValues?.find((item) => {
    return item.blockId === props.blockId && item.conditionId === conditionId && item.parameterName === parameterName
  })?.parameterValue
}

function findVerificationError(conditionId: string, parameterName: string) {
  if (!props.verificationResult) return null
  return (
    props.verificationResult.errors.find((item) => {
      return item.blockId === props.blockId && item.conditionId === conditionId && item.parameterName === parameterName
    }) ?? null
  )
}
</script>

<template>
  <template v-for="(seg, index) in segments" :key="index">
    <PreviewTextBlock v-if="seg.type === 'text'" :text="seg.value" />
    <PreviewParamInput
      v-else-if="seg.type === 'param'"
      :type="seg.paramType"
      :label="seg.label"
      :value="seg.value"
      :value-constraint="seg.valueConstraint"
      :is-invalid="seg.isInvalid"
      :invalid-tip="seg.invalidTip"
      @update:value="(val) => onParamValueChange(seg, val)"
    />
    <span
      v-else-if="seg.type === 'newline'"
      :class="[previewNewlineSpanClass, 'preview-newline-break']"
      aria-hidden="true"
    />
  </template>
  <template v-if="typedClauseEditable">
    <span class="ml-2 align-middle">
      <button
        v-if="!editingTypedClause"
        type="button"
        class="btn btn-outline btn-xs"
        @click="editingTypedClause = true"
      >
        Edit typed values
      </button>
    </span>
    <div v-if="editingTypedClause && typedClause" class="mt-2 rounded border border-base-300 bg-base-200/30 p-3">
      <TypedClauseEditor
        :instance="typedClause"
        submit-label="Save values"
        @submit="saveTypedClause"
        @cancel="editingTypedClause = false"
      />
    </div>
  </template>
</template>

<style scoped>
.preview-newline-break + .preview-newline-break {
  margin-bottom: 0.2rem;
}
</style>
