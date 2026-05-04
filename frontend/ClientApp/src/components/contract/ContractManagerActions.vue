<script setup lang="ts">
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import { computed, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'

defineOptions({
  inheritAttrs: false,
})

const props = defineProps<{
  contract: Contract
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const authStore = useAuthStore()

const isManager = computed(() => {
  return authStore.user?.roles?.includes('CONTRACT_MANAGER') ?? false
})

const canTerminate = computed(() => {
  return isManager && props.contract.state !== ContractState.terminated
})

const terminate = async () => {
  try {
    if (!confirmationModal.value) return
    const { isCanceled, data: reason } = await confirmationModal.value.reveal({
      message: 'Proceed with terminating?',
      editor: { requiredText: true, placeholder: 'Reason' },
    })
    if (!reason) {
      console.error('Reason is required for termination')
      return
    }
    if (!isCanceled) {
      const response = await contractWorkflowService.terminate({
        did: props.contract.did,
        updated_at: props.contract.updated_at,
        reason: reason,
      })
      if (response.did) {
        router.push({ name: ROUTES.CONTRACTS.LIST })
      }
    }
  } catch (err) {
    console.error('Termination failed:', err)
  }
}
</script>

<template>
  <button v-if="canTerminate" :class="[$attrs.class, 'hover:btn-error']" @click="terminate">Terminate</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
