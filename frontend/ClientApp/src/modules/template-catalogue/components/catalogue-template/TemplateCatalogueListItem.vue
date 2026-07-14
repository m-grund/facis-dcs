<script setup lang="ts">
import { ROUTES } from '@/router/router'
import { toProperCase } from '@/utils/string'
import type { TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'

const props = defineProps<{
  template: TemplateResourcesItem
  templates: TemplateResourcesItem[]
}>()

function existLocally(did: string): boolean {
  const result = props.templates.filter((contract) => contract.did === did)
  if (result.length > 0) {
    return true
  }
  return false
}
</script>

<template>
  <li class="list-row w-full min-w-0">
    <div class="list-col-grow card w-full min-w-0 border-base-content/10 bg-base-100 card-border hover:bg-base-300">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ template.name }}</div>
            <div v-if="template.template_type" class="badge badge-accent sm:h-full sm:badge-md">
              {{ toProperCase(template.template_type) }}
            </div>
          </div>
          <RouterLink
            :to="{
              name: ROUTES.TEMPLATE_CATALOGUES.VIEW,
              params: { did: template.did },
              query: {
                version: template.version,
              },
            }"
            class="btn btn-sm btn-primary"
          >
            View
          </RouterLink>
        </h2>
        <div class="flex justify-between">
          <div v-if="template.document_number">Document number: {{ template.document_number }}</div>
          <div v-if="template.version">Version: {{ template.version }}</div>
        </div>
        <div v-if="template.did" class="min-w-0 text-sm break-all">DID: {{ template.did }}</div>
        <div v-if="template.created_at">Creation date: {{ new Date(template.created_at).toLocaleDateString() }}</div>
        <div v-if="template.description?.trim()" class="min-w-0 truncate">Description: {{ template.description }}</div>
        <div class="flex min-w-0 justify-between">
          <div v-if="existLocally(template.did)" class="flex min-w-0 justify-between">In local repository</div>
          <div v-else class="flex min-w-0 justify-between">In catalogue</div>
        </div>
      </div>
    </div>
  </li>
</template>
