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
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
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
