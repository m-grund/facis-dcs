<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ROUTES } from '@/router/router'
import { type SignatureContract, signatureManagementService } from '@/services/signature-management-service'

// SM-01: retrieve approved contracts prepared for signing. Each row opens the
// per-contract Secure Contract Viewer, where signing is driven step by step.
const contracts = ref<SignatureContract[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

onMounted(async () => {
  loading.value = true
  try {
    // The retrieve feed also carries ACTIVE (already-executed) contracts for the
    // compliance viewer; the signing workspace only lists contracts still to sign.
    const all = await signatureManagementService.retrieveContracts()
    contracts.value = all.filter((c) => c.state === 'APPROVED' || c.state === 'SIGNED')
  } catch {
    error.value = 'Failed to load contracts for signing.'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mb-4 flex justify-between border-b border-base-content/10 bg-base-100 p-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">Signing</h2>
  </div>

  <div class="p-4">
    <div v-if="loading" class="text-base-content/60">Loading approved contracts…</div>
    <div v-else-if="error" class="mb-4 alert alert-error">{{ error }}</div>
    <div v-else-if="contracts.length === 0" class="text-base-content/60">
      Nothing awaits your signature. Contracts appear here once they are approved for signing.
    </div>

    <div v-else class="overflow-x-auto">
      <table class="table w-full table-zebra">
        <thead>
          <tr>
            <th>DID</th>
            <th>Name</th>
            <th>Version</th>
            <th>Updated</th>
            <th>State</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="contract in contracts" :key="contract.did">
            <td class="max-w-xs truncate font-mono text-xs">{{ contract.did }}</td>
            <td>{{ contract.name ?? '—' }}</td>
            <td>{{ contract.contract_version ?? 1 }}</td>
            <td>{{ new Date(contract.updated_at).toLocaleDateString() }}</td>
            <td>
              <span class="badge badge-ghost badge-sm">{{ contract.state }}</span>
            </td>
            <td>
              <RouterLink
                class="btn btn-sm btn-primary"
                :to="{ name: ROUTES.SIGNING.VIEWER, params: { did: contract.did } }"
              >
                Open &amp; sign
              </RouterLink>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
