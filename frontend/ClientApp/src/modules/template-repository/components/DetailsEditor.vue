<script setup lang="ts">
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import { storeToRefs } from 'pinia'
import { computed } from 'vue'
import { TemplateType } from '@/modules/template-repository/models/contract-template'
import { TemplateState } from '@/types/contract-template-state'
import { useTemplatePermissions } from '../composables/useTemplatePermissions'

const store = useDcsDraftStore()
const uiStore = useTemplateEditorUiStore()
const { templateType, state, version } = storeToRefs(store)

const { isManager } = useTemplatePermissions()

const document_number = computed({
  get: () => store.document_number,
  set: (value: string) => store.updateDocumentNumber(value),
})

const name = computed({
  get: () => store.name,
  set: (value: string) => store.updateName(value.trim()),
})

const description = computed({
  get: () => store.description,
  set: (value: string) => store.updateDescription(value),
})
</script>

<template>
  <div class="grid grid-cols-1 gap-4">
    <!-- Contract Kind -->
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Version: {{ version }}</legend>
    </fieldset>
    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Contract Type</legend>
      <div class="mt-1 grid grid-cols-2 gap-3">
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.contractTemplate ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Contract</span>
            <p class="text-xs font-normal text-base-content/60">Top-level contract template that can serve as parent</p>
          </div>
        </div>
        <div
          class="pointer-events-none card border-2 transition-all"
          :class="templateType === TemplateType.component ? 'border-primary bg-primary/5' : 'border-base-300'"
        >
          <div class="card-body gap-1 p-4">
            <span class="card-title text-sm">Component</span>
            <p class="text-xs font-normal text-base-content/60">
              Reusable partial contract, embeddable in other templates
            </p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset v-if="isManager" class="fieldset border-none p-0">
      <legend class="fieldset-legend">Template State</legend>
      <select v-model="state" class="input-bordered select w-full" required :disabled="!uiStore.isTemplateEditable">
        <option>DRAFT</option>
        <option>REJECTED</option>
        <option>SUBMITTED</option>
        <option>REVIEWED</option>
        <option>APPROVED</option>
        <option>DELETED</option>
        <option v-if="state == TemplateState.registered">REGISTERED</option>
        <option v-if="state == TemplateState.published">PUBLISHED</option>
      </select>
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Document number</legend>
      <input
        v-model="document_number"
        class="input-bordered input w-full"
        type="text"
        required
        :disabled="!uiStore.isTemplateEditable"
      />
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Global Name</legend>
      <input
        v-model="name"
        class="input-bordered input w-full"
        type="text"
        required
        :disabled="!uiStore.isTemplateEditable"
      />
    </fieldset>

    <fieldset class="fieldset border-none p-0">
      <legend class="fieldset-legend">Base Description</legend>
      <textarea
        v-model="description"
        class="textarea-bordered textarea h-24 w-full"
        required
        :disabled="!uiStore.isTemplateEditable"
      ></textarea>
    </fieldset>
  </div>
</template>
