package launcher

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// CacheStore — 通用文件缓存管理器
//
// 从 repo 的 files/ 目录提取为独立组件，支持：
//   - 按 SHA256 hash 去重存储（兼容 SHA1 key）
//   - 引用计数管理
//   - 缓存清理（未引用 + 过期）
//   - 跨版本复用
//
// 目录结构:
//   {cacheDir}/
//   ├── files/               ← 按 hash 去重的文件缓存
//   │   ├── sha256:abc123...
//   │   └── sha1:def456...
//   ├── indexes/             ← 元数据索引（asset index JSON 等）
//   └── cache_meta.json      ← 全局缓存元信息
// ============================================================

// CacheMeta 缓存元信息
type CacheMeta struct {
	Version    int              `json:"version"`    // 缓存格式版本
	Entries    map[string]int64 `json:"entries"`    // hash → last_access (Unix ts)
	RefCounts  map[string]int   `json:"ref_counts"` // hash → 引用次数
	TotalFiles int64            `json:"total_files"`
	TotalSize  int64            `json:"total_size"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// CacheStore 文件缓存
type CacheStore struct {
	mu       sync.RWMutex
	baseDir  string // 缓存根目录
	filesDir string // files/ 目录
	indexDir string // indexes/ 目录
	meta     *CacheMeta
}

// NewCacheStore 创建文件缓存
// cacheDir: 缓存根目录（如 config/.cache/）
func NewCacheStore(cacheDir string) *CacheStore {
	cs := &CacheStore{
		baseDir:  cacheDir,
		filesDir: filepath.Join(cacheDir, "files"),
		indexDir: filepath.Join(cacheDir, "indexes"),
		meta: &CacheMeta{
			Entries:   make(map[string]int64),
			RefCounts: make(map[string]int),
		},
	}
	cs.loadMeta()
	return cs
}

// Init 初始化缓存目录结构（幂等）
func (cs *CacheStore) Init() error {
	for _, dir := range []string{cs.baseDir, cs.filesDir, cs.indexDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建缓存目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// FilesDir 返回文件缓存目录
func (cs *CacheStore) FilesDir() string {
	return cs.filesDir
}

// IndexDir 返回索引缓存目录
func (cs *CacheStore) IndexDir() string {
	return cs.indexDir
}

// ============================================================
// 核心缓存操作
// ============================================================

// Put 将一个文件放入缓存
// srcPath: 源文件路径
// key: 缓存 key（通常是 SHA1 或 SHA256 hex 字符串）
// 返回 true 表示新缓存，false 表示已存在
func (cs *CacheStore) Put(srcPath, key string) (bool, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	destPath := filepath.Join(cs.filesDir, key)

	// 已存在则增加引用计数
	if _, err := os.Stat(destPath); err == nil {
		cs.meta.RefCounts[key]++
		cs.meta.Entries[key] = time.Now().Unix()
		cs.meta.UpdatedAt = time.Now()
		cs.saveMeta()
		return false, nil
	}

	// 复制文件到缓存
	if err := copyFile(srcPath, destPath); err != nil {
		return false, fmt.Errorf("缓存文件 %s 失败: %w", srcPath, err)
	}

	fi, _ := os.Stat(destPath)
	cs.meta.Entries[key] = time.Now().Unix()
	cs.meta.RefCounts[key] = 1
	cs.meta.TotalFiles++
	cs.meta.TotalSize += fi.Size()
	cs.meta.UpdatedAt = time.Now()
	cs.saveMeta()

	return true, nil
}

// Get 获取缓存文件路径
// 如果缓存存在，返回完整路径和 nil；否则返回空串和错误
func (cs *CacheStore) Get(key string) (string, error) {
	destPath := filepath.Join(cs.filesDir, key)

	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return "", fmt.Errorf("缓存未找到: %s", shortHash(key))
	}

	// 更新访问时间
	cs.mu.Lock()
	cs.meta.Entries[key] = time.Now().Unix()
	cs.meta.UpdatedAt = time.Now()
	cs.meta.RefCounts[key]++
	cs.saveMeta()
	cs.mu.Unlock()

	return destPath, nil
}

// Has 检查缓存是否存在
func (cs *CacheStore) Has(key string) bool {
	destPath := filepath.Join(cs.filesDir, key)
	_, err := os.Stat(destPath)
	return err == nil
}

// RefCount 返回指定 key 的引用计数
func (cs *CacheStore) RefCount(key string) int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.meta.RefCounts[key]
}

// ============================================================
// 索引元数据缓存（Asset index JSON、version.json 等）
// ============================================================

// PutIndex 缓存索引元数据（小 JSON 文件）
// name: 索引名（如 "13.json"、"1.20.4.json"）
// data: 原始字节
func (cs *CacheStore) PutIndex(name string, data []byte) error {
	path := filepath.Join(cs.indexDir, name)
	if err := os.MkdirAll(cs.indexDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetIndex 获取索引元数据
func (cs *CacheStore) GetIndex(name string) ([]byte, error) {
	path := filepath.Join(cs.indexDir, name)
	return os.ReadFile(path)
}

// HasIndex 检查索引元数据是否存在
func (cs *CacheStore) HasIndex(name string) bool {
	_, err := os.Stat(filepath.Join(cs.indexDir, name))
	return err == nil
}

// ============================================================
// 缓存清理
// ============================================================

// CleanOptions 缓存清理选项
type CleanOptions struct {
	DryRun      bool            // 仅显示不删除
	MinRefCount int             // 低于此引用数的删除（默认 0 = 无引用才删）
	MaxAge      time.Duration   // 超过此时间未访问的删除（0 = 不过期）
	KeepHashes  map[string]bool // 强制保留的 hash
}

// DefaultCleanOptions 默认清理选项
var DefaultCleanOptions = CleanOptions{
	MinRefCount: 0,
	MaxAge:      0,
	KeepHashes:  nil,
}

// Clean 清理缓存
// 返回：删除的文件数、释放的字节数
func (cs *CacheStore) Clean(opts CleanOptions) (deleted int, freed int64, errs []error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	entries, err := os.ReadDir(cs.filesDir)
	if err != nil {
		return 0, 0, append(errs, fmt.Errorf("读取 files 目录失败: %w", err))
	}

	now := time.Now()
	var deletedHashes []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		hash := entry.Name()

		// 强制保留
		if opts.KeepHashes != nil && opts.KeepHashes[hash] {
			continue
		}

		// 检查引用计数
		if opts.MinRefCount > 0 {
			if cs.meta.RefCounts[hash] >= opts.MinRefCount {
				continue
			}
		}

		// 检查访问时间
		if opts.MaxAge > 0 {
			lastAccess, ok := cs.meta.Entries[hash]
			if ok {
				if time.Unix(lastAccess, 0).Add(opts.MaxAge).After(now) {
					continue // 未过期
				}
			}
		}

		fullPath := filepath.Join(cs.filesDir, hash)
		fi, _ := os.Stat(fullPath)
		var size int64
		if fi != nil {
			size = fi.Size()
		}

		if opts.DryRun {
			logger.Debug("[DRY-RUN] 删除缓存: %s (%.1f KB)", shortHash(hash), float64(size)/1024)
		} else {
			if err := os.Remove(fullPath); err != nil {
				errs = append(errs, fmt.Errorf("删除缓存 %s 失败: %w", shortHash(hash), err))
				continue
			}
		}

		deletedHashes = append(deletedHashes, hash)
		deleted++
		freed += size
	}

	if !opts.DryRun && deleted > 0 {
		cs.meta.TotalFiles -= int64(deleted)
		cs.meta.TotalSize -= freed
		for _, hash := range deletedHashes {
			delete(cs.meta.Entries, hash)
			delete(cs.meta.RefCounts, hash)
		}
		cs.meta.UpdatedAt = time.Now()
		cs.saveMeta()
	}

	return deleted, freed, errs
}

// ReferencedHashes 返回当前所有引用的 hash
func (cs *CacheStore) ReferencedHashes() map[string]bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes := make(map[string]bool, len(cs.meta.RefCounts))
	for hash, count := range cs.meta.RefCounts {
		if count > 0 {
			hashes[hash] = true
		}
	}
	return hashes
}

// Stats 返回缓存统计
func (cs *CacheStore) Stats() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	return fmt.Sprintf("缓存: %d 文件, %.1f MB, %d 引用条目",
		cs.meta.TotalFiles,
		float64(cs.meta.TotalSize)/1024/1024,
		len(cs.meta.RefCounts),
	)
}

// ============================================================
// 辅助方法
// ============================================================

// ComputeSHA256 计算文件 SHA256
func (cs *CacheStore) ComputeSHA256(path string) (string, error) {
	return computeSHA256(path)
}

// ComputeSHA1 计算文件 SHA1
func (cs *CacheStore) ComputeSHA1(path string) (string, error) {
	return computeSHA1(path)
}

// CachedFilePath 返回缓存文件的完整路径
func (cs *CacheStore) CachedFilePath(key string) string {
	return filepath.Join(cs.filesDir, key)
}

// PutData 直接写入字节到缓存
// data: 文件内容
// key: 缓存 key
// 返回缓存文件路径
func (cs *CacheStore) PutData(data []byte, key string) (string, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	destPath := filepath.Join(cs.filesDir, key)

	if _, err := os.Stat(destPath); err == nil {
		cs.meta.RefCounts[key]++
		cs.meta.Entries[key] = time.Now().Unix()
		cs.meta.UpdatedAt = time.Now()
		cs.saveMeta()
		return destPath, nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("创建缓存目录失败: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("写入缓存文件失败: %w", err)
	}

	cs.meta.Entries[key] = time.Now().Unix()
	cs.meta.RefCounts[key] = 1
	cs.meta.TotalFiles++
	cs.meta.TotalSize += int64(len(data))
	cs.meta.UpdatedAt = time.Now()
	cs.saveMeta()

	return destPath, nil
}

// ============================================================
// Persistence
// ============================================================

// cacheMetaPath 返回元数据文件路径
func (cs *CacheStore) cacheMetaPath() string {
	return filepath.Join(cs.baseDir, "cache_meta.json")
}

// loadMeta 从磁盘加载缓存元信息
func (cs *CacheStore) loadMeta() {
	path := cs.cacheMetaPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, cs.meta); err != nil {
		logger.Warn("解析缓存元信息失败: %v", err)
		cs.meta = &CacheMeta{
			Entries:   make(map[string]int64),
			RefCounts: make(map[string]int),
		}
	}
}

// saveMeta 写入缓存元信息
func (cs *CacheStore) saveMeta() {
	path := cs.cacheMetaPath()
	data, err := json.MarshalIndent(cs.meta, "", "  ")
	if err != nil {
		logger.Warn("序列化缓存元信息失败: %v", err)
		return
	}
	// 原子写入
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.Warn("写入缓存元临时文件失败: %v", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		logger.Warn("原子重命名缓存元文件失败: %v", err)
	}
}

// ============================================================
// 包级工具函数
// ============================================================

// computeSHA256 计算文件 SHA256
func computeSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// computeSHA1 计算文件 SHA1
func computeSHA1(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha1.Sum(data)
	return hex.EncodeToString(h[:]), nil
}

// VerifySHA1 校验文件 SHA1（公开方法，给其他包用）
func VerifySHA1(path, expected string) (bool, error) {
	return verifySHA1(path, expected)
}

// DecorateURL 装饰下载 URL（添加镜像 base，可选）
func DecorateURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	path = strings.TrimLeft(path, "/")
	if base == "" {
		return path
	}
	return base + "/" + path
}
