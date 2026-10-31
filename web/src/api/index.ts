import http from './http'
import type {
  BackupData,
  Dashboard,
  Health,
  Instance,
  NetHints,
  Proxy,
  ProxyPingResult,
  Settings,
  VersionsLocalResponse,
  VersionsResponse
} from './types'

// 认证
export const apiLogin = (username: string, password: string) =>
  http.post<{ token: string }>('/auth/login', { username, password }).then((r) => r.data)

export const apiMe = () => http.get<{ username: string }>('/auth/me').then((r) => r.data)

export const apiChangePassword = (old_password: string, new_password: string) =>
  http.put<{ ok: boolean }>('/auth/password', { old_password, new_password }).then((r) => r.data)

// 健康检查 / 版本
export const apiHealth = () =>
  http.get<Health>('/health', { silent: true } as any).then((r) => r.data)

// 仪表盘
export const apiDashboard = (silent = false) =>
  http
    .get<Dashboard>('/dashboard', { silent } as any)
    .then((r) => r.data)

// FRPS 服务器（实例）
export const apiListInstances = (silent = false) =>
  http.get<Instance[]>('/instances', { silent } as any).then((r) => r.data)

export const apiGetInstance = (id: number, silent = false) =>
  http.get<Instance>(`/instances/${id}`, { silent } as any).then((r) => r.data)

export const apiCreateInstance = (data: Partial<Instance>) =>
  http.post<Instance>('/instances', data).then((r) => r.data)

export const apiUpdateInstance = (id: number, data: Partial<Instance>) =>
  http.put<Instance>(`/instances/${id}`, data).then((r) => r.data)

export const apiDeleteInstance = (id: number) =>
  http.delete<{ ok: boolean }>(`/instances/${id}`).then((r) => r.data)

export const apiStartInstance = (id: number) =>
  http.post<{ ok: boolean }>(`/instances/${id}/start`).then((r) => r.data)

export const apiStopInstance = (id: number) =>
  http.post<{ ok: boolean }>(`/instances/${id}/stop`).then((r) => r.data)

export const apiRestartInstance = (id: number) =>
  http.post<{ ok: boolean }>(`/instances/${id}/restart`).then((r) => r.data)

export const apiInstanceLogs = (id: number, tail = 500) =>
  http
    .get<{ lines: string[] }>(`/instances/${id}/logs`, { params: { tail }, silent: true } as any)
    .then((r) => r.data)

export const apiClearInstanceLogs = (id: number) =>
  http.post<{ ok: boolean }>(`/instances/${id}/logs/clear`).then((r) => r.data)

export const apiPingProxies = (id: number) =>
  http.post<ProxyPingResult[]>(`/instances/${id}/proxies/ping`).then((r) => r.data)

export const apiInstanceConfig = (id: number) =>
  http.get<{ toml: string }>(`/instances/${id}/config`).then((r) => r.data)

// 代理 / 访问器
export const apiListProxies = (instanceId: number) =>
  http.get<Proxy[]>(`/instances/${instanceId}/proxies`).then((r) => r.data)

// 在线（隧道建立且本地目标可连通）的代理/访问器 ID 列表
export const apiProxiesOnline = (instanceId: number) =>
  http.get<{ online_ids: number[] }>(`/instances/${instanceId}/proxies/online`).then((r) => r.data)

export const apiCreateProxy = (instanceId: number, data: Partial<Proxy>) =>
  http.post<Proxy>(`/instances/${instanceId}/proxies`, data).then((r) => r.data)

export const apiUpdateProxy = (pid: number, data: Partial<Proxy>) =>
  http.put<Proxy>(`/proxies/${pid}`, data).then((r) => r.data)

export const apiSetProxyEnabled = (pid: number, enabled: boolean) =>
  http.put<{ ok: boolean }>(`/proxies/${pid}/enabled`, { enabled }).then((r) => r.data)

export const apiDeleteProxy = (pid: number) =>
  http.delete<{ ok: boolean }>(`/proxies/${pid}`).then((r) => r.data)

// 设置
export const apiGetSettings = () => http.get<Settings>('/settings').then((r) => r.data)

export const apiSaveSettings = (data: { auto_start: boolean }) =>
  http.put<Settings>('/settings', data).then((r) => r.data)

// 本地地址快捷提示（宿主机网关 / 本机 IP）
export const getNetHints = () =>
  http
    .get<NetHints>('/settings/net-hints', { silent: true } as any)
    .then((r) => r.data)

// 备份与恢复
export const apiExportBackup = () => http.get<BackupData>('/settings/backup').then((r) => r.data)

export const apiRestoreBackup = (data: BackupData) =>
  http.post<{ instances: number; proxies: number }>('/settings/backup/restore', data).then((r) => r.data)

// frpc 版本管理
// 仅本地已下载版本，秒回不访问网络，用于版本页首屏渲染
export const getVersionsLocal = () =>
  http
    .get<VersionsLocalResponse>('/frpc/versions/local', { silent: true } as any)
    .then((r) => r.data)

// 后端拉取 GitHub 失败时约需 30s 才返回 error 字段，超时需放宽以拿到该字段
export const getVersions = (source: 'github' | 'ghfast') =>
  http
    .get<VersionsResponse>(
      '/frpc/versions',
      { params: { source }, silent: true, timeout: 60000 } as any
    )
    .then((r) => r.data)

// 下载为异步后台任务，POST 立即返回，需轮询 download/status 直到 done/error
export const downloadVersion = (version: string, source: 'github' | 'ghfast') =>
  http
    .post<{ ok: boolean }>('/frpc/versions/download', { version, source })
    .then((r) => r.data)

export const getDownloadStatus = (version: string) =>
  http
    .get<{ status: 'downloading' | 'done' | 'error' | 'none'; message: string }>(
      '/frpc/versions/download/status',
      { params: { version }, silent: true } as any
    )
    .then((r) => r.data)

export const activateVersion = (version: string) =>
  http.post<{ ok: boolean }>('/frpc/versions/activate', { version }).then((r) => r.data)

export const deleteVersion = (version: string) =>
  http.delete<{ ok: boolean }>(`/frpc/versions/${version}`).then((r) => r.data)
