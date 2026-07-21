<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, ref } from 'vue'
import ClauseTextEditor from '@template-repository/components/clauses-editor/ClauseTextEditor.vue'
import OdrlRuleBuilder from '@template-repository/components/clauses-editor/OdrlRuleBuilder.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import {
  type HubAsset,
  ONTOLOGY_ASSETS,
  ONTOLOGY_DOMAIN_FIELDS,
} from '@template-repository/utils/ontology-domain-fields'
import type { DcsContentSegment, OdrlRule } from '@/models/dcs-jsonld'
import type { DomainFieldDefinition, SemanticCondition } from '@template-repository/models/contract-template'

/**
 * One clause, the SRS split editor: human prose with placeholders on the left,
 * its machine-readable ODRL meaning on the right. Both sides reference objects
 * picked from the Semantic Hub — data fields (flat domain vocabulary) and
 * assets (a shape's target class, e.g. an imported Gaia-X ServiceOffering,
 * whose properties become fields). A clause's meaning IS an ODRL rule; an asset
 * is what that rule targets.
 */

const store = useDcsDraftStore()
const { partyAnchors, contractTargetIri } = storeToRefs(store)

interface ClauseField {
  id: string
  field: DomainFieldDefinition
  /** Set when this field is a property of a declared asset. */
  assetLocalId?: string
}
interface ClauseAsset {
  id: string
  asset: HubAsset
}

const title = ref('')
const content = ref<DcsContentSegment[]>([])
const clauseFields = ref<ClauseField[]>([])
const clauseAssets = ref<ClauseAsset[]>([])
const rule = ref<OdrlRule | null>(null)
const objectToAdd = ref('')

const uuid = () => `urn:uuid:${crypto.randomUUID()}`
const localName = (iri: string) => iri.replace(/^.*[:#/]/, '')

// One picker of hub objects: an object may be an asset (a shape's target class,
// carrying properties) or a bare data field (a property). Its role — an ODRL
// target, a constraint operand — is decided by how it is wired into the rule,
// not by which list it came from. Assets are marked ▣ because they carry a
// shape whose properties come along.
const objectGroups = computed(() => {
  const groups = new Map<string, { value: string; label: string }[]>()
  const push = (source: string | undefined, option: { value: string; label: string }) => {
    const key = source ?? 'Semantic Hub'
    const group = groups.get(key)
    if (group) group.push(option)
    else groups.set(key, [option])
  }
  for (const asset of ONTOLOGY_ASSETS)
    push(asset.source?.name, { value: `asset:${asset.id}`, label: `▣ ${asset.label}` })
  for (const field of ONTOLOGY_DOMAIN_FIELDS)
    push(field.source?.name, { value: `field:${field.ontologyId}`, label: field.label })
  return [...groups.entries()].map(([name, entries]) => ({ name, entries }))
})

function addObject() {
  const picked = objectToAdd.value
  objectToAdd.value = ''
  if (picked.startsWith('asset:')) {
    const asset = ONTOLOGY_ASSETS.find((a) => a.id === picked.slice('asset:'.length))
    if (!asset) return
    // Declaring an asset makes it an ODRL target and brings in its shape's properties as fields.
    const assetLocalId = uuid()
    clauseAssets.value.push({ id: assetLocalId, asset })
    for (const property of asset.properties) {
      clauseFields.value.push({ id: uuid(), field: property, assetLocalId })
    }
  } else if (picked.startsWith('field:')) {
    const field = ONTOLOGY_DOMAIN_FIELDS.find((f) => f.ontologyId === picked.slice('field:'.length))
    if (field) clauseFields.value.push({ id: uuid(), field })
  }
}

function removeField(id: string) {
  clauseFields.value = clauseFields.value.filter((cf) => cf.id !== id)
}
function removeAsset(assetLocalId: string) {
  clauseAssets.value = clauseAssets.value.filter((a) => a.id !== assetLocalId)
  clauseFields.value = clauseFields.value.filter((cf) => cf.assetLocalId !== assetLocalId)
}

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
const assetAnchors = computed(() => clauseAssets.value.map((a) => ({ id: a.id, label: a.asset.label })))

const canSave = computed(() => !!title.value.trim() && content.value.length > 0)

function save() {
  if (!canSave.value) return
  store.addClauseWithMeaning({
    title: title.value.trim(),
    content: content.value,
    fields: [
      ...clauseFields.value.map((cf) => ({
        id: cf.id,
        parameterName: cf.field.parameterName,
        domainFieldIri: cf.field.ontologyId,
      })),
      ...clauseAssets.value.map((a) => ({
        id: a.id,
        parameterName: localName(a.asset.id),
        domainFieldIri: a.asset.id,
      })),
    ],
    rule: rule.value,
  })
  title.value = ''
  content.value = []
  clauseFields.value = []
  clauseAssets.value = []
  rule.value = null
}
</script>

<template>
  <div class="space-y-3" data-testid="split-clause-editor">
    <input v-model="title" type="text" placeholder="Clause title" class="input-bordered input input-sm w-full" />

    <div class="space-y-2 rounded bg-base-200/50 p-2">
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-xs text-base-content/60">Objects (Semantic Hub):</span>
        <select v-model="objectToAdd" class="select-bordered select select-xs" @change="addObject">
          <option value="">+ add object…</option>
          <optgroup v-for="group in objectGroups" :key="group.name" :label="group.name">
            <option v-for="o in group.entries" :key="o.value" :value="o.value">{{ o.label }}</option>
          </optgroup>
        </select>
        <span class="text-[10px] text-base-content/40">▣ = asset (carries a shape)</span>
      </div>

      <div v-if="clauseAssets.length" class="flex flex-wrap items-center gap-1">
        <span v-for="ca in clauseAssets" :key="ca.id" class="badge gap-1 badge-sm badge-primary">
          ▣ {{ ca.asset.label }}
          <button type="button" class="text-primary-content/70" @click="removeAsset(ca.id)">✕</button>
        </span>
      </div>
      <div v-if="clauseFields.length" class="flex flex-wrap items-center gap-1">
        <span v-for="cf in clauseFields" :key="cf.id" class="badge gap-1 badge-outline badge-sm">
          {{ cf.field.label }}
          <button type="button" class="text-error" @click="removeField(cf.id)">✕</button>
        </span>
      </div>
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
          :assets="assetAnchors"
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
