<template>
  <span class="tooltip tooltip-top inline-flex items-baseline" :data-tip="tipText">
    <select
      v-if="allowedValues.length && (type === 'string' || type === 'enum')"
      v-model="stringValue"
      :class="selectClass"
      :aria-label="label"
      @change="emitStringValue"
    >
      <option value=""></option>
      <option v-for="option in allowedValues" :key="option" :value="option">{{ option }}</option>
    </select>
    <input
      v-else-if="type === 'string' || type === 'enum'"
      v-model="stringValue"
      type="text"
      :class="inputClass"
      :aria-label="label"
      @input="emitStringValue"
    />
    <input
      v-else-if="type === 'integer'"
      v-model="numberValue"
      type="text"
      inputmode="numeric"
      :class="inputClass"
      :aria-label="label"
      @keydown="onIntegerKeyDown"
      @input="emitIntegerValue"
    />
    <input
      v-else-if="type === 'decimal'"
      v-model="numberValue"
      type="number"
      :class="inputClass"
      :aria-label="label"
      @input="emitDecimalValue"
    />
    <input
      v-else-if="type === 'date'"
      v-model="dateValue"
      type="date"
      :class="inputClass"
      :aria-label="label"
      @input="emitDateValue"
    />
  </span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type {
  SemanticParameterType,
  SemanticValueConstraint,
} from '@/modules/template-repository/models/contract-template'
import { resolveAllowedValues } from '@template-repository/utils/value-constraint-catalog'

const props = defineProps<{
  type: SemanticParameterType
  label?: string
  value?: string | number | boolean
  valueConstraint?: SemanticValueConstraint
  isInvalid?: boolean
  invalidTip?: string
}>()
const emit = defineEmits<(e: 'update:value', value: string | number | boolean) => void>()

const stringValue = ref('')
const numberValue = ref('')
const dateValue = ref('')
const booleanValue = ref(false)
const allowedValues = computed(() => resolveAllowedValues(props.valueConstraint))
const tipText = computed(() => props.invalidTip ?? props.valueConstraint?.description ?? props.label ?? '')
const inputClass = computed(
  () =>
    `border-b bg-transparent text-sm leading-relaxed px-0.5 outline-none ${props.isInvalid ? 'border-error text-error' : 'border-base-400'}`,
)
const selectClass = computed(
  () =>
    `select select-xs h-7 min-h-0 w-28 rounded-md bg-transparent px-1 text-sm leading-relaxed ${props.isInvalid ? 'select-error text-error' : 'select-bordered'}`,
)

watch(
  () => props.type,
  () => {
    stringValue.value = ''
    numberValue.value = ''
    dateValue.value = ''
    booleanValue.value = false
  },
)

watch(
  () => props.value,
  (value) => {
    const next = value ?? ''
    if (props.type === 'string' || props.type === 'enum') stringValue.value = `${next}`
    if (props.type === 'decimal' || props.type === 'integer') numberValue.value = `${next}`
    if (props.type === 'date') dateValue.value = `${next}`
    if (props.type === 'boolean') booleanValue.value = Boolean(next)
  },
  { immediate: true },
)

function emitStringValue(event: Event) {
  const next = (event.target as HTMLInputElement | HTMLSelectElement | null)?.value ?? ''
  emit('update:value', next)
}

function emitIntegerValue(event: Event) {
  const next = getIntegerInput((event.target as HTMLInputElement | null)?.value ?? '')
  if (next === '' || next === '-') {
    emit('update:value', '')
    return
  }
  const parsed = Number(next)
  emit('update:value', Number.isInteger(parsed) ? parsed : '')
}

function emitDecimalValue(event: Event) {
  const next = (event.target as HTMLInputElement | null)?.value ?? ''
  if (next === '') {
    emit('update:value', '')
    return
  }
  const parsed = Number(next)
  emit('update:value', Number.isNaN(parsed) ? '' : parsed)
}

function emitDateValue(event: Event) {
  const next = (event.target as HTMLInputElement | null)?.value ?? ''
  emit('update:value', next)
}

function getIntegerInput(value: string): string {
  if (!value) return ''
  const trimmed = value.trim()
  const negative = trimmed.startsWith('-')
  const digitsOnly = trimmed.replace(/[^\d]/g, '')
  if (!digitsOnly) return negative ? '-' : ''
  return `${negative ? '-' : ''}${digitsOnly}`
}

function onIntegerKeyDown(event: KeyboardEvent) {
  const allowedControlKeys = new Set([
    'Backspace',
    'Delete',
    'Tab',
    'Escape',
    'Enter',
    'ArrowLeft',
    'ArrowRight',
    'Home',
    'End',
  ])
  if (allowedControlKeys.has(event.key) || event.metaKey || event.ctrlKey) return
  if (event.key === '-') {
    const input = event.target as HTMLInputElement | null
    const cursorAtStart = (input?.selectionStart ?? 0) === 0
    const hasMinus = numberValue.value.includes('-')
    const allSelected = input?.selectionStart === 0 && input?.selectionEnd === input?.value.length
    if ((cursorAtStart || allSelected) && !hasMinus) return
    event.preventDefault()
    return
  }
  if (event.key.length === 1 && !/\d/.test(event.key)) {
    event.preventDefault()
  }
}
</script>
