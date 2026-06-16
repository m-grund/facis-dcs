<template>
  <div class="space-y-6">
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Add data requirement</h3>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-5">
        <button
          v-for="action in requirementActions"
          :key="action.id"
          type="button"
          class="btn btn-outline h-auto min-h-0 justify-start px-3 py-2 text-left"
          :class="{ 'btn-primary': requirementDraft?.action.id === action.id }"
          @click="startRequirementDraft(action)"
        >
          <span class="flex min-w-0 flex-col items-start">
            <span class="truncate text-sm font-medium">{{ action.label }}</span>
            <span class="truncate text-xs font-normal opacity-60">{{ actionSummary(action) }}</span>
          </span>
        </button>
      </div>

      <div v-if="requirementDraft" class="mt-4 rounded-lg border border-base-300 bg-base-200/30 p-3">
        <div class="mb-3 flex flex-wrap items-end gap-3">
          <label class="flex min-w-56 flex-1 flex-col gap-1">
            <span class="label-text text-xs text-base-content/60">Requirement</span>
            <input v-model="requirementDraft.name" type="text" class="input-bordered input input-sm w-full" />
          </label>
          <label v-if="requirementDraft.action.roleRequired" class="flex min-w-44 flex-col gap-1">
            <span class="label-text text-xs text-base-content/60">Role *</span>
            <select
              v-model="requirementDraft.role"
              class="select-bordered select select-sm w-full"
              @change="syncDraftNameWithRole"
            >
              <option value="">Select role</option>
              <option v-for="role in roleOptions" :key="role.value" :value="role.value">
                {{ role.label }}
              </option>
            </select>
          </label>
          <div class="ml-auto flex gap-2">
            <button type="button" class="btn btn-ghost btn-sm" @click="cancelRequirementDraft">Cancel</button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="!canAddRequirementDraft"
              @click="addRequirementDraft"
            >
              Add requirement
            </button>
          </div>
        </div>

        <div class="space-y-3">
          <div
            v-for="(parameter, index) in requirementDraft.parameters"
            :key="parameter.semanticPath"
            class="grid grid-cols-1 items-start gap-x-3 gap-y-3 rounded border border-base-300 bg-base-100 p-3 md:grid-cols-12"
          >
            <div class="min-w-0 md:col-span-4">
              <p class="truncate text-sm font-medium text-base-content">{{ semanticParameterLabel(parameter) }}</p>
              <p class="truncate text-xs text-base-content/50">{{ parameter.semanticPath }}</p>
              <p v-if="parameter.valueConstraint" class="truncate text-xs text-base-content/50">
                {{ formatValueConstraint(parameter.valueConstraint) }}
              </p>
            </div>
            <label class="flex flex-col gap-1 md:col-span-2">
              <span class="label-text text-xs text-base-content/60">Required</span>
              <span class="flex h-9 items-center gap-2">
                <input v-model="parameter.isRequired" type="checkbox" class="checkbox checkbox-sm checkbox-primary" />
                <span class="text-xs text-base-content/60">{{ parameter.isRequired ? 'Required' : 'Optional' }}</span>
              </span>
            </label>
            <ParameterObligationEditor
              :parameter="parameter"
              :operators="parameter.operators"
              @update:operators="updateDraftParameterOperators(index, $event)"
              @validity-change="updateDraftParameterValidity(parameter.semanticPath, $event)"
            />
          </div>
        </div>
      </div>
    </section>

    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Data requirements</h3>
      <p v-if="!conditionItems.length" class="py-6 text-center text-xs text-base-content/40 italic">
        No data requirements defined yet.
      </p>
      <div v-else class="space-y-4">
        <article
          v-for="item in conditionItems"
          :key="item.condition.conditionId"
          class="rounded-lg border border-base-300 bg-base-200/30 p-3"
        >
          <div class="mb-3 flex items-start justify-between gap-3">
            <div class="min-w-0">
              <h4 class="truncate text-sm font-semibold text-base-content">{{ item.condition.conditionName }}</h4>
              <div class="mt-1 flex flex-wrap gap-1">
                <span v-if="item.condition.entityType" class="badge badge-outline badge-sm">
                  {{ item.condition.entityType }}
                </span>
                <span v-if="item.condition.entityRole" class="badge badge-outline badge-sm">
                  {{ item.condition.entityRole }}
                </span>
                <span class="badge badge-ghost badge-sm">
                  used in {{ item.usedInClauseCount }} clause{{ item.usedInClauseCount === 1 ? '' : 's' }}
                </span>
              </div>
            </div>
            <button
              v-if="uiStore.isTemplateEditable"
              type="button"
              class="btn btn-ghost btn-xs text-error"
              @click="deleteRequirement(item)"
            >
              Delete
            </button>
          </div>

          <div class="space-y-3">
            <div
              v-for="(parameter, index) in item.condition.parameters"
              :key="parameter.semanticPath"
              class="grid grid-cols-1 items-start gap-x-3 gap-y-3 rounded border border-base-300 bg-base-100 p-3 md:grid-cols-12"
            >
              <div class="min-w-0 md:col-span-4">
                <p class="truncate text-sm font-medium text-base-content">{{ semanticParameterLabel(parameter) }}</p>
                <p class="truncate text-xs text-base-content/50">{{ parameter.semanticPath }}</p>
                <p v-if="parameter.valueConstraint" class="truncate text-xs text-base-content/50">
                  {{ formatValueConstraint(parameter.valueConstraint) }}
                </p>
              </div>
              <label class="flex flex-col gap-1 md:col-span-2">
                <span class="label-text text-xs text-base-content/60">Required</span>
                <span class="flex h-9 items-center gap-2">
                  <input
                    v-model="parameter.isRequired"
                    type="checkbox"
                    class="checkbox checkbox-sm checkbox-primary"
                    :disabled="!uiStore.isTemplateEditable || !!item.subTemplateRef"
                  />
                  <span class="text-xs text-base-content/60">{{ parameter.isRequired ? 'Required' : 'Optional' }}</span>
                </span>
              </label>
              <ParameterObligationEditor
                :parameter="parameter"
                :operators="parameter.operators"
                @update:operators="updateParameterOperators(item.condition, index, $event)"
              />
            </div>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import {
  SEMANTIC_CONDITION_SCHEMA_VERSION,
  isClauseBlock,
  type DomainFieldDefinition,
  type SemanticCondition,
  type SemanticConditionParameter,
  type SemanticEntityRole,
  type SemanticParameterOperator,
  type SemanticValueConstraint,
} from '@template-repository/models/contract-template'
import type { SubTemplateReference } from '@template-repository/models/template-draft-store'
import ParameterObligationEditor from '@template-repository/components/semantic-rules-editor/ParameterObligationEditor.vue'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import {
  ONTOLOGY_DOMAIN_TYPES,
  type OntologyDomainType,
  buildOntologyDomainTypeParameters,
  ontologyRoleOptions,
  roleLabelFor,
} from '@template-repository/utils/ontology-domain-types'
import { semanticParameterLabel } from '@template-repository/utils/semantic-parameter-label'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'

interface RequirementItem {
  condition: SemanticCondition
  usedInClauseCount: number
  subTemplateRef?: SubTemplateReference
}

type RequirementActionId =
  | 'contract-party'
  | 'payment-term'
  | 'jurisdiction'
  | 'sla-objective'
  | 'signature-requirement'

interface RequirementAction {
  id: RequirementActionId
  label: string
  roleRequired: boolean
  domainType?: OntologyDomainType
  entityType?: string
  fields: readonly DomainFieldDefinition[]
}

interface RequirementDraft {
  action: RequirementAction
  name: string
  role: SemanticEntityRole | ''
  parameters: SemanticConditionParameter[]
  parameterValidity: Record<string, boolean>
}

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { semanticConditions: mainSemanticConditions, documentBlocks, subTemplateSnapshots } = storeToRefs(store)

const roleOptions = ontologyRoleOptions
const requirementDraft = ref<RequirementDraft | null>(null)
const companyDomainType = ONTOLOGY_DOMAIN_TYPES.find((type) => type.roleRequired)
const requirementActions = buildRequirementActions()
const canAddRequirementDraft = computed(() => {
  const draft = requirementDraft.value
  if (!draft) return false
  if (!draft.name.trim()) return false
  if (draft.action.roleRequired && !draft.role) return false
  if (!draft.parameters.length) return false
  return Object.values(draft.parameterValidity).every((isValid) => isValid)
})

const allBlocks = computed(() => {
  const subTemplateBlocks = subTemplateSnapshots.value.flatMap(
    (subTemplate) => subTemplate.template_data?.documentBlocks ?? [],
  )
  return [...documentBlocks.value, ...subTemplateBlocks]
})

const clauseCountByConditionId = computed(() => {
  const counts: Record<string, number> = {}
  for (const block of allBlocks.value) {
    if (!isClauseBlock(block)) continue
    for (const id of block.conditionIds) counts[id] = (counts[id] ?? 0) + 1
  }
  return counts
})

const conditionItems = computed<RequirementItem[]>(() => {
  const main = mainSemanticConditions.value.map((condition) => ({
    condition,
    usedInClauseCount: clauseCountByConditionId.value[condition.conditionId] ?? 0,
  }))
  const subTemplateItems = subTemplateSnapshots.value.flatMap((template) => {
    const conditions = template.template_data?.semanticConditions ?? []
    return conditions.map((condition) => ({
      condition,
      usedInClauseCount: clauseCountByConditionId.value[condition.conditionId] ?? 0,
      subTemplateRef: {
        did: template.did,
        version: template.version,
        document_number: template.document_number,
      },
    }))
  })
  return [...main, ...subTemplateItems]
})

function startRequirementDraft(action: RequirementAction) {
  const parameters = action.domainType
    ? buildOntologyDomainTypeParameters(action.domainType)
    : action.fields.map((field) => parameterFromField(field))
  requirementDraft.value = {
    action,
    name: defaultRequirementName(action, ''),
    role: '',
    parameters,
    parameterValidity: Object.fromEntries(parameters.map((parameter) => [parameter.semanticPath, true])),
  }
}

function cancelRequirementDraft() {
  requirementDraft.value = null
}

function syncDraftNameWithRole() {
  const draft = requirementDraft.value
  if (!draft || !draft.action.roleRequired) return
  draft.name = defaultRequirementName(draft.action, draft.role)
}

function addRequirementDraft() {
  const draft = requirementDraft.value
  if (!draft || !canAddRequirementDraft.value) return
  store.addSemanticCondition({
    conditionName: draft.name.trim(),
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    ...(draft.action.entityType ? { entityType: draft.action.entityType } : {}),
    ...(draft.role ? { entityRole: draft.role } : {}),
    parameters: draft.parameters.map(cloneParameter),
  })
  requirementDraft.value = null
}

function parameterFromField(field: DomainFieldDefinition): SemanticCondition['parameters'][number] {
  return {
    parameterName: field.semanticPath,
    type: field.type,
    schemaRef: field.schemaRef,
    semanticPath: field.semanticPath,
    valueConstraint: cloneValueConstraint(field.valueConstraint),
    uiMetadata: { label: field.label },
    isRequired: true,
    operators: [],
    value: undefined,
  }
}

function cloneParameter(parameter: SemanticConditionParameter): SemanticConditionParameter {
  return {
    ...parameter,
    valueConstraint: cloneValueConstraint(parameter.valueConstraint),
    uiMetadata: parameter.uiMetadata ? { ...parameter.uiMetadata } : undefined,
    operators: parameter.operators.map((operator) => ({
      ...operator,
      targets: [...operator.targets],
    })),
    value: undefined,
  }
}

function updateDraftParameterOperators(index: number, operators: SemanticParameterOperator[]) {
  const draft = requirementDraft.value
  const parameter = draft?.parameters[index]
  if (!parameter) return
  parameter.operators = operators
}

function updateDraftParameterValidity(semanticPath: string, isValid: boolean) {
  const draft = requirementDraft.value
  if (!draft) return
  draft.parameterValidity[semanticPath] = isValid
}

function updateParameterOperators(condition: SemanticCondition, index: number, operators: SemanticParameterOperator[]) {
  const parameter = condition.parameters[index]
  if (!parameter) return
  parameter.operators = operators
}

function deleteRequirement(item: RequirementItem) {
  store.deleteSemanticCondition(item.condition.conditionId, item.subTemplateRef)
}

function cloneValueConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
  }
}

function buildRequirementActions(): RequirementAction[] {
  return [
    buildContractPartyAction(),
    buildGroupedFieldAction('payment-term', 'Payment term', isPaymentTermField),
    buildGroupedFieldAction('jurisdiction', 'Jurisdiction', (field) => field.semanticPath === 'contract.jurisdiction'),
    buildGroupedFieldAction('sla-objective', 'SLA objective', isSlaObjectiveField),
    buildGroupedFieldAction('signature-requirement', 'Signature requirement', isSignatureRequirementField),
  ].filter((action): action is RequirementAction => !!action && !!action.fields.length)
}

function buildContractPartyAction(): RequirementAction | undefined {
  if (!companyDomainType) return undefined
  return {
    id: 'contract-party',
    label: 'Contract party',
    roleRequired: true,
    domainType: companyDomainType,
    entityType: companyDomainType.entityType,
    fields: companyDomainType.fields,
  }
}

function buildGroupedFieldAction(
  id: RequirementActionId,
  label: string,
  predicate: (field: DomainFieldDefinition) => boolean,
): RequirementAction | undefined {
  const fields = ONTOLOGY_DOMAIN_FIELDS.filter(predicate).sort((left, right) => left.label.localeCompare(right.label))
  if (!fields.length) return undefined
  return {
    id,
    label,
    roleRequired: false,
    fields,
  }
}

function isPaymentTermField(field: DomainFieldDefinition) {
  return localOntologyName(field.statementType ?? '') === 'PaymentTerm' || field.semanticPath.startsWith('contract.payment.')
}

function isSlaObjectiveField(field: DomainFieldDefinition) {
  return localOntologyName(field.statementType ?? '') === 'SLO' || field.semanticPath.startsWith('service.sla.')
}

function isSignatureRequirementField(field: DomainFieldDefinition) {
  return field.semanticPath.startsWith('signature.')
}

function actionSummary(action: RequirementAction) {
  if (action.roleRequired) return 'Role required'
  if (action.fields.length === 1) return action.fields[0]?.label ?? '1 field'
  return `${action.fields.length} fields`
}

function defaultRequirementName(action: RequirementAction, role: SemanticEntityRole | '') {
  if (action.roleRequired) {
    const roleLabel = role ? roleLabelFor(role) : ''
    return roleLabel ? `${roleLabel} Company` : 'Company'
  }
  return action.label
}

function localOntologyName(resource: string) {
  return resource.replace(/^.*[:#/]/, '')
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

</script>
