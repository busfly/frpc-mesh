.PHONY: build dev docker web test release

GO        ?= go
VERSION   ?= 1.0.1
PLATFORMS = linux-arm64 linux-amd64 darwin-arm64
# 版本号注入二进制：--version 自检与网页端「当前版本」都取这个值
LDFLAGS   = -trimpath -ldflags "-s -w -X main.Version=$(VERSION)"
# 随包 frpc 底座（按版本号分目录，三平台共用 pack-base/start.sh）
FRPC_BASE ?= pack-base/frpc-0.71.1

build:
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o frpc-mesh .

dev:
	$(GO) run . --http 127.0.0.1:18080 --data ./data

web:
	cd web && pnpm install && pnpm build

test:
	$(GO) test ./...

docker:
	docker compose build

# 从 pack-base 组装 dist/ 交付压缩包：
# 三平台普通运行包 tar.gz + arm64 镜像 tar；dist/ 内只留压缩包

release:
	@test -d $(FRPC_BASE) || { echo "缺少 frpc 底座目录 $(FRPC_BASE)，请放入三平台 frpc 或用 FRPC_BASE=<路径> 指定"; exit 1; }
	@for p in $(PLATFORMS); do \
		test -x $(FRPC_BASE)/$$p/frpc || { echo "缺少 $(FRPC_BASE)/$$p/frpc"; exit 1; }; \
	done
	@rm -rf dist/.stage && mkdir -p dist/.stage dist
	@for p in $(PLATFORMS); do \
		os=$${p%-*}; arch=$${p#*-}; name=frpc-mesh-v$(VERSION)-$$p; \
		mkdir -p dist/.stage/$$name dist/.stage/$$name/docs; \
		cp pack-base/start.sh $(FRPC_BASE)/$$p/frpc dist/.stage/$$name/; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build $(LDFLAGS) -o dist/.stage/$$name/frpc-mesh . || exit 1; \
		cp README.md README-CN.md dist/.stage/$$name/ && cp docs/*.md dist/.stage/$$name/docs/ && cp API.md dist/.stage/$$name/docs/api.md && cp -R docs/screenshots dist/.stage/$$name/docs/; \
		tar czf dist/$$name.tar.gz -C dist/.stage $$name || exit 1; \
	done
	@rm -rf dist/.stage
	docker build --build-arg VERSION=$(VERSION) -t frpc-mesh:latest .
	docker save frpc-mesh:latest | gzip > dist/frpc-mesh-v$(VERSION)-docker-linux-arm64.tar.gz
	@cd dist && { sha256sum frpc-mesh-v$(VERSION)-*.tar.gz 2>/dev/null || shasum -a 256 frpc-mesh-v$(VERSION)-*.tar.gz; } > sha256sums.txt
	@echo '--- sha256sums.txt（一键升级依赖此文件，须与安装包一起上传 Release）---'
	@cat dist/sha256sums.txt
	@ls -lh dist/*.tar.gz
