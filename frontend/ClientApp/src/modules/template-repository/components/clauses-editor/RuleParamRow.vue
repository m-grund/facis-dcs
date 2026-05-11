<template>
  <li class="flex items-center gap-1.5 rounded px-1 py-0.5 -mx-1" :class="rowClass" @click="$emit('click')">
    <span class="font-mono border border-base-300 rounded px-1" @mouseenter="$emit('mouseenter')"
      @mouseleave="$emit('mouseleave')">{{ param.parameterName }}</span>
    <span class="text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
    <span class="text-base-content/40">({{ param.type }})</span>
    <span v-if="constraintLabel" class="text-base-content/40">{{ constraintLabel }}</span>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SemanticConditionParameter } from '@/modules/template-repository/models/contract-template'

const props = defineProps<{
  param: SemanticConditionParameter
  isUsed?: boolean
  isRequiredAndUnused?: boolean
}>()

defineEmits<{
  mouseenter: []
  mouseleave: []
  click: []
}>()

const rowClass = computed(() => ({
  'text-primary font-medium': props.isUsed,
  'text-error cursor-pointer hover:bg-base-200': props.isRequiredAndUnused,
}))

const constraintLabel = computed(() => {
  const constraint = props.param.valueConstraint
  if (!constraint) return ''
  if (constraint.allowedValuesRef) return constraint.allowedValuesRef
  if (constraint.format) return constraint.format
  if (constraint.allowedValues?.length) return constraint.allowedValues.join(', ')
  if (constraint.min !== undefined || constraint.max !== undefined) return `${constraint.min ?? '-'}-${constraint.max ?? '-'}`
  return ''
})
</script>
