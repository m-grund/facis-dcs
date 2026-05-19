<script setup lang="ts">
import type { SignatureContract } from '@/models/signature/signature-contract'
import SignatureListItem from './SignatureListItem.vue'

defineProps<{
  contracts: SignatureContract[]
}>()
</script>

<template>
  <ul class="list">
    <template v-if="contracts.length > 0">
      <SignatureListItem
        v-for="contract in contracts"
        :key="`${contract.did}|${contract.contract_version}`"
        :contract="contract"
      />
    </template>
    <li v-else class="px-4">No contracts found.</li>
  </ul>
</template>
