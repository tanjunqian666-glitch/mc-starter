package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/downloader"
	"github.com/gege-tlph/mc-starter/internal/logger"
)

// AssetIndex Asset 索引
type AssetIndex struct {
	Objects    map[string]AssetObject `json:"objects"`
	Virtual    bool                   `json:"virtual,omitempty"`
	fetchedAt  time.Time              `json:"-"`
}

// AssetObject 单个 Asset 文件
type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// AssetManager Asset 索引管理器
type AssetManager struct {
	mu          sync.RWMutex
	cacheDir    string
	downloadDir string // 通常是 .minecraft/assets/
	dl          *downloader.Downloader
	mm          *VersionManifestManager
	vm          *VersionMetaManager
}

// NewAssetManager 创建 Asset 管理器
// cacheDir: 缓存目录（存储 asset index JSON）
// downloadDir: 资源下载目录（assets/objects/）
func NewAssetManager(cacheDir, downloadDir string, mm *VersionManifestManager, vm *VersionMetaManager) *AssetManager {
	return &AssetManager{
		cacheDir:    cacheDir,
		downloadDir: downloadDir,
		dl:          downloader.New(),
		mm:          mm,
		vm:          vm,
	}
}

// FetchIndex 拉取指定版本 ID 的 Asset 索引
func (m *AssetManager) FetchIndex(versionID string) (*AssetIndex, error) {
	// 确保有 version meta
	if m.vm == nil {
		return nil, fmt.Errorf("AssetManager: VersionMetaManager 未设置")
	}

	meta, err := m.vm.Fetch(versionID)
	if err != nil {
		return nil, fmt.Errorf("获取版本元数据失败: %w", err)
	}

	if meta.AssetIndex == nil {
		return nil, fmt.Errorf("版本 %s 没有 assetIndex", versionID)
	}

	ref := meta.AssetIndex

	// 尝试缓存
	cachePath := m.cacheIndexPath(ref.ID)
	if idx, ok := m.loadCachedIndex(cachePath); ok {
		logger.Debug("Asset 索引缓存命中: %s", ref.ID)
		return idx, nil
	}

	// 下载 asset index JSON
	indexesDir := filepath.Join(m.cacheDir, "indexes")
	if err := os.MkdirAll(indexesDir, 0755); err != nil {
		return nil, fmt.Errorf("创建 asset indexes 目录失败: %w", err)
	}

	// 先写入临时文件
	tmpFile := filepath.Join(indexesDir, fmt.Sprintf(".tmp_%s_%d.json", ref.ID, time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	logger.Info("下载 Asset 索引: %s (%d KB)", ref.ID, ref.Size/1024)
	if err := m.dl.File(ref.URL, tmpFile, ""); err != nil {
		return nil, fmt.Errorf("下载 Asset 索引失败: %w", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("读取 Asset 索引失败: %w", err)
	}

	var idx AssetIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析 Asset 索引失败: %w", err)
	}

	// 写入最终缓存
	idx.fetchedAt = time.Now()
	m.saveCachedIndex(cachePath, data)

	logger.Info("Asset 索引就绪: %s (%d 个资源文件, 总计 %d MB)",
		ref.ID, len(idx.Objects), ref.TotalSize/1024/1024)

	return &idx, nil
}

// ListObjects 列出索引中的所有 Asset 对象
func (m *AssetManager) ListObjects(idx *AssetIndex) []struct {
	VirtualPath string
	Hash        string
	Size        int64
} {
	var result []struct {
		VirtualPath string
		Hash        string
		Size        int64
	}
	for vpath, obj := range idx.Objects {
		result = append(result, struct {
			VirtualPath string
			Hash        string
			Size        int64
		}{VirtualPath: vpath, Hash: obj.Hash, Size: obj.Size})
	}
	return result
}

// AssetObjectPath 返回 Asset 文件在本地 objects 目录的路径
func (m *AssetManager) AssetObjectPath(hash string) string {
	if len(hash) < 2 {
		return ""
	}
	return filepath.Join(m.downloadDir, "objects", hash[:2], hash)
}

// ObjectURL 返回 Asset 对象的下载 URL（从官方源）
func ObjectURL(hash string) string {
	if len(hash) < 2 {
		return ""
	}
	return fmt.Sprintf("https://resources.download.minecraft.net/%s/%s", hash[:2], hash)
}

// MirrorObjectURL 返回 Asset 对象的镜像下载 URL
func MirrorObjectURL(hash string) string {
	if len(hash) < 2 {
		return ""
	}
	return fmt.Sprintf("https://bmclapi2.bangbang93.com/assets/%s/%s", hash[:2], hash)
}

// DownloadFile 下载单个 Asset 文件（镜像优先，SHA1 校验）
func (m *AssetManager) DownloadFile(hash, destPath string) error {
	// Asset index 中的 hash 是 SHA1，但 downloader 原生只支持 SHA256
	// 所以我们用 downloader 下载（不传 hash），下载完再做 SHA1 校验
	url := MirrorObjectURL(hash)
	logger.Debug("下载 Asset: %s", hash[:8])

	if err := m.dl.File(url, destPath, ""); err != nil {
		logger.Debug("镜像下载失败，切换到官方: %v", err)
		url = ObjectURL(hash)
		os.Remove(destPath)
		if err := m.dl.File(url, destPath, ""); err != nil {
			return fmt.Errorf("Asset 下载失败(%s): %w", hash[:12], err)
		}
	}

	// SHA1 校验
	ok, err := verifySHA1(destPath, hash)
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("Asset SHA1 校验执行失败(%s): %w", hash[:12], err)
	}
	if !ok {
		os.Remove(destPath)
		return fmt.Errorf("Asset SHA1 不匹配(%s)", hash[:12])
	}

	return nil
}

// IndexID 从 VersionMeta 获取 Asset 索引 ID
func (m *AssetManager) IndexID(versionID string) (string, error) {
	meta, err := m.vm.Fetch(versionID)
	if err != nil {
		return "", err
	}
	if meta.AssetIndex == nil {
		return "", fmt.Errorf("版本 %s 没有 assetIndex", versionID)
	}
	return meta.AssetIndex.ID, nil
}

// Statistics 返回 Asset 索引统计
func (m *AssetManager) Statistics(idx *AssetIndex) struct {
	TotalFiles int
	TotalSize  int64
	AvgSize    float64
} {
	var totalSize int64
	for _, obj := range idx.Objects {
		totalSize += obj.Size
	}
	stats := struct {
		TotalFiles int
		TotalSize  int64
		AvgSize    float64
	}{
		TotalFiles: len(idx.Objects),
		TotalSize:  totalSize,
	}
	if len(idx.Objects) > 0 {
		stats.AvgSize = float64(totalSize) / float64(len(idx.Objects))
	}
	return stats
}

// cacheIndexPath 返回索引缓存路径
func (m *AssetManager) cacheIndexPath(assetID string) string {
	return filepath.Join(m.cacheDir, "indexes", fmt.Sprintf("%s.json", assetID))
}

// loadCachedIndex 从本地加载缓存的 Asset 索引
func (m *AssetManager) loadCachedIndex(path string) (*AssetIndex, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	// Asset 索引缓存 24 小时（不常变）
	if time.Since(fi.ModTime()) > 24*time.Hour {
		return nil, false
	}

	var idx AssetIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, false
	}

	if len(idx.Objects) == 0 {
		return nil, false
	}

	idx.fetchedAt = fi.ModTime()
	return &idx, true
}

// saveCachedIndex 缓存 Asset 索引
func (m *AssetManager) saveCachedIndex(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logger.Warn("创建 Asset 索引缓存目录失败: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logger.Warn("写入 Asset 索引缓存失败: %v", err)
	}
}
