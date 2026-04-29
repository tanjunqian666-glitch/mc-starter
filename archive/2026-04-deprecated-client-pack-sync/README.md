# 已废弃：客户端 zip 同步模块 (2026-04)

## 删除时间

2026-04-29, commit `4d6d73e`

## 废弃原因

原来设计是客户端侧下载 zip → 解压 → diff → 应用到 `.minecraft`。但实际需求是**服务端**导入 zip 对比差异后发布增量版本，客户端只通过 API 拉增量文件列表。

## 对应文件

- `internal/pack/zip.go` — zip 下载 + 解压 + hash 校验
- `internal/pack/diff.go` — 与本地目录的差异计算和 ApplyDiff
- `internal/pack/sync.go` — 整合 server.json modpack 的全量同步管理器
- `internal/pack/zip_test.go` — 7 个测试用例

## 替代方案

服务端 `internal/pack/pack.go` 提供 `ImportZip` + `PublishDraft` + `ComputeDiff`，不再涉及客户端。
