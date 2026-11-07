import { computed, ref, type ComputedRef, type Ref } from 'vue'

const MOBILE = '(max-width: 767px)'
const NARROW_DESKTOP = '(min-width: 768px) and (max-width: 1023px)'

// 同一个查询全站只挂一个监听：模块级缓存，组件卸载不回收（生命周期与页面一致）
const cache = new Map<string, Ref<boolean>>()

function stateOf(query: string): Ref<boolean> {
  let s = cache.get(query)
  if (s) return s
  s = ref(false)
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const mq = window.matchMedia(query)
    s.value = mq.matches
    mq.addEventListener('change', (e) => {
      s!.value = e.matches
    })
  }
  cache.set(query, s)
  return s
}

/** 移动形态：只看视口宽度，不看 UA、不看 pointer —— 窄窗口的桌面浏览器应保持桌面形态 */
export function useIsMobile(): ComputedRef<boolean> {
  return computed(() => stateOf(MOBILE).value)
}

/** 768–1023px：仍是桌面形态，但侧栏默认折叠 */
export function useIsNarrowDesktop(): ComputedRef<boolean> {
  return computed(() => stateOf(NARROW_DESKTOP).value)
}
