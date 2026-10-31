export interface Transport {
  protocol: string
  tcp_mux: boolean
  pool_count: number
  heartbeat: number
  heartbeat_timeout?: number
  tcp_mux_keepalive?: number
  dial_timeout: number
  dial_server_keepalive?: number
  tls_enable: boolean
  proxy_protocol_version?: string
  custom_stun_server?: string
}

export interface AdminApi {
  enabled: boolean
  port: number
  user: string
  password: string
}

export interface InstanceRuntime {
  status: string
  pid: number
  started_at: string
  uptime_seconds: number
  last_error: string
}

export interface Instance {
  id: number
  name: string
  server_addr: string
  server_port: number
  auth_method: 'token' | 'none'
  auth_token: string
  manage_port: number
  remark: string
  domain: string
  enabled: boolean
  transport?: Transport | null
  admin?: AdminApi | null
  created_at?: string
  updated_at?: string
  runtime?: InstanceRuntime | null
}

export interface Proxy {
  id: number
  instance_id: number
  kind: 'proxy' | 'visitor'
  name: string
  type: string
  enabled: boolean
  local_ip: string
  local_port: number
  remote_port: number
  custom_domains: string
  subdomain: string
  locations: string
  http_user: string
  http_password: string
  host_header_rewrite: string
  use_encryption: boolean
  use_compression: boolean
  bandwidth_limit?: string
  secret_key: string
  server_name: string
  bind_addr: string
  bind_port: number
  remark: string
  created_at?: string
  updated_at?: string
}

export interface ProxyPingResult {
  id: number
  name: string
  type: string
  latency_ms: number
}


export interface DashboardInstance {
  id: number
  name: string
  status: string
  enabled: boolean
  server_addr: string
  server_port: number
  proxies_total: number
  proxies_online: number
}

export interface Dashboard {
  instances_total: number
  instances_online: number
  proxies_total: number
  proxies_online: number
  visitors_total: number
  instances: DashboardInstance[]
}

export interface Settings {
  auto_start: boolean
  auto_update_check: boolean
  version: string
}

/** 新版本检测：frpc-mesh 自身相对 GitHub Releases 的状态 */
export interface UpdateAsset {
  name: string
  size_bytes: number
  url: string
}

export interface UpdateInfo {
  current: string
  latest: string
  has_update: boolean
  release_name: string
  release_url: string
  published_at: string
  notes: string
  assets: UpdateAsset[]
  /** 上次检查时间（RFC3339）；空表示从未检查过 */
  checked_at: string
  /** github | ghfast */
  source: string
  error?: string
  auto_check: boolean
}

/** 当前运行环境能否一键升级：容器内只提示，二进制可就地替换 */
export interface UpgradeCapability {
  can_upgrade: boolean
  mode: 'binary' | 'container' | 'unsupported'
  reason: string
  asset_name: string
  binary_path?: string
  has_checksum: boolean
}

/** 升级任务状态：idle | downloading | verifying | swapping | restarting | error */
export interface UpgradeJob {
  state: string
  message: string
  from: string
  to: string
}

export interface UpdateStatus {
  info: UpdateInfo
  upgrade: UpgradeCapability
  upgrade_job: UpgradeJob
  last_upgrade: string
}

/** GET /settings/net-hints：后端探测的本地地址快捷提示 */
export interface NetHints {
  /** 可解析时为 "host.docker.internal"，容器访问宿主机的首选地址 */
  host_docker_internal: string
  /** 容器内到达宿主机的网桥网关，非容器环境为空 */
  host_gateway: string
  /** 本机出口 IP */
  container_ip: string
  /** env | proc | none */
  source: string
}

export interface VersionInfo {
  version: string
  size_bytes?: number
  download_count?: number
  published_at?: string
  downloaded: boolean
  active: boolean
}

export interface VersionsResponse {
  active: string
  current: string
  versions: VersionInfo[] | null
  local: VersionInfo[] | null
  error: string
}

/** GET /frpc/versions/local：仅本地已下载版本，秒回不访问网络 */
export interface VersionsLocalResponse {
  active: string
  current: string
  versions: VersionInfo[] | null
}

export interface Health {
  status: string
  version: string
}

/** 备份/恢复文件结构：服务器 + 其下代理 */
export interface BackupData {
  app: string
  version: number
  exported_at: string
  instances: (Instance & { proxies: Proxy[] })[]
}
