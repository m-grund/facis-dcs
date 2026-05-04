<script setup lang="ts">
import ContractManagerActions from '@/components/contract/ContractManagerActions.vue'
import NegotiationList from '@/components/lists/contract/negotiation/NegotiationList.vue'
import { useScrollStore } from '@/core/store/scroll'
import type { ContractData } from '@/models/contract-data'
import type { Contract, ContractChangeRequest } from '@/models/contract/contract'
import type { ContractNegotiation } from '@/models/contract/contract-negotiation'
import AuditView from '@/modules/contract-workflow-engine/components/AuditView.vue'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import DiffView from '@/modules/contract-workflow-engine/components/DiffView.vue'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import TemplatePreview from '@/modules/template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useTemplateDraftStore } from '@/modules/template-repository/store/templateDraftStore'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import { useAuthStore } from '@/stores/auth-store'
import { useNavStore } from '@/stores/nav-store'
import { ContractState } from '@/types/contract-state'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const navStore = useNavStore()

const authStore = useAuthStore()
const templateDraftStore = useTemplateDraftStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)
const { setActiveTab } = contractEditorUiStore
const contractContentValuesStore = useContractContentValuesStore()
const scrollStore = useScrollStore()

const username = computed(() => authStore.user?.username)
const isSubmitting = ref(false)

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const isManager = computed(() => useAuthStore().user?.roles?.includes('CONTRACT_MANAGER') ?? false)

const tabs = computed(() => contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft))

const verificationResult: Ref<VerificationResult | null> = ref(null)

const contract: Ref<Contract | null> = ref(null)
const editedContract: Ref<Contract | null> = ref(null)
const compareChangesData: Ref<Contract | null> = ref(null)

const hasChangeRequest = computed(() => {
  return changedName.value || changedDescription.value || changedContractData.value
})

const changedName = computed(() => editedContract.value?.name !== contract.value?.name)
const changedDescription = computed(() => editedContract.value?.description !== contract.value?.description)
const changedContractData = computed(() => {
  return (
    contract.value?.contract_data &&
    !arrayEqual(
      contractContentValuesStore.semanticConditionValues.map((v) => v.parameterValue),
      contract.value.contract_data.semanticConditionValues.map((v) => v.parameterValue),
    )
  )
})

const arrayEqual = (a: unknown[], b: unknown[]) => {
  if (a.length !== b.length) return false
  return a.every((value, i) => value === b[i])
}

watch(
  () => !!route.params.did,
  async (value) => {
    if (value) {
      try {
        const id = route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          editedContract.value = !!contract.value ? { ...contract.value } : null
          applyContractDataToDraft(contract.value?.contract_data)
        }
      } catch (err: any) {
        console.error('Failed to load contract', err)
      }
    }
  },
  { immediate: true },
)

watch(
  () => [
    templateDraftStore.documentBlocks,
    templateDraftStore.semanticConditions,
    templateDraftStore.subTemplateSnapshots,
  ],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(
          conditionValue,
          templateDraftStore.documentBlocks,
          templateDraftStore.semanticConditions,
          templateDraftStore.subTemplateSnapshots,
        ),
    )
    contractContentValuesStore.removeSemanticConditionValues(invalidValues)
  },
  { deep: true },
)

const negotiateContractChange = async () => {
  if (!contract.value || !editedContract.value || !username.value) return
  isSubmitting.value = true
  try {
    const changeRequest: ContractChangeRequest = {}
    if (changedName.value) {
      changeRequest.name = editedContract.value.name
    }
    if (changedDescription.value) {
      changeRequest.description = editedContract.value.description
    }
    if (changedContractData.value) {
      changeRequest.contract_data = { semanticConditionValues: contractContentValuesStore.semanticConditionValues }
    }
    const response = await contractWorkflowService.negotiate({
      did: contract.value?.did,
      updated_at: contract.value?.updated_at,
      negotiated_by: username.value,
      change_request: changeRequest,
    })
    if (response.did) {
      navStore.goToPreviousRoute()
    }
  } catch (err) {
    console.error('Failed to submit change request', err)
  } finally {
    isSubmitting.value = false
  }
}

const submitContract = async () => {
  if (!contract.value) return
  try {
    const response = await contractWorkflowService.submit({
      did: contract.value.did,
      updated_at: contract.value.updated_at,
    })
    if (response.did) {
      navStore.goToPreviousRoute()
    }
  } catch (err) {
    console.error('Failed to submit', err)
  }
}

const hasOpenDecisions = computed(() =>
  contract.value?.negotiations?.every((negotiation) =>
    negotiation.negotiation_decisions.every((decision) => !!decision.decision),
  ),
)

onMounted(() => {
  templateEditorUiStore.reset({ workflow: 'contract' })
})

onUnmounted(() => {
  templateDraftStore.reset({ workflow: 'contract' })
  contractContentValuesStore.reset()
  contractEditorUiStore.reset()
  templateEditorUiStore.reset({ workflow: 'contract' })
  verificationResult.value = null
})

// Contract data includes the template data used to fill the contract template
function applyContractDataToDraft(contractData?: unknown) {
  if (contractData == null) {
    templateDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
    verificationResult.value = null
    return
  }
  const cd = preprocessContractData(contractData as ContractData)
  templateDraftStore.reset({
    workflow: 'contract',
    documentOutline: cd.documentOutline ?? [],
    documentBlocks: cd.documentBlocks ?? [],
    semanticConditions: cd.semanticConditions ?? [],
    subTemplateSnapshots: cd.subTemplateSnapshots ?? [],
    templateDataVersion: cd.templateDataVersion,
  })
  contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  verificationResult.value = null
}

const handleSelectedNegotiation = (negotiation: ContractNegotiation | null, selectedContract: Contract | null) => {
  if (!contract.value || !selectedContract) return
  compareChangesData.value = !!negotiation
    ? {
        ...contract.value,
        name: negotiation.change_request.name
          ? `${contract.value.name} -> ${negotiation.change_request.name}`
          : contract.value.name,
        description: negotiation.change_request.description
          ? `${contract.value.description} -> ${negotiation.change_request.description}`
          : contract.value.description,
        contract_data: contract.value.contract_data, // TODO
      }
    : null
  if (compareChangesData.value) {
    scrollStore.scrollToTop()
  }
}

const shownData = computed(() => {
  if (!!editedContract.value) {
    return editedContract.value
  }
  return contract.value
})

// TODO: The historical contract data function is not ready yet.
// So we just return the current contract and draft data for now.
const priorContractData = computed<ContractData | undefined>(() => {
  const data = contract.value?.contract_data
  if (!data) return undefined
  return preprocessContractData(data)
})

const currentContractData = computed<ContractData | undefined>(() => {
  const data = contract.value?.contract_data
  if (!data) return undefined
  const preprocessedContractData = preprocessContractData(data)

  return {
    ...preprocessedContractData,
    semanticConditionValues: [...contractContentValuesStore.semanticConditionValues],
  }
})
</script>

<template>
  <div class="flex flex-col min-h-full -mx-4 md:-mx-8 -my-4 md:-my-8">
    <div v-if="!!contract && !!editedContract && !!shownData">
      <div class="flex-1 flex flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 bg-base-200 border-b border-base-300">
          <div class="max-w-4xl mx-auto px-6 pt-3">
            <p class="text-xs font-black uppercase tracking-widest text-base-content/40 mb-2">Negotiate Contract</p>
            <div role="tablist" class="tabs tabs-lift tabs-lg">
              <a
                v-for="tab in tabs"
                :key="tab.id"
                role="tab"
                class="tab"
                :class="{ 'tab-active': activeTab === tab.id }"
                @click="setActiveTab(tab.id)"
              >
                {{ tab.label }}
              </a>
            </div>
          </div>
        </div>
        <!-- Tab content -->
        <div class="grow mt-5">
          <div class="max-w-4xl mx-auto p-6">
            <div class="grid grid-cols-1 gap-4">
              <div v-show="activeTab === 'details'">
                <ContractDetailsEditor
                  :contract="shownData"
                  :inserted="{ name: compareChangesData?.name, description: compareChangesData?.description }"
                />
              </div>

              <div v-show="activeTab === 'content'">
                <div class="card bg-base-100 border border-base-300 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :document-outline="templateDraftStore.documentOutline"
                        :document-blocks="templateDraftStore.documentBlocks"
                        :semantic-conditions="templateDraftStore.semanticConditions"
                        :semantic-condition-values="contractContentValuesStore.semanticConditionValues"
                        :verification-result="verificationResult"
                        :sub-template-snapshots="templateDraftStore.subTemplateSnapshots"
                        :set-semantic-condition-value="setSemanticConditionValue"
                      />
                    </div>
                  </div>
                </div>
              </div>

              <div v-show="activeTab === 'diff'">
                <DiffView :prior-contract-data="priorContractData" :current-contract-data="currentContractData" />
              </div>

              <template v-if="isManager">
                <div v-show="activeTab === 'audit'">
                  <div class="card bg-base-100 border border-base-300 shadow-sm">
                    <div class="card-body">
                      <h2 class="card-title text-sm">Audit History</h2>
                      <AuditView />
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </div>
      <template v-if="activeTab !== 'audit'">
        <div class="divider"></div>
        <div class="max-w-4xl mx-auto p-6" v-if="(contract.negotiations?.length ?? -1) > 0">
          <div class="text-lg">Active negotiations</div>
          <NegotiationList
            :contract="contract"
            @selected-negotiation="(negotiation) => handleSelectedNegotiation(negotiation, contract)"
          /></div
      ></template>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="max-w-4xl mx-auto px-6 py-3 flex flex-col md:flex-row gap-3">
        <button class="btn btn-ghost md:w-32" @click="$router.back()">Cancel</button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn btn-primary flex-1"
          :disabled="isSubmitting || !hasChangeRequest"
          @click="negotiateContractChange"
        >
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Submit change request
        </button>
        <button
          v-if="contract?.state === ContractState.negotiation"
          class="btn btn-primary flex-1"
          :disabled="isSubmitting || !hasOpenDecisions"
          @click="submitContract"
        >
          <span v-if="isSubmitting" class="loading loading-spinner loading-sm"></span>
          Submit contract
        </button>
        <ContractManagerActions v-if="contract" :contract="contract" class="btn btn-primary flex-1" />
      </div>
    </div>
  </div>
</template>
