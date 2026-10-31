.PHONY: build dev docker web test release

GO        ?= go
VERSION   ?= 1.0.0
PLATFORMS = linux-arm64 linux-amd64 darwin-arm64
LDFLAGS   = -trimpath -ldflags "-s -w"

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

# 从 pack-base/<platform>（start.sh + 随包 frpc 底座）组装 dist/ 交付压缩包：
# 三平台普通运行包 tar.gz + arm64 镜像 tar；dist/ 内只留压缩包
release:
	@rm -rf dist/.stage && mkdir -p dist/.stage dist
	@for p in $(PLATFORMS); do \
		os=$${p%-*}; arch=$${p#*-}; name=frpc-mesh-v$(VERSION)-$$p; \
		mkdir -p dist/.stage/$$name dist/.stage/$$name/docs; \
		cp pack-base/$$p/start.sh pack-base/$$p/frpc dist/.stage/$$name/; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build $(LDFLAGS) -o dist/.stage/$$name/frpc-mesh . || exit 1; \
		cp README.md dist/.stage/$$name/ && cp docs/*.md dist/.stage/$$name/docs/ && cp API.md dist/.stage/$$name/docs/api.md; \
		tar czf dist/$$name.tar.gz -C dist/.stage $$name || exit 1; \
	done
	@rm -rf dist/.stage
	docker save frpc-mesh:latest | gzip > dist/frpc-mesh-v$(VERSION)-docker-linux-arm64.tar.gz
	@ls -lh dist/*.tar.gz
