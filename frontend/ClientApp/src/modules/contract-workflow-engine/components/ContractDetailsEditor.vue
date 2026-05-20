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
        <legend class="fieldset-legend">Start Date</legend>
        <input
          v-if="!inserted?.start_date"
          v-model="startDateLocal"
          type="datetime-local"
          class="input input-bordered w-full"
          :min="minStartDate"
          @change="onStartDateChange"
          :disabled="disabled"
        />
        <input
          v-else
          type="text"
          v-model="inserted.start_date"
          class="input input-bordered w-full"
          :class="{ 'text-red-400': inserted.start_date !== contract.start_date }"
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
        <select v-if="!inserted?.exp_policy" v-model="contract.exp_policy" class="select w-full" :disabled="disabled">
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
      <fieldset v-if="showResponsiblities" class="fieldset p-0 border-none">
        <div class="collapse collapse-arrow [&>input~.collapse-title::after]:scale-75">
          <input type="checkbox" name="responsibles" />
          <legend class="fieldset-legend collapse-title font-semibold pl-0">Responsible Persons</legend>
          <div class="collapse-content grid">
            <ul class="list col-start-1 row-start-1">
              <li class="p-4 pb-2 text-xs opacity-60 tracking-wide">Creator</li>
              <li class="list-row py-0">{{ contract.responsible_persons?.creator }}</li>
            </ul>
            <ul class="list col-start-2 row-start-1">
              <li class="p-4 pb-2 text-xs opacity-60 tracking-wide">Approver</li>
              <li class="list-row py-0">{{ contract.responsible_persons?.approver }}</li>
            </ul>
            <ul class="list col-start-1 row-start-2">
              <li class="p-4 pb-2 text-xs opacity-60 tracking-wide">Negotiators:</li>
              <li
                v-for="(negotiator, i) in contract.responsible_persons?.negotiators"
                :key="i + negotiator"
                class="list-row py-0"
              >
                {{ negotiator }}
              </li>
            </ul>
            <ul class="list col-start-2 row-start-2">
              <li class="p-4 pb-2 text-xs opacity-60 tracking-wide">Reviewers</li>
              <li
                v-for="(reviewer, i) in contract.responsible_persons?.reviewers"
                :key="i + reviewer"
                class="list-row py-0"
              >
                {{ reviewer }}
              </li>
            </ul>
          </div>
        </div>
      </fieldset>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ContractState } from '@/types/contract-state';
import { ref, watch, computed } from 'vue'

const props = defineProps<{
  contract: Contract
  inserted?: ContractDetailData
  disabled?: boolean
}>()

const expDateLocal = ref<string>('')
const startDateLocal = ref<string>('')

watch(
  () => props.contract.exp_date,
  (val) => {
    if (!val) return
    // "2026-05-09T11:24:00Z" → "2026-05-09T11:24"
    expDateLocal.value = val.slice(0, 16)
  },
  { immediate: true },
)

watch(
  () => props.contract.start_date,
  (val) => {
    if (!val) return
    // "2026-05-09T11:24:00Z" → "2026-05-09T11:24"
    startDateLocal.value = val.slice(0, 16)
  },
  { immediate: true },
)

function onExpDateChange() {
  if (!expDateLocal.value) {
    props.contract.exp_date = undefined
    return
  }
  // "2026-05-09T11:24" → "2026-05-09T11:24:00Z"
  props.contract.exp_date = new Date(expDateLocal.value + ':00Z').toISOString()
}

function onStartDateChange() {
  if (!startDateLocal.value) {
    props.contract.start_date = undefined
    return
  }
  // "2026-05-09T11:24" → "2026-05-09T11:24:00Z"
  props.contract.start_date = new Date(startDateLocal.value + ':00Z').toISOString()
}

const expirationPolicies = [
  { name: 'Renewal', value: 'RENEWAL' },
  { name: 'Archiving', value: 'ARCHIVING' },
  { name: 'Termination', value: 'TERMINATION' },
]

interface ContractDetailData {
  name?: string
  description?: string
  start_date?: string
  exp_date?: string
  exp_notice_period?: string
  exp_policy?: string
}

const minStartDate = computed(() => {
  const tomorrow = new Date()
  tomorrow.setDate(tomorrow.getDate() + 1)
  tomorrow.setHours(0, 0, 0, 0)
  return tomorrow.toISOString().slice(0, 16) // "2026-05-09T00:00"
})

const minExpDate = computed(() => {
  const tomorrow = new Date()
  tomorrow.setDate(tomorrow.getDate() + 1)
  tomorrow.setHours(0, 0, 0, 0)
  return tomorrow.toISOString().slice(0, 16) // "2026-05-09T00:00"
})

const originalContract = ref(Object.assign({}, props.contract))

const showResponsiblities = computed(() => !([ContractState.draft, ContractState.terminated] as ContractState[]).includes(props.contract.state))
</script>
