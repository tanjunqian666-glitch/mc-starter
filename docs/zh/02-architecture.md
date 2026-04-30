# 02-architecture.md — 系统架构说明（更新：C/S 架构 + Windows GUI）

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
                    │   ├─ packs/ (多版本索引)      │
                    │   └─ versions/ (版本目录)     │
                    └──────────┬───────────────────┘
                               │ HTTPS
                               ▼
                    ┌──────────────────────────────┐
                    │   mc-starter (客户端)         │
                    │                              │
                    │   ┌─ GUI ──────────────────┐ │
                    │   │ Windows 原生小工具界面   │ │
                    │   │ 版本选择 + 一键更新      │ │
                    │   │ 打开启动器              │ │
                    │   └────────────────────────┘ │
                    │   ┌─ update ───────────────┐ │
                    │   │ 拉取 server API →       │ │
                    │   │ 增量清单 → 按 hash 下载 │ │
                    │   │ CacheStore 去重         │ │
                    │   │ 多版本管理 (主/副版本)  │ │
                    │   │ 自动装 Fabric/Forge     │ │
                    │   └────────────────────────┘ │
                    │   ┌─ sync ─────────────────┐ │
                    │   │ Mojang 原版 MC 下载     │ │
                    │   │ (version/asset/library) │ │
                    │   │ 镜像选择, 断点恢复      │ │
                    │   └────────────────────────┘ │
                    │   ┌─ repair ───────────────┐ │
                    │   │ 备份/修复/崩溃检测      │ │
                    │   │ 静默守护               │ │
                    │   └────────────────────────┘ │
                    │   ┌─ CLI ──────────────────┐ │
                    │   │ 子命令 (debug/headless) │ │
                    │   └────────────────────────┘ │
                    └──────────────────────────────┘
```

## 二、模块职责

| 模块 | 包 | 职责 |
|------|---|------|
| CLI 入口 | `cmd/starter/main.go` | 子命令分发；无参数→启动 GUI |
| GUI 界面 | `internal/gui/` | Windows 原生小工具（lxn/walk） |
| 配置 | `internal/config/` | 读写 local.json（本地偏好） |
| 模型 | `internal/model/` | 通用数据类型（LocalConfig, PackInfo, PackState） |
| 下载器 | `internal/downloader/` | HTTP GET + hash 校验，两端共用 |
| 镜像 | `internal/mirror/` | 智能镜像选择（BMCLAPI/MCBBS/官方） |
| 启动器 | `internal/launcher/` | 核心包：version/asset/library/fabric/sync/repo/cache/update |
| 服务端包管理 | `internal/pack/` | zip 导入/diff/publish（被 server 二进制引用） |
| 修复 | `internal/repair/` | backup/repair/detector/daemon |
| 日志 | `internal/logger/` | 分级日志输出 |

## 三、sync 与 update 的关系

```
sync（MC 原版）                     update（整合包）
────────                          ────────
拉 Mojang 版本清单                   拉服务端 /api/v1/packs 列表
下载 MC jar                         对比本地版本
下载 asset + library                下载增量文件（按 hash）
创建 .minecraft/versions/          自动安装 Fabric/Forge
初始化 repo 快照                    写入 packs/{name}/...
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
    POST /api/v1/admin/packs/main/import
    → 服务端解包扫描 → 生成 draft
    → 对比上一版本 → 返回差异

  发布:
    POST /api/v1/admin/packs/main/publish
    → draft → published
    → 更新 packs/ 索引

客户端（用户无感）:
  双击 starter.exe
    → GUI 加载
    → 拉 GET /api/v1/packs → 填入版本下拉
    → 用户选版本 → 自动检查更新
    → 有更新 → 更新按钮亮
    → 点更新 → 
        GET /api/v1/packs/{name}/update?from={ver}
        → 按 hash 下载文件
        → 自动安装 FabricLoader/FML
        → 更新本地版本号
    → 点打开启动器 → 拉起 PCL2/HMCL（启动器检测 + 目录感知）
```

## 五、服务端目录结构

```
/var/mc-starter/
├── packs/
│   ├── index.json                    ← 所有版本索引（名称/display/primary）
│   └── main/
│       ├── meta.json                 ← 版本元信息
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
└── .cache/
    ├── manifest/                     ← MC 版本清单缓存
    └── mc_cache/                     ← CacheStore

.minecraft/
├── versions/                         ← MC 版本目录
├── assets/                           ← MC 资源文件
├── libraries/                        ← MC 库文件
├── packs/                            ← 整合包目录
│   ├── main/                         ← 主版本
│   │   ├── mods/
│   │   └── config/
│   └── optifine/                     ← 副版本（需启用）
│       └── mods/
└── starter_repo/                     ← 本地仓库（快照/缓存）
```

## 七、客户端配置格式

```json
{
  "minecraft_dir": "D:\\MC\\.minecraft",
  "server_url": "https://mc.example.com/api",
  "packs": {
    "main": {
      "enabled": true,
      "status": "synced",
      "local_version": "v1.2.0",
      "dir": "packs/main"
    },
    "optifine": {
      "enabled": false,
      "status": "none",
      "local_version": "",
      "dir": "packs/optifine"
    }
  }
}
```

## 八、已完工

| ID | 状态 | 说明 |
|----|------|------|
| P0.1-P0.6 | ✅ | CLI框架/配置/下载器/日志/镜像 |
| P1.1-P1.15 | ✅ | MC 下载/仓库/缓存/增量更新/服务端包管理 |
| P2.1-P2.2 | ✅ | Fabric 安装器 + libraries |
| P2.6-P2.9 | ✅ | 备份/修复/崩溃检测/静默守护 |
| GUI 框架 | ✅ | Windows 原生 GUI（walk）替代 TUI |

## 九、版本记录

| 版本 | 日期 | 变更 |
|------|------|------|
| v3 | 2026-04-30 | TUI→GUI 改造，lxn/walk 原生 Windows 界面 |
| v2 | 2026-04-30 | 新增 C/S 架构 + 多版本管理模式说明 |
| v1 | 2026-04-28 | 初版（纯 CLI 架构） |
