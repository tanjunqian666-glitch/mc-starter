# Launcher 开发知识速查

参见: https://ryanccn.dev/posts/inside-a-minecraft-launcher/
wiki.vg → 已 shut down 并入 minecraft.wiki

## 国内下载生态

- **BMCLAPI** (bangbang93) — 国内最核心的 MC 镜像，MCBBS 源 2024 关停后流量全压 BMCLAPI
  - 主端点: `https://bmclapi2.bangbang93.com`
  - 回退: `https://piston-meta.mojang.com` (官方)
  - 实测: 从国外访问 F522 较频繁，但 fallback 逻辑必须可靠
  - 文档站: https://bmclapidoc.bangbang93.com/ (SPA 无法 curl 抓取)
- **HMCL 源码** — Java 项目，参考其 `DownloadProviders` 架构
  - GitHub: https://github.com/HMCL-dev/HMCL
  - 镜像配置: `--bmclapiRoot` 可覆盖 BMCLAPI 地址
  - 优势: 统一下载管理器 + 镜像选择器架构
- **Modrinth API v2**: https://docs.modrinth.com/api/
- **CurseForge**: 需要 API key，closed API

## 版本下载全流程

1. 版本清单 → version_manifest_v2.json
2. 选版本 → 读取 versions[].url → version.json
3. 解析 version.json → downloads.client/assetIndex/libraries/logging
4. Asset Index → assetIndex.url → 下载 JSON → objects{hash→path}
5. Asset 文件 → `https://resources.download.minecraft.net/{hash[:2]}/{hash}`
6. Libraries → 逐条解析 rules + 下载 artifact/classifiers

### Libraries 难点

```go
// Maven 坐标格式: group:artifact:version
// 例: "org.lwjgl:lwjgl:3.3.1"
// 下载 URL 规则: {maven_url}/{group.replace('.','/')}/{artifact}/{version}/{artifact}-{version}.jar

// Rules 解析
// Library.rules[] 包含 allow/disallow + os.name/version/arch 匹配
// 只有匹配的 library 才下载
// 没有 rules = 总是下载

// Natives 处理
// Library.natives 存在时 → 取 natives[os] 得到 classifier
// → 用 classifiers[classifier] 下载对应 JAR
// → 解压 JAR 中的 *.dylib/*.so/*.dll 到 natives_dir
// → ${natives_directory} 替换启动参数
```

### Arguments 解析

```
启动格式: java [jvm_args] mainClass [game_args]
jvm_args 和 game_args 可以包含 Rule 对象（条件参数）
需要替换的占位符:
  JVM: ${natives_directory}, ${launcher_name}, ${launcher_version},
       ${classpath}, ${classpath_separator}, ${primary_jar},
       ${library_directory}, ${game_directory}
  Game: ${auth_player_name}, ${version_name}, ${game_directory},
        ${assets_root}, ${assets_index_name}, ${auth_uuid},
        ${user_type}, ${version_type}
```

## Asset 存储结构

```
. minecraft/assets/
├── indexes/
│   └── {asset_id}.json      # assetIndex 下载到这里
└── objects/
    └── {hash[:2]}/
        └── {full_hash}       # 实际文件
```

下载 URL: `https://resources.download.minecraft.net/{hash[:2]}/{full_hash}`
镜像 URL: `https://bmclapi2.bangbang93.com/assets/{hash[:2]}/{full_hash}`

## Fabric 安装流程

1. 查 meta API: `GET {mirror}/fabric-meta/v2/versions/loader/{mc_version}`
2. 选 loader 版本 → 取 launcherMeta.libraries.common + client
3. 下载 fabric-loader 和 intermediary 的 JAR
4. 合并到原版 libraries 列表
5. 替换 mainClass → KnotClient
6. Fabric libraries 格式: {name, url} → Maven 坐标转 URL

## Forge 安装

- 无统一 meta API, 依赖 Forge 安装器 jar 执行
- BMCLAPI 可查列表: `/forge/minecraft/{mc_version}`
- 安装器: `forge-installer.jar --installClient`

## 开源实现参考

- HMCL (Java): https://github.com/HMCL-dev/HMCL
- PrismLauncher (C++): https://github.com/PrismLauncher/PrismLauncher
- Ryan Cao 的 launcher 博文是**最好**的单篇入门资料

## 实用 git 搜索技巧

```bash
# 在 HMCL 源码中找某个功能
git clone https://github.com/HMCL-dev/HMCL
cd HMCL
# 找 mirror 相关代码
git log --all --oneline --grep="mirror" -- HMCL/src/main/java/org/jackhuang/hmcl/download/
# 找某个 API 端点定义
git log --all --oneline --grep="bmclapi"
```
