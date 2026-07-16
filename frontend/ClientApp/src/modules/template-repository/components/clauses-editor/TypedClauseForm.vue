<script setup lang="ts">
import '@ulb-darmstadt/shacl-form'
import jsonld from 'jsonld'
import { onMounted, ref, watch } from 'vue'
import type { DcsTypedClauseInstance } from '@/models/dcs-jsonld'
import type { ClauseCatalogType } from '@/services/semantic-hub-service'

/**
 * SHACL-driven form for one typed clause: <shacl-form> (ULB Darmstadt)
 * renders widgets and validates client-side directly from the hub's raw
 * shapes Turtle — sh:name/sh:description/sh:order, nested sh:node groups,
 * sh:in/datatype/pattern constraints all come from the shapes, whatever
 * namespace they live in. The server (goRDFlib against the same graph)
 * remains the enforcement point. Submitted instances are the form's own
 * JSON-LD serialization: absolute IRIs, self-contained.
 */
const props = defineProps<{
  clause: ClauseCatalogType
  /** The raw shapes Turtle the clause's NodeShape lives in. */
  shapes: string
  /** Prefill for editing an existing instance (absolute-IRI JSON-LD). */
  initialValues?: DcsTypedClauseInstance
  initialTitle?: string
  submitLabel?: string
  showCancel?: boolean
}>()

const emit = defineEmits<{
  submit: [payload: { clauseType: string; title: string; instance: DcsTypedClauseInstance }]
  cancel: []
}>()

const title = ref(props.initialTitle ?? '')
const formError = ref<string | null>(null)
const formHost = ref<HTMLElement | null>(null)

interface ShaclFormElement extends HTMLElement {
  serialize(format?: string): string
  validate(ignoreEmptyValues?: boolean): Promise<boolean>
}

let formEl: ShaclFormElement | null = null

async function instanceToNQuads(instance: DcsTypedClauseInstance): Promise<string> {
  return (await jsonld.toRDF(instance as object, { format: 'application/n-quads' })) as unknown as string
}

async function mountForm() {
  if (!formHost.value) return
  formHost.value.innerHTML = ''
  const el = document.createElement('shacl-form') as ShaclFormElement
  el.setAttribute('data-shapes', props.shapes)
  el.setAttribute('data-shape-subject', props.clause.shape)
  if (props.initialValues?.['@id']) {
    el.setAttribute('data-values', await instanceToNQuads(props.initialValues))
    el.setAttribute('data-value-subject', String(props.initialValues['@id']))
  }
  formEl = el
  formHost.value.appendChild(el)
}

onMounted(mountForm)
watch(
  () => [props.clause, props.shapes, props.initialValues] as const,
  () => {
    title.value = props.initialTitle ?? ''
    void mountForm()
  },
)

function isInstanceNode(node: unknown): node is DcsTypedClauseInstance {
  return typeof node === 'object' && node !== null && Object.keys(node).length > 1
}

async function submit() {
  if (!formEl) return
  formError.value = null
  if (!(await formEl.validate(false))) {
    formError.value = 'The values do not satisfy the clause shape yet'
    return
  }
  try {
    const serialized = JSON.parse(formEl.serialize('application/ld+json')) as unknown
    const nodes = Array.isArray(serialized) ? serialized : [serialized]
    const instance = nodes.find(isInstanceNode)
    if (!instance) {
      formError.value = 'The form produced no values'
      return
    }
    emit('submit', { clauseType: props.clause.type, title: title.value.trim(), instance })
  } catch (err: unknown) {
    formError.value = err instanceof Error ? err.message : 'Could not serialize the form values'
  }
}
</script>

<template>
  <form class="space-y-3" @submit.prevent="submit">
    <div class="form-control">
      <label class="label py-1"><span class="label-text text-xs">Title (optional)</span></label>
      <input v-model="title" type="text" class="input-bordered input input-sm w-full" :placeholder="clause.label" />
    </div>

    <div ref="formHost"></div>

    <p v-if="formError" class="text-error text-xs">{{ formError }}</p>

    <div class="flex items-center justify-end gap-2 pt-1">
      <button v-if="showCancel" type="button" class="btn btn-ghost btn-sm" @click="emit('cancel')">Cancel</button>
      <button type="submit" class="btn btn-sm btn-primary">
        {{ submitLabel ?? 'Add typed clause' }}
      </button>
    </div>
  </form>
</template>
