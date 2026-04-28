# MC-Starter — 构建与 CI

> Windows only build pipeline

---

## 一、本地构建

### 1.1 前置要求

- Go 1.22+
- Git
- GNU Make（可选）

### 1.2 构建命令

```bash
# 开发构建 （带控制台窗口，方便调试）
make build

# 发布构建 （无控制台窗口，双击运行）
make build-release

# 查看二进制体积
make size

# 运行测试
make test
```

### 1.3 直接使用 Go

```bash
# 开发版
go build -ldflags="-s -w -X main.version=dev" -o build/starter.exe ./cmd/starter/

# 发布版（隐藏控制台窗口）
GOOS=windows GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=1.0.0 -H windowsgui" \
    -o build/starter-1.0.0-x64.exe \
    ./cmd/starter/
```

### 1.4 手动压缩

```bash
# 安装 UPX（可选）
scoop install upx   # Windows
apt install upx     # WSL

# 压缩
upx --best build/starter.exe
# 典型压缩比: 6MB → 2MB
```

---

## 二、Makefile 参考

```makefile
APP = starter
BUILD_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" \
	  -o $(BUILD_DIR)/$(APP).exe ./cmd/starter/

build-release:
	GOOS=windows GOARCH=amd64 go build \
	  -ldflags="-s -w -X main.version=$(VERSION) -H windowsgui" \
	  -o $(BUILD_DIR)/$(APP)-$(VERSION)-x64.exe ./cmd/starter/

test:
	go test ./... -v -count=1

clean:
	rm -rf $(BUILD_DIR)/
```

---

## 三、持续集成

### 3.1 自动化发布流程

```
 push: main / tag v*
   ↓
 单元测试 → 构建二进制 → UPX 压缩 → 生成 checksum → 发布 artifacts
```

### 3.2 依赖更新管理（Dependabot）

在 `.github/dependabot.yml` 配置：

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    labels:
      - "dependencies"
    commit-message:
      prefix: "chore"
      prefix-development: "chore"
```

---

## 四、CI 配置参考

### GitHub Actions (`.github/workflows/build.yml`)

```yaml
name: build
on:
  push:
    branches: [main]
    tags: ["v*"]
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true

      - name: Test
        run: go test ./... -v -count=1

      - name: Build
        run: |
          $version = if ("${{ github.ref }}" -match "^refs/tags/v") { "${{ github.ref_name }}" } else { "dev" }
          go build -ldflags="-s -w -X main.version=$version -H windowsgui" `
            -o build/starter-$version-x64.exe `
            ./cmd/starter/

      - name: Sign （optional, 需要证书）
        run: |
          # signtool sign /fd SHA256 /a build/*.exe

      - name: Upload artifact
        uses: actions/upload-artifact@v4
        with:
          name: mc-starter
          path: build/

  release:
    if: startsWith(github.ref, 'refs/tags/v')
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: mc-starter

      - name: Generate checksum
        run: sha256sum *.exe > checksums.txt

      - name: Create Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            *.exe
            checksums.txt
          generate_release_notes: true
```

---

## 五、发布流程

1. 打 tag: `git tag v1.0.0 && git push origin v1.0.0`
2. CI 自动构建 → 压缩 → 生成 checksum → 创建 Release
3. 用户去 GitHub Releases 下载 `.exe` 文件
4. 自更新通道检测到新版本，自动拉取替换

---

## 六、相关文档

| 文档 | 说明 |
|---|---|
| [详细开发流程](详细开发流程.md) | 分阶段开发、验收标准 |
| [WBS 迭代计划](WBS-迭代计划.md) | 开发排期 |
| [自更新方案](自更新方案.md) | 启动器自身更新机制 |
