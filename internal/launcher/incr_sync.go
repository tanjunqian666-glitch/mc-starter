package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// P1.9 增量同步 — 将 CacheStore + LocalRepo 整合进 sync 流程
//
// 职责：
//   1. 在 asset 下载前检查 CacheStore，跳过已有
//   2. 在 library 下载前检查 CacheStore，跳过已有
//   3. sync 完成后创建/更新 repo 快照
//   4. 如果有旧快照，计算差异做增量同步
//
// 不改变 sync_state 流程，只在各阶段的下载过程中注入缓存检查。
// ============================================================

// IncrementalSync 增量同步管理器
type IncrementalSync struct {
	cache  *CacheStore
	repo   *LocalRepo
	cfgDir string
	mcDir  string
}

// NewIncrementalSync 创建增量同步管理器
// cfgDir: 配置目录（config/）
// mcDir: .minecraft 目录（用于 repo）
func NewIncrementalSync(cfgDir, mcDir string) *IncrementalSync {
	// 缓存目录: config/.cache/mc_cache/
	cacheDir := filepath.Join(cfgDir, ".cache", "mc_cache")
	cache := NewCacheStore(cacheDir)
	cache.Init()

	repo := NewLocalRepo(mcDir)

	return &IncrementalSync{
		cache:  cache,
		repo:   repo,
		cfgDir: cfgDir,
		mcDir:  mcDir,
	}
}

// CacheStore 返回内部的 CacheStore（供外部调用）
func (is *IncrementalSync) CacheStore() *CacheStore {
	return is.cache
}

// LocalRepo 返回内部的 LocalRepo（供外部调用）
func (is *IncrementalSync) LocalRepo() *LocalRepo {
	return is.repo
}

// ============================================================
// Asset 文件缓存检查
// ============================================================

// TryCacheAsset 检查 Asset 文件是否已在缓存中
// 如果在，返回缓存路径（可直接复制）；如果不在，返回空串
func (is *IncrementalSync) TryCacheAsset(hash string) string {
	// Cache store 中按 SHA1 hash 存储
	path, err := is.cache.Get(hash)
	if err != nil {
		return ""
	}
	return path
}

// StoreAsset 将刚下载的 Asset 存入缓存（去重）
func (is *IncrementalSync) StoreAsset(hash, srcPath string) {
	is.cache.Put(srcPath, hash)
}

// ============================================================
// Library 文件缓存检查
// ============================================================

// TryCacheLibrary 检查 Library JAR 是否已在缓存中
// 按 SHA1 hash 检查
func (is *IncrementalSync) TryCacheLibrary(sha1 string) string {
	if sha1 == "" {
		return ""
	}
	path, err := is.cache.Get(sha1)
	if err != nil {
		return ""
	}
	return path
}

// StoreLibrary 将刚下载的 Library 存入缓存
func (is *IncrementalSync) StoreLibrary(sha1, srcPath string) {
	if sha1 == "" {
		return
	}
	is.cache.Put(srcPath, sha1)
}

// ============================================================
// version.json / client.jar 缓存
// ============================================================

// CacheVersionMeta 缓存已下载的 version.json
// key 为版本 ID（如 "1.20.4"）
// path 为 version.json 的本地路径
func (is *IncrementalSync) CacheVersionMeta(versionID, path string) {
	// version.json 用 SHA1 作为缓存 key，但这里用版本 ID 作索引更直观
	// 用 "version_meta:" 前缀区分
	key := "version_meta:" + versionID
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	is.cache.PutData(data, key)
}

// TryCacheVersionMeta 尝试从缓存获取 version.json
func (is *IncrementalSync) TryCacheVersionMeta(versionID string) string {
	key := "version_meta:" + versionID
	path, err := is.cache.Get(key)
	if err != nil {
		return ""
	}
	return path
}

// CacheClientJar 缓存 client.jar
func (is *IncrementalSync) CacheClientJar(sha1, localPath string) {
	if sha1 == "" {
		return
	}
	is.cache.Put(localPath, sha1)
}

// TryCacheClientJar 尝试从缓存获取 client.jar
func (is *IncrementalSync) TryCacheClientJar(sha1 string) string {
	path, err := is.cache.Get(sha1)
	if err != nil {
		return ""
	}
	return path
}

// ============================================================
// Repo 快照操作（整合到 sync 流程末尾）
// ============================================================

// EnsureRepo 确保 repo 已初始化
func (is *IncrementalSync) EnsureRepo(mcVersion string) error {
	return is.repo.Init(mcVersion)
}

// CreateSyncSnapshot 在 sync 完成后创建系统快照
// scanDirs: 要扫描的目录（如 ["mods", "config"]）
// 返回快照名
func (is *IncrementalSync) CreateSyncSnapshot(snapshotName string, scanDirs []string) error {
	// 检查是否有上一条快照
	latest := is.repo.LatestSnapshot()
	var source string
	if latest == "" {
		source = "full_download"
	} else {
		source = "incremental_update"
	}

	manifest, err := is.repo.CreateFullSnapshot(snapshotName, scanDirs, source, latest)
	if err != nil {
		return fmt.Errorf("创建 sync 快照失败: %w", err)
	}

	_ = manifest // 快照已创建，内部日志已记录
	return nil
}

// DiffSinceSnapshot 计算自上次快照以来的差异
// scanDirs: 要扫描的目录
// 返回差异结果（不含 diff 信息用 DiffResult.Empty() 判断）
func (is *IncrementalSync) DiffSinceSnapshot(snapshotName string, scanDirs []string) (*DiffResult, error) {
	// 加载旧快照
	oldManifest, err := is.repo.LoadSnapshotManifest(snapshotName)
	if err != nil {
		return nil, fmt.Errorf("加载快照 %s 失败: %w", snapshotName, err)
	}

	// 扫描当前文件系统，带着目录前缀以使路径匹配快照格式
	newManifest := &SnapshotManifest{
		Version: "current",
		Files:   make(map[string]RepoFileEntry),
	}
	for _, dir := range scanDirs {
		scanPath := filepath.Join(is.mcDir, dir)
		err := filepath.Walk(scanPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// 使用 dir + "/" + filename 格式，与 CreateFullSnapshot 一致
			relPath := dir + "/" + info.Name()

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h := sha256.Sum256(data)
			hash := hex.EncodeToString(h[:])

			newManifest.Files[relPath] = RepoFileEntry{
				Hash:   hash,
				Size:   info.Size(),
				Action: "add",
				Cached: false,
			}
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			logger.Warn("扫描目录 %s 失败: %v", dir, err)
		}
	}

	diff := is.repo.ComputeDiff(oldManifest, newManifest)

	// 缓存新文件
	for _, f := range diff.Added {
		fullPath := filepath.Join(is.mcDir, f.RelPath)
		if _, err := os.Stat(fullPath); err == nil {
			is.cache.Put(fullPath, f.NewHash)
		}
	}
	for _, f := range diff.Updated {
		fullPath := filepath.Join(is.mcDir, f.RelPath)
		if _, err := os.Stat(fullPath); err == nil {
			is.cache.Put(fullPath, f.NewHash)
		}
	}

	return diff, nil
}

// ============================================================
// 辅助方法
// ============================================================

// SyncStats 返回同步统计摘要
func (is *IncrementalSync) SyncStats() string {
	cacheStats := is.cache.Stats()
	repoStats := is.repo.Stats()
	return fmt.Sprintf("增量同步 | %s | %s", cacheStats, repoStats)
}

// CleanOrphaned 清理未被 repo 快照引用的缓存文件
func (is *IncrementalSync) CleanOrphaned(dryRun bool) (deleted int, freed int64, errs []error) {
	refs, err := is.repo.ReferencedHashes()
	if err != nil {
		return 0, 0, append(errs, fmt.Errorf("获取引用 hash 失败: %w", err))
	}

	// 清理不引用的文件（保留被快照引用的 hash）
	opts := CleanOptions{
		DryRun:      dryRun,
		MinRefCount: 0,
		KeepHashes:  refs,
	}
	return is.cache.Clean(opts)
}

// CacheVersionMetaData 缓存 version.json 数据
// 与 CacheVersionMeta 类似，但接受字节直接写入
func (is *IncrementalSync) CacheVersionMetaData(versionID string, data []byte) {
	key := "version_meta:" + versionID
	is.cache.PutData(data, key)
}

// CacheDownloadedAsset 连接已下载的 Asset 到缓存
// hash: Asset SHA1
// localPath: 已下载到的本地路径（assets/objects/hh/hash）
// 如果缓存中已有则跳过（增加引用计数）
func (is *IncrementalSync) CacheDownloadedAsset(hash, localPath string) {
	is.cache.Put(localPath, hash)
}

// CacheDownloadedLibrary 连接已下载的 Library 到缓存
func (is *IncrementalSync) CacheDownloadedLibrary(sha1, localPath string) {
	if sha1 != "" {
		is.cache.Put(localPath, sha1)
	}
}

// AssetFromCache 从缓存读取 Asset 文件内容并写入指定目录
// 如果在缓存中命中，直接复制到 assets/objects/{hh}/{hash}
// 返回 true 表示缓存命中并复制成功
func (is *IncrementalSync) AssetFromCache(hash, destPath string) bool {
	cachedPath, err := is.cache.Get(hash)
	if err != nil {
		return false
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		logger.Warn("创建 Asset 目标目录失败: %v", err)
		return false
	}

	if err := copyFile(cachedPath, destPath); err != nil {
		logger.Warn("从缓存复制 Asset 失败: %v", err)
		return false
	}

	return true
}

// ConsumeLibraryFiles 批量处理 LibraryFile 列表的缓存加速
// 对每个文件，先查缓存；缓存命中则直接复制到目标位置；
// 缓存未命中则返回需要下载的文件列表
// 返回: 需要下载的文件列表，从缓存复制的文件数
func (is *IncrementalSync) ConsumeLibraryFiles(files []model.LibraryFile) (toDownload []model.LibraryFile, fromCache int) {
	for _, f := range files {
		if f.SHA1 == "" {
			// 无 SHA1 无法检查缓存，直接下载
			toDownload = append(toDownload, f)
			continue
		}

		cachedPath, err := is.cache.Get(f.SHA1)
		if err != nil {
			toDownload = append(toDownload, f)
			continue
		}

		// 缓存命中，复制到目标位置
		if err := os.MkdirAll(filepath.Dir(f.LocalPath), 0755); err != nil {
			logger.Warn("创建 library 目录失败: %v", err)
			toDownload = append(toDownload, f)
			continue
		}

		if err := copyFile(cachedPath, f.LocalPath); err != nil {
			logger.Warn("从缓存复制 library 失败: %v", err)
			toDownload = append(toDownload, f)
			continue
		}

		fromCache++
	}
	return
}

// CacheNatives 将解压后的 natives 文件存入缓存
// 不做去重——原生库文件通常不大
func (is *IncrementalSync) CacheNatives(sha1, jarPath string) {
	if sha1 != "" {
		is.cache.Put(jarPath, sha1)
	}
}
