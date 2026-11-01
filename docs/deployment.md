# 部署指南

三种部署方式与首次登录。日常运维（备份/升级/排错）见 [维护手册](operations.md)。

## 一、Docker Compose（推荐，NAS / 任意 Docker 主机）

```bash
docker compose up -d --build     # 源码构建并启动
docker compose ps                # 状态应为 healthy
docker compose logs -f frpc-mesh # 看日志
```

- 端口：`18080:18080`（bridge + 显式映射，可在 compose 改宿主端口）
- 数据：宿主机目录 bind 挂载 → 容器 `/data`（仓库默认写的是本机路径，部署前改成目标机器自己的目录）
- 宿主机目录方式的好处：配置直接落在可见目录，卸载/重装 Docker 不丢数据，整目录拷贝即完成备份
- 回退命名卷方式：compose 中 volumes 改回 `frpc-mesh-data:/data` 并在末尾恢复 `volumes:` 段声明
- Mac 上 Docker Desktop 需在 Settings → Resources → File Sharing 中包含该目录（外置盘同理）
- 已配置 `extra_hosts: host.docker.internal:host-gateway`，Linux 容器内可用该域名访问宿主机
- 日志与数据同风格：compose 已把宿主机 `<统一目录>/logs` 挂到容器 `/logs` 并设了 `FRPC_MESH_LOG_DIR=/logs`，所以面板运行日志会同时出现在宿主机目录里（按 10MB 滚动留 5 份）；路径同样按本机改，起容器前先 `mkdir -p`；不想落文件就不挂这个目录、不设该变量。Docker 的 Volumes 界面只显示命名卷，bind 挂载要看容器的 Mounts
- 镜像构建时自动尝试下载 frpc（ghfast.top 镜像 → GitHub 直连 → 跳过）；无外网环境可挂载宿主机 frpc：

```yaml
volumes:
  - /usr/local/bin/frpc:/usr/local/bin/frpc:ro
```

## 二、导入镜像包（无源码 / 离线环境）

```bash
gunzip -c frpc-mesh-vX.Y.Z-docker-linux-arm64.tar.gz | docker load   # vX.Y.Z = 你下载的包版本
docker run -d --name frpc-mesh --restart unless-stopped \
  --add-host host.docker.internal:host-gateway \
  -p 18080:18080 -v /path/on/host/frpc-mesh/data:/data frpc-mesh:latest
# 或把 image 名写进自己的 compose
```

镜像为 linux/arm64（Apple Silicon 与 arm64 NAS 适用）；x86 机器用方式一源码构建或方式三普通包。

- 想让运行日志也落到宿主机目录，给上面的 `docker run` 再补 `-v /path/on/host/frpc-mesh/logs:/logs -e FRPC_MESH_LOG_DIR=/logs`；不设置就只写标准输出，`docker logs frpc-mesh` 照样能看

## 三、普通运行包（无 Docker）

```bash
tar xzf frpc-mesh-vX.Y.Z-linux-arm64.tar.gz
cd frpc-mesh-vX.Y.Z-linux-arm64
./start.sh                        # 前台运行，Ctrl+C 停止
nohup ./start.sh > frpc-mesh.out 2>&1 &   # 或后台
```

- 包内 `./frpc` 为随包二进制（离线可用）；每台服务器按「本机指定的版本 → 版本管理页激活的默认版本 → PATH / 常见路径 / 包内 `./frpc`」顺序取二进制
- 数据默认写在包内 `./data/`（`--data` 或环境变量 `FRPC_MESH_DATA` 可改）
- 监听地址用 `--http` 改，默认 `0.0.0.0:18080`
- 建议 systemd 托管：`ExecStart=/opt/frpc-mesh/frpc-mesh --data /opt/frpc-mesh/data`，`Restart=always`

## 运行参数（三种方式通用）

| 参数/环境变量 | 默认值 | 说明 |
|---|---|---|
| `--http` | `0.0.0.0:18080` | Web 监听地址 |
| `--data` / `FRPC_MESH_DATA` | `./data` | 数据目录（内容结构见维护手册） |
| `--log-dir` / `FRPC_MESH_LOG_DIR` | —（不写文件） | 运行日志同时落盘到该目录的 `frpc-mesh.log`，单文件 10MB、最多留 5 份 |
| `--reset-password` | — | 重置 admin 密码后退出（仅本机 CLI，见维护手册） |

## 首次登录

- 地址：`http://<主机IP>:18080`
- 默认账号 `admin / admin123`，**登录后立即在「设置-修改密码」修改**
- 忘记密码的重置方法见 [维护手册](operations.md) 第 2 节
