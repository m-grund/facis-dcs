<script setup lang="ts">
import { computed, normalizeClass, ref, useAttrs, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'
import { useContractPermissions } from '@contract-workflow-engine/composables/useContractPermissions'
import ConfirmationModal from '@/components/ConfirmationModal.vue'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { ContractState } from '@/types/contract-state'
import type { Contract } from '@/models/contract/contract'

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
  contract: Contract
}>()

const confirmationModal = useTemplateRef<InstanceType<typeof ConfirmationModal>>('confirmation-modal')

const router = useRouter()
const { isCreator, isManager } = useContractPermissions()

// SRS DCS-IR-CWE-01 / §1.2 offer→acceptance lifecycle: only a Contract Creator
// may transmit a DRAFT to the counterparty (EventOffer is allowed solely from
// DRAFT — backend command/offer.go gates on the ContractCreator role + this
// transition and derives the offerer from the caller's identity).
const canOffer = computed(() => {
  return isCreator.value && props.contract.state === ContractState.draft
})

const canTerminate = computed(() => {
  return isManager.value && props.contract.state !== ContractState.terminated
})

const canDeploy = computed(() => {
  return isManager.value && props.contract.state === ContractState.signed
})

const offering = ref(false)

const offer = async () => {
  if (!isCreator.value || props.contract.state !== ContractState.draft) return
  offering.value = true
  try {
    await contractWorkflowService.offer({
      did: props.contract.did,
      updated_at: props.contract.updated_at,
    })
    router.go(0)
  } catch (err) {
    console.error('Offer failed:', err)
  } finally {
    offering.value = false
  }
}

const deploying = ref(false)

const deploy = async () => {
  if (!isManager.value || props.contract.state !== ContractState.signed) return
  deploying.value = true
  try {
    await contractWorkflowService.deploy({
      did: props.contract.did,
      updated_at: props.contract.updated_at,
    })
    router.go(0)
  } catch (err) {
    console.error('Deployment failed:', err)
  } finally {
    deploying.value = false
  }
}

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
        await router.push({ name: ROUTES.CONTRACTS.LIST })
      }
    }
  } catch (err) {
    console.error('Termination failed:', err)
  }
}
</script>

<template>
  <button v-if="canOffer" :class="[filteredClass, 'btn-primary']" :disabled="offering" @click="offer">
    {{ offering ? 'Offering…' : 'Offer to counterparty' }}
  </button>
  <button v-if="canDeploy" :class="[filteredClass, 'btn-primary']" :disabled="deploying" @click="deploy">
    {{ deploying ? 'Deploying…' : 'Deploy' }}
  </button>
  <button v-if="canTerminate" :class="[filteredClass, 'btn-error']" @click="terminate">Terminate</button>
  <ConfirmationModal ref="confirmation-modal" />
</template>
