# 架构说明

## P1.9 增量同步

### 思路

P1.6-P1.8 已经建好了基础设施：
- **P1.6 (sync_state)**: 阶段级断点恢复 — 知道哪些阶段已完成
- **P1.7 (repo)**: 本地版本仓库 — 快照+差异计算+文件缓存
- **P1.8 (cache store)**: 独立文件缓存 — 按 hash 去重，引用计数管理

P1.9 把这三者整合到 `sync` 命令中，实现增量同步：

1. **version.json / client.jar** → 用 CacheStore 缓存
2. **Asset files** → 用 CacheStore 缓存，跳过已有
3. **Libraries** → 用 CacheStore 缓存
4. **repo 快照** → 创建全量/增量快照，差异同步

### sync 命令新流程

```
sync:
  phase0: 拉取版本清单 + 确定目标版本
  phase1: 尝试断点恢复（P1.6）
  phase2: 拉取 version meta（P1.6）
  phase3: 下载 client.jar（P1.6）
  phase4: Asset index + Asset 文件（P1.6）
  phase5: Libraries + Natives（P1.6）
  phase6: 创建 repo 全量快照（P1.7）
  phase7: 如果已有快照 → 增量同步
  complete: 标记完成 + 清理状态文件
```

### 新组件

**IncrementalSync** — `internal/launcher/incr_sync.go`
- 把 CacheStore 接入 asset/library 下载流程
- 在 sync 命令的 asset 和 library 阶段注入缓存检查
- 在 sync 最后创建/更新 repo 快照
