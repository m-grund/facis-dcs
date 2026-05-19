<script setup lang="ts">
import SignatureList from '@/components/lists/signature/SignatureList.vue'
import type { SignatureContract } from '@/models/signature/signature-contract'
import { signatureManagementService } from '@/services/signature-management-service'
import { onMounted, ref, type Ref } from 'vue'

const loading = ref(false)
const error = ref<string | null>(null)
const contracts: Ref<SignatureContract[]> = ref([])

const loadSignedContracts = async () => {
  loading.value = true
  error.value = null
  try {
    const data = await signatureManagementService.retrieve()
    contracts.value = data.contracts
  } catch (err: any) {
    error.value = err.message || 'Error loading the contracts'
  } finally {
    loading.value = false
  }
}

onMounted(loadSignedContracts)
</script>

<template>
  <div class="flex bg-base-100 border-b border-base-content/10 justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>
  </div>
  <div>
    <div v-if="loading" class="pl-4">Loading Contracts...</div>
    <div v-else-if="error" class="pl-4">{{ error }}</div>
    <div v-else><SignatureList :contracts="contracts" /></div>
  </div>
</template>
