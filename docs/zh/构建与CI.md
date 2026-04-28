# MC 版本更新器 — 构建脚本 + CI 配置

> Go 项目，三平台编译

---

## 一、Makefile

```makefile
# === 变量 ===
BINARY_NAME    = mc-starter
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT        ?= $(shell git log -1 --format=%h 2>/dev/null || echo "unknown")
BUILD_TIME    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS        = -ldflags="-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -s -w"
OUTPUT_DIR     = build

# === 默认目标 ===
.PHONY: all build test clean

all: build

# === 构建 ===
build:
	@mkdir -p $(OUTPUT_DIR)
	go build $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) ./cmd/starter/
	@echo "✅  Built: $(OUTPUT_DIR)/$(BINARY_NAME)"

build-windows:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/starter/
	@echo "✅  Built: $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe"

build-linux:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/starter/
	@echo "✅  Built: $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64"

build-darwin:
	@mkdir -p $(OUTPUT_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/starter/
	@echo "✅  Built: $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64"

# 三平台全量编译
build-all: build-windows build-linux build-darwin
	@echo "✅  All builds complete"

# === 测试 ===
test:
	go test -v -race -count=1 ./... 2>&1 | tee test-output.log

test-short:
	go test -short -count=1 ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅  Coverage report: coverage.html"

# === 代码质量 ===
lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

# === 清理 ===
clean:
	rm -rf $(OUTPUT_DIR)/
	rm -f coverage.out coverage.html test-output.log
	@echo "✅  Cleaned"

# === 开发辅助 ===
run:
	go run ./cmd/starter/ $(ARGS)

dev: build
	./$(OUTPUT_DIR)/$(BINARY_NAME) $(ARGS)

# === 发布 ===
release: test build-all
	@echo "📦  Release $(VERSION) ready in $(OUTPUT_DIR)/"
	@ls -lh $(OUTPUT_DIR)/

# === Docker 开发环境 ===
docker-build:
	docker build -t mc-starter-builder -f Dockerfile.build .
	docker run --rm -v $(PWD):/workspace mc-starter-builder make build-all

Dockerfile.build:
	@echo "FROM golang:1.22-alpine" > Dockerfile.build
	@echo "RUN apk add --no-cache git make" >> Dockerfile.build
	@echo "WORKDIR /workspace" >> Dockerfile.build
	@echo "CMD [\"make\", \"build-all\"]" >> Dockerfile.build

.PHONY: build build-windows build-linux build-darwin build-all test test-short test-coverage
.PHONY: lint fmt vet clean run dev release docker-build
```

---

## 二、GitHub Actions CI

```yaml
# .github/workflows/ci.yml

name: Build & Test

on:
  push:
    branches: [main, dev]
    tags: [v*]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Lint
        uses: golangci/golangci-lint-action@v4

      - name: Test
        run: go test -v -race -count=1 ./...

      - name: Build
        run: go build -ldflags="-s -w" -o mc-starter ./cmd/starter/

  release:
    if: startsWith(github.ref, 'refs/tags/v')
    needs: [test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Cross compile
        run: |
          mkdir -p dist
          GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/mc-starter-windows-amd64.exe ./cmd/starter/
          GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o dist/mc-starter-linux-amd64   ./cmd/starter/
          GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o dist/mc-starter-darwin-amd64  ./cmd/starter/
          cd dist
          sha256sum * > checksums.txt

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: dist/*
          generate_release_notes: true
```

---

## 三、版本号管理

### 方案：语义化版本 + git tag

```
v1.0.0         ← 正式发布
v1.1.0-beta    ← beta 通道
v1.1.0-alpha   ← dev 通道
```

### Go 代码中注入版本号

```go
// cmd/starter/main.go
package main

var (
    Version   = "dev"
    Commit    = "unknown"
    BuildTime = "unknown"
)

func main() {
    rootCmd := &cobra.Command{
        Use: "mc-starter",
        // ...
    }

    versionCmd := &cobra.Command{
        Use: "version",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("mc-starter %s\n", Version)
            fmt.Printf("Commit: %s\n", Commit)
            fmt.Printf("Built:  %s\n", BuildTime)
        },
    }
}
```

### 发布流程

```bash
# 1. 打 tag
git tag -a v1.0.0 -m "v1.0.0: 正式发布"

# 2. 推送 tag（触发 CI release）
git push origin v1.0.0

# 3. CI 自动编译 + 创建 GitHub Release
```

---

## 四、开发环境要求

```bash
# 必须
Go 1.22+
Git

# 可选
golangci-lint    # 代码检查：go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
goreleaser       # Release 自动化（后续再看需不需要）
Docker           # 可选的构建环境
```

---

## 五、快速上手

```bash
# 克隆
git clone https://github.com/你的名字/mc-starter.git
cd mc-starter

# 开发
make dev ARGS="--help"
make dev ARGS="init"

# 测试
make test

# 全平台编译
make build-all
# → build/ 目录下三个平台的可执行文件

# 发布
git tag v0.1.0
git push origin v0.1.0
# → CI 自动编译 + 发布 Release
```
