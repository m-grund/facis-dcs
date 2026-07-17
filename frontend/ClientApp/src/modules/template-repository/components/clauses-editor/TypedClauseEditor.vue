<script setup lang="ts">
import TypedClauseForm from '@template-repository/components/clauses-editor/TypedClauseForm.vue'
import { findCatalogClause } from '@template-repository/utils/typed-clause'
import { computed, onMounted, ref } from 'vue'
import { getHubPrefixes } from '@/services/semantic-hub-service'
import { useClauseCatalogStore } from '@/stores/clause-catalog-store'
import type { DcsTypedClauseInstance } from '@/models/dcs-jsonld'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * Shape-driven editing of an EXISTING typed clause instance: resolves the
 * instance's rdf:type back to its Semantic Hub clause-catalog entry and
 * re-renders the same shacl-form the palette generated it with, prefilled.
 * The template freezes a clause's shape; whoever fills the document (the
 * template author or, later, the contract creator) edits its values here —
 * never through the free-text clause form, which would let the prose and
 * the machine-readable instance drift apart (DCS-FR-CWE-04).
 */
const props = defineProps<{
  instance: DcsTypedClauseInstance
  initialTitle?: string
  submitLabel?: string
}>()

const emit = defineEmits<{
  submit: [payload: { clauseType: string; title: string; instance: DcsTypedClauseInstance }]
  cancel: []
}>()

const catalog = useClauseCatalogStore()
const prefixes = ref<Record<string, string> | null>(null)
const loadError = ref<string | null>(null)

onMounted(async () => {
  try {
    await catalog.ensureLoaded()
    prefixes.value = await getHubPrefixes()
  } catch (err: unknown) {
    loadError.value = err instanceof Error ? err.message : 'Could not load the clause catalog'
  }
})

const catalogClause = computed<ClauseCatalogType | undefined>(() => {
  if (!prefixes.value) return undefined
  return findCatalogClause(props.instance, catalog.clauses, prefixes.value)
})

const resolving = computed(() => !loadError.value && !catalog.error && (catalog.loading || !prefixes.value))
</script>

<template>
  <div v-if="resolving" class="text-sm text-base-content/60">Loading clause catalog…</div>
  <div v-else-if="loadError || catalog.error" class="text-sm text-error">{{ loadError ?? catalog.error }}</div>
  <div v-else-if="!catalogClause" class="text-sm text-warning">
    This clause's type is no longer in the Semantic Hub's clause catalog, so its values cannot be edited with a
    generated form. Re-activate the catalog version that defines it in the Semantic Hub.
  </div>
  <TypedClauseForm
    v-else
    :clause="catalogClause"
    :shapes="catalog.shapes"
    :initial-values="instance"
    :initial-title="initialTitle"
    :submit-label="submitLabel ?? 'Save values'"
    :show-cancel="true"
    @submit="(payload) => emit('submit', payload)"
    @cancel="emit('cancel')"
  />
</template>
