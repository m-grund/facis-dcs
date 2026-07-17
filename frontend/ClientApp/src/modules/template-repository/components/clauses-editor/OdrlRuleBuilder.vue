<script setup lang="ts">
import IriPicker from '@template-repository/components/clauses-editor/IriPicker.vue'
import {
  ODRL_ACTIONS,
  ODRL_CONTEXT_OPERANDS,
  ODRL_OPERATORS,
  ODRL_RULE_TYPES,
} from '@template-repository/utils/odrl-vocabulary'
import { computed, reactive, watch } from 'vue'
import type { JsonLdReference, JsonLdTypedValue, OdrlConstraint, OdrlRule } from '@/models/dcs-jsonld'

/**
 * The machine-readable meaning of a clause: an ODRL rule. Reference terms —
 * action, assigner, assignee, target — are picked from the DCS vocabulary or
 * given as a custom IRI (ODRL sets no closed vocabulary on them). A rule is one
 * permission/prohibition/obligation — who (assignee) may/must-not/must do what
 * (action) toward whom (target), bounded by constraints that all must hold.
 * The SRS Appendix C access policy is a permission to use an asset bounded by
 * spatial and dateTime constraints.
 */

export interface Anchor {
  /** The IRI the document references — generated elsewhere, hidden here. */
  id: string
  label: string
}

const props = withDefaults(
  defineProps<{
    modelValue: OdrlRule | null
    /** Data fields declared for this clause (document constraint left operands). */
    fields: Anchor[]
    /** Assets declared for this clause (targetable objects — an ODRL rule's target). */
    assets?: Anchor[]
    /** Parties the rule can bind (assigner/assignee/target). */
    parties: Anchor[]
    /** The prose block this rule is backed by (dcs:prose). */
    proseId: string
    /** The contract/asset IRI the rule targets by default. */
    contractTargetId: string
  }>(),
  { assets: () => [] },
)

const emit = defineEmits<{ 'update:modelValue': [OdrlRule | null] }>()

const actionOptions = ODRL_ACTIONS.map((a) => ({ value: a.id, label: a.label }))
const partyOptions = computed(() => props.parties.map((p) => ({ value: p.id, label: p.label })))
const targetOptions = computed(() => [{ value: props.contractTargetId, label: 'the contract' }])
const targetGroups = computed(() => {
  const groups: { label: string; options: { value: string; label: string }[] }[] = []
  if (props.assets.length) {
    groups.push({ label: 'Asset', options: props.assets.map((a) => ({ value: a.id, label: a.label })) })
  }
  if (props.fields.length) {
    groups.push({ label: 'Data field', options: props.fields.map((f) => ({ value: f.id, label: f.label })) })
  }
  groups.push({ label: 'Parties', options: props.parties.map((p) => ({ value: p.id, label: p.label })) })
  return groups
})

interface ConstraintDraft {
  leftOperand: string
  operator: string
  /** '' = a fixed literal boundary (use `value`); otherwise a field @id whose
   *  value is agreed during contract negotiation. */
  rightSource: string
  value: string
}

const draft = reactive<{
  type: string
  actions: string[]
  assigneeId: string
  assignerId: string
  targetId: string
  constraints: ConstraintDraft[]
}>({
  type: props.modelValue?.['@type'] ?? ODRL_RULE_TYPES[0]?.type ?? 'odrl:Permission',
  actions: readActions(props.modelValue),
  assigneeId: props.modelValue?.['odrl:assignee']?.['@id'] ?? props.parties[0]?.id ?? '',
  assignerId: props.modelValue?.['odrl:assigner']?.['@id'] ?? props.parties[0]?.id ?? '',
  targetId: props.modelValue?.['odrl:target']?.['@id'] ?? props.contractTargetId,
  constraints: readConstraints(props.modelValue),
})

// Seeded once. Reading props.modelValue inside the emitting `rule` computed
// would make each emit retrigger the computed (a reactive feedback loop).
const ruleId = props.modelValue?.['@id'] ?? `urn:uuid:${crypto.randomUUID()}`

const complete = computed(() => draft.actions.some((a) => !!a) && !!draft.assigneeId)

function readActions(rule: OdrlRule | null): string[] {
  const action = rule?.['odrl:action']
  if (!action) return [ODRL_ACTIONS[0]?.id ?? 'odrl:use']
  const list = Array.isArray(action) ? action : [action]
  const ids = list.map((a) => a['@id']).filter(Boolean)
  return ids.length ? ids : [ODRL_ACTIONS[0]?.id ?? 'odrl:use']
}

function addAction() {
  draft.actions.push(ODRL_ACTIONS[0]?.id ?? 'odrl:use')
}
function removeAction(index: number) {
  draft.actions.splice(index, 1)
}

function readConstraints(rule: OdrlRule | null): ConstraintDraft[] {
  return (rule?.['odrl:constraint'] ?? []).map((c) => {
    const right = c['odrl:rightOperand']
    if (right && '@id' in right) {
      return {
        leftOperand: c['odrl:leftOperand']['@id'],
        operator: c['odrl:operator']['@id'],
        rightSource: right['@id'],
        value: '',
      }
    }
    const value = Array.isArray(right) ? right.map((r) => r['@value']).join(', ') : (right?.['@value'] ?? '')
    return { leftOperand: c['odrl:leftOperand']['@id'], operator: c['odrl:operator']['@id'], rightSource: '', value }
  })
}

function addConstraint() {
  draft.constraints.push({
    leftOperand: ODRL_CONTEXT_OPERANDS[0]?.id ?? 'odrl:spatial',
    operator: ODRL_OPERATORS[0]?.id ?? 'odrl:eq',
    rightSource: '',
    value: '',
  })
}

function removeConstraint(index: number) {
  draft.constraints.splice(index, 1)
}

function typed(value: string): JsonLdTypedValue {
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(value)) return { '@value': value, '@type': 'xsd:dateTime' }
  const isNumber = value !== '' && !Number.isNaN(Number(value))
  return { '@value': value, '@type': isNumber ? 'xsd:decimal' : 'xsd:string' }
}

function rightOperand(value: string, operator: string): JsonLdTypedValue | JsonLdTypedValue[] | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  if (operator === 'odrl:isAnyOf' || operator === 'odrl:isNoneOf' || operator === 'odrl:isAllOf') {
    return trimmed.split(',').map((part) => typed(part.trim()))
  }
  return typed(trimmed)
}

const rule = computed<OdrlRule | null>(() => {
  if (!complete.value) return null
  const constraints: OdrlConstraint[] = draft.constraints
    .filter((c) => c.leftOperand)
    .map((c) => {
      const constraint: OdrlConstraint = {
        '@type': 'odrl:Constraint',
        'odrl:leftOperand': { '@id': c.leftOperand },
        'odrl:operator': { '@id': c.operator },
      }
      const right: JsonLdTypedValue | JsonLdTypedValue[] | JsonLdReference | undefined = c.rightSource
        ? { '@id': c.rightSource }
        : rightOperand(c.value, c.operator)
      if (right !== undefined) constraint['odrl:rightOperand'] = right
      return constraint
    })
  const actions = draft.actions.filter(Boolean)
  const built: OdrlRule = {
    '@id': ruleId,
    '@type': draft.type as OdrlRule['@type'],
    'odrl:action': actions.length === 1 ? { '@id': actions[0] ?? '' } : actions.map((a) => ({ '@id': a })),
    'odrl:assigner': { '@id': draft.assignerId },
    'odrl:assignee': { '@id': draft.assigneeId },
    'odrl:target': { '@id': draft.targetId || props.contractTargetId },
    'dcs:prose': { '@id': props.proseId },
  }
  if (constraints.length) built['odrl:constraint'] = constraints
  return built
})

watch(rule, (value) => emit('update:modelValue', value))
</script>

<template>
  <div class="space-y-3 text-xs">
    <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <label class="form-control">
        <span class="label-text text-xs">Rule</span>
        <select v-model="draft.type" class="select-bordered select select-sm">
          <option v-for="rt in ODRL_RULE_TYPES" :key="rt.type" :value="rt.type">{{ rt.label }}</option>
        </select>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Action(s)</span>
        <div class="flex flex-col gap-1">
          <div v-for="(_, i) in draft.actions" :key="i" class="flex items-center gap-1">
            <IriPicker
              :model-value="draft.actions[i] ?? ''"
              :options="actionOptions"
              @update:model-value="draft.actions[i] = $event"
            />
            <button v-if="draft.actions.length > 1" type="button" class="btn btn-ghost btn-xs" @click="removeAction(i)">
              ✕
            </button>
          </div>
          <button type="button" class="btn w-fit btn-ghost btn-xs" @click="addAction">+ action</button>
        </div>
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Granted by (assigner)</span>
        <IriPicker v-model="draft.assignerId" :options="partyOptions" placeholder="party DID / IRI" />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Applies to (assignee)</span>
        <IriPicker v-model="draft.assigneeId" :options="partyOptions" placeholder="party DID / IRI" />
      </label>
      <label class="form-control">
        <span class="label-text text-xs">Toward (target)</span>
        <IriPicker v-model="draft.targetId" :options="targetOptions" :groups="targetGroups" placeholder="asset IRI" />
      </label>
    </div>

    <div class="space-y-2">
      <div class="flex items-center justify-between">
        <span class="label-text text-xs font-semibold">Constraints (all must hold)</span>
        <button type="button" class="btn btn-ghost btn-xs" @click="addConstraint">+ constraint</button>
      </div>
      <div v-for="(c, i) in draft.constraints" :key="i" class="flex flex-wrap items-center gap-1">
        <select v-model="c.leftOperand" class="select-bordered select select-xs">
          <optgroup v-if="fields.length" label="Data fields">
            <option v-for="f in fields" :key="f.id" :value="f.id">{{ f.label }}</option>
          </optgroup>
          <optgroup label="Access context">
            <option v-for="o in ODRL_CONTEXT_OPERANDS" :key="o.id" :value="o.id">{{ o.label }}</option>
          </optgroup>
        </select>
        <select v-model="c.operator" class="select-bordered select select-xs">
          <option v-for="op in ODRL_OPERATORS" :key="op.id" :value="op.id">{{ op.label }}</option>
        </select>
        <select v-model="c.rightSource" class="select-bordered select select-xs" title="What the boundary is">
          <option value="">a fixed value</option>
          <optgroup v-if="fields.length" label="Agreed at negotiation">
            <option v-for="f in fields" :key="f.id" :value="f.id">the “{{ f.label }}”</option>
          </optgroup>
        </select>
        <input
          v-if="!c.rightSource"
          v-model="c.value"
          type="text"
          placeholder="value"
          class="input-bordered input input-xs w-28"
        />
        <button type="button" class="btn btn-ghost btn-xs" @click="removeConstraint(i)">✕</button>
      </div>
    </div>
  </div>
</template>
