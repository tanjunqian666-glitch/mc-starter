# PCL 源码蒸馏分析：Libraries 下载 & 整合包安装

> 源码参考：https://github.com/Meloong-Git/PCL  
> 关键文件：`ModMinecraft.vb`（McLib* 系列）、`ModDownload.vb`（下载调度）、`ModModpack.vb`（整合包）

---

## 一、PCL Libraries 处理的精妙设计

### 1. McLibToken — 统一的库文件描述

```vb
Public Class McLibToken
    Public LocalPath As String   ' 完整本地路径
    Public Size As Long          ' 文件大小
    Public IsNatives As Boolean  ' 是否为 Natives
    Public SHA1 As String        ' 可选 SHA1
    Public Url As String         ' 原始 URL（可能为 Nothing）
    Public OriginalName As String ' 原始 Maven 坐标
End Class
```

PCL 把**所有**库文件（含 Minecraft client.jar）统一抽象为 `McLibToken`，后面下载、校验、启动参数构建全部用同一个类型。  
我们用的是 `LibraryEntry` + `DownloadLibrary()` 返回 `(string, bool, error)`，混合了下载、校验、本地路径三个职责。

**可取之处**：将"解析"和"下载"分离——先解析出一个 `McLibToken` 列表，再逐一/批量下载。

### 2. McLibListGet — 递归继承版本解析

```vb
Public Function McLibListGet(Instance As McInstance, IncludeMainJar As Boolean) As List(Of McLibToken)
    ' 1. 获取当前版本 JSON 的 libraries
    ' 2. 如果 JSON 中有 inheritsFrom，递归遍历父版本
    ' 3. 子版本 libraries 放前面（Java classpath 顺序）
    ' 4. IncludeMainJar=true 时把 client.jar 也加进去
```

关键逻辑：`JsonObject` 的 Getter 中已经完成了 `inheritsFrom` 的合并（子版本 libraries + 父版本 libraries），所以 `McLibListGet` 拿到的是**合并后的完整列表**。

**我们当前**：`version_manifest.go` 的 `Fetch()` 不处理继承，`version.go` 的 `VersionMeta` 结构也没管 `inheritsFrom`。如果目标版本继承自另一个版本（如 Fabric/Forge 依赖于原版），我们的 libraries 列表会不完整。

### 3. McJsonRuleCheck — 精确的 rules 匹配

```vb
Public Function McJsonRuleCheck(RuleToken As JToken) As Boolean
    ' 支持 os.name / os.version / os.arch 完全匹配
    ' 支持 features (is_demo_user 反选)
    ' 支持 quick_play 规则过滤
    ' allow/disallow 双逻辑
End Function
```

**我们当前**：`ShouldInclude` 是 Windows-only 简化版，只匹配 `os.name=="windows"`，不支持 `os.version` 正则、`os.arch` 架构判断、`features` 标签。

### 4. McLibGet — 灵活映射机制

```vb
Public Function McLibGet(Name As String, Optional IsUrl As Boolean = False,
                         Optional IsRootUrl As Boolean = False,
                         Optional CustomMcFolder As String = Nothing) As String
```

这是一个核心翻译函数，把 Maven 坐标转为本地路径，支持：

- 标准的 Maven 仓库路径
- `${arch}` 占位符替换（32/64 位系统）
- 自定义 Minecraft 文件夹
- 通过 `IsUrl` 标识返回 URL 还是本地路径

### 5. DlSourceLauncherOrMetaGet — 智能镜像切换

```vb
Public Function DlSourceLauncherOrMetaGet(Url As String, ...) As String
```

自动检测响应速度决定是否使用 BMCLAPI 镜像。判断逻辑：
- 首次用官方源，记录时间
- 若耗时 < 4s，标记为"可优先使用"
- 若 > 4s，切换到 BMCLAPI 镜像

PCL 维护一个 `DlPreferMojang` 全局状态，根据实际延迟动态切换。

---

## 二、整合包安装设计（ModModpack）

### 1. 多格式识别

| 类型 | 标识文件 | 说明 |
|------|---------|------|
| CurseForge | `manifest.json` (无 addons) | 最主流 |
| HMCL | `modpack.json` | HMCL 私有格式 |
| MMC | `mmc-pack.json` | MultiMC/Prism |
| MCBBS | `mcbbs.packmeta` | 国内论坛 |
| Modrinth | `modrinth.index.json` | 新晋标准 |
| LauncherPack | `modpack.zip`/`.mrpack` | 含启动器 |

文件遍历顺序：根目录优先 → 一级子目录兜底 → 根据关键文件判断类型。

### 2. 解压 → 覆写目录 → 安装 Mod

流程：
1. 解压整合包到临时目录
2. 复制 `overrides/` 到版本目录（包括 config、mods、scripts 等）
3. 处理 `.minecraft` 顶层文件（options.txt、servers.dat 等）
4. 下载 Mod 文件（CurseForge API / Modrinth API）

### 3. CurseForge 的 Mod 依赖链

```vb
' 1. 从 manifest.json 读取 files[]（projectID + fileID）
' 2. 批量请求 curseforge API 获取下载信息
' 3. 如果文件已删除，尝试获取该 project 的替代文件
' 4. 对于可选 Mod，弹窗让用户选择
```

---

## 三、对我们项目的借鉴价值

### 高优先级（直接影响正确性）

1. **`inheritsFrom` 支持** — 当前 library.go 假设所有库都在同一个 JSON 里，但 Fabric/Forge 版依赖原始版本。需要：
   - 在 `VersionMeta` 中加入 `InheritsFrom` 字段
   - `Fetch()` 时递归拉取父版本的 libraries 合并
   - 子版本 libraries 放 classpath 前面

2. **rules 精确匹配** — 当前 `ShouldInclude` 需要扩展支持：
   - `os.version` 正则匹配
   - `os.arch` (x86 vs x64)
   - `features`（is_demo_user 反选）

### 中优先级（提升健壮性）

3. **分离"解析"和"下载"** — 改成像 PCL 那样两步走：
   - Step 1: `ResolveLibraries(meta)` → `[]LibraryFile`（全是本地路径 + URL + SHA1）
   - Step 2: 批量下载/校验

4. **智能镜像 fallback** — `DownloadLibrary` 中加入类似 `DlSourceLauncherOrMetaGet` 的逻辑，根据实际响应速度决定用 BMCLAPI 还是官方源

### 低优先级（新功能）

5. **整合包多格式支持** — 目前 modpack 方案只写了概念文档（`docs/zh/`），可以参考 PCL 的 6 种格式识别器 + 解压/覆写/Mod 下载流水线

6. **client.jar 纳入统一管理** — 把 client.jar 也作为 `LibraryFile` 处理，这样 classpath 构建和下载调度可以统一

---

## 四、从 PCL 源码中确认的一些实现细节

### Maven 坐标异常处理
- `${arch}` 占位符：32 位系统 → "32"，64 位 → "64"
- 部分库有 `pack` / `pack.xz` 格式（Mojang 1.17+ 开始压缩），PCL 不处理

### Natives 提取
- 和我们的思路一致：从 `classifiers[natives[os]]` 获取下载 URL
- 下载后解压 `.dll` / `.so` / `.dylib`（我们已实现）
- PCL 额外做了 `.pcf` 和 `.properties` 的提取（某些包需要）

### Classpath 构建顺序
- `子版本 libraries 在前，父版本在后` — Java classpath 顺序语义
- 同一个 `groupId:artifactId` 不同版本 → 保留先出现的

### 进度与 UI 分离
- PCL 的 `LoaderTask` / `LoaderDownload` / `LoaderCombo` 将下载流程抽象为可组合的流水线
- 每个步骤有独立的 `ProgressWeight`，支持进度百分比计算
- 我们的 `sync` 命令中 asset 下载的 goroutine worker pool 已经实现了类似的并发下载模式，但缺少进度报告和可组合性
