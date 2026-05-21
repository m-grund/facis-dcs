<template>
  <div class="card bg-base-100 border border-base-300 shadow-sm">
    <div class="card-body gap-5">
      <fieldset class="fieldset p-0 border-none">
        <legend class="fieldset-legend">Version: {{ contract.contract_version }}</legend>
      </fieldset>
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
            <div>
              <ul class="list col-start-2 row-start-2">
              <li class="p-4 pb-2 text-xs opacity-60 tracking-wide">Approvers:</li>
              <li
                v-for="(approver, i) in contract.responsible_persons?.approvers"
                :key="i + approver"
                class="list-row py-0"
              >
                {{ approver }}
              </li>
            </ul>
            </div>
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
import { ref, computed } from 'vue'

const props = defineProps<{
  contract: Contract
  inserted?: ContractDetailData
  disabled?: boolean
}>()

const expirationPolicies = [
  { name: 'Renewal', value: 'RENEWAL' },
  { name: 'Archiving', value: 'ARCHIVING' },
  { name: 'Termination', value: 'TERMINATION' },
]

interface ContractDetailData {
  name?: string
  description?: string
  exp_notice_period?: string
  exp_policy?: string
}

const originalContract = ref(Object.assign({}, props.contract))

const showResponsiblities = computed(() => !([ContractState.draft, ContractState.terminated] as ContractState[]).includes(props.contract.state))
</script>
