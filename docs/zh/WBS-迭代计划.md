# MC 版本更新器 — WBS 工作分解 + 迭代计划

> 配套文档：立项报告.md、详细开发流程.md

---

## 一、总览

```
P0 CLI框架+配置 (2d)     →  P1 版本下载+仓库+zip (5d)  →  P2 Loader+启动+修复 (3.5d)
    ├─ P0.1 项目初始化         ├─ P1.1 版本清单        ├─ P2.1 Fabric 安装器
    ├─ P0.2 CLI 框架          ├─ P1.2 版本 Jar        ├─ P2.2 Fabric libraries
    ├─ P0.3 配置系统          ├─ P1.3 Asset 索引      ├─ P2.3 启动参数
    ├─ P0.4 镜像管理器        ├─ P1.4 Asset 文件      ├─ P2.4 裸启动执行
    ├─ P0.5 下载器            ├─ P1.5 Libraries       ├─ P2.5 Forge（降级）
    └─ P0.6 日志系统          ├─ P1.6 断点恢复        ├─ P2.6 备份系统
                              ├─ P1.7 仓库结构        ├─ P2.7 修复命令
                              ├─ P1.8 文件缓存        ├─ P2.8 崩溃检测
                              ├─ P1.9 增量同步        ├─ P2.9 静默守护
                              ├─ P1.10 快照回滚       ├─ P2.10 报告上传
                              ├─ P1.11 全局缓存       ├─ P2.11 修复重启
                              ├─ P1.12 zip下载解压     ├─ P2.12 TUI界面
                              ├─ P1.13 zip diff同步    ├─ P2.13 托盘入口
                              └─ P1.14 zip+仓库整合    ├─ P2.14 弹窗兜底
                                                       └─ P2.15 PCL刷新

P3 Java检测 (1d)          →  P4 启动器兼容 (3d)   →  P5 自更新 (2d)
    ├─ P3.1 路径检测            ├─ P4.1 PCL2 独立模式     ├─ P5.1 更新检查
    ├─ P3.2 版本校验            ├─ P4.2 PCL2 集成模式     ├─ P5.2 下载校验
    └─ P3.3 引导安装            ├─ P4.3 PCL.ini 读写      ├─ P5.3 替换重启
                                ├─ P4.4 HMCL 兼容         ├─ P5.4 回滚
                                └─ P4.5 官方启动器        ├─ P5.5 多通道
                                                           └─ P5.6 交互通知
```

---

## 二、完整 WBS

### P0：CLI 框架 + 配置系统（预估 2 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P0.1 | 项目初始化：go mod init + 目录结构 | 0.5h | — | go.mod + 空目录骨架 |
| P0.2 | CLI 框架：cobra 子命令树 + flag 解析 | 2h | P0.1 | cmd/starter/main.go |
| P0.3 | 配置系统：ServerConfig + LocalConfig 结构体、JSON 读写、默认值合并 | 4h | P0.1 | internal/config/ |
| P0.4 | 镜像管理器：多镜像列表 + 自动回退算法 | 2h | P0.3 | internal/mirror/ |
| P0.5 | 下载器：HTTP GET + 进度回调 + Hash 校验 + 超时 + ETag 缓存 | 4h | P0.1 | internal/downloader/ |
| P0.6 | 日志系统：分级输出 + 文件日志 + 颜色终端 | 1.5h | P0.2 | internal/logger/ |
| **P0 合计** | | **14h** | | |

**P0 验收**：`go build` 通过，`./starter --help` 输出完整，`./starter init` 生成 local.json

---

### P1：MC 版本下载 + Asset 管理（预估 3 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P1.1 | 版本清单同步：请求 Mojang API + 解析 + 缓存 | 3h | P0.3, P0.4 | internal/launcher/version.go |
| P1.2 | 版本 Jar 下载：version.json 解析 + client.jar 下载 | 4h | P1.1 | internal/launcher/version.go |
| P1.3 | Asset 索引同步：下载 asset index JSON | 1h | P1.2 | internal/launcher/asset.go |
| P1.4 | Asset 文件下载：并发下载 + SHA1 校验 | 6h | P1.3 | internal/launcher/asset.go |
| P1.5 | Libraries 下载：解析依赖树 + 下载 + 路径组织 | 4h | P1.2 | internal/launcher/library.go |
| P1.6 | 断点恢复：同步中断后支持继续（增量检测已下载文件） | 2h | P1.4, P1.5 | internal/launcher/sync.go |
| P1.7 | 本地仓库结构：repo.json + snapshots + files 目录 | 3h | P0.3 | internal/repo/repo.go |
| P1.8 | 文件缓存：hash 去重 + 缓存读写 + 完整性校验 | 2h | P1.7 | internal/repo/cache.go |
| P1.9 | 增量同步算法：diff 计算 + 增量快照生成 | 4h | P1.8 | internal/repo/diff.go |
| P1.10 | 快照回滚：从快照链还原指定版本 | 2h | P1.9 | internal/repo/rollback.go |
| P1.11 | 全局缓存（跨整合包复用） | 2h | P1.8 | internal/repo/global.go |
| P1.12 | zip 解包 + 文件扫描 + hash 计算（服务端） | 3h | P0.5 | internal/pack/zip.go |
| P1.13 | 新旧版本差异分析（服务端） | 3h | P1.12 | internal/pack/diff.go |
| P1.14 | 发布管理：draft/published 版本管理（服务端） | 2h | P1.13 | internal/pack/sync.go |
| P1.15 | 客户端增量更新（按 hash 拉单个文件） | 4h | P1.14 | internal/launcher/update.go |
| **P1 合计** | | **39h** | | |

**P1 验收**：首次 `./starter sync` 全量下载整合包 zip → 解压 → 后续只差异化同步

---

### P2：Fabric 安装 + 裸启动 + 修复（预估 3.5 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P2.1 | Fabric 安装器下载：BMCLAPI meta API 获取 | 2h | P0.4 | internal/launcher/fabric.go |
| P2.2 | Fabric libraries 组装：解析 meta profile JSON | 4h | P2.1 | internal/launcher/fabric.go |
| P2.3 | 启动参数生成：classpath + JVM args + MC args | 4h | P2.2 | internal/launcher/launch.go |
| P2.4 | 裸启动执行：exec java + 子进程管理 | 3h | P2.3 | internal/launcher/launch.go |
| P2.5 | Forge 支持（降级）：安装器 jar 执行 | 3h | P2.2 | internal/launcher/forge.go |
| P2.6 | 备份系统：CreateBackup + Rollback + 自动清理 | 4h | P2.4 | internal/repair/backup.go |
| P2.7 | 修复命令：repair 命令树 + 清理 + 全量同步 | 3h | P2.6 | internal/repair/repair.go |
| P2.8 | 崩溃检测：退出码 + 崩溃报告 + JVM hs_err | 2h | P2.4 | internal/repair/detector.go |
| P2.9 | 静默守护：后台轮询 + 日志监听 + 托盘 | 4h | P2.8 | internal/daemon/daemon.go |
| P2.10 | 崩溃报告上传：收集 + 上传 + 隐私确认 | 2h | P2.8 | internal/repair/upload.go |
| P2.11 | 一键修复重启：repair 后自动 launch | 2h | P2.7, P2.9 | internal/repair/run.go |
| P2.12 | 修复 TUI 界面：bubbletea 布局 + 选项交互 | 4h | P2.7 | internal/repair/tui.go |
| P2.13 | 托盘菜单添加入口：同步/修复/备份 | 2h | P2.9 | internal/daemon/tray.go |
| P2.14 | Windows 原生弹窗兜底（无终端时） | 2h | P2.8 | internal/repair/dialog.go |
| P2.15 | 修复后 PCL2 自动刷新 | 1h | P2.12, P4.3 | internal/repair/pcl.go |
| **P2 合计** | | **42h** | | |

**P2 验收**：`./starter run` 全流程 + `starter repair` GUI 界面 + 托盘快捷入口

---

### P3：Java 环境检测（预估 1 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P3.1 | 路径检测：JAVA_HOME / PATH / 注册表 / 常见路径 | 3h | — | internal/java/detector.go |
| P3.2 | 版本校验：执行 java -version 解析 | 2h | P3.1 | internal/java/detector.go |
| P3.3 | 提示引导：Java 不存在/过低的错误信息 + 下载引导 | 1h | P3.2 | cmd/starter/check.go |
| **P3 合计** | | **6h** | | |

**P3 验收**：`./starter check` 正确报告 Java 状态

---

### P4：启动器兼容模式（预估 3 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P4.1 | PCL2 独立模式：生成 launcher_profiles.json | 3h | P2.3 | internal/launcher/pcl2.go |
| P4.2 | PCL2 集成模式：搜索 PCL2.exe、关联目录、注册表操作 | 6h | P2.3 | internal/launcher/pcl2_integration.go |
| P4.3 | PCL.ini 读写：INI 解析、版本注入、卡片更新 | 4h | P4.2 | internal/launcher/pcl_ini.go |
| P4.4 | HMCL 兼容：写入 hmcl.json | 2h | P2.3 | internal/launcher/hmcl.go |
| P4.5 | 官方启动器兼容：launcher_profiles 完整版 | 1h | P4.1 | internal/launcher/vanilla.go |
| **P4 合计** | | **14h** | | |

**P4 验收**：PCL2 / HMCL 能直接识别并启动

---

### P5：自更新 + 多通道（预估 2 天）

| ID | 任务 | 预估 | 前置 | 产出物 |
|---|---|---|---|---|
| P5.1 | 更新检查：remote version.json + semver 比较 + 状态文件 | 3h | P0.3 | internal/selfupdate/check.go |
| P5.2 | 下载与校验：下载 + hash 校验 + 签名校验 + 断点续传 | 3h | P0.5, P5.1 | internal/selfupdate/download.go |
| P5.3 | 替换与重启：Windows bat 脚本 | 4h | P5.2 | internal/selfupdate/apply.go |
| P5.4 | 回滚：自动回滚（10秒启动检测）+ 手动回滚 + 历史记录 | 3h | P5.3 | internal/selfupdate/rollback.go |
| P5.5 | 多通道：stable/beta/dev + 通道端点 + 通道切换验证 | 2h | P5.1 | internal/selfupdate/channel.go |
| P5.6 | 交互通知：静默后台下载 + 启动时询问 + 更新日志展示 | 2h | P5.3 | internal/selfupdate/notify.go |
| **P5 合计** | | **17h** | | |

**P5 验收**：后台静默下载 → 下次启动提示 → 替换自身 → 自动回滚保护

---

### QA + 文档（穿插）

| ID | 任务 | 预估 | 前置 |
|---|---|---|---|
| QA.1 | 单元测试：config / mirror / downloader / version / java | 逐阶段 | 各阶段完成 |
| QA.2 | 集成测试：mock Mojang API + 完整 sync 流程 | 4h | P1 完成 |
| QA.3 | 手动测试清单：Windows 实机全流程 | 3h | P2 完成 |
| QA.4 | README.md + 示例配置 | 2h | P4 完成 |
| QA.5 | FAQ 编写 | 1h | P5 完成 |

**总计工时**：**~77.5h ≈ 10 个工作日**

---

## 三、迭代计划

### Sprint 1（Day 1-2）：骨架期

```
目标：go build 通过，能跑 --help，能读写配置
P0.1 项目初始化          → 0.5h
P0.2 CLI 框架            → 2h
P0.3 配置系统            → 4h
P0.4 镜像管理器          → 2h
P0.5 下载器              → 4h
P0.6 日志系统            → 1.5h
──────────────────────────────
里程碑 M1：./starter init 生成配置
```

### Sprint 2（Day 3-5）：下载期

```
目标：能下载指定版本的 Minecraft
P1.1 版本清单同步        → 3h
P1.2 版本 Jar 下载       → 4h
P1.3 Asset 索引同步      → 1h
P1.4 Asset 文件下载      → 6h
P1.5 Libraries 下载      → 4h
P1.6 断点恢复            → 2h
──────────────────────────────
里程碑 M2：./starter sync 搞定 .minecraft
```

### Sprint 3（Day 6-7）：启动期

```
目标：能安装 Fabric 并启动 MC
P2.1 Fabric 安装器       → 2h
P2.2 Fabric libraries    → 4h
P2.3 启动参数生成        → 4h
P2.4 裸启动执行          → 3h
P2.5 Forge（如需要）     → 3h
──────────────────────────────
里程碑 M3：./starter sync && ./starter launch 进游戏
```

### Sprint 4（Day 8-9）：完善期

```
P3.1 Java 路径检测       → 3h
P3.2 版本校验            → 2h
P3.3 引导提示            → 1h
P4.1 PCL2 兼容           → 4h
P4.2 HMCL 兼容           → 2h
P4.3 官方启动器兼容      → 1h
──────────────────────────────
里程碑 M4：完整的启动器体验
```

### Sprint 5（Day 10）：收尾期

```
P5.1 更新检查            → 2h
P5.2 自身替换            → 3h
P5.3 回滚                → 1h
P5.4 多通道              → 1.5h
QA.3 手动测试            → 3h
QA.4 README              → 2h
QA.5 FAQ                 → 1h
──────────────────────────────
里程碑 M5：v1.0 发布
```

---

## 四、关键路径

```
P0.1 → P0.2 → P0.3 ────────────────────────────── 关键路径
                    ↘                      ↗
                     P0.4 → P0.5 → P1.x → P2.x
                                 ↗
                     P0.6───────┘
```

**关键路径**：P0.1 → P0.2 → P0.3 → P0.4 → P0.5 → P1.x → P2.x（不可并行，必须按顺序）

**高风险点**：
- P1.4 Asset 下载：文件数量多（几千个），并发控制 + 断点续传容易出 bug
- P2.2 Fabric libraries 解析：BMCLAPI 的 meta API 格式需要仔细对接
- P2.4 裸启动：classpath 拼接错了 MC 直接启动不了，调试困难
