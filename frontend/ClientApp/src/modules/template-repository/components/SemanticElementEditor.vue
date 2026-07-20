<script setup lang="ts">
import SemanticRuleList from '@template-repository/components/clauses-editor/SemanticRuleList.vue'
import TypedClauseForm from '@template-repository/components/clauses-editor/TypedClauseForm.vue'
import { getSemanticConditionsFromTemplateData, useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { DOMAIN_FIELD_SHAPE, domainFieldShape, fieldInstanceValue } from '@template-repository/utils/domain-field-shape'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { useClauseCatalogStore } from '@/stores/clause-catalog-store'
import type { DcsTypedClauseInstance } from '@/models/dcs-jsonld'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'
import type { DomainFieldDefinition, SemanticCondition } from '@template-repository/models/contract-template'

/**
 * One shacl-form editor for every semantic element: hub-registered domain
 * fields and typed clauses (incl. ODRL obligation clauses) are all authored
 * through the same <shacl-form>. A field renders as a single optional-value
 * shape generated from the ontology; a clause renders from its hub NodeShape.
 * Placeholders bind to the fields this produces exactly as before.
 */

const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'

const store = useDcsDraftStore()
const catalog = useClauseCatalogStore()
const { semanticConditions, subTemplateSnapshots } = storeToRefs(store)
const { clauses: clauseTypes, shapes: clauseShapes } = storeToRefs(catalog)
onMounted(() => catalog.refresh())

const allConditions = computed<SemanticCondition[]>(() => [
  ...semanticConditions.value,
  ...subTemplateSnapshots.value.flatMap((snapshot) => getSemanticConditionsFromTemplateData(snapshot.template_data)),
])

const selected = ref<
  { kind: 'field'; field: DomainFieldDefinition } | { kind: 'clause'; clause: ClauseCatalogType } | null
>(null)

const activeForm = computed<{ clause: ClauseCatalogType; shapes: string } | null>(() => {
  if (!selected.value) return null
  if (selected.value.kind === 'clause') return { clause: selected.value.clause, shapes: clauseShapes.value }
  const field = selected.value.field
  return {
    clause: { type: field.ontologyId, label: field.label, shape: `${DCS}${DOMAIN_FIELD_SHAPE.split(':')[1]}` },
    shapes: domainFieldShape(field),
  }
})

function selectField(field: DomainFieldDefinition) {
  selected.value = selected.value?.kind === 'field' && selected.value.field === field ? null : { kind: 'field', field }
}

function selectClause(clause: ClauseCatalogType) {
  selected.value =
    selected.value?.kind === 'clause' && selected.value.clause === clause ? null : { kind: 'clause', clause }
}

async function onSubmit(payload: { clauseType: string; title: string; instance: DcsTypedClauseInstance }) {
  if (selected.value?.kind === 'field') {
    addFieldRequirement(selected.value.field, fieldInstanceValue(payload.instance))
  } else {
    await store.addTypedClause(payload)
  }
  selected.value = null
}

function addFieldRequirement(field: DomainFieldDefinition, value: string | number | boolean | undefined) {
  store.addSemanticCondition({
    conditionName: field.label,
    schemaVersion: 'v1',
    parameters: [
      {
        parameterName: field.parameterName,
        type: field.type,
        fieldIri: field.ontologyId,
        valueConstraint: field.valueConstraint,
        isRequired: true,
        operators: [],
        value,
      },
    ],
  })
}
</script>

<template>
  <div class="space-y-4">
    <div class="space-y-2">
      <h4 class="text-xs font-semibold text-base-content/70">Add a field</h4>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="field in ONTOLOGY_DOMAIN_FIELDS"
          :key="field.ontologyId"
          type="button"
          class="btn btn-xs"
          :class="selected?.kind === 'field' && selected.field === field ? 'btn-primary' : 'btn-outline'"
          @click="selectField(field)"
        >
          {{ field.label }}
        </button>
      </div>
    </div>

    <div class="space-y-2">
      <h4 class="text-xs font-semibold text-base-content/70">Add a typed clause</h4>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="clause in clauseTypes"
          :key="clause.type"
          type="button"
          class="btn btn-xs"
          :class="selected?.kind === 'clause' && selected.clause === clause ? 'btn-primary' : 'btn-outline'"
          @click="selectClause(clause)"
        >
          {{ clause.label }}
        </button>
        <p v-if="!clauseTypes.length" class="text-xs text-base-content/50 italic">
          No typed clauses registered in the Semantic Hub.
        </p>
      </div>
    </div>

    <div v-if="activeForm" class="rounded border border-base-300 p-3">
      <TypedClauseForm
        :clause="activeForm.clause"
        :shapes="activeForm.shapes"
        :submit-label="selected?.kind === 'field' ? 'Add field' : 'Add typed clause'"
        @submit="onSubmit"
      />
    </div>

    <SemanticRuleList
      title="Current data requirements"
      empty-message="No data requirements yet — add a field or clause above."
      :conditions="allConditions"
    />
  </div>
</template>
