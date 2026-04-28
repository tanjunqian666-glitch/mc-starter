# MC 版本更新器 — 用户界面

> 修复工具不仅被动触发（崩溃检测），还需要主动入口（GUI + TUI + 命令行快捷方式）。

---

## 一、设计原则

1. **GUI 用 Windows 原生** — 托盘菜单 + MessageBox 弹窗，不引入额外 UI 框架
2. **TUI 风格干练** — 信息密度高、操作流畅、视觉效果媲美原生 GUI，少用 emoji
3. **跨平台不牺牲 Windows 体验** — TUI 通用，Windows 原生弹窗兜底

---

## 二、GUI — 系统托盘

### 2.1 右键菜单结构

```
┌─────────────────────┐
│ MC-Starter v1.0     │
│─────────────────────│
│ 状态: 已同步         │
│─────────────────────│
│ 同步到服务器版本      │
│ 修复工具              │
│ 管理备份              │
│─────────────────────│
│ 上传问题报告          │
│ 打开 .minecraft       │
│─────────────────────│
│ 退出守护模式          │
└─────────────────────┘
```

### 2.2 托盘状态图标

```go
const (
    IconNormal  = iota  // 正常
    IconSyncing         // 正在同步
    IconWarning         // 有更新可用
    IconError           // 需要修复
)

func updateTrayIcon(status int) {
    switch status {
    case IconNormal:
        setIcon(greenIcon)
        setTooltip("MC-Starter: 已同步")
    case IconSyncing:
        setIcon(blueIcon)
        setTooltip("MC-Starter: 正在同步...")
    case IconWarning:
        setIcon(yellowIcon)
        setTooltip("MC-Starter: 有新版本可用")
    case IconError:
        setIcon(redIcon)
        setTooltip("MC-Starter: 需要修复")
    }
}
```

### 2.3 实现方案

使用 `getlantern/systray` 库，纯 Go，跨平台，（Windows 原生托盘）

---

## 三、TUI — 终端交互界面

### 3.1 状态面板

```
 MC-Starter 修复工具

  Java    21.0.3 (64-bit)
  PCL2    D:\Game\MC\PCL2\Plain Craft Launcher 2.exe
  .mine   D:\Game\MC\PCL2\.minecraft
  Fabric  fabric-loader-0.15.11-1.20.1
  模组     15/15 已同步
  配置     23 个文件（含 2 个用户自定义）

  上次同步: 2026-04-29 01:30 (13 分钟前)
  备份数:   2
  最新备份: 2026-04-29 00:28 (修复前自动创建)

  1) 同步到服务器版本
     下载最新模组和配置，保持用户数据
  2) 完全修复（推荐）
     清空 mods+config，重新下载纯净版，保留存档
  3) 管理备份
     查看、恢复、删除备份
  4) 上传问题报告
     上传日志和状态给服务器

  [ESC] 退出  [1-4] 选择操作
```

### 3.2 下载进度

```
 同步中: 步骤 3/5 下载模组 (12/15)  80%

  sodium-fabric-0.5.3.jar    [████████████████████] 100%
  lithium-fabric-0.11.2.jar  [██████████░░░░░░░░░░]  45%
  iris-fabric-1.6.4.jar      [░░░░░░░░░░░░░░░░░░░░]   0%
  预计剩余: 30 秒
```

### 3.3 实现方案

使用 `charmbracelet/bubbletea` 库（Go 的终端 UI 框架），支持：
- 键盘导航（上下方向键 + 回车选择）
- 实时进度条
- 选择/确认对话框
- 跨平台一致体验

### 3.4 无终端时的兜底 — Windows 原生弹窗

当从 PCL2 启动或双击运行且无终端窗口时，使用 MessageBoxW：

```go
func showCrashDialog(crash CrashInfo) int {
    title := "MC-Starter - Minecraft 崩溃了"
    message := fmt.Sprintf(
        "Minecraft 异常退出（退出码 %d）\n\n", crash.ExitCode)
    if crash.Reason != "" {
        message += "可能原因: " + crash.Reason + "\n\n"
    }
    message += "是否执行自动修复？\n"
    message += "修复将备份当前 mods/config，" +
        "恢复为服务器纯净版本，并重新启动。\n\n"
    message += "不修复可手动运行: starter repair"

    // 调用 user32.dll MessageBoxW
    return MessageBox(hWnd, message, title, MB_YESNO|MB_ICONERROR)
}
```

触发策略：
- 有终端窗口 → TUI
- 从 PCL2/快捷方式启动（无终端）→ MessageBox
- `--headless` → 自动选择 + 日志

---

## 四、修复入口总览

### 命令行树

```
starter
├── run                全流程: 同步 -> 启动 -> 守护
├── sync               仅同步
├── repair             修复工具（自动选择 TUI / 弹窗）
│   ├── --clean        全量修复（不询问）
│   ├── --sync-only    仅同步到服务器版本
│   └── --select       选择性修复
├── backup
│   ├── list           列出备份
│   ├── restore        恢复备份
│   ├── create         手动创建备份
│   └── delete         删除备份
├── pcl
│   ├── detect         检测 PCL2
│   └── path           设置 PCL2 路径
├── check              环境检查
├── init               初始化
├── version            版本信息
└── self-update        自更新
```

### 快捷方式

在 PCL2 目录旁放修复脚本（不弹黑框）：

```batch
@REM 修复.bat
@echo off
start "" /wait "%~dp0starter.exe" repair
```

```vbs
' 启动 MC-Starter 修复.vbs — 双击运行，隐藏命令行窗口
CreateObject("WScript.Shell").Run "starter.exe repair", 0, True
MsgBox "修复完成！请刷新 PCL2 版本列表。", vbInformation, "MC-Starter"
```

---

## 五、WBS 补充

| ID | 任务 | 预估 | 前置 |
|---|---|---|---|
| P2.12 | 修复 TUI 界面: bubbletea 布局 + 选项交互 | 4h | P2.7 |
| P2.13 | 托盘菜单入口: 同步/修复/备份 | 2h | P2.9 |
| P2.14 | Windows 原生弹窗兜底（无终端时） | 2h | P2.8 |
| P2.15 | 修复后 PCL2 自动刷新 | 1h | P2.12, P4.3 |
