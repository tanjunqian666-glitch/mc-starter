# MC-Starter

> Minecraft 整合包更新器。双击、选版本、打开启动器。

## 快速开始

```
1. 下载 starter.exe
2. 首次双击 → 配置向导（填 API → 自动检测启动器 → 自动检测 MC 目录）
3. 选版本 → 点更新（自动同步 + 自动装 Fabric/Forge）
4. 点打开启动器 → 开玩
```

### 系统要求

- Windows 10/11
- Java 17+（首次运行会自动引导安装）
- PCL2 或 HMCL

## 构建

```bash
# 开发构建（带控制台窗口，便于调试）
go build -o starter.exe ./cmd/starter/

# 生产构建（无控制台窗口）
go build -ldflags="-s -w -H=windowsgui" -o starter.exe ./cmd/starter/
```

## GUI 界面

```
┌─ MC Starter ────────────────[⚙]─┐
│                                    │
│  版本: [主整合包 v1.2.0       ▼]   │
│                                    │
│  [📂 打开启动器]  [🔄 更新]       │
│                                    │
│  主整合包  本地: v1.2.0  最新: v1.3.0
│  有可用更新                        │
└────────────────────────────────────┘
```

详见 [GUI 界面设计](docs/zh/GUI界面设计.md)

## CLI 子命令（调试/批处理用）

| 命令 | 说明 |
|---|---|
| `starter run` | 全自动：检测→同步→拉起 PCL2 |
| `starter sync` | 同步 MC 版本（jar/asset/library） |
| `starter update` | 增量更新整合包 |
| `starter repair` | 修复工具（清理/回滚） |
| `starter backup` | 备份管理 |
| `starter cache` | 缓存管理 |
| `starter fabric install` | Fabric 安装 |
| `starter daemon` | 崩溃守护 |
| `starter init \| check` | 初始化/检查 |
| `starter pack` | 服务端打包管理 |
| `starter version` | 版本信息 |

## 设计原则

- **小工具**：不需要用户理解 Fabric/Forge/内存/备份
- **无感更新**：后台自动安装加载器，用户只看到进度条
- **Windows 原生**：walk GUI，无浏览器引擎，无额外进程

## 许可证

MIT

---

> [English →](README.md)
