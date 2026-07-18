<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import SigningCeremonyDialog from '@/components/signing/SigningCeremonyDialog.vue'
import { useContractPermissions } from '@/modules/contract-workflow-engine/composables/useContractPermissions'
import { ROUTES } from '@/router/router'
import {
  type SignatureContract,
  type SignatureEnvelope,
  signatureManagementService,
  type SignatureValidateResult,
  type SignatureVerifyResult,
} from '@/services/signature-management-service'

// AcroForm field the ceremony binds to; placed by the PDF renderer (see the dashboard).
const SIGNATURE_FIELD_NAME = 'Signature1'
// QES is descoped (ADR-12); AES with PoA is the credential the wallet applies.
const CREDENTIAL_TYPE = 'AES'

const route = useRoute()
const router = useRouter()
const did = computed(() => (Array.isArray(route.params.did) ? route.params.did[0] : route.params.did) ?? '')

const { isSigner, isManager } = useContractPermissions()

const ceremonyDialog = useTemplateRef<InstanceType<typeof SigningCeremonyDialog>>('ceremony-dialog')

const contract = ref<SignatureContract | null>(null)
const loading = ref(true)
const loadError = ref<string | null>(null)

const envelope = ref<SignatureEnvelope | undefined>()
const verifyResult = ref<SignatureVerifyResult | undefined>()
const validateResult = ref<SignatureValidateResult | undefined>()

// After prepare (Apply Signature), the contract awaits the externally-signed
// upload; holds the ceremony's signatory DID for the submit call.
const pendingSignerDid = ref<string | null>(null)

const busy = ref(false)

type StepId = 'retrieve' | 'verify' | 'apply' | 'submit' | 'validate'
const stepError = ref<Partial<Record<StepId, string>>>({})

const done = ref<Record<StepId, boolean>>({
  retrieve: false,
  verify: false,
  apply: false,
  submit: false,
  validate: false,
})

const STEPS: { id: StepId; title: string }[] = [
  { id: 'retrieve', title: 'Retrieve' },
  { id: 'verify', title: 'Verify' },
  { id: 'apply', title: 'Apply Signature' },
  { id: 'submit', title: 'Submit' },
  { id: 'validate', title: 'Validate' },
]

const currentStep = computed<StepId>(() => STEPS.find((s) => !done.value[s.id])?.id ?? 'validate')

function stepState(id: StepId): 'done' | 'active' | 'pending' {
  if (done.value[id]) return 'done'
  return id === currentStep.value ? 'active' : 'pending'
}

const signed = computed(() => envelope.value?.status === 'SIGNED')
const executed = computed(() => done.value.validate && signed.value)

onMounted(async () => {
  try {
    const contracts = await signatureManagementService.retrieveContracts()
    const found = contracts.find((c) => c.did === did.value)
    if (!found) {
      loadError.value = 'This contract is not available for signing under your account.'
      return
    }
    contract.value = found
    done.value.retrieve = true
  } catch (e: unknown) {
    loadError.value = `Failed to retrieve the contract: ${message(e)}`
  } finally {
    loading.value = false
  }
})

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

// Step 2 — integrity of the contract content and its signature envelope.
async function verify() {
  busy.value = true
  delete stepError.value.verify
  try {
    verifyResult.value = await signatureManagementService.verifySignature(did.value)
    done.value.verify = true
  } catch (e: unknown) {
    stepError.value.verify = `Verification failed: ${message(e)}`
  } finally {
    busy.value = false
  }
}

// Step 3 — run the PID ceremony, then fetch the to-be-signed PDF (PoA + summary
// embedded, signature field placed) for the signatory to sign externally (ADR-12).
async function applySignature() {
  busy.value = true
  delete stepError.value.apply
  try {
    const outcome = await ceremonyDialog.value?.reveal({ contractDid: did.value, fieldName: SIGNATURE_FIELD_NAME })
    if (!outcome || outcome.isCanceled || !outcome.data) return
    const prepared = await signatureManagementService.prepareSignature(did.value, outcome.data.signerDid, CREDENTIAL_TYPE)
    downloadBlob(prepared, `${contract.value?.name ?? did.value}-to-sign.pdf`)
    pendingSignerDid.value = outcome.data.signerDid
    done.value.apply = true
  } catch (e: unknown) {
    stepError.value.apply = `Could not prepare the contract for signing: ${message(e)}`
  } finally {
    busy.value = false
  }
}

// Step 4 — upload the externally-signed PDF; the DCS validates sole control and records it.
async function submitSigned(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || !pendingSignerDid.value) return
  busy.value = true
  delete stepError.value.submit
  try {
    envelope.value = await signatureManagementService.submitSignature(
      did.value,
      pendingSignerDid.value,
      CREDENTIAL_TYPE,
      file,
      '',
    )
    pendingSignerDid.value = null
    done.value.submit = true
  } catch (e: unknown) {
    stepError.value.submit = `The signed contract was rejected: ${message(e)}`
  } finally {
    busy.value = false
    input.value = ''
  }
}

// Step 5 — validate the applied signature(s) against trust policies.
async function validate() {
  busy.value = true
  delete stepError.value.validate
  try {
    validateResult.value = await signatureManagementService.validateSignature(did.value)
    done.value.validate = true
  } catch (e: unknown) {
    stepError.value.validate = `Validation failed: ${message(e)}`
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div class="flex items-center justify-between gap-3 border-b border-base-content/10 bg-base-100 p-4">
      <div>
        <p class="text-xs font-black tracking-widest text-base-content/40 uppercase">Secure Contract Viewer</p>
        <h2 class="truncate text-2xl font-bold">{{ contract?.name ?? 'Contract' }}</h2>
      </div>
      <button class="btn btn-outline btn-sm" @click="router.push({ name: ROUTES.SIGNING.LIST })">Back to list</button>
    </div>

    <div v-if="loading" class="p-6 text-base-content/60">Loading contract…</div>
    <div v-else-if="loadError" class="p-6">
      <div class="alert alert-error">{{ loadError }}</div>
    </div>

    <div v-else class="grid min-h-0 flex-1 grid-cols-1 gap-0 lg:grid-cols-2">
      <!-- LEFT: contract content -->
      <section class="flex min-h-0 flex-col border-base-content/10 lg:border-r">
        <div class="flex items-center gap-2 border-b border-base-content/10 px-4 py-2">
          <h3 class="text-sm font-semibold">Contract document</h3>
          <span class="badge badge-ghost badge-sm">{{ contract?.state }}</span>
          <span v-if="signed" class="badge badge-success badge-sm">{{ envelope?.status }}</span>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto bg-base-200 p-4">
          <div class="card border border-base-300 bg-base-100 shadow-sm">
            <div class="card-body gap-4">
              <div>
                <h4 class="text-lg font-bold">{{ contract?.name ?? 'Untitled contract' }}</h4>
                <p class="font-mono text-xs break-all text-base-content/50">{{ did }}</p>
              </div>
              <p v-if="contract?.description" class="text-sm whitespace-pre-line">{{ contract.description }}</p>
              <p v-else class="text-sm text-base-content/50 italic">No description provided.</p>
              <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                <dt class="text-base-content/50">State</dt>
                <dd>{{ contract?.state }}</dd>
                <dt class="text-base-content/50">Version</dt>
                <dd>{{ contract?.contract_version ?? 1 }}</dd>
                <dt class="text-base-content/50">Created</dt>
                <dd>{{ contract ? new Date(contract.created_at).toLocaleString() : '—' }}</dd>
                <dt class="text-base-content/50">Updated</dt>
                <dd>{{ contract ? new Date(contract.updated_at).toLocaleString() : '—' }}</dd>
              </dl>
              <p class="text-xs text-base-content/50">
                Review the full clauses and machine-readable terms before signing. The to-be-signed PDF with the
                embedded PoA and signing summary is produced at the Apply Signature step.
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- RIGHT: guided signing wizard -->
      <section class="flex min-h-0 flex-col overflow-y-auto p-4">
        <ul class="steps steps-vertical mb-4 w-full lg:steps-horizontal">
          <li
            v-for="step in STEPS"
            :key="step.id"
            class="step"
            :class="{
              'step-primary': stepState(step.id) === 'done',
              'step-info': stepState(step.id) === 'active',
            }"
          >
            {{ step.title }}
          </li>
        </ul>

        <div v-if="executed" class="mb-4 alert alert-success">
          Executed contract submitted. All required signatures are applied and validated.
        </div>

        <div class="flex flex-col gap-4">
          <!-- Step 1: Retrieve -->
          <div class="card border border-base-300 bg-base-100">
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">1 · Retrieve</h4>
                <span class="badge badge-success badge-sm">Retrieved</span>
              </div>
              <p class="text-xs text-base-content/60">
                Approved contract and its signature envelope loaded. Version {{ contract?.contract_version ?? 1 }}.
              </p>
            </div>
          </div>

          <!-- Step 2: Verify -->
          <div
            class="card border bg-base-100"
            :class="stepState('verify') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">2 · Verify integrity &amp; envelope</h4>
                <span v-if="done.verify" class="badge badge-success badge-sm">Verified</span>
              </div>
              <p class="text-xs text-base-content/60">
                Confirm the contract content hash and required signing policies before signing.
              </p>
              <div v-if="verifyResult" class="text-xs" :class="verifyResult.match ? 'text-success' : 'text-error'">
                Hash match: {{ verifyResult.match ? 'match ✓' : 'mismatch ✗' }} ({{ verifyResult.sig_count }} signature(s))
              </div>
              <ul v-if="verifyResult?.findings?.length" class="list-disc pl-5 text-xs text-warning">
                <li v-for="(f, i) in verifyResult.findings" :key="i">{{ f }}</li>
              </ul>
              <div v-if="stepError.verify" class="text-xs text-error">{{ stepError.verify }}</div>
              <div class="card-actions">
                <button class="btn btn-sm btn-primary" :disabled="busy" @click="verify">
                  <span v-if="busy && currentStep === 'verify'" class="loading loading-xs loading-spinner" />
                  {{ done.verify ? 'Re-verify' : 'Verify' }}
                </button>
              </div>
            </div>
          </div>

          <!-- Step 3: Apply Signature -->
          <div
            class="card border bg-base-100"
            :class="stepState('apply') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">3 · Apply signature</h4>
                <span v-if="done.apply" class="badge badge-success badge-sm">Prepared</span>
              </div>
              <p class="text-xs text-base-content/60">
                Present your PID in the wallet ceremony, then sign the downloaded to-be-signed PDF externally
                (credential: {{ CREDENTIAL_TYPE }} with PoA). The DCS holds no signing key.
              </p>
              <div v-if="!isSigner" class="text-xs text-warning">
                Your role can review and validate this contract, but applying a signature requires the Signer role.
              </div>
              <div v-if="stepError.apply" class="text-xs text-error">{{ stepError.apply }}</div>
              <div class="card-actions">
                <button
                  class="btn btn-sm btn-primary"
                  :disabled="busy || !isSigner || !done.verify || signed"
                  @click="applySignature"
                >
                  <span v-if="busy && currentStep === 'apply'" class="loading loading-xs loading-spinner" />
                  {{ done.apply ? 'Re-prepare' : 'Apply Signature' }}
                </button>
              </div>
            </div>
          </div>

          <!-- Step 4: Submit -->
          <div
            class="card border bg-base-100"
            :class="stepState('submit') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">4 · Submit signed contract</h4>
                <span v-if="signed" class="badge badge-success badge-sm">Submitted</span>
              </div>
              <p class="text-xs text-base-content/60">
                Upload the externally-signed PDF. The DCS validates sole control and records the executed contract.
              </p>
              <div v-if="stepError.submit" class="text-xs text-error">{{ stepError.submit }}</div>
              <div class="card-actions">
                <label class="btn btn-sm btn-primary" :class="{ 'btn-disabled': busy || !pendingSignerDid }">
                  <span v-if="busy && currentStep === 'submit'" class="loading loading-xs loading-spinner" />
                  Submit signed PDF
                  <input
                    type="file"
                    accept="application/pdf"
                    class="hidden"
                    :disabled="busy || !pendingSignerDid"
                    @change="submitSigned"
                  />
                </label>
              </div>
            </div>
          </div>

          <!-- Step 5: Validate -->
          <div
            class="card border bg-base-100"
            :class="stepState('validate') === 'active' ? 'border-info' : 'border-base-300'"
          >
            <div class="card-body gap-2 p-4">
              <div class="flex items-center justify-between">
                <h4 class="card-title text-sm">5 · Validate applied signatures</h4>
                <span v-if="done.validate" class="badge badge-success badge-sm">Validated</span>
              </div>
              <p class="text-xs text-base-content/60">Validate the applied signature(s) against trust policies.</p>
              <ul v-if="validateResult?.findings?.length" class="list-disc pl-5 text-xs text-warning">
                <li v-for="(f, i) in validateResult.findings" :key="i">{{ f }}</li>
              </ul>
              <p v-else-if="done.validate" class="text-xs text-success">Signature validation passed with no findings.</p>
              <div v-if="stepError.validate" class="text-xs text-error">{{ stepError.validate }}</div>
              <div class="card-actions">
                <button class="btn btn-sm btn-primary" :disabled="busy || !signed" @click="validate">
                  <span v-if="busy && currentStep === 'validate'" class="loading loading-xs loading-spinner" />
                  {{ done.validate ? 'Re-validate' : 'Validate' }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <p v-if="isManager && !isSigner" class="mt-4 text-xs text-base-content/50">
          Manager view: retrieval, verification and validation are available; signing is performed by a Signer.
        </p>
      </section>
    </div>

    <SigningCeremonyDialog ref="ceremony-dialog" />
  </div>
</template>
