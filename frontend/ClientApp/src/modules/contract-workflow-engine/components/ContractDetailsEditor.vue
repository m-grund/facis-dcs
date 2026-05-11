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
      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Expiration Date</legend>
        <input
          v-if="!inserted?.exp_date"
          v-model="expDateLocal"
          type="datetime-local"
          class="input input-bordered w-full"
          :min="minExpDate"
          @change="onExpDateChange"
          :disabled="disabled"
        />
        <input
          v-else
          type="text"
          v-model="inserted.exp_date"
          class="input input-bordered w-full"
           :class="{ 'text-red-400': inserted.exp_date !== contract.exp_date }"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Expiration Notice Period (in days)</legend>
        <input
          v-if="!inserted?.exp_notice_period"
          v-model="contract.exp_notice_period"
          type="number"
          min="0"
          class="input w-full"
          :disabled="disabled"
        />
        <input
          v-else
          type="text"
          v-model="inserted.exp_notice_period"
          class="input input-bordered w-full"
           :class="{ 'text-red-400': inserted.exp_notice_period !== contract.exp_notice_period?.toString() }"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Expiration Policy</legend>
        <select
          v-if="!inserted?.exp_policy"
          v-model="contract.exp_policy"
          class="select w-full"
          :disabled="disabled"
        >
          <option v-for="policy in expirationPolicies" :key="policy.value" :value="policy.value">
            {{ policy.name }}
          </option>
        </select>
        <input
          v-else
          type="text"
          v-model="inserted.exp_policy"
          class="input input-bordered w-full"
           :class="{ 'text-red-400': inserted.exp_policy !== contract.exp_policy }"
          disabled
        />
      </fieldset>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Contract, ExpirationPolicy } from '@/models/contract/contract'
import { ref, watch, computed} from 'vue'

const props = defineProps<{
  contract: Contract
  inserted?: ContractDetailData
  disabled?: boolean
}>()

const expDateLocal = ref<string>('')

watch(
  () => props.contract.exp_date,
  (val) => {
    if (!val) return
    // "2026-05-09T11:24:00Z" → "2026-05-09T11:24"
    expDateLocal.value = val.slice(0, 16)
  },
  { immediate: true }
)

function onExpDateChange() {
  if (!expDateLocal.value) {
    props.contract.exp_date = undefined
    return
  }
  // "2026-05-09T11:24" → "2026-05-09T11:24:00Z"
  props.contract.exp_date = new Date(expDateLocal.value + ':00Z').toISOString()
}

const expirationPolicies = [
  { name: 'Renewal', value: 'RENEWAL' },
  { name: 'Archiving', value: 'ARCHIVING' },
  { name: 'Termination', value: 'TERMINATION' }
]

interface ContractDetailData {
  name?: string
  description?: string
  exp_date?: string
  exp_notice_period?: string
  exp_policy?: string
}

const minExpDate = computed(() => {
  const tomorrow = new Date()
  tomorrow.setDate(tomorrow.getDate() + 1)
  tomorrow.setHours(0, 0, 0, 0)
  return tomorrow.toISOString().slice(0, 16) // "2026-05-09T00:00"
})

const originalContract = ref(Object.assign({}, props.contract))
</script>
