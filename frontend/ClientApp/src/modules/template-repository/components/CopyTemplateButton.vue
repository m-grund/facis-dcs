<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useTemplatePermissions } from '../composables/useTemplatePermissions'
import { useTemplateDraftStore } from '../store/templateDraftStore'

const router = useRouter()
const draftStore = useTemplateDraftStore()

const { isCreator, isManager } = useTemplatePermissions()

const copyTemplate = async () => {
  if (!draftStore.did || (!isCreator && !isManager)) return

  const response = await contractTemplateService.copy({ did: draftStore.did })
  if (response.did) {
    await router.push({ name: ROUTES.TEMPLATES.EDIT, params: { did: response.did } })
  }
}
</script>

<template>
  <button @click="copyTemplate">Copy</button>
</template>
