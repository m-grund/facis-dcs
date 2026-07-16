<script setup lang="ts">
import TypedClauseForm from '@template-repository/components/clauses-editor/TypedClauseForm.vue'
import { storeToRefs } from 'pinia'
import { computed, onMounted, ref } from 'vue'
import { useClauseCatalogStore } from '@/stores/clause-catalog-store'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * Typed clause palette (ADR-10): lists the Semantic Hub's clause types and
 * generates a SHACL-driven form (TypedClauseForm) for the selected one.
 * Refreshes the catalog on mount so a clause-catalog version activated in
 * the hub is available here immediately — no reload or redeploy.
 */
const emit = defineEmits<{
  submit: [payload: { clauseType: string; title: string; instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance }]
}>()

const catalog = useClauseCatalogStore()
const { clauses, shapes, loading, error } = storeToRefs(catalog)
onMounted(() => catalog.refresh())

const selectedType = ref<string | null>(null)
const selectedClause = computed<ClauseCatalogType | undefined>(() =>
  clauses.value.find((c) => c.type === selectedType.value),
)

function selectClauseType(type: string) {
  selectedType.value = selectedType.value === type ? null : type
}

function handleSubmit(payload: { clauseType: string; title: string; instance: import('@/models/dcs-jsonld').DcsTypedClauseInstance }) {
  emit('submit', payload)
  selectedType.value = null
}
</script>

<template>
  <div class="space-y-3">
    <div v-if="loading && !clauses.length" class="text-sm text-base-content/60">Loading clause catalog…</div>
    <div v-else-if="error" class="text-sm text-error">{{ error }}</div>
    <template v-else>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="clause in clauses"
          :key="clause.type"
          type="button"
          class="btn btn-xs"
          :class="selectedType === clause.type ? 'btn-primary' : 'btn-outline'"
          @click="selectClauseType(clause.type)"
        >
          {{ clause.label }}
        </button>
        <p v-if="!clauses.length" class="text-sm text-base-content/60">
          No typed clauses registered in the Semantic Hub.
        </p>
      </div>

      <div v-if="selectedClause" class="rounded border border-base-300 p-3">
        <TypedClauseForm :clause="selectedClause" :shapes="shapes" @submit="handleSubmit" />
      </div>
    </template>
  </div>
</template>
