import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

const storage = typeof window === 'undefined' ? null : window.sessionStorage

export const useSignStore = defineStore('sign', () => {
  const username = ref(storage?.getItem('emos_username') ?? '')
  const authorizationHeader = ref(storage?.getItem('emos_authorization') ?? '')
  const authEnabled = ref(true)
  const authenticated = ref(false)
  const fileStorages = ref<string[]>([])

  const isSignedIn = computed(() => !authEnabled.value || authenticated.value)

  function setSession(nextUsername: string, authorization: string) {
    username.value = nextUsername.trim()
    authorizationHeader.value = authorization
    storage?.setItem('emos_username', username.value)
    storage?.setItem('emos_authorization', authorizationHeader.value)
  }

  function setAuthState(enabled: boolean, isAuthenticated: boolean) {
    authEnabled.value = enabled
    authenticated.value = isAuthenticated
  }

  function setFileStorages(nextFileStorages: string[]) {
    fileStorages.value = [...nextFileStorages]
  }

  function signOut() {
    username.value = ''
    authorizationHeader.value = ''
    authenticated.value = false
    storage?.removeItem('emos_username')
    storage?.removeItem('emos_authorization')
  }

  function authorization() {
    return authorizationHeader.value
  }

  return {
    username,
    authorizationHeader,
    authEnabled,
    authenticated,
    fileStorages,
    isSignedIn,
    setSession,
    setAuthState,
    setFileStorages,
    signOut,
    authorization,
  }
})

export default useSignStore
