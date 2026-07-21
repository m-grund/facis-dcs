<script setup lang="ts">
import { ref, useId } from 'vue'
import { registerSchema } from '@/services/semantic-hub-service'

/**
 * Publishes a brand-new (name, kind) hub entry from pasted or uploaded raw
 * content — how an operator brings an external vocabulary (a Gaia-X shapes
 * graph, a partner ontology) into the running instance without a rebuild.
 * Registering further versions of an existing entry happens on the entry's
 * own panel (RegisterVersionForm).
 */

const KINDS = ['context', 'shapes', 'profile', 'ontology', 'clause-catalog'] as const

const MEDIA_TYPE_BY_KIND: Record<(typeof KINDS)[number], string> = {
  context: 'application/ld+json',
  shapes: 'text/turtle',
  profile: 'application/yaml',
  ontology: 'text/turtle',
  'clause-catalog': 'text/turtle',
}

const emit = defineEmits<{
  published: [name: string, kind: string, version: number]
}>()

const name = ref('')
const kind = ref<(typeof KINDS)[number]>('shapes')
const content = ref('')
const activate = ref(true)
const submitting = ref(false)
const error = ref<string | null>(null)

const nameId = useId()
const kindId = useId()
const contentId = useId()

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  content.value = await file.text()
  input.value = ''
}

async function submit() {
  if (!name.value.trim() || !content.value.trim() || submitting.value) return
  submitting.value = true
  error.value = null
  try {
    const result = await registerSchema({
      name: name.value.trim(),
      kind: kind.value,
      media_type: MEDIA_TYPE_BY_KIND[kind.value],
      content: content.value,
      activate: activate.value,
    })
    emit('published', name.value.trim(), kind.value, result.version)
    name.value = ''
    content.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Publishing failed'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <form class="space-y-3" @submit.prevent="submit">
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div class="form-control">
        <label :for="nameId" class="label py-1 text-base-content/70">
          <span class="label-text text-xs">Name</span>
        </label>
        <input
          :id="nameId"
          v-model="name"
          type="text"
          class="input-bordered input input-sm w-full"
          placeholder="e.g. gaiax-participant"
          aria-label="Entry name"
        />
      </div>
      <div class="form-control">
        <label :for="kindId" class="label py-1 text-base-content/70">
          <span class="label-text text-xs">Kind</span>
        </label>
        <select :id="kindId" v-model="kind" class="select-bordered select w-full select-sm" aria-label="Entry kind">
          <option v-for="option in KINDS" :key="option" :value="option">{{ option }}</option>
        </select>
      </div>
    </div>
    <div class="form-control">
      <label :for="contentId" class="label py-1 text-base-content/70">
        <span class="label-text text-xs">Content ({{ MEDIA_TYPE_BY_KIND[kind] }})</span>
        <label class="label-text-alt link cursor-pointer text-xs">
          Upload file
          <input type="file" class="hidden" accept=".ttl,.jsonld,.json,.yaml,.yml,text/*" @change="onFileSelected" />
        </label>
      </label>
      <textarea
        :id="contentId"
        v-model="content"
        class="textarea-bordered textarea h-40 w-full resize-y font-mono text-xs"
        placeholder="Paste the vocabulary's raw content, or upload a file"
        spellcheck="false"
        aria-label="Entry content"
      />
    </div>
    <div class="flex items-center justify-between gap-3">
      <label class="label cursor-pointer gap-2 py-0 text-base-content/70">
        <input v-model="activate" type="checkbox" class="checkbox checkbox-sm" />
        <span class="label-text text-xs">Activate immediately</span>
      </label>
      <button type="submit" class="btn btn-sm btn-primary" :disabled="!name.trim() || !content.trim() || submitting">
        <span v-if="submitting" class="loading loading-xs loading-spinner" />
        Publish entry
      </button>
    </div>
    <p v-if="error" class="text-xs text-error">{{ error }}</p>
  </form>
</template>
