<script setup lang="ts">
import ContractList from '@/components/lists/contract/ContractList.vue'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { storeToRefs } from 'pinia'
import { computed, onMounted } from 'vue'

const contractsStore = useContractsStore()

const { contracts, loading, error } = storeToRefs(contractsStore)

const authStore = useAuthStore()

const loadContracts = async () => {
  await contractsStore.loadContracts()
}

const isContractCreator = computed(() => authStore.user?.roles?.some((role) => ['CONTRACT_CREATOR'].includes(role)))

onMounted(loadContracts)
</script>

<template>
  <div class="flex bg-base-100 border-b border-base-content/10 justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      {{ $route.meta.name }}
    </h2>

    <RouterLink
      v-if="isContractCreator"
      :to="{ name: ROUTES.CONTRACTS.NEW }"
      class="btn self-end btn-primary gap-2"
      #default="{ route }"
    >
      {{ route.meta.name }}
    </RouterLink>
    <div v-else></div>
  </div>
  <div>
    <div v-if="loading" class="pl-4">Loading Contracts...</div>
    <div v-else-if="error" class="pl-4">{{ error }}</div>
    <div v-else>
      <ContractList :items="contracts" />
    </div>
  </div>
</template>
