<script setup lang="ts">
import type { PartialContractTemplate } from '@/models/contract-template'
import type { Contract } from '@/models/contract/contract'
import { ROUTES } from '@/router/router'
import { contractWorkflowService } from '@/services/contract-workflow-service'
import type { SemanticConditionValueSetter } from '@/modules/contract-workflow-engine/models/contract-content-values-store'
import {
  useSemanticValueVerification,
  type VerificationResult,
} from '@/modules/contract-workflow-engine/composables/useSemanticValueVerification'
import { useContractDataPreprocess } from '@/modules/contract-workflow-engine/composables/useContractDataPreprocess'
import { useErrorStore } from '@/stores/error-store'
import { ContractState } from '@/types/contract-state'
import ContractDetailsEditor from '@/modules/contract-workflow-engine/components/ContractDetailsEditor.vue'
import { useContractEditorUiStore } from '@/modules/contract-workflow-engine/store/contractEditorUiStore'
import { useContractContentValuesStore } from '@/modules/contract-workflow-engine/store/contractContentValuesStore'
import TemplatePreview from '@template-repository/components/builder-editor/preview/TemplatePreview.vue'
import { useTemplateDraftStore } from '@template-repository/store/templateDraftStore'
import { storeToRefs } from 'pinia'
import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import type { ContractData } from '@/models/contract-data'
import { useTemplateEditorUiStore } from '@/modules/template-repository/store/templateEditorUiStore'
import BuilderEditor from '@template-repository/components/BuilderEditor.vue'
import AddBlockModal from '@template-repository/components/builder-editor/AddBlockModal.vue'
import SemanticRulesEditor from '@template-repository/components/SemanticRulesEditor.vue'
import ClausesEditor from '@template-repository/components/ClausesEditor.vue'
import BuilderPreviewDialog from '@template-repository/components/builder-editor/BuilderPreviewDialog.vue'
import ViewContractTemplateView from '@/modules/template-repository/views/ViewContractTemplateView.vue'
import { useScrollStore } from '@/core/store/scroll'
import {
  buildContractDocument,
  getSemanticConditionsFromTemplateData,
} from '@/modules/template-repository/store/dcsDraftStore'
import { useContractsStore } from '@/stores/contracts-store'
import ParticipantSelectionDialog from '@/components/ParticipantSelectionDialog.vue'
import type { ParticipantSelection } from '@/utils/participant-selection'

const route = useRoute()
const router = useRouter()

const errorStore = useErrorStore()
const contractStore = useContractsStore()

const { hasApprovedTemplates, approvedTemplates } = storeToRefs(contractStore)
const templateDraftStore = useTemplateDraftStore()
const contractContentValuesStore = useContractContentValuesStore()
const contractEditorUiStore = useContractEditorUiStore()
const templateEditorUiStore = useTemplateEditorUiStore()
const { hasConditionParameterForValue, verifySemanticValue } = useSemanticValueVerification()
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)

const did = ref<string | null>(null)
const isEditMode = computed(() => !!route.params.did || !!did.value)
const isSubmitting = ref(false)
const selectedTemplate: Ref<PartialContractTemplate | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)
const selectedParentContractDid = ref<string | null>(null)

const contract: Ref<Contract | null> = ref(null)

const draftContracts = computed(() => contractStore.contracts.filter((c) => c.state === ContractState.draft))

const canSubmit = computed(() => isEditMode.value || (hasApprovedTemplates.value && selectedTemplate.value !== null))
const canSubmitContract = computed(
  () =>
    isEditMode.value &&
    contract.value !== null &&
    (contract.value.state === ContractState.draft || contract.value.state === ContractState.rejected),
)

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  if (!isEditMode.value) return null
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const tabs = computed(() => contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft))

function buildCurrentContractData(): ContractData | undefined {
  if (!contract.value) return undefined
  return buildContractDocument({
    documentId: contract.value.did,
    name: contract.value.name,
    description: contract.value.description,
    blocks: templateDraftStore.blocks,
    layout: templateDraftStore.layout,
    contractData: templateDraftStore.contractData,
    policies: templateDraftStore.policies,
    subTemplateSnapshots: templateDraftStore.subTemplateSnapshots,
    semanticConditionValues: contractContentValuesStore.semanticConditionValues,
    parentContractDid: selectedParentContractDid.value ?? undefined,
    sourceTemplate: contract.value.contract_data?.sourceTemplate,
    derivedFromTemplate: contract.value.contract_data?.derivedFromTemplate,
  })
}

function currentExpNoticePeriod(): number | undefined {
  const value = contract.value?.exp_notice_period as unknown
  if (value === undefined || value === null || value === '') return undefined
  const numericValue = Number(value)
  return Number.isFinite(numericValue) ? numericValue : undefined
}

async function saveContractDraftForSubmit(): Promise<Contract> {
  if (!contract.value) throw new Error('No contract selected')

  await contractWorkflowService.update({
    did: contract.value.did,
    updated_at: contract.value.updated_at,
    exp_notice_period: currentExpNoticePeriod(),
    exp_policy: contract.value.exp_policy,
    name: contract.value.name,
    description: contract.value.description,
    contract_data: buildCurrentContractData(),
  })

  const updatedContract = await contractWorkflowService.retrieveById({ did: contract.value.did })
  if (!updatedContract) throw new Error('Could not reload contract after update')
  contract.value = updatedContract
  return updatedContract
}

function verifySemanticValues(): boolean {
  const subTemplateSemanticConditions = templateDraftStore.subTemplateSnapshots.map((subTemplate) => ({
    templateId: subTemplate.did,
    version: subTemplate.version,
    document_number: subTemplate.document_number,
    semanticConditions: getSemanticConditionsFromTemplateData(subTemplate.template_data),
  }))
  const result = verifySemanticValue(
    templateDraftStore.semanticConditions,
    subTemplateSemanticConditions,
    contractContentValuesStore.semanticConditionValues,
    templateDraftStore.blocks,
  )
  verificationResult.value = result
  if (result.isValid) {
    return true
  }
  result.errors.forEach((error) => errorStore.add(error.message))
  contractEditorUiStore.setActiveTab('content')
  return false
}

const createContract = async ({ reviewers, approvers, negotiators }: ParticipantSelection) => {
  isSubmitting.value = true
  try {
    if (selectedTemplate.value) {
      const response = await contractWorkflowService.create({
        template_did: selectedTemplate.value.did,
        reviewers,
        approvers,
        negotiators,
      })
      did.value = response.did
      if (selectedParentContractDid.value) {
        const newContract = await contractWorkflowService.retrieveById({ did: response.did })
        if (!newContract?.contract_data) {
          throw new Error('Could not reload created contract')
        }
        const contractData = {
          ...newContract.contract_data,
          'dcs:parentContract': { '@id': selectedParentContractDid.value },
        } as ContractData
        await contractWorkflowService.update({
          did: newContract.did,
          updated_at: newContract.updated_at,
          contract_data: contractData,
        })
      }
      errorStore.add('Contract created.', 'info')
    }
  } catch (error) {
    console.error('creation failed', error)
  } finally {
    isSubmitting.value = false
  }
}

const updateContract = async () => {
  isSubmitting.value = true
  try {
    if (contract.value) {
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        exp_notice_period: currentExpNoticePeriod(),
        exp_policy: contract.value.exp_policy,
        name: contract.value.name,
        description: contract.value.description,
        contract_data: buildCurrentContractData(),
      })
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    console.error('Submission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

const submitContract = async ({ reviewers, approvers, negotiators }: ParticipantSelection) => {
  if (!contract.value || !verifySemanticValues()) return
  isSubmitting.value = true
  try {
    const updatedContract = await saveContractDraftForSubmit()
    const response = await contractWorkflowService.submit({
      did: updatedContract.did,
      updated_at: updatedContract.updated_at,
      reviewers,
      approvers,
      negotiators,
    })
    if (response.did) {
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    console.error('Contract submission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

const submitRejectedContract = async () => {
  if (!contract.value || !verifySemanticValues()) return
  isSubmitting.value = true
  try {
    const updatedContract = await saveContractDraftForSubmit()
    const response = await contractWorkflowService.submit({
      did: updatedContract.did,
      updated_at: updatedContract.updated_at,
    })
    if (response.did) {
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    console.error('Contract resubmission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

watch(
  isEditMode,
  async (value) => {
    if (value) {
      try {
        const id = did.value ?? route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          applyContractDataToDraft(contract.value?.contract_data)
          selectedParentContractDid.value = contract.value?.contract_data?.['dcs:parentContract']?.['@id'] ?? null
          const uneditableStates = [ContractState.approved, ContractState.terminated].map((s) => s.toLowerCase())
          templateEditorUiStore.setTemplateEditable(
            !uneditableStates.includes(contract.value?.state.toLowerCase() ?? ''),
          )
        }
      } catch (err: unknown) {
        console.error('Failed to load contract', err)
      }
    } else if (!hasApprovedTemplates.value) {
      await contractStore.loadApprovedTemplates()
    }
  },
  { immediate: true },
)

onMounted(async () => {
  if (!contractStore.hasContracts) {
    await contractStore.loadContracts()
  }
})

watch(
  () => [templateDraftStore.blocks, templateDraftStore.semanticConditions, templateDraftStore.subTemplateSnapshots],
  () => {
    const invalidValues = contractContentValuesStore.semanticConditionValues.filter(
      (conditionValue) =>
        !hasConditionParameterForValue(
          conditionValue,
          templateDraftStore.blocks,
          templateDraftStore.semanticConditions,
          templateDraftStore.subTemplateSnapshots,
        ),
    )
    contractContentValuesStore.removeSemanticConditionValues(invalidValues)
  },
  { deep: true },
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
  const cd = preprocessContractData(contractData)
  if (cd) {
    templateDraftStore.reset({
      workflow: 'contract',
      blocks: cd.blocks,
      layout: cd.layout,
      contractData: cd.contractData,
      policies: cd.policies,
      subTemplateSnapshots: cd.subTemplateSnapshots,
    })
    contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
  } else {
    templateDraftStore.reset({ workflow: 'contract' })
    contractContentValuesStore.reset()
  }
  verificationResult.value = null
}

const scrollStore = useScrollStore()

watch(selectedTemplate, (value) => {
  if (!!value?.did) {
    scrollStore.addGutter()
  } else {
    scrollStore.removeGutter()
  }
})

onBeforeRouteLeave(() => {
  scrollStore.removeGutter()
})
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="!isEditMode" class="flex flex-1 flex-col">
      <div v-if="!selectedTemplate" class="flex flex-1 items-center justify-center px-6 py-20">
        <select v-model="selectedTemplate" class="select w-150" :disabled="!hasApprovedTemplates">
          <option :value="null" disabled selected>
            {{ hasApprovedTemplates ? 'Pick a template' : 'No templates available' }}
          </option>
          <option v-for="template in approvedTemplates" :key="template.did" :value="template">
            Version {{ template.version }} - {{ template.name?.slice(0, 80)
            }}{{ (template.name?.length ?? 0) > 80 ? '…' : '' }}
          </option>
        </select>
      </div>
      <ViewContractTemplateView v-else :did="selectedTemplate.did" :embedded="true">
        <template #before-tabs>
          <div class="flex items-end gap-4">
            <div class="flex-1">
              <p class="mb-1 text-xs font-semibold text-base-content/60">Template</p>
              <select v-model="selectedTemplate" class="select w-full select-sm">
                <option v-for="template in approvedTemplates" :key="template.did" :value="template">
                  Version {{ template.version }} - {{ template.name?.slice(0, 80)
                  }}{{ (template.name?.length ?? 0) > 80 ? '…' : '' }}
                </option>
              </select>
            </div>
            <div v-if="draftContracts.length > 0" class="flex-1">
              <p class="mb-1 text-xs font-semibold text-base-content/60">Add to existing contract (optional)</p>
              <select v-model="selectedParentContractDid" class="select w-full select-sm">
                <option :value="null">— none —</option>
                <option v-for="c in draftContracts" :key="c.did" :value="c.did">
                  {{ c.name ?? c.did }}
                </option>
              </select>
            </div>
          </div>
        </template>
      </ViewContractTemplateView>
    </div>
    <div v-else-if="!!contract" class="flex flex-1 flex-col">
      <div class="flex flex-1 flex-col">
        <!-- Tabs -->
        <div class="sticky top-0 z-10 shrink-0 border-b border-base-300 bg-base-100">
          <div class="mx-auto max-w-4xl px-6 pt-3">
            <p class="mb-2 text-xs font-black tracking-widest text-base-content/40 uppercase">
              {{ isEditMode ? 'Update Contract' : 'Create Contract' }}
            </p>
            <div role="tablist" class="tabs-border tabs tabs-lg">
              <a
                v-for="tab in tabs"
                :key="tab.id"
                role="tab"
                class="tab"
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
                <ContractDetailsEditor :contract="contract" />
              </div>
              <div v-show="activeTab === 'content'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <div>
                      <TemplatePreview
                        :layout="templateDraftStore.layout"
                        :blocks="templateDraftStore.blocks"
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
              <!-- SEMANTIC RULES TAB -->
              <div v-show="activeTab === 'semantic'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <SemanticRulesEditor />
                  </div>
                </div>
              </div>

              <!-- CLAUSES TAB -->
              <div v-show="activeTab === 'clauses'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body gap-5">
                    <ClausesEditor />
                  </div>
                </div>
              </div>

              <!-- BUILDER TAB -->
              <div v-show="activeTab === 'builder'">
                <div class="card border border-base-300 bg-base-100 shadow-sm">
                  <div class="card-body">
                    <div class="mb-2 flex items-center justify-between">
                      <h2 class="card-title text-sm">Builder</h2>
                      <button
                        type="button"
                        class="btn btn-sm btn-secondary"
                        @click="templateEditorUiStore.togglePreviewDialog"
                      >
                        Preview
                      </button>
                    </div>
                    <BuilderEditor />
                  </div>
                </div>
                <AddBlockModal />
                <BuilderPreviewDialog />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="sticky bottom-0 shrink-0 border-t border-base-300 bg-base-100">
      <div class="mx-auto flex max-w-4xl flex-col gap-3 px-6 py-3 md:flex-row">
        <button class="btn btn-outline md:w-32" @click="$router.back()">Back</button>
        <ParticipantSelectionDialog
          v-if="!isEditMode"
          :disabled="isSubmitting || !canSubmit"
          class="btn flex-1 btn-primary"
          @submit="createContract"
        />
        <button
          v-if="isEditMode"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting || !canSubmit"
          @click="updateContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Update
        </button>
        <ParticipantSelectionDialog
          v-if="contract?.state === ContractState.draft && canSubmitContract"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting"
          @submit="submitContract"
        />
        <button
          v-else-if="contract?.state === ContractState.rejected && canSubmitContract"
          class="btn flex-1 btn-primary"
          :disabled="isSubmitting"
          @click="submitRejectedContract"
        >
          <span v-if="isSubmitting" class="loading loading-sm loading-spinner"></span>
          Submit
        </button>
      </div>
    </div>
  </div>
</template>
