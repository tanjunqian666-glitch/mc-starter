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

// ManifestURLs 版本清单下载地址（带镜像兜底）
var ManifestURLs = struct {
	Direct string // Mojang 官方
	Mirror string // BMCLAPI 镜像
}{
	Direct: "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
	Mirror: "https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json",
}

// VersionEntry 清单中的单个版本
type VersionEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Time        string `json:"time"`
	ReleaseTime string `json:"releaseTime"`
	Sha1        string `json:"sha1,omitempty"`
}

// VersionManifest 版本清单顶层结构
type VersionManifest struct {
	Latest   LatestVersions `json:"latest"`
	Versions []VersionEntry `json:"versions"`
	// 元信息（非 API 返回）
	fetchedAt time.Time `json:"-"`
}

// LatestVersions 最新版本号
type LatestVersions struct {
	Release  string `json:"release"`
	Snapshot string `json:"snapshot"`
}

// VersionManifestManager 版本清单管理器
type VersionManifestManager struct {
	mu       sync.RWMutex
	manifest *VersionManifest
	cacheDir string
	dl       *downloader.Downloader
}

// NewVersionManifestManager 创建版本清单管理器
func NewVersionManifestManager(cacheDir string) *VersionManifestManager {
	return &VersionManifestManager{
		cacheDir: cacheDir,
		dl:       downloader.New(),
	}
}

// Fetch 拉取最新版本清单，优先 BMCLAPI 镜像，失败回退官方
// cacheTTL: 缓存有效期，0 则强制重新拉取
func (m *VersionManifestManager) Fetch(cacheTTL time.Duration) (*VersionManifest, error) {
	// 尝试从缓存加载
	if cacheTTL > 0 {
		if manifest, ok := m.loadCached(); ok {
			if time.Since(manifest.fetchedAt) < cacheTTL {
				m.mu.Lock()
				m.manifest = manifest
				m.mu.Unlock()
				logger.Debug("版本清单缓存命中, 抓取时间: %s", manifest.fetchedAt.Format(time.RFC3339))
				return manifest, nil
			}
			logger.Debug("版本清单缓存过期, 重新拉取")
		}
	}

	// 尝试镜像，失败回退官方
	urls := []string{ManifestURLs.Mirror, ManifestURLs.Direct}
	var lastErr error
	for _, url := range urls {
		manifest, err := m.fetchFromURL(url)
		if err == nil {
			manifest.fetchedAt = time.Now()
			m.mu.Lock()
			m.manifest = manifest
			m.mu.Unlock()
			m.saveCache(manifest)
			logger.Info("版本清单拉取成功, 来源: %s", url)
			return manifest, nil
		}
		lastErr = err
		logger.Warn("版本清单拉取失败(来源=%s): %v", url, err)
	}

	return nil, fmt.Errorf("版本清单拉取失败(所有来源): %w", lastErr)
}

// GetManifest 获取当前缓存的清单（不重新拉取）
func (m *VersionManifestManager) GetManifest() *VersionManifest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.manifest
}

// FindVersion 在清单中查找指定版本
func (m *VersionManifestManager) FindVersion(id string) *VersionEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.manifest == nil {
		return nil
	}
	for _, v := range m.manifest.Versions {
		if v.ID == id {
			return &v
		}
	}
	return nil
}

// ListVersionsByType 按类型列出版本
func (m *VersionManifestManager) ListVersionsByType(typ string, limit int) []VersionEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.manifest == nil {
		return nil
	}
	var result []VersionEntry
	for _, v := range m.manifest.Versions {
		if v.Type == typ {
			result = append(result, v)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// CachePath 返回缓存文件路径
func (m *VersionManifestManager) CachePath() string {
	return filepath.Join(m.cacheDir, "version_manifest_v2.json")
}

// fetchFromURL 从指定 URL 拉取并解析清单
func (m *VersionManifestManager) fetchFromURL(url string) (*VersionManifest, error) {
	tmpDir := filepath.Join(m.cacheDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("manifest_%d.json", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	if err := m.dl.File(url, tmpFile, ""); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("读取临时文件失败: %w", err)
	}

	var manifest VersionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析版本清单失败: %w", err)
	}

	if len(manifest.Versions) == 0 {
		return nil, fmt.Errorf("版本清单为空")
	}

	return &manifest, nil
}

// loadCached 从本地缓存加载清单
func (m *VersionManifestManager) loadCached() (*VersionManifest, bool) {
	path := m.CachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	// 获取文件修改时间作为 fetchedAt
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	var manifest VersionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false
	}

	if len(manifest.Versions) == 0 {
		return nil, false
	}

	// 填充 fetchedAt
	manifest.fetchedAt = fi.ModTime()
	return &manifest, true
}

// saveCache 缓存清单到本地
func (m *VersionManifestManager) saveCache(manifest *VersionManifest) {
	path := m.CachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logger.Warn("创建缓存目录失败: %v", err)
		return
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		logger.Warn("序列化版本清单失败: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logger.Warn("写入版本清单缓存失败: %v", err)
	}
}
