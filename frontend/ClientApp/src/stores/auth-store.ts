import { mapRoleLabelsToUserRoles, rolesFromJwtPayload, type UserRole } from '@/types/user-role'
import { useJwt } from '@vueuse/integrations/useJwt'
import { defineStore } from 'pinia'
import { computed, ref, type Ref } from 'vue'
import { useAuthTokenStore } from './auth-token-store'

interface User {
  issuer: string
  holder: string
  roles: UserRole[]
}

export const useAuthStore = defineStore('auth', () => {
  const authTokenStore = useAuthTokenStore()
  const user: Ref<User | null> = ref(null)

  const isAuthenticated = computed(() => !!user.value && authTokenStore.isAuthSet)

  function setHolder(holder: string) {
    const authTokenStore = useAuthTokenStore()
    const payload = useJwt<{
      sub?: string
      roles?: unknown
      ext?: { iss?: string, roles?: unknown }
    }>(authTokenStore.accessToken).payload.value

    if (payload?.sub !== holder) {
      console.error('User Error: JWT sub mismatch', { expected: holder, sub: payload?.sub })
      return
    }

    const roles = mapRoleLabelsToUserRoles(rolesFromJwtPayload(payload))
    if (roles.length === 0) {
      console.error('User Error: Hydra access token has no mapped roles', { sub: payload.sub })
      return
    }

    user.value = {
      holder: holder,
      issuer: payload?.ext?.iss ?? '',
      roles,
    }
  }

  function remove() {
    user.value = null
  }

  return { user, isAuthenticated, setHolder, remove }
})
