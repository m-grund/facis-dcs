<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import {
  SEMANTIC_CONDITION_SCHEMA_VERSION,
  isClauseBlock,
  type ClauseBlock,
  type SemanticCondition,
} from '@/modules/template-repository/models/contract-template'
import ExistingClausesList from '@template-repository/components/clauses-editor/ExistingClausesList.vue'
import ClauseEditorForm from '@template-repository/components/clauses-editor/ClauseEditorForm.vue'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import {
  ONTOLOGY_DOMAIN_TYPES,
  buildOntologyDomainTypeClauseText,
  buildOntologyDomainTypeParameters,
  ontologyRoleOptions,
  roleLabelFor,
} from '@template-repository/utils/ontology-domain-types'

const store = useTemplateDraftStore()
const uiStore = useTemplateEditorUiStore()
const { documentBlocks, semanticConditions: mainSemanticConditions, subTemplateSnapshots } = storeToRefs(store)

const editingBlockId = ref<string | null>(null)
const selectedDomainTypeRoles = ref<Record<string, string>>({})
const newClauseTitle = ref('')
const newClauseText = ref('')
const draftDomainTypeCondition = ref<SemanticCondition | null>(null)
const draftDomainTypeMeta = ref<{ schemaRef?: string; semanticPath?: string } | null>(null)
const ontologyDomainTypes = ONTOLOGY_DOMAIN_TYPES
const roleOptions = ontologyRoleOptions

/** Extract conditionIds from clause text placeholders {{conditionId.parameterName}}. */
function conditionIdsFromText(text: string): string[] {
  const set = new Set<string>()
  const re = /\{\{([^}]+)\}\}/g
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    const inner = m[1] ?? ''
    const dot = inner.indexOf('.')
    const conditionId = dot >= 0 ? inner.slice(0, dot) : inner
    if (conditionId) set.add(conditionId)
  }
  return [...set]
}

const clauseBlocks = computed((): ClauseBlock[] => {
  const mainClauses = documentBlocks.value.filter((b): b is ClauseBlock => isClauseBlock(b))
  const subTemplateClauses = subTemplateSnapshots.value.flatMap((subTemplate) =>
    (subTemplate.template_data?.documentBlocks ?? []).filter((block): block is ClauseBlock => isClauseBlock(block)),
  )
  return [...mainClauses, ...subTemplateClauses]
})

const semanticConditions = computed(() => {
  const subTemplateConditions = subTemplateSnapshots.value.flatMap(
    (subTemplate) => subTemplate.template_data?.semanticConditions ?? [],
  )
  return [...mainSemanticConditions.value, ...subTemplateConditions]
})

const newClauseSemanticConditions = computed(() =>
  draftDomainTypeCondition.value
    ? [...semanticConditions.value, draftDomainTypeCondition.value]
    : semanticConditions.value,
)

function addClause(payload: { title: string; text: string }) {
  const text = payload.text
  if (!text.trim()) return
  const conditionIds = conditionIdsFromText(text)
  const draftCondition = draftDomainTypeCondition.value
  const usesDraftCondition = !!draftCondition && conditionIds.includes(draftCondition.conditionId)
  if (usesDraftCondition) store.semanticConditions.push(draftCondition)
  store.addClause({
    title: payload.title.trim(),
    text,
    conditionIds,
    schemaRef: usesDraftCondition ? draftDomainTypeMeta.value?.schemaRef : undefined,
    semanticPath: usesDraftCondition ? draftDomainTypeMeta.value?.semanticPath : undefined,
  })
  newClauseTitle.value = ''
  newClauseText.value = ''
  draftDomainTypeCondition.value = null
  draftDomainTypeMeta.value = null
}

function startEditClause(blockId: string) {
  editingBlockId.value = blockId
}

function cancelEdit() {
  editingBlockId.value = null
}

function saveEditedClause(payload: { blockId: string; title: string; text: string }) {
  const text = payload.text
  const title = payload.title.trim()
  if (!text.trim()) return
  store.updateClause(payload.blockId, {
    title,
    text,
    conditionIds: conditionIdsFromText(text),
  })
  if (editingBlockId.value === payload.blockId) cancelEdit()
}

function deleteClause(blockId: string) {
  store.deleteClause(blockId)
  if (editingBlockId.value === blockId) cancelEdit()
}

function describeNewClauseFromDomainType(domainTypeId: string) {
  const domainType = ontologyDomainTypes.find((item) => item.id === domainTypeId)
  if (!domainType) return
  const role = domainType.roleRequired ? (selectedDomainTypeRoles.value[domainType.id] ?? '') : ''
  if (domainType.roleRequired && !role) return

  const conditionId = `${domainType.id}-${role || 'default'}-${crypto.randomUUID()}`
  const roleLabel = role ? roleLabelFor(role) : ''
  const title = roleLabel ? `${roleLabel} ${domainType.label}` : domainType.label
  const parameters = buildOntologyDomainTypeParameters(domainType)
  const text = buildOntologyDomainTypeClauseText(conditionId, domainType, role)

  draftDomainTypeCondition.value = {
    conditionId,
    conditionName: title,
    schemaVersion: SEMANTIC_CONDITION_SCHEMA_VERSION,
    entityType: domainType.entityType,
    ...(role ? { entityRole: role } : {}),
    parameters,
  }
  draftDomainTypeMeta.value = {
    schemaRef: domainType.fields[0]?.schemaRef,
    semanticPath: domainType.fields[0]?.semanticPath.split('.', 1)[0] ?? domainType.id,
  }
  newClauseTitle.value = title
  newClauseText.value = text
}
</script>

<template>
  <div class="space-y-6">
    <!-- Section 1: New clause -->
    <section v-if="uiStore.isTemplateEditable" class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <ClauseEditorForm
        mode="create"
        :initial-title="newClauseTitle"
        :initial-text="newClauseText"
        :semantic-conditions="newClauseSemanticConditions"
        @submit="addClause"
      />
    </section>

    <!-- Section 2: Existing clauses -->
    <section class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm">
      <h3 class="mb-4 text-sm font-semibold text-base-content/80">Existing clauses</h3>
      <ExistingClausesList
        :clause-blocks="clauseBlocks"
        :semantic-conditions="semanticConditions"
        :block-ids-in-outline="store.blockIdsInOutline"
        :editing-block-id="editingBlockId"
        :editable="uiStore.isTemplateEditable"
        @delete="deleteClause"
        @edit="startEditClause"
        @save="saveEditedClause"
        @cancel-edit="cancelEdit"
      />

      <div v-if="uiStore.isTemplateEditable && ontologyDomainTypes.length" class="mt-5 border-t border-base-300 pt-4">
        <h4 class="mb-3 text-xs font-semibold text-base-content/50 uppercase">Domain types</h4>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <div
            v-for="domainType in ontologyDomainTypes"
            :key="domainType.id"
            class="group grid min-h-[88px] grid-cols-1 items-center gap-3 rounded-lg border border-base-300 bg-base-100 px-3 py-3 shadow-sm transition-all hover:border-primary/50 hover:bg-base-200 hover:shadow md:grid-cols-[minmax(0,1fr)_minmax(11rem,14rem)_auto]"
          >
            <div class="min-w-0">
              <span class="block text-sm font-medium text-base-content">{{ domainType.label }}</span>
              <span class="block text-xs text-base-content/50">{{ domainType.fields.length }} domain fields</span>
            </div>
            <label v-if="domainType.roleRequired" class="mx-auto w-full max-w-56">
              <span class="label-text mb-1 block text-xs text-base-content/60">Contract role</span>
              <select
                v-model="selectedDomainTypeRoles[domainType.id]"
                class="select-bordered select w-full select-xs text-left"
              >
                <option value="">Select role</option>
                <option v-for="option in roleOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <span v-else class="hidden md:block" />
            <button
              type="button"
              class="btn justify-self-start transition-transform btn-xs btn-secondary group-hover:translate-x-0.5 md:justify-self-end"
              :disabled="domainType.roleRequired && !selectedDomainTypeRoles[domainType.id]"
              @click="describeNewClauseFromDomainType(domainType.id)"
            >
              Use
            </button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
