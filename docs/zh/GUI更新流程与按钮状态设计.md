# GUI 更新按钮流程与状态设计

## 一、按钮布局（三个功能按钮）

```
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ 📥 安装      │  │ 🔧 修复      │  │ 📂 打开启动器│
│ 🔄 更新      │  │              │  │              │
│ ✅ 已最新    │  │              │  │              │
└──────────────┘  └──────────────┘  └──────────────┘
```

| 按钮 | 行为 | 备注 |
|------|------|------|
| 安装/更新/已最新 | 装新版本 / 拉增量 | 文案随状态变 |
| 🔧 修复 | 强制全量覆盖，补缺失/损坏文件 | 始终可用 |
| 📂 打开启动器 | 启动 PCL2/HMCL | 需已安装且配置了启动器路径 |

### 安装/更新按钮状态规则

| 本地状态 | 按钮文案 | 按钮可用 | 行为 |
|---------|---------|---------|------|
| 未安装（localVersion 为空或"(未安装)"） | 「📥 安装」 | ✅ 可点击 | 全量下载 |
| 已安装 + 落后服务端版本 | 「🔄 更新」 | ✅ 可点击 | 增量/全量更新 |
| 已安装 + 已是最新 | 「✅ 已最新」 | ❌ 禁用 | — |

### 实现要点

- 安装/更新按钮文案随 `refreshUI()` 动态更新
- 回调统一走 `startSync()`，不区分安装/更新
- `startSync()` 内部调用 `UpdatePack()`，由服务端返回的 `mode` 自动决定全量/增量

## 二、用户点击按钮后的完整流程

```
用户点击按钮
└─→ GUI.startSync()
    │
    ├─→ 1. 锁定 UI（禁用按钮，显示进度条）
    │
    ├─→ 2. 创建 Updater 实例
    │      updater := launcher.NewUpdater(cfgDir, mcDir, cfg)
    │
    ├─→ 3. 获取包状态（本地版本号，频道）
    │      packState := localCfg.Packs[selectedPack]
    │
    ├─→ 4. 核心：UpdatePack(serverURL, packName, packState, forceFull=false)
    │      │
    │      ├── 4a. config.FetchUpdate() → 服务端返回增量清单
    │      │     ├── update.Mode == "full" → 全量下载+解压
    │      │     └── update.Mode == "incremental" → 增删改差异文件
    │      │
    │      └── 4b. 返回 UpdateResult → 统计 Added/Updated/Deleted
    │
    ├─→ 5. EnsureVersion(mcVersion, loader) → 装 MC 本体 + Fabric/Forge
    │      │
    │      ├── 下载 client.jar
    │      ├── 下载 libraries
    │      ├── 安装 Fabric（如有）
    │      └── 写入 version.json
    │
    ├─→ 6. 刷新 PCL2 版本列表（可选）
    │      launcher.RefreshPCLVersions(mcDir)
    │
    └─→ 7. 保存 + 刷新 UI
           ├── localCfg.Packs[packName].LocalVersion = 新版本号
           ├── cfg.SaveLocal()
           ├── refreshUI() → 按钮变"✅ 已最新"
           └── 解锁 UI → 弹窗提示结果
```

### 修复按钮流程

```
用户点击「🔧 修复」
└─→ GUI.startRepair()
    │
    ├─→ 1. 确认弹窗："将全量覆盖差异文件，继续？"
    │
    ├─→ 2. 创建 Updater 实例
    │
    ├─→ 3. UpdatePack(serverURL, packName, packState, forceFull=true)
    │      └─ forceFull=true → 跳过增量判断，直接全量覆盖
    │
    ├─→ 4. EnsureVersion() → 重新校验 MC + Loader
    │
    ├─→ 5. 刷新 PCL2
    │
    └─→ 6. 提示结果
```

## 三、当前 GUI 需要改的部分

### 文件：`internal/gui/app.go`

1. **`refreshUI()`** 中更新 `updateBtn` 文案：
   - 未安装 → "📥 安装"
   - 有更新 → "🔄 更新"
   - 已最新 → "✅ 已最新"（禁用）

2. **新增修复按钮 `repairBtn`**：
   - 文案固定 "🔧 修复"
   - 默认可用
   - 回调 `startRepair()` → 调 `UpdatePack(forceFull=true)`

3. **`startSync()`** 中替换假进度条：
   - 当前：`time.Sleep` 模拟进度 + 直接写版本号
   - 改为：调用 `updater.UpdatePack()` + `updater.EnsureVersion()`
   - 需要：从 `localCfg` 获取 `mcDir`、`serverURL`、包版本

4. **进度反馈**：
   - `UpdatePack()` 和 `EnsureVersion()` 没有内置进度回调
   - 可后续考虑加进度通道，当前至少显示"正在同步..."文字

## 四、依赖关系

调用链涉及的模块和它们的位置：

| 步骤 | 方法 | 文件 |
|------|------|------|
| 获取包列表 | `cfg.FetchPacks()` | `internal/config/config.go` |
| 创建更新器 | `launcher.NewUpdater()` | `internal/launcher/update.go` |
| 增量/全量更新 | `updater.UpdatePack()` | `internal/launcher/update.go` |
| 安装 MC+Loader | `updater.EnsureVersion()` | `internal/launcher/update.go` |
| 下载文件 | `cfg.DownloadFile()` | `internal/config/config.go` |
| 文件缓存 | `cache.Get() / cache.Put()` | `internal/launcher/cache.go` |
| 快照记录 | `updatePackSnapshot()` | `internal/launcher/update.go` |
| Fabric 安装 | `installer.Install()` | `internal/launcher/fabric.go` |
| PCL2 刷新 | `launcher.RefreshPCLVersions()` | `internal/launcher/pcl_refresh.go` |
| 配置保存 | `cfg.SaveLocal()` | `internal/config/config.go` |

---

*文档生成：2026-05-01 · 最后更新：2026-05-01（添加修复按钮）*

