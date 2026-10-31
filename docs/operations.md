# 维护手册

备份恢复、升级、安全边界与排错速查。首次部署见 [部署指南](deployment.md)，功能与架构见根目录 [中文说明](../README-CN.md)。

## 1. 数据目录结构（`/data` 或 `./data`）

| 路径 | 内容 | 备份必要性 |
|---|---|---|
| `frpc-mesh.db`（+ `-wal`/`-shm`） | 服务器、代理、设置、账号哈希、JWT 密钥 | 必须 |
| `versions/frpc-<版本>` | 页面下载的 frpc 二进制（单文件） | 可选（可重新下载） |
| `configs/instance-<id>.toml` | 运行时生成的 frpc 配置 | 不需要（自动重建） |

运行日志在内存中（页面「运行日志」可见），不落盘。

## 2. 账号与忘记密码

- 默认账号 `admin / admin123`，首次登录后立即在「设置-修改密码」修改
- **忘记密码**（宿主机/本机执行，立即生效、无需重启、只改密码不动数据）：
  - Docker：`docker exec -it frpc-mesh /app/frpc-mesh --reset-password --data /data`
  - 普通运行：`./frpc-mesh --reset-password --data ./data`
  - 密码统一重置为 `admin123`，登录后自行修改
- 该能力**只有本机 CLI，没有对应 HTTP 接口**；能执行它的前提是已有主机登录权限

## 3. 备份与恢复

### 3.1 配置级备份（日常推荐，Web 内操作）

- 「设置-备份与恢复-导出备份」→ 下载 JSON（含全部服务器+代理，含 token/备注/域名；**不含账号密码**）
- 导入：选文件 → 确认覆盖 → 在线生效（先停全部 frpc，清空重建，再拉起已启用项）
- 跨版本兼容性好，适合迁移到不同部署形态

### 3.2 数据目录级备份（整机迁移/归档）

bind 挂载下直接拷贝宿主机数据目录（建议先停容器保证 WAL 一致，备完再起）：

```bash
docker compose stop
tar czf frpc-mesh-data-$(date +%Y%m%d).tgz -C <数据目录父目录> data
docker compose start
```

恢复：停容器 → 清空数据目录 → `tar xzf 备份.tgz -C 父目录` → 起容器。

### 3.3 重装/迁移流程

1. 备份：按 3.2 打数据目录 tar（或 3.1 导出 JSON）
2. 重装：新机器部署源码/compose（数据目录路径改成本机的）→ `docker compose up -d --build`
3. 恢复：解 tar 到数据目录（或登录 Web 导入 JSON）
4. 验证：登录 Web → FRPS 服务器，确认服务器与代理数据完整，逐个"启动"并观察状态变为在线

> 本部署历史：早期用命名卷 `frpc-mesh_frpc-mesh-data`，2026-10-31 已迁到宿主机目录（当前 `/Volumes/MAC_U_202411/docker/frpc-mesh/data`）并逐表验证一致。旧卷作为回退兜底保留，确认稳定后可 `docker volume rm frpc-mesh_frpc-mesh-data` 删除。

## 4. 升级

### 4.1 frpc-mesh 本体（Web「设置 → 检查新版本」）
- 数据源选 GitHub 官方或 ghfast 镜像，直连失败会自动再试镜像；结果写入数据库，重启后仍显示上次检查时间与版本
- 「定时检查 frpc-mesh 新版本」开关只负责每 6 小时联网查一次并提醒，**不会自动下载或替换**；页面顶栏出现「发现新版本 vX.Y.Z」即为有更新
- **二进制运行（NAS/软路由 `./start.sh`）**：环境标签为「二进制运行」时可点「升级到 vX.Y.Z」——下载 → 按 Release 的 `sha256sums.txt` 校验 → 新二进制 `--version` 自检 → 替换 → 自重启。期间**所有隧道中断数秒**后自动恢复（重启后即使 `auto_start` 关闭也会拉起已启用的服务器）。旧二进制保留为 `frpc-mesh.bak`，出问题可停进程后 `mv frpc-mesh.bak frpc-mesh` 回滚
- **容器运行**：环境标签为「容器运行」，页面不提供就地升级（容器内替换的二进制会在镜像重建时丢失）。改用新镜像包：`docker load < frpc-mesh-vX.Y.Z-docker-linux-arm64.tar.gz` → 把 compose 的 `image` 改成新标签 → `docker compose up -d`（数据目录不变，配置不丢）
- 拒绝自动升级的几种情况（只做提示，需手动下载包替换）：Release 缺 `sha256sums.txt`、当前平台没有对应安装包、二进制不可写、容器环境
- 手动升级普通包：停旧进程 → 解压新包覆盖同目录（`frpc-mesh`/`docs/` 等）→ 用原命令启动；数据目录不动
- Compose 源码部署：`git pull && docker compose up -d --build`（数据库自动迁移，配置不丢）
- 校验手动下载的包：`shasum -a 256 -c sha256sums.txt`（与包同名文件同目录执行）
- 版本号来自构建注入（`make VERSION=x.y.z release`），`./frpc-mesh --version` 可查看

### 4.2 frpc 底座版本
Web「frpc 版本管理」在线下载/切换（运行中服务器自动重启生效）；离线环境把 frpc 放 `data/versions/frpc-<版本号>`（可执行权限）并在页面激活，或放程序目录 `./frpc`

- 前端有改动时部署后浏览器需强刷（Mac: Cmd+Shift+R），否则看到的是旧页面

## 5. 安全边界

- 除 `login`/`health` 外全部 API 需 JWT；改密接口额外校验旧密码——外网知道地址也无法匿名重置密码
- 登录防爆破：连续失败 5 次锁 3 分钟、10 次锁 10 分钟、15 次及以后锁 1 小时（按 IP+用户名计数，成功登录清零）；锁定提示仅对内网来源附带 CLI 重置指引，不外泄给公网访问者。内网范围=环回/RFC1918/链路本地/IPv6 ULA/CGNAT `100.64.0.0/10`（Tailscale 组网），对照下表
- 内网来源判定**只看连接来源 IP，与你用哪个域名访问无关**（域名最终都解析成下表某种 IP）：

| 访问方式 | 容器看到的来源 IP | 判定 |
|---|---|---|
| 宿主机 `http://127.0.0.1:18080` / `localhost` | 127.0.0.1 或引擎网关（OrbStack 192.168.215.1 / Docker Desktop vpnkit 同段） | 内网 ✓ |
| 局域网其它设备 `http://192.168.1.x:18080` | 访问者的 192.168.x / 10.x / 172.16-31.x | 内网 ✓ |
| OrbStack 域名 `*.orb.local:端口`（NAS/本机直连 VM） | 192.168.x（RFC1918 段） | 内网 ✓ |
| 容器内互访 / `host.docker.internal` | 172.17.x、网桥网关 | 内网 ✓ |
| Tailscale 等 VPN 组网 `100.64.0.0/10`（100.64.x.x–100.127.x.x） | CGNAT 段，已显式纳入内网判定 | 内网 ✓ |
| `.local` mDNS、`nas.home.arpa` 等本机解析的域名 | 解析结果仍是上面某种内网 IP | 内网 ✓ |
| 公网 IP 直连（不推荐） | 访问者公网 IP | 公网，不显示重置指引 |
| 经公网反向代理转发（未设内网 XFF） | 代理服务器 IP（公网） | 公网；如确需内网友好提示，代理应设 `X-Forwarded-For` 且代理本身部署在内网侧 |
- 管理面板不要直接暴露公网：仅内网/VPN/按需隧道访问；云主机安全组只放行需要的代理端口
- 备份 JSON 含 frps token 等敏感信息，注意存放位置

## 6. 排错速查

| 现象 | 处理 |
|---|---|
| 启动报"未找到 frpc 二进制" | 设置页指定 frpc 绝对路径；容器部署确认镜像内有 `/usr/local/bin/frpc` 或已挂载 |
| 状态一直 error / 日志 `connect to server error` | frps 地址端口不通、token 不一致、frps 未启动 |
| Admin API 端口被占用 | 多台服务器的 Admin 端口必须互不相同，改该服务器的 admin.port |
| 容器内穿透宿主机服务 | 本地地址填 `host.docker.internal`（表单有快捷芯片）；数字网关 IP 仅备选 |
| 穿透"另一个 Docker 服务" | 用「host.docker.internal + 宿主机映射端口」，无需容器 IP；该服务须监听 0.0.0.0（只听 127.0.0.1 则容器不可达） |
| 隧道在线但远程打不开 | 先 Ping 看「不通/可通」；不通多为 frps 所在云主机**安全组未放行远程端口**（frps 监听 ≠ 外部可达） |
| 代理名冲突（日志 `already exists`） | 同一 frps 下代理名全局唯一；检查其它机器/客户端是否注册了同名代理 |
| 同一远程端口被两个客户端占用 | 不同代理名抢同一 remotePort 会在重连窗口翻转归属；一台 frps 的一个端口只给一个客户端 |
| 状态/延迟含义 | 在线 = 隧道建立 + 本地目标数据层存活；Ping：`-1 不通 / 0 可通 / >0 延迟ms`（容器内 vpnkit 使连接层探测失真，已改数据层判定） |
| 海外线路慢/偶发卡死 | 国际段数据层 stall，非本应用问题；日常优先国内服务器，海外线路作备用 |
| 本地服务被 watchdog 误杀 | 自检脚本若在 launchd/systemd 下用 `lsof` 判活可能恒失败（本项目踩过），改用 HTTP 探测 |
| 登录返回 429「已锁定」 | 防爆破锁定生效中；等窗口过去后用正确密码登录即恢复。**内网**来源的锁定提示会附带本机 CLI 重置指引（见第 2 节），外网来源不显示该指引 |

## 7. 常用命令速查

```bash
docker compose ps / logs -f / restart            # 状态/日志/重启
docker exec -it frpc-mesh /app/frpc-mesh --reset-password --data /data   # 重置密码
curl -s http://127.0.0.1:18080/api/health        # 健康检查（无需登录）
```

API 契约见 [../API.md](../API.md)。
