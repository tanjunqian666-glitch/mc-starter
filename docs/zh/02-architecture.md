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
  phase3: 下载 client.jar（缓存加速，P1.9）
  phase4: Asset index + Asset 文件（三级检查：磁盘→CacheStore→下载）
  phase5: Libraries + Natives（CacheStore 分流）
  phase6: 创建/更新 repo 快照（P1.9 新增）
  complete: 标记完成 + 清理状态文件
```

### Asset 三级检查流程

```
下载 Asset 文件时:
  ① 检查本地磁盘 (assets/objects/hh/hash)         → 已存在→跳过
  ② 检查 CacheStore (config/.cache/mc_cache/files/)  → 命中→复制到磁盘
  ③ 下载 → 存入 CacheStore → 写入磁盘
```

### Library 缓存分流流程

```
① 将所有 LibraryFile 传入 ConsumeLibraryFiles()
② 有 SHA1 的查 CacheStore：
   - 命中 → 复制到目标路径，记为 fromCache
   - 未命中 → 加入 toDownload 列表
③ 无 SHA1 的 → 直接加入 toDownload 列表
④ toDownload 传入 lm.DownloadFiles() 下载
⑤ 下载成功后将 SHA1 写回 CacheStore
```

### 新组件

**IncrementalSync** — `internal/launcher/incr_sync.go`
- 把 CacheStore 接入 asset/library 下载流程
- 在 sync 命令的 asset 和 library 阶段注入缓存检查
- 在 sync 最后创建/更新 repo 快照

### 踩坑记录

1. **ScanDirectory 路径不匹配**：`DiffSinceSnapshot` 中用 `ScanDirectory` 生成的文件 key 是相对于扫描根目录的（如 `sodium.jar`），但 snapshot manifest 中的 key 是 `dir + "/" + filename` 格式（如 `mods/sodium.jar`）。两边 key 不一致导致差异计算无变化。

   修复：用手动 `filepath.Walk` + `dir + "/" + info.Name()` 拼接，与 `CreateFullSnapshot` 的格式一致。

2. **copyFile 未导出**：`repo.go` 的 `copyFile()` 未导出（小写），`main.go` 引用不了。在 `cmd/main.go` 中改为 `os.ReadFile + os.WriteFile`。

3. **refs 变量未使用**：`CleanOrphaned` 中收集了 `refs`（来自 `ReferencedHashes()`）但未传给 `CleanOptions.KeepHashes`，导致 Go 编译器报错。修复：正确传递。

### P1 已实现清单

| ID | 状态 | 文件 |
|----|------|------|
| P1.0-P1.5 | ✅ | version.go, asset.go, library.go, downloader.go |
| P1.6 断点恢复 | ✅ | sync_state.go |
| P1.7 本地仓库 | ✅ | repo.go |
| P1.8 文件缓存 | ✅ | cache.go |
| P1.9 增量同步 | ✅ | incr_sync.go |
| P1.10 快照回滚 | ✅ (合入P1.7) | repo.go: RestoreSnapshot |
| P1.11 全局缓存 | ✅ (合入P1.8) | cache.go: CacheStore独立 |
| P1.12-P1.14 zip | ⏳ | — |
