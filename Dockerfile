# ---------- 前端构建 ----------
FROM node:22-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml* web/.npmrc* ./
RUN pnpm install --no-frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---------- 后端构建 ----------
FROM golang:1.23-alpine AS builder
ENV GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0
WORKDIR /src
# VERSION 决定容器内 frpc-mesh 自报版本，供「检查新版本」与自升级判定使用
# 与 Makefile / main.go / docker-compose.yml 的版本保持一致；make release 会用 --build-arg 覆盖
ARG VERSION=1.1.0
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/frpc-mesh .

# ---------- 运行时 ----------
FROM alpine:3.20
ARG FRPC_VERSION=0.61.1
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -H -u 10001 appuser \
    && mkdir -p /data /opt/frpc \
    && (wget -qO /tmp/frp.tar.gz "https://ghfast.top/https://github.com/fatedier/frp/releases/download/v${FRPC_VERSION}/frp_${FRPC_VERSION}_linux_${TARGETARCH}.tar.gz" \
        || wget -qO /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRPC_VERSION}/frp_${FRPC_VERSION}_linux_${TARGETARCH}.tar.gz" \
        || true) \
    && if [ -s /tmp/frp.tar.gz ]; then tar -xzf /tmp/frp.tar.gz -C /tmp && mv /tmp/frp_*/frpc /usr/local/bin/frpc && chmod +x /usr/local/bin/frpc; fi \
    && rm -rf /tmp/frp* \
    && chown -R appuser:appuser /data
WORKDIR /app
COPY --from=builder /out/frpc-mesh /app/frpc-mesh
USER appuser
ENV FRPC_MESH_DATA=/data TZ=Asia/Shanghai
VOLUME /data
EXPOSE 18080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:18080/api/health || exit 1
ENTRYPOINT ["/app/frpc-mesh", "--http", "0.0.0.0:18080", "--data", "/data"]
