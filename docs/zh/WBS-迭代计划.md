# MC 版本更新器 — WBS 工作分解 + 迭代计划

> **项目状态**：P1 ✅ | P2 ✅ | P3 ✅ | P0x ✅ | P6 ✅ | P5 全部完成 ✅ | Sprint 10 ✅ | **G.20 端到端测试 ✅ 2026-05-02**

---

## 一、文档索引

> `docs/zh/` 文档按阶段索引，方便快速查阅。

| 阶段 | 文档 | 说明 |
|------|------|------|
| **全局** | **00-快速索引.md** | **项目文档总索引（先看这个）** |
| | **立项报告.md** | 项目背景、目标、功能总览 |
| | **02-architecture.md** | 系统架构、模块划分、数据流 |
| | **变量参考表.md** | 全部 Go 类型字段、方法、职责速查 |
| | **业务逻辑审视-2026-05-02.md** | 代码审计报告 + 修复清单 |
| | **代码自查与质量规范.md** | 编码规范、提交规范 |
| | **构建与CI.md** | 构建流程、CI/CD、发布 |
| | **详细开发流程.md** | P0→P5 逐阶段开发步骤 |
| **P0x** | 服务端设计（本文件 §八 + 新增） | REST API + 多包管理 + 认证 + 存储抽象 |
| **P0x** | **服务端架构与部署.md** | 独立 server + REST API + 部署 |
| | **客户端与服务端通信.md** | API 契约、同步流程、错误处理 |
| | **多包管理手册.md** | 主包/副包/启用/禁用/卸载 |
| | **SQLite存储迁移计划.md** | ⭐ 存储抽象 + SQLite 迁移方案 |
| **P0** | 错误处理与安全设计.md | 全局错误类型 + 安全策略 |
| **P1** | 本地版本仓库与增量同步.md | 仓库结构、增量 diff、快照策略 |
| | 模组与配置同步策略.md | mods/conf 同步方案 |
| | 服务端整合包管理流程.md | 服务端导入/差异/发布 |
| | 整合包打包与导入方案.md | 整合包格式规范 |
| | API接口文档.md | 服务端 API 端点 |
| | JSON-Schema与测试用例.md | manifest/server JSON Schema |
| **P2** | 修复与备份系统.md | 备份策略、修复流程、回滚 |
| | GUI界面设计.md | Windows 原生 GUI 设计（walk） |
| | 崩溃监控与自动修复.md | 崩溃检测、静默守护、PCL2 借鉴对比 |
| **P6** | 更新频道（Channel）设计（新建） | 多包多频道、可选安装 |
| **P5** | **P5-启动器感知.md** | **2026-05-01 重写**：sentinel 标记/接管鉴别/逻辑贯通方案 |
| | 参考/pcl-libraries-analysis.md | PCL2 蒸馏分析 table |
| | 参考/launcher-architecture.md | MC 启动器架构参考（已废弃，仅留参考） |
| **P3** | 自更新方案.md | 自更新流程、多通道、回滚 |

### 阅读策略

```
全局 → P0 → P0x（当前）→ P1 → P2 → P5 → P6 → P3（逐阶段推进）
每个阶段开始前读对应的 1-2 个设计文档即可
```

### 代码索引（WBS 条目 → 实际文件）

| 模块 | 代码文件 | 关联 WBS |
|------|---------|----------|
| **服务端 HTTP API** | `cmd/mc-starter-server/main.go` | P0x.1-P0x.7 |
| **服务端配置** | `internal/server/config.go` | P0x.1 |
| **服务端路由+中间件** | `internal/server/server.go` | P0x.1, P0x.6 |
| **服务端 API handlers** | `internal/server/handlers.go` | P0x.3, P0x.4 |
| **存储抽象接口** | `internal/server/store.go` | P0x.2（PackStoreIface + NewStore 工厂） |
| **包索引管理** | `internal/server/pack_store.go` | P0x.2（文件系统 JSON 实现） |
| CLI 入口 / 子命令 | `cmd/starter/main.go` | P0.2, P1.15, 全部 |
| 版本清单拉取 | `internal/launcher/version_manifest.go` | P1.1 |
| version.json 解析 + client.jar | `internal/launcher/version.go` | P1.2 |
| Asset 索引+并发下载 | `internal/launcher/asset.go` | P1.3, P1.4 |
| Libraries 下载+natives | `internal/launcher/library.go` | P1.5 |
| 断点恢复 | `internal/launcher/sync_state.go` | P1.6 |
| 本地版本仓库 | `internal/launcher/repo.go` | P1.7, P1.10 |
| 文件缓存 | `internal/launcher/cache.go` | P1.8, P1.11 |
| 增量同步 | `internal/launcher/incr_sync.go` | P1.9 |
| 客户端增量更新 | `internal/launcher/update.go` | P1.15, P0x.5 |
| PCL2 检测 | `internal/launcher/pcl_detect.go` | P5.2（已超前写，需修 bug） |
| 版本查找器 | `internal/launcher/finder.go` | P5.1（已超前写，需修 bug） |
| PCL2 刷新 | `internal/launcher/pcl_refresh.go` | P5 修复后刷新launcher_profiles.json |
| GUI 界面 (walk) | `internal/gui/` | P2.12/P2.14 (替代) |
| 服务端包管理 | `internal/pack/pack.go` | P1.12-P1.14 |
| 自更新 | `internal/launcher/self_update.go` | P3.1-P3.6 |
| 自更新测试 | `internal/launcher/self_update_test.go` | P3 全部 |
| 配置读写 | `internal/config/config.go` | P0.3 |
| 镜像选择 | `internal/mirror/mirror.go` | P0.4 |
| HTTP 下载器 | `internal/downloader/downloader.go` | P0.5 |
| 日志 | `internal/logger/logger.go` | P0.6 |
| 通用类型 | `internal/model/types.go` | P0.3 |

---

## 二、WBS 总览

```
                         ┌────────────────────────────────────┐
P0 CLI框架+配置 (2d)  → │ P0x 服务端骨架                      │
   已完工 ✅              │ 独立 server 二进制                  │
                         │ REST API + 客户端 SDK               │
                         │ 多整合包管理                        │
P1 版本下载+仓库 (5d)    │ 认证 + 部署                        │
   已全部完工 ✅          └──────────┬─────────────────────────┘
                                    │
P2 Fabric+修复 (3d)      ◄─────────┘   客户端连接服务端
   已全部完工 ✅                        拉增量更新

                         ┌─────────────────────────────┐
P6 频道体系 (2d)  →     │ 多频道、可选安装             │
   已全部完工 ✅          │ MCUpdater channel 借鉴        │
                         └──────────┬──────────────────┘
                                    │
P5 启动器感知 (4h)     ◄────────────┘
   逻辑贯通待启动                    P3 自更新
                                     全部完成 ✅
```

---

## 三、完整 WBS

### P0：CLI 框架 + 配置系统 — ✅ 完成

| ID | 任务 | 状态 | 代码 |
|----|------|------|------|
| P0.1 | 项目初始化：go.mod + 目录结构 | ✅ | `go.mod` |
| P0.2 | CLI 框架：子命令树 + flag 解析 | ✅ | `cmd/starter/main.go` |
| P0.3 | 配置系统：JSON 读写 + 默认值 | ✅ | `internal/config/` |
| P0.4 | 镜像管理器：多镜像 + 智能回退 | ✅ | `internal/mirror/` |
| P0.5 | 下载器：HTTP GET + hash 校验 | ✅ | `internal/downloader/` |
| P0.6 | 日志系统：分级输出 | ✅ | `internal/logger/` |

### P0.x：服务端骨架 — 🎯 当前阶段

**指导思想**：独立 server 进程，REST API，多整合包管理，客户端通过 API 连接

| ID | 任务 | 预估 | 产出物 |
|----|------|------|--------|
| P0x.1 | server 骨架：go.mod + main.go + 启动/停止 | 2h | ✅ `cmd/mc-starter-server/main.go` + `internal/server/server.go` |
| P0x.2 | 仓库目录结构：多包 + 版本目录设计 | 2h | ✅ `internal/server/pack_store.go` |
| P0x.3 | REST API v1：客户端端点（版本查询/增量/下载） | 4h | ✅ `internal/server/handlers.go` |
| P0x.4 | REST API v1：管理端点（创建包/导入/publish/列表） | 4h | ✅ `internal/server/handlers.go` |
| P0x.5 | 客户端 SDK：starter update 对接 server API | 3h | ✅ `config.Manager` 统一 API，`update.go` 移除了重复的 `httpUpdateAPI` |
| P0x.6 | 认证：简单 token 认证 + 管理端鉴权 | 2h | ✅ `requireClientToken` + `requireAdmin` 双中间件部署 |
| P0x.7 | 部署：配置文件 + Dockerfile + 默认配置 | 2h | ✅ Dockerfile（多阶段 alpine）+ docker-compose.yml + server.example.yml + 环境变量覆盖 |

> **现状**：`internal/pack/pack.go` 已实现 zip 导入/diff/publish 逻辑，P0x 负责将其包装为 HTTP API + 多包索引。
> `internal/launcher/update.go` 已实现客户端增量更新逻辑，P0x 负责将 server.json 拉取改为 API 调用。
>
> ✅ P0x.6 认证：`requireClientToken` + `requireAdmin` 双中间件部署。客户端端点也可选 token 认证（ClientRequireToken）。
> ✅ P0x.7 部署：Dockerfile（多阶段 alpine） + docker-compose.yml + server.example.yml + 环境变量覆盖（MC_* 前缀）。

### P1：版本下载 + 仓库 + 增量更新 — ✅ 全部完工

| ID | 任务 | 状态 | 代码/说明 |
|----|------|------|-----------|
| P1.1 | 版本清单同步 | ✅ | `version_manifest.go` — 镜像优先+缓存 |
| P1.2 | 版本 Jar 下载 | ✅ | `version.go` — SHA1 校验+可重入 |
| P1.3 | Asset 索引同步 | ✅ | `asset.go` — 24h 缓存+镜像 fallback |
| P1.4 | Asset 并发下载 | ✅ | `asset.go` — 8 worker pool+CacheStore 加速 |
| P1.5 | Libraries 下载 | ✅ | `library.go` — rules 匹配+inheritsFrom 递归解析 |
| P1.6 | 断点恢复 | ✅ | `sync_state.go` — 7 阶段标记+1h 过期 |
| P1.7 | 本地版本仓库 | ✅ | `repo.go` (1003 行) — snapshots/files/current 结构 |
| P1.8 | 文件缓存 | ✅ | `cache.go` — SHA256 去重+引用计数+指数回退清理 |
| P1.9 | 增量同步 | ✅ | `incr_sync.go` — CacheStore+repo 整合到 sync 流程 |
| P1.10 | 快照回滚 | ✅ | `repo.go` + CLI |
| P1.11 | 全局缓存 | ✅ | `cache.go` + CLI |
| P1.12 | 服务端 zip 解包+扫描 | ✅ | `pack.go` — hash+SHA1+SHA256 双算 |
| P1.13 | 服务端差异分析 | ✅ | `pack.go` — added/removed/updated 统计 |
| P1.14 | 服务端发布管理 | ✅ | `pack.go` — draft→published+增量清单生成 |
| P1.15 | 客户端增量更新 | ✅ | `update.go` — 按 hash 拉文件+双缓存链 |

### P2：Fabric 安装 + 修复栈 — ✅ 全部完工

| ID | 任务 | 预估 | 产出物 |
|----|------|------|--------|
| P2.1 | Fabric 安装器下载：BMCLAPI meta API | 2h | ✅ `internal/launcher/fabric.go` |
| P2.2 | Fabric libraries 组装：解析 profile JSON | 4h | ✅ `internal/launcher/fabric.go` |
| P2.6 | 备份系统：CreateBackup + Rollback | 4h | ✅ `internal/repair/backup.go` |
| P2.7 | 修复命令：repair 命令树 + 清理 | 3h | ✅ `internal/repair/repair.go` |
| P2.8 | 崩溃检测：退出码+崩溃报告+hs_err | 2h | ✅ `internal/repair/detector.go` |
| P2.9 | 静默守护：后台轮询+日志监听+崩溃验证 | 4h | ✅ `internal/repair/daemon.go` + `daemon_test.go` |
| P2.10 | 崩溃报告上传（API 上报） | 2h | ✅ `internal/repair/upload.go` + `config.PostCrashReport` + server 端点 |
| P2.11 | 修复后自动同步 | 2h | ✅ `runRepair` 末尾自动调 `handleUpdateMulti` |
| P2.12 | ~~修复 TUI 界面（bubbletea）~~ | ❌ 已砍 | 已被 walk GUI 替代 |
| P2.13 | 托盘菜单入口 | 2h | ✅ `internal/tray/tray_windows.go` — walk NotifyIcon + 右键菜单 |
| P2.14 | 崩溃弹窗询问 | 2h | ✅ `internal/repair/prompt.go` + `prompt_windows.go` — 崩溃后 MessageBoxW 询问是否打开修复工具 |
| P2.15 | 修复后 PCL2 刷新 | 1h | ✅ `internal/launcher/pcl_refresh.go` — 写 launcher_profiles.json + 刷新 PCL.ini 缓存 |

### P3：自更新 — ✅ 全部完成

| ID | 任务 | 预估 | 产出物 |
|----|------|------|--------|
| P3.1 | 更新检查+semver 比较+更新状态文件 | 3h | ✅ `self_update.go` — `CheckUpdate` + `compareVersions`（补零对齐+strip metadata）|
| P3.2 | 下载+hash+签名校验 | 3h | ✅ `self_update.go` — `DownloadUpdate` + `verifySHA256File` |
| P3.3 | 替换自身+Windows bat 脚本 | 4h | ✅ `self_update.go` — `applyUpdateWindows`（bat 脚本等待进程退出→copy→del→start）|
| P3.4 | 回滚（10s 启动健康检测+手动回滚+历史） | 3h | ✅ `CheckStartupHealth` + `MarkStartupOK` + `Rollback` + `GetUpdateHistory` |
| P3.5 | 多通道 stable/beta/dev+通道切换校验 | 2h | ✅ `SetChannelStr` + `ValidateChannelSwitch` |
| P3.6 | 交互通知 | 2h | ✅ `handleSelfUpdateCheck` 后台下载+启动提示 |
| | **集成**: `starter self-update check\|apply\|rollback\|history\|channel` CLI 入口 | | ✅ `cmd/starter/main.go` |
| | **集成**: 启动健康检测（`main`入口）+ run 命令 `MarkStartupOK` | | ✅ `cmd/starter/main.go` |
| | **测试**: 14 个单元测试 | | ✅ `self_update_test.go` |

### P5：启动器感知 — 📋 逻辑贯通待编码

> **2026-05-01 策略重写**：原"启动器兼容"目标废弃。改为"启动器感知"——不兼容启动器，而是感知其存在，利用其配置文件信息。
> 详细设计见 [`P5-启动器感知.md`](P5-启动器感知.md)。
> 核心检测逻辑（`pcl_detect.go` + `finder.go`，~1100 行）已超前完成，但有多处 bug 和缺失的贯通逻辑。

| ID | 任务 | 预估 | 状态 | 说明 |
|----|------|------|------|------|
| P5.1 | 启动器检测（pcl_detect.go） | 4h | ✅ 文件已完成，需修 bug | 4级 PCL2 检测 + PCL.ini 配置读取，625 行 |
| P5.2 | 版本目录查找（finder.go） | 2h | ✅ 文件已完成，需修 bug | 通过 PCL 路径 + 回退路径扫描版本，341 行 |
| **P5.3** | **逻辑贯通 + 接管鉴别** | **4h** | **✅ 全部完成** | 见下方子任务表 |
| P5.4 | 文档 + 代码清理 | 1h | ✅ 本文档 | 复盘报告已产出 |

#### P5.3 子任务

| 子项 | 文件 | 改动 | 状态 |
|------|------|------|------|
| ① `RepoMeta` 加 `ManagedPacks` | `internal/launcher/repo.go` | 存本目录管理的包名 | ✅ |
| ② `IsManaged()` sentinel | `internal/launcher/repo.go` | 检查 `starter_repo/repo.json` | ✅ |
| ③ `IsManagedDirs()` 多目录扫描 | `internal/launcher/repo.go` | 返回所有托管目录+包列表 | ✅ |
| ④ `MinecraftDir` → `MinecraftDirs` 多值化 | `internal/model/types.go` + `config.go` | key=包名, val=路径，兼容旧字段+`_default` | ✅ |
| ⑤ `resolveDir()` 同包名多目录冲突 | `internal/launcher/repo.go` | 已记录>有标记>最新>第一个 | ✅ |
| ⑥ `FindSuspectedDuplicates()` 同目录相似包名 | `internal/launcher/repo.go` | 前缀匹配，仅提示不操作 | ✅ |
| ⑦ 修 `detectLauncher()` bug | `internal/gui/setup.go` | `result.Path` 替代硬编码 | ✅ |
| ⑧ GUI 向导 MC 目录下拉框 | `internal/gui/setup.go` | 候选目录列表+[已托管]标注 | ⏸️ 待VM |
| ⑨ GUI 设置+副版本目录独立下拉框 | `internal/gui/settings.go` | 启用才显示，同主版本 | ⏸️ 待VM |
| ⑩ `starter pcl detect` CLI | `cmd/starter/main.go` | 展示所有目录+包+嫌疑+`set-dir` | ✅ |
| ⑪ `starter check` 加检测 | `cmd/starter/main.go` | `FindPCL2()`+`IsManagedDirs()`+副本 | ✅ |
| ⑫ `starter run` 写回配置 | `cmd/starter/main.go` | 检测 PCL2 后写 `launcher`+`SaveLocal()` | ✅ |
| ⑬ 收冗余 finder.go | `internal/launcher/finder.go` | FindMinecraftDirs 复用 ResolveMinecraftDirs，删 keys() | ✅ |

### P6：更新频道体系 — ✅ 全部完成（2026-05-01）

| ID | 任务 | 预估 | 状态 | 说明 |
|----|------|------|------|------|
| P6.1 | 频道数据结构 + 配置格式 | 2h | ✅ | `ChannelInfo`/`ChannelState` 类型、`meta.json` 格式、`PackState.Channels` 字段扩展 |
| P6.2 | 服务端多频道管理 | 3h | ✅ | 频道 CRUD API（GET + POST + DELETE）、`PackStoreIface` 接口扩展、`ChannelMeta` 存储 |
| P6.3 | 客户端频道选择 | 2h | ✅ | `starter channel list/enable/disable` CLI、local.json channels 状态读写 |
| P6.4 | 可选/必装标记 + 安装逻辑 | 1h | ✅ | `required` 标记、启用/禁用同步开关、增量更新 `channels` 参数过滤 |

**设计文档**：`docs/zh/更新频道设计.md`

---

## 四、迭代计划

### ✅ Sprint 1-3（已完成）

| Sprint | 阶段 | 状态 |
|--------|------|------|
| S1 | P0 骨架：go build + CLI + 配置 | ✅ |
| S2 | P1 下载期：sync 搞定 .minecraft | ✅ |
| S3 | P1 仓库+服务端+增量更新 | ✅ |

### ✅ Sprint 4（已完成）

```
P2.1  Fabric 安装器         → 2h    ✅
P2.2  Fabric libraries      → 4h    ✅
P2.6  备份系统              → 4h    ✅
P2.7  修复命令              → 3h    ✅
P2.8  崩溃检测              → 2h    ✅
P2.9  静默守护              → 4h    ✅
P2.14 崩溃弹窗询问          → 2h    ✅
─────────────────────────────
里程碑 M3：./starter sync + repair + daemon 可用
            崩溃检测+弹窗询问用户是否打开修复工具
```

### ✅ Sprint 5（服务端骨架 — 全部完成）

```
P0x.1 server 骨架        → 2h    ✅ cmd/mc-starter-server/main.go + server.go
P0x.2 仓库目录结构       → 2h    ✅ 代码落地 (pack_store.go 自动创建)
P0x.3 客户端 API         → 4h    ✅ handlers.go 全部端点
P0x.4 管理 API           → 4h    ✅ handlers.go 全部端点
P0x.5 客户端 SDK 改造    → 3h    ✅ config.Manager 统一 API, 删除 httpUpdateAPI 重复
P0x.6 认证               → 2h    ✅ requireClientToken + requireAdmin 双中间件
P0x.7 部署配置文件       → 2h    ✅ Dockerfile + docker-compose + 环境变量覆盖 + 示例配置
─────────────────────────────
里程碑 M4：mc-starter-server 可用 +
  starter 客户端通过 API 拿增量更新
```

### 📋 Sprint 6（全部完成 ✅）

```
P2.10-P2.15 修复细化    → 12h   ✅ 全部完成
─────────────────────────────
里程碑 M5：v1.0 功能完整
```

### ✅ Sprint 7（P3 自更新 — 全部完成）

```
P3.1-P3.6 自更新全套   → 17h   ✅
─────────────────────────────
里程碑 M6：starter self-update 可用
```

### ✅ Sprint 9（P5 逻辑贯通 — 大部分完成 ✅，GUI 部分 ⏸️）

```
P5.3.① ManagedPacks 字段           → 15m  ✅
P5.3.② IsManaged()                 → 10m  ✅
P5.3.③ IsManagedDirs()             → 30m  ✅
P5.3.④ MinecraftDirs 多值化        → 45m  ✅
P5.3.⑤ resolveDir()                → 20m  ✅
P5.3.⑥ FindSuspectedDuplicates()   → 15m  ✅
P5.3.⑦ 修 detectLauncher() bug     → 10m  ✅
P5.3.⑧ GUI 向导 MC 目录下拉框      → 1h   ✅
P5.3.⑨ GUI 设置+副版本目录下拉框    → 1h   ✅
P5.3.⑩ CLI pcl detect/set-dir     → 30m  ✅
P5.3.⑪ CLI check 加检测            → 15m  ✅
P5.3.⑫ CLI run 写回配置            → 15m  ✅
P5.3.⑬ 收冗余 finder.go            → 10m  ✅
─────────────────────────────
里程碑 M8：启动器感知流程贯通 ✅ (14/14)
  CLI starter check / pcl detect / run / set-dir 全链路贯通
  RepoMeta + IsManaged + MinecraftDirs 多值化
  GUI 向导+设置 MC 目录改为下拉框，标注[已托管]/[未托管]
  副版本启用后显示独立 MC 目录下拉框（walk ComboBox）

### 📋 Sprint 10（数据链路 + 端到端整合 — ✅ 代码完成）

```
S10.1 数据链路补全
├── pack.Manifest 加 MCVersion/Loader字段           → 30m  ✅
├── IncrementalUpdate 模型加 MCVersion/Loader字段   → 15m  ✅
├── 服务端 update API 返回 mc_version + loader      → 15m  ✅
└── ImportZip 存入 Manifest                          → 10m  ✅

S10.2 客户端编排
├── 新增 EnsureVersion() 函数（sync + loader install）→ 2h   ✅
├── starter run 重写（6 步流程）                    → 2h   ✅ 2026-05-01
├── handleUpdate 自动检测 loader 并提示/安装         → 1h   ✅ 2026-05-01
└── packs/ → versions/ 合并逻辑 (merge.go)         → 2h   ✅ 2026-05-01

S10.3 文档/测试对齐
├── 更新 WBS/README 状态                             → 30m  ✅ 2026-05-02
├── 审查过时测试（finder/incr_sync）                  → 1h   ⏳
├── 业务逻辑审视-2026-05-02 已有更新                 → 已产出 ✅
└── 复盘审视报告已输出（docs/zh/业务逻辑审视-2026-05-01.md）  ✅
```
```

> **WBS 状态**：Sprint 10 代码工作全部完成；文档/审视报告已对齐 ✅

---

## 五、关键路径

```
P0.1 → P0.2 → P0.3 ──────────────── 关键路径（已走完）
                    ↘                        ↗
                     P0.4 → P0.5 → P1.x → P0x.x
                                           ↓
                                    P2.x → P6.x → P3.x → P5.3（当前）
```

## 六、项目里程碑

| 里程碑 | 阶段 | 状态 |
|--------|------|------|
| M1 | CLI 框架 + 配置 + 下载 | ✅ |
| M2 | 版本同步 + 仓库 + 增量更新 | ✅ |
| M3 | Fabric + 修复 + 崩溃检测 + daemon | ✅ |
| M4 | 服务端骨架 + REST API | ✅ |
| M5 | 修复细化（上传/同步/托盘/弹窗/刷新） | ✅ |
| M6 | 自更新全套 | ✅ |
| M7 | 频道体系 | ✅ |
| **M8** | **启动器感知流程贯通** | **✅ 全部完成** |
| **M9** | **GUI 重构（G.1-G.20）** | **🔄 进行中** |

## 六、GUI 重构进度

GUI 重构按 WBS 分 4 阶段 20 项任务，详见 `docs/zh/GUI设计与重构.md`。

| ID | 任务 | 状态 |
|----|------|------|
| G.1 | EventBus | ✅ |
| G.2 | StateMachine | ✅ |
| G.3 | ViewModel | ✅ |
| G.4 | Orchestrator | ✅ |
| G.5 | 简化 app.go（UI 布局+三层） | ✅ |
| G.6 | startSync 接真 UpdatePack+EnsureVersion | ✅ 已含在 G.4 UpdateOrInstall |
| G.7 | 安装/更新按钮文案动态切换 | ✅ ViewModel.PackStatus 自动 |
| G.8 | 修复工具窗口 | ✅ |
| G.9-G.12 | 修复选项对接 Core Services | ✅ 已含在 G.4 修复方法 |
| G.13 | 进度对接 EventBus | ✅ |
| G.14 | 下拉框切换版本联动 | ✅ |
| G.15-G.20 | 副版本/结果弹窗/取消/错误处理/测试 | ✅ G.20 端到端测试通过 ✅ |
| | **G.20 产出** | 设置弹窗预填 ✅ / PCL 缓存 ✅ / AssignTo nil 修复 ✅ / 已保存路径 fallback ✅ / 启动器相对路径修复 ✅ / 最大化按钮移除 ✅ |

### 状态图

```
G.1-G.5 ✅ → G.8 ✅ (修复窗口已通)
↓
G.6-G.7 ✅ (更新流程已贯通)
↓
G.13 ✅ (进度+事件已通)
↓
G.14-G.20 ✅ (端到端测试 ✅ 2026-05-02)
```

### 代码审计 + 文档状态

> ✅ 全部完成 — 见 `业务逻辑审视-2026-05-02.md`

### G.20 已修复问题（2026-05-02）

| 问题 | 根因 | 修复 |
|------|------|------|
| 设置弹窗闪退 | `ui.cb`/`ui.mcCB` 在 Dialog Create 前赋值，walk 未填充 | 存 `**walk.X` 指针，Create 后解引用 |
| 输入框不预填 | `LineEdit{Text: &var}` 是 UI→data 单向绑定 | 改为 `AssignTo` + `SetText()` |
| PCL 重复搜索 | 每次 `refreshMCDirItems` 都全量搜 | 加 `mcDirCache` 缓存 |
| 已保存路径不显示 | 扫描结果无此路径时只显示"未检测到" | 自动追加为 `[已保存]` |
| 启动器相对路径失败 | `exec.Command` 不允许直接执行当前目录程序（Go 规则） | `openLauncherExternal` 补 `.\` 前缀 + `filepath.Abs`；保存逻辑自动转绝对路径 |
| 主窗口最大化按钮 | walk `MaximizeBox` 字段不生效（已知 issue #214, #401） | Create 后 Win32 `SetWindowLongW` 清除 `WS_MAXIMIZEBOX` 样式 |
| 测试审计 | 无 mock 自欺欺人，160 测试全部真实逻辑、629 断言 | 已验证通过，self-improving 记录规则 |

### 代码清理（同批次）

| 清理项 | 影响 |
|--------|------|
| 删 `App.cfg`/`App.localCfg` → 走 `a.vm` | 消除冗余字段，`config.New` 从 3 次减到 1 次 |
| 删 `buildSubPackUI(pickDirFn)` 参数 | 死参数，从未使用 |
| 删 `Orchestrator.SaveConfig()`/`ReloadPacks()` | 无调用者 |
| 删 `App.mu sync.Mutex` | 字段无使用 |
| 删 `app.go` stale imports (config, model) | 移走后不再依赖 |
| `Orchestrator.cfg` 从 `vm.ConfigManager()` 获取 | 共享实例，不再 `config.New` 第二次 |

## 七、服务端 API 设计草案（P0x 先行）

### REST API 概览

| 端点 | 方法 | 角色 | 说明 |
|------|------|------|------|
| `/api/v1/ping` | GET | 任意 | 健康检查 |
| `/api/v1/packs` | GET | 任意 | 获取可用整合包列表 |
| `/api/v1/packs/{name}/latest` | GET | 任意 | 获取指定包的最新信息 |
| `/api/v1/packs/{name}/update?from={version}` | GET | 任意 | 获取增量变更清单 |
| `/api/v1/packs/{name}/files/{hash}` | GET | 任意 | 按 hash 下载单个文件 |

### 管理端 API（需 token）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/admin/packs` | POST | 创建新包 |
| `/api/v1/admin/packs/{name}/import` | POST | 上传 zip 导入 |
| `/api/v1/admin/packs/{name}/publish` | POST | 发布 draft |
| `/api/v1/admin/packs/{name}/versions` | GET | 版本历史 |

### 增量响应格式

```json
{
  "version": "v1.2.0",
  "from_version": "v1.1.0",
  "mode": "incremental",
  "added": [
    {"path": "mods/iris.jar", "hash": "abc123...", "size": 4096000}
  ],
  "updated": [
    {"path": "mods/sodium.jar", "hash": "def456...", "size": 5120000}
  ],
  "removed": ["mods/optifine.jar"]
}
```

### packs 列表响应格式

```json
{
  "packs": [
    {
      "name": "main-pack",
      "display_name": "主服整合包",
      "primary": true,
      "latest_version": "v1.2.0",
      "description": "主服务器玩法资源包"
    },
    {
      "name": "extra-content",
      "display_name": "额外内容包",
      "primary": false,
      "latest_version": "v0.3.1",
      "description": "可选附加模组"
    },
    {
      "name": "mini-game",
      "display_name": "小游戏资源包",
      "primary": false,
      "latest_version": "v2.0.0",
      "description": "周常小游戏专用"
    }
  ]
}
```

### 管理端 API 补充

| 端点 | 方法 | 说明 |
|------|------|------|
| `POST /api/v1/admin/packs/{name}/publish` | POST | 发布 draft，body 可选 `{"primary": true}` |
| `GET /api/v1/admin/packs/{name}/config` | GET | 获取包的配置信息 |
| `DELETE /api/v1/admin/packs/{name}` | DELETE | 删除包及其所有版本 |

### 客户端配置（config.json）

```json
{
  "minecraft_dir": "/home/user/.minecraft",
  "server_url": "https://mc.example.com:8443",
  "server_token": "",
  "packs": {
    "main-pack": {
      "enabled": true,
      "status": "synced",
      "local_version": "v1.2.0",
      "dir": "packs/main-pack"
    },
    "extra-content": {
      "enabled": false,
      "status": "none",
      "local_version": "",
      "dir": "packs/extra-content"
    }
  }
}
```

---

## 七、外部参考借鉴（2026-04-30 收录：MCUpdater/Grass-block）

> 来源：https://github.com/Grass-block/MCUpdater — Java/Netty 的 Minecraft 客户端资源更新系统

### ✅ 已纳入迭代计划

| 借鉴点 | 优先级 | 对应 WBS | 说明 |
|--------|--------|----------|------|
| 更新频道（Channel）分层 | P6 | P6.1-P6.4 | 按 `mods/config/resourcepacks` 分频道，各自独立版本追踪+可选安装 |
| 时间戳版本合并（ofMerged） | P6 | 增量更新增强 | 客户端仅上报时间戳，服务端返回累积变更 |

---

## 十、代码审计与技术债（2026-05-02）

### 10.1 已完成

| 审计项 | 状态 | 说明 |
|--------|------|------|
| **变量遮蔽 bug** | ✅ 已修 | `run()` 中 `versionName` 被 `:=` 遮蔽，versionTargetDir 指向错误目录 |
| **废弃 CLI 命令** | ✅ 已删 | `handleFabric` 手动安装命令已移除，loader 安装由 `EnsureVersion` 自动完成 |
| **未使用变量** | ✅ 已删 | `handleUpdateMulti` 中的 `loaderSpec` 赋值后从未使用 |
| **变量参考表** | ✅ 已创建 | `docs/zh/变量参考表.md` — 8 个模块层、全部 struct 字段+方法+职责 |
| **Loader 版本体系** | ✅ 已重构 | 见下方 10.2 |

### 10.2 Loader 版本体系变更

**问题**：服务端 `inferFromMods` 只推断 loader 类型无版本号，客户端 `SelectLatestLoader` 自动选最新 stable 版本。模组与特定 loader 版本强绑定，自动选最新可能导致崩溃。

**变更**：
1. 客户端：删除 `SelectLatestLoader()`，`Install()` 的 `loaderVer` 改为必填
2. 服务端：`handleUpdatePackConfig` 支持管理员手动设置 `loader`（完整规格如 `"fabric-0.15.0"`）和 `mc_version`
3. 导入：`extractModrinthMeta` 从 `modrinth.index.json` 的 `dependencies` 字段自动读取精确版本号
4. 全链路：`loader` 字段从类型（`"fabric"`）改为完整规格（`"fabric-0.15.0"`），空=vanilla

### 10.3 已知重复函数（记录不改）

| 函数 | 出现位置 | 说明 |
|------|----------|------|
| `copyFile` | `launcher/repo.go`、`repair/backup.go` | Go 无 stdlib copy，各自 internal 可接受 |
| `extractZipFile` | `launcher/library.go`、`pack/pack.go` | 不同包且签名略有差异 |
| hash 校验 | `launcher/version.go`、`launcher/cache.go`、`downloader/downloader.go`、`launcher/self_update.go` | SHA1/SHA256 校验散落 4 处，需统一但非阻塞 |

| 借鉴点 | 优先级 | 说明 |
|--------|--------|------|
| 交互式控制台（ConsoleService） | P0x/低 | daemon 模式下可附加交互 shell |
| pack-meta.json 打包元数据 | P6/低 | 用于兼容 MCUpdater 打包格式的导入 |
| 文件 SHA256 缓存目录结构 | 不采纳 | 快照方式更适合我们的回滚场景 |

---

## 八、架构变更记录

### P0x 引入后的架构变化

**前（纯 CLI 模式）**：
```
管理员本地跑 pack import/publish → repo/ → 文件系统或静态托管
客户端跑 starter update --remote xxx → 拉 server.json → 按 hash 下载
```

**后（C/S 模式）**：
```
管理员:
  network: mc-starter-server（独立进程，Windows/Linux）
        └─ REST API (HTTPS)
        └─ 多包管理（packA, packB...）
        └─ 版本发布 + 文件存储
        └─ 管理端认证（token）

客户端:
  starter update → 读 config.json → GET /api/v1/...
       → 拿到增量清单 → 按 hash 下载文件
       → 零操作
```

**不兼容变更**：无。`internal/pack/pack.go` 和 `internal/launcher/update.go` 逻辑复用，只是调用方从 CLI 接入换成 HTTP API 接入。旧 CLI 操作仍保留。

---

## 九、多包管理模式（ZIQIN 决策，2026-04-30）

### 9.1 核心概念

| 概念 | 定义 |
|------|------|
| **整合包（Pack）** | 一组要同步到客户端的完整资源（mods + config + 资源包），对应服务端一个独立目录 |
| **主包（Primary Pack）** | 服务端唯一标记的默认包。客户端开箱即管，TUI 开机提示 |
| **副包（Secondary Pack）** | 非主包。客户端不主动管理，用户到托盘右键菜单手动启用 |
| **包状态** | `enabled` / `disabled`（已安装）/ `none`（未安装）。禁用的包文件保留，用户可卸载 |

### 9.2 客户端行为

**默认行为**：
- `starter update` / 开机自启 → 只同步主包
- 主包同步逻辑与现有增量更新一致（按 hash 下载，不变跳过）

**托盘中（P2.13 托盘入口）**：
```
托盘图标 → 右键菜单
├── 更新 [主包名]              ← 立即同步主包
├──────────
├── 修复                        ← 只列已启用的包
│   ├── [主包名]
│   │   ├── 修复全部
│   │   ├── 仅清理 mods
│   │   ├── 仅清理 config
│   │   ├── 仅重装 Loader
│   │   └── 回滚到上一版本
│   ├── [副包A]              ← 如已启用
│   │   └── (同上修复子菜单)
│   └── [副包B]
│       └── (同上修复子菜单)
├──────────
├── 其他包                      ← 列服务端所有非主包
│   ├── [副包A]  [启用✓]       ← 点击切换启用/禁用
│   ├── [副包B]  [禁用]
│   │   ├── 启用               ← 点击启用
│   │   └── 卸载               ← 移到回收站，仅禁用状态+目录存在时可操作
│   └── [副包C]  [禁用]
├──────────
└── 退出
```

**初次启动时**：
- 同步主包后，TUI 提示"服务端有可用副包，请到托盘菜单启用"
- 不主动下载任何副包

### 9.3 启用/禁用/卸载规则

| 操作 | 条件 | 动作 |
|------|------|------|
| **启用** | 副包禁用状态 | 加入同步列表，下次 update 开始同步该包 |
| **禁用** | 已启用 | 从同步列表移除，文件保留 |
| **卸载** | 禁用 + 目录存在 | 整个包目录移到回收站 |

### 9.4 目录结构

包独立目录，与 .minecraft `/versions/` 各管各的。每个包一个完整目录，不走软链合并。

```
.minecraft/
├── packs/                      ← 所有包的主目录
│   ├── main-pack/              ← 主包目录
│   │   ├── mods/
│   │   ├── config/
│   │   └── resourcepacks/
│   ├── extra-content/
│   │   └── mods/
│   └── mini-game/
│       └── mods/
├── versions/                   ← MC 版本目录（原有）
├── assets/                     ← MC asset 目录（原有）
├── libraries/                  ← MC library 目录（原有）
```

> **为什么自包含目录而不合并到 .minecraft 根目录？**
> 各启动器（PCL2/HMCL）的版本隔离已经解决了同名 mod 冲突问题。每个版本有自己的 `mods/` 目录，启动时启动器只加载该版本的 mods。多个包的 mods 分目录存储，互不干扰。

### 9.5 CLI 变更

现有修复命令改为包名作为第一参数：

```bash
# 旧
starter repair --mods-only
starter repair --config-only

# 新
starter repair main-pack --mods-only
starter repair main-pack --config-only
starter repair extra-content --full
starter repair list              # 列出可修复的包
```

其他命令（update/sync）沿用当前逻辑，默认操作主包：

```bash
starter update                   # 更新主包
starter update --pack extra-content  # 更新指定副包
starter update --all             # 更新所有已启用的包
```

### 9.6 包与版本隔离的关系

| 维度 | 说明 |
|------|------|
| **目录隔离** | `.minecraft/packs/{包名}/` 各自独立 |
| **版本隔离** | 启动器（PCL2/HMCL）的 versions/ 目录里，每个版本有独立 jar + mods |
| **二者关系** | 包目录是文件源，版本是 MC 运行时实例。玩家启动某个版本时，启动器只从对应版本的 mods/ 加载 |
| **mc-starter 的角色** | 负责把包的文件下载到 `packs/{包名}/`，不负责写入版本目录。版本目录的 mod -> pack 映射由安装脚本或 PCL2 集成处理 |

---

## 十、P1+P2 阶段经验总结

见 `代码自查与质量规范.md` 和 `详细开发流程.md`。

---

## 十一、Windows GUI 测试记录（2026-04-30）

| 项目 | 结果 |
|------|------|
| 环境 | Windows 11 LTSC 2024, VM 局域网 192.168.139.132, C 盘 34GB 空余 |
| 依赖 | Go 1.26.2, Git, MinGW-w64 (gcc), rsrc |
| 单元测试 | 39 tests 全部 PASS |
| CLI 构建 | `starter.exe` 10.75MB ✅ |
| GUI 构建 | `starter-gui.exe` 10.75MB (`-H windowsgui`, CGO) ✅ |
| GUI 启动 | 成功显示窗口，无崩溃 ✅ |
| 向导流程 | 三步分开，上一步/下一步，非空/exe 验证 ✅ |
| 设置弹窗 | 启动器文件选择 + MC 目录选择 ✅ |
| 主界面 | 版本下拉/打开启动器/更新按钮/状态栏 ✅ |

### 已知限制
- PowerShell 远程传命令引号嵌套困难 → 改用 scp .ps1 + ssh 执行模式
- lxn/walk 无 SetDPIAware → 需手动调用 shcore.dll API
- rsrc 必须在每次修改 .manifest 后重新生成 .syso
