import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import { useTemplateDraftStore } from '../store/templateDraftStore'

export const useTemplatePermissions = () => {
  const authStore = useAuthStore()
  const draftStore = useTemplateDraftStore()

  const isCreator = computed(() => {
    return draftStore.created_by === authStore.user?.username
  })

  const isManager = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
  })

  return { isCreator, isManager }
}
