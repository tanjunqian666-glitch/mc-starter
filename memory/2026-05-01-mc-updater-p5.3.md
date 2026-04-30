# 2026-05-01 — P5.3 启动器感知逻辑贯通

**人物**: ZIQIN（Discord）
**项目**: mc-starter (github.com/gege-tlph/mc-starter)
**时间**: 01:49 — 02:15 (Asia/Shanghai)

## P5.3 完成（10/14 项）

### 新增功能
1. **RepoMeta.ManagedPacks** — 仓库元信息记录管理的包名列表
2. **IsManaged(mcDir)** — sentinel 检测（检查 starter_repo/repo.json 是否存在）
3. **IsManagedDirs(candidates)** — 多目录扫描，返回已托管/未托管列表
4. **MinecraftDirs 多值化** — `LocalConfig.MinecraftDirs map[string]string` key=包名 val=路径，旧 MinecraftDir 读取时自动迁移到 `_default`，保存时清空旧字段
5. **ResolveDir()** — 同包名多副本冲突：已记录 > 标记存在包目录 > 最新 > 路径最短
6. **FindSuspectedDuplicates()** — 前缀匹配疑似副本，仅提示不操作
7. **`starter pcl detect`** — 检测启动器+MC目录+当前配置，支持序号选择
8. **`starter pcl set-dir <包名> <序号>`** — 为指定包选择 MC 目录并写入配置
9. **`starter check` 增强** — 加 PCL2 检测、IsManagedDirs、FindSuspectedDuplicates
10. **`starter run` 写 PCL2 配置** — 检测到 PCL2 后写 localCfg.Launcher + SaveLocal
11. **修 detectLauncher() bug** — `result.Path` 替代硬编码 `result.PCLDir + "PCL2.exe"`
12. **config.go 适配** — LoadLocal 自动迁移旧 MinecraftDir，SaveLocal 只写 MinecraftDirs

### 顺便修了：tray_windows.go walk API 不兼容
- Go 1.26.2 + walk@v0.0.0-20210112085537 新版 API 变动
- `walk.NewNotifyIcon()` → `walk.NewNotifyIcon(form)` 需要 Form 参数
- `walk.NewAction(text, func)` → `walk.NewAction()` + SetText + Triggered().Attach
- `walk.NewMenuAction(menu, text)` → `walk.NewMenuAction(menu)` + SetText
- `walk.Run`/`walk.Executable` 已移除 → 用 `os.Executable()` + `exec.Command`
- `walk.NotifyIconMessageButton` 不存在 → 用 `MouseDown().Attach(func(x,y,button))`
- `ShowCustom(title, msg)` → `ShowCustom(title, msg, icon)` 需传 Image

### VM 验证
- Windows VM 编译 `starter.exe` 16MB 成功
- `starter check` 正常运行（PCL2 检测、版本清单拉取）
- 旧构建产物已清理（删除 starter.exe/starter_new.exe/starter_test_build.exe/mc-starter-server.exe）

### 学习点
- **win_exec.py 的 cmd 模式**对后台命令（go clean -cache + go build 组合）有编码问题，需分两次执行或用 PowerShell -Command 包裹
- **PowerShell 远程执行**：空格/引号/参数括号容易出问题，`os/exec` 比 PowerShell `Start-Process` 更可靠
- **Go 1.26 的 walk API 兼容性**：lxn/walk 2021年的版本在 Go 1.26 上有大量 API 不兼容，NewAction/NewMenuAction/NewNotifyIcon/Run/Executable/ShowCustom 全改
- **win@v0.0.0-20210218163916** 删了 `MakeIntResource` 和 `IDI_APPLICATION`，不能用 `win.LoadIcon` 设置图标
