# 02-architecture.md — 系统架构说明（更新：C/S 架构 + 多包管理）

> **注意**：此架构说明仅描述客户端架构（mc-starter）。服务端架构见 `服务端架构与部署.md`。

---

## 一、总体架构

```
                    ┌──────────────────────────────┐
                    │   mc-starter-server           │
                    │   (独立进程, Windows/Linux)   │
                    │                              │
                    │   REST API v1                │
                    │   ├─ 客户端端 (/api/v1/)     │
                    │   │  packs, latest, update   │
                    │   │  files/{hash}            │
                    │   ├─ 管理端 (/api/v1/admin/) │
                    │   │  import, publish, config │
                    │   │  versions, delete        │
                    │   └─ 健康检查 (/api/v1/ping) │
                    │                              │
                    │   存储层                      │
                    │   ├─ packs/ (多包索引)        │
                    │   └─ versions/ (版本目录)     │
                    └──────────┬───────────────────┘
                               │ HTTPS
                               ▼
                    ┌──────────────────────────────┐
                    │   mc-starter (客户端 CLI)     │
                    │                              │
                    │   ┌─ update ───────────────┐ │
                    │   │ 拉取 server API →       │ │
                    │   │ 增量清单 → 按 hash 下载 │ │
                    │   │ CacheStore 去重         │ │
                    │   │ 多包管理 (主包/副包)    │ │
                    │   └────────────────────────┘ │
                    │   ┌─ sync ─────────────────┐ │
                    │   │ Mojang 原版 MC 下载     │ │
                    │   │ (version/asset/library) │ │
                    │   │ 镜像选择, 断点恢复      │ │
                    │   └────────────────────────┘ │
                    │   ┌─ repair ───────────────┐ │
                    │   │ 备份/修复/崩溃检测      │ │
                    │   │ 静默守护, 托盘菜单      │ │
                    │   └────────────────────────┘ │
                    │   ┌─ TUI ──────────────────┐ │
                    │   │ 全自动模式 (双击场景)   │ │
                    └──────────────────────────────┘
```

## 二、模块职责

| 模块 | 包 | 职责 |
|------|---|------|
| CLI 入口 | `cmd/starter/main.go` | 子命令分发，1600 行（后续拆 cmd_*.go） |
| 配置 | `internal/config/` | 读写 local.json（本地偏好）/ server.json（服务端下发） |
| 模型 | `internal/model/` | 通用数据类型（LibraryFile, PackConfig, ClientConfig） |
| 下载器 | `internal/downloader/` | HTTP GET + hash 校验，两端共用 |
| 镜像 | `internal/mirror/` | 智能镜像选择（MC 原版文件：BMCLAPI/MCBBS/官方） |
| 启动器 | `internal/launcher/` | 核心包：version/asset/library/fabric/sync/repo/cache/update |
| 服务端包管理 | `internal/pack/` | zip 导入/diff/publish（被 server 二进制引用） |
| 修复 | `internal/repair/` | backup/repair/detector/daemon |
| TUI | `internal/tui/` | 全自动模式 bubbletea 界面 |
| 日志 | `internal/logger/` | 分级日志输出 |

## 三、sync 与 update 的关系

```
sync（MC 原版）                     update（整合包）
────────                          ────────
拉 Mojang 版本清单                   拉服务端 /api/v1/packs 列表
下载 MC jar                         对比本地版本
下载 asset + library                下载增量文件（按 hash）
创建 .minecraft/versions/           写入 packs/{name}/...
初始化 repo 快照                    更新 repo 快照
                               ───  两者独立运行 ───
```

## 四、数据流

```
管理员:
  mc-starter-server start
    → 监听 :8443 (HTTPS)
    → 加载 packs/ 索引
    → 等待管理请求

  上传 zip:
    POST /api/v1/admin/packs/main-pack/import
    → 服务端解包扫描 → 生成 draft
    → 对比上一版本 → 返回差异

  发布:
    POST /api/v1/admin/packs/main-pack/publish
    → draft → published
    → 包文件按 hash 存储到 files/
    → 更新 packs/ 索引

客户端:
  starter update
    → GET /api/v1/packs
    → 对比本地 packs 配置
    → 对主包 + 已启用副包:
       GET /api/v1/packs/{name}/update?from={ver}
       → 拿到增量 (added/updated/removed)
       → 按 hash: 先查 CacheStore → 再 GET /files/{hash}
       → 写入 packs/{name}/{path}
       → 更新 repo 快照
```

## 五、服务端目录结构

```
/var/mc-starter/
├── packs/
│   ├── index.json                    ← 所有包索引（名称/display/primary）
│   └── main-pack/
│       ├── meta.json                 ← 包元信息
│       ├── server.json               ← 客户端拉取的入口
│       ├── versions/
│       │   ├── v1.0.0/manifest.json
│       │   ├── v1.1.0/manifest.json
│       │   └── v1.2.0.draft/manifest.json
│       └── files/                    ← 按 hash 存储
│           ├── a1/b2/c3d4...
│           └── ...
├── server.yml                        ← 服务端配置
└── data/                             ← 运行时数据
```

## 六、本地客户端目录结构

```
config/
├── local.json                        ← 本地配置（MC目录/server_url/packs）
├── server.json                       ← 服务端下发配置缓存
└── .cache/
    ├── manifest/                     ← MC 版本清单缓存
    └── mc_cache/                     ← CacheStore

.minecraft/
├── versions/                         ← MC 版本目录
├── assets/                           ← MC 资源文件
├── libraries/                        ← MC 库文件
├── packs/                            ← 整合包目录
│   ├── main-pack/                    ← 主包
│   │   ├── mods/
│   │   └── config/
│   └── extra-content/                ← 副包（需启用）
│       └── mods/
└── starter_repo/                     ← 本地仓库（快照/缓存）
```

## 七、客户端配置格式

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

## 八、状态记录

### 已完工

| ID | 状态 | 文件 |
|----|------|------|
| P0.1-P0.6 | ✅ | CLI框架/配置/下载器/日志/镜像 |
| P1.1-P1.15 | ✅ | MC 下载/仓库/缓存/增量更新/服务端包管理 |
| P2.1-P2.2 | ✅ | Fabric 安装器 + libraries |
| P2.6-P2.9 | ✅ | 备份/修复/崩溃检测/静默守护 |
| P2.12 (超前) | ✅ | TUI 界面 |

### 当前阶段

| 阶段 | 状态 | 说明 |
|------|------|------|
| P0x 服务端骨架 | 🎯 当前 | 独立 server + REST API + 多包管理 |
| P2.x 剩余修复细化 | 📋 | 上传/同步/TUI 集成/托盘 |
| P5 启动器兼容 | 📋 | PCL2/HMCL 集成 |
| P6 频道体系 | 📋 | 多频道/可选安装 |
| P3 自更新 | 📋 | 自身版本管理 |

## 九、版本记录

| 版本 | 日期 | 变更 |
|------|------|------|
| v2 | 2026-04-30 | 新增 C/S 架构 + 多包管理模式说明 |
| v1.1 | 2026-04-29 | 更新 P1/P2 完工状态 |
| v1 | 2026-04-28 | 初版（纯 CLI 架构） |
