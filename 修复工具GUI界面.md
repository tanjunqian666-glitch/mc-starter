# MC 版本更新器 — 修复工具 GUI 界面

> 修复工具不仅被动触发（崩溃检测），还需要主动入口（GUI 界面 + 命令行快捷方式）

---

## 一、GUI 界面设计

### 1.1 触发方式

| 方式 | 入口 |
|---|---|
| **命令行** | `starter repair`（沿用已有的 CLI 命令） |
| **托盘菜单** | 右键托盘图标 → "修复到服务器版本" |
| **桌面快捷方式** | `starter.exe --repair-gui`（直接打开修复界面） |
| **双击 + 检测** | 启动时检测到问题 → 自动弹出修复界面 |
| **PCL2 旁快捷方式** | 在 PCL2 目录下放一个 `修复.bat` 双击打开 |

### 1.2 GUI 实现方案

**不做真 GUI**（维护成本高），而是用 **终端 UI** + 可选的 **Web UI**：

| 方案 | 适用场景 | 理由 |
|---|---|---|
| **终端交互 UI（TUI）** | 默认方案 | Go 的 bubbletea 库，伪 GUI 体验，跨平台 |
| **Windows 弹窗** | 崩溃检测时 | go-ole / 原生 MessageBox |
| **Web UI（远期）** | 远程管理 | `localhost:8080` 开网页控制 |

**目前就用终端 TUI**，但体验上像 GUI（用方向键选择、回车确认、进度条）。

### 1.3 TUI 修复界面

```
╔══════════════════ MC-Starter 修复工具 ═══════════════════╗
║                                                          ║
║  当前状态：                                               ║
║    Java:    ✓ 21.0.3 (64-bit)                             ║
║    PCL2:    ✓ D:\Game\MC\PCL2\Plain Craft Launcher 2.exe ║
║    .mine:   ✓ D:\Game\MC\PCL2\.minecraft                  ║
║    Fabric:  ✓ fabric-loader-0.15.11-1.20.1               ║
║    模组:     15/15 已同步                                  ║
║    配置:     ✓ 23 个文件（含 2 个用户自定义）               ║
║    存档:     2 个                                          ║
║                                                          ║
║  ┌──────────────────────────────────────────────────────┐ ║
║  │  上次同步: 2026-04-29 01:30（13 分钟前）             │ ║
║  │  备 份 数: 2 个                                      │ ║
║  │  最新备份: 2026-04-29 00:28（修复前自动创建）         │ ║
║  └──────────────────────────────────────────────────────┘ ║
║                                                          ║
║  [1] 🔄 同步到服务器版本                                  ║
║      → 下载最新模组和配置，保持用户数据                    ║
║                                                          ║
║  [2] 🔧 完全修复（推荐）                                  ║
║      → 清空 mods+config → 重新下载纯净版 → 保留存档       ║
║                                                          ║
║  [3] 📦 管理备份                                          ║
║      → 查看、恢复、删除备份                               ║
║                                                          ║
║  [4] 📤 上传问题报告                                      ║
║      → 上传日志和状态给服务器                             ║
║                                                          ║
║  [ESC] 退出                                              ║
╚═══════════════════════════════════════════════════════════╝
```

---

## 二、修复 GUI 的三种模式

### 2.1 快速同步（日常维护）

```go
func quickSyncGUI() {
    // 不删任何文件，只下载新增/更新的
    showProgress("正在检查更新...")
    
    // 对比 server.json 和本地状态
    changed := checkForChanges()
    
    if len(changed.Mods) == 0 && len(changed.Configs) == 0 {
        showSuccess("你的整合包已经是最新版本！")
        return
    }
    
    // 显示变更内容
    fmt.Println("\n检测到以下变更：")
    for _, m := range changed.Mods {
        switch m.Action {
        case "add":
            fmt.Printf("  📥 + %s\n", m.Name)
        case "update":
            fmt.Printf("  🔄 ~ %s\n", m.Name)
        case "remove":
            fmt.Printf("  🗑  - %s\n", m.Name)
        }
    }
    for _, c := range changed.Configs {
        fmt.Printf("  📄 ~ %s\n", c.Path)
    }
    
    if confirm("是否同步？") {
        doSync()
        showSuccess("同步完成！")
    }
}
```

### 2.2 完全修复（恢复纯净状态）

```go
func fullRepairGUI() {
    // 显示警告
    fmt.Println("")
    fmt.Println("⚠ 完全修复将执行以下操作：")
    fmt.Println("  1. 备份当前 mods/、config/、resourcepacks/ 到 starter_backups/")
    fmt.Println("  2. 清空上述目录（存档和截图不受影响）")
    fmt.Println("  3. 从服务器重新下载所有文件")
    fmt.Println("  4. （可选）重新安装 Fabric")
    fmt.Println("")
    
    // 询问是否保留存档
    keepSaves := confirm("是否保留存档？(建议保留) [Y/n]")
    
    // 询问是否清空资源包
    clearResourcePacks := confirm("是否清空资源包？[y/N]", false)
    
    // 询问是否覆盖用户配置
    forceConfig := promptSelect("配置文件处理方式:",
        []string{"覆盖为服务器版本（推荐）", "保留用户自定义"})
    
    // 执行修复
    startRepair(RepairConfig{
        Backup:             true,
        ClearMods:          true,
        ClearConfig:        true,
        ClearResourcepacks: clearResourcePacks,
        KeepSaves:          keepSaves,
        ForceConfig:        forceConfig == "覆盖为服务器版本",
    })
}
```

### 2.3 选择性修复（精细控制）

```go
func selectiveRepairGUI() {
    // 类似"修复→选文件"的交互
    // 显示文件列表，空格多选
    options := []string{
        "[ ] 重新下载所有模组",
        "[ ] 覆盖配置文件",
        "[x] 重新安装 Fabric",
        "[ ] 清除崩溃报告",
        "[ ] 清除日志",
    }
    
    selected := multiSelect("选择要修复的项目（空格选择，回车确认）:", options)
    // ...
}
```

---

## 三、托盘菜单

### 3.1 右键菜单结构

```
───────────────
  MC-Starter v1.0
  ───────────────
  状态: 已同步 ✓
  ───────────────
  🔄 同步到服务器版本    ← 快速同步
  🔧 修复工具            ← 打开修复 GUI
  📦 管理备份            ← 切换窗口到备份管理
  ───────────────
  📤 上传问题报告        ← 上传日志
  📂 打开 .minecraft     ← 打开目录
  📖 查看文档            ← 打开 README
  ───────────────
  退出守护模式
───────────────
```

### 3.2 托盘状态图标

```go
// 用不同颜色/图标表示状态
const (
    IconNormal  = iota  // 绿色：正常
    IconSyncing         // 蓝色：正在同步
    IconWarning         // 黄色：有更新可用
    IconError           // 红色：需要修复
)

func updateTrayIcon(status int) {
    // 根据状态更换图标
    // 实际实现用系统托盘库（如 getlantern/systray）
    switch status {
    case IconNormal:
        setIcon(greenIcon)
    case IconSyncing:
        setIcon(blueIcon)
        setTooltip("正在同步...")
    case IconWarning:
        setIcon(yellowIcon)
        setTooltip("有新版本可用")
    case IconError:
        setIcon(redIcon)
        setTooltip("需要修复")
    }
}
```

---

## 四、修复入口与命令行

### 4.1 命令行更新

```bash
# 已有命令
starter repair              ← 交互式修复 GUI（默认）
starter repair --clean      ← 全量修复，不询问
starter repair --mods-only  ← 只修模组

# 新增
starter repair --gui        ← 以 TUI 模式启动修复界面（同无参）
starter repair --select     ← 选择性修复（列出文件让用户选）
starter repair --sync-only  ← 仅同步到服务器版本（不删文件）

starter backup              ← 备份管理
starter backup list         ← 列出备份
starter backup restore <id> ← 恢复指定备份
starter backup delete <id>  ← 删除指定备份
```

### 4.2 完整命令行树

```
starter
├── run              ← 全流程：同步→启动→守护
├── sync             ← 仅同步
├── repair           ← 交互式修复 GUI
│   ├── --clean      ← 全量修复（自动执行）
│   ├── --sync-only  ← 仅同步到服务器版本
│   ├── --select     ← 选择性修复
│   └── --gui        ← 同无参
├── backup
│   ├── list         ← 列出备份
│   ├── restore      ← 恢复备份
│   ├── create       ← 手动创建备份
│   └── delete       ← 删除备份
├── pcl
│   ├── detect       ← 检测 PCL2
│   └── path         ← 设置 PCL2 路径
├── check            ← 环境检查
├── init             ← 初始化
├── version          ← 版本号
└── self-update      ← 自更新
```

### 4.3 "一键修复"快捷方式

在 PCL2 目录下放一个 `修复.bat`：

```batch
@echo off
echo MC-Starter 修复工具
start "" /wait "%~dp0starter.exe" repair
pause
```

用户也可以把这个 .bat 发送到桌面快捷方式，双击就进修复界面。

---

## 五、Windows 弹窗（轻量替代 TUI）

如果不想依赖 bubbletea（增加二进制体积），也可以用 Windows 原生 MessageBox：

```go
// Windows 原生弹窗实现
// 使用 syscall 调用 user32.dll 的 MessageBoxW
//
// 优点：不需要额外依赖，最终二进制自包含
// 缺点：只能点按钮，不能做复杂交互

func showCrashDialog(crash CrashInfo) int {
    title := "MC-Starter - Minecraft 崩溃了"
    message := fmt.Sprintf("Minecraft 异常退出（退出码 %d）\n\n", crash.ExitCode)
    
    if crash.Reason != "" {
        message += "可能原因：" + crash.Reason + "\n\n"
    }
    
    message += "是否执行自动修复？\n修复将：\n"
    message += "  • 备份当前 mods 和 config\n"
    message += "  • 恢复为服务器纯净版本\n"
    message += "  • 重新启动\n"
    message += "\n若不修复，可手动运行 starter repair"
    
    // 显示消息框
    // result = MessageBox(hWnd, message, title, MB_YESNO | MB_ICONERROR)
    // 
    // 返回:
    //   IDYES (6) = 执行修复
    //   IDNO  (7) = 不修复
    
    return MessageBox(hWnd, message, title, MB_YESNO)
}
```

**推荐优先级**：
1. 有终端时 → TUI（更丰富的交互）
2. 无终端时（如从 PCL2 启动）→ Windows 原生弹窗
3. headless 模式 → 自动选择 + 日志输出

---

## 六、进度条与反馈

### 6.1 终端进度条

```go
// 下载进度条示例
func showDownloadProgress(current, total int64, filename string) {
    pct := float64(current) / float64(total) * 100
    bar := renderProgressBar(pct, 30)
    fmt.Printf("\r  %s [%s] %5.1f%%", filename, bar, pct)
}

func renderProgressBar(pct float64, width int) string {
    filled := int(pct / 100 * float64(width))
    bar := strings.Repeat("█", filled)
    bar += strings.Repeat("░", width-filled)
    return bar
}
```

输出效果：
```
  sodium-fabric-0.5.3.jar [██████████████████░░░░░░░░]  62.5%
```

### 6.2 多文件总进度

```
  [步骤 3/5] 下载模组  (12/15)  80%
    ┌─ sodium-fabric-0.5.3.jar     [████████████████████] 100%
    ├─ lithium-fabric-0.11.2.jar   [██████████░░░░░░░░░░]  45%
    ├─ iris-fabric-1.6.4.jar       [░░░░░░░░░░░░░░░░░░░░]   0%
    ├─ ...
    └─ 预计剩余: 30 秒
```

---

## 七、与 PCL2 的交互

### 7.1 修复后自动刷新 PCL2

修复完成后，如果 PCL2 已经在运行：
```go
// 通知 PCL2 刷新版本列表
func notifyPCL2Refresh() {
    // PCL2 可以通过更新 PCL.ini 的 VersionCache 时间来触发刷新
    pclIni := LoadPCLIni(pclIniPath)
    pclIni.Set("VersionCache", strconv.FormatInt(time.Now().Unix(), 10))
    pclIni.Save(pclIniPath)
    
    // 或者直接通知 PCL2 窗口
    // 通过 FindWindowW 找到 PCL2 窗口句柄
    // 发送 WM_USER 消息通知刷新
}
```

### 7.2 在 PCL2 中添加修复入口

可以在 PCL2 目录下放一个 `启动 MC-Starter 修复.vbs`：

```vbs
' 双击运行，不显示命令行窗口
CreateObject("WScript.Shell").Run "starter.exe repair", 0, True
MsgBox "修复完成！请刷新 PCL2 版本列表。", vbInformation, "MC-Starter"
```

---

## 八、WBS 补充

| ID | 任务 | 预估 | 前置 |
|---|---|---|---|
| P2.12 | 修复 TUI 界面：bubbletea 布局 + 选项交互 | 4h | P2.7 |
| P2.13 | 托盘菜单中添加入口：同步/修复/备份 | 2h | P2.9 |
| P2.14 | Windows 原生弹窗兜底（无终端时） | 2h | P2.8 |
| P2.15 | 修复后 PCL2 自动刷新 | 1h | P2.12, P4.3 |
