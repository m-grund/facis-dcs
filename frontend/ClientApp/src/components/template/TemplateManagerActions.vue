<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { PartialContractTemplate } from '@/models/contract-template'
import { TemplateType } from '@/modules/template-repository/models/contract-template'
import { ROUTES } from '@/router/router'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState, type ContractTemplateState } from '@/types/contract-template-state'
import { computed, normalizeClass, ref, useAttrs, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'

defineOptions({
  inheritAttrs: false,
})

const attrs = useAttrs()

const filteredClass = computed(() => {
  return normalizeClass(attrs.class)
    .split(' ')
    .filter(
      (cls) =>
        !['btn-primary', 'btn-secondary', 'btn-accent', 'btn-success', 'btn-warning', 'btn-error', 'btn-info'].includes(
          cls,
        ),
    )
    .join(' ')
})

const props = defineProps<{
  template: PartialContractTemplate
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const authStore = useAuthStore()

const isPublishing = ref(false)

const isManager = computed(() => {
  return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
})

const canArchive = computed(() => {
  const archiveStates: ContractTemplateState[] = [TemplateState.deleted, TemplateState.deprecated]
  return isManager.value && !archiveStates.includes(props.template.state)
})

const showPublishButton = computed(() => {
  return (
    isManager.value &&
    props.template.state === TemplateState.approved &&
    props.template.template_type === TemplateType.frameContract
  )
})

const archive = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with archiving?' })
    if (!isCanceled) {
      await contractTemplateService.archive({ did: props.template.did, updated_at: props.template.updated_at })
      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Archiving failed:', err)
  }
}

const publish = async () => {
  if (isPublishing.value) return
  try {
    if (!confirmationModal.value) return
    const { isCanceled } = await confirmationModal.value.reveal({ message: 'Proceed with publishing?' })
    if (!isCanceled) {
      isPublishing.value = true
      await contractTemplateService.publish({ did: props.template.did, updated_at: props.template.updated_at })
      await router.push({ name: ROUTES.TEMPLATES.LIST })
    }
  } catch (err) {
    console.error('Publishing failed:', err)
  } finally {
    isPublishing.value = false
  }
}

const exportPdf = async () => {
  const blob = await contractTemplateService.exportPdf(props.template.did)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `template-${props.template.did}.pdf`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <button :class="$attrs.class" @click="exportPdf">Export PDF</button>
  <button v-if="showPublishButton" :class="$attrs.class" :disabled="isPublishing" @click="publish">
    <span v-if="isPublishing" class="loading loading-sm loading-spinner"></span>
    Publish
  </button>
  <button v-if="canArchive" :class="[filteredClass, 'btn-error']" @click="archive">Archive</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
