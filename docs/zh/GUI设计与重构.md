# GUI 设计与重构

> 2026-05-02 · v4 · 整合 UI 设计 + 重构计划 + WBS
>
> **变更说明**: v4 将修复工具窗口、崩溃日志上传、备份恢复标记为"后续迭代"（v1 暂不包含），
> 主窗口移除 🔧 修复按钮。详见 §十四「后续迭代计划」。

---

## 一、设计哲学

mc-starter 的 GUI 定位是 **小工具**，不是大应用：

- **固定小窗口** ~420×250，不可缩放
- **双击即用**：自动检测配置，无配置则弹出向导
- **隐藏复杂度**：用户不需要知道 Fabric/Forge/内存/备份/缓存
- **Windows 原生**：基于 lxn/walk，单 exe 发布
- **DPI 感知**：高分辨率屏幕自动缩放字体、控件不模糊

---

## 二、主窗口布局

```
┌─ MC Starter ────────────────────────[⚙]─┐
│                                            │
│  版本: [主整合包 v1.2.0   ▼]              │
│                                            │
│  ┌──────────────┐ ┌──────────────────────┐│
│  │ 📥 安装      │ │ 📂 打开启动器       ││
│  │ 🔄 更新      │ │                      ││
│  │ ✅ 已最新    │ │                      ││
│  └──────────────┘ └──────────────────────┘│
│                                            │
│  当前版本: v1.2.0    最新版本: v1.3.0       │
│  有可用更新                                │
└────────────────────────────────────────────┘
```

### 版本下拉框行为

- 下拉框列举服务端返回的所有版本
- **副版本只有被启用后才出现在下拉框中**（通过设置页的勾选控制）
- 下拉框选中项切换时，下方版本信息和按钮状态跟随变化
- 下拉项格式：`显示名 v版本号`（如"主整合包 v1.2.0"）

### 控件说明

| 控件 | 作用 |
|------|------|
| ⚙ | 打开设置弹窗 |
| 版本下拉 | 显示可用的版本列表，切换时更新下方状态 |
| 安装/更新/已最新 | 文案随状态变，详见按钮状态规则 |
| 📂 打开启动器 | 需已安装且配置了启动器路径 |
| 版本信息栏 | 格式：`当前版本: x  最新版本: y` |
| 状态文字 | 就绪/有可用更新/已是最新/正在同步... |

### 按钮状态规则

| 本地状态 | 按钮文案 | 可用 | 行为 |
|---------|---------|------|------|
| 未安装（localVersion 为空或"(未安装)"） | 「📥 安装」 | ✅ | 全量下载 |
| 已安装 + 落后服务端版本 | 「🔄 更新」 | ✅ | 增量/全量更新 |
| 已安装 + 已是最新 | 「✅ 已最新」 | ❌ 禁用 | — |
| 同步中 | 跟随任务阶段变化 | ❌ 禁用 | — |

### 状态流转

```
无版本信息     → "就绪"（绿色）
版本未安装     → "未安装，请点击安装"（橙色）
已安装有更新   → "有可用更新"（橙色）
已安装无更新   → "已是最新"（绿色）
同步中         → "正在检查版本..." / "正在下载文件..." / "正在安装加载器..."（橙色）
同步取消       → "已取消"（橙色）
同步出错       → "更新失败: xxx"（红色）
```

---

## 三、修复工具窗口（延后至后续迭代）

> ⏳ 本节（修复窗口 UI、修复流程、崩溃日志上传、备份恢复）计划在 v2 中实现。
> 当前 v1 仅提供安装/更新/版本切换功能。

### 设计参考（v2 实施时用）

修复工具窗口设计如下：

```
┌─────── 修复工具 ─────────────────────────┐
│                                            │
│  全量修复                                  │
│  只保留 saves/ 和 screenshots/，           │
│  清理后从服务端重新下载                    │
│                                            │
│  [🔧 执行全量修复]                        │
│                                            │
│  ─────────────────────────────────────     │
│                                            │
│  MC 本体修复                              │
│  重新安装 Minecraft + Fabric/Forge        │
│                                            │
│  [⚡ 执行 MC 修复]                        │
│                                            │
│  ─────────────────────────────────────     │
│                                            │
│  模组同步                                  │
│  清空 mods/ 目录，从服务端拉取最新         │
│                                            │
│  [📦 执行模组同步]                        │
│                                            │
│  ─────────────────────────────────────     │
│                                            │
│  崩溃日志上传                              │
│  收集最近崩溃报告上传至服务端              │
│                                            │
│  [📤 上传崩溃日志]                        │
│                                            │
│  ─────────────────────────────────────     │
│                                            │
│  备份: [05/01/26 22:30  ▼]                │
│        （下拉框显示最近 5 个备份）          │
│  [🔄 恢复选中备份]                        │
│                                            │
└────────────────────────────────────────────┘
```

### 设计要点

- **每个功能是一个独立按钮**，不是选项+执行按钮
- **备份策略**：点任意修复按钮时弹窗询问"是否备份当前用户数据？"，选"是"则先 `repair.CreateBackup()` 再执行修复，选"否"直接执行
- **备份下拉框**：显示最近 5 个备份时间戳（mm/dd/yy hh:mm 格式）。选中后点"恢复选中备份"触发 `repair.Repair(mcDir, ActionRollback, backupID)`
- **关闭**：使用 Windows 窗口自带关闭按钮，不需要额外加

### 选项与现有代码覆盖

| 功能按钮 | 行为 | 现有代码 | 状态 |
|---------|------|---------|------|
| 🔧 执行全量修复 | 弹窗询问是否备份 → 可选 `repair.CreateBackup(mcDir, ReasonRepair)` → `repair.Repair(mcDir, ActionCleanAll)` 清理 mods/config/resourcepacks/shaders → 提示用户回主界面点安装 | `internal/repair/repair.go` Repair() + backup.go | ✅ 可用 |
| ⚡ 执行 MC 修复 | `updater.EnsureVersion()` 重新走一遍 MC 本体+Loader 安装 | `internal/launcher/update.go` EnsureVersion() | ✅ 可用 |
| 📦 执行模组同步 | 清理 mods/ → `updater.UpdatePack(forceFull=true)` 全量覆盖 | `internal/launcher/update.go` UpdatePack() | ✅ 可用（forceFull） |
| 📤 上传崩溃日志 | 扫描 .minecraft/crash-reports/ → 调用 `repair.CollectAndUpload()` | `internal/repair/upload.go` | ✅ 可用 |
| 🔄 恢复选中备份 | 下拉框选中旧备份 → 点此按钮触发 `repair.Repair(mcDir, ActionRollback, backupID)` | `internal/repair/repair.go` rollback() | ✅ 可用 |

> ⏳ 以上表格中的所有功能代码已写就，相应的 GUI 入口（`repair_window.go`）也已完成，
> 但**主窗口的 🔧 入口按钮和修复工具的 GUI 调用暂未暴露给用户**，计划在 v2 中激活。

---

## 四、设置窗口

```
┌─────── 设置 ─────────────────────────┐
│                                        │
│  设置                                  │
│                                        │  
│  服务器 API:                           │
│  [https://mc.example.com/api      ]   │
│                                        │
│  启动器路径:                           │
│  [C:\PCL\PCL2.exe           ] [🔍][📁]│
│                                        │
│  Minecraft 根目录:                     │
│  [C:\MC\.minecraft           ] [🔍][📁]│
│                                        │
│  ┌ 副版本 ──────────────────┐         │
│  │ ☑ OptiFine (v2.0.1)      │         │
│  │   MC 目录 (副): [C:\... ▼] [🔍]    │
│  │ ☐ 原版纯净 (v1.0.0)      │         │
│  └────────────────────────────┘         │
│                                        │
│              [保存]  [取消]             │
└────────────────────────────────────────┘
```

---

## 五、首次配置向导

首次双击弹出 3 步向导，**每次只显示当前步骤**，支持上一步回退：

| 步骤 | 内容 | 验证 | 自动行为 |
|------|------|------|---------|
| 1 | 填写服务器 API 地址 | 非空校验 | — |
| 2 | 启动器路径 | 必须是 .exe 文件（可跳过） | 自动搜索 PCL2/HMCL |
| 3 | Minecraft 根目录 | 目录必须存在 | 自动搜索常见位置 |
| 完成 | 保存配置 + 拉取服务端版本列表 | — | 回到主窗口 |

---

## 六、用户操作流程

### 6.1 首次安装流程

```
首次启动
└─→ 弹出配置向导
    ├─→ 步骤1: 填写 API 地址
    ├─→ 步骤2: 自动检测/手动选启动器
    ├─→ 步骤3: 自动检测/手动选 MC 目录
    └─→ 完成 → 回到主界面

主界面
└─→ 版本下拉显示服务端版本列表
└─→ 按钮显示「📥 安装」
└─→ 用户点击安装
    └─→ startSync()
        ├─→ UpdatePack(forceFull=true)  全量下载
        ├─→ EnsureVersion()             装 MC+Loader
        └─→ 保存版本 → 刷新 UI → 按钮变「✅ 已最新」
```

### 6.2 日常更新流程

```
用户日常打开
└─→ 版本下拉显示服务端版本列表
└─→ 有更新 → 按钮显示「🔄 更新」
└─→ 用户点击更新
    └─→ startSync()
        ├─→ FetchUpdate() → 服务端返回增量清单
        ├─→ applyIncremental() → 只下载差异文件
        ├─→ EnsureVersion() → 校验 MC+Loader
        └─→ 保存版本 → 刷新 UI → 按钮变「✅ 已最新」
```

### 6.3 修复流程（延后至 v2）

> ⏳ 当前 v1 暂不提供修复入口。以下流程设计已写好代码（`repair_window.go` + orchestrator 对接），
> 待 v2 在主窗口添加 🔧 按钮后激活。

```
用户发现游戏异常
└─→ 点击「🔧 修复」
    └─→ 弹出修复工具窗口
        ├─→ 选择修复项（单选）
        │   ├─ 全量修复 → 备份 → 清理 → 提示用户点更新
        │   ├─ MC本体修复 → EnsureVersion()
        │   ├─ 模组同步 → 清 mods → UpdatePack(forceFull)
        │   └─ 崩溃日志上传 → CollectAndUpload()
        └─→ 执行 → 显示结果

另：崩溃检测到后，静默守护自动弹出修复建议
```

### 6.4 版本间切换

```
用户在下拉框切换版本（如主版本→副版本）
└─→ onPackSelected()
    ├─→ 更新 selectedPack
    ├─→ 更新 latestVersion
    ├─→ refreshUI()
    │   ├─ currentVersion = localCfg.Packs[新包名].LocalVersion
    │   ├─ 按钮文案：安装/更新/已最新（自动跟随）
    │   └─ 版本信息栏更新
    └─→ 用户可对该版本执行安装/更新/修复/启动
```

---

## 七、重构计划

### 7.1 现状问题

| 问题 | 表现 | 原因 |
|------|------|------|
| UI 与业务逻辑耦合 | `app.go` 混着 walk 控件、状态管理、假的更新逻辑 | 没有分层 |
| 更新流程是假的 | `startSync()` 只有进度条动画 | GUI 与 launcher 包未对接 |
| 状态管理散乱 | `syncing bool` 一个标识控全部 | 无状态机 |
| 无进度回调 | `UpdatePack()` 无进度接口 | 模块间无事件通道 |

### 7.2 目标架构

```
┌───────────┐     ┌────────────┐     ┌──────────────┐     ┌───────────────┐
│  Walk GUI  │ ──→ │ ViewModel  │ ──→ │ Orchestrator │ ──→ │ Core Services │
│  (app.go)  │     │ (状态+绑定) │     │ (调度)       │     │ (launcher/)   │
└───────────┘     └────────────┘     └──────────────┘     └───────────────┘
                        │                    │
                        ▼                    ▼
                  EventBus ←────────── ProgressEvent
                                              LogEvent
                                              ErrorEvent
                                              StateEvent
```

### 7.3 各层职责

| 层 | 职责 | 文件 |
|----|------|------|
| **GUI Layer** | Walk 控件定义、UI 布局、按钮回调转发到 ViewModel | `internal/gui/app.go` |
| **ViewModel** | 应用状态（版本号、进度、状态）、线程安全的 UI 刷新、命令绑定 | `internal/gui/viewmodel.go` |
| **Orchestrator** | Update/Repair/Launch 流程的调度逻辑，emit 事件 | `internal/gui/orchestrator.go` |
| **EventBus** | 模块间事件传递：进度、日志、错误、状态变更 | `internal/gui/eventbus.go` |
| **StateMachine** | 状态定义和转换规则，防止 UI 状态不一致 | `internal/gui/state.go` |
| **Core Services** | 已有的 launcher 包保持不变 | `internal/launcher/` |

### 7.4 事件总线设计

```go
type EventType int

const (
    EvtProgress EventType = iota
    EvtLog
    EvtError
    EvtStateChange
)

type Event struct {
    Type  EventType
    Data  interface{}
}

type ProgressData struct {
    Percent int
    Phase   string  // "检查更新" / "下载文件" / "安装Fabric"
}

type LogData struct {
    Level   string
    Message string
}
```

### 7.5 状态机设计

```go
type AppState int

const (
    StateIdle AppState = iota
    StateChecking          // 检查版本
    StateDownloading       // 下载文件
    StateInstalling        // 安装 Fabric/Forge
    StateDone              // 完成
    StateError             // 出错
    StateCancelled         // 用户取消
)
```

允许的状态转换：

```
Idle ──→ Checking ──→ Downloading ──→ Installing ──→ Done
  │                    │                 │
  └────→ Error ←───────┴─────────────────┘
  │
  └────→ Cancelled
```

---

## 八、WBS — 执行顺序

### 阶段 1：基础设施（先在 CLI 验证，再接 GUI）

| ID | 任务 | 依赖 | 涉及文件 | 预计工时 | 状态 |
|----|------|------|---------|---------|------|
| G.1 | 建 EventBus | — | `internal/gui/eventbus.go` | 1h | ✅ |
| G.2 | 建 StateMachine | — | `internal/gui/state.go` | 1h | ✅ |
| G.3 | 从 `App` 中抽 ViewModel | G.1, G.2 | `internal/gui/viewmodel.go` | 2h | ✅ |
| G.4 | 建 Orchestrator | G.3 | `internal/gui/orchestrator.go` | 2h | ✅ |
| G.5 | 简化 `app.go`，只留 UI 布局 | G.4 | `internal/gui/app.go` | 2h | ✅ |

### 阶段 2：真更新流程接上

| ID | 任务 | 依赖 | 涉及文件 | 预计工时 | 状态 |
|----|------|------|---------|---------|------|
| G.6 | startSync 改调 UpdatePack+EnsureVersion | G.5 | `orchestrator.go` | 2h | ✅ (已含在 G.4 UpdateOrInstall) |
| G.7 | 安装/更新按钮文案动态切换 | G.6 | `viewmodel.go`, `app.go` | 1h | ✅ (ViewModel.PackStatus 自动) |
| G.8 | 修复工具窗口（4 选项+互斥+进度+备份恢复） | G.5 | `internal/gui/repair_window.go` | 3h | ✅ 代码就绪，主入口延至 v2 |
| G.9 | 修复选项对接 repair.Repair | G.8 | `orchestrator.go` | 2h | ✅ 代码就绪，延至 v2 |
| G.10 | 修复选项对接 EnsureVersion | G.8 | `orchestrator.go` | 1h | ✅ 代码就绪，延至 v2 |
| G.11 | 修复选项对接 UpdatePack(forceFull) | G.8 | `orchestrator.go` | 1h | ✅ 代码就绪，延至 v2 |
| G.12 | 崩溃日志上传入口 | G.8 | `orchestrator.go` | 1h | ✅ 代码就绪，延至 v2 |
| G.13 | 进度条对接 EventBus | G.6 | `viewmodel.go` | 1h | ✅ |

### 阶段 3：多版本切换 + 副版本

| ID | 任务 | 依赖 | 涉及文件 | 预计工时 | 状态 |
|----|------|------|---------|---------|------|
| G.14 | 下拉框切换版本时 UI 联动 | G.6 | `app.go`, `viewmodel.go` | 1h | ✅ |
| G.15 | 副版本启用时才显示在下拉框 | G.14 | `app.go`, `settings.go` | 1h | ✅ |
| G.16 | 副版本独立 MC 目录 | — | 已有 `settings.go` | ✅ 已有 |

### 阶段 4：收尾

| ID | 任务 | 依赖 | 涉及文件 | 预计工时 | 状态 |
|----|------|------|---------|---------|------|
| G.17 | 更新结果弹窗 | G.6 | `orchestrator.go` | 0.5h | ✅ (EventBus SyncDone) |
| G.18 | 取消同步/回滚 | G.6 | `orchestrator.go` | 1h | ✅ (Orchestrator.Cancel) |
| G.19 | 错误处理和重试 | G.6 | `orchestrator.go` | 1h | ✅ (EventBus Error 事件) |
| G.20 | 端到端验收 + 文档更新 | 全部 | — | 2h | ⬜（延迟至 stash 中 BUG 修复后） |

### 总计：~24h

---

## 九、不做的事（v1 范围）

- ❌ CLI 子进程调用（`exec.Command`）——没意义
- ❌ 拆 Service 包（`update/`, `sync/`, `repair/`）——代码规模不需要
- ❌ 第三方事件总线库——一个结构体+channel 够用
- ❌ 状态机库——`type int + switch` 够用
- ❌ PCL2 版本列表手动刷新（`RefreshPCL2AfterRepair`）——PCL2 会通过 version.json 自动识别
- ❌ 卸载按钮——文件删除 = 手动删 packs/ 目录

---

## 十、文件结构（最终）

```
internal/gui/
├── app.go              ← 主窗口 + 布局（简化后）
├── viewmodel.go        ← 状态管理 + 命令绑定（新增）
├── orchestrator.go     ← 流程调度（新增）
├── eventbus.go         ← 事件总线（新增）
├── state.go            ← 状态机（新增）
├── repair_window.go    ← 修复工具窗口（新增）
├── settings.go         ← 设置弹窗（基本不变）
├── setup.go            ← 首次配置向导（基本不变）
├── dpi_windows.go      ← DPI 感知（不变）
└── gui.manifest        ← Common Controls 6.0 manifest（不变）
```

---

## 十一、依赖关系速查

| 功能 | 方法 | 文件 |
|------|------|------|
| 获取包列表 | `cfg.FetchPacks()` | `internal/config/config.go` |
| 获取包详情 | `cfg.FetchPackDetail()` | `internal/config/config.go` |
| 创建更新器 | `launcher.NewUpdater()` | `internal/launcher/update.go` |
| 增量/全量更新 | `updater.UpdatePack()` | `internal/launcher/update.go` |
| 安装 MC+Loader | `updater.EnsureVersion()` | `internal/launcher/update.go` |
| 下载文件 | `cfg.DownloadFile()` | `internal/config/config.go` |
| 文件缓存 | `cache.Get() / cache.Put()` | `internal/launcher/cache.go` |
| 快照记录 | `updatePackSnapshot()` | `internal/launcher/update.go` |
| Fabric 安装 | `installer.Install()` | `internal/launcher/fabric.go` |
| 修复（清理+备份） | `repair.Repair()` | `internal/repair/repair.go` |
| 备份 | `repair.CreateBackup()` | `internal/repair/backup.go` |
| 崩溃日志上传 | `repair.CollectAndUpload()` | `internal/repair/upload.go` |
| 配置保存 | `cfg.SaveLocal()` | `internal/config/config.go` |

---

## 十四、后续迭代（v2 计划）

以下功能代码已就绪但暂不暴露给用户，计划在 v2 中激活：

| 功能 | 当前状态 | 激活方式 |
|------|---------|---------|
| 🔧 **修复工具入口按钮** | 主窗口无该按钮 | 在 `app.go` 操作按钮行添加 PushButton，回调 `showRepairWindow(a)` |
| **全量修复** | `repair_window.go` + orchestrator 就绪 | 通过修复窗口调用 |
| **MC 本体修复** | `orchestrator.go` EnsureVersion 就绪 | 通过修复窗口调用 |
| **模组同步** | `orchestrator.go` UpdatePack(forceFull) 就绪 | 通过修复窗口调用 |
| **崩溃日志上传** | `orchestrator.go` CollectAndUpload 就绪 | 通过修复窗口调用 |
| **备份恢复** | `repair_window.go` 下拉框+恢复按钮就绪 | 通过修复窗口调用 |
| **进度条撑大窗口 BUG** | walk layout 已知限制 | 需 walk layout workaround |
| **安装按钮无响应 BUG** | `CanSync()` 状态检测不一致 | 需修复 `packStatusLocked()` |
| **安装完成不刷新 UI** | `EvtSyncDone` 处理顺序需调整 | 先 `RefreshState()` 再 `refreshUI()` |

---

*文档生成：2026-05-02 · v4 · v1 交付，修复/备份/崩溃检测延至 v2*
