import { onBeforeUnmount, watch, type Ref } from 'vue'

const THRESHOLD_PX = 80
const THRESHOLD_SPEED = 0.5

/**
 * 把手下滑关闭：位移 >80px 或速度 >0.5px/ms 才关闭，否则回弹。
 * 只跟手把手区（不绑整个弹层），避免与内层列表滚动抢手势。
 * 用 watch 而不是 onMounted：el-drawer 的内容是懒渲染的，挂载那一刻把手还不存在。
 */
export function useSwipeToClose(handle: Ref<HTMLElement | null>, onClose: () => void) {
  let panel: HTMLElement | null = null
  let startY = 0
  let startT = 0
  let dy = 0
  let tracking = false

  const onDown = (e: TouchEvent) => {
    if (e.touches.length !== 1) return
    startY = e.touches[0].clientY
    startT = Date.now()
    dy = 0
    tracking = true
  }
  const onMove = (e: TouchEvent) => {
    if (!tracking || e.touches.length !== 1) return
    dy = Math.max(0, e.touches[0].clientY - startY)
    // 拖的是整块面板，只动把手会出现「把手跑了、内容没动」的错觉
    if (panel) panel.style.transform = `translateY(${dy}px)`
    if (dy > 0 && e.cancelable) e.preventDefault()
  }
  const onEnd = () => {
    if (!tracking) return
    tracking = false
    const speed = dy / Math.max(1, Date.now() - startT)
    if (panel) panel.style.transform = ''
    if (dy > THRESHOLD_PX || speed > THRESHOLD_SPEED) onClose()
  }

  let bound: HTMLElement | null = null
  const unbind = () => {
    if (!bound) return
    bound.removeEventListener('touchstart', onDown)
    bound.removeEventListener('touchmove', onMove)
    bound.removeEventListener('touchend', onEnd)
    bound.removeEventListener('touchcancel', onEnd)
    bound = null
    panel = null
  }

  watch(
    handle,
    (el) => {
      unbind()
      if (!el) return
      bound = el
      panel = el.closest('.el-drawer') ?? el
      el.addEventListener('touchstart', onDown, { passive: true })
      // touchmove 必须非 passive：不 preventDefault 时页面会跟着滚，手势被判给滚动容器
      el.addEventListener('touchmove', onMove, { passive: false })
      el.addEventListener('touchend', onEnd)
      el.addEventListener('touchcancel', onEnd)
    },
    { immediate: true }
  )

  onBeforeUnmount(unbind)
}
