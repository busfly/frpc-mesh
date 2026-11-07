export type CopyResult = 'copied' | 'selected' | 'failed'

/**
 * 复制命令。面板常走 http://内网IP，非安全上下文下 navigator.clipboard 会被浏览器拒绝，
 * 所以三级兜底：clipboard → execCommand → 选中该行文本让用户自己按 Ctrl/Cmd+C。
 */
export async function copyText(text: string, fallbackEl?: HTMLElement | null): Promise<CopyResult> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return 'copied'
    }
  } catch {
    /* 继续走兜底 */
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    let ok = false
    try {
      ok = document.execCommand('copy')
    } finally {
      ta.remove()
    }
    if (ok) return 'copied'
  } catch {
    /* 继续走选中 */
  }
  if (fallbackEl && selectRange(fallbackEl)) return 'selected'
  return 'failed'
}

function selectRange(el: HTMLElement): boolean {
  const range = document.createRange()
  range.selectNodeContents(el)
  const sel = window.getSelection()
  if (!sel) return false
  sel.removeAllRanges()
  sel.addRange(range)
  return true
}

/**
 * 三档结果对应的提示文案。触屏上没有 Ctrl/Cmd+C，措辞按是否有 hover 能力分开，
 * 免得给手机用户弹出「按 Ctrl/Cmd+C 复制」这种做不到的指令。
 */
export function copyHint(r: CopyResult): { type: 'success' | 'info' | 'error'; text: string } {
  const touch =
    typeof window !== 'undefined' && !!window.matchMedia && !window.matchMedia('(hover: hover)').matches
  if (r === 'copied') return { type: 'success', text: '已复制' }
  if (r === 'selected')
    return { type: 'info', text: touch ? '已选中，长按可复制' : '已选中，按 Ctrl/Cmd+C 复制' }
  return { type: 'error', text: '复制失败，请手动选中内容复制' }
}
