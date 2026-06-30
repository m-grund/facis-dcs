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
  FACIS_CONTRACT_POLICY_REFS,
  FACIS_CONTRACT_VALIDATION_PROFILE,
  FACIS_SCHEMA_REFS,
} from '@/modules/template-repository/models/contract-template'
import { buildSemanticTemplateExtension } from '@/models/semantic/facis-dcs-semantic'
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
const { preprocessContractData } = useContractDataPreprocess()
const { activeTab } = storeToRefs(contractEditorUiStore)

const did = ref<string | null>(null)
const isEditMode = computed(() => !!route.params.did || !!did.value)
const isSubmitting = ref(false)
const selectedTemplate: Ref<PartialContractTemplate | null> = ref(null)
const verificationResult: Ref<VerificationResult | null> = ref(null)

const contract: Ref<Contract | null> = ref(null)

const canSubmit = computed(() => isEditMode.value || (hasApprovedTemplates.value && selectedTemplate.value !== null))

const { hasConditionParameterForValue } = useSemanticValueVerification()

const setSemanticConditionValue = computed<SemanticConditionValueSetter>(() => {
  if (!isEditMode.value) return null
  return (blockId: string, conditionId: string, parameterName: string, parameterValue: string | number | boolean) =>
    contractContentValuesStore.setSemanticConditionValue({ blockId, conditionId, parameterName, parameterValue })
})

const tabs = computed(() => contractEditorUiStore.availableTabs(contract.value?.state ?? ContractState.draft))

watch(
  isEditMode,
  async (value) => {
    if (value) {
      try {
        const id = did.value ?? route.params.did
        if (id && !Array.isArray(id)) {
          contract.value = await contractWorkflowService.retrieveById({ did: id })
          applyContractDataToDraft(contract.value?.contract_data)
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
    schemaRefs: {
      documentStructure: cd.schemaRefs?.documentStructure ?? FACIS_SCHEMA_REFS.documentStructure,
      semanticCondition: cd.schemaRefs?.semanticCondition ?? FACIS_SCHEMA_REFS.semanticCondition,
      contractData: cd.schemaRefs?.contractData ?? FACIS_SCHEMA_REFS.contractData,
    },
    policyRefs: cd.policyRefs ?? FACIS_CONTRACT_POLICY_REFS,
    validation: cd.validation ?? FACIS_CONTRACT_VALIDATION_PROFILE,
    semanticProfile: cd.semanticProfile,
    templateVariables: cd.templateVariables ?? [],
    placeholderBindings: cd.placeholderBindings ?? [],
    semanticRules: cd.semanticRules ?? [],
    sla: cd.sla ?? null,
  })
  contractContentValuesStore.reset({ semanticConditionValues: cd.semanticConditionValues ?? [] })
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

const createContract = async ({ reviewers, approvers, negotiators }: ParticipantSelection) => {
  isSubmitting.value = true
  try {
    if (!!selectedTemplate.value) {
      const response = await contractWorkflowService.create({
        template_did: selectedTemplate.value.did,
      reviewers,
      approvers,
      negotiators
    })
      did.value = response.did
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
      const semanticExtension = buildSemanticTemplateExtension(
        templateDraftStore.documentBlocks,
        templateDraftStore.semanticConditions,
        templateDraftStore.semanticProfile,
      )
      const contractData: ContractData = {
        documentOutline: templateDraftStore.documentOutline,
        documentBlocks: templateDraftStore.documentBlocks,
        semanticConditions: templateDraftStore.semanticConditions,
        subTemplateSnapshots: templateDraftStore.subTemplateSnapshots,
        templateDataVersion: templateDraftStore.templateDataVersion,
        schemaRefs: {
          documentStructure: FACIS_SCHEMA_REFS.documentStructure,
          semanticCondition: FACIS_SCHEMA_REFS.semanticCondition,
          contractData: FACIS_SCHEMA_REFS.contractData,
        },
        policyRefs: FACIS_CONTRACT_POLICY_REFS,
        validation: FACIS_CONTRACT_VALIDATION_PROFILE,
        semanticProfile: semanticExtension.semanticProfile,
        templateVariables: templateDraftStore.templateVariables,
        placeholderBindings: semanticExtension.placeholderBindings,
        semanticRules: semanticExtension.semanticRules,
        sla: templateDraftStore.sla ?? undefined,
        semanticConditionValues: contractContentValuesStore.semanticConditionValues,
      }
      await contractWorkflowService.update({
        did: contract.value.did,
        updated_at: contract.value.updated_at,
        exp_notice_period: contract.value.exp_notice_period,
        exp_policy: contract.value.exp_policy,
        name: contract.value.name,
        description: contract.value.description,
        contract_data: contractData,
      })
      await router.push({ name: ROUTES.CONTRACTS.LIST })
    }
  } catch (error) {
    console.error('Submission failed', error)
  } finally {
    isSubmitting.value = false
  }
}

</script>

<template>
  <div class="-mx-4 -my-4 flex min-h-full flex-col md:-mx-8 md:-my-8">
    <div v-if="!isEditMode" class="px-6 py-12">
      <div class="flex justify-center">
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
      <div v-if="selectedTemplate" class="pt-20">
        <ViewContractTemplateView :did="selectedTemplate.did" />
      </div>
    </div>
    <div v-else-if="!!contract">
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
        <button v-else class="btn flex-1 btn-primary" @click="updateContract">
          Update
        </button>
      </div>
    </div>
  </div>
</template>
