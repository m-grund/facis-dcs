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
  const valueSubject = props.initialValues?.['@id']
  if (props.initialValues && typeof valueSubject === 'string' && valueSubject !== '') {
    el.setAttribute('data-values', await instanceToNQuads(props.initialValues))
    el.setAttribute('data-value-subject', valueSubject)
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

/** shacl-form serializes one subject as several fragment node objects
 *  sharing an @id; in JSON-LD those are the same node — merge them. */
function mergeNodesById(nodes: unknown[]): Record<string, unknown>[] {
  const byId = new Map<string, Record<string, unknown>>()
  const anonymous: Record<string, unknown>[] = []
  for (const raw of nodes) {
    if (typeof raw !== 'object' || raw === null) continue
    const node = raw as Record<string, unknown>
    const id = node['@id']
    if (typeof id !== 'string' || !id) {
      anonymous.push(node)
      continue
    }
    const target = byId.get(id)
    if (!target) {
      byId.set(id, { ...node })
      continue
    }
    for (const [key, value] of Object.entries(node)) {
      if (key === '@id') continue
      const existing = target[key]
      if (existing === undefined) {
        target[key] = value
      } else {
        target[key] = [
          ...(Array.isArray(existing) ? existing : [existing]),
          ...(Array.isArray(value) ? value : [value]),
        ]
      }
    }
  }
  return [...byId.values(), ...anonymous]
}

const RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type'

/** shacl-form serializes the type as an expanded rdf:type property; JSON-LD
 *  keyword form (@type) is what every consumer targets. */
function canonicalizeInstanceType(instance: DcsTypedClauseInstance): void {
  if (instance['@type'] || !(RDF_TYPE in instance)) return
  const raw = instance[RDF_TYPE]
  const ids = (Array.isArray(raw) ? raw : [raw])
    .map((node) => (typeof node === 'object' && node !== null ? (node as { '@id'?: string })['@id'] : String(node)))
    .filter((id): id is string => !!id)
  if (ids.length === 0) return
  instance['@type'] = (ids.length === 1 ? ids[0] : ids) as DcsTypedClauseInstance['@type']
  delete instance[RDF_TYPE]
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
    const nodes = mergeNodesById(Array.isArray(serialized) ? serialized : [serialized])
    const instance = nodes.find(isInstanceNode)
    if (!instance) {
      formError.value = 'The form produced no values'
      return
    }
    canonicalizeInstanceType(instance)
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
