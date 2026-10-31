import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { checkUpdate, getUpdateStatus, getUpgradeStatus, startUpgrade } from '../api'
import type { UpdateStatus, UpgradeJob } from '../api/types'

const RUNNING = ['downloading', 'verifying', 'swapping', 'restarting']

export const useUpdateStore = defineStore('update', () => {
  // 首屏只读本地已知结果，联网检查由按钮触发，状态可预测
  const status = ref<UpdateStatus | null>(null)
  const loading = ref(false)
  const checking = ref(false)
  const upgrading = ref(false)
  const job = ref<UpgradeJob | null>(null)

  const info = computed(() => status.value?.info ?? null)
  const capability = computed(() => status.value?.upgrade ?? null)
  const hasUpdate = computed(() => !!status.value?.info.has_update)
  const canUpgrade = computed(() => !!status.value?.upgrade.can_upgrade)
  const running = computed(() => !!job.value && RUNNING.includes(job.value.state))

  function apply(res: UpdateStatus) {
    status.value = res
    job.value = res.upgrade_job
  }

  async function load() {
    loading.value = true
    try {
      apply(await getUpdateStatus())
    } catch {
      /* 拦截器已提示 */
    } finally {
      loading.value = false
    }
  }

  async function check(source: 'github' | 'ghfast') {
    checking.value = true
    try {
      apply(await checkUpdate(source))
    } catch {
      /* 拦截器已提示 */
    } finally {
      checking.value = false
    }
  }

  let cancelled = false

  function stopPolling() {
    cancelled = true
  }

  // 升级过程轮询任务状态；restarting 表示进程即将被替换，之后改探健康接口
  async function poll() {
    cancelled = false
    while (!cancelled) {
      try {
        const st = await getUpgradeStatus()
        job.value = st
        if (!RUNNING.includes(st.state)) return
      } catch {
        // 服务重启中会短暂连不上，交给 waitForBackUp
        if (job.value?.state === 'restarting') return
      }
      await new Promise((r) => setTimeout(r, 1200))
    }
  }

  async function waitForBackUp(timeoutMs = 90000) {
    const start = Date.now()
    while (!cancelled) {
      const ac = new AbortController()
      const timer = setTimeout(() => ac.abort(), 3000)
      try {
        const res = await fetch('/api/health', { signal: ac.signal })
        if (res.ok) return true
      } catch {
        /* 仍在重启 */
      } finally {
        clearTimeout(timer)
      }
      if (Date.now() - start > timeoutMs) return false
      await new Promise((r) => setTimeout(r, 1500))
    }
    return false
  }

  async function upgrade(version: string) {
    upgrading.value = true
    cancelled = false
    try {
      job.value = await startUpgrade(version)
      upgrading.value = false
      await poll()
      if (job.value?.state === 'restarting') {
        const back = await waitForBackUp()
        if (back) await load()
      }
      return job.value
    } catch (e: unknown) {
      upgrading.value = false
      throw e
    }
  }

  return {
    status,
    info,
    capability,
    job,
    hasUpdate,
    canUpgrade,
    running,
    loading,
    checking,
    upgrading,
    load,
    check,
    upgrade,
    stopPolling
  }
})
