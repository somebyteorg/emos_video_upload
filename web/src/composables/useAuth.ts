import { computed, ref } from 'vue'
import api, { errorMessage, isUnauthorized } from '@/utils/ky'
import signStore from '@/stores/sign'
import type { AuthStatus } from '@/types'

function basicAuthorization(username: string, password: string) {
  const bytes = new TextEncoder().encode(`${username}:${password}`)
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
  return `Basic ${btoa(binary)}`
}

export function useAuth() {
  const sign = signStore()
  const authReady = ref(false)
  const loginBusy = ref(false)
  const loginError = ref('')
  const loginUsername = ref('')
  const loginPassword = ref('')
  const isAuthenticated = computed(() => sign.isSignedIn)
  const authEnabled = computed(() => sign.authEnabled)

  async function checkAuth() {
    authReady.value = false
    loginError.value = ''
    try {
      const status = await api.get('api/auth/status').json<AuthStatus>()
      sign.setAuthState(status.enabled, status.authenticated)
      sign.setFileStorages(status.file_storages)
      return status.authenticated || !status.enabled
    } catch (error) {
      if (isUnauthorized(error)) {
        sign.setAuthState(true, false)
      } else {
        loginError.value = await errorMessage(error)
      }
      return false
    } finally {
      authReady.value = true
    }
  }

  async function login() {
    loginBusy.value = true
    loginError.value = ''
    const authorization = basicAuthorization(loginUsername.value, loginPassword.value)
    try {
      await api.post('api/auth/login', {
        headers: { Authorization: authorization },
      })
      sign.setSession(loginUsername.value, authorization)
      sign.setAuthState(true, true)
      return true
    } catch (error) {
      loginError.value = await errorMessage(error)
      return false
    } finally {
      loginBusy.value = false
    }
  }

  function logout() {
    sign.signOut()
  }

  return {
    authReady,
    loginBusy,
    loginError,
    loginUsername,
    loginPassword,
    isAuthenticated,
    authEnabled,
    checkAuth,
    login,
    logout,
  }
}
