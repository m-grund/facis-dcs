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
  <div class="flex flex-col min-h-full">
    <div class="sticky top-0 z-20 shrink-0 bg-base-100 border-b border-base-300">
      <div class="max-w-4xl mx-auto px-6 pt-3">
        <p class="text-xs font-black uppercase tracking-widest text-base-content/40 mb-2">Secure Contract Viewer</p>
      </div>
    </div>
    <div v-if="contract" class="flex-1 flex flex-row h-full">
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
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
      </div>
    </div>
  </div>
</template>
