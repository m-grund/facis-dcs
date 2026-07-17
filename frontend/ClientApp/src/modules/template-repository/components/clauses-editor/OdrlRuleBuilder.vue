<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { JsonLdTypedValue, OdrlConstraint, OdrlRule } from '@/models/dcs-jsonld'

/**
 * The machine-readable meaning of a clause: an ODRL rule the author builds by
 * picking, never by typing an IRI. A rule is one obligation/permission/
 * prohibition — who (assignee) must/may/must-not do what (action) toward whom
 * (target), under which constraints on the clause's data fields. This is all a
 * "typed clause" ever was: e.g. a payment clause is an obligation, action
 * "payment", assignee the counterparty, constraints amount/currency.
 */

export interface Anchor {
  /** The IRI the document references — generated elsewhere, hidden here. */
  id: string
  label: string
}

const props = defineProps<{
  modelValue: OdrlRule | null
  /** Data fields declared for this clause (constraint left operands). */
  fields: Anchor[]
  /** Parties the rule can bind (assigner/assignee/target). */
  parties: Anchor[]
  /** The prose block this rule is backed by (dcs:prose). */
  proseId: string
  /** The contract/asset IRI the rule targets by default. */
  contractTargetId: string
}>()

const emit = defineEmits<{ 'update:modelValue': [OdrlRule | null] }>()

const RULE_TYPES = [
  { type: 'odrl:Duty', label: 'Obligation — the party MUST' },
  { type: 'odrl:Permission', label: 'Permission — the party MAY' },
  { type: 'odrl:Prohibition', label: 'Prohibition — the party MUST NOT' },
] as const

const ACTIONS = [
  { id: 'dcs:provideCompliantValue', label: 'provide a compliant value' },
  { id: 'dcs:payment', label: 'make a payment' },
  { id: 'dcs:delivery', label: 'deliver' },
  { id: 'dcs:notification', label: 'notify' },
  { id: 'dcs:disclosure', label: 'disclose' },
] as const

const OPERATORS = [
  { id: 'odrl:eq', label: 'must equal' },
  { id: 'odrl:neq', label: 'must not equal' },
  { id: 'odrl:gt', label: 'must be greater than' },
  { id: 'odrl:gteq', label: 'must be at least' },
  { id: 'odrl:lt', label: 'must be less than' },
  { id: 'odrl:lteq', label: 'must be at most' },
  { id: 'odrl:hasPart', label: 'must contain' },
  { id: 'odrl:isAnyOf', label: 'must be one of' },
  { id: 'odrl:isNoneOf', label: 'must not be one of' },
] as const

interface ConstraintDraft {
  fieldId: string
  operator: string
  value: string
}

const draft = reactive<{
  type: string
  action: string
  assigneeId: string
  assignerId: string
  targetId: string
  constraints: ConstraintDraft[]
}>({
  type: props.modelValue?.['@type'] ?? 'odrl:Duty',
  action: props.modelValue?.['odrl:action']?.['@id'] ?? ACTIONS[0].id,
  assigneeId: props.modelValue?.['odrl:assignee']?.['@id'] ?? props.parties[0]?.id ?? '',
  assignerId: props.modelValue?.['odrl:assigner']?.['@id'] ?? props.parties[0]?.id ?? '',
  targetId: props.modelValue?.['odrl:target']?.['@id'] ?? props.contractTargetId,
  constraints: readConstraints(props.modelValue),
})

const complete = computed(() => !!draft.action && !!draft.assigneeId)

function readConstraints(rule: OdrlRule | null): ConstraintDraft[] {
  const constraint = rule?.['odrl:constraint']
  if (!constraint) return []
  const right = constraint['odrl:rightOperand']
  const value = Array.isArray(right)
    ? right.map((r) => r['@value']).join(', ')
    : right != null
      ? String(right['@value'])
      : ''
  return [{ fieldId: constraint['odrl:leftOperand']['@id'], operator: constraint['odrl:operator']['@id'], value }]
}

function addConstraint() {
  draft.constraints.push({ fieldId: props.fields[0]?.id ?? '', operator: OPERATORS[0].id, value: '' })
}

function removeConstraint(index: number) {
  draft.constraints.splice(index, 1)
}

function typed(value: string): JsonLdTypedValue {
  const isNumber = value !== '' && !Number.isNaN(Number(value))
  return { '@value': value, '@type': isNumber ? 'xsd:decimal' : 'xsd:string' }
}

function rightOperand(value: string, operator: string): JsonLdTypedValue | JsonLdTypedValue[] | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  if (operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf') {
    return trimmed.split(',').map((part) => typed(part.trim()))
  }
  return typed(trimmed)
}

// The document carries one enclosing rule with a single constraint today; emit
// the first constraint as the rule's odrl:constraint (multi-constraint rules
// are a follow-up once the enforcement graph accepts a constraint list).
const rule = computed<OdrlRule | null>(() => {
  if (!complete.value) return null
  const first = draft.constraints[0]
  const built: OdrlRule = {
    '@id': props.modelValue?.['@id'] ?? `urn:uuid:${crypto.randomUUID()}`,
    '@type': draft.type as OdrlRule['@type'],
    'odrl:action': { '@id': draft.action },
    'odrl:assigner': { '@id': draft.assignerId },
    'odrl:assignee': { '@id': draft.assigneeId },
    'odrl:target': { '@id': draft.targetId || props.contractTargetId },
    'dcs:prose': { '@id': props.proseId },
  }
  if (first?.fieldId) {
    const constraint: OdrlConstraint = {
      '@type': 'odrl:Constraint',
      'odrl:leftOperand': { '@id': first.fieldId },
      'odrl:operator': { '@id': first.operator },
    }
    const right = rightOperand(first.value, first.operator)
    if (right !== undefined) constraint['odrl:rightOperand'] = right
    built['odrl:constraint'] = constraint
  }
  return built
})

watch(rule, (value) => emit('update:modelValue', value), { deep: true })
</script>

<template>
  <div class="space-y-3 text-xs">
    <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <label class="form-control">
        <span class="label-text text-xs">Rule</span>
        <select v-model="draft.type" class="select-bordered select select-sm">
          <option v-for="rt in RULE_TYPES" :key="rt.type" :value="rt.type">{{ rt.label }}</option>
        </select>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Action</span>
        <select v-model="draft.action" class="select-bordered select select-sm">
          <option v-for="a in ACTIONS" :key="a.id" :value="a.id">{{ a.label }}</option>
        </select>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Applies to (assignee)</span>
        <select v-model="draft.assigneeId" class="select-bordered select select-sm">
          <option v-for="p in parties" :key="p.id" :value="p.id">{{ p.label }}</option>
        </select>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Toward (target)</span>
        <select v-model="draft.targetId" class="select-bordered select select-sm">
          <option :value="contractTargetId">the contract</option>
          <option v-for="p in parties" :key="p.id" :value="p.id">{{ p.label }}</option>
        </select>
      </label>
    </div>

    <div class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="label-text text-xs font-semibold">Constraints</span>
        <button type="button" class="btn btn-ghost btn-xs" @click="addConstraint">+ constraint</button>
      </div>
      <p v-if="!fields.length" class="text-xs text-base-content/50 italic">
        Add a data field (a placeholder in the prose) to constrain it.
      </p>
      <div v-for="(c, i) in draft.constraints" :key="i" class="flex flex-wrap items-center gap-1">
        <select v-model="c.fieldId" class="select-bordered select select-xs">
          <option v-for="f in fields" :key="f.id" :value="f.id">{{ f.label }}</option>
        </select>
        <select v-model="c.operator" class="select-bordered select select-xs">
          <option v-for="op in OPERATORS" :key="op.id" :value="op.id">{{ op.label }}</option>
        </select>
        <input v-model="c.value" type="text" placeholder="value" class="input-bordered input input-xs w-28" />
        <button type="button" class="btn btn-ghost btn-xs" @click="removeConstraint(i)">✕</button>
      </div>
    </div>
  </div>
</template>
