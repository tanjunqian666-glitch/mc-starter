# MC 版本更新器 — WBS 工作分解 + 迭代计划

> **项目状态**：P1 全部完成 ✅ | P2 全部完成 ✅ | P5 全部完成 ✅ | **当前阶段：P0.x 服务端**
> **外部参考**见文末 §七（MCUpdater）

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
| **P0x** | 服务端设计（本文件 §八 + 新增） | REST API + 多包管理 + 认证 |
| **P0** | 错误处理与安全设计.md | 全局错误类型 + 安全策略 |
| **P1** | 本地版本仓库与增量同步.md | 仓库结构、增量 diff、快照策略 |
| | 模组与配置同步策略.md | mods/conf 同步方案 |
| | 服务端整合包管理流程.md | 服务端导入/差异/发布 |
| | 整合包打包与导入方案.md | 整合包格式规范 |
| | API接口文档.md | 服务端 API 端点 |
| | JSON-Schema与测试用例.md | manifest/server JSON Schema |
| **P2** | 修复与备份系统.md | 备份策略、修复流程、回滚 |
| | 修复工具GUI界面.md | TUI 界面设计 |
| | 崩溃监控与自动修复.md | 崩溃检测、静默守护、PCL2 借鉴对比 |
| **P6** | 更新频道（Channel）设计（新建） | 多包多频道、可选安装 |
| **P5** | PCL2集成方案.md | PCL2 集成模式 |
| | 参考/pcl-libraries-analysis.md | PCL2 蒸馏分析 table |
| | 参考/launcher-architecture.md | PCL2/HMCL 结构对应 |
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
| **多包管理器** | `internal/server/` | P0x.1-P0x.7 |
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
| PCL2 检测 | `internal/launcher/pcl_detect.go` | P5.2 (已超前写) |
| 版本查找器 | `internal/launcher/finder.go` | P5.1 (已超前写) |
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
                         ┌────────────────────────────────────┐
P0 CLI框架+配置 (2d)  → │ P0x 服务端骨架 (PRIORITY NOW)     │
   已完工 ✅              │ 独立 server 二进制                  │
                         │ REST API + 客户端 SDK               │
                         │ 多整合包管理                        │
P1 版本下载+仓库 (5d)    │ 认证 + 部署                        │
   已全部完工 ✅          └──────────┬─────────────────────────┘
                                    │
P2 Fabric+修复 (3d)      ◄─────────┘   P5 启动器兼容 (3d)
   已全部完工 ✅           客户端连接服务端       待启动
                           拉增量更新
                         ┌─────────────────────────────┐
P6 频道体系 (2d)  →     │ 多频道、可选安装           │
   待启动                 │ MCUpdater channel 借鉴      │
                         └──────────┬──────────────────┘
                                    │
P3 自更新 (2d)          ◄───────────┘
   待启动
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
| P0x.1 | server 骨架：go.mod + main.go + 启动/停止 | 2h | `cmd/mc-starter-server/main.go` |
| P0x.2 | 仓库目录结构：多包 + 版本目录设计 | 2h | 设计文档 + `internal/server/repo.go` |
| P0x.3 | REST API v1：客户端端点（版本查询/增量/下载） | 4h | `internal/server/api_client.go` |
| P0x.4 | REST API v1：管理端点（创建包/导入/publish/列表） | 4h | `internal/server/api_admin.go` |
| P0x.5 | 客户端 SDK：starter update 对接 server API | 3h | `internal/launcher/update.go` 改造 |
| P0x.6 | 认证：简单 token 认证 + 管理端鉴权 | 2h | `internal/server/auth.go` |
| P0x.7 | 部署：配置文件 + Dockerfile + 默认配置 | 2h | `server-config.yml` + `Dockerfile` |

> **现状**：`internal/pack/pack.go` 已实现 zip 导入/diff/publish 逻辑，P0x 负责将其包装为 HTTP API + 多包索引。
> `internal/launcher/update.go` 已实现客户端增量更新逻辑，P0x 负责将 server.json 拉取改为 API 调用。

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
| P2.10 | 崩溃报告上传（改为 API 上报） | 2h | 📋 待启动（对接 P0x 后） |
| P2.11 | 修复后自动同步 | 2h | 📋 待启动 |
| P2.12 | 修复 TUI 界面（bubbletea） | 4h | 📋 待启动 |
| P2.13 | 托盘菜单入口 | 2h | 📋 待启动 |
| P2.14 | Windows 弹窗兜底（无终端） | 2h | 📋 待启动 |
| P2.15 | 修复后 PCL2 刷新 | 1h | 📋 待启动 |

### P3：自更新 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P3.1 | 更新检查+semver 比较 | 3h |
| P3.2 | 下载+hash+签名校验 | 3h |
| P3.3 | 替换自身+bat 脚本 | 4h |
| P3.4 | 回滚（10s 检测） | 3h |
| P3.5 | 多通道 stable/beta/dev | 2h |
| P3.6 | 交互通知 | 2h |

### P5：启动器兼容 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P5.1 | PCL2/裸启动模式 | 3h* |
| P5.2 | PCL2 集成：ini 读写+注册表 | 6h* |
| P5.3 | HMCL 兼容 | 2h |
| P5.4 | 官方启动器兼容 | 1h |

> *P5.1 版本查找器 `finder.go` 和 P5.2 PCL2 检测 `pcl_detect.go` 已超前完成。

### P6：更新频道体系 — 📋 待启动

| ID | 任务 | 预估 |
|----|------|------|
| P6.1 | 频道数据结构 + 配置格式 | 2h |
| P6.2 | 服务端多频道管理 | 3h |
| P6.3 | 客户端频道选择 | 2h |
| P6.4 | 可选/必装标记及安装逻辑 | 1h |

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
P2.1 Fabric 安装器      → 2h
P2.2 Fabric libraries   → 4h
P2.6 备份系统           → 4h
P2.7 修复命令           → 3h
P2.8 崩溃检测           → 2h
P2.9 静默守护           → 4h
─────────────────────────────
里程碑 M3：./starter sync + repair 可用
```

### 📋 Sprint 5（当前 — 服务端骨架）

```
P0x.1 server 骨架       → 2h
P0x.2 仓库目录结构      → 2h
P0x.3 客户端 API        → 4h
P0x.4 管理 API          → 4h
P0x.5 客户端 SDK 改造   → 3h
P0x.6 认证              → 2h
P0x.7 部署配置文件      → 2h
─────────────────────────────
里程碑 M4：mc-starter-server 可用 +
  starter 客户端通过 API 拿增量更新
```

### 📋 Sprint 6（收尾）

```
P2.10-P2.15 修复细化    → 各2h
P5 启动器兼容            → 6h
P6 频道体系              → 8h
QA 手动测试+README       → 6h
P3 自更新                → 12h
─────────────────────────────
里程碑 M5：v1.0 发布
```

---

## 五、关键路径

```
P0.1 → P0.2 → P0.3 ──────────────── 关键路径（已走完）
                    ↘                        ↗
                     P0.4 → P0.5 → P1.x → P0x.x ← 当前
                                           ↓
                                    P2.x → P5.x → P6.x → P3.x
```

## 六、服务端 API 设计草案（P0x 先行）

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

### 🔍 后续评估

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
