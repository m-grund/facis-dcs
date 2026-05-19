<script setup lang="ts">
import type { SignatureContract } from '@/models/signature/signature-contract';
import { signatureManagementService } from '@/services/signature-management-service';
import { ref, watch, type Ref } from 'vue';

const props = defineProps<{
  did: string
}>()

const contract: Ref<SignatureContract | null> = ref(null)
const envelope: Ref<unknown> = ref(null)

watch(() => props.did, async (newVal, oldVal) => {
  if (newVal === oldVal) return

  const response = await signatureManagementService.retrieveByID({did: props.did})
  contract.value = response.contract
  envelope.value = response.signature_envelope
}, { immediate: true })
</script>

<template>
  <div v-if="contract">Contract: {{ contract }}</div>
  <div v-if="envelope">Signature Envelope: {{ envelope }}</div>
</template>
