# v2 — 待迭代功能（代码存档）

本目录包含的代码不参与当前 v1 构建。启动 v2 迭代时，按以下步骤恢复：

## 1. 修复窗口 GUI
- `gui/repair_window.go` → `internal/gui/repair_window.go`
- 恢复后需在 `internal/gui/app.go` 主窗口添加 🔧 按钮

## 2. 修复 Orchestrator 函数
- `gui/orchestrator_v2.go` 中的函数 → `internal/gui/orchestrator.go`
- 需要还原 import 中的 `"strings"` 和 `"github.com/gege-tlph/mc-starter/internal/repair"`
- 在 `internal/gui/state.go` 中恢复 `StateRepairing`

## 3. 崩溃检测 + 静默守护
- `repair/daemon.go` + `daemon_test.go` → `internal/repair/`

## 恢复命令
```bash
git restore -s HEAD~1 -- internal/gui/repair_window.go
# 以及手动从 HEAD~1 复制 orchestrator.go 中的修复函数
```

## 当前 v1 提交
`1859d9b` — 这是代码隔离后的第一个干净 v1 版本。
