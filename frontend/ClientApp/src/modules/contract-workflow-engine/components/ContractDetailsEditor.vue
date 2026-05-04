<template>
  <div class="card bg-base-100 border border-base-300 shadow-sm">
    <div class="card-body gap-5">
      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Global Name</legend>
        <input
          v-if="!inserted?.name"
          v-model="contract.name"
          class="input input-bordered w-full"
          :class="{ 'input-primary border-2': !!inserted && originalContract.name !== contract.name }"
          type="text"
          :disabled="disabled"
          required
        />
        <input
          v-else
          v-model="inserted.name"
          class="input input-bordered w-full"
          :class="{ 'text-red-400': inserted.name !== contract.name }"
          type="text"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Base Description</legend>
        <textarea
          v-if="!inserted?.description"
          v-model="contract.description"
          class="textarea textarea-bordered w-full h-24"
          :class="{ 'textarea-primary border-2': originalContract.description !== contract.description }"
          :disabled="disabled"
          required
        />
        <textarea
          v-else
          v-model="inserted.description"
          class="textarea textarea-bordered w-full h-24"
          :class="{ 'text-red-400': !!inserted && inserted.description !== contract.description }"
          disabled
        />
      </fieldset>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ref } from 'vue'

interface ContractDetailData {
  name?: string
  description?: string
}

const props = defineProps<{
  contract: Contract
  inserted?: ContractDetailData
  disabled?: boolean
}>()

const originalContract = ref(Object.assign({}, props.contract))
</script>
