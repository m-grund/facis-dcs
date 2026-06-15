<template>
  <h3 class="mb-4 text-sm font-semibold text-base-content/80">{{ formTitle }}</h3>
  <div class="space-y-4">
    <div>
      <label class="label-text mb-1 block text-xs text-base-content/60">
        Rule name
        <RequiredIndicator />
      </label>
      <input
        v-model="newCondition.conditionName"
        type="text"
        class="input-bordered input input-sm w-full"
        :class="{ 'input-error': isRuleNameDuplicate }"
        placeholder=""
      />
      <p class="mt-0.5 text-xs text-base-content/50">Used when selecting this rule for a clause.</p>
      <p v-if="isRuleNameDuplicate" class="mt-0.5 text-xs text-error">Rule name already exists.</p>
    </div>

    <div class="space-y-4">
      <p class="label-text mb-1 text-xs text-base-content/60">Parameters</p>
      <div
        class="grid grid-cols-1 items-start gap-x-3 gap-y-3 rounded-lg border-2 border-dashed border-base-300 bg-base-200/50 p-3 md:grid-cols-12"
      >
        <p class="mb-2 text-xs font-medium text-base-content/70 md:col-span-12">New parameter</p>
        <div class="flex flex-col gap-1 md:col-span-4">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">
              Domain field
              <RequiredIndicator />
            </span>
          </label>
          <div class="relative">
            <input
              v-model="domainFieldSearch"
              type="search"
              class="input-bordered input input-sm h-9 w-full"
              :class="{ 'input-primary': selectedDomainPath }"
              placeholder="Search domain fields"
              autocomplete="off"
              @focus="showDomainFieldOptions = true"
              @input="handleDomainFieldInput"
              @keydown.escape="showDomainFieldOptions = false"
              @blur="hideDomainFieldOptions"
            />
            <div
              v-if="showDomainFieldOptions"
              class="absolute z-30 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border border-base-300 bg-base-100 shadow-lg"
            >
              <template v-if="groupedDomainFields.length">
                <section v-for="group in groupedDomainFields" :key="group.name">
                  <div class="sticky top-0 z-10 border-b border-base-200 bg-base-100 px-3 py-1">
                    <p class="text-xs font-semibold text-base-content/50 uppercase">{{ group.name }}</p>
                  </div>
                  <button
                    v-for="field in group.fields"
                    :key="field.semanticPath"
                    type="button"
                    class="w-full border-b border-base-200 px-3 py-2 text-left transition-colors last:border-b-0 hover:bg-base-200"
                    :class="{ 'bg-primary/10': selectedDomainPath === field.semanticPath }"
                    @mousedown.prevent="selectDomainField(field.semanticPath)"
                  >
                    <span class="block text-sm font-medium text-base-content">{{ field.label }}</span>
                    <span class="block text-xs text-base-content/50">{{ field.semanticPath }}</span>
                    <span v-if="field.valueConstraint" class="block text-xs text-base-content/50">
                      {{ formatValueConstraint(field.valueConstraint) }}
                    </span>
                  </button>
                </section>
              </template>
              <p v-else class="p-3 text-sm text-base-content/50">No matching domain fields.</p>
            </div>
          </div>
          <p v-if="selectedDomainField" class="text-xs text-base-content/50">{{ selectedDomainField.semanticPath }}</p>
          <p v-if="selectedDomainField?.valueConstraint" class="text-xs text-base-content/50">
            {{ formatValueConstraint(selectedDomainField.valueConstraint) }}
          </p>
          <p v-if="isParameterNameDuplicate" class="text-xs text-error">Parameter name already exists.</p>
        </div>
        <div class="flex flex-col gap-1 md:col-span-3">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">Obligation</span>
          </label>
          <select v-model="draftOperator" class="select-bordered select h-9 w-full select-sm">
            <option value="">None</option>
            <option v-for="option in operatorOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </div>
        <div v-if="!usesSetConstraintEditor" class="flex flex-col gap-1 md:col-span-2">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">Value</span>
          </label>
          <select
            v-if="draftParameter.type === 'boolean'"
            v-model="draftTarget"
            class="select-bordered select h-9 w-full select-sm"
            :disabled="!draftOperator"
          >
            <option value="">Select</option>
            <option value="true">true</option>
            <option value="false">false</option>
          </select>
          <input
            v-else
            v-model="draftTarget"
            :type="draftParameter.type === 'date' ? 'date' : 'text'"
            class="input-bordered input input-sm h-9 w-full"
            :disabled="!draftOperator"
            placeholder=""
          />
          <p v-if="operatorError" class="text-xs text-error">{{ operatorError }}</p>
        </div>
        <div class="flex flex-col gap-1 md:col-span-2">
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">Required</span>
          </label>
          <div class="flex h-9 items-center">
            <label class="label h-auto min-h-0 cursor-pointer justify-start gap-2 py-0">
              <input
                v-model="draftParameter.isRequired"
                type="checkbox"
                class="checkbox checkbox-sm checkbox-primary"
              />
              <span class="label-text text-xs">Required</span>
            </label>
          </div>
        </div>
        <div class="flex flex-col gap-1 md:col-span-1">
          <label class="invisible label min-h-0 py-0">
            <span class="label-text text-xs">Add</span>
          </label>
          <div class="flex h-9 items-center">
            <button
              type="button"
              class="btn btn-square w-full btn-sm btn-secondary"
              aria-label="Add parameter"
              title="Add parameter"
              :disabled="!canAddParameter"
              @click="addParameter"
            >
              +
            </button>
          </div>
        </div>
        <div
          v-if="usesSetConstraintEditor && isSetOperator(draftOperator)"
          class="flex flex-col gap-1 md:col-span-5 md:col-start-5"
        >
          <label class="label min-h-0 py-0">
            <span class="label-text text-xs text-base-content/60">Values</span>
          </label>
          <input
            v-if="valueOptions.length"
            v-model="valueOptionSearch"
            type="search"
            class="input-bordered input input-sm h-9 w-full"
            placeholder="Search values"
          />
          <div
            v-if="valueOptions.length"
            class="max-h-36 overflow-y-auto rounded border border-base-300 bg-base-100 p-2"
          >
            <label
              v-for="option in filteredValueOptions"
              :key="option.value"
              class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-base-200"
            >
              <input
                v-model="draftSetTargets"
                type="checkbox"
                class="checkbox checkbox-xs checkbox-primary"
                :value="option.value"
              />
              <span>{{ option.label }} ({{ option.value }})</span>
            </label>
          </div>
          <input
            v-else
            v-model="draftTokenTargets"
            type="text"
            class="input-bordered input input-sm h-9 w-full"
            placeholder=""
          />
          <div v-if="draftSetTargets.length" class="flex flex-wrap gap-1">
            <span v-for="value in draftSetTargets" :key="value" class="badge badge-outline badge-sm">
              {{ formatSelectedValue(value) }}
            </span>
          </div>
          <p v-if="operatorError" class="text-xs text-error">{{ operatorError }}</p>
        </div>
      </div>

      <!-- Added parameters -->
      <div v-if="newCondition.parameters.length" class="space-y-2">
        <p class="text-xs font-medium text-base-content/70">Added parameters</p>
        <ul class="space-y-2">
          <li
            v-for="(param, idx) in newCondition.parameters"
            :key="idx"
            class="flex items-center gap-3 rounded-lg border border-base-300 bg-base-100 px-3 py-2.5"
          >
            <span class="rounded border border-base-300 bg-base-200/50 px-2 py-0.5 font-mono text-sm font-medium">
              {{ semanticParameterLabel(param) }}
            </span>
            <span class="text-xs text-base-content/50">{{ param.semanticPath }}</span>
            <span v-if="param.valueConstraint" class="text-xs text-base-content/50">
              {{ formatValueConstraint(param.valueConstraint) }}
            </span>
            <span
              v-for="operator in param.operators"
              :key="formatOperatorConstraint(operator)"
              class="badge badge-outline badge-sm"
            >
              {{ formatOperatorConstraint(operator) }}
            </span>
            <span class="text-xs text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
            <button
              type="button"
              class="btn ml-auto shrink-0 text-error btn-ghost btn-xs"
              aria-label="Delete parameter"
              @click="deleteParameter(idx)"
            >
              ✕
            </button>
          </li>
        </ul>
      </div>
    </div>

    <div class="flex items-center justify-between">
      <button v-if="isEditMode" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">Cancel</button>
      <span v-else />
      <button type="button" class="btn btn-sm btn-secondary" :disabled="!canSubmitRule" @click="submitRule">
        {{ submitLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { DcsOperator } from '@/models/semantic/facis-dcs-semantic'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import {
  type SemanticCondition,
  type SemanticConditionParameter,
  type DomainSemanticPath,
  type SemanticValueConstraint,
  type SemanticEntityRole,
  type SemanticEntityType,
  type SemanticOperateType,
  SEMANTIC_CONDITION_SCHEMA_VERSION,
} from '@/modules/template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS } from '@/modules/template-repository/utils/ontology-domain-fields'
import { ONTOLOGY_DOMAIN_TYPE_FIELD_PATHS } from '@/modules/template-repository/utils/ontology-domain-types'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'
import {
  formatValueOption,
  isTokenValueConstraint,
  resolveValueOptions,
} from '@template-repository/utils/value-option-catalog'

type NewConditionPayload = Omit<SemanticCondition, 'conditionId'>
type DraftConditionPayload = NewConditionPayload & {
  entityType: SemanticEntityType
  entityRole: SemanticEntityRole
}

const props = defineProps<{
  existingConditions: SemanticCondition[]
  mode?: 'create' | 'edit'
  initialCondition?: SemanticCondition | null
}>()

const emit = defineEmits<{
  'add-rule': [payload: NewConditionPayload]
  'update-rule': [payload: { conditionId: string; data: NewConditionPayload }]
  cancel: []
}>()

function defaultParam(): SemanticConditionParameter {
  const defaultField = semanticRuleDomainFields[0] ?? ONTOLOGY_DOMAIN_FIELDS[0]!
  return {
    parameterName: '',
    type: defaultField.type,
    schemaRef: defaultField.schemaRef,
    semanticPath: defaultField.semanticPath,
    valueConstraint: cloneValueConstraint(defaultField.valueConstraint),
    uiMetadata: { label: defaultField.label },
    isRequired: true,
    operators: [],
    value: undefined,
  }
}

const semanticRuleDomainFields = ONTOLOGY_DOMAIN_FIELDS.filter(
  (field) => !ONTOLOGY_DOMAIN_TYPE_FIELD_PATHS.has(field.semanticPath),
)

function getDefaultNewCondition(): DraftConditionPayload {
  return {
    conditionName: '',
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    entityType: '',
    entityRole: '',
    parameters: [],
  }
}

const newCondition = ref<DraftConditionPayload>(getDefaultNewCondition())
const draftParameter = ref<SemanticConditionParameter>(defaultParam())
const draftOperator = ref<SemanticOperateType | ''>('')
const draftTarget = ref('')
const draftSetTargets = ref<string[]>([])
const draftTokenTargets = ref('')
const valueOptionSearch = ref('')
const selectedDomainPath = ref<DomainSemanticPath>('')
const domainFieldSearch = ref('')
const showDomainFieldOptions = ref(false)
const isEditMode = computed(() => props.mode === 'edit')
const formTitle = computed(() => (isEditMode.value ? 'Edit rule' : 'New rule'))
const submitLabel = computed(() => (isEditMode.value ? 'Save changes' : 'Add rule'))
const selectedDomainField = computed(() =>
  semanticRuleDomainFields.find((field) => field.semanticPath === selectedDomainPath.value),
)
const valueOptions = computed(() => resolveValueOptions(selectedDomainField.value?.valueConstraint))
const usesSetConstraintEditor = computed(() => {
  const constraint = selectedDomainField.value?.valueConstraint
  return !!constraint && (valueOptions.value.length > 0 || isTokenValueConstraint(constraint))
})
const operatorOptions = computed(() =>
  usesSetConstraintEditor.value ? setOperatorOptions() : operatorOptionsForType(draftParameter.value.type),
)
const filteredValueOptions = computed(() => {
  const query = valueOptionSearch.value.trim().toLowerCase()
  if (!query) return valueOptions.value
  return valueOptions.value.filter(
    (option) => option.value.toLowerCase().includes(query) || option.label.toLowerCase().includes(query),
  )
})
const operatorError = computed(() => validateDraftOperator())
const groupedDomainFields = computed(() => {
  const query = domainFieldSearch.value.trim().toLowerCase()
  const filtered = semanticRuleDomainFields.filter((field) => {
    if (!query) return true
    return [
      field.label,
      field.semanticPath,
      field.schemaRef,
      field.type,
      field.group,
      field.valueConstraint?.format ?? '',
      field.valueConstraint?.allowedValuesRef ?? '',
      field.valueConstraint?.allowedValues?.join(' ') ?? '',
      field.valueConstraint?.valueOptions?.map((option) => `${option.label ?? ''} ${option.symbol ?? ''}`).join(' ') ?? '',
    ].some((value) => value.toLowerCase().includes(query))
  })
  const byGroup = new Map<string, typeof filtered>()
  for (const field of filtered) {
    byGroup.set(field.group, [...(byGroup.get(field.group) ?? []), field])
  }
  return [...byGroup.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, fields]) => ({
      name,
      fields: [...fields].sort((left, right) => left.label.localeCompare(right.label)),
    }))
})

watch(
  () => [props.mode, props.initialCondition] as const,
  () => {
    if (!isEditMode.value || !props.initialCondition) {
      newCondition.value = getDefaultNewCondition()
      draftParameter.value = defaultParam()
      draftOperator.value = ''
      draftTarget.value = ''
      resetSetConstraintDraft()
      selectedDomainPath.value = ''
      domainFieldSearch.value = ''
      showDomainFieldOptions.value = false
      return
    }
    newCondition.value = {
      conditionName: props.initialCondition.conditionName,
      schemaVersion: props.initialCondition.schemaVersion,
      entityType: props.initialCondition.entityType ?? '',
      entityRole: props.initialCondition.entityRole ?? '',
      parameters: props.initialCondition.parameters.map((p) => ({
        ...p,
        valueConstraint: cloneValueConstraint(p.valueConstraint),
      })),
    }
    draftParameter.value = defaultParam()
    draftOperator.value = ''
    draftTarget.value = ''
    resetSetConstraintDraft()
    selectedDomainPath.value = ''
    domainFieldSearch.value = ''
    showDomainFieldOptions.value = false
  },
  { immediate: true },
)

watch(selectedDomainPath, (path) => {
  const field = semanticRuleDomainFields.find((item) => item.semanticPath === path)
  if (!field) {
    draftParameter.value = defaultParam()
    return
  }
  draftParameter.value = {
    ...draftParameter.value,
    parameterName: field.semanticPath,
    schemaRef: field.schemaRef,
    semanticPath: field.semanticPath,
    valueConstraint: cloneValueConstraint(field.valueConstraint),
    uiMetadata: { label: field.label },
    type: field.type,
  }
  domainFieldSearch.value = formatDomainFieldLabel(field)
  draftOperator.value = ''
  draftTarget.value = ''
  resetSetConstraintDraft()
})

watch(draftOperator, (operator) => {
  if (!operator) draftTarget.value = ''
  if (isSetOperator(operator)) {
    draftTarget.value = ''
    return
  }
  clearSetConstraintTargets()
})

watch(usesSetConstraintEditor, (usesSet) => {
  if (usesSet) {
    draftOperator.value = ''
    draftTarget.value = ''
  } else {
    resetSetConstraintDraft()
  }
})

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
  }
}

function formatValueConstraint(constraint: SemanticValueConstraint) {
  if (constraint.allowedValuesRef) return constraint.allowedValuesRef
  if (constraint.format) return constraint.format
  if (constraint.allowedValues?.length) return `Allowed: ${constraint.allowedValues.join(', ')}`
  if (constraint.min !== undefined || constraint.max !== undefined) {
    return `Range: ${constraint.min ?? '-'} - ${constraint.max ?? '-'}`
  }
  return constraint.description ?? 'Constrained value'
}

function formatDomainFieldLabel(field: (typeof ONTOLOGY_DOMAIN_FIELDS)[number]) {
  return `${field.label} (${field.semanticPath})`
}

function selectDomainField(path: DomainSemanticPath) {
  selectedDomainPath.value = path
  showDomainFieldOptions.value = false
}

function handleDomainFieldInput() {
  showDomainFieldOptions.value = true
  if (!selectedDomainField.value) return
  if (domainFieldSearch.value === formatDomainFieldLabel(selectedDomainField.value)) return
  selectedDomainPath.value = ''
}

function hideDomainFieldOptions() {
  window.setTimeout(() => {
    showDomainFieldOptions.value = false
  }, 100)
}

const isParameterNameDuplicate = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
  const lower = name.toLowerCase()
  return newCondition.value.parameters.some((p) => p.parameterName.trim().toLowerCase() === lower)
})

const canAddParameter = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
  if (operatorError.value) return false
  return !isParameterNameDuplicate.value
})

const isRuleNameDuplicate = computed(() => {
  const name = newCondition.value.conditionName?.trim()
  if (!name) return false
  const lower = name.toLowerCase()
  const currentConditionId = props.initialCondition?.conditionId
  return props.existingConditions.some(
    (c) =>
      // When in edit mode, the current condition is not included in the check
      c.conditionId !== currentConditionId && c.conditionName.trim().toLowerCase() === lower,
  )
})

const canSubmitRule = computed(() => {
  const name = newCondition.value.conditionName?.trim()
  if (!name) return false
  if (newCondition.value.parameters.length === 0) return false
  return !isRuleNameDuplicate.value
})

function addParameter() {
  if (!canAddParameter.value) return
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return
  newCondition.value.parameters.push({
    ...draftParameter.value,
    parameterName: name,
    operators: buildDraftOperators(),
  })
  draftParameter.value = defaultParam()
  draftOperator.value = ''
  draftTarget.value = ''
  resetSetConstraintDraft()
  selectedDomainPath.value = ''
  domainFieldSearch.value = ''
  showDomainFieldOptions.value = false
}

function deleteParameter(index: number) {
  newCondition.value.parameters.splice(index, 1)
}

function buildConditionPayload(): NewConditionPayload {
  const payload: NewConditionPayload = {
    conditionName: newCondition.value.conditionName.trim(),
    schemaVersion: newCondition.value.schemaVersion,
    parameters: newCondition.value.parameters.map((p) => ({
      ...p,
      parameterName: p.parameterName.trim(),
    })),
  }
  if (newCondition.value.entityType) {
    payload.entityType = newCondition.value.entityType
  }
  if (newCondition.value.entityRole) {
    payload.entityRole = newCondition.value.entityRole
  }
  return payload
}

function submitRule() {
  if (!canSubmitRule.value) return
  const payload = buildConditionPayload()
  if (isEditMode.value) {
    if (!props.initialCondition?.conditionId) return
    emit('update-rule', { conditionId: props.initialCondition.conditionId, data: payload })
  } else {
    emit('add-rule', payload)
  }
  newCondition.value = getDefaultNewCondition()
  draftParameter.value = defaultParam()
  draftOperator.value = ''
  draftTarget.value = ''
  resetSetConstraintDraft()
  selectedDomainPath.value = ''
  domainFieldSearch.value = ''
  showDomainFieldOptions.value = false
}

function buildDraftOperators() {
  if (usesSetConstraintEditor.value) {
    if (!isSetOperator(draftOperator.value)) return []
    const targets = setConstraintTargets()
    return targets.length
      ? [
          {
            operate: draftOperator.value,
            targets,
          },
        ]
      : []
  }
  if (!draftOperator.value) return []
  return [
    {
      operate: draftOperator.value,
      targets: [parseDraftTarget()],
    },
  ]
}

function setConstraintTargets(): string[] {
  if (!isSetOperator(draftOperator.value)) return []
  if (valueOptions.value.length) return [...draftSetTargets.value]
  return draftTokenTargets.value
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean)
}

function parseDraftTarget(): unknown {
  const raw = draftTarget.value.trim()
  switch (draftParameter.value.type) {
    case 'decimal':
    case 'integer':
      return Number(normalizeDecimalInput(raw))
    case 'boolean':
      return raw === 'true'
    default:
      return raw
  }
}

function validateDraftOperator(): string {
  if (usesSetConstraintEditor.value) {
    const targets = setConstraintTargets()
    if (!targets.length) return ''
    const invalid = targets.find((target) => !targetMatchesConstraint(target))
    return invalid ? `"${invalid}" does not match the field format.` : ''
  }
  if (!draftOperator.value) return ''
  const raw = draftTarget.value.trim()
  if (raw === '') return 'Target is required.'
  if (draftParameter.value.type === 'decimal' || draftParameter.value.type === 'integer') {
    const number = Number(normalizeDecimalInput(raw))
    if (!Number.isFinite(number)) return 'Target must be numeric.'
    if (draftParameter.value.type === 'integer' && !Number.isInteger(number)) return 'Target must be an integer.'
    const constraint = selectedDomainField.value?.valueConstraint
    if (constraint?.min !== undefined && number < constraint.min) return `Minimum is ${constraint.min}.`
    if (constraint?.max !== undefined && number > constraint.max) return `Maximum is ${constraint.max}.`
  }
  if (draftParameter.value.type === 'boolean' && raw !== 'true' && raw !== 'false') {
    return 'Use true or false.'
  }
  return ''
}

function targetMatchesConstraint(target: string): boolean {
  const constraint = selectedDomainField.value?.valueConstraint
  if (!constraint) return true
  if (valueOptions.value.length) return valueOptions.value.some((option) => option.value === target)
  if (constraint.pattern) return new RegExp(constraint.pattern).test(target)
  if (constraint.format === 'iso-3166-1-alpha-3') return /^[A-Z]{3}$/.test(target)
  if (constraint.format === 'iso-4217') return /^[A-Z]{3}$/.test(target)
  return true
}

function normalizeDecimalInput(value: string): string {
  return value.replace(',', '.')
}

function operatorOptionsForType(type: SemanticConditionParameter['type']) {
  const equality = [
    { value: 'Equals' as SemanticOperateType, label: 'Must equal' },
    { value: 'NotEquals' as SemanticOperateType, label: 'Must not equal' },
  ]
  if (type === 'decimal' || type === 'integer' || type === 'date') {
    return [
      { value: 'GreaterThan' as SemanticOperateType, label: 'Must be greater than' },
      { value: 'GreaterThanOrEqual' as SemanticOperateType, label: 'Must be at least' },
      { value: 'LessThan' as SemanticOperateType, label: 'Must be less than' },
      { value: 'LessThanOrEqual' as SemanticOperateType, label: 'Must be at most' },
      ...equality,
    ]
  }
  if (type === 'string' || type === 'enum') {
    return [
      ...equality,
      { value: 'Contains' as SemanticOperateType, label: 'Must contain' },
      { value: 'MatchesRegex' as SemanticOperateType, label: 'Must match pattern' },
    ]
  }
  return equality
}

function setOperatorOptions() {
  return [
    { value: 'In' as SemanticOperateType, label: 'Allow only' },
    { value: 'NotIn' as SemanticOperateType, label: 'Exclude' },
  ]
}

function isSetOperator(operator: SemanticOperateType | ''): operator is Extract<SemanticOperateType, 'In' | 'NotIn'> {
  return operator === 'In' || operator === 'NotIn'
}

function formatOperatorConstraint(operator: SemanticConditionParameter['operators'][number]): string {
  const operate = typeof operator === 'string' ? operator : operator.operate
  const targets = typeof operator === 'string' ? [] : (operator.targets ?? [])
  return `${operatorLabel(operate)} ${targets.join(', ')}`
}

function operatorLabel(operator: DcsOperator): string {
  switch (operator) {
    case 'Equals':
      return 'must equal'
    case 'NotEquals':
      return 'must not equal'
    case 'In':
      return 'allow only'
    case 'NotIn':
      return 'exclude'
    case 'GreaterThan':
      return 'must be greater than'
    case 'GreaterThanOrEqual':
      return 'must be at least'
    case 'LessThan':
      return 'must be less than'
    case 'LessThanOrEqual':
      return 'must be at most'
    case 'Contains':
      return 'must contain'
    case 'MatchesRegex':
      return 'must match pattern'
    default:
      return operator
  }
}

function formatSelectedValue(value: string): string {
  return formatValueOption(value, valueOptions.value)
}

function resetSetConstraintDraft() {
  clearSetConstraintTargets()
}

function clearSetConstraintTargets() {
  draftSetTargets.value = []
  draftTokenTargets.value = ''
  valueOptionSearch.value = ''
}
</script>
