<script setup lang="ts">
import type { SignatureContract } from '@/models/signature/signature-contract'
import { compareValues } from '@/utils/comparison'
import { computed, ref } from 'vue'
import ListSort from '../ListSort.vue'
import SignatureContractListItem from './SignatureContractListItem.vue'

const props = defineProps<{
  contracts: SignatureContract[]
}>()

const sorter = new Map<keyof SignatureContract, string>([
  ['created_at', 'Creation date'],
  ['updated_at', 'Update date'],
  ['name', 'Name'],
])
const defaultSort = sorter.keys().next().value!
const sortBy = ref(defaultSort)
const sortOrder = ref(1)

const sortedContracts = computed(() => {
  if (!sorter.has(sortBy.value)) return props.contracts
  return props.contracts
    .slice()
    .sort((contractA, contractB) => compareValues(contractA, contractB, sortBy.value, sortOrder.value))
})
</script>

<template>
  <ul class="list">
    <li class="flex flex-col justify-end px-4 tracking-wide sm:flex-row">
      <ListSort v-model:sort-by="sortBy" v-model:sort-order="sortOrder" :sorter="sorter" />
    </li>
    <template v-if="contracts.length > 0">
      <SignatureContractListItem
        v-for="contract in sortedContracts"
        :key="`${contract.did}|${contract.contract_version}`"
        :contract="contract"
      />
    </template>
    <li v-else class="px-4">No contracts found.</li>
  </ul>
</template>
