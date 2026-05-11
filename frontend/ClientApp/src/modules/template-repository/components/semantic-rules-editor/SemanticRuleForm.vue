<template>

  <h3 class="text-sm font-semibold text-base-content/80 mb-4">{{ formTitle }}</h3>
  <div class="space-y-4">
    <div>
      <label class="label-text text-xs text-base-content/60 block mb-1">Rule name
        <RequiredIndicator />
      </label>
      <input v-model="newCondition.conditionName" type="text" class="input input-bordered input-sm w-full"
        :class="{ 'input-error': isRuleNameDuplicate }" placeholder="" />
      <p class="text-xs text-base-content/50 mt-0.5">Used when selecting this rule for a clause.</p>
      <p v-if="isRuleNameDuplicate" class="text-xs text-error mt-0.5">Rule name already exists.</p>
    </div>

    <div class="space-y-4">
      <p class="label-text text-xs text-base-content/60 mb-1">Parameters</p>
      <div
        class="grid grid-cols-1 md:grid-cols-12 gap-x-3 p-3 rounded-lg border-2 border-dashed border-base-300 bg-base-200/50">
        <p class="md:col-span-12 text-xs font-medium text-base-content/70 mb-2">New parameter</p>
        <div class="md:col-span-4 flex flex-col gap-1">
          <label class="label py-0 min-h-0">
            <span class="label-text text-xs text-base-content/60">Domain field
              <RequiredIndicator />
            </span>
          </label>
          <div class="relative">
            <input
              v-model="domainFieldSearch"
              type="search"
              class="input input-bordered input-sm w-full h-9"
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
              class="absolute z-30 mt-1 w-full rounded-lg border border-base-300 bg-base-100 shadow-lg max-h-64 overflow-y-auto"
            >
              <template v-if="groupedDomainFields.length">
                <section v-for="group in groupedDomainFields" :key="group.name">
                  <div class="sticky top-0 z-10 bg-base-100 px-3 py-1 border-b border-base-200">
                    <p class="text-xs font-semibold uppercase text-base-content/50">{{ group.name }}</p>
                  </div>
                  <button
                    v-for="field in group.fields"
                    :key="field.semanticPath"
                    type="button"
                    class="w-full text-left px-3 py-2 hover:bg-base-200 transition-colors border-b border-base-200 last:border-b-0"
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
        <div class="md:col-span-3 flex flex-col gap-1">
          <label class="label py-0 min-h-0">
            <span class="label-text text-xs text-base-content/60">Type
              <RequiredIndicator />
            </span>
          </label>
          <select v-model="draftParameter.type" class="select select-bordered select-sm w-full h-9" disabled>
            <option value="date">Date</option>
            <option value="string">Text</option>
            <option value="decimal">Decimal</option>
            <option value="integer">Integer</option>
          </select>
        </div>
        <div class="md:col-span-2 flex flex-col gap-1">
          <label class="label py-0 min-h-0">
            <span class="label-text text-xs text-base-content/60">Required</span>
          </label>
          <div class="flex items-center h-9">
            <label class="label cursor-pointer justify-start gap-2 py-0 min-h-0 h-auto">
              <input v-model="draftParameter.isRequired" type="checkbox"
                class="checkbox checkbox-sm checkbox-primary" />
              <span class="label-text text-xs">Required</span>
            </label>
          </div>
        </div>
        <div class="md:col-span-2 flex flex-col gap-1">
          <label class="label py-0 min-h-0 invisible">
            <span class="label-text text-xs">Add</span>
          </label>
          <div class="h-9 flex items-center">
            <button type="button" class="btn btn-secondary btn-sm w-full whitespace-nowrap" :disabled="!canAddParameter"
              @click="addParameter">
              + Add parameter
            </button>
          </div>
        </div>
      </div>

      <!-- Added parameters -->
      <div v-if="newCondition.parameters.length" class="space-y-2">
        <p class="text-xs font-medium text-base-content/70">Added parameters</p>
        <ul class="space-y-2">
          <li v-for="(param, idx) in newCondition.parameters" :key="idx"
            class="flex items-center gap-3 py-2.5 px-3 rounded-lg bg-base-100 border border-base-300">
            <span class="font-mono text-sm font-medium border border-base-300 rounded px-2 py-0.5 bg-base-200/50">{{
              param.parameterName }}</span>
            <span class="text-xs text-base-content/50">{{ param.semanticPath }}</span>
            <span v-if="param.valueConstraint" class="text-xs text-base-content/50">
              {{ formatValueConstraint(param.valueConstraint) }}
            </span>
            <span class="badge badge-ghost badge-sm">{{ param.type }}</span>
            <span class="text-xs text-base-content/50">{{ param.isRequired ? 'required' : 'optional' }}</span>
            <button type="button" class="btn btn-ghost btn-xs text-error ml-auto shrink-0" aria-label="Delete parameter"
              @click="deleteParameter(idx)"> ✕ </button>
          </li>
        </ul>
      </div>
    </div>

    <div class="flex justify-between items-center">
      <button v-if="isEditMode" type="button" class="btn btn-outline btn-xs" @click="$emit('cancel')">Cancel</button>
      <span v-else />
      <button type="button" class="btn btn-secondary btn-sm" :disabled="!canSubmitRule" @click="submitRule">
        {{ submitLabel }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import RequiredIndicator from '@core/components/RequiredIndicator.vue'
import {
  type SemanticCondition,
  type SemanticConditionParameter,
  type DomainSemanticPath,
  type SemanticValueConstraint,
  FACIS_DOMAIN_FIELDS,
  SEMANTIC_CONDITION_SCHEMA_VERSION,
} from '@/modules/template-repository/models/contract-template'

type NewConditionPayload = Omit<SemanticCondition, 'conditionId'>

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
  const defaultField = FACIS_DOMAIN_FIELDS[0]!
  return {
    parameterName: '',
    type: defaultField.type,
    schemaRef: defaultField.schemaRef,
    semanticPath: defaultField.semanticPath,
    valueConstraint: cloneValueConstraint(defaultField.valueConstraint),
    isRequired: true,
    operators: [],
    value: undefined,
  }
}

function getDefaultNewCondition(): NewConditionPayload {
  return {
    conditionName: '',
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    parameters: [],
  }
}

const newCondition = ref<NewConditionPayload>(getDefaultNewCondition())
const draftParameter = ref<SemanticConditionParameter>(defaultParam())
const selectedDomainPath = ref<DomainSemanticPath | ''>('')
const domainFieldSearch = ref('')
const showDomainFieldOptions = ref(false)
const isEditMode = computed(() => props.mode === 'edit')
const formTitle = computed(() => (isEditMode.value ? 'Edit rule' : 'New rule'))
const submitLabel = computed(() => (isEditMode.value ? 'Save changes' : 'Add rule'))
const selectedDomainField = computed(() =>
  FACIS_DOMAIN_FIELDS.find((field) => field.semanticPath === selectedDomainPath.value),
)
const groupedDomainFields = computed(() => {
  const query = domainFieldSearch.value.trim().toLowerCase()
  const filtered = FACIS_DOMAIN_FIELDS.filter((field) => {
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
    ].some((value) =>
      value.toLowerCase().includes(query),
    )
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
      selectedDomainPath.value = ''
      domainFieldSearch.value = ''
      showDomainFieldOptions.value = false
      return
    }
    newCondition.value = {
      conditionName: props.initialCondition.conditionName,
      schemaVersion: props.initialCondition.schemaVersion,
      parameters: props.initialCondition.parameters.map((p) => ({ ...p })),
    }
    draftParameter.value = defaultParam()
    selectedDomainPath.value = ''
    domainFieldSearch.value = ''
    showDomainFieldOptions.value = false
  },
  { immediate: true },
)

watch(selectedDomainPath, (path) => {
  const field = FACIS_DOMAIN_FIELDS.find((item) => item.semanticPath === path)
  if (!field) {
    draftParameter.value = defaultParam()
    return
  }
  draftParameter.value = {
    ...draftParameter.value,
    parameterName: field.semanticPath.split('.').join('_'),
    schemaRef: field.schemaRef,
    semanticPath: field.semanticPath,
    valueConstraint: cloneValueConstraint(field.valueConstraint),
    type: field.type,
  }
  domainFieldSearch.value = formatDomainFieldLabel(field)
})

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
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

function formatDomainFieldLabel(field: (typeof FACIS_DOMAIN_FIELDS)[number]) {
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
  return newCondition.value.parameters.some(
    (p) => p.parameterName.trim().toLowerCase() === lower
  )
})

const canAddParameter = computed(() => {
  const name = draftParameter.value.parameterName?.trim()
  if (!name) return false
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
      c.conditionId !== currentConditionId &&
      c.conditionName.trim().toLowerCase() === lower
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
  })
  draftParameter.value = defaultParam()
  selectedDomainPath.value = ''
  domainFieldSearch.value = ''
  showDomainFieldOptions.value = false
}

function deleteParameter(index: number) {
  newCondition.value.parameters.splice(index, 1)
}

function buildConditionPayload(): NewConditionPayload {
  return {
    conditionName: newCondition.value.conditionName.trim(),
    schemaVersion: newCondition.value.schemaVersion,
    parameters: newCondition.value.parameters.map((p) => ({
      ...p,
      parameterName: p.parameterName.trim(),
    })),
  }
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
  selectedDomainPath.value = ''
  domainFieldSearch.value = ''
  showDomainFieldOptions.value = false
}
</script>
