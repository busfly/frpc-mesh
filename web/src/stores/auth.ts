import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiHealth, apiLogin, apiMe } from '../api'
import { TOKEN_KEY } from '../api/http'
import router from '../router'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem(TOKEN_KEY) || '')
  const username = ref<string>('')
  const version = ref<string>('')

  async function login(user: string, password: string) {
    const data = await apiLogin(user, password)
    token.value = data.token
    localStorage.setItem(TOKEN_KEY, data.token)
    username.value = user
  }

  async function fetchMe() {
    if (!token.value) return
    try {
      const me = await apiMe()
      username.value = me.username
    } catch {
      // 401 由拦截器统一处理
    }
  }

  async function fetchVersion() {
    try {
      const h = await apiHealth()
      version.value = h.version || ''
    } catch {
      version.value = ''
    }
  }

  function logout() {
    token.value = ''
    username.value = ''
    localStorage.removeItem(TOKEN_KEY)
    router.replace({ name: 'login' })
  }

  return { token, username, version, login, logout, fetchMe, fetchVersion }
})
