<script setup lang="ts">
import type { SignatureContract } from '@/models/signature/signature-contract'
import { signatureManagementService } from '@/services/signature-management-service'
import { ref, watch, type Ref } from 'vue'
import ViewContractView from '../contract/ViewContractView.vue'

const props = defineProps<{
  did: string
}>()

const contract: Ref<SignatureContract | null> = ref(null)
const envelope: Ref<unknown> = ref(null)

const isSubmitting = ref(false)

watch(
  () => props.did,
  async (newVal, oldVal) => {
    if (newVal === oldVal) return

    const response = await signatureManagementService.retrieveByID({ did: props.did })
    contract.value = response.contract
    envelope.value = response.signature_envelope
  },
  { immediate: true },
)

const verify = async () => {
  if (!contract.value) return

  isSubmitting.value = true
  try {
    const response = await signatureManagementService.verify({ did: contract.value.did })
    console.log(response.did)
  } catch (err) {
    console.error('Verify Error:', err)
  } finally {
    isSubmitting.value = false
  }
}

const applySignature = async () => {
  if (!contract.value) return

  isSubmitting.value = true
  try {
    const response = await signatureManagementService.apply({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
    })
    console.log(response.did)
  } catch (err) {
    console.error('Apply Error:', err)
  } finally {
    isSubmitting.value = false
  }
}

const validate = async () => {
  if (!contract.value) return

  isSubmitting.value = true

  try {
    const response = await signatureManagementService.validate({ did: contract.value.did })
    console.log(response.did)
  } catch (err) {
    console.error('Validate Error:', err)
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <div v-if="contract" class="flex flex-row h-full">
    <div class="w-1/2 min-h-full overflow-y-auto"><ViewContractView /></div>
    <div class="flex-1 m-20 max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3 md:items-end">
      <button class="btn btn-primary flex-1" @click="verify" :disabled="isSubmitting">
        <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>Verify
      </button>
      <button class="btn btn-primary flex-1" @click="applySignature" :disabled="isSubmitting">
        <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>Apply
      </button>
      <button class="btn btn-primary flex-1" @click="validate" :disabled="isSubmitting">
        <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>Validate
      </button>
    </div>
  </div>
</template>
