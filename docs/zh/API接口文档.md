# MC 版本更新器 — API 接口文档

> Minecraft + Fabric + BMCLAPI 涉及的所有外部接口

---

## 一、Mojang 版本清单 API

### 1.1 版本清单

**端点**：`GET https://piston-meta.mojang.com/mc/game/version_manifest_v2.json`

**镜像**：`https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json`

**响应**：
```json
{
  "latest": {
    "release": "1.20.4",
    "snapshot": "24w14a"
  },
  "versions": [
    {
      "id": "1.20.4",
      "type": "release",
      "url": "https://piston-meta.mojang.com/v1/packages/.../1.20.4.json",
      "time": "2024-02-01T09:54:13+00:00",
      "releaseTime": "2023-12-07T11:09:23+00:00"
    },
    {
      "id": "24w14a",
      "type": "snapshot",
      "url": "...",
      "time": "2024-02-01T09:54:13+00:00",
      "releaseTime": "2024-02-01T11:09:23+00:00"
    }
  ]
}
```

**注意**：`type` 可以是 `release` / `snapshot` / `old_beta` / `old_alpha`

---

### 1.2 版本元数据（version.json）

**端点**：版本清单中 `versions[].url`

**关键字段**：
```json
{
  "id": "1.20.4",
  "type": "release",
  "minecraftArguments": "--username ${auth_player_name} --version ${version_name} ...",
  "mainClass": "net.minecraft.client.main.Main",
  "minimumLauncherVersion": 21,
  "assets": "13",
  "assetIndex": {
    "id": "13",
    "sha1": "a32b31c2d4e5f...",
    "size": 423456,
    "totalSize": 123456789,
    "url": "https://piston-meta.mojang.com/v1/packages/.../13.json"
  },
  "downloads": {
    "client": {
      "sha1": "abc123...",
      "size": 28512345,
      "url": "https://piston-data.mojang.com/v1/objects/.../client.jar"
    },
    "client_mappings": {
      "sha1": "...",
      "size": 12345,
      "url": "https://piston-data.mojang.com/v1/objects/.../client.txt"
    },
    "server": {
      "sha1": "...",
      "size": 12345,
      "url": "https://piston-data.mojang.com/v1/objects/.../server.jar"
    }
  },
  "libraries": [
    {
      "name": "com.mojang:patchy:2.2.10",
      "downloads": {
        "artifact": {
          "path": "com/mojang/patchy/2.2.10/patchy-2.2.10.jar",
          "sha1": "...",
          "size": 12345,
          "url": "https://libraries.minecraft.net/com/mojang/patchy/2.2.10/patchy-2.2.10.jar"
        }
      }
    },
    {
      "name": "org.lwjgl:lwjgl:3.3.1",
      "rules": [
        { "action": "allow" },
        { "action": "disallow", "os": { "name": "osx" } }
      ],
      "downloads": {
        "artifact": { "...": "..." },
        "classifiers": {
          "natives-windows": { "...": "..." },
          "natives-linux": { "...": "..." },
          "natives-macos": { "...": "..." }
        }
      }
    }
  ],
  "logging": {
    "client": {
      "argument": "-Dlog4j.configurationFile=${path}",
      "file": {
        "id": "client-1.12.xml",
        "sha1": "...",
        "size": 1234,
        "url": "https://..."
      },
      "type": "log4j2-xml"
    }
  }
}
```

### 1.3 Asset 索引

**端点**：`version.json` 中 `assetIndex.url`

**关键字段**：
```json
{
  "objects": {
    "icons/icon_16x16.png": {
      "hash": "abc123def456...",
      "size": 1234
    },
    "minecraft/sounds/block/stone/hit1.ogg": {
      "hash": "def789abc123...",
      "size": 5678
    }
  }
}
```

**文件路径规则**：`assets/objects/{hash[0:2]}/{hash}`，例如 `assets/objects/ab/abc123def456...`

---

## 二、Fabric Meta API

使用 BMCLAPI 镜像（已在 mirror 管理器中配置）

### 2.1 获取 Loader 版本列表

**端点**：`GET https://bmclapi2.bangbang93.com/fabric-meta/v2/versions/loader/{mc_version}`

> 如果 `{mc_version}` 格式不匹配（如 `1.20.1`），可先查所有版本

**响应**：
```json
[
  {
    "loader": {
      "separator": "",
      "build": 11,
      "maven": "net.fabricmc:fabric-loader:0.15.11",
      "version": "0.15.11",
      "stable": true
    },
    "intermediary": {
      "maven": "net.fabricmc:intermediary:1.20.1",
      "version": "1.20.1",
      "stable": true
    },
    "launcherMeta": {
      "version": 1,
      "libraries": {
        "client": [
          {
            "name": "net.fabricmc:tiny-mappings-parser:0.3.0+build.12",
            "url": "https://maven.fabricmc.net/"
          }
        ],
        "common": [
          {
            "name": "net.fabricmc:sponge-mixin:0.13.3+mixin.0.8.5",
            "url": "https://maven.fabricmc.net/"
          }
        ]
      },
      "mainClass": {
        "client": "net.fabricmc.loader.impl.launch.knot.KnotClient"
      }
    }
  }
]
```

### 2.2 获取完整 Profile JSON

**端点**：`GET https://bmclapi2.bangbang93.com/fabric-meta/v2/versions/loader/{mc_version}/{loader_version}/profile/json`

**响应**（完整的启动 profile，可以直接替换 minecraft.jar 的 version.json 使用）：
```json
{
  "id": "fabric-loader-0.15.11-1.20.1",
  "inheritsFrom": "1.20.1",
  "releaseTime": "...",
  "time": "...",
  "type": "release",
  "mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
  "arguments": {
    "game": [],
    "jvm": [
      "-DFabricMcEmu= net.minecraft.client.main.Main "
    ]
  },
  "libraries": [
    {
      "name": "net.fabricmc:fabric-loader:0.15.11",
      "url": "https://maven.fabricmc.net/"
    },
    {
      "name": "net.fabricmc:intermediary:1.20.1",
      "url": "https://maven.fabricmc.net/"
    }
  ]
}
```

**用法**：这就是一个"轻量"的 version.json。我们的代码可以：
1. 下载原版 `1.20.1.json` → 获取 `libraries` + `downloads`
2. 合并 Fabric 的 `profile.json` → 添加 Fabric libraries + 替换 `mainClass`
3. 组合成最终启动参数

### 2.3 获取最新 Installer 版本

**端点**：`GET https://bmclapi2.bangbang93.com/fabric-meta/v2/versions/installer`

**响应**：
```json
[
  {
    "version": "1.0.1",
    "stable": true,
    "url": "https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar",
    "maven": "net.fabricmc:fabric-installer:1.0.1"
  }
]
```

### 2.4 Maven 下载库文件

**端点模式**：`{library.url} + {library 按 group/artifact 转路径}`

示例：
```
name: "net.fabricmc:fabric-loader:0.15.11"
url:  "https://maven.fabricmc.net/"
→ 下载: https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.15.11/fabric-loader-0.15.11.jar
```

---

## 三、Forge 相关 API

> Forge 没有统一的 meta API，依赖 Forge 安装器直接执行

### 3.1 Forge 版本列表

**端点**：`GET https://bmclapi2.bangbang93.com/forge/minecraft/{mc_version}`

**响应**：
```json
[
  {
    "version": "1.20.1-47.1.43",
    "build": 47,
    "mcversion": "1.20.1",
    "modified": "2023-12-12T10:32:55+00:00",
    "branch": null,
    "files": {
      "installer": "https://maven.minecraftforge.net/net/minecraftforge/forge/1.20.1-47.1.43/forge-1.20.1-47.1.43-installer.jar"
    }
  }
]
```

### 3.2 Forge 安装（备选方案）

```
forge-installer.jar --installClient .minecraft
```

安装后插件在 `.minecraft/libraries/net/minecraftforge/forge/` 下，生成 `launcher_profiles.json` 中的 profile。

---

## 四、Adoptium JRE API

### 4.1 获取 JRE 下载地址

**端点**：`GET https://api.adoptium.net/v3/assets/version/{version}/hotspot`

示例：获取 Windows x64 JRE 21 最新版本

```bash
GET https://api.adoptium.net/v3/assets/latest/21/hotspot?os=windows&arch=x64&image_type=jre
```

**镜像**：`https://mirrors.tuna.tsinghua.edu.cn/Adoptium/21/jre/x64/windows/`

清华镜像没有 API，直接列出目录结构：
```
https://mirrors.tuna.tsinghua.edu.cn/Adoptium/21/jre/x64/windows/
  OpenJDK21U-jre_x64_windows_hotspot_21.0.3_9.zip
  OpenJDK21U-jre_x64_windows_hotspot_21.0.3_9.zip.sha256.txt
```

---

## 五、Packwiz 接口

### 5.1 pack.toml

**格式**：TOML

```toml
name = "My Modpack"
author = "Author"
version = "1.0.0"
minecraft = "1.20.1"

[index]
  file = "index.toml"

[versions]
  fabric = "0.15.11"

[options]
  optional-install = true

[hashes]
  sha256 = "abc123..."
```

### 5.2 index.toml

模组清单文件，列出所有模组的元数据和下载地址

```toml
[[files]]
file = "mods/sodium.pw.toml"
metaurl = "https://cdn.modrinth.com/data/..."

[[files]]
file = "mods/lithium.pw.toml"
hash = "sha256:..."
```

### 5.3 模组元数据文件（.pw.toml）

```toml
name = "sodium"
filename = "sodium-fabric-0.5.3+mc1.20.1.jar"
side = "client"

[download]
url = "https://cdn.modrinth.com/data/.../sodium-0.5.3.jar"
hash-format = "sha256"
hash = "abc..."

[update]
[update.curseforge]
file-id = 123456
project-id = 123456
[update.modrinth]
mod-id = "AANobbMI"
version = "v0.5.3"
```

---

## 六、自定义服务器 API（我们自己要建的）

### 6.1 server.json（服务端配置）

**端点**：`GET https://你的服务器.com/server.json`

响应结构见 `立项报告.md` 的 4.2 节

### 6.2 version.json（自更新信息）

**端点**：`GET https://你的服务器.com/starter/version.json`

```json
{
  "version": "1.2.0",
  "channel": "stable",
  "download_url": "https://你的服务器.com/downloads/mc-starter-1.2.0-windows-amd64.exe",
  "hash": "sha256:abc123...",
  "changelog": "- 新增 Fabric 支持\n- 修复断点续传问题"
}
```

---

## 七、接口调用总结

| 功能 | 端点 | 优先级 |
|---|---|---|
| MC 版本清单 | mojang/bmclapi | P1 必须 |
| MC 版本元数据 | 清单中的每个 url | P1 必须 |
| Asset 索引 | version.json 中的 assetIndex.url | P1 必须 |
| Asset/Libraries | 下载 object url | P1 必须 |
| Fabric Loader 列表 | bmclapi fabric-meta | P2 必须 |
| Fabric Profile JSON | bmclapi fabric-meta profile/json | P2 必须 |
| Fabric 库文件 | maven.fabricmc.net | P2 必须 |
| Forge 版本列表 | bmclapi forge | P2 可选 |
| Java 下载 | Adoptium API / 清华镜像 | P3 可选 |
| server.json | 自建 | P0 必须 |
| starter/version.json | 自建 | P5 必须 |
