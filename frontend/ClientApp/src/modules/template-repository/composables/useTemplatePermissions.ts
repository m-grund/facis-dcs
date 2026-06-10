import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth-store'

export const useTemplatePermissions = () => {
  const authStore = useAuthStore()

  const isCreator = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_CREATOR') ?? false
  })

  const isReviewer = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_REVIEWER') ?? false
  })

  const isApprover = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_APPROVER') ?? false
  })

  const isManager = computed(() => {
    return authStore.user?.roles?.includes('TEMPLATE_MANAGER') ?? false
  })

  return { isCreator, isReviewer, isApprover, isManager }
}
