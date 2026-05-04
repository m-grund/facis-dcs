<script setup lang="ts">
import { contractTemplateService } from '@/services/contract-template-service'
import { useTemplateDraftStore } from '../store/templateDraftStore'
import { ref, watch, type Ref } from 'vue'
import { useTemplateEditorUiStore } from '../store/templateEditorUiStore'
import type { ContractTemplateAuditResponse } from '@/models/responses/template-response'
import TemplateAuditList from '@/components/lists/template/TemplateAuditList.vue'

const store = useTemplateDraftStore()
const editorStore = useTemplateEditorUiStore()
const data: Ref<ContractTemplateAuditResponse> = ref([])


const isLoading = ref(false)

const loadAudit = async () => {
  const did = store.did
  if (!did) return
  try {
    isLoading.value = true
    data.value = await contractTemplateService.audit({ did })
  } catch (err) {
    console.error('Audit failed', err)
  } finally {
    isLoading.value = false
  }
}

watch(
  () => editorStore.activeTab === 'audit',
  async (value) => {
    if (value) await loadAudit()
    else data.value = []
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="isLoading" class="loading loading-spinner loading-sm"></div>
  <div v-else-if="data.length < 1">No audit data</div>
  <TemplateAuditList v-else :audits="data" />
</template>
