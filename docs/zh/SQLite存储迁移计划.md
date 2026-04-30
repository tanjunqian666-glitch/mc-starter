# SQLite 存储迁移计划

## 为什么需要数据库

当前 `PackStore` 全部基于文件系统 JSON：
- `index.json` — 包列表 + 元数据
- `versions/{ver}/manifest.json` — 每个版本的完整文件清单

规模上去后瓶颈会出现在：
1. **查询场景** — 查"哪个包含了 sodium.jar"需要遍历所有 manifest
2. **并发写入** — index.json 靠 `sync.RWMutex` 保护，分布式部署不可用
3. **版本历史** — 按时间线、按文件、按条件查询都很麻烦
4. **部分更新** — 索引更新必须重写整个 JSON 文件

## 当前接口

`PackStoreIface` 定义在 `internal/server/store.go`：

```go
type PackStoreIface interface {
    // 包管理
    CreatePack(name, displayName, description string, primary bool) error
    DeletePack(name string) error
    ListPacks() []model.PackInfo
    GetPack(name string) (*model.PackDetail, error)
    UpdateLatestVersion(name, version string) error

    // 目录查询（用于文件读写）
    PackDir(name string) string
    FilesDir(name string) string
    VersionsDir(name string) string

    // 配置访问
    Config() *ServerConfig
}
```

工厂函数：`NewStore(cfg, storageType)` 切换。

## 数据库表设计

```sql
-- 整合包
CREATE TABLE packs (
    name          TEXT PRIMARY KEY,         -- 包标识符（如 "main-pack"）
    display_name  TEXT NOT NULL,            -- 显示名称
    description   TEXT NOT NULL DEFAULT '',
    primary       INTEGER NOT NULL DEFAULT 0,  -- 0/1 布尔
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 版本
CREATE TABLE versions (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    pack_name TEXT NOT NULL REFERENCES packs(name) ON DELETE CASCADE,
    version   TEXT NOT NULL,                   -- 版本号（如 "v1.2.0"）
    published INTEGER NOT NULL DEFAULT 0,      -- 0=draft, 1=published
    message   TEXT NOT NULL DEFAULT '',        -- 发布说明
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(pack_name, version)
);

-- 文件清单（每个版本的每个文件一条记录）
CREATE TABLE manifest_entries (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    version_id INTEGER NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    path      TEXT NOT NULL,                -- 相对路径（如 "mods/sodium.jar"）
    sha1      TEXT NOT NULL,               -- SHA1 hex
    sha256    TEXT NOT NULL,               -- SHA256 hex
    size      INTEGER NOT NULL,             -- 字节数
    UNIQUE(version_id, path)
);

CREATE INDEX idx_manifest_sha256 ON manifest_entries(sha256);
CREATE INDEX idx_manifest_path ON manifest_entries(path);
CREATE INDEX idx_versions_pack ON versions(pack_name);
```

**关键设计点：**
- `packs.primary` 只是标记位，不存最新版本号，最新版本通过 SQL `MAX(versions.version) WHERE published=1` 实时计算（或缓存）
- `manifest_entries` 按 path 和 sha256 建索引，支持跨包查文件
- 文件内容（二进制 blob）仍然按 hash 存文件系统，不上 SQLite Blob

## 迁移影响

| 维度 | 文件系统 JSON | SQLite |
|------|:-:|:-:|
| 依赖 | 无（Go stdlib） | `modernc.org/sqlite`（纯 Go，无 CGO） |
| 安装方式 | 内置 | `go get modernc.org/sqlite`，约 5MB 编译产物增加 |
| 数据迁移 | — | `packs/` 下已有目录结构可自动导入 |
| 并发性能 | 单机够用 | 多服务实例，WAL 模式 |
| 查询能力 | 无 | SQL 自由 |
| 回滚 | git 级别 | 版本级 + 快照 |

## 迁移路径

### Phase 1 — 双写兼容（可选，数据安全优先）
1. 实现 `SQLiteStore` 实现 `PackStoreIface`
2. 写一个 `migrate.go` 命令：扫描 JSON 库 → 写入 SQLite
3. 验证数据一致性后切换默认存储类型

### Phase 2 — 纯 SQLite
1. 默认 `store_type: "sqlite"`
2. 保留 JSON 迁移命令，方便降级

## 实施步骤

1. `internal/server/sqlite_store.go` — 实现 `PackStoreIface`
2. `internal/server/migrate.go` — JSON → SQLite 迁移工具（可内嵌到 `mc-starter-server migrate` 子命令）
3. CLI 增加 `mc-starter-server migrate --to sqlite` 命令
4. 测试覆盖全部接口方法
5. 合入、默认关闭，留 `store_type` 开关

## 不做的边界

- **不上 ORM** — 裸 `database/sql`，简单直接（总共就 3 张表）
- **不存文件内容进 DB** — 文件还是走 `files/{hash[:2]}/{hash}`，DB 只管索引
- **不做分布式中继** — SQLite 就是单文件数据库，分布式场景应该上 PostgreSQL，那是另一个 thread
- **不做自动迁移** — 手动跑 `migrate` 命令，确保数据安全
