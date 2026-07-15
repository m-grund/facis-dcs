<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { ClauseCatalogProperty, ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * SHACL-driven form for one typed clause (ADR-10): widgets and inline
 * validation are generated from the clause type's digested SHACL
 * properties — sh:in becomes a select, xsd numeric datatypes become number
 * inputs bounded by sh:minInclusive/maxInclusive, sh:pattern validates
 * live, sh:minCount >= 1 marks the field required. Client-side validation
 * is UX only; the server (goRDFlib against the same hub shapes) remains
 * the enforcement point.
 */
const props = defineProps<{
  clause: ClauseCatalogType
  /** Prefill for editing an existing instance (dcs:-prefixed keys). */
  initialValues?: Record<string, unknown>
  initialTitle?: string
  submitLabel?: string
  showCancel?: boolean
}>()

const emit = defineEmits<{
  submit: [payload: { clauseType: string; title: string; values: Record<string, unknown> }]
  cancel: []
}>()

const title = ref('')
const values = reactive<Record<string, string>>({})
const touched = reactive<Record<string, boolean>>({})
const submitAttempted = ref(false)

/** dcs-prefixed instance key for a digest property path. */
function valueKey(prop: ClauseCatalogProperty): string {
  return `dcs:${prop.path.replace(/^dcs:/, '')}`
}

watch(
  () => [props.clause, props.initialValues, props.initialTitle] as const,
  ([clause, initialValues, initialTitle]) => {
    title.value = initialTitle ?? ''
    for (const key of Object.keys(values)) delete values[key]
    for (const key of Object.keys(touched)) delete touched[key]
    submitAttempted.value = false
    for (const prop of clause.properties) {
      const existing = initialValues?.[valueKey(prop)]
      values[prop.path] = existing === undefined || existing === null ? '' : String(existing)
    }
  },
  { immediate: true },
)

type InputKind = 'select' | 'number' | 'checkbox' | 'text'

function inputKind(prop: ClauseCatalogProperty): InputKind {
  if (prop.in && prop.in.length > 0) return 'select'
  if (prop.datatype === 'integer' || prop.datatype === 'decimal' || prop.datatype === 'double') return 'number'
  if (prop.datatype === 'boolean') return 'checkbox'
  return 'text'
}

function isRequired(prop: ClauseCatalogProperty): boolean {
  return (prop.min_count ?? 0) >= 1
}

function numberStep(prop: ClauseCatalogProperty): string {
  return prop.datatype === 'integer' ? '1' : 'any'
}

/** Live per-field validation mirroring the SHACL constraints the server enforces. */
function fieldError(prop: ClauseCatalogProperty): string | null {
  const raw = (values[prop.path] ?? '').trim()
  if (raw === '') {
    return isRequired(prop) ? 'This field is required' : null
  }
  const kind = inputKind(prop)
  if (kind === 'number') {
    const parsed = Number(raw)
    if (Number.isNaN(parsed)) return 'Must be a number'
    if (prop.datatype === 'integer' && !Number.isInteger(parsed)) return 'Must be a whole number'
    if (prop.min_inclusive !== undefined && parsed < prop.min_inclusive) {
      return `Must be at least ${prop.min_inclusive}`
    }
    if (prop.max_inclusive !== undefined && parsed > prop.max_inclusive) {
      return `Must be at most ${prop.max_inclusive}`
    }
  }
  if (prop.pattern) {
    let re: RegExp
    try {
      re = new RegExp(prop.pattern)
    } catch {
      // An unparsable pattern is a shapes-authoring problem the server
      // will surface; don't block the form on it client-side.
      return null
    }
    if (!re.test(raw)) return `Must match ${prop.pattern}`
  }
  return null
}

function showError(prop: ClauseCatalogProperty): string | null {
  if (!touched[prop.path] && !submitAttempted.value) return null
  return fieldError(prop)
}

const isValid = computed(() => props.clause.properties.every((p) => fieldError(p) === null))

function fieldLabel(prop: ClauseCatalogProperty): string {
  const local = prop.path.replace(/^dcs:/, '')
  return local.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/^./, (c) => c.toUpperCase())
}

function constraintHint(prop: ClauseCatalogProperty): string {
  const parts: string[] = []
  if (prop.datatype) parts.push(prop.datatype)
  if (prop.min_inclusive !== undefined && prop.max_inclusive !== undefined) {
    parts.push(`${prop.min_inclusive}–${prop.max_inclusive}`)
  } else if (prop.min_inclusive !== undefined) {
    parts.push(`≥ ${prop.min_inclusive}`)
  } else if (prop.max_inclusive !== undefined) {
    parts.push(`≤ ${prop.max_inclusive}`)
  }
  if (prop.pattern) parts.push(prop.pattern)
  return parts.join(' · ')
}

function coerceValue(raw: string, prop: ClauseCatalogProperty): unknown {
  const trimmed = raw.trim()
  if (trimmed === '') return undefined
  if (prop.datatype === 'integer') return parseInt(trimmed, 10)
  if (prop.datatype === 'decimal' || prop.datatype === 'double') return parseFloat(trimmed)
  if (prop.datatype === 'boolean') return trimmed === 'true'
  return trimmed
}

function submit() {
  submitAttempted.value = true
  if (!isValid.value) return
  const typedValues: Record<string, unknown> = {}
  for (const prop of props.clause.properties) {
    const coerced = coerceValue(values[prop.path] ?? '', prop)
    if (coerced !== undefined) typedValues[valueKey(prop)] = coerced
  }
  emit('submit', { clauseType: props.clause.type, title: title.value.trim(), values: typedValues })
}
</script>

<template>
  <form class="space-y-3" @submit.prevent="submit">
    <div class="form-control">
      <label class="label py-1"><span class="label-text text-xs">Title (optional)</span></label>
      <input v-model="title" type="text" class="input-bordered input input-sm w-full" :placeholder="clause.label" />
    </div>

    <div v-for="prop in clause.properties" :key="prop.path" class="form-control">
      <label class="label py-1">
        <span class="label-text text-xs">
          {{ fieldLabel(prop) }}
          <span v-if="isRequired(prop)" class="text-error" aria-hidden="true">*</span>
        </span>
        <span v-if="constraintHint(prop)" class="label-text-alt text-[10px] opacity-60">
          {{ constraintHint(prop) }}
        </span>
      </label>

      <select
        v-if="inputKind(prop) === 'select'"
        v-model="values[prop.path]"
        class="select-bordered select w-full select-sm"
        :class="showError(prop) ? 'select-error' : ''"
        :required="isRequired(prop)"
        @blur="touched[prop.path] = true"
      >
        <option value="" disabled>Select…</option>
        <option v-for="opt in prop.in" :key="opt" :value="opt">{{ opt }}</option>
      </select>
      <select
        v-else-if="inputKind(prop) === 'checkbox'"
        v-model="values[prop.path]"
        class="select-bordered select w-full select-sm"
        :class="showError(prop) ? 'select-error' : ''"
        @blur="touched[prop.path] = true"
      >
        <option value="" disabled>Select…</option>
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
      <input
        v-else
        v-model="values[prop.path]"
        :type="inputKind(prop) === 'number' ? 'number' : 'text'"
        :min="prop.min_inclusive"
        :max="prop.max_inclusive"
        :step="inputKind(prop) === 'number' ? numberStep(prop) : undefined"
        class="input-bordered input input-sm w-full"
        :class="showError(prop) ? 'input-error' : ''"
        @blur="touched[prop.path] = true"
      />

      <p v-if="showError(prop)" class="mt-1 text-xs text-error">{{ showError(prop) }}</p>
    </div>

    <div class="flex items-center justify-end gap-2 pt-1">
      <button v-if="showCancel" type="button" class="btn btn-ghost btn-sm" @click="emit('cancel')">Cancel</button>
      <button type="submit" class="btn btn-sm btn-primary" :disabled="submitAttempted && !isValid">
        {{ submitLabel ?? 'Add typed clause' }}
      </button>
    </div>
  </form>
</template>
