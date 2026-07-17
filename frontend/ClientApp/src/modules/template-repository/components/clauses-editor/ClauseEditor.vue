<script setup lang="ts">
import ClauseTextEditor from '@template-repository/components/clauses-editor/ClauseTextEditor.vue'
import OdrlRuleBuilder from '@template-repository/components/clauses-editor/OdrlRuleBuilder.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import type { DcsContentSegment, OdrlRule } from '@/models/dcs-jsonld'
import type { DomainFieldDefinition, SemanticCondition } from '@template-repository/models/contract-template'

/**
 * One clause, the SRS split editor (§ template review layout): human prose with
 * placeholders on the left, its machine-readable ODRL meaning on the right.
 * Both sides reference the same data fields — objects picked from the Semantic
 * Hub's registered domain vocabulary — so a field wired into a constraint is
 * the same field a placeholder fills. There is no separate "typed clause": a
 * clause's meaning IS an ODRL rule.
 */

const store = useDcsDraftStore()
const { partyAnchors, contractTargetIri } = storeToRefs(store)

interface ClauseField {
  id: string
  field: DomainFieldDefinition
}

const title = ref('')
const content = ref<DcsContentSegment[]>([])
const clauseFields = ref<ClauseField[]>([])
const rule = ref<OdrlRule | null>(null)
const fieldToAdd = ref('')

function addField() {
  const field = ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === fieldToAdd.value)
  fieldToAdd.value = ''
  if (!field) return
  clauseFields.value.push({ id: `urn:uuid:${crypto.randomUUID()}`, field })
}

// Fields grouped by the hub schema they came from, so an imported schema
// (e.g. Gaia-X) shows as its own group in the picker.
const fieldGroups = computed(() => {
  const groups = new Map<string, DomainFieldDefinition[]>()
  for (const field of ONTOLOGY_DOMAIN_FIELDS) {
    const key = field.source?.name ?? 'Semantic Hub'
    const group = groups.get(key)
    if (group) group.push(field)
    else groups.set(key, [field])
  }
  return [...groups.entries()].map(([name, fields]) => ({ name, fields }))
})

function removeField(id: string) {
  clauseFields.value = clauseFields.value.filter((cf) => cf.id !== id)
}

// The clause's fields drive both panes: placeholders on the left, constraint
// operands on the right — one labelled anchor set, never an IRI.
const proseConditions = computed<SemanticCondition[]>(() =>
  clauseFields.value.map((cf) => ({
    conditionId: cf.id,
    conditionName: cf.field.label,
    schemaVersion: 'v1',
    parameters: [
      {
        parameterName: cf.field.parameterName,
        fieldId: cf.id,
        fieldIri: cf.field.ontologyId,
        type: cf.field.type,
        isRequired: true,
        operators: [],
        value: undefined,
      },
    ],
  })),
)

const fieldAnchors = computed(() => clauseFields.value.map((cf) => ({ id: cf.id, label: cf.field.label })))

const canSave = computed(() => !!title.value.trim() && content.value.length > 0)

function save() {
  if (!canSave.value) return
  store.addClauseWithMeaning({
    title: title.value.trim(),
    content: content.value,
    fields: clauseFields.value.map((cf) => ({
      id: cf.id,
      parameterName: cf.field.parameterName,
      domainFieldIri: cf.field.ontologyId,
    })),
    rule: rule.value,
  })
  title.value = ''
  content.value = []
  clauseFields.value = []
  rule.value = null
}
</script>

<template>
  <div class="space-y-3" data-testid="split-clause-editor">
    <input v-model="title" type="text" placeholder="Clause title" class="input-bordered input input-sm w-full" />

    <div class="flex flex-wrap items-center gap-2 rounded bg-base-200/50 p-2">
      <span class="text-xs text-base-content/60">Data fields (Semantic Hub):</span>
      <select v-model="fieldToAdd" class="select-bordered select select-xs" @change="addField">
        <option value="">+ add field…</option>
        <optgroup v-for="group in fieldGroups" :key="group.name" :label="group.name">
          <option v-for="f in group.fields" :key="f.ontologyId" :value="f.ontologyId">{{ f.label }}</option>
        </optgroup>
      </select>
      <span v-for="cf in clauseFields" :key="cf.id" class="badge gap-1 badge-outline badge-sm">
        {{ cf.field.label }}
        <button type="button" class="text-error" @click="removeField(cf.id)">✕</button>
      </span>
    </div>

    <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
      <div class="rounded border border-base-300 p-3">
        <h5 class="mb-2 text-xs font-semibold text-base-content/70">Human prose</h5>
        <ClauseTextEditor
          :model-value="content"
          :semantic-conditions="proseConditions"
          @update:model-value="content = $event"
        />
      </div>
      <div class="rounded border border-base-300 p-3">
        <h5 class="mb-2 text-xs font-semibold text-base-content/70">Machine-readable meaning (ODRL)</h5>
        <OdrlRuleBuilder
          v-model="rule"
          :fields="fieldAnchors"
          :parties="partyAnchors"
          prose-id=""
          :contract-target-id="contractTargetIri"
        />
      </div>
    </div>

    <div class="flex justify-end">
      <button type="button" class="btn btn-sm btn-primary" :disabled="!canSave" @click="save">Add clause</button>
    </div>
  </div>
</template>
