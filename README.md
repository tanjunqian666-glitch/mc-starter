# MC-Starter

轻量级 Minecraft 版本管理 + 整合包更新器。

> 不自带启动器、不捆绑代理、不占资源。只做一件事：**把指定版本的 Minecraft + 模组包下载配置好，打开 PCL2 就能玩。**

## 快速开始

### 最简单的方式（30 秒上手）

```
1. 把 starter.exe 扔到 PCL2.exe 旁边
2. 把 config/server.json 也放旁边
3. 双击 starter.exe
   └→ 自动完成：搜索 PCL2 → 下载 MC → 安装 Fabric → 同步模组 → 更新 PCL.ini → 拉起 PCL2
4. 在 PCL2 里点启动 → 开始玩
```

**全程只做一步：双击。**

如果自动搜索不到 PCL2 或 .minecraft，程序会提示你手动选择文件夹。

### 前提

- **Java 17+**（没有的话 `starter run` 会引导你下载）

### 命令参考

| 命令 | 作用 |
|---|---|
| `starter` / `starter run` | 全自动模式：检测→同步→集成→拉起 PCL2（最常用） |
| `starter run --headless` | 静默模式，不交互 |
| `starter init` | 初始化本地配置 |
| `starter check` | 检查 Java / PCL2 / 配置完整性 |
| `starter sync` | 仅同步，不拉 PCL2 |
| `starter pcl detect` | 手动检测 PCL2.exe 位置 |
| `starter pcl path <路径>` | 手动设置 PCL2 路径 |
| `starter version` | 显示版本信息 |
| `starter self-update` | 更新启动器自身 |

### 手动控制

如果你不想开全自动模式，也可以分步执行：

```bash
# 1. 初始化配置
starter init

# 2. 检查环境
starter check

# 3. 同步版本+模组
starter sync

# 4. 手动启动 PCL2
#（starter 已经帮你写好了 PCL.ini，打开 PCL2 就能看到新版本）
```

### 常用选项

| 选项 | 作用 |
|---|---|
| `--config ./my-config` | 指定配置目录（默认 ./config） |
| `--verbose` | 详细日志 |
| `--headless` | 静默模式，不弹交互提示 |
| `--dry-run` | 仅检查不下载 |

## 配置说明

### server.json（服务端下发，不需要手动编辑）

```
config/
├── server.json     ← 自动更新，不要手动改
└── local.json      ← 你可以编辑这个
```

**local.json** 可配置项：

```json
{
  "install_path": "./.minecraft",     // MC 安装目录
  "launcher": "bare",                  // bare / pcl2 / hmcl
  "java_home": "",                     // Java 路径（留空自动检测）
  "memory": 4096,                      // 分配内存 MB
  "username": "Player",                // 离线用户名
  "mirror_mode": "auto"                // auto / china / global
}
```

## 文件结构

```
你的整合包目录/
├── starter(.exe)          ← 主程序
├── config/
│   ├── server.json        ← 服务端配置（自动更新）
│   └── local.json         ← 你的偏好设置
├── updater/
│   └── cache/             ← 下载缓存
├── .minecraft/            ← Minecraft 目录
│   ├── versions/
│   ├── assets/
│   ├── mods/
│   └── ...
├── starter.log            ← 日志
└── README.md
```

## 常见问题

**Q: 需要管理员权限吗？**
不需要，除非你把 .minecraft 装在系统盘 Program Files 下。

**Q: 支持 Forge 吗？**
支持。在 `server.json` 的 `loader.type` 填 `forge` 即可。

**Q: 下载太慢怎么办？**
starter 内置了国内镜像加速。你可以在 local.json 设置 `mirror_mode: "china"`。

**Q: 可以同时玩多个整合包吗？**
可以。每个整合包放在不同目录，各自有独立的 starter + config + .minecraft。

**Q: 怎么用 PCL2 / HMCL 启动？**
sync 完成后会生成 `launcher_profiles.json`，PCL2 可以直接识别。
或者设置 `local.json` 的 `launcher: "pcl2"` 让 starter 帮你配置。

## 从源码构建

```bash
git clone https://github.com/你的名字/mc-starter.git
cd mc-starter
make build
```

交叉编译：

```bash
make build-all
# → build/ 目录下 windows / linux / mac 三平台二进制
```

## 许可证

MIT
