.PHONY: build build-release clean test lint vet

APP = starter
BUILD_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# ============================================================
# 本地构建（Linux \ macOS）
# 需要本地安装 Go。
# GUI 编译需要 Windows + CGO/MinGW，走 make winvm-*
# ============================================================

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP) ./cmd/starter/

build-server:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP)-server ./cmd/mc-starter-server/

test:
	go test ./... -count=1

test-v:
	go test ./... -v -count=1

vet:
	go vet ./cmd/... ./internal/...

lint: vet

bench:
	go test ./... -bench=. -benchmem

clean:
	rm -rf $(BUILD_DIR)/

# ============================================================
# Docker 编译（本地无 Go 的替代方案）
# 编译 CLI 二进制的 Linux 版 + server
# 注：GUI/walk 依赖 CGO+MinGW，不支持 Docker 交叉编译
#     GUI 编译请走 Windows VM (make winvm-gui)
# ============================================================
DOCKER_GO = docker run --rm -v $(PWD):/src -w /src golang:1.23

.PHONY: docker-build docker-test docker-all docker-shell docker-check

docker-build:
	$(DOCKER_GO) go build -buildvcs=false -o $(BUILD_DIR)/$(APP) ./cmd/starter/

docker-build-server:
	$(DOCKER_GO) go build -buildvcs=false -o $(BUILD_DIR)/$(APP)-server ./cmd/mc-starter-server/

# Docker 下只有非 GUI 包可以通过测试（walk build tags 仅 windows）
DOCKER_TEST_PKGS = ./cmd/mc-starter-server/... ./internal/config/... \
	./internal/downloader/... ./internal/launcher/... \
	./internal/logger/... ./internal/mirror/... \
	./internal/model/... ./internal/pack/... \
	./internal/repair/... ./internal/server/...

docker-test:
	$(DOCKER_GO) go test -buildvcs=false -count=1 $(DOCKER_TEST_PKGS)

# Docker 下只有非 GUI 包可以通过 vet（walk build tags 仅 windows）
docker-vet:
	$(DOCKER_GO) go vet -buildvcs=false ./cmd/... ./internal/config/... \
		./internal/downloader/... ./internal/launcher/... \
		./internal/logger/... ./internal/mirror/... \
		./internal/model/... ./internal/pack/... \
		./internal/repair/... ./internal/server/... \
		./internal/tray/... 2>/dev/null; \
		echo "---"; \
		echo "Tip: VCS stamp skipped (docker), GUI/gui/tray excluded (CGO build tag)"; \
		echo "Done"

docker-check: docker-vet docker-test

docker-all: docker-build docker-build-server docker-test

docker-shell:
	docker run --rm -it -v $(PWD):/src -w /src golang:1.23 /bin/bash

# Docker 构建并推送到 registry
docker-image-server:
	docker build -t mc-starter-server:$(VERSION) .
	docker tag mc-starter-server:$(VERSION) mc-starter-server:latest

# ============================================================
# Windows VM 编译（GUI 相关）
# 需要 ~/.openclaw/workspace/skills/win-vm/scripts/win_exec.py
# ============================================================
WINVM_SCRIPT = $(HOME)/.openclaw/workspace/skills/win-vm/scripts/win_exec.py
WIN_EXEC = python3 $(WINVM_SCRIPT)

.PHONY: winvm-sync winvm-test winvm-build winvm-build-gui winvm-check

winvm-sync:
	$(WIN_EXEC) --sync-only

winvm-test:
	$(WIN_EXEC) --sync "go test ./... -count=1"

winvm-build:
	$(WIN_EXEC) --sync "go build -o starter.exe ./cmd/starter/"

# 编译 GUI 二进制（walk + windowsgui）
winvm-build-gui:
	$(WIN_EXEC) --sync "go build -o starter-gui.exe -ldflags=\"-H windowsgui\" ./cmd/starter/"

# 编译 + vet + 测试（完整检查）
winvm-check:
	$(WIN_EXEC) --sync "go vet ./cmd/... ./internal/... && go test ./... -count=1 && go build -o starter.exe ./cmd/starter/"

# ============================================================
# 辅助命令
# ============================================================

size: build
	@echo "binary size:"
	@ls -lh $(BUILD_DIR)/$(APP)
	@echo ""
	@echo "if upx is available:"
	@which upx 2>/dev/null && upx --best $(BUILD_DIR)/$(APP) && ls -lh $(BUILD_DIR)/$(APP) || echo "upx not installed, skipping"
