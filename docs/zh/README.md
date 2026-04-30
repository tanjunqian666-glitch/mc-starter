# MC-Starter

> Minecraft 整合包更新器。双击配置，自动化更新。

mc-starter 是一款专注于 Minecraft 整合包管理的工具，采用 C/S（客户端/服务端）架构。管理员在服务端发布整合包版本，玩家客户端一键增量更新。支持 PCL2 / HMCL 启动器配合使用。

## 快速开始

**给玩家：**
```
1. 下载 starter-gui.exe
2. 首次双击 → 配置向导（API 地址 → 启动器路径 → MC 目录）
3. 主界面选版本 → 点更新（自动增量同步）
4. 点打开启动器 → 开玩
```

**给服主（服务端）：**
```bash
# 1. 生成默认配置
mc-starter-server init

# 2. 启动服务
mc-starter-server start

# 3. 创建整合包
curl -X POST http://localhost:8443/api/v1/admin/packs \
  -H "Authorization: Bearer change-me-please" \
  -d '{"name":"main-pack","display_name":"主服整合包","primary":true}'

# 4. 导入 zip 并发布
curl -X POST http://localhost:8443/api/v1/admin/packs/main-pack/import \
  -F "file=@modpack.zip"
curl -X POST http://localhost:8443/api/v1/admin/packs/main-pack/publish
```

## 架构

```
管理员:                         玩家:
┌─────────────────────┐         ┌──────────────────────┐
│  mc-starter-server   │  REST  │  starter (CLI / GUI)  │
│  ─── REST API (8443) │◄──────►│                      │
│  ─── 多包管理         │  API   │  ─── 增量更新(按hash) │
│  ─── 版本发布         │        │  ─── 缓存加速         │
│  ─── 文件存储         │        │  ─── Fabric 安装      │
│  ─── Token 认证       │        │  ─── 崩溃守护         │
└─────────────────────┘         └──────────────────────┘
```

## GUI 界面

```
┌─ MC Starter ──────────────────────[⚙]─┐
│                                          │
│  整合包: [主整合包    v1.2.0      ▼]     │
│                                          │
│  [📂 打开启动器]    [🔄 同步更新]       │
│                                          │
│  状态: 本地 v1.2.0 → 服务端 v1.3.0      │
│  有可用更新                              │
└──────────────────────────────────────────┘
```

详见 [GUI 界面设计](docs/zh/GUI界面设计.md)

## CLI 子命令

| 命令 | 说明 |
|---|---|
| `starter update` | 增量更新整合包（对接服务端 API） |
| `starter sync` | 同步 MC 版本（jar/asset/library） |
| `starter run` | 全自动：检测→同步→拉起启动器 |
| `starter repair` | 修复工具（清理 mods/config/回滚） |
| `starter daemon` | 崩溃守护（后台监控+日志+自动修复） |
| `starter backup` | 快照管理（创建/回滚/删除） |
| `starter cache` | 缓存管理 |
| `starter fabric install` | Fabric 安装器下载与组装 |
| `starter pack` | 服务端打包（import/publish/diff） |
| `starter pcl` | PCL2 检测/路径设置 |
| `starter init / check` | 初始化/检查 |
| `starter version` | 版本信息 |

### 服务端命令

```bash
mc-starter-server start [--config server.yml]   # 启动服务
mc-starter-server init                           # 生成默认配置
mc-starter-server check                          # 校验配置
```

## 系统要求

- Windows 10/11（GUI + 全功能）
- Linux（CLI + 服务端）
- Java 17+（首次运行会自动引导）

## 构建

```bash
# CLI（跨平台）
go build -o starter ./cmd/starter/

# GUI（仅 Windows，需要 MinGW-w64 + rsrc）
rsrc -manifest gui.manifest -o gui.syso
CGO_ENABLED=1 GOOS=windows go build -ldflags="-s -w -H windowsgui" -o starter-gui.exe ./cmd/starter/

# 服务端（跨平台）
go build -o mc-starter-server ./cmd/mc-starter-server/
```

## 设计原则

- **小工具**：不需要用户理解 Fabric/Forge/内存/备份
- **C/S 架构**：服主管理版本，玩家无感增量更新
- **无感更新**：后台自动装加载器，用户只看到进度条
- **Windows 原生**：walk GUI，无浏览器引擎，无额外进程

## 项目状态

| 模块 | 状态 |
|------|------|
| P1 版本下载 + 仓库 + 增量更新 | ✅ 完成 |
| P2 Fabric 安装 + 修复栈 + 静默守护 | ✅ 完成 |
| P0x 服务端 REST API + 认证 + 部署 | ✅ 完成 |
| P3 自更新（多通道 + 回滚） | ✅ 完成 |
| P6 频道体系（多频道 + 可选安装） | ✅ 完成 |
| GUI（walk） | ✅ Windows 完整测试通过 |
| P5 启动器感知（检测 + 目录识别 + 贯通） | 📋 逻辑贯通待编码 |

## 许可证

MIT

## 致谢

- [PCL2](https://github.com/Hex-Dragon/PCL2) — 启动器核心参考。mc-starter 的规则匹配、崩溃检测策略、增量更新思路均来自对 PCL2 源码的深入学习与分析。
- [HMCL](https://github.com/huanghongxun/HMCL) — 多版本隔离与多启动器兼容模式的参考。
- [MCUpdater / Grass-block](https://github.com/Grass-block/MCUpdater) — Minecraft 客户端资源更新系统的频道设计（Channel）和多包版本追踪方案，已被纳入 mc-starter 的 P6 频道体系规划。

---

> [English →](README.md)
