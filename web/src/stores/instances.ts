import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  apiDashboard,
  apiListInstances,
  getVersionsLocal,
  apiRestartInstance,
  apiStartInstance,
  apiStopInstance
} from '../api'
import type { Dashboard, Instance } from '../api/types'
import type { VersionState } from '../utils/frpcVersion'

const POLL_INTERVAL = 10000

export const useInstancesStore = defineStore('instances', () => {
  const instances = ref<Instance[]>([])
  const dashboard = ref<Dashboard>({
    instances_total: 0,
    instances_online: 0,
    proxies_total: 0,
    proxies_online: 0,
    visitors_total: 0,
    instances: []
  })
  const loading = ref(false)
  // 本地 frpc 版本快照（默认版本 + 已下载列表），随实例轮询刷新，供各处版本列共用
  const frpcVersions = ref<VersionState>({ active: '', downloaded: [] })
  // 正在执行操作的服务器 id 集合，用于按钮 loading 态
  const acting = ref<Record<number, boolean>>({})
  let timer: ReturnType<typeof setInterval> | null = null
  let started = false

  const onlineCount = computed(
    () => instances.value.filter((i) => i.runtime?.status === 'online').length
  )

  /** 静默刷新（轮询用），后端不可用时不打扰用户 */
  async function refreshSilently() {
    try {
      const [list, dash] = await Promise.all([apiListInstances(true), apiDashboard(true)])
      instances.value = list || []
      dashboard.value = dash
    } catch {
      // 静默处理
    }
    loadFrpcVersions()
  }

  /** 版本列要区分「指定已生效 / 指定未下载」，故与实例列表同节奏刷新 */
  async function loadFrpcVersions() {
    try {
      const local = await getVersionsLocal()
      frpcVersions.value = {
        active: local.active || '',
        downloaded: (local.versions || []).map((v) => v.version)
      }
    } catch {
      // 保留上次结果，不打扰用户
    }
  }

  /** 带 loading 的刷新（页面首次加载用），失败仍静默 */
  async function refresh() {
    loading.value = true
    try {
      await refreshSilently()
    } finally {
      loading.value = false
    }
  }

  function startPolling() {
    if (started) return
    started = true
    refresh()
    timer = setInterval(() => {
      if (document.visibilityState === 'visible') {
        refreshSilently()
      }
    }, POLL_INTERVAL)
  }

  function stopPolling() {
    started = false
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  async function withActing(id: number, fn: () => Promise<unknown>) {
    acting.value[id] = true
    try {
      await fn()
      await refreshSilently()
    } finally {
      acting.value[id] = false
    }
  }

  function start(id: number) {
    return withActing(id, () => apiStartInstance(id))
  }
  function stop(id: number) {
    return withActing(id, () => apiStopInstance(id))
  }
  function restart(id: number) {
    return withActing(id, () => apiRestartInstance(id))
  }

  function getInstance(id: number): Instance | undefined {
    return instances.value.find((i) => i.id === id)
  }

  return {
    instances,
    frpcVersions,
    loadFrpcVersions,
    dashboard,
    loading,
    acting,
    onlineCount,
    refresh,
    refreshSilently,
    startPolling,
    stopPolling,
    start,
    stop,
    restart,
    getInstance
  }
})
