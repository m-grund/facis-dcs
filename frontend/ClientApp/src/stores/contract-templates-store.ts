import type { PartialContractTemplate } from '@/models/contract-template'
import type { ContractTemplateApprovalTask } from '@/models/contract-template-approval-task'
import type { ContractTemplateReviewTask } from '@/models/contract-template-review-task'
import { contractTemplateService } from '@/services/contract-template-service'
import { useAuthStore } from '@/stores/auth-store'
import { TemplateState } from '@/types/contract-template-state'
import { defineStore } from 'pinia'
import { computed, ref, type Ref } from 'vue'
import { TemplateType } from '@template-repository/models/contract-templace.ts'

export const useContractTemplatesStore = defineStore('contractTemplates', () => {
  const contractTemplates: Ref<PartialContractTemplate[]> = ref([])
  const paginatedTemplates: Ref<PartialContractTemplate[]> = ref([])
  const reviewTasks: Ref<ContractTemplateReviewTask[]> = ref([])
  const approvalTasks: Ref<ContractTemplateApprovalTask[]> = ref([])

  const loading = ref(false)
  const error = ref<string | null>(null)

  const authStore = useAuthStore()

  const hasTemplates = computed(() => contractTemplates.value.length > 0)
  const hasApprovedOrPublishedTemplates = computed(() =>
    contractTemplates.value.some(
      (template) =>
        (template.state === TemplateState.approved || template.state === TemplateState.published) &&
        template.template_type === TemplateType.frameContract,
    ),
  )
  const approvedOrPublishedTemplates = computed(() =>
    contractTemplates.value.filter(
      (template) =>
        (template.state === TemplateState.approved || template.state === TemplateState.published) &&
        template.template_type === TemplateType.frameContract,
    ),
  )

  const findTemplateByDid = (did: string) => contractTemplates.value.find((template) => template.did === did)

  const fetchTemplates = async (limit?: number, offset?: number) =>
    await contractTemplateService.retrieve({ limit, offset })

  async function loadTemplates() {
    loading.value = true
    error.value = null
    try {
      const data = await fetchTemplates()
      contractTemplates.value = data.contract_templates
      reviewTasks.value = data.review_tasks.map((task) => ({ ...task, type: 'template' }))
      approvalTasks.value = data.approval_tasks.map((task) => ({ ...task, type: 'template' }))
    } catch (err: unknown) {
      error.value = err instanceof Error && err.message ? err.message : 'Error loading the templates'
    } finally {
      loading.value = false
    }
  }

  async function loadPaginatedTemplates(currentPage: number, limit: number) {
    loading.value = true
    error.value = null
    try {
      const offset = currentPage
      const paginatedResult = await fetchTemplates(limit, offset)
      paginatedTemplates.value = paginatedResult.contract_templates
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Error loading templates'
    } finally {
      loading.value = false
    }
  }

  function hasReviewTask(template: PartialContractTemplate): boolean {
    return reviewTasks.value.some((task) => {
      const isDidMatch = task.did === template.did
      const isVersionMatch = !template.version || task.version === template.version
      const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
      return isDidMatch && isVersionMatch && isDocumentNumberMatch && task.reviewer === authStore.user?.username
    })
  }

  function hasApprovalTask(template: PartialContractTemplate): boolean {
    return approvalTasks.value.some((task) => {
      const isDidMatch = task.did === template.did
      const isVersionMatch = !template.version || task.version === template.version
      const isDocumentNumberMatch = !template.document_number || task.document_number === template.document_number
      return isDidMatch && isVersionMatch && isDocumentNumberMatch && task.approver === authStore.user?.username
    })
  }

  return {
    contractTemplates,
    reviewTasks,
    approvalTasks,
    hasTemplates,
    hasApprovedOrPublishedTemplates,
    approvedOrPublishedTemplates,
    paginatedTemplates,
    findTemplateByDid,
    fetchTemplates,
    loadTemplates,
    loadPaginatedTemplates,
    hasReviewTask,
    hasApprovalTask,
    loading,
    error,
  }
})
