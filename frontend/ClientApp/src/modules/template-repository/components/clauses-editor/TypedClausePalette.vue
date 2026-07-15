<script setup lang="ts">
import { useClauseCatalog } from '@template-repository/composables/useClauseCatalog'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * Phase 3 (DCS-FR-TR-03/TR-04, ADR-10): renders a form generated from the
 * Semantic Hub's clause catalog (GET /semantic/clauses) — the palette lists
 * clause types, selecting one generates typed inputs from that type's SHACL
 * properties (datatype -> input kind, sh:in -> select, min/max -> HTML
 * min/max), and submitting emits a typed clause instance. Server-side
 * validation (validateAgainstHubShapes) checks the same shapes this form
 * was generated from, so client-side validity here is a UX nicety, not the
 * enforcement point.
 */
const emit = defineEmits<{
  submit: [payload: { clauseType: string; title: string; values: Record<string, unknown> }]
}>()

const { clauses, loading, error, load } = useClauseCatalog()
onMounted(load)

const selectedType = ref<string | null>(null)
const selectedClause = computed<ClauseCatalogType | undefined>(() =>
  clauses.value.find((c) => c.type === selectedType.value),
)
const title = ref('')
const values = reactive<Record<string, string>>({})

watch(selectedClause, (clause) => {
  title.value = ''
  for (const key of Object.keys(values)) delete values[key]
  if (!clause) return
  for (const prop of clause.properties) values[prop.path] = ''
})

function selectClauseType(type: string) {
  selectedType.value = type
}

function inputKind(prop: { datatype?: string; in?: string[] }): 'select' | 'number' | 'text' {
  if (prop.in && prop.in.length > 0) return 'select'
  if (prop.datatype === 'integer' || prop.datatype === 'decimal' || prop.datatype === 'double') return 'number'
  return 'text'
}

function coerceValue(raw: string, prop: { datatype?: string }): unknown {
  if (prop.datatype === 'integer') return raw === '' ? undefined : parseInt(raw, 10)
  if (prop.datatype === 'decimal' || prop.datatype === 'double') return raw === '' ? undefined : parseFloat(raw)
  if (prop.datatype === 'boolean') return raw === 'true'
  return raw === '' ? undefined : raw
}

const canSubmit = computed(() => {
  const clause = selectedClause.value
  if (!clause) return false
  return clause.properties
    .filter((p) => (p.min_count ?? 0) > 0)
    .every((p) => (values[p.path] ?? '').toString().trim() !== '')
})

function submit() {
  const clause = selectedClause.value
  if (!clause || !canSubmit.value) return
  const typedValues: Record<string, unknown> = {}
  for (const prop of clause.properties) {
    const coerced = coerceValue(values[prop.path] ?? '', prop)
    if (coerced !== undefined) typedValues[`dcs:${prop.path.replace(/^dcs:/, '')}`] = coerced
  }
  emit('submit', { clauseType: clause.type, title: title.value.trim(), values: typedValues })
  selectedType.value = null
}
</script>

<template>
  <div class="space-y-3">
    <div v-if="loading" class="text-sm text-base-content/60">Loading clause catalog…</div>
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

      <form v-if="selectedClause" class="space-y-3 rounded border border-base-300 p-3" @submit.prevent="submit">
        <div class="form-control">
          <label class="label"><span class="label-text">Title (optional)</span></label>
          <input
            v-model="title"
            type="text"
            class="input-bordered input input-sm"
            :placeholder="selectedClause.label"
          />
        </div>
        <div v-for="prop in selectedClause.properties" :key="prop.path" class="form-control">
          <label class="label">
            <span class="label-text">
              {{ prop.path }}
              <span v-if="(prop.min_count ?? 0) > 0" class="text-error">*</span>
            </span>
          </label>
          <select
            v-if="inputKind(prop) === 'select'"
            v-model="values[prop.path]"
            class="select-bordered select select-sm"
          >
            <option value="" disabled>Select…</option>
            <option v-for="opt in prop.in" :key="opt" :value="opt">{{ opt }}</option>
          </select>
          <input
            v-else
            v-model="values[prop.path]"
            :type="inputKind(prop) === 'number' ? 'number' : 'text'"
            :min="prop.min_inclusive"
            :max="prop.max_inclusive"
            class="input-bordered input input-sm"
          />
        </div>
        <button type="submit" class="btn btn-sm btn-primary" :disabled="!canSubmit">Add typed clause</button>
      </form>
    </template>
  </div>
</template>
