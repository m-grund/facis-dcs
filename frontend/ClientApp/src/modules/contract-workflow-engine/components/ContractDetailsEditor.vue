<script setup lang="ts">
import { ref, useId } from 'vue'
import type { Contract } from '@/models/contract/contract'

defineProps<{
  disabled?: boolean
}>()

const contract = defineModel<Contract>('contract', { required: true })
const inserted = defineModel<ContractDetailData>('inserted', { required: false })

function getTemplateLink(contract: Contract): string {
  return `/ui/templates/view/${contract.template_did}`
}

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

const nameId = useId()
const descriptionId = useId()
const exp_notice_periodId = useId()
const exp_policyId = useId()
const responsiblesId = useId()

const originalContract = ref(Object.assign({}, contract.value))
</script>

<template>
  <div class="card border border-base-300 bg-base-100 shadow-sm">
    <div class="card-body gap-5">
      <h2 class="card-title justify-between text-sm">
        <div class="flex gap-2">Contract Details</div>
        <div class="badge badge-sm badge-secondary">{{ contract.state }}</div>
      </h2>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Version: {{ contract.contract_version }}</legend>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Base Template</legend>
        <a class="badge badge-sm badge-primary" :href="getTemplateLink(contract)">{{ contract.template_did }}</a>
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Global Name</legend>
        <label :for="nameId" class="sr-only">Global Name</label>
        <input
          v-if="!inserted?.name"
          :id="nameId"
          v-model="contract.name"
          class="input-bordered input w-full"
          :class="{ 'border-2 input-primary': !!inserted && originalContract.name !== contract.name }"
          type="text"
          :disabled="disabled"
          required
        />
        <input
          v-else
          :id="nameId"
          v-model="inserted.name"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.name !== contract.name }"
          type="text"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Base Description</legend>
        <label :for="descriptionId" class="sr-only">Base Description</label>
        <textarea
          v-if="!inserted?.description"
          :id="descriptionId"
          v-model="contract.description"
          class="textarea-bordered textarea h-24 w-full"
          :class="{ 'border-2 textarea-primary': originalContract.description !== contract.description }"
          :disabled="disabled"
          required
        />
        <textarea
          v-else
          :id="descriptionId"
          v-model="inserted.description"
          class="textarea-bordered textarea h-24 w-full"
          :class="{ 'text-red-400': !!inserted && inserted.description !== contract.description }"
          disabled
        />
      </fieldset>

      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Notice Period (in days)</legend>
        <label :for="exp_notice_periodId" class="sr-only">Expiration Notice Period (in days)</label>
        <input
          v-if="!inserted?.exp_notice_period"
          :id="exp_notice_periodId"
          v-model="contract.exp_notice_period"
          type="number"
          min="0"
          class="input w-full"
          :disabled="disabled"
        />
        <input
          v-else
          :id="exp_notice_periodId"
          v-model="inserted.exp_notice_period"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_notice_period !== contract.exp_notice_period?.toString() }"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <legend class="fieldset-legend">Expiration Policy</legend>
        <label :for="exp_policyId" class="sr-only">Expiration Policy</label>
        <select
          v-if="!inserted?.exp_policy"
          :id="exp_policyId"
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
          :id="exp_policyId"
          v-model="inserted.exp_policy"
          type="text"
          class="input-bordered input w-full"
          :class="{ 'text-red-400': inserted.exp_policy !== contract.exp_policy }"
          disabled
        />
      </fieldset>
      <fieldset class="fieldset border-none p-0">
        <div class="collapse-arrow collapse [&>input~.collapse-title::after]:scale-75">
          <label :for="responsiblesId" class="sr-only">Responsible Participants</label>
          <input :id="responsiblesId" type="checkbox" name="responsibles" />
          <legend class="collapse-title fieldset-legend pl-0 font-semibold">Responsible Participants</legend>
          <div class="collapse-content grid">
            <ul class="list col-start-1 row-start-1">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Creator</li>
              <li class="list-row py-0">{{ contract.responsible?.creator }}</li>
            </ul>
            <ul class="list col-start-2 row-start-1">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Approvers:</li>
              <li v-for="(approver, i) in contract.responsible?.approvers" :key="i + approver" class="list-row py-0">
                {{ approver }}
              </li>
            </ul>
            <ul class="list col-start-1 row-start-2">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Negotiators:</li>
              <li
                v-for="(negotiator, i) in contract.responsible?.negotiators"
                :key="i + negotiator"
                class="list-row py-0"
              >
                {{ negotiator }}
              </li>
            </ul>
            <ul class="list col-start-2 row-start-2">
              <li class="p-4 pb-2 text-xs tracking-wide opacity-60">Reviewers</li>
              <li v-for="(reviewer, i) in contract.responsible?.reviewers" :key="i + reviewer" class="list-row py-0">
                {{ reviewer }}
              </li>
            </ul>
          </div>
        </div>
      </fieldset>
    </div>
  </div>
</template>
