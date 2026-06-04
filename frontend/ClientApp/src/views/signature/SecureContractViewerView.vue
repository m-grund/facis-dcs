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
  <div class="flex min-h-full flex-col">
    <div class="sticky top-0 z-20 shrink-0 border-b border-base-300 bg-base-100">
      <div class="mx-auto max-w-4xl px-6 pt-3">
        <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">Secure Contract Viewer</p>
      </div>
    </div>
    <div v-if="contract" class="flex h-full flex-1 flex-col md:flex-row">
      <div class="min-h-full w-full overflow-y-auto md:flex-1 md:overflow-x-hidden"><ViewContractView /></div>
      <div class="m-20 mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-1 md:flex-row md:items-end">
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="verify">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Verify
        </button>
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="applySignature">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Apply
        </button>
        <button class="btn flex-1 btn-primary" :disabled="isSubmitting" @click="validate">
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Validate
        </button>
      </div>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
      </div>
    </div>
  </div>
</template>
