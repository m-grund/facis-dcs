<script setup lang="ts">
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { ContractState, contractStates } from '@/types/contract-state'
import { computed } from 'vue'

const props = defineProps<{
  contract: Contract
}>()

const authStore = useAuthStore()
const contractsStore = useContractsStore()

const canEdit = computed(() => {
  return props.contract.created_by === authStore.user?.username && props.contract.state === ContractState.draft
})

const hasNegotiationTask = computed(() => contractsStore.hasNegotiationTask(props.contract))
const hasReviewTask = computed(() => contractsStore.hasReviewTask(props.contract))
const hasApprovalTask = computed(() => contractsStore.hasApprovalTask(props.contract))

const resolveViewRouteName = computed(() => {
  if (props.contract.state === ContractState.negotiation && hasNegotiationTask.value) {
    return ROUTES.CONTRACTS.NEGOTIATE
  }
  if (props.contract.state === ContractState.submitted && hasReviewTask.value) {
    return ROUTES.CONTRACTS.REVIEW
  }
  if (props.contract.state === ContractState.reviewed && hasApprovalTask.value) {
    return ROUTES.CONTRACTS.APPROVE
  }
  return ROUTES.CONTRACTS.VIEW
})

function daysUntil(date: string | Date): number {
  const target = new Date(date)
  const now = new Date()
  
  // Beide auf Mitternacht normalisieren für reine Tagesberechnung
  target.setHours(0, 0, 0, 0)
  now.setHours(0, 0, 0, 0)
  
  const diffMs = target.getTime() - now.getTime()
  return Math.ceil(diffMs / (1000 * 60 * 60 * 24))
}

function getExpirationBadgeClass(days: number, noticePeriod?: number): string {
  if (!noticePeriod) {
    return "flex"
  }

  const threshold = Math.floor(noticePeriod/3)
  if (days > noticePeriod) {
    return "flex"
  } else if (days <= noticePeriod && days > threshold) {
    return "flex badge badge-warning"
  } else {
    return "flex badge badge-error"
  }
}

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
        <div class="flex justify-end">
          <div v-if="contract.contract_version">Version: {{ contract.contract_version }}</div>
        </div>
        <div class="flex justify-between min-w-0">
          <div>Creation date: {{ new Date(contract.created_at).toLocaleDateString() }}</div>
          <div v-if="contract.description" class="px-10 flex-1 min-w-0 truncate hidden sm:block">
            {{ contract.description }}
          </div>
          <div class="card-actions justify-end">
            <RouterLink
              :to="{ name: resolveViewRouteName, params: { did: contract.did } }"
              class="btn btn-sm btn-primary"
            >
              View
            </RouterLink>
            <RouterLink
              :to="
                canEdit
                  ? {
                      name: ROUTES.CONTRACTS.EDIT,
                      params: { did: contract.did },
                    }
                  : '#'
              "
              class="btn btn-sm btn-primary gap-2"
              :class="{ 'btn-disabled': !canEdit }"
            >
              Edit
            </RouterLink>
          </div>
        </div>
        <div v-if="contract?.state !== ContractState.draft" class="flex justify-between">
            <div v-if="contract?.exp_date">Expiration date: {{ new Date(contract?.exp_date ?? '').toLocaleDateString() }}</div>
            <div :class="getExpirationBadgeClass(daysUntil(contract?.exp_date), contract?.exp_notice_period)" v-if="(contract?.exp_date && contract?.exp_notice_period) && daysUntil(contract?.exp_date) > 0">Contract expires in {{daysUntil(contract?.exp_date) }} days</div>
        </div>
      </div>
    </div>
  </li>
</template>
