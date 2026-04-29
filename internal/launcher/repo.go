package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// 本地版本仓库 — 支持增量同步和快照回滚
//
// 目录结构:
//   {mcDir}/starter_repo/
//   ├── repo.json              ← 仓库元信息
//   ├── snapshots/             ← 各版本快照清单
//   │   ├── v1.0/
//   │   │   ├── manifest.json  ← 文件清单（hash+size+action）
//   │   │   └── meta.json      ← 版本信息
//   │   └── v1.1/
//   │       └── ...
//   ├── files/                 ← 按 hash 去重的文件缓存
//   │   ├── abc123...         ← sha256:abc123...
//   │   └── def456...
//   └── current → snapshots/v1.0  ← 当前快照 symlink
// ============================================================

// RepoMeta 仓库元信息
type RepoMeta struct {
	Version        int       `json:"version"`          // 仓库格式版本
	MCVersion      string    `json:"mc_version"`       // 关联的 MC 版本
	Snapshots      []string  `json:"snapshots"`        // 快照名列表（有序）
	LatestSnapshot string    `json:"latest_snapshot"`  // 最新快照名
	TotalCached    int64     `json:"total_cached"`     // 缓存文件数
	TotalCacheSize int64     `json:"total_cache_size"` // 缓存总字节数
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SnapshotMeta 快照元信息
type SnapshotMeta struct {
	Version     string    `json:"version"`      // 快照版本号（如 "1.0"）
	MCVersion   string    `json:"mc_version"`   // MC 版本
	CreatedAt   time.Time `json:"created_at"`   // 创建时间
	Source      string    `json:"source"`       // "full_download" | "incremental_update"
	FromVersion string    `json:"from_version"` // 来源版本（增量升级时）
	FileCount   int       `json:"file_count"`   // 该快照包含的文件数
	TotalSize   int64     `json:"total_size"`   // 该快照总大小
}

// RepoFileEntry 快照中单个文件的记录
type RepoFileEntry struct {
	Hash   string `json:"hash"`             // SHA256:hex
	Size   int64  `json:"size"`             // 字节数
	Action string `json:"action,omitempty"` // add | update | delete（增量记录用）
	Cached bool   `json:"cached"`           // 是否已在 files/ 缓存
}

// SnapshotManifest 快照的文件清单
type SnapshotManifest struct {
	Version     string                   `json:"version"`
	CreatedAt   time.Time                `json:"created_at"`
	Source      string                   `json:"source"`
	FromVersion string                   `json:"from_version"`
	Files       map[string]RepoFileEntry `json:"files"`
}

// LocalRepo 本地版本仓库
type LocalRepo struct {
	mu           sync.RWMutex
	baseDir      string // starter_repo/ 目录
	snapshotsDir string // starter_repo/snapshots/
	filesDir     string // starter_repo/files/
	currentDir   string // starter_repo/current (symlink)
	meta         *RepoMeta
}

// NewLocalRepo 创建/加载本地仓库
// mcDir: .minecraft 目录
func NewLocalRepo(mcDir string) *LocalRepo {
	baseDir := filepath.Join(mcDir, "starter_repo")
	r := &LocalRepo{
		baseDir:      baseDir,
		snapshotsDir: filepath.Join(baseDir, "snapshots"),
		filesDir:     filepath.Join(baseDir, "files"),
		currentDir:   filepath.Join(baseDir, "current"),
		meta:         &RepoMeta{},
	}
	r.loadMeta()
	return r
}

// Init 初始化仓库结构（幂等）
func (r *LocalRepo) Init(mcVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 创建目录
	for _, dir := range []string{r.baseDir, r.snapshotsDir, r.filesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建仓库目录 %s 失败: %w", dir, err)
		}
	}

	// 如果已有 meta 且 MC 版本匹配，跳过
	if r.meta.MCVersion == mcVersion {
		logger.Debug("仓库已存在: %s (mc=%s)", r.baseDir, mcVersion)
		return nil
	}

	// 初始化 meta
	r.meta = &RepoMeta{
		Version:        1,
		MCVersion:      mcVersion,
		Snapshots:      make([]string, 0),
		LatestSnapshot: "",
		TotalCached:    0,
		TotalCacheSize: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	return r.saveMeta()
}

// IsInitialized 检查仓库是否已初始化
func (r *LocalRepo) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.meta.MCVersion != "" && r.meta.Version > 0
}

// HasSnapshots 检查是否有快照
func (r *LocalRepo) HasSnapshots() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.meta.Snapshots) > 0
}

// SnapshotCount 返回快照数量
func (r *LocalRepo) SnapshotCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.meta.Snapshots)
}

// LatestSnapshot 返回最新快照名
func (r *LocalRepo) LatestSnapshot() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.meta.LatestSnapshot
}

// SnapshotDir 返回指定快照的目录路径
func (r *LocalRepo) SnapshotDir(name string) string {
	return filepath.Join(r.snapshotsDir, name)
}

// FilesDir 返回文件缓存目录
func (r *LocalRepo) FilesDir() string {
	return r.filesDir
}

// CachedFilePath 返回缓存文件的完整路径（按 hash）
func (r *LocalRepo) CachedFilePath(hash string) string {
	return filepath.Join(r.filesDir, hash)
}

// ============================================================
// 快照操作
// ============================================================

// CreateFullSnapshot 基于当前 mods/config 目录创建全量快照
// scanDirs: 要扫描的目录列表（如 ["mods", "config"]）
// source: "full_download" | "incremental_update"
// fromVersion: 来源版本（增量时填写）
//
// 参考 PCL 的 McLibToken：PCL 把 libraries 统一为 McLibToken 类型，
// 我们在这里把文件统一为 SnapshotManifest + 按 hash 去重的缓存。
func (r *LocalRepo) CreateFullSnapshot(name string, scanDirs []string, source string, fromVersion string) (*SnapshotManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapDir := r.SnapshotDir(name)
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return nil, fmt.Errorf("创建快照目录 %s 失败: %w", snapDir, err)
	}

	manifest := &SnapshotManifest{
		Version:     name,
		CreatedAt:   time.Now(),
		Source:      source,
		FromVersion: fromVersion,
		Files:       make(map[string]RepoFileEntry),
	}

	// 扫描目录，计算 hash，建立清单
	for _, dir := range scanDirs {
		scanPath := filepath.Join(filepath.Dir(r.baseDir), dir) // .minecraft/{dir}
		files, err := os.ReadDir(scanPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			logger.Warn("扫描目录 %s 失败: %v", scanPath, err)
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				// 递归扫描一级子目录（如 mods/sodium-fabric-0.5.3.jar 下的 meta-inf 等）
				r.scanDirRecursive(filepath.Join(scanPath, f.Name()), dir+"/"+f.Name(), manifest)
			} else {
				fullPath := filepath.Join(scanPath, f.Name())
				relPath := dir + "/" + f.Name()
				entry, err := r.fileEntry(fullPath)
				if err != nil {
					logger.Warn("计算文件 hash 失败: %s (%v)", fullPath, err)
					continue
				}
				manifest.Files[relPath] = entry

				// 同步到缓存
				r.cacheFile(fullPath, entry.Hash)
			}
		}
	}

	// 写入 manifest.json
	if err := r.writeSnapshotManifest(name, manifest); err != nil {
		return nil, fmt.Errorf("写入快照清单失败: %w", err)
	}

	// 写入 meta.json
	meta := &SnapshotMeta{
		Version:     name,
		MCVersion:   r.meta.MCVersion,
		CreatedAt:   manifest.CreatedAt,
		Source:      source,
		FromVersion: fromVersion,
		FileCount:   len(manifest.Files),
	}
	var totalSize int64
	for _, e := range manifest.Files {
		totalSize += e.Size
	}
	meta.TotalSize = totalSize

	if err := r.writeSnapshotMeta(name, meta); err != nil {
		return nil, fmt.Errorf("写入快照元信息失败: %w", err)
	}

	// 更新仓库元信息
	r.meta.Snapshots = append(r.meta.Snapshots, name)
	r.meta.LatestSnapshot = name
	r.meta.UpdatedAt = time.Now()
	if err := r.saveMeta(); err != nil {
		return nil, err
	}

	// 更新 current symlink
	r.updateCurrentSymlink(name)

	logger.Info("快照 %s 已创建: %d 个文件, %.1f MB", name, len(manifest.Files), float64(totalSize)/1024/1024)
	return manifest, nil
}

// LoadSnapshotManifest 加载指定快照的文件清单
func (r *LocalRepo) LoadSnapshotManifest(name string) (*SnapshotManifest, error) {
	path := filepath.Join(r.SnapshotDir(name), "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取快照 %s 清单失败: %w", name, err)
	}

	var manifest SnapshotManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析快照 %s 清单失败: %w", name, err)
	}
	return &manifest, nil
}

// LoadSnapshotMeta 加载指定快照的元信息
func (r *LocalRepo) LoadSnapshotMeta(name string) (*SnapshotMeta, error) {
	path := filepath.Join(r.SnapshotDir(name), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取快照 %s 元信息失败: %w", name, err)
	}

	var meta SnapshotMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析快照 %s 元信息失败: %w", name, err)
	}
	return &meta, nil
}

// ListSnapshots 列出所有快照（按创建时间排序，最新在前）
func (r *LocalRepo) ListSnapshots() ([]string, error) {
	r.mu.RLock()
	snapshots := make([]string, len(r.meta.Snapshots))
	copy(snapshots, r.meta.Snapshots)
	r.mu.RUnlock()

	// 按时间排序（最新的在前）
	type snapInfo struct {
		name string
		time time.Time
	}
	var infos []snapInfo
	for _, name := range snapshots {
		meta, err := r.LoadSnapshotMeta(name)
		if err != nil {
			continue
		}
		infos = append(infos, snapInfo{name: name, time: meta.CreatedAt})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].time.After(infos[j].time)
	})

	result := make([]string, len(infos))
	for i, info := range infos {
		result[i] = info.name
	}
	return result, nil
}

// DeleteSnapshot 删除指定快照
func (r *LocalRepo) DeleteSnapshot(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapDir := r.SnapshotDir(name)
	if err := os.RemoveAll(snapDir); err != nil {
		return fmt.Errorf("删除快照 %s 失败: %w", name, err)
	}

	// 从列表移除
	var newSnapshots []string
	for _, s := range r.meta.Snapshots {
		if s != name {
			newSnapshots = append(newSnapshots, s)
		}
	}
	r.meta.Snapshots = newSnapshots
	if len(newSnapshots) > 0 && r.meta.LatestSnapshot == name {
		r.meta.LatestSnapshot = newSnapshots[len(newSnapshots)-1]
		r.updateCurrentSymlink(r.meta.LatestSnapshot)
	} else if len(newSnapshots) == 0 {
		r.meta.LatestSnapshot = ""
		os.Remove(r.currentDir)
	}
	r.meta.UpdatedAt = time.Now()
	return r.saveMeta()
}

// ============================================================
// 文件缓存操作
// ============================================================

// CacheFile 将文件加入缓存（按 hash 去重）
// 返回 true 表示新缓存，false 表示已有
func (r *LocalRepo) CacheFile(srcPath, hash string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cacheFile(srcPath, hash)
}

// cacheFile 内部实现（不持有锁）
func (r *LocalRepo) cacheFile(srcPath, hash string) (bool, error) {
	destPath := filepath.Join(r.filesDir, hash)

	// 已存在则跳过
	if _, err := os.Stat(destPath); err == nil {
		return false, nil
	}

	if err := copyFile(srcPath, destPath); err != nil {
		return false, fmt.Errorf("缓存文件 %s 失败: %w", srcPath, err)
	}

	fi, _ := os.Stat(destPath)
	r.meta.TotalCached++
	r.meta.TotalCacheSize += fi.Size()
	logger.Debug("缓存: %s → %s", filepath.Base(srcPath), shortHash(hash))

	return true, nil
}

// IsCached 检查文件是否已在缓存中
func (r *LocalRepo) IsCached(hash string) bool {
	path := filepath.Join(r.filesDir, hash)
	_, err := os.Stat(path)
	return err == nil
}

// CacheStats 返回缓存统计
func (r *LocalRepo) CacheStats() (count int64, totalSize int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.meta.TotalCached, r.meta.TotalCacheSize
}

// CleanCache 清理未使用的缓存文件
// referencedHashes: 当前快照引用的所有 hash
// dryRun: 仅显示不删除
// 返回删除的文件数和释放的字节数，以及错误列表
func (r *LocalRepo) CleanCache(referencedHashes map[string]bool, dryRun bool) (deleted int, freed int64, errs []error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 枚举 files/ 目录
	entries, err := os.ReadDir(r.filesDir)
	if err != nil {
		return 0, 0, append(errs, fmt.Errorf("枚举缓存目录失败: %w", err))
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		hash := entry.Name()
		if referencedHashes[hash] {
			continue
		}

		// 未被引用 → 删除
		fullPath := filepath.Join(r.filesDir, hash)
		fi, _ := os.Stat(fullPath)
		var size int64
		if fi != nil {
			size = fi.Size()
		}

		if dryRun {
			logger.Debug("[DRY-RUN] 删除未引用缓存: %s (%.1f KB)", shortHash(hash), float64(size)/1024)
		} else {
			if err := os.Remove(fullPath); err != nil {
				errs = append(errs, fmt.Errorf("删除缓存 %s 失败: %w", shortHash(hash), err))
				continue
			}
		}

		deleted++
		freed += size
	}

	if !dryRun && deleted > 0 {
		r.meta.TotalCached -= int64(deleted)
		r.meta.TotalCacheSize -= freed
		r.meta.UpdatedAt = time.Now()
		r.saveMeta()
	}

	return deleted, freed, errs
}

// ============================================================
// DSM（差异同步管理器）— 计算两个版本间的增量
// ============================================================

// DiffResult 两个快照间的差异计算结果
type DiffResult struct {
	Added     []DiffFile `json:"added"`     // 新增的文件
	Updated   []DiffFile `json:"updated"`   // hash 发生变化的文件
	Deleted   []DiffFile `json:"deleted"`   // 已被删除的文件
	Unchanged int        `json:"unchanged"` // 未变化的文件数
}

// DiffFile 差异文件信息
type DiffFile struct {
	RelPath string `json:"rel_path"` // 相对路径（如 "mods/sodium.jar"）
	OldHash string `json:"old_hash"` // 旧 hash（新增时为空）
	NewHash string `json:"new_hash"` // 新 hash（删除时为空）
	Size    int64  `json:"size"`     // 新文件大小
	NewPath string `json:"new_path"` // 新文件本地路径（如果已在缓存中）
}

// ComputeDiff 计算两个快照间的差异
// oldManifest: 旧版本清单
// newManifest: 新版本清单（本地文件系统的扫描结果）
func (r *LocalRepo) ComputeDiff(oldManifest, newManifest *SnapshotManifest) *DiffResult {
	result := &DiffResult{}

	// 建立旧清单 hash 索引
	oldFiles := make(map[string]RepoFileEntry) // key: relPath
	for path, entry := range oldManifest.Files {
		oldFiles[path] = entry
	}

	// 遍历新清单，与旧清单对比
	for path, newEntry := range newManifest.Files {
		oldEntry, exists := oldFiles[path]
		if !exists {
			// 新增
			result.Added = append(result.Added, DiffFile{
				RelPath: path,
				OldHash: "",
				NewHash: newEntry.Hash,
				Size:    newEntry.Size,
				NewPath: r.CachedFilePath(newEntry.Hash),
			})
		} else if oldEntry.Hash != newEntry.Hash {
			// 更新（hash 变了）
			result.Updated = append(result.Updated, DiffFile{
				RelPath: path,
				OldHash: oldEntry.Hash,
				NewHash: newEntry.Hash,
				Size:    newEntry.Size,
				NewPath: r.CachedFilePath(newEntry.Hash),
			})
		} else {
			result.Unchanged++
		}
		// 移除已处理项
		delete(oldFiles, path)
	}

	// 剩下的旧文件就是已被删除的
	for path, entry := range oldFiles {
		result.Deleted = append(result.Deleted, DiffFile{
			RelPath: path,
			OldHash: entry.Hash,
			NewHash: "",
		})
	}

	return result
}

// PrintDiff 打印差异摘要（用于 CLI 输出）
func PrintDiff(diff *DiffResult) {
	fmt.Printf("    新增: %d, 更新: %d, 删除: %d, 未变: %d\n",
		len(diff.Added), len(diff.Updated), len(diff.Deleted), diff.Unchanged)
	if len(diff.Added) > 0 && len(diff.Added) <= 5 {
		for _, f := range diff.Added {
			fmt.Printf("      + %s (%d KB)\n", f.RelPath, f.Size/1024)
		}
	} else if len(diff.Added) > 5 {
		fmt.Printf("      + %s ... (+%d more)\n", diff.Added[0].RelPath, len(diff.Added)-1)
	}
	if len(diff.Deleted) > 0 && len(diff.Deleted) <= 5 {
		for _, f := range diff.Deleted {
			fmt.Printf("      - %s\n", f.RelPath)
		}
	}
}

// ============================================================
// 辅助方法
// ============================================================

// scanDirRecursive 递归扫描目录（用于 mods/ 下的子目录）
func (r *LocalRepo) scanDirRecursive(dirPath, relPrefix string, manifest *SnapshotManifest) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())
		relPath := relPrefix + "/" + entry.Name()
		if entry.IsDir() {
			r.scanDirRecursive(fullPath, relPath, manifest)
		} else {
			entryInfo, err := r.fileEntry(fullPath)
			if err != nil {
				continue
			}
			manifest.Files[relPath] = entryInfo
			r.cacheFile(fullPath, entryInfo.Hash)
		}
	}
}

// fileEntry 计算单个文件的 RepoFileEntry
func (r *LocalRepo) fileEntry(path string) (RepoFileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RepoFileEntry{}, err
	}
	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])
	fi, _ := os.Stat(path)
	return RepoFileEntry{
		Hash:   hash,
		Size:   fi.Size(),
		Action: "add",
		Cached: false,
	}, nil
}

// computeFileHash 计算文件 SHA256 hash
func computeFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// updateCurrentSymlink 更新 current symlink
func (r *LocalRepo) updateCurrentSymlink(name string) {
	snapDir := r.SnapshotDir(name)

	// 删除旧 symlink
	if _, err := os.Lstat(r.currentDir); err == nil {
		os.Remove(r.currentDir)
	}

	// 在 Windows 上 symlink 需要特权，用 junction 或目录重命名兜底
	// 先尝试 symlink，失败则忽略（repo 本身仍可用）
	if err := os.Symlink(snapDir, r.currentDir); err != nil {
		logger.Debug("创建 current symlink 失败(非致命): %v", err)
	}
}

// writeSnapshotManifest 写入快照清单文件
func (r *LocalRepo) writeSnapshotManifest(name string, manifest *SnapshotManifest) error {
	snapDir := r.SnapshotDir(name)
	path := filepath.Join(snapDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// writeSnapshotMeta 写入快照元信息
func (r *LocalRepo) writeSnapshotMeta(name string, meta *SnapshotMeta) error {
	snapDir := r.SnapshotDir(name)
	path := filepath.Join(snapDir, "meta.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadCurrentSnapshot 读取 current symlink 指向的快照清单
func (r *LocalRepo) ReadCurrentSnapshot() (*SnapshotManifest, string, error) {
	// 读 symlink 目标
	linkTarget, err := os.Readlink(r.currentDir)
	if err != nil {
		return nil, "", fmt.Errorf("读取 current symlink 失败: %w", err)
	}
	name := filepath.Base(linkTarget)

	manifest, err := r.LoadSnapshotManifest(name)
	if err != nil {
		return nil, name, err
	}
	return manifest, name, nil
}

// CreateIncrementalSnapshot 基于增量创建新快照
// newManifest: 合并了增量变更后的清单
// fromVersion: 来源版本
func (r *LocalRepo) CreateIncrementalSnapshot(name string, newManifest *SnapshotManifest, fromVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapDir := r.SnapshotDir(name)
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return err
	}

	// 标记源
	newManifest.Source = "incremental_update"
	newManifest.FromVersion = fromVersion
	newManifest.Version = name

	// 写入
	if err := r.writeSnapshotManifest(name, newManifest); err != nil {
		return err
	}

	// 计算总大小
	var totalSize int64
	for _, e := range newManifest.Files {
		totalSize += e.Size
	}

	snapMeta := &SnapshotMeta{
		Version:     name,
		MCVersion:   r.meta.MCVersion,
		CreatedAt:   newManifest.CreatedAt,
		Source:      "incremental_update",
		FromVersion: fromVersion,
		FileCount:   len(newManifest.Files),
		TotalSize:   totalSize,
	}
	if err := r.writeSnapshotMeta(name, snapMeta); err != nil {
		return err
	}

	// 更新仓库元信息
	r.meta.Snapshots = append(r.meta.Snapshots, name)
	r.meta.LatestSnapshot = name
	r.meta.UpdatedAt = time.Now()
	if err := r.saveMeta(); err != nil {
		return err
	}

	r.updateCurrentSymlink(name)

	logger.Info("增量快照 %s 已创建: %d 个文件, %.1f MB (基于 %s)", name, len(newManifest.Files), float64(totalSize)/1024/1024, fromVersion)
	return nil
}

// ============================================================
// 文件 IO 辅助
// ============================================================

// loadMeta 从磁盘加载仓库元信息
func (r *LocalRepo) loadMeta() {
	path := filepath.Join(r.baseDir, "repo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, r.meta); err != nil {
		logger.Warn("解析仓库元信息失败: %v", err)
		r.meta = &RepoMeta{}
	}
}

// saveMeta 写入仓库元信息
func (r *LocalRepo) saveMeta() error {
	path := filepath.Join(r.baseDir, "repo.json")
	data, err := json.MarshalIndent(r.meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// ============================================================
// 仓库迁移 — 当 MC 版本变更时重建仓库
// ============================================================

// Migrate 当 MC 大版本变更时，重建仓库并清空缓存
// newMCVersion: 新的 MC 版本
func (r *LocalRepo) Migrate(newMCVersion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 记录旧版本
	oldMCVersion := r.meta.MCVersion

	// 清空快照
	for _, name := range r.meta.Snapshots {
		os.RemoveAll(r.SnapshotDir(name))
	}
	r.meta.Snapshots = make([]string, 0)
	r.meta.LatestSnapshot = ""

	// 清空文件缓存（MC 大版本变更，模组大概率不兼容）
	entries, _ := os.ReadDir(r.filesDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			os.Remove(filepath.Join(r.filesDir, entry.Name()))
		}
	}
	r.meta.TotalCached = 0
	r.meta.TotalCacheSize = 0

	// 更新版本
	r.meta.MCVersion = newMCVersion
	r.meta.UpdatedAt = time.Now()

	logger.Info("仓库迁移: %s → %s (快照和缓存已清空)", oldMCVersion, newMCVersion)
	return r.saveMeta()
}

// Stats 返回仓库统计信息
func (r *LocalRepo) Stats() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var totalSnapSize int64
	for _, name := range r.meta.Snapshots {
		meta, err := r.LoadSnapshotMeta(name)
		if err == nil {
			totalSnapSize += meta.TotalSize
		}
	}

	return fmt.Sprintf("MC=%s, 快照=%d, 缓存=%d 文件(%.1f MB), 引用数据=%.1f MB",
		r.meta.MCVersion,
		len(r.meta.Snapshots),
		r.meta.TotalCached,
		float64(r.meta.TotalCacheSize)/1024/1024,
		float64(totalSnapSize)/1024/1024,
	)
}

// HasSnapshot 检查指定快照是否存在
func (r *LocalRepo) HasSnapshot(name string) bool {
	_, err := os.Stat(r.SnapshotDir(name))
	return err == nil
}

// RestoreSnapshot 恢复快照到指定目录
// snapshotName: 快照名
// destRoot: .minecraft 根目录
// 会覆盖目标目录中的文件
func (r *LocalRepo) RestoreSnapshot(snapshotName string, destRoot string) error {
	manifest, err := r.LoadSnapshotManifest(snapshotName)
	if err != nil {
		return fmt.Errorf("加载快照 %s 失败: %w", snapshotName, err)
	}

	var restored, failed int
	for relPath, entry := range manifest.Files {
		if entry.Action == "delete" {
			// 删除文件
			fullPath := filepath.Join(destRoot, relPath)
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("恢复快照: 删除 %s 失败: %v", relPath, err)
				failed++
			}
			continue
		}

		srcPath := r.CachedFilePath(entry.Hash)
		dstPath := filepath.Join(destRoot, relPath)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			logger.Warn("恢复快照: 缓存文件 %s 不存在", shortHash(entry.Hash))
			failed++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			logger.Warn("恢复快照: 创建目录 %s 失败: %v", filepath.Dir(dstPath), err)
			failed++
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			logger.Warn("恢复快照: 复制 %s 失败: %v", relPath, err)
			failed++
			continue
		}
		restored++
	}

	logger.Info("快照 %s 恢复完成: %d 恢复, %d 失败", snapshotName, restored, failed)
	return nil
}

// EnsureBaseDir 确保仓库基础目录存在（幂等）
func (r *LocalRepo) EnsureBaseDir() error {
	return os.MkdirAll(r.baseDir, 0755)
}

// ReferencedHashes 返回当前最新快照引用的所有 hash
func (r *LocalRepo) ReferencedHashes() (map[string]bool, error) {
	latest := r.LatestSnapshot()
	if latest == "" {
		return make(map[string]bool), nil
	}

	manifest, err := r.LoadSnapshotManifest(latest)
	if err != nil {
		return nil, err
	}

	hashes := make(map[string]bool)
	for _, entry := range manifest.Files {
		hashes[entry.Hash] = true
	}
	return hashes, nil
}

// ApplyDiff 将差异应用到 .minecraft 目录
// diff: 差异计算结果
// destRoot: .minecraft 根目录
// 返回: 应用的文件数
func (r *LocalRepo) ApplyDiff(diff *DiffResult, destRoot string) (applied int) {
	for _, f := range diff.Deleted {
		fullPath := filepath.Join(destRoot, f.RelPath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("删除文件失败: %s (%v)", f.RelPath, err)
		}
	}

	for _, f := range diff.Added {
		srcPath := r.CachedFilePath(f.NewHash)
		dstPath := filepath.Join(destRoot, f.RelPath)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			// 不在缓存中，需要返回让调用方下载
			logger.Debug("文件 %s 不在缓存中，跳过应用", f.RelPath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			logger.Warn("创建目录失败: %s (%v)", filepath.Dir(dstPath), err)
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			logger.Warn("复制文件失败: %s (%v)", f.RelPath, err)
			continue
		}
		applied++
	}

	for _, f := range diff.Updated {
		srcPath := r.CachedFilePath(f.NewHash)
		dstPath := filepath.Join(destRoot, f.RelPath)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			logger.Debug("更新文件 %s 不在缓存中，跳过应用", f.RelPath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			logger.Warn("创建目录失败: %s (%v)", filepath.Dir(dstPath), err)
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			logger.Warn("复制文件失败: %s (%v)", f.RelPath, err)
			continue
		}
		applied++
	}

	return applied
}

// shortHash 截断 hash 为可读短串（日志用），不足则返回原串
func shortHash(hash string) string {
	if len(hash) >= 12 {
		return hash[:12]
	}
	return hash
}

// Ensure 确保仓库已初始化（幂等快捷方法）
func (r *LocalRepo) Ensure(mcVersion string) error {
	if r.IsInitialized() && r.meta.MCVersion == mcVersion {
		return nil
	}
	return r.Init(mcVersion)
}

// BaseDir 返回仓库基础目录
func (r *LocalRepo) BaseDir() string {
	return r.baseDir
}

// ScanDirectory 扫描一个目录，返回文件清单（用于创建快照或差异计算）
func ScanDirectory(rootDir string, manifest *SnapshotManifest) error {
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		// 统一为 / 分隔符
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		hash := hex.EncodeToString(h[:])

		manifest.Files[relPath] = RepoFileEntry{
			Hash:   hash,
			Size:   info.Size(),
			Action: "add",
			Cached: false,
		}
		return nil
	})
	return err
}
