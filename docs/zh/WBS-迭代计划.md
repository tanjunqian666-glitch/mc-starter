# MC 版本更新器 — WBS 工作分解 + 迭代计划 & P1 回顾

> **项目状态**：P1 全部完成 ✅ | 当前阶段：P2 Fabric 安装 + 修复栈
> **P1 后经验总结**见文末 §六

---

## 一、文档索引

> `docs/zh/` 文档按阶段索引，方便快速查阅。

| 阶段 | 文档 | 说明 |
|------|------|------|
| **全局** | **立项报告.md** | 项目背景、目标、功能总览 |
| | **02-architecture.md** | 系统架构、模块划分、数据流 |
| | **代码自查与质量规范.md** | 编码规范、提交规范 |
| | **构建与CI.md** | 构建流程、CI/CD、发布 |
| | **详细开发流程.md** | P0→P5 逐阶段开发步骤 |
| **P0** | 错误处理与安全设计.md | 全局错误类型 + 安全策略 |
| **P1** | 本地版本仓库与增量同步.md | 仓库结构、增量 diff、快照策略 |
| | 模组与配置同步策略.md | mods/conf 同步方案 |
| | 服务端整合包管理流程.md | 服务端导入/差异/发布 |
| | 整合包打包与导入方案.md | 整合包格式规范 |
| | API接口文档.md | 服务端 API 端点 |
| | JSON-Schema与测试用例.md | manifest/server JSON Schema |
| **P2** | 修复与备份系统.md | 备份策略、修复流程、回滚 |
| | 修复工具GUI界面.md | TUI 界面设计 |
| | 崩溃监控与自动修复.md | 崩溃检测、静默守护 |
| **P4** | PCL2集成方案.md | PCL2 集成模式 |
| | 参考/pcl-libraries-analysis.md | PCL2 蒸馏分析 table |
| | 参考/launcher-architecture.md | PCL2/HMCL 结构对应 |
| **P5** | 自更新方案.md | 自更新流程、多通道、回滚 |

### 阅读策略

```
全局 → P0 → P1 → P2 → P4 → P5（逐阶段推进）
每个阶段开始前读对应的 1-2 个设计文档即可
```

### 代码索引（WBS 条目 → 实际文件）

| 模块 | 代码文件 | 关联 WBS |
|------|---------|----------|
| CLI 入口 / 子命令 | `cmd/starter/main.go` | P0.2, P1.15, 全部 |
| 版本清单拉取 | `internal/launcher/version_manifest.go` | P1.1 |
| version.json 解析 + client.jar | `internal/launcher/version.go` | P1.2 |
| Asset 索引+并发下载 | `internal/launcher/asset.go` | P1.3, P1.4 |
| Libraries 下载+natives | `internal/launcher/library.go` | P1.5 |
| 断点恢复 | `internal/launcher/sync_state.go` | P1.6 |
| 本地版本仓库 | `internal/launcher/repo.go` | P1.7, P1.10 |
| 文件缓存 | `internal/launcher/cache.go` | P1.8, P1.11 |
| 增量同步 | `internal/launcher/incr_sync.go` | P1.9 |
| 客户端增量更新 | `internal/launcher/update.go` | P1.15 |
| PCL2 检测 | `internal/launcher/pcl_detect.go` | P4.2 (已超前写) |
| 版本查找器 | `internal/launcher/finder.go` | P4.1 (已超前写) |
| TUI 全自动界面 | `internal/tui/` | P2.12 (已超前写) |
| 服务端包管理 | `internal/pack/pack.go` | P1.12-P1.14 |
| 配置读写 | `internal/config/config.go` | P0.3 |
| 镜像选择 | `internal/mirror/mirror.go` | P0.4 |
| HTTP 下载器 | `internal/downloader/downloader.go` | P0.5 |
| 日志 | `internal/logger/logger.go` | P0.6 |
| 通用类型 | `internal/model/types.go` | P0.3 |

---

## 二、WBS 总览

```
P0 CLI框架+配置 (2d)  →  P1 版本下载+仓库+更新 (5d)  →  P2 Loader+修复 (3d)
   已完工 ✅                 已全部完工 ✅                   ⬅ 当前阶段
                              ├─ P1.1-P1.6 基础下载        ├─ P2.1 Fabric 安装器
                              ├─ P1.7-P1.11 仓库+缓存      ├─ P2.2 Fabric libraries
                              ├─ P1.12-P1.14 服务端        ├─ P2.6 备份系统
                              └─ P1.15 客户端更新          ├─ P2.7 修复命令
                                                           ├─ P2.8 崩溃检测
P3 Java检测 (1d)      →  P4 启动器兼容 (3d)               ├─ P2.9 静默守护
   ⬅ 待启动              ⬅ 待启动                          ├─ P2.10 报告上传
                                                           ├─ P2.11 修复后同步
P5 自更新 (2d)                                            ├─ P2.12 TUI界面
   ⬅ 待启动                                                ├─ P2.13 托盘入口
                                                           └─ P2.14-P2.15 细化
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
| **P1.15** | **客户端增量更新** | **✅** | **`update.go` — 按 hash 拉文件+双缓存链** |

### P2：Fabric 安装 + 修复栈 — 📋 待启动

| ID | 任务 | 预估 | 产出物 |
|----|------|------|--------|
| P2.1 | Fabric 安装器下载：BMCLAPI meta API | 2h | `internal/launcher/fabric.go` |
| P2.2 | Fabric libraries 组装：解析 profile JSON | 4h | `internal/launcher/fabric.go` |
| P2.6 | 备份系统：CreateBackup + Rollback | 4h | `internal/repair/backup.go` |
| P2.7 | 修复命令：repair 命令树 + 清理 | 3h | `internal/repair/repair.go` |
| P2.8 | 崩溃检测：退出码+崩溃报告+hs_err | 2h | `internal/repair/detector.go` |
| P2.9 | 静默守护：后台轮询+日志监听+托盘 | 4h | `internal/daemon/daemon.go` |
| P2.10 | 崩溃报告上传 | 2h | `internal/repair/upload.go` |
| P2.11 | 修复后自动同步 | 2h | `internal/repair/run.go` |
| P2.12 | 修复 TUI 界面（bubbletea） | 4h | `internal/repair/tui.go` |
| P2.13 | 托盘菜单入口 | 2h | `internal/daemon/tray.go` |
| P2.14 | Windows 弹窗兜底（无终端） | 2h | `internal/repair/dialog.go` |
| P2.15 | 修复后 PCL2 刷新 | 1h | `internal/repair/pcl.go` |

### P3：Java 环境检测 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P3.1 | 路径检测：JAVA_HOME/PATH/注册表 | 3h |
| P3.2 | 版本校验：java -version 解析 | 2h |
| P3.3 | 引导安装提示 | 1h |

### P4：启动器兼容 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P4.1 | PCL2/裸启动模式 | 3h* |
| P4.2 | PCL2 集成：ini 读写+注册表 | 6h* |
| P4.3 | HMCL 兼容 | 2h |
| P4.4 | 官方启动器兼容 | 1h |

> *P4.1 版本查找器 `finder.go` 和 P4.2 PCL2 检测 `pcl_detect.go` 已超前完成。

### P5：自更新 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P5.1 | 更新检查+semver 比较 | 3h |
| P5.2 | 下载+hash+签名校验 | 3h |
| P5.3 | 替换自身+bat 脚本 | 4h |
| P5.4 | 回滚（10s 检测） | 3h |
| P5.5 | 多通道 stable/beta/dev | 2h |
| P5.6 | 交互通知 | 2h |

---

## 四、迭代计划

### ✅ Sprint 1-3（已完成）

| Sprint | 阶段 | 状态 |
|--------|------|------|
| S1 | P0 骨架：go build + CLI + 配置 | ✅ |
| S2 | P1 下载期：sync 搞定 .minecraft | ✅ |
| S3 | P1 仓库+服务端+增量更新 | ✅ |

### 📋 Sprint 4（当前 — Fabric 安装器 + 修复栈）

```
P2.1 Fabric 安装器      → 2h
P2.2 Fabric libraries   → 4h
P2.6 备份系统           → 4h
P2.7 修复命令           → 3h
P2.8 崩溃检测           → 2h
P2.12 TUI界面           → 4h
P2.13 托盘入口          → 2h
P2.14 弹窗兜底          → 2h
P2.15 PCL刷新           → 1h
─────────────────────────────
里程碑 M3：./starter sync 带 Fabric + repair 可用
```

### 📋 Sprint 5（Java 检测 + 启动器兼容）

```
P3.1 Java 路径检测       → 3h
P3.2 版本校验            → 2h
P3.3 引导提示            → 1h
P4.1 PCL2 兼容           → 3h
P4.2 HMCL 兼容           → 2h
P4.3 官方启动器兼容      → 1h
─────────────────────────────
里程碑 M4：完整启动器体验
```

### 📋 Sprint 6（自更新 + 收尾）

```
P5.1 更新检查            → 2h
P5.2 自身替换            → 3h
P5.3 回滚                → 1h
P5.4 多通道              → 1.5h
QA  手动测试+README      → 6h
─────────────────────────────
里程碑 M5：v1.0 发布
```

---

## 五、关键路径

```
P0.1 → P0.2 → P0.3 ────────────────────────────── 关键路径（已走完）
                    ↘                      ↗
                     P0.4 → P0.5 → P1.x → P2.x
                                 ↗
                     P0.6───────┘
```

## 六、P1 阶段经验总结

### 做得好的 👍

1. **模块化设计** — sync 流程拆成 version/asset/library 三个独立 manager，各自负责自己的阶段，`main.go` 只做编排。后期加增量同步（P1.9）和客户端更新（P1.15）都是独立文件，零侵入。

2. **两阶段分离（解析/下载）** — PCL 蒸馏学来的 `ResolveLibrary → DownloadFiles` 模式，测试和 debug 都方便。后来 cache 注入也只需要改中间层。

3. **包内测试先行** — `launcher` 包全部测试 0.09s 跑完，重构时敢改敢动。

4. **双缓存链设计** — `CacheStore`（全局 dedup）+ repo `files/`（快照引用）组合，下载过的文件跨版本复用，实测高收益。

5. **代码审查修了 5 个 bug** — 包括 `cache.Clean` 误删全部 meta 这种潜伏 bug，审查机制到位。

### 踩过的坑 💀

1. **`downloader.File` 的 `expectedHash` 参数是设计陷阱** — 参数名是 hash 但不说明是 SHA256 还是 SHA1，注释自己都写了"注意"。Minecraft 生态文件多数用 SHA1，这个参数实际传 ""，调用方自行校验。**教训**：要么明确算法，要么移除 hash 参数。

2. **Win symlink 需要管理员权限** — `updateCurrentSymlink` 的 `os.Symlink` 在 Windows 上大概率失败，现有 `logger.Debug` 静默忽略是对的，但没有对应的替代方案（junction / copy）。

3. **`mirror.go` 探测超时写死 3s** — 和构造函数 `threshold (4s)` 不一致，导致 switch 逻辑跑偏。probe 超时应取自 threshold 而不是硬编码。

4. **增量更新 API 设计滞后** — P1.15 实现前 service pack publish 生成的是完整清单，没有增量清单。后补的 `BuildServerUpdateInfo` 基于 repo 快照 diff 生成增量 JSON，耦合了服务端和客户端逻辑。

### 设计权衡保留的 ⚠️

- `pack.ComputeDiff` 和 `launcher.ComputeDiff` 同名不同参 → 不同包，阅读时注意区分
- `pack.UpdatedEntry` 声明未使用 → 等 P2 修复流程用到再删
- CacheStore.Clean 删 entry 后不维护 RefCounts 残留条目 → 改为只删已删除 hash
- `IncrementalSync.CacheStore()` 暴露 `*CacheStore` 指针 → 当前无并发风险，后续做只读接口

### 后续可复用模式 📐

1. **"解析/下载分离"** 用在 Fabric 安装器（P2.1-P2.2）：先解析 meta JSON 得到 LibraryFile 列表，再用已有 DownloadFiles 批量下载
2. **`sync_state.go` 的原子写入 + 断点** 直接复用到 P2.9 守护进程
3. **`CacheStore` 的引用计数** 用于 P5 自更新中旧版本的缓存保留策略
