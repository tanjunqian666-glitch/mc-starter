# MC-Starter

> Minecraft 更新器。双击，等，玩。

## Quick Start

```
1. 把 starter.exe 放到 PCL2.exe 旁边
2. 把 config/server.json 放在一起
3. 双击 starter.exe
   └→ 自动: 找 PCL2 → 下 MC → 装 Fabric → 同步模组 → 配 PCL.ini → 启动 PCL2
4. 点 Play
```

### 前提

- Windows 10/11
- Java 17+（没有会引导安装）

## Commands

| 命令 | 描述 |
|---|---|
| `starter run` | 全自动: 检测 → 同步 → 集成 → 启动 PCL2 |
| `starter init` | 初始化本地配置 |
| `starter check` | 检查 Java / PCL2 / 配置 |
| `starter sync` | 仅同步版本 + 模组 |
| `starter repair` | 修复工具 |
| `starter pcl detect` | 查找 PCL2.exe |
| `starter pcl path <path>` | 手动指定 PCL2 路径 |
| `starter version` | 版本信息 |

### Flags

`--config ./dir` 配置目录（默认 `./config`）
`--verbose` / `--headless` / `--dry-run`

## Build

```bash
make build          # → build/starter.exe
make build-release  # GUI 模式，无控制台窗口
make size           # 查看二进制大小
```

## 设计目标

- **小**：标准库依赖，`-ldflags="-s -w"`，可选 UPX
- **省**：无轮询、无浏览器引擎、无多余进程
- **快**：启动到决策 ~5ms
- **专**：Windows only，系统托盘 + 原生弹窗

## License

MIT

---

> [中文文档 →](docs/zh/README.md)
