import type { Instance } from '../api/types'
import { getVersionsLocal } from '../api'

export interface VersionState {
  /** 全局默认版本（版本管理页激活的那个） */
  active: string
  downloaded: string[]
}

export async function loadVersionState(): Promise<VersionState> {
  try {
    const local = await getVersionsLocal()
    return { active: local.active || '', downloaded: (local.versions || []).map((v) => v.version) }
  } catch {
    return { active: '', downloaded: [] }
  }
}

/** 实际生效版本：运行中以进程记录为准；未运行按「指定且已下载→指定，否则默认」推算 */
export function effectiveVersion(inst: Instance, st: VersionState): string {
  if (inst.runtime?.frpc_version) return inst.runtime.frpc_version
  if (inst.frpc_version && st.downloaded.includes(inst.frpc_version)) return inst.frpc_version
  return st.active
}

/** 版本列文案：跟随默认显示「默认 vX」；指定且生效显示「vY」；指定但暂用别的显示「vY · 暂用 vX」 */
export function frpcVersionLabel(inst: Instance, st: VersionState): string {
  const eff = effectiveVersion(inst, st)
  if (!inst.frpc_version) return eff ? `默认 v${eff}` : '默认'
  if (inst.frpc_version === eff) return `v${inst.frpc_version}`
  return `v${inst.frpc_version} · 暂用 v${eff || '默认'}`
}

export function frpcVersionFallback(inst: Instance, st: VersionState): boolean {
  return !!inst.frpc_version && inst.frpc_version !== effectiveVersion(inst, st)
}

export const FRPC_VERSION_FALLBACK_TIP =
  '指定的版本还没下载完：先用默认版本运行，后台下载完成后会自动重启切过去'
