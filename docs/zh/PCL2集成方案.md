# MC 版本更新器 — PCL2 集成方案（补充）

> 功能补充：允许更新器放在 PCL2 目录下，自动识别并管理 PCL2 已发现的 MC 版本。

---

## 一、PCL2 配置与数据说明

根据 PCL2 源码（ModMinecraft.vb）分析，PCL2 的数据结构如下：

### 1.1 PCL.ini（PCL2 的"当前状态"快照）

**位置**：`{Minecraft 目录}/PCL.ini`（即 `.minecraft/PCL.ini`）

**格式**：INI 文件，记录当前选中的版本、卡片布局、自定义内容

```ini
VersionCache=196994886        ; 版本列表缓存时间戳
Version=main-test              ; 当前选中的版本
CardCount=1                    ; 卡片数量
CardKey1=2                     ; 卡片 1 类型（2 = 版本卡片）
CardValue1=小猪之家2.0:...     ; 卡片 1 的版本列表（: 分隔）
CardKey2=6                     ; 卡片 2 类型（6 = 服务器）
CardValue2=xydk:               ; 卡片 2 的服务器列表
InstanceCache=1521624170       ; 版本实例缓存时间戳
```

**卡片类型**：

| CardKey | 含义 |
|---|---|
| 2 | 版本卡片（显示多个 MC 版本） |
| 3 | 图片卡片 |
| 4 | 下载卡片 |
| 5 | 设置卡片 |
| 6 | 服务器卡片 |
| 8 | 整合包卡片 |

### 1.2 PCL 注册表设置

PCL2 将大量设置存储在 Windows 注册表中（`Source:=Sources.Registry`），包括：
- 登录信息
- 窗口设置
- 下载配置
- 自定义文件夹列表 (`LaunchFolders`)
- 选中的文件夹 (`LaunchFolderSelect`)

### 1.3 版本实例配置（Source:=Sources.Instance）

**位置**：每个版本在 `.minecraft/versions/{version}/` 下有其独立的配置

PCL2 的 `Settings` 系统有三层来源：
- `Normal`：内存默认值
- `Registry`：Windows 注册表
- `Instance`：每个版本独立的实例设置（存储方式需进一步确认）

### 1.4 .minecraft 文件夹发现逻辑

PCL2 搜索 MC 文件夹的顺序：
1. **当前目录** — 扫描 PCL2.exe 所在目录及子目录中是否有 `versions/` 文件夹
2. **子目录** — 检查当前目录的子目录是否有 `.minecraft` 目录
3. **官方启动器目录** — `%APPDATA%/.minecraft/`
4. **用户自定义** — `LaunchFolders` 设置中保存的自定义路径

### 1.5 版本列表

**位置**：`.minecraft/versions/{version_name}/{version_name}.json`

PCL2 扫描此目录获取所有可用版本，每个版本子目录下必须有一个同名的 `.json` 文件（version.json 格式）。

---

## 二、更新器与 PCL2 的交互方式

### 2.1 更新器定位模式

**模式 A：独立目录（已有的设计）**
```
your-modpack/
├── mc-starter.exe
├── config/
│   ├── server.json
│   └── local.json
├── .minecraft/
│   ├── versions/
│   ├── mods/
│   ├── PCL.ini          ← 更新器生成（让 PCL2 识别）
│   ├── launcher_profiles.json
│   └── ...
└── updater/cache/
```

**模式 B：放在 PCL2 目录下（新增）**
```
(在 PCL2.exe 旁边)
your-modpack/
├── mc-starter.exe         ← 放在 PCL2 同目录
├── Plain Craft Launcher 2.exe  ← PCL2 启动器
├── config/
│   └── server.json        ← 更新器的配置
├── .minecraft/
│   ├── versions/
│   │   └── main-test/     ← 更新器管理的版本
│   ├── mods/
│   └── PCL.ini            ← 更新器写入，PCL2 读取
└── ...
```

### 2.2 搜索 PCL2.exe 的逻辑

```go
// 启动时，如果指定了 --pcl-mode，或者检测到 PCL2.exe 在附近
func FindPCL2() (string, error) {
    // 1. 检查 local.json 中的 pcl2_path 配置
    // 2. 检查当前目录
    // 3. 检查父目录
    // 4. 检查 PATH 环境变量
    // 5. 扫描同级目录
    
    searchPaths := []string{
        localConfig.PCL2Path,           // 手动指定
        filepath.Join(workDir, "Plain Craft Launcher 2.exe"),
        filepath.Join(workDir, "..", "Plain Craft Launcher 2.exe"),
    }
    
    for _, p := range searchPaths {
        if p != "" {
            if _, err := os.Stat(p); err == nil {
                return p, nil
            }
        }
    }
    return "", fmt.Errorf("未找到 Plain Craft Launcher 2.exe")
}
```

### 2.3 更新 .minecraft 版本

当更新器同步完成后，它应该：

1. 将下载的 MC 版本写入 `.minecraft/versions/{version_name}/`
2. 确保 `version.json` 存在且格式正确
3. 创建/更新 `PCL.ini`

```go
// 同步完成后
func SyncToPCL2(mcDir string, versionName string) error {
    // 1. 确保 versions 目录结构
    versionDir := filepath.Join(mcDir, "versions", versionName)
    
    // 2. 更新 PCL.ini
    pclIniPath := filepath.Join(mcDir, "PCL.ini")
    ini := LoadPCLIni(pclIniPath)
    
    // 更新版本缓存时间戳（让 PCL2 知道版本列表变了）
    ini.Set("VersionCache", strconv.FormatInt(time.Now().Unix(), 10))
    
    // 设置当前版本
    ini.Set("Version", versionName)
    
    // 在卡片列表中注册（如果版本卡片存在）
    for i := 1; ; i++ {
        key := fmt.Sprintf("CardKey%d", i)
        if !ini.Has(key) {
            break
        }
        if ini.Get(key) == "2" { // 版本卡片
            valKey := fmt.Sprintf("CardValue%d", i)
            versions := ini.Get(valKey)
            if !strings.Contains(versions, versionName) {
                versions = versionName + ":" + versions
                ini.Set(valKey, versions)
            }
            break
        }
    }
    
    // 如果没有版本卡片，创建一个
    // (简化处理)
    
    return ini.Save(pclIniPath)
}
```

### 2.4 读取 PCL2 已有的版本

```go
// 让更新器识别 PCL2 已管理的版本
func GetPCL2Versions(mcDir string) ([]string, error) {
    versionsDir := filepath.Join(mcDir, "versions")
    entries, err := os.ReadDir(versionsDir)
    if err != nil {
        return nil, err
    }
    
    var versions []string
    for _, entry := range entries {
        if entry.IsDir() {
            versionJson := filepath.Join(versionsDir, entry.Name(), entry.Name()+".json")
            if _, err := os.Stat(versionJson); err == nil {
                versions = append(versions, entry.Name())
            }
        }
    }
    return versions, nil
}
```

### 2.5 PCL.ini 读写

```go
// 简单 INI 读写实现（PCL.ini 格式较简单，不需要完整 INI 解析库）
type PCLIni struct {
    sections map[string]map[string]string
}

func LoadPCLIni(path string) (*PCLIni, error) {
    ini := &PCLIni{sections: map[string]map[string]string{
        "": {},
    }}
    
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return ini, nil // 文件不存在就返回空
        }
        return nil, err
    }
    
    currentSection := ""
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
            continue
        }
        if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
            currentSection = line[1 : len(line)-1]
            if _, ok := ini.sections[currentSection]; !ok {
                ini.sections[currentSection] = map[string]string{}
            }
            continue
        }
        if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])
            ini.sections[currentSection][key] = value
        }
    }
    return ini, nil
}

func (ini *PCLIni) Get(key string) string {
    return ini.sections[""][key]
}

func (ini *PCLIni) Set(key, value string) {
    ini.sections[""][key] = value
}

func (ini *PCLIni) Has(key string) bool {
    _, ok := ini.sections[""][key]
    return ok
}

func (ini *PCLIni) Save(path string) error {
    var buf strings.Builder
    for _, line := range ini.sections[""] {
        // 没有 section 头，直接写 key=value
    }
    for key, value := range ini.sections[""] {
        buf.WriteString(fmt.Sprintf("%s=%s\n", key, value))
    }
    return os.WriteFile(path, []byte(buf.String()), 0644)
}
```

---

## 三、新增的 CLI 命令

### 3.1 `starter pcl [mode]`

```bash
# 检测并关联 PCL2
starter pcl detect
  → 搜索当前目录及父目录中的 PCL2.exe
  → 输出找到的 PCL2 路径
  → 写入 local.json 的 pcl2_path

# 同步 PCL2 版本列表到更新器
starter pcl list
  → 读取 .minecraft/versions/ 下的版本
  → 输出所有版本名称
  → 标记更新器要管理的目标版本

# 设置当前版本
starter pcl select <version>
  → 写入 PCL.ini 的 Version 字段
  → 更新卡片列表

# 集成（全流程）
starter pcl sync
  → 检测 PCL2
  → 读取已有版本
  → 执行完整 sync
  → 更新 PCL.ini
```

### 3.2 配置变化

在 `local.json` 中新增字段：

```json
{
  "pcl2_path": "",                // PCL2.exe 路径（自动检测后填充）
  "pcl2_integration": "auto",     // auto / manual / disabled
  "pcl2_version_name": "main-test",  // 在 PCL2 中显示的版本名称
  "pcl2_update_card": true        // 自动更新 PCL.ini 卡片
}
```

### 3.3 完整集成流程

```go
func PCL2Integration() error {
    // 1. 定位 PCL2
    pcl2Path, err := FindPCL2()
    if err != nil {
        return fmt.Errorf("未找到 PCL2: %w", err)
    }
    
    // 2. 确定 .minecraft 目录
    // PCL2 同目录下找 .minecraft，或子目录下的 .minecraft
    mcDir := filepath.Join(filepath.Dir(pcl2Path), ".minecraft")
    if _, err := os.Stat(mcDir); os.IsNotExist(err) {
        // 检查 PCL2 同目录的子目录
        entries, _ := os.ReadDir(filepath.Dir(pcl2Path))
        for _, e := range entries {
            if e.IsDir() {
                subMc := filepath.Join(filepath.Dir(pcl2Path), e.Name(), ".minecraft")
                if _, err := os.Stat(subMc); err == nil {
                    mcDir = subMc
                    break
                }
            }
        }
    }
    
    // 3. 更新版本
    // ...
    
    // 4. 写入 PCL.ini
    pclIni, _ := LoadPCLIni(filepath.Join(mcDir, "PCL.ini"))
    pclIni.Set("Version", localConfig.PCL2VersionName)
    pclIni.Set("VersionCache", fmt.Sprintf("%d", time.Now().Unix()))
    pclIni.Save(filepath.Join(mcDir, "PCL.ini"))
    
    // 5. 通知用户
    fmt.Printf("✅ 集成完成！打开 %s 即可看到新版本\n", pcl2Path)
    return nil
}
```

---

## 四、用户场景（核心设计）

### 场景 0：最爽的体验（无感模式）

```
1. 服务器下发：
    整合包文件夹/
     ├── starter.exe
     └── config/
         └── server.json

2. 用户双击 starter.exe → 啥都不用干

3. 程序自动：
   ┌─ ① 搜索当前目录及父目录有没有 PCL2.exe
   │      ├─ 找到 → 记下路径，确定 .minecraft 目录
   │      └─ 没找到 → 弹窗让用户手动选 PCL2.exe 位置
   │
   ├─ ② 检查 .minecraft/versions/ 下有没有我们的版本
   │      ├─ 有 → 检查是否需要更新
   │      └─ 没有 → 创建版本目录
   │
   ├─ ③ 执行 sync（下载/更新 MC + Fabric + 模组）
   │
   ├─ ④ 更新 PCL.ini（写入 Version + 刷新卡片）
   │
   └─ ⑤ 自动拉起 PCL2.exe ← 用户直接看到新版本点启动

4. 用户：？ 已经可以玩了？
```

**用户全程只需要做一件事：双击 starter.exe。**

### 场景 1：手动指定路径（自动搜索失败时）

```
1. 用户双击 starter.exe
2. 自动搜索 PCL2.exe → 没找到
3. 弹出文件选择窗口 / 命令行提示：
   "[?] 未找到 Plain Craft Launcher 2.exe
        请输入 PCL2 所在文件夹路径（或拖拽文件夹到此处）:
        > _"
4. 用户输入路径（或拖拽文件夹）
5. 继续自动执行 ②→③→④→⑤
```

如果也找不到 `.minecraft` 目录：
```
   "[?] 未找到 .minecraft 文件夹
        请输入 Minecraft 游戏目录路径:
        > _"
```

### 场景 2：已有 PCL2，想加一个整合包

```
1. 把 starter.exe 丢到 PCL2.exe 旁边
2. 把 server.json 放到 config/ 目录
3. 双击 starter.exe
4. 自动完成全部流程，最后拉起 PCL2
```

### 场景 3：纯 CLI 模式（服务器批量分发）

```
starter --headless run
  → 不交互、不弹窗、不调 PCL2
  → 只执行同步，适合脚本调用
```

---

## 五、核心交互流程（详细）

### 5.1 启动流程图

```
starter.exe 启动
    │
    ▼
┌─────────────────────────────────────┐
│  Phase 0: 环境检测                  │
│                                     │
│  1. 搜索 PCL2.exe                   │
│     ├─ 同目录/子目录/父目录         │
│     ├─ LaunchFolders 注册表         │
│     └─ 用户上次保存的路径(local.json)│
│                                     │
│  2. 结果？                          │
│     ├─ 找到 → 记录路径，继续        │
│     └─ 没找到 → 交互模式？         │
│         ├─ 是 → 让用户手动选择      │
│         └─ 否(headless) → 报错退出  │
│                                     │
│  3. 确定 .minecraft 目录            │
│     ├─ PCL2 同目录下 .minecraft/    │
│     ├─ 子目录里找 .minecraft/       │
│     ├─ %APPDATA%/.minecraft/        │
│     └─ 手动指定                     │
└──────────────┬──────────────────────┘
               ▼
┌─────────────────────────────────────┐
│  Phase 1: 同步                      │
│                                     │
│  1. 读取 server.json                │
│  2. 同步 MC 版本                    │
│  3. 安装 Fabric/Forge               │
│  4. packwiz 同步模组               │
│  5. 写入 launcher_profiles.json     │
└──────────────┬──────────────────────┘
               ▼
┌─────────────────────────────────────┐
│  Phase 2: PCL2 集成                 │
│                                     │
│  1. 更新 PCL.ini                    │
│     ├─ Version = 我们的版本名       │
│     ├─ VersionCache = 当前时间戳    │
│     └─ 卡片注入（如果有版本卡片）   │
│                                     │
│  2. 更新注册表（可选）              │
│     ├─ 写入 LaunchFolderSelect      │
│     └─ 让 PCL2 下次直接选中我们     │
└──────────────┬──────────────────────┘
               ▼
┌─────────────────────────────────────┐
│  Phase 3: 拉起 PCL2                 │
│                                     │
│  1. 启动 PCL2.exe                   │
│  2. 等待 PCL2 退出（可选）         │
│  3. starter 退出                    │
└─────────────────────────────────────┘
```

### 5.2 交互模式 vs 静默模式

| 模式 | 触发条件 | 行为 |
|---|---|---|
| **交互模式** | 无 `--headless` flag | 找不到 PCL2/.minecraft 时弹窗让用户选 |
| **静默模式** | `--headless` 或 `run` 子命令 | 找不到就报错退出，不抛交互窗口 |

```go
// 启动入口
func main() {
    cfg := loadConfig()
    needsInteractive := !cfg.Headless && isTerminal()
    
    // Phase 0: 找 PCL2
    pcl2Path, err := FindPCL2()
    if err != nil {
        if needsInteractive {
            pcl2Path = promptUser("请输入 PCL2.exe 所在路径:")
        } else {
            return fmt.Errorf("未找到 PCL2: %w", err)
        }
    }
    
    // 找 .minecraft
    mcDir, err := FindMinecraftDir(pcl2Path)
    if err != nil {
        if needsInteractive {
            mcDir = promptUser("请输入 .minecraft 文件夹路径:")
        } else {
            return fmt.Errorf("未找到 .minecraft: %w", err)
        }
    }
    
    // Phase 1: 同步
    sync(cfg, mcDir)
    
    // Phase 2: 集成
    updatePCLIni(mcDir, cfg.PCL2VersionName)
    
    // Phase 3: 拉起 PCL2
    launchPCL2(pcl2Path)
}
```

### 5.3 用户路径选择界面

**Windows GUI 文件夹选择对话框**（不是让用户手敲路径）：

```go
import "syscall"
import "unsafe"

// 使用 Windows Shell API 打开文件夹选择对话框
// 不需要额外的 GUI 库，直接调用 shell32.dll 的 PickFolderDialog

func pickFolderDialog(title string) (string, bool) {
    // 方法 1: 使用 ole32 的 CoCreateInstance + IFileOpenDialog (Windows Vista+)
    // 这是最推荐的方式，原生的 Windows 文件夹选择器
    
    hr := ole32.CoInitializeEx(0, 2) // COINIT_APARTMENTTHREADED
    if FAILED(hr) {
        return pickFolderFallback(title) // 回退到命令行
    }
    defer ole32.CoUninitialize()
    
    // 创建 IFileOpenDialog 实例
    var foid *IFileOpenDialog
    hr = ole32.CoCreateInstance(
        &CLSID_FileOpenDialog, nil, 1, // CLSCTX_INPROC_SERVER
        &IID_IFileOpenDialog, unsafe.Pointer(&foid))
    if FAILED(hr) {
        return pickFolderFallback(title)
    }
    defer foid.Release()
    
    // 设置为文件夹选择模式（不是文件选择）
    opts, _ := foid.GetOptions()
    foid.SetOptions(opts | FOS_PICKFOLDERS)
    
    // 设置标题
    foid.SetTitle(StringToUTF16Ptr(title))
    
    // 显示对话框
    hr = foid.Show(0) // 父窗口句柄 = 0
    if FAILED(hr) {
        return "", false // 用户点了取消
    }
    
    // 获取选中的路径
    var psi *IShellItem
    foid.GetResult(&psi)
    defer psi.Release()
    
    pathBuf := make([]uint16, 260)
    psi.GetDisplayName(SIGDN_FILESYSPATH, &pathBuf[0])
    path := UTF16ToString(pathBuf)
    
    return path, true
}

// 简化版本：使用 Shell32.SHBrowseForFolder (Windows 2000+, 更简单)
func pickFolderDialogSimple(title string) (string, bool) {
    // 通过调用 shell32 的 SHBrowseForFolderW API
    // 这是经典的文件选择对话框，在所有 Windows 版本上工作
    
    var bi BROWSEINFOW
    bi.hwndOwner = 0
    bi.pidlRoot = 0
    bi.lpszTitle = StringToUTF16Ptr(title)
    bi.ulFlags = BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE
    
    pidl := shell32.SHBrowseForFolder(&bi)
    if pidl == 0 {
        return "", false // 用户取消
    }
    defer ole32.CoTaskMemFree(pidl)
    
    pathBuf := make([]uint16, 260)
    shell32.SHGetPathFromIDListW(pidl, &pathBuf[0])
    path := UTF16ToString(pathBuf)
    
    if path == "" {
        return "", false
    }
    
    return path, true
}

// 无终端时回退到命令行输入
func pickFolderFallback(title string) (string, bool) {
    fmt.Printf("\n[?] %s\n", title)
    fmt.Print("    > ")
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    path := strings.TrimSpace(input)
    if path == "" {
        return "", false
    }
    // 处理拖拽时的引号包裹
    path = strings.Trim(path, "\"'")
    return path, true
}

// 对外统一接口
func promptFolderPath(title string) (string, bool) {
    if isWindows() {
        return pickFolderDialogSimple(title)
    }
    return pickFolderFallback(title)
}
```

**用户看到的交互效果**：

```
=== MC-Starter v1.0 ===

[!] 未检测到 Minecraft 文件夹
[?] 请选择操作:
    1) 自动搜索常见位置
    2) 手动选择文件夹 (推荐)  ← 弹出 Windows 文件夹选择对话框
    3) 退出
    > 2

（弹出了 Windows 原生文件夹选择框）
┌──────────────────────────────────────────────┐
│  选择 Minecraft 游戏目录                       │
│                                              │
│  📂 Desktop                                  │
│  📂 Downloads                                │
│  📂 Documents                                │
│  📂 D:\Game\MC\PCL2\                         │
│  📂 D:\Game\MC\PCL2\.minecraft\              │ ← 用户选中这个
│  📂 D:\Game\MC\MyModPack\                    │
│                                              │
│           [确定]       [取消]                 │
└──────────────────────────────────────────────┘

[检测] 正在检查 D:\Game\MC\MyModPack\...
  ...（后续流程）
```

**路径合法性校验**：

```go
func validateSelectedFolder(path string) (string, error) {
    // 1. 去除引号和空白
    path = strings.TrimSpace(path)
    path = strings.Trim(path, "\"'")
    
    if path == "" {
        return "", fmt.Errorf("路径不能为空")
    }
    
    // 2. 检查路径长度
    if len(path) > 260 {
        return "", fmt.Errorf("路径过长（最大 260 字符）")
    }
    
    // 3. 检查非法字符
    illegal := []string{"<", ">", "\"", "|", "?", "*", "\x00"}
    for _, c := range illegal {
        if strings.Contains(path, c) {
            return "", fmt.Errorf("路径包含非法字符: %s", c)
        }
    }
    
    // 4. 检查路径是否存在
    info, err := os.Stat(path)
    if err != nil {
        if os.IsNotExist(err) {
            return "", fmt.Errorf("路径不存在: %s", path)
        }
        return "", fmt.Errorf("无法访问路径: %w", err)
    }
    
    // 5. 必须是目录
    if !info.IsDir() {
        return "", fmt.Errorf("请选择文件夹，不是文件")
    }
    
    // 6. 检查写权限
    testFile := filepath.Join(path, ".mc-starter-write-test")
    if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
        return "", fmt.Errorf("没有写入权限: %w", err)
    }
    os.Remove(testFile)
    
    // 7. 规范化路径（统一分隔符，补充末尾斜杠）
    path = filepath.Clean(path)
    path = path + string(filepath.Separator)
    
    return path, nil
}
```

### 5.4 选择 PCL2.exe 位置

同理，用 Windows GUI 对话框选择文件（不是文件夹）：

```go
func pickPCL2Path() (string, bool) {
    // 使用 IFileOpenDialog（文件选择模式，非文件夹模式）
    // 过滤条件：只显示 .exe 文件
    // 默认文件名提示：Plain Craft Launcher 2.exe
    
    if isWindows() {
        return pickFileDialog(
            "选择 Plain Craft Launcher 2.exe",
            "可执行文件 (*.exe)\0*.exe\0所有文件 (*.*)\0*.*\0",
        )
    }
    return pickFolderFallback("请输入 PCL2.exe 路径:")
}

// 交互效果：
//
// [!] 未检测到 PCL2
// [?] 请选择操作:
//     1) 自动搜索常见位置
//     2) 手动选择 PCL2.exe  ← 弹出文件选择对话框
//     3) 退出
//     > 2
//
// （弹出 Windows 文件选择框，默认筛选 *.exe）
// 用户选中 Plain Craft Launcher 2.exe
// → 校验路径
// → 继续
```

### 5.5 完整搜索结果 + 验证流程

```
┌─────────────────────────────────────────────┐
│  搜索 PCL2.exe                               │
│  ├─ 自动搜索命中 → 验证路径                   │
│  ├─ 没找到 → Windows 文件选择框              │
│  │     → 用户选了一个文件                     │
│  │     → 验证：文件名是 PCL2?.exe?            │
│  │     → 验证：文件可访问                     │
│  │     → 通过 → 继续                         │
│  │     → 不通过 → 提示用户重新选择            │
│  └─ 用户取消 → 退出                          │
└────────────────┬────────────────────────────┘
                 ▼
┌─────────────────────────────────────────────┐
│  搜索 .minecraft                              │
│  ├─ 自动搜索命中 → 验证（versions/ 存在）     │
│  ├─ 没找到 → Windows 文件夹选择框             │
│  │     → 用户选了一个文件夹                    │
│  │     → Step 1: 扫描此目录及子目录下         │
│  │     │   是否有 versions/ 文件夹            │
│  │     │   ├─ 有 → 识别为 MC 目录            │
│  │     │   └─ 没有 → 判断是否要创建           │
│  │     │                                      │
│  │     → Step 2: 检查是否包含我们的整合包      │
│  │     │   ├─ versions/{versionName}/          │
│  │     │   └─ 没有 → 标记为新装               │
│  │     │                                      │
│  │     → Step 3: 检查 PCL2 注册表             │
│  │     │   是否已有这个文件夹                  │
│  │     │   ├─ 有 → 跳过                       │
│  │     │   └─ 没有 → 写入注册表               │
│  │     │                                      │
│  │     → Step 4: 路径合法性验证               │
│  │         ├─ 存在                           │
│  │         ├─ 是目录                          │
│  │         ├─ 可写入                          │
│  │         ├─ 不超过路径长度限制               │
│  │         └─ 无非法字符                      │
│  │                                            │
│  └─ 用户取消 → 退出                           │
└────────────────┬────────────────────────────┘
                 ▼
           继续执行 sync
```

### 5.6 路径验证合并函数

```go
func searchMinecraftDirInteractive(pcl2Path string) (string, error) {
    // 先尝试自动搜索
    mcDir, err := findMinecraftDirAuto(pcl2Path)
    if err == nil {
        // 自动搜索找到后，自动注册到 PCL2
        registerIfNotInPCL2(pcl2Path, mcDir)
        return mcDir, nil
    }
    
    // 自动搜索失败 → 交互模式
    if isHeadlessMode() {
        return "", fmt.Errorf("未找到 Minecraft 文件夹（静默模式）")
    }
    
    for {
        fmt.Println("\n[!] 未找到 Minecraft 文件夹")
        fmt.Println("[?] 请选择操作:")
        fmt.Println("    1) 自动搜索常见位置（重试）")
        fmt.Println("    2) 手动选择文件夹")
        fmt.Println("    3) 退出")
        fmt.Print("    > ")
        
        var choice string
        fmt.Scanln(&choice)
        
        switch choice {
        case "1":
            mcDir, err = findMinecraftDirAuto(pcl2Path)
            if err == nil {
                registerIfNotInPCL2(pcl2Path, mcDir)
                return mcDir, nil
            }
            fmt.Println("  自动搜索仍然没有结果，请手动选择")
            
        case "2":
            // 弹出 Windows 原生文件夹选择框
            selectedPath, ok := promptFolderPath(
                "请选择 Minecraft 游戏目录或整合包文件夹")
            if !ok {
                fmt.Println("  已取消")
                continue
            }
            
            // 执行完整的目录判断流程
            mcDir, err = handleUserSelectedFolder(selectedPath, pcl2Path)
            if err != nil {
                fmt.Printf("  ⚠ 路径验证失败: %v\n", err)
                fmt.Println("  请重新选择")
                continue
            }
            return mcDir, nil
            
        case "3":
            os.Exit(0)
            
        default:
            fmt.Println("  无效选择，请重新输入")
        }
    }
}

func handleUserSelectedFolder(selectedPath string, pcl2Path string) (string, error) {
    // Step 0: 路径合法性验证
    validPath, err := validateSelectedFolder(selectedPath)
    if err != nil {
        return "", fmt.Errorf("路径不合法: %w", err)
    }
    selectedPath = validPath
    pcl2Dir := filepath.Dir(pcl2Path)
    
    // ───────────────────────────────────────────
    // Step 1: 扫描这个文件夹及子目录是否包含 versions/
    // ───────────────────────────────────────────
    fmt.Println("\n[检测] 正在扫描文件夹...")
    
    var mcDir string
    var isMcDir bool
    
    // 先检查所选文件夹本身
    if hasVersions(selectedPath) {
        mcDir = selectedPath
        isMcDir = true
        fmt.Println("  ✓ 包含 versions/ → 有效 Minecraft 目录")
    } else {
        // 递归搜索子目录（1 层深度）
        fmt.Println("  所选文件夹没有 versions/，正在搜索子目录...")
        entries, _ := os.ReadDir(selectedPath)
        found := false
        for _, e := range entries {
            if !e.IsDir() {
                continue
            }
            subPath := filepath.Join(selectedPath, e.Name()) + "\\"
            if hasVersions(subPath) {
                mcDir = subPath
                isMcDir = true
                found = true
                fmt.Printf("  ✓ 在子目录 %s 中找到 versions/\n", e.Name())
                break
            }
            // 跳过 .minecraft 命名的子目录（直接识别）
            subMc := filepath.Join(selectedPath, e.Name(), ".minecraft") + "\\"
            if hasVersions(subMc) {
                mcDir = subMc
                isMcDir = true
                found = true
                fmt.Printf("  ✓ 在子目录 %s\\.minecraft 中找到 versions/\n", e.Name())
                break
            }
        }
        if !found {
            // 完全没有 → 使用所选目录，创建 .minecraft/versions/
            fmt.Println("  ⚠ 未找到现有 Minecraft 目录")
            mcDir = filepath.Join(selectedPath, ".minecraft") + "\\"
            os.MkdirAll(filepath.Join(mcDir, "versions"), 0755)
            isMcDir = true
            fmt.Printf("  📁 将在 %s 创建新的 Minecraft 目录\n", mcDir)
        }
    }
    
    // ───────────────────────────────────────────
    // Step 2: 检查是否包含我们的整合包
    // ───────────────────────────────────────────
    versionName := getOurVersionName()
    if versionName != "" {
        versionDir := filepath.Join(mcDir, "versions", versionName)
        if _, err := os.Stat(versionDir); err == nil {
            fmt.Println("  ✓ 已检测到整合包版本")
        } else {
            fmt.Println("  ⚠ 未检测到整合包版本（后续同步将创建）")
        }
    }
    
    // ───────────────────────────────────────────
    // Step 3: 检查 PCL2 是否已注册此文件夹
    // ───────────────────────────────────────────
    if !isFolderRegisteredInPCL2(mcDir) {
        err := registerFolderInPCL2(pcl2Path, mcDir)
        if err != nil {
            fmt.Printf("  ⚠ 注册到 PCL2 失败（不影响使用）: %v\n", err)
        } else {
            fmt.Println("  ✓ 已注册到 PCL2 启动器文件夹列表")
            fmt.Println("    下次打开 PCL2 即可看到新版本")
        }
    } else {
        fmt.Println("  ✓ PCL2 已识别此目录")
    }
    
    fmt.Println()
    return mcDir, nil
}
```

### 5.7 搜索 PCL2.exe 的选择对话框

```go
func searchPCL2Interactive() (string, error) {
    // 先自动搜索
    pcl2Path, err := FindPCL2()
    if err == nil {
        return pcl2Path, nil
    }
    
    if isHeadlessMode() {
        return "", fmt.Errorf("未找到 PCL2（静默模式）")
    }
    
    for {
        fmt.Println("\n[!] 未检测到 Plain Craft Launcher 2.exe")
        fmt.Println("[?] 请选择操作:")
        fmt.Println("    1) 自动搜索（扫描当前目录及子目录）")
        fmt.Println("    2) 手动选择 PCL2.exe 文件")
        fmt.Println("    3) 退出")
        fmt.Print("    > ")
        
        var choice string
        fmt.Scanln(&choice)
        
        switch choice {
        case "1":
            pcl2Path, err = FindPCL2()
            if err == nil {
                return pcl2Path, nil
            }
            fmt.Println("  仍未找到，请手动选择")
        
        case "2":
            // 弹出 Windows 原生文件选择框
            selectedFile, ok := pickPCL2Path()
            if !ok {
                fmt.Println("  已取消")
                continue
            }
            
            // 验证
            info, err := os.Stat(selectedFile)
            if err != nil {
                fmt.Printf("  ⚠ 无法访问文件: %v\n", err)
                continue
            }
            if info.IsDir() {
                fmt.Println("  ⚠ 请选择 PCL2.exe 文件，不是文件夹")
                continue
            }
            if !strings.HasSuffix(strings.ToLower(info.Name()), ".exe") {
                fmt.Println("  ⚠ 请选择 .exe 文件")
                continue
            }
            
            fmt.Printf("  ✓ 已选择: %s\n", selectedFile)
            return selectedFile, nil
            
        case "3":
            os.Exit(0)
            
        default:
            fmt.Println("  无效选择")
        }
    }
}
```

---

## 六、自动拉起 PCL2

```go
func launchPCL2(pcl2Path string) error {
    // 判断是否需要拉起（headless 模式不拉）
    if config.Headless {
        fmt.Println("同步完成。PCL2 位于:", pcl2Path)
        return nil
    }
    
    // 检查 PCL2 是否已经在运行
    if isProcessRunning("Plain Craft Launcher 2.exe") {
        fmt.Println("PCL2 已在运行，请刷新版本列表")
        return nil
    }
    
    // 启动 PCL2
    cmd := exec.Command(pcl2Path)
    cmd.Dir = filepath.Dir(pcl2Path)  // 工作目录设为 PCL2 所在目录
    
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("启动 PCL2 失败: %w", err)
    }
    
    fmt.Printf("✅ 已启动 PCL2: %s\n", pcl2Path)
    
    // 等待 PCL2 退出（可选：starter 也退出）
    // cmd.Wait()
    
    return nil
}

func isProcessRunning(name string) bool {
    // Windows: tasklist /FI "IMAGENAME eq Plain Craft Launcher 2.exe"
    cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", name))
    output, _ := cmd.Output()
    return strings.Contains(string(output), name)
}
```

---

## 七、搜索 PCL2 的完整算法

```go
func FindPCL2() (string, error) {
    // 优先级 1: local.json 中已保存的路径
    if config.PCL2Path != "" {
        if _, err := os.Stat(config.PCL2Path); err == nil {
            return config.PCL2Path, nil
        }
    }
    
    // 优先级 2: 当前目录及子目录
    exeDir := filepath.Dir(os.Args[0])
    
    // 广度搜索：当前目录 → 1 层子目录 → 2 层子目录
    paths := []string{
        filepath.Join(exeDir, "Plain Craft Launcher 2.exe"),
    }
    
    // 搜索子目录
    entries, _ := os.ReadDir(exeDir)
    for _, e := range entries {
        if e.IsDir() {
            paths = append(paths,
                filepath.Join(exeDir, e.Name(), "Plain Craft Launcher 2.exe"),
            )
            // 搜索子目录的子目录
            subEntries, _ := os.ReadDir(filepath.Join(exeDir, e.Name()))
            for _, se := range subEntries {
                if se.IsDir() {
                    paths = append(paths,
                        filepath.Join(exeDir, e.Name(), se.Name(), "Plain Craft Launcher 2.exe"),
                    )
                }
            }
        }
    }
    
    for _, p := range paths {
        if _, err := os.Stat(p); err == nil {
            return p, nil
        }
    }
    
    // 优先级 3: 父目录
    parentDir := filepath.Dir(exeDir)
    parentPath := filepath.Join(parentDir, "Plain Craft Launcher 2.exe")
    if _, err := os.Stat(parentPath); err == nil {
        return parentPath, nil
    }
    
    // 优先级 4: 桌面/下载常见位置（Windows）
    home, _ := os.UserHomeDir()
    commonPaths := []string{
        filepath.Join(home, "Desktop", "Plain Craft Launcher 2.exe"),
        filepath.Join(home, "Downloads", "Plain Craft Launcher 2.exe"),
        filepath.Join(home, "Documents", "Plain Craft Launcher 2.exe"),
        filepath.Join(home, "AppData", "Local", "PCL", "Plain Craft Launcher 2.exe"),
    }
    for _, p := range commonPaths {
        if _, err := os.Stat(p); err == nil {
            return p, nil
        }
    }
    
    return "", fmt.Errorf("未找到 Plain Craft Launcher 2.exe")
}
```

---

## 八、确定 .minecraft 目录 + 注册到 PCL2

### 8.1 整体搜索策略

搜索 `.minecraft` 的优先级（从上到下，命中即停）：

```
1. local.json 中已保存的 .minecraft 路径
2. PCL2 同目录下的 .minecraft/
3. PCL2 同目录的子目录中的 .minecraft/
4. PCL2 同目录下子目录本身就是 .minecraft（重命名情况）
5. 官方启动器目录 %APPDATA%/.minecraft/
6. 扫描 PCL2 注册表中的 LaunchFolders 列表
7. 搜索失败 → 让用户手动选择文件夹
```

### 8.2 用户手动添加文件夹后的全流程判断

```go
// 用户手动选择了一个文件夹后，更新器执行以下判断
func handleUserSelectedFolder(selectedPath string, pcl2Path string) (string, error) {
    // 规范化路径
    selectedPath = strings.TrimRight(selectedPath, "\\/") + "\\"
    
    // ─────────────────────────────────────────────────────────
    // 第一步：检查这个文件夹本身是否是一个 MC 游戏目录
    // ─────────────────────────────────────────────────────────
    isMcDir := hasVersions(selectedPath)
    
    // 如果选中了 .minecraft 里面，尝试往上层找真正的 .minecraft
    mcDir := selectedPath
    if !isMcDir && strings.HasPrefix(selectedPath, filepath.Join(pcl2Dir, ".minecraft")) {
        // 用户选了 .minecraft/versions/ 或里面的东西 → 往上回溯到 .minecraft
        parent := filepath.Dir(strings.TrimRight(selectedPath, "\\"))
        for parent != pcl2Dir && parent != "." {
            if hasVersions(parent) {
                mcDir = parent
                isMcDir = true
                break
            }
            parent = filepath.Dir(parent)
        }
    }
    
    // ─────────────────────────────────────────────────────────
    // 第二步：判断这个目录是否包含我们的整合包
    // ─────────────────────────────────────────────────────────
    hasOurPack := false
    if isMcDir {
        // 检查 versions/ 下是否有我们的版本
        versionName := getOurVersionName()
        versionDir := filepath.Join(mcDir, "versions", versionName)
        if _, err := os.Stat(versionDir); err == nil {
            hasOurPack = true
        }
    }
    
    // ─────────────────────────────────────────────────────────
    // 第三步：判断 PCL2 是否已经注册了这个文件夹
    // ─────────────────────────────────────────────────────────
    registered := isFolderRegisteredInPCL2(mcDir)
    
    // ─────────────────────────────────────────────────────────
    // 第四步：根据情况决定动作
    // ─────────────────────────────────────────────────────────
    if !isMcDir {
        // 选中的文件夹没有 versions/ → 当作要创建新整合包的目录
        newMcDir := filepath.Join(mcDir, ".minecraft")
        os.MkdirAll(filepath.Join(newMcDir, "versions"), 0755)
        
        fmt.Printf("📁 将在 %s 创建新的 Minecraft 目录\n", newMcDir)
        
        // 注册到 PCL2
        if !registered {
            registerFolderInPCL2(pcl2Path, newMcDir)
            fmt.Println("  ✓ 已注册到 PCL2 启动器文件夹列表")
        }
        
        return newMcDir, nil
    }
    
    // 已识别为有效 MC 目录
    fmt.Printf("📁 已识别 Minecraft 目录: %s\n", mcDir)
    
    if hasOurPack {
        fmt.Println("  ✓ 检测到整合包版本，可直接使用")
    }
    
    if !registered {
        // 关键：PCL2 没有注册这个文件夹 → 帮用户注册
        registerFolderInPCL2(pcl2Path, mcDir)
        fmt.Println("  ✓ 已注册到 PCL2 启动器文件夹列表")
    }
    
    return mcDir, nil
}
```

### 8.3 注册文件夹到 PCL2（注册表操作）

```go
func registerFolderInPCL2(pcl2Path string, mcDir string) error {
    mcDir = strings.TrimRight(mcDir, "\\/") + "\\"
    pcl2Dir := filepath.Dir(pcl2Path)
    
    // 打开 PCL2 注册表键
    k, err := registry.OpenKey(registry.CURRENT_USER,
        `Software\PCL`,
        registry.SET_VALUE|registry.QUERY_VALUE|registry.WRITE)
    if err != nil {
        // 注册表键可能不存在，创建它
        k, _, err = registry.CreateKey(registry.CURRENT_USER,
            `Software\PCL`,
            registry.SET_VALUE|registry.QUERY_VALUE|registry.WRITE)
        if err != nil {
            return fmt.Errorf("无法打开 PCL2 注册表: %w", err)
        }
    }
    defer k.Close()
    
    // ── 1. 读取现有的 LaunchFolders ──
    existing, _, _ := k.GetStringValue("LaunchFolders")
    var folders []string
    if existing != "" {
        folders = strings.Split(existing, "|")
    }
    
    // ── 2. 检查是否已存在 ──
    mcDirNormalized := strings.ReplaceAll(mcDir, "/", "\\")
    for _, f := range folders {
        if f == "" {
            continue
        }
        parts := strings.SplitN(f, ">", 2)
        if len(parts) == 2 && parts[1] == mcDirNormalized {
            Log("文件夹已在 PCL2 列表中，跳过注册: %s", mcDirNormalized)
            // 但仍然更新 LaunchFolderSelect 到当前目录
            updateLaunchFolderSelect(k, pcl2Dir, mcDirNormalized)
            return nil
        }
    }
    
    // ── 3. 组装新条目 ──
    // 文件夹命名规则：
    //   如果是 PCL2 同目录的 .minecraft → 名称 = ".minecraft"
    //   如果是子目录 → 名称 = 子目录名
    //   如果是外部目录 → 名称 = 整合包名
    folderName := getFolderDisplayName(pcl2Dir, mcDirNormalized)
    entry := folderName + ">" + mcDirNormalized
    
    folders = append(folders, entry)
    newValue := strings.Join(folders, "|")
    
    // ── 4. 写入注册表 ──
    if err := k.SetStringValue("LaunchFolders", newValue); err != nil {
        return fmt.Errorf("写入 LaunchFolders 注册表失败: %w", err)
    }
    
    // ── 5. 设置为当前选中 ──
    updateLaunchFolderSelect(k, pcl2Dir, mcDirNormalized)
    
    Log("已注册 Minecraft 文件夹到 PCL2: %s > %s", folderName, mcDirNormalized)
    return nil
}

func getFolderDisplayName(pcl2Dir string, mcDir string) string {
    pcl2Dir = strings.TrimRight(strings.ReplaceAll(pcl2Dir, "/", "\\"), "\\") + "\\"
    mcDir = strings.TrimRight(mcDir, "\\") + "\\"
    
    // 同目录 .minecraft
    if mcDir == pcl2Dir + ".minecraft\\" {
        return ".minecraft"
    }
    
    // PCL2 子目录
    if strings.HasPrefix(mcDir, pcl2Dir) {
        relative := strings.TrimPrefix(mcDir, pcl2Dir)
        parts := strings.Split(relative, "\\")
        return parts[0]
    }
    
    // 外部目录 → 使用整合包名称
    packName := config.PackName
    if packName == "" {
        // 用目录名
        dirName := filepath.Base(strings.TrimRight(mcDir, "\\"))
        if dirName == ".minecraft" {
            dirName = filepath.Base(filepath.Dir(strings.TrimRight(mcDir, "\\")))
        }
        packName = dirName
    }
    return packName
}

func updateLaunchFolderSelect(k registry.Key, pcl2Dir string, mcDir string) {
    pcl2Dir = strings.TrimRight(strings.ReplaceAll(pcl2Dir, "/", "\\"), "\\") + "\\"
    mcDir = strings.TrimRight(mcDir, "\\") + "\\"
    
    // 如果 .minecraft 在 PCL2 目录下，用 $ 代指 PCL2 目录
    var selectValue string
    if strings.HasPrefix(mcDir, pcl2Dir) {
        relative := strings.TrimPrefix(mcDir, pcl2Dir)
        selectValue = "$" + relative
    } else {
        selectValue = mcDir
    }
    
    k.SetStringValue("LaunchFolderSelect", selectValue)
    Log("已设置 PCL2 当前文件夹: %s", selectValue)
}

func isFolderRegisteredInPCL2(mcDir string) bool {
    k, err := registry.OpenKey(registry.CURRENT_USER,
        `Software\PCL`,
        registry.QUERY_VALUE)
    if err != nil {
        return false
    }
    defer k.Close()
    
    mcDir = strings.TrimRight(strings.ReplaceAll(mcDir, "/", "\\"), "\\") + "\\"
    
    existing, _, err := k.GetStringValue("LaunchFolders")
    if err != nil || existing == "" {
        return false
    }
    
    for _, f := range strings.Split(existing, "|") {
        parts := strings.SplitN(f, ">", 2)
        if len(parts) == 2 && parts[1] == mcDir {
            return true
        }
    }
    return false
}
```

### 8.4 自动搜索时也注册到 PCL2

```go
func FindMinecraftDir(pcl2Path string) (string, error) {
    pcl2Dir := filepath.Dir(pcl2Path)
    
    // 规则 1: PCL2 同目录下 .minecraft/
    mcDir := filepath.Join(pcl2Dir, ".minecraft\\")
    if hasVersions(mcDir) {
        registerIfNotInPCL2(pcl2Path, mcDir)
        return mcDir, nil
    }
    
    // 规则 2: PCL2 同目录的子目录中有 .minecraft/
    entries, _ := os.ReadDir(pcl2Dir)
    for _, e := range entries {
        if e.IsDir() {
            subMc := filepath.Join(pcl2Dir, e.Name(), ".minecraft\\")
            if hasVersions(subMc) {
                registerIfNotInPCL2(pcl2Path, subMc)
                return subMc, nil
            }
        }
    }
    
    // 规则 3: 官方启动器目录
    appData := os.Getenv("APPDATA")
    if appData != "" {
        official := filepath.Join(appData, ".minecraft\\")
        if hasVersions(official) {
            registerIfNotInPCL2(pcl2Path, official)
            return official, nil
        }
    }
    
    // 规则 4: PCL2 同目录下子目录本身就是 .minecraft（重命名情况）
    for _, e := range entries {
        if e.IsDir() {
            subDir := filepath.Join(pcl2Dir, e.Name(), "\\")
            if hasVersions(subDir) {
                registerIfNotInPCL2(pcl2Path, subDir)
                return subDir, nil
            }
        }
    }
    
    // 规则 5: 扫描 PCL2 注册表已有的 LaunchFolders
    registeredDir := findRegisteredPCL2Folder(pcl2Dir)
    if registeredDir != "" {
        registerIfNotInPCL2(pcl2Path, registeredDir)
        return registeredDir, nil
    }
    
    // 规则 6: 搜索常见位置
    // (桌面、下载等位置的 .minecraft)
    home, _ := os.UserHomeDir()
    commonPaths := []string{
        filepath.Join(home, "Desktop", ".minecraft"),
        filepath.Join(home, "Downloads", ".minecraft"),
        filepath.Join(home, "Documents", ".minecraft"),
    }
    for _, p := range commonPaths {
        mcDir := p + "\\"
        if hasVersions(mcDir) {
            registerIfNotInPCL2(pcl2Path, mcDir)
            return mcDir, nil
        }
    }
    
    // 都没找到 → 创建一个新的
    newMcDir := filepath.Join(pcl2Dir, ".minecraft\\")
    os.MkdirAll(filepath.Join(newMcDir, "versions"), 0755)
    registerIfNotInPCL2(pcl2Path, newMcDir)
    return newMcDir, nil
}

func registerIfNotInPCL2(pcl2Path, mcDir string) {
    if !isFolderRegisteredInPCL2(mcDir) {
        err := registerFolderInPCL2(pcl2Path, mcDir)
        if err != nil {
            Log("注册到 PCL2 失败（不影响使用）: %v", err)
        }
    }
}

func findRegisteredPCL2Folder(pcl2Dir string) string {
    k, err := registry.OpenKey(registry.CURRENT_USER,
        `Software\PCL`,
        registry.QUERY_VALUE)
    if err != nil {
        return ""
    }
    defer k.Close()
    
    existing, _, err := k.GetStringValue("LaunchFolders")
    if err != nil || existing == "" {
        return ""
    }
    
    for _, f := range strings.Split(existing, "|") {
        parts := strings.SplitN(f, ">", 2)
        if len(parts) == 2 {
            path := parts[1]
            if hasVersions(path) {
                // 检查 versions/ 下是否有我们的版本
                versionName := getOurVersionName()
                if versionName != "" {
                    versionDir := filepath.Join(path, "versions", versionName)
                    if _, err := os.Stat(versionDir); err == nil {
                        return path
                    }
                }
                // 没指定版本名也先用第一个有 versions/ 的
                return path
            }
        }
    }
    return ""
}

func hasVersions(dir string) bool {
    info, err := os.Stat(filepath.Join(dir, "versions"))
    return err == nil && info.IsDir()
}
```

---

## 九、用户手动选择文件夹后的完整流程

```
用户手动选择了一个文件夹路径
    │
    ▼
┌───────────────────────────────────────────────┐
│  Step 1: 检查是否包含 versions/                │
│                                               │
│  包含 → 识别为有效 MC 目录 → Step 2            │
│  不包含 → 在选中路径下创建 .minecraft/versions/ │
│         → Step 3                               │
└──────────────────┬────────────────────────────┘
                   ▼
┌───────────────────────────────────────────────┐
│  Step 2: 检查 versions/ 下是否有我们的整合包    │
│                                               │
│  有 → 直接可用                                 │
│  没有 → 后续 sync 时会创建                     │
└──────────────────┬────────────────────────────┘
                   ▼
┌───────────────────────────────────────────────┐
│  Step 3: 检查 PCL2 是否已注册此文件夹           │
│                                               │
│  已注册 → 跳过                                 │
│  未注册 → 写入注册表 LaunchFolders +            │
│           设置 LaunchFolderSelect              │
│           让 PCL2 下次启动就能看到              │
└──────────────────┬────────────────────────────┘
                   ▼
           继续执行 sync
```

### 用户交互示例

```
[?] 未找到 Minecraft 文件夹
[?] 请选择操作:
    1) 自动搜索常见位置
    2) 输入 Minecraft 文件夹路径
    3) 退出
    > 2

[?] 请输入或拖拽文件夹路径:
    > D:\Game\MC\MyModPack

[检测] 正在检查 D:\Game\MC\MyModPack\...
  ✓ 包含 versions/ → 有效 Minecraft 目录
  ⚠ 未检测到整合包版本（后续同步将创建）
  ✓ 已注册到 PCL2 启动器文件夹列表
  
同步完成后打开 PCL2 即可看到新版本！
```

---

## 九、命令行设计更新

### 主命令：无参运行 = 全自动模式

```bash
# 最简单的用法：直接双击 / 直接运行
starter
  → 等价于 starter run（全自动模式）

# 子命令
starter run                ← 全自动：检测 → 同步 → 集成 → 拉起 PCL2（默认）
starter run --headless     ← 静默模式：不交互、不拉 PCL2
starter init               ← 初始化配置
starter check              ← 检查环境
starter sync               ← 仅同步，不集成、不拉 PCL2
starter pcl detect         ← 仅检测 PCL2 路径
starter pcl path <path>    ← 手动设置 PCL2 路径
```

### 入口逻辑

```go
func main() {
    if len(os.Args) == 1 {
        // 无参数 = run 模式
        run()
    }
    // 解析子命令...
}
```

### "无感"的完整定义

```
无参双击 starter.exe：
  ✓ 不需要用户知道什么叫 PCL2、.minecraft、整合包
  ✓ 不需要用户开命令行输命令
  ✓ 找不到东西 → 弹提示让用户选文件夹
  ✓ 同步完自动打开 PCL2
  ✓ 用户看到的唯一变化：PCL2 里多了一个版本
```

### 开箱即用路径总结

| 步骤 | 自动 | 需要用户 |
|---|---|---|
| 找 PCL2.exe | 搜索当前目录+子目录+父目录+桌面+下载 | 找不到才让用户选 |
| 找 .minecraft | PCL2 同目录/子目录/%APPDATA% | 找不到才让用户选 |
| 下载 MC | 自动从镜像拉 | 无 |
| 安装 Fabric | 自动 | 无 |
| 同步模组 | 自动 | 无 |
| 写 PCL.ini | 自动注入版本和卡片 | 无 |
| 拉起 PCL2 | 自动 | 无 |
