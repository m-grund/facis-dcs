<script setup lang="ts">
import type { SignatureContract } from '@/models/signature/signature-contract'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { computed } from 'vue'

defineProps<{
  contract: SignatureContract
}>()

const authStore = useAuthStore()

const isContractSigner = computed(() => authStore.user?.roles?.includes('CONTRACT_SIGNER') ?? false)
</script>

<template>
  <li class="list-row min-w-0 w-full">
    <div class="list-col-grow card bg-base-100 card-border hover:bg-base-300 min-w-0 w-full border-base-content/10">
      <div class="card-body min-w-0">
        <h2 class="card-title flex-wrap sm:justify-between">
          <div class="flex gap-8 sm:h-full">
            <div>Name: {{ contract.name }}</div>
          </div>
          <div class="badge badge-secondary">{{ contract.state }}</div>
        </h2>
        <div class="flex justify-start">
          <div v-if="contract.contract_version">Version: {{ contract.contract_version }}</div>
        </div>
        <div class="flex justify-between min-w-0">
          <div>Creation date: {{ new Date(contract.created_at).toLocaleString() }}</div>
          <div v-if="contract.description" class="px-10 flex-1 min-w-0 truncate hidden sm:block">
            {{ contract.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="
                isContractSigner
                  ? { name: ROUTES.SIGNATURE_MANAGEMENT.VIEW.CONTRACT, params: { did: contract.did } }
                  : '#'
              "
              class="btn btn-sm btn-primary"
              :class="{ 'btn-disabled': !isContractSigner }"
            >
              View
            </RouterLink>
          </div>
        </div>
      </div>
    </div>
  </li>
</template>
