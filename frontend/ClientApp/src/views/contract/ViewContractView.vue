<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, type Ref, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useTemplateEditorUiStore } from '@template-repository/store/templateEditorUiStore'
import AuditView from '@contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@contract-workflow-engine/components/ContractDetailsEditor.vue'
import ContractStructureTree from '@contract-workflow-engine/components/ContractStructureTree.vue'
import { useContractDataPreprocess } from '@contract-workflow-engine/composables/useContractDataPreprocess'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import { useDocumentExport } from '@/composables/useDocumentExport'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import { ContractState } from '@/types/contract-state'
import type { Contract } from '@/models/contract/contract'
import type { UserRole } from '@/types/user-role'
import type { VerificationResult } from '@contract-workflow-engine/composables/useSemanticValueVerification'

const route = useRoute()

const authStore = useAuthStore()
const contractsStore = useContractsStore()
const { contracts } = storeToRefs(contractsStore)
const dcsDraftStore = useDcsDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const contractContentValuesStore = useContractContentValuesStore()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)

const contract: Ref<Contract | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)

const isAuditingAuthorized = computed(
  () =>
    (['AUDITOR', 'COMPLIANCE_OFFICER', 'SYSTEM_ADMINISTRATOR'] as UserRole[]).some((role) =>
      authStore.user?.roles?.includes(role),
    ) ?? false,
)

const tabs = computed(() =>
  contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft).filter((tab) => {
    // Don't show diff tab in the contract view.
    return tab.id !== 'diff'
  }),
)

watch(
  () => !!route.params.did,
  async (value) => {
    if (value) {
      try {
        const id = route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          applyContractDataToDraft(contract.value?.contract_data)
        }
      } catch (err: unknown) {
        console.error('Failed to load contract', err)
      }
    }
  },
  { immediate: true },
)

const parentContract = computed(() => ancestors.value[ancestors.value.length - 1] ?? null)

const ancestors = computed(() => {
  const chain: Contract[] = []
  let currentDid = contract.value?.contract_data?.['dcs:parentContract']?.['@id']
  while (currentDid) {
    const parent = contracts.value.find((c) => c.did === currentDid)
    if (!parent) {
      chain.unshift({ did: currentDid, name: currentDid } as Contract)
      break
    }
    chain.unshift(parent)
    currentDid = parent.parent_contract_did ?? undefined
  }
  return chain
})

const childContracts = computed(() => contracts.value.filter((c) => c.parent_contract_did === contract.value?.did))

const contractTitle = computed(
  () => contract.value?.name ?? contract.value?.contract_data?.['dcs:metadata']?.['dcs:title'] ?? contract.value?.did,
)

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract', isTemplateEditable: false })
  if (contracts.value.length === 0) void contractsStore.loadContracts()
})

onUnmounted(() => {
  dcsDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
  verificationResult.value = null
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    verificationResult.value = null
    return
  }
  const cd = preprocessContractData(contractData)
  if (cd) {
    dcsDraftStore.reset({
      workflow: 'contract',
      documentIri: ((contractData as Record<string, unknown>)['@id'] as string | undefined) ?? null,
      blocks: cd.blocks,
      layout: cd.layout,
      contractData: cd.contractData,
      policies: cd.policies,
      subTemplateSnapshots: cd.subTemplateSnapshots,
    })
    contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  } else {
    dcsDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
  }
  verificationResult.value = null
}

const { download: downloadExport, exporting } = useDocumentExport()

const exportPDF = async () => {
  const did = contract?.value?.did
  if (!did) return
  await downloadExport(() => contractWorkflowService.exportPdf(did), `contract-${did}.pdf`)
}

// The zip bundle of this contract's locally-known hierarchy
// (DCS-FR-CWE-30): the contract, its ancestors, and every descendant this
// instance holds, each as JSON-LD + provenanced PDF plus a manifest.
const exportBundle = async () => {
  const did = contract?.value?.did
  if (!did) return
  await downloadExport(() => contractWorkflowService.exportBundle(did), `contract-bundle-${did}.zip`)
}
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="!!contract" class="flex flex-1 flex-col">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/70 uppercase">View Contract</p>
            <div role="tablist" class="tabs-border tabs tabs-lg">
              <a
                v-for="tab in tabs"
                :key="tab.id"
                role="tab"
                class="tab text-base-content/70"
                :class="{ 'tab-active text-primary': activeTab === tab.id }"
                @click="contractEditorUiStore.setActiveTab(tab.id)"
              >
                {{ tab.label }}
              </a>
            </div>
          </div>
        </div>
        <!-- Tab content -->
        <div class="mt-5 grow">
          <div class="mx-auto max-w-4xl p-6">
            <div class="grid grid-cols-1 gap-4">
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor :contract="contract" disabled />

                <!-- Deployment KPIs (DCS-FR-CWE-31, DCS-FR-CWE-09) -->
                <div
                  v-if="contract.kpis && contract.kpis.length > 0"
                  class="card mt-4 border border-base-300 bg-base-100 shadow-sm"
                >
                  <div class="card-body gap-2">
                    <h2 class="card-title text-sm">KPIs</h2>
                    <ul class="flex flex-col gap-1">
                      <li
                        v-for="kpi in contract.kpis"
                        :key="`${kpi.metric}-${kpi.observed_at}`"
                        class="flex items-center gap-2 text-sm"
                      >
                        <span class="font-medium">{{ kpi.metric }}</span>
                        <span>{{ kpi.value }}</span>
                        <span class="text-xs text-base-content/40">{{ kpi.observed_at }}</span>
                        <span v-if="kpi.violation" class="badge badge-sm badge-error">Violation</span>
                      </li>
                    </ul>
                  </div>
                </div>

                <!-- Parent contract -->
                <div v-if="parentContract" class="card mt-4 border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-2">
                    <h2 class="card-title text-sm">Part of Contract</h2>
                    <RouterLink
                      :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: parentContract.did } }"
                      class="badge badge-outline badge-primary"
                    >
                      {{ parentContract.name ?? parentContract.did }}
                    </RouterLink>
                  </div>
                </div>

                <!-- Child contracts -->
                <div v-if="childContracts.length > 0" class="card mt-4 border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-3">
                    <h2 class="card-title text-sm">Component Contracts</h2>
                    <div class="flex flex-wrap gap-2">
                      <RouterLink
                        v-for="child in childContracts"
                        :key="child.did"
                        :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: child.did } }"
                        class="badge badge-outline badge-secondary"
                      >
                        {{ child.name ?? child.did }}
                      </RouterLink>
                    </div>
                  </div>
                </div>
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :layout="dcsDraftStore.layout"
                        :blocks="dcsDraftStore.blocks"
                        :semantic-conditions="dcsDraftStore.semanticConditions"
                        :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                        :verification-result="verificationResult"
                        :sub-template-snapshots="dcsDraftStore.subTemplateSnapshots"
                      />
                    </div>
                  </div>
                </div>
              </div>

              <template v-if="isAuditingAuthorized">
                <div v-show="activeTab === 'audit'">
                  <div class="card border border-base-300 bg-base-100 shadow-sm">
                    <div class="card-body">
                      <h2 class="card-title text-sm">Audit History</h2>
                      <AuditView />
                    </div>
                  </div>
                </div>
              </template>

              <div v-show="activeTab === 'structure'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-4">
                    <!-- Ancestor chain -->
                    <div v-if="ancestors.length > 0" class="space-y-1">
                      <div
                        v-for="(ancestor, i) in ancestors"
                        :key="ancestor.did"
                        class="flex items-center gap-2 text-sm text-base-content/60"
                        :style="{ paddingLeft: `${i * 1}rem` }"
                      >
                        <span class="shrink-0 text-xs">↑</span>
                        <RouterLink
                          :to="{ name: ROUTES.CONTRACTS.VIEW, params: { did: ancestor.did } }"
                          class="link font-medium text-base-content link-hover"
                          target="_blank"
                        >
                          {{ ancestor.name ?? ancestor.did }}
                        </RouterLink>
                        <span class="badge badge-ghost badge-xs">{{ ancestor.state }}</span>
                      </div>
                    </div>

                    <!-- Current contract -->
                    <div
                      class="flex items-center gap-2"
                      :style="ancestors.length > 0 ? { paddingLeft: `${ancestors.length}rem` } : {}"
                    >
                      <span class="h-2 w-2 shrink-0 rounded-full bg-primary"></span>
                      <span class="text-sm font-semibold">{{ contractTitle }}</span>
                      <span class="badge badge-xs badge-primary">{{ contract.state }}</span>
                    </div>

                    <!-- Children -->
                    <div
                      v-if="childContracts.length > 0"
                      :style="{ paddingLeft: `${ancestors.length + 1}rem` }"
                      class="border-l border-base-300 pl-4"
                    >
                      <ContractStructureTree :root-did="contract.did" :contracts="contracts" />
                    </div>

                    <p v-else-if="ancestors.length === 0" class="text-sm text-base-content/40">
                      This contract has no parent or child contracts.
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <button class="btn btn-outline md:w-32" :disabled="exporting" @click="exportPDF">Export PDF</button>
        <button class="btn btn-outline md:w-36" :disabled="exporting" @click="exportBundle">Export bundle</button>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn flex-1 btn-primary" />
      </div>
    </div>
  </div>
</template>
