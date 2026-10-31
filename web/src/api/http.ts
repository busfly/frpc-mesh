import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

export const TOKEN_KEY = 'frpc_mesh_token'

const http = axios.create({
  baseURL: '/api',
  timeout: 15000
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

let redirecting = false

http.interceptors.response.use(
  (resp) => resp,
  (error) => {
    const status = error?.response?.status
    const msg = error?.response?.data?.error
    if (status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      const current = router.currentRoute.value
      if (current.name !== 'login' && !redirecting) {
        redirecting = true
        router.replace({ name: 'login' }).finally(() => {
          redirecting = false
        })
      }
    } else if (!error?.config?.silent) {
      // 网络错误 / 5xx：非静默请求统一提示
      if (msg) {
        ElMessage.error(msg)
      } else if (error?.code === 'ECONNABORTED') {
        ElMessage.error('请求超时，请稍后重试')
      } else if (!error?.response) {
        ElMessage.error('无法连接后端服务')
      } else {
        ElMessage.error(`请求失败 (${status})`)
      }
    }
    return Promise.reject(error)
  }
)

export default http
