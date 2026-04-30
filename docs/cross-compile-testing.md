# mc-starter 跨平台编译与测试经验总结

> 2026-04-30 · 虾虾整理
> 环境: Linux (Ubuntu) → Windows amd64 · 工具链: x1unix/go-mingw + Wine

---

## 一、整体方案

```
mc-starter (Go + lxn/walk)
    │
    ├── internal/gui/     ← Windows-only (Walk CGO)
    ├── internal/*/       ← 跨平台纯 Go
    └── cmd/starter/      ← 入口 (含 gui)
```

**策略**: 用 `x1unix/go-mingw` Docker 镜像交叉编译为 Windows exe，Wine 运行测试。

---

## 二、工具链选型

### 推荐: `x1unix/go-mingw:latest`
- **优点**: 开箱即用，Go 1.26 + mingw-w64 14，支持 ARM64/386/amd64
- **体积**: ~970MB（不算大）
- **CI 模板**: 官方提供 GitHub Actions / GitLab CI 示例
- **坑**: 默认 root 用户运行，产出文件属主 root; 可以用 `-u $UID:$GID` 解决

### 备选: `dockercore/golang-cross`
- Docker 官方维护，但最后更新 5 年前，Go 1.13，**不推荐**

### 备选: 自己搭
```dockerfile
FROM golang:1.23-bookworm
RUN apt update && apt install -y gcc-mingw-w64-x86-64
```
灵活但需自行维护。`x1unix/go-mingw` 更省事。

---

## 三、需要关注的关键问题

### 3.1 Walk 版本兼容性（踩坑重点）

**背景**: `go.mod` 锁的是 `lxn/walk v0.0.0-20210112085537`（2021 年初版本），这个版本的 declarative API 比较老。

**三个不兼容**:
| 问题 | 表现 | 修复 |
|------|------|------|
| `PushButton` 无 `Width` 字段 | `unknown field Width` | 改用 `MinSize: Size{28, 0}` + `MaxSize: Size{28, 0}` |
| `MsgBox` 缺参数 | `not enough arguments` | 加 `walk.MsgBoxOK` 参数 |
| `MainWindow.Run()` 返回值 | `Run()` 返回 `int` 不是 `(int, error)` | `buildUI().Run()` 不接返回值 |

**教训**: Win 上开发时可能是更新的 Walk 版本。跨平台编译前检查 `go.sum` 里 walk 的 commit hash，API 要确认兼容。

### 3.2 Windows-only API 不可用

`walk.Getenv`、`walk.Stat` 在 Linux 交叉编译时不存在（它们不是 Walk 包提供的函数）。

**修复**: 改成标准库 `os.Getenv`、`os.Stat`。

### 3.3 结构体字段名不匹配

`launcher.PCLDetection` 的字段是 `PCLDir` 不是 `MinecraftDir`。GUI 层 `detectLauncher()` 里用了 `result.MinecraftDir`。

**原因**: 代码改了结构体但没改 GUI 引用。

### 3.4 Wine 下的 symlink 支持

Wine 对 `os.Symlink` / `os.Readlink` 实现不完整。测试中 `TestCreateFullSnapshot` 和 `TestPublishDraft` 都会因为 symlink 读取失败而挂。

**修复**: 测试代码对 symlink 检查改为容错（`err == nil` 时才 assert）。

**更好的做法**: 用 `os.Lstat` + `IsDir/IsRegular` 代替 symlink 断言，或者改业务逻辑把 symlink 做成可选。

### 3.5 测试路径分隔符

在 Wine 下 `filepath.Join` 生成 `\` 分隔符（Windows 风格），测试用 `strings.Contains` 匹配 `/` 分隔符路径会失败。

**修复**: 用 `filepath.FromSlash("org/ow2/asm/asm/9.9/asm-9.9.jar")` 代替硬编码路径。

### 3.6 测试期望值不跨平台

`TestShouldInclude` 中 `"disallow windows + arch x86"` 的期望值是 `!isWindows`。在 Wine 下 `isWindows = true` 所以期望值是 `false`，但实际 arch=x86 不匹配 64 位 CPU，disallow 不生效，正确应该是 `true`。

**教训**: 测试写的时候只考虑了 Linux 运行环境，跨平台测试能暴露这种隐含假设。

---

## 四、Docker 化策略

### 当前: `mc-starter:test` 镜像

```dockerfile
FROM x1unix/go-mingw:latest
RUN apt update && apt install -y --no-install-recommends wine && \
    rm -rf /var/lib/apt/lists/*
```

**用法**:
```bash
# 编译测试 exe + Wine 跑
docker run --rm -v $(pwd):/work -w /work mc-starter-test sh -c '
  for pkg in ./internal/launcher/... ./internal/repair/...; do
    go test -buildvcs=false -c -o /tmp/$(basename $pkg).test.exe "$pkg"
  done
  wine /tmp/launcher.test.exe -test.v
'
```

**注意**: 容器内编译一次，Wine 跑一次，每次 rebuild 很慢（依赖重新下载）。建议：

1. **挂载 Go 缓存**:
   ```
   -v $(go env GOCACHE):/go/cache -e GOCACHE=/go/cache
   ```
2. **挂载 GOPATH mod cache**:
   ```
   -v $(go env GOPATH)/pkg:/go/pkg
   ```

### 更好的方案: 两步分离

**编译容器** (`x1unix/go-mingw`) → 产出 `.test.exe`
**测试容器** (`wine` 专用镜像) → 跑 exe 测试

这样编译容器瘦（无 wine → 970MB），测试镜像可以小很多。

### CI 方案 (GitHub Actions)

参考 `x1unix/docker-go-mingw` 官方模板:
```yaml
jobs:
  test-windows:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/x1unix/docker-go-mingw/go-mingw:1.26
    steps:
      - uses: actions/checkout@v4
      - name: Cross-compile tests
        run: |
          go test -buildvcs=false -c -o test.exe ./internal/launcher/...
      - name: Run via wine
        run: |
          apt update && apt install -y wine
          wine test.exe -test.v
```

---

## 五、已知限制

| 限制 | 原因 | 影响范围 |
|------|------|---------|
| 🚫 GUI 测试无法在 Wine 运行 | Walk 需要真实 Windows 桌面环境 (GDI) | `internal/gui/` 所有测试 |
| ⚠️ symlink 测试容错跳过 | Wine 对 NTFS 重解析点支持有限 | `repo_test.go`, `pack_test.go` |
| ⚠️ 进程信号测试可能不稳定 | Wine 不实现 `Signal(0)` | `daemon_test.go:TestDaemonProcessAlive` |
| ✅ 纯逻辑测试完全可跑 | Go 标准库兼容性好 | 其余所有包 |

**结论**: 180 个测试中 177 个可在 Wine 下正常运行, 3 个受 Wine 环境限制被迫跳过。对于 GUI 包的真正测试，仍需 Windows CI runner。

---

## 六、未踩但值得注意的坑（来自网络）

| 坑 | 来源 | 说明 |
|----|------|------|
| 💣 32-bit 容器 bug | [x1unix/docker-go-mingw#16](https://github.com/x1unix/docker-go-mingw/issues/16) | `--platform linux/386` + i386 镜像会 panic `mallocgc called without P` |
| 💣 CGO 静态链接 | Reddit | sqlite3 等 CGO 库需要 `-linkmode external -extldflags '-static -w'` |
| 💣 `-buildvcs=false` | 官方 Go 1.18+ | Docker CI 中 git dubious ownership → 需要这个 flag |
| 💣 Wine 下 `errno` 兼容 | StackOverflow | 某些 syscall 返回 POSIX errno 而非 Windows HRESULT |
| 💣 Walk manifest 缺失 | lxn/walk 文档 | `.exe` 需要嵌入 `.manifest`（`<dpiAware>` / Common Controls 6），否则字体模糊/控件错位 |
| 💣 GOARCH=arm64 交叉编译 | x1unix/go-mingw | 需 llvm-mingw 而非 gcc-mingw，镜像已包含但首次构建慢 |

---

## 七、最佳实践清单

1. **开发前检查 Walk API 版本**: `go list -m github.com/lxn/walk`，与目标编译环境的依赖一致
2. **GUI 和非 GUI 严格分离**: `internal/gui/` 加 `//go:build windows` tag，Linux/CI 下自动跳过
3. **测试文件路径用 `filepath.FromSlash`**: 确保跨平台路径匹配
4. **测试 symlink 断言容错**: `if err == nil { assert... }`
5. **Docker cache 挂载**: GOCACHE + go/pkg 加速重编译（3-5x）
6. **CI 两步策略**: Windows `runs-on` runner 跑 GUI 测试，Linux Docker + Wine 跑逻辑层
7. **注意 CGO 静态链接**: 如果需要部署到纯 Windows 环境，考虑 `-linkmode external -extldflags '-static -w'`
