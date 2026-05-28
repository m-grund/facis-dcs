<script setup lang="ts">
import {
  signatureManagementService,
  type SignatureComplianceResult,
  type SignatureContract,
  type SignatureEnvelope,
  type SignatureValidateResult,
  type SignatureVerifyResult,
} from '@/services/signature-management-service'
import { useAuthStore } from '@/stores/auth-store'
import { onMounted, ref } from 'vue'

const authStore = useAuthStore()

const contracts = ref<SignatureContract[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Per-contract state: signing in progress, result envelope, verify result.
const signing = ref<Record<string, boolean>>({})
const envelopes = ref<Record<string, SignatureEnvelope | undefined>>({})
const verifyResults = ref<Record<string, SignatureVerifyResult | undefined>>({})
const validateResults = ref<Record<string, SignatureValidateResult | undefined>>({})
const complianceResults = ref<Record<string, SignatureComplianceResult | undefined>>({})

onMounted(async () => {
  loading.value = true
  try {
    contracts.value = await signatureManagementService.retrieveContracts()
  } catch (e) {
    error.value = 'Failed to load contracts for signing.'
  } finally {
    loading.value = false
  }
})

async function sign(contract: SignatureContract) {
  const signerDid = authStore.user?.name ?? 'unknown'
  signing.value[contract.did] = true
  try {
    const env = await signatureManagementService.applySignature(contract.did, signerDid)
    envelopes.value[contract.did] = env
  } catch (e) {
    error.value = `Failed to sign contract ${contract.did}: ${e}`
  } finally {
    signing.value[contract.did] = false
  }
}

async function verify(contract: SignatureContract) {
  try {
    verifyResults.value[contract.did] = await signatureManagementService.verifySignature(
      contract.did,
    )
  } catch (e) {
    error.value = `Failed to verify contract ${contract.did}: ${e}`
  }
}

async function validate(contract: SignatureContract) {
  try {
    validateResults.value[contract.did] = await signatureManagementService.validateSignature(
      contract.did,
    )
  } catch (e) {
    error.value = `Failed to validate contract ${contract.did}: ${e}`
  }
}

async function compliance(contract: SignatureContract) {
  try {
    complianceResults.value[contract.did] = await signatureManagementService.complianceCheck(
      contract.did,
    )
  } catch (e) {
    error.value = `Failed to run compliance check for ${contract.did}: ${e}`
  }
}
</script>

<template>
  <div class="flex bg-base-100 border-b border-base-content/10 justify-between p-4 mb-4">
    <h2 class="text-2xl/7 font-bold sm:truncate sm:text-3xl sm:tracking-tight">
      Signing Dashboard
    </h2>
  </div>

  <div class="p-4">
    <div v-if="loading" class="text-base-content/60">Loading approved contracts…</div>
    <div v-else-if="error" class="alert alert-error mb-4">{{ error }}</div>
    <div v-else-if="contracts.length === 0" class="text-base-content/60">
      No approved contracts available for signing.
    </div>

    <div v-else class="overflow-x-auto">
      <table class="table table-zebra w-full">
        <thead>
          <tr>
            <th>DID</th>
            <th>Name</th>
            <th>Version</th>
            <th>Updated</th>
            <th>Signature</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="contract in contracts" :key="contract.did">
            <td class="font-mono text-xs max-w-xs truncate">{{ contract.did }}</td>
            <td>{{ contract.name ?? '—' }}</td>
            <td>{{ contract.contract_version ?? 1 }}</td>
            <td>{{ new Date(contract.updated_at).toLocaleDateString() }}</td>
            <td>
              <span
                v-if="envelopes[contract.did]"
                :class="[
                  'badge',
                  envelopes[contract.did]?.status === 'SIGNED' ? 'badge-success' : 'badge-warning',
                ]"
              >
                {{ envelopes[contract.did]?.status }}
              </span>
              <span v-else class="badge badge-ghost">UNSIGNED</span>

              <div
                v-if="verifyResults[contract.did]"
                class="text-xs mt-1"
                :class="verifyResults[contract.did]?.match ? 'text-success' : 'text-error'"
              >
                MR/HR: {{ verifyResults[contract.did]?.match ? 'match ✓' : 'mismatch ✗' }}
                ({{ verifyResults[contract.did]?.sig_count }} sig(s))
              </div>

              <div v-if="validateResults[contract.did]?.findings?.length" class="text-xs mt-1">
                Validation: {{ validateResults[contract.did]?.findings?.[0] }}
              </div>
              <div v-if="complianceResults[contract.did]?.findings?.length" class="text-xs mt-1">
                Compliance: {{ complianceResults[contract.did]?.findings?.[0] }}
              </div>
            </td>
            <td class="flex gap-2">
              <button
                class="btn btn-sm btn-primary"
                :disabled="signing[contract.did]"
                @click="sign(contract)"
              >
                <span v-if="signing[contract.did]" class="loading loading-spinner loading-xs" />
                Sign
              </button>
              <button class="btn btn-sm btn-ghost" @click="verify(contract)">Verify</button>
              <button class="btn btn-sm btn-ghost" @click="validate(contract)">Validate</button>
              <button class="btn btn-sm btn-ghost" @click="compliance(contract)">Compliance</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
