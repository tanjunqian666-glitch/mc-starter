# MC-Starter

> 轻量级 Minecraft 版本管理 + 整合包更新器。

不自带启动器、不捆绑代理、不占资源。只做一件事：**把指定版本的 Minecraft + 模组包下载配置好，打开 PCL2 就能玩。**

## 快速开始（30 秒上手）

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

## 命令参考

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

## 文档目录

| 文档 | 说明 |
|---|---|
| [详细开发流程](详细开发流程.md) | 分阶段开发路线、验收标准、技术要点 |
| [WBS 迭代计划](WBS-迭代计划.md) | 工作分解 + 迭代排期 |
| [API 接口文档](API接口文档.md) | Mojang / Fabric / BMCLAPI 外部接口 |
| [立项报告](立项报告.md) | 项目背景、目标、范围 |
| [构建与 CI](构建与CI.md) | 编译构建、CI 配置 |
| [自更新方案](自更新方案.md) | 启动器自身更新机制 |
| [整合包打包与导入方案](整合包打包与导入方案.md) | 整合包分发格式 |
| [模组与配置同步策略](模组与配置同步策略.md) | 模组及配置文件的同步机制 |
| [本地版本仓库与增量同步](本地版本仓库与增量同步.md) | 本地缓存、增量下载 |
| [修复与备份系统](修复与备份系统.md) | 文件修复、备份回滚 |
| [修复工具 GUI 界面](修复工具GUI界面.md) | 图形修复界面设计 |
| [崩溃监控与自动修复](崩溃监控与自动修复.md) | 崩溃检测、自动恢复 |
| [PCL2 集成方案](PCL2集成方案.md) | PCL2 启动器集成详情 |
| [服务端整合包管理流程](服务端整合包管理流程.md) | 服务端配置下发流程 |
| [JSON Schema 与测试用例](JSON-Schema与测试用例.md) | 配置校验、测试 |
| [错误处理与安全设计](错误处理与安全设计.md) | 错误处理策略、安全考量 |

---

> [English README →](../../README.md)
