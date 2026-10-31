import type { Instance } from '../api/types'

/** 状态 → Element Plus tag 类型 */
export function statusTagType(status?: string): 'success' | 'primary' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'online':
      return 'success'
    case 'running':
      return 'primary'
    case 'starting':
      return 'warning'
    case 'error':
      return 'danger'
    case 'stopping':
      return 'warning'
    case 'stopped':
    default:
      return 'info'
  }
}

/** 状态 → 圆点颜色（online 绿 / starting·stopping 橙 / error 红 / 其余灰），语义与 statusTagType 一致 */
export function statusDotColor(status?: string): string {
  switch (status) {
    case 'online':
    case 'running':
      return '#22c55e'
    case 'starting':
    case 'stopping':
      return '#f59e0b'
    case 'error':
      return '#ef4444'
    default:
      return '#c0c4cc'
  }
}

export function statusText(status?: string): string {
  const map: Record<string, string> = {
    online: '在线',
    running: '运行中',
    starting: '启动中',
    stopping: '停止中',
    stopped: '已停止',
    error: '错误'
  }
  return map[status || ''] || status || '未知'
}

/** 服务器（frpc 进程）是否处于运行状态（可停止/查看日志） */
export function isRunning(inst?: Pick<Instance, 'runtime'> | null): boolean {
  const s = inst?.runtime?.status
  return s === 'online' || s === 'running' || s === 'starting'
}

/** 代理类型 → tag 颜色 */
export function proxyTypeTag(type: string): 'success' | 'primary' | 'warning' | 'danger' | 'info' {
  switch (type) {
    case 'tcp':
      return 'primary'
    case 'udp':
      return 'success'
    case 'http':
    case 'https':
      return 'warning'
    case 'stcp':
    case 'sudp':
    case 'xtcp':
      return 'danger'
    default:
      return 'info'
  }
}

/** 代理类型 → 彩色 badge 背景色 */
export function proxyTypeColor(type: string): string {
  switch (type) {
    case 'tcp':
      return '#409eff'
    case 'udp':
      return '#8b5cf6'
    case 'http':
      return '#10b981'
    case 'https':
      return '#f59e0b'
    case 'stcp':
    case 'sudp':
      return '#ec4899'
    case 'xtcp':
      return '#06b6d4'
    default:
      return '#909399'
  }
}

/** 代理类型 → 中文显示名 */
export function proxyTypeLabel(type: string): string {
  const map: Record<string, string> = {
    tcp: 'TCP',
    udp: 'UDP',
    http: 'HTTP',
    https: 'HTTPS',
    stcp: 'STCP',
    sudp: 'SUDP',
    xtcp: 'P2P'
  }
  return map[type] || type.toUpperCase()
}

export function formatUptime(seconds?: number): string {
  if (!seconds || seconds < 0) return '-'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (d > 0) return `${d}天 ${h}小时 ${m}分`
  if (h > 0) return `${h}小时 ${m}分`
  if (m > 0) return `${m}分 ${s}秒`
  return `${s}秒`
}

export function formatTime(iso?: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
