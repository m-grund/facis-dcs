<script setup lang="ts">
import {
  CONSTRAINT_COMBINATORS,
  type ConstraintNodeDraft,
  type GroupDraft,
  isGroupDraft,
  newAtomic,
  newGroup,
} from '@template-repository/components/clauses-editor/constraint-draft'
import { ODRL_CONTEXT_OPERANDS, ODRL_OPERATORS } from '@template-repository/utils/odrl-vocabulary'

/**
 * Authors one ODRL constraint group — a combinator over child nodes, each an
 * atomic constraint or a nested group (recursion via the component's own name,
 * an arbitrarily deep constraint tree, ODRL IM §2.6). The rule and every duty
 * embed one root group.
 */

defineProps<{
  /** Fields offered as a constraint's left operand and negotiated boundary. */
  fields: { id: string; label: string }[]
  /** The title on this group's combinator select (targets the top-level one). */
  combineTitle?: string
}>()

const group = defineModel<GroupDraft>({ required: true })

function addConstraint() {
  group.value.children.push(newAtomic(ODRL_CONTEXT_OPERANDS[0]?.id ?? 'odrl:spatial', ODRL_OPERATORS[0]?.id ?? 'odrl:eq'))
}
function addGroup() {
  group.value.children.push(newGroup())
}
function removeChild(index: number) {
  group.value.children.splice(index, 1)
}

// Narrows a child node to a group for the recursive editor. The editor mutates
// this same reactive object in place, so a one-way :model-value binding still
// propagates every edit up through the shared draft graph.
function childGroup(child: ConstraintNodeDraft): GroupDraft {
  return child as GroupDraft
}
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-center gap-1">
      <select
        v-if="group.children.length > 1"
        v-model="group.combine"
        class="select-bordered select select-xs"
        :title="combineTitle ?? 'How this group combines'"
      >
        <option v-for="c in CONSTRAINT_COMBINATORS" :key="c.op" :value="c.op">{{ c.label }}</option>
      </select>
      <button type="button" class="btn btn-ghost btn-xs" @click="addConstraint">+ constraint</button>
      <button type="button" class="btn btn-ghost btn-xs" @click="addGroup">+ group</button>
    </div>

    <template v-for="(child, i) in group.children" :key="i">
      <!-- A nested group: recurse into this same editor, indented. -->
      <div v-if="isGroupDraft(child)" class="ml-3 space-y-1 rounded border border-base-300 border-dashed p-1">
        <div class="flex items-center justify-between">
          <span class="label-text text-2xs opacity-60">group</span>
          <button type="button" class="btn btn-ghost btn-xs" @click="removeChild(i)">✕</button>
        </div>
        <ConstraintGroupEditor :model-value="childGroup(child)" :fields="fields" />
      </div>

      <!-- An atomic constraint row. -->
      <div v-else class="flex flex-wrap items-center gap-1">
        <select v-model="child.leftOperand" class="select-bordered select select-xs">
          <optgroup v-if="fields.length" label="Data fields">
            <option v-for="f in fields" :key="f.id" :value="f.id">{{ f.label }}</option>
          </optgroup>
          <optgroup label="Access context">
            <option v-for="o in ODRL_CONTEXT_OPERANDS" :key="o.id" :value="o.id">{{ o.label }}</option>
          </optgroup>
        </select>
        <select v-model="child.operator" class="select-bordered select select-xs">
          <option v-for="op in ODRL_OPERATORS" :key="op.id" :value="op.id">{{ op.label }}</option>
        </select>
        <select v-model="child.rightSource" class="select-bordered select select-xs" title="What the boundary is">
          <option value="">a fixed value</option>
          <optgroup v-if="fields.length" label="Agreed at negotiation">
            <option v-for="f in fields" :key="f.id" :value="f.id">the “{{ f.label }}”</option>
          </optgroup>
        </select>
        <input
          v-if="!child.rightSource"
          v-model="child.value"
          type="text"
          placeholder="value"
          class="input-bordered input input-xs w-28"
        />
        <button type="button" class="btn btn-ghost btn-xs" @click="removeChild(i)">✕</button>
      </div>
    </template>
  </div>
</template>
