import { useLocalStorage } from '@vueuse/core'
import { useJwt } from '@vueuse/integrations/useJwt'
import { defineStore } from 'pinia'
import { computed } from 'vue'

export const useAuthTokenStore = defineStore('token', () => {
  const tokenType = useLocalStorage<string>('token_type', null)
  const accessToken = useLocalStorage<string>('access_token', null)

  const isAuthSet = computed(() => !!tokenType.value && !!accessToken.value)
  const getAuthenticationHeader = computed(() => `${tokenType.value} ${accessToken.value}`)
  const getUserId = computed(() => useJwt(accessToken.value).payload.value?.sub)

  function setTokens(type: string, access_token: string) {
    tokenType.value = type
    accessToken.value = access_token
  }

  function remove() {
    tokenType.value = null
    accessToken.value = null
  }

  return { tokenType, accessToken, isAuthSet, getAuthenticationHeader, getUserId, setTokens, remove }
})
