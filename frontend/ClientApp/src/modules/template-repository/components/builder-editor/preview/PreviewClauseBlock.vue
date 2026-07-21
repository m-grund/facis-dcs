<script setup lang="ts">
import {
  isNewline,
  isPlaceholder,
  isText,
  parseSegmentsFromContent,
} from '@template-repository/composables/useClauseTextChips'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'
import { computed } from 'vue'
import { PREVIEW_NEWLINE_SPAN_CLASS } from './preview-classes'
import PreviewParamInput from './PreviewParamInput.vue'
import PreviewTextBlock from './PreviewTextBlock.vue'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { DcsContentSegment } from '@/models/dcs-jsonld'
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
}>()

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
</template>

<style scoped>
.preview-newline-break + .preview-newline-break {
  margin-bottom: 0.2rem;
}
</style>
