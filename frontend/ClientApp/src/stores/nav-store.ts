import { ROUTES } from '@/router/router'
import { defineStore } from 'pinia'
import { ref, type Ref } from 'vue'
import { useRouter, type RouteLocationNormalized } from 'vue-router'

export const useNavStore = defineStore('nav', () => {
  const previousRoute: Ref<RouteLocationNormalized | null> = ref(null)
  const router = useRouter()

  function goToPreviousRoute() {
    if (!previousRoute.value) {
      router.push({ name: ROUTES.HOME })
    } else {
      router.push(previousRoute.value)
    }
  }

  return { previousRoute, goToPreviousRoute }
})
