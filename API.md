# frpc-mesh API 契约 (v1)

后端: Go (net/http)，SQLite 存储，JWT 认证。所有接口前缀 `/api`，请求/响应均为 JSON。
除 `/api/auth/login` 与 `/api/health` 外，均需请求头 `Authorization: Bearer <token>`，未认证返回 401 `{"error":"unauthorized"}`。
错误统一格式: `{"error":"<message>"}`，HTTP 状态码 4xx/5xx。

## 认证
- `POST /api/auth/login` `{"username":"admin","password":"..."}` → `{"token":"<jwt>"}`；失败 401。默认账号 admin / admin123（首次登录后请在设置中修改）。
  登录失败锁定（按 IP+用户名连续失败计数，成功即清零）：5 次锁 3 分钟、10 次锁 10 分钟、15 次及以后锁 1 小时；锁定期内返回 429 `{"error":"失败次数过多，已锁定，请 N 秒后再试"}`。内网来源（环回/RFC1918/链路本地）的 429 消息会附带本机 CLI 重置密码的指引，外网来源不带。
- `GET /api/auth/me` → `{"username":"admin"}`
- `PUT /api/auth/password` `{"old_password":"...","new_password":"..."}` → `{"ok":true}`；需登录态且校验旧密码，旧密码错误返回 400。
- 忘记密码的重置**只有本机 CLI**（`--reset-password` 标志），无任何对外重置接口。

## 仪表盘
- `GET /api/dashboard` →
```json
{
  "instances_total": 3,
  "instances_online": 2,
  "proxies_total": 10,
  "proxies_online": 4,        // frps 侧隧道状态为 running 的代理数（当前真正通的连接）
  "visitors_total": 1,
  "instances": [ {"id":1,"name":"阿里云","status":"online","enabled":true,"server_addr":"1.2.3.4","server_port":7000,"proxies_total":5,"proxies_online":4} ]
}
```

## 实例（每个实例 = 一个远程 frps = 一个独立 frpc 进程）
- `GET /api/instances` → `[instance]`
- `POST /api/instances` `{instance 不含 id}` → `instance` (201)
- `GET /api/instances/{id}` → `instance`
- `PUT /api/instances/{id}` `{instance}` → `instance`（若进程在跑会自动重启生效）
- `DELETE /api/instances/{id}` → `{"ok":true}`（自动先停止进程）
- `POST /api/instances/{id}/start` → `{"ok":true}`；frpc 不可用等错误返回 400 `{"error":"..."}`
- `POST /api/instances/{id}/stop` → `{"ok":true}`
- `POST /api/instances/{id}/restart` → `{"ok":true}`
- `GET /api/instances/{id}/logs?tail=500` → `{"lines":["...","..."]}`（最近 N 行 frpc 输出）
- `POST /api/instances/{id}/logs/clear` → `{"ok":true}`（清空内存日志缓冲）
- `POST /api/instances/{id}/proxies/ping` → `[{"id":1,"name":"ssh","type":"tcp","latency_ms":23}]`
  仅 POST（GET 返回 405）；从 frpc-mesh 所在主机 TCP 连接 frps 的远程端口测隧道延迟。`latency_ms`：`-1` 不通/不可测（非 tcp 类型、未启用、无远程端口或连接失败）、`0` 隧道通但服务静默无回包、`>0` 往返毫秒
- `GET /api/instances/{id}/config` → `{"toml":"serverAddr = ...\n..."}`（生成的 frpc TOML 预览）
- `GET /api/instances/{id}/proxies/online` → `{"online_ids":[1,2,3]}`（在线隧道 ID；判定 = frpc 日志确认隧道建立 + 本地目标数据层可达，visitor/udp 只看隧道）

### instance 对象
```json
{
  "id": 1,
  "name": "阿里云 frps",
  "server_addr": "1.2.3.4",
  "server_port": 7000,
  "auth_method": "token",          // "token" | "none"
  "auth_token": "xxxx",
  "manage_port": 7500,              // frps 管理面板端口，选填；前端「管理」按钮据此组装 http://server_addr:manage_port
  "remark": "备注说明",              // 选填，服务器备注
  "domain": "frps.example.com",     // 选填，服务器域名；代理列表远程链接优先用域名，缺省回退 server_addr
  "enabled": true,                  // 应用启动时自动拉起
  "transport": {                    // 可整体缺省
    "protocol": "tcp",              // tcp|kcp|quic|wss|websocket
    "tcp_mux": true,
    "pool_count": 1,
    "heartbeat": 30,                // 心跳间隔秒
    "heartbeat_timeout": 90,        // 心跳超时秒
    "tcp_mux_keepalive": 30,        // TCP MUX 保活间隔秒
    "dial_timeout": 10,
    "tls_enable": false,
    "proxy_protocol_version": "",   // 启用线路协议: ""|v1|v2
    "custom_stun_server": ""        // STUN 服务器（P2P/XTCP 用），空为 frp 默认
  },
  "admin": {                        // frpc 自带 admin API，用于查询代理状态
    "enabled": true,
    "port": 17400,                  // 每实例必须唯一
    "user": "admin",
    "password": "admin"
  },
  "created_at": "2026-10-25T10:00:00Z",
  "updated_at": "2026-10-25T10:00:00Z",
  "runtime": {                      // 只读，仅 GET 返回
    "status": "online",             // stopped | starting | running | online | error | stopping
    "pid": 12345,
    "started_at": "2026-10-25T10:00:05Z",
    "uptime_seconds": 120,
    "last_error": ""
  }
}
```

## 代理 / 访问器（挂在实例下）
- `GET /api/instances/{id}/proxies` → `[proxy]`（含 kind=proxy 与 kind=visitor）
- `POST /api/instances/{id}/proxies` `{proxy}` → `proxy` (201)（若实例进程在跑会自动重启生效）
- `PUT /api/proxies/{pid}` `{proxy}` → `proxy`
- `DELETE /api/proxies/{pid}` → `{"ok":true}`

### proxy 对象
```json
{
  "id": 1,
  "instance_id": 1,
  "kind": "proxy",                  // "proxy" → [[proxies]]；"visitor" → [[visitors]]（stcp/xtcp/sudp 访问端）
  "name": "nas-ssh",
  "type": "tcp",                    // tcp|udp|http|https|stcp|sudp|xtcp
  "local_ip": "127.0.0.1",
  "local_port": 22,
  "remote_port": 6000,              // tcp/udp 用；0 表示随机
  "custom_domains": "a.com,b.com",  // http/https 用，逗号分隔
  "subdomain": "",
  "locations": "",
  "http_user": "", "http_password": "",
  "host_header_rewrite": "",
  "use_encryption": false,
  "use_compression": false,
  "bandwidth_limit": "",            // 带宽限制，如 "1MB"/"500KB"，空为不限制
  "secret_key": "",                 // stcp/xtcp/sudp
  "server_name": "",                // visitor: 要访问的对端 proxy 名
  "bind_addr": "127.0.0.1",         // visitor 专用
  "bind_port": 9000,                // visitor 专用
  "remark": "备注说明",              // 选填，仅保存与编辑可见，不在列表显示
  "created_at": "...", "updated_at": "..."
}
```
visitor 的 `type` 也取 stcp/xtcp/sudp；visitor 的 `name` 生成 `[[visitors]]` 的 name。

## 设置
- `GET /api/settings` → `{"auto_start":true,"auto_update_check":true,"version":"1.0.0"}`
  - `auto_start`: 服务启动时自动拉起所有 enabled 服务器连接
  - `auto_update_check`: 后台每 6 小时联网检查 frpc-mesh 自身新版本（仅提醒，不自动下载/升级）
- `PUT /api/settings` `{"auto_start":true,"auto_update_check":false}` → 同 GET；**字段缺省时保留原值**（外层 `*bool` 承接，不会把未提交的开关重置为默认）
- frpc 二进制不再支持自定义路径：生效顺序为「frpc 版本管理中已激活的版本 → 系统 PATH/常见位置探测」；离线场景可将二进制放入数据目录 `versions/frpc-<版本号>` 即被识别为已下载版本

- `GET /api/settings/backup` → 备份文件：`{"app":"frpc-mesh","version":1,"exported_at":"...","instances":[{...instance 全字段(不含 runtime)...,"proxies":[...proxy 全字段...]}]}`
- `POST /api/settings/backup/restore`（body 为上述备份 JSON）→ 先停止全部 frpc 进程，清空现有服务器/代理后按备份重建（ID 重新分配），再自动拉起 enabled 的服务器；返回 `{"instances":n,"proxies":m}`；body 非备份格式返回 400
- `GET /api/settings/net-hints` → `{"host_docker_internal":"host.docker.internal","host_gateway":"172.17.0.1","container_ip":"172.17.0.5","source":"proc"}`
  host_docker_internal：容器内可解析该域名时返回，为访问宿主机的首选地址（compose 已配 extra_hosts host-gateway，Linux 亦可解析）；host_gateway：默认路由网关（备选）；source: env(环境变量 FRPC_MESH_HOST_GATEWAY 覆盖) | proc(/proc/net/route) | none(非容器环境探测不到，host_gateway 为空)

## frpc 版本管理
- `GET /api/frpc/versions/local` → `{"active","current","versions":[本地已下载版本]}`（**秒回，不访问网络**；页面首屏用）
- `GET /api/frpc/versions?source=github|ghfast` →
```json
{
  "active": "0.61.1",            // 已激活（生效中）的版本，空串表示用自定义路径/PATH
  "current": "0.61.1",           // 当前解析到的二进制实际版本
  "versions": [ ... ],           // 远程+本地合并、按版本号降序；本地独有版本也在此列表（downloaded=true，元数据为 0）
  "local": [ ... ],              // 仅本地版本（同 /local 接口）
  "error": ""                    // 拉取 GitHub 失败时的错误信息（versions 仍含本地版本）
}
```
远程结果服务端缓存 10 分钟。
- `POST /api/frpc/versions/download` `{"version":"0.71.1","source":"ghfast"}` → `{"ok":true}`（**异步**启动后台下载任务，立即返回；重复提交返回 400「正在下载中」）
- `GET /api/frpc/versions/download/status?version=0.71.1` → `{"status":"downloading|done|error|none","message":"错误信息"}`（前端轮询此接口直到 done/error）
- `POST /api/frpc/versions/activate` `{"version":"0.71.1"}` → `{"ok":true}`（设为生效版本并**重启所有运行中实例**以在线生效）
- `DELETE /api/frpc/versions/{version}` → `{"ok":true}`（删除本地二进制；生效中的版本拒绝删除）

## frpc-mesh 自更新
检测本工具自身在 GitHub Releases 上的新版本，并在**非容器**环境支持一键升级。

- `GET /api/update/status` → 见下方响应体（**不访问网络**，读上次检查的结果，秒回；页面首屏用）
- `POST /api/update/check?source=github|ghfast` → 同响应体（**强制联网检查**：直连 `api.github.com` 失败后自动改走 `ghfast.top` 镜像，结果落盘，重启后仍在）
- `POST /api/update/upgrade` `{"version":"1.0.1"}` → `{"state":"downloading",...}`（**异步**启动后台升级任务，立即返回；环境不允许/版本不是最新/已有任务在跑返回 400，`message` 为中文原因）
- `GET /api/update/upgrade/status` → `{"state","message","from","to"}`（前端轮询直到 `error`/`restarting`；服务重启后该接口返回 `idle`）

```json
{
  "info": {
    "current": "1.0.0",           // 当前运行版本（构建期 -X main.Version 注入，不带 v）
    "latest": "1.0.1",            // 最新已发布版本，空串表示还没检查过
    "has_update": true,           // 仅当两者都是 x.y.z 且 latest 更大
    "release_name": "frpc-mesh v1.0.1",
    "release_url": "https://github.com/busfly/frpc-mesh/releases/tag/v1.0.1",
    "published_at": "2026-11-01T10:00:00Z",
    "notes": "…",                 // Release 正文，截断到 4KB
    "assets": [{"name":"frpc-mesh-v1.0.1-linux-arm64.tar.gz","size_bytes":14048894,"url":"https://github.com/…"}],
    "checked_at": "2026-11-01T18:00:00+08:00",  // 空串表示从未检查
    "source": "github",           // github | ghfast
    "error": "",                  // 本次检查失败原因（成功时为空）
    "auto_check": true            // 定时自动检查开关（读自设置）
  },
  "upgrade": {
    "can_upgrade": true,
    "mode": "binary",             // binary 可就地升级 | container 容器内不自动替换 | unsupported 需手动
    "reason": "可直接升级：替换二进制后进程自重启，隧道会中断数秒",
    "asset_name": "frpc-mesh-v1.0.1-darwin-arm64.tar.gz",   // 按当前 GOOS/GOARCH 匹配
    "binary_path": "/path/to/frpc-mesh",
    "has_checksum": true          // Release 是否带 sha256sums.txt
  },
  "upgrade_job": {"state":"idle","message":"","from":"","to":""},
  "last_upgrade": "1.0.0→1.0.1"   // 本进程最近一次完成的自升级，空串表示没有
}
```
`can_upgrade` 判定顺序：无 latest / 容器内 / 当前平台无安装包 / 缺 `sha256sums.txt` / 二进制不可写 → 一律拒绝并给出 `reason`（宁可不升，也不替换成未经校验的文件）。
升级流程：下载 → sha256 比对 → 解包出 `frpc-mesh` 并跑 `--version` 自检 → 旧二进制改名保留为 `.bak` → 同目录原子替换 → 优雅停掉全部 frpc 子进程 → `exec` 自身重启（环境带 `FRPC_MESH_RESUME=1`、`FRPC_MESH_UPGRADED_FROM=<旧版本>`，重启后无论 `auto_start` 与否都会拉起已启用服务器）。任一步失败都不动原二进制（替换失败会回滚），任务状态转 `error`。

## 健康检查
- `GET /api/health` → `{"status":"ok","version":"1.0.0"}`

## 静态资源
非 `/api` 路径返回内嵌 SPA（Vue3 history 路由，需 fallback 到 index.html）。
