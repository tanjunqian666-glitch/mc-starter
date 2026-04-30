package launcher

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// 客户端增量更新 — 整合包同步引擎（适配 C/S + 多包）
//
// 职责：从服务端拉增量清单 → 按 hash 下载文件 → 写 CacheStore
// 不负责 MC 原版文件（那是 sync 做的事）
// ============================================================

// UpdateResult 更新操作的结果统计
type UpdateResult struct {
	PackName      string   `json:"pack_name"`
	Version       string   `json:"version"`
	FromVersion   string   `json:"from_version"`
	Added         int      `json:"added"`
	Updated       int      `json:"updated"`
	Deleted       int      `json:"deleted"`
	Skipped       int      `json:"skipped"`
	CacheHits     int      `json:"cache_hits"`
	Downloaded    int      `json:"downloaded"`
	DownloadBytes int64    `json:"download_bytes"`
	Errors        []string `json:"errors,omitempty"`
}

// Summary 返回人类可读的摘要
func (r *UpdateResult) Summary() string {
	parts := []string{}
	if r.Added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", r.Added))
	}
	if r.Updated > 0 {
		parts = append(parts, fmt.Sprintf("~%d", r.Updated))
	}
	if r.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("-%d", r.Deleted))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("=%d", r.Skipped))
	}
	changeStr := strings.Join(parts, ", ")
	if changeStr == "" {
		return fmt.Sprintf("[%s] 已是最新版本 ✓", r.PackName)
	}

	cacheStr := ""
	if r.CacheHits > 0 {
		cacheStr = fmt.Sprintf(" (缓存命中 %d 个)", r.CacheHits)
	}

	bytesStr := ""
	if r.DownloadBytes > 0 {
		bytesStr = fmt.Sprintf(", %.1f MB 下载", float64(r.DownloadBytes)/1024/1024)
	}

	return fmt.Sprintf("[%s] %s → %s: %s | 下载 %d 个文件%s%s",
		r.PackName, r.FromVersion, r.Version, changeStr, r.Downloaded, cacheStr, bytesStr)
}

// ============================================================
// Updater — 客户端增量更新管理器
// ============================================================

type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// Updater 客户端增量更新管理器
type Updater struct {
	cfgDir     string
	mcDir      string
	cache      *CacheStore
	repo       *LocalRepo
	httpClient HTTPClient
}

// NewUpdater 创建增量更新管理器
func NewUpdater(cfgDir, mcDir string) *Updater {
	cacheDir := filepath.Join(cfgDir, ".cache", "mc_cache")
	cache := NewCacheStore(cacheDir)
	cache.Init()

	repo := NewLocalRepo(mcDir)

	return &Updater{
		cfgDir: cfgDir,
		mcDir:  mcDir,
		cache:  cache,
		repo:   repo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CacheStore 返回内部缓存
func (u *Updater) CacheStore() *CacheStore { return u.cache }

// LocalRepo 返回内部仓库
func (u *Updater) LocalRepo() *LocalRepo { return u.repo }

// ============================================================
// API 助手
// ============================================================

type updateAPI interface {
	FetchUpdate(serverURL, packName, fromVersion string) (*model.IncrementalUpdate, error)
	DownloadFile(serverURL, packName, fileHash, destPath string) error
}

func makeUpdateAPI(cfgDir string) updateAPI {
	// 返回一个简单的实现，使用 config.Manager 的 HTTP 方法
	return &httpUpdateAPI{cfgDir: cfgDir}
}

type httpUpdateAPI struct {
	cfgDir string
}

func (h *httpUpdateAPI) fetch(url string) ([]byte, error) {
	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "file://") {
		filePath := strings.TrimPrefix(url, "file://")
		return os.ReadFile(filePath)
	}
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (h *httpUpdateAPI) FetchUpdate(serverURL, packName, fromVersion string) (*model.IncrementalUpdate, error) {
	base := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/update", base, packName)
	if fromVersion != "" {
		url += "?from=" + fromVersion
	}
	data, err := h.fetch(url)
	if err != nil {
		return nil, err
	}
	var u model.IncrementalUpdate
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (h *httpUpdateAPI) DownloadFile(serverURL, packName, fileHash, destPath string) error {
	base := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/files/%s", base, packName, fileHash)
	return downloadToFile(url, destPath)
}

func downloadToFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, destPath)
}

// ============================================================
// 核心：更新单个包
// ============================================================

// UpdatePack 更新指定包
// packState: 本地包状态（含版本号），nil 表示首次安装
// forceFull: 强制全量更新
func (u *Updater) UpdatePack(serverURL, packName string, packState *model.PackState, forceFull bool) (*UpdateResult, error) {
	result := &UpdateResult{PackName: packName}

	api := makeUpdateAPI(u.cfgDir)

	// 1. 拉取增量信息
	localVer := ""
	if packState != nil && packState.LocalVersion != "" {
		localVer = packState.LocalVersion
	}
	result.FromVersion = localVer

	update, err := api.FetchUpdate(serverURL, packName, localVer)
	if err != nil {
		return nil, fmt.Errorf("拉取 %s 更新信息失败: %w", packName, err)
	}
	result.Version = update.Version

	// 已是最新
	if localVer == update.Version && !forceFull {
		result.Skipped = -1
		logger.Info("[%s] 已是最新: %s", packName, localVer)
		return result, nil
	}

	// 全量模式
	if update.Mode == "full" || forceFull {
		logger.Info("[%s] 全量更新: %s → %s", packName, localVer, update.Version)
		return u.applyFullUpdate(api, serverURL, packName, update, result)
	}

	// 2. 增量模式
	return u.applyIncremental(api, serverURL, packName, update, result, packState)
}

// ============================================================
// 增量更新
// ============================================================

func (u *Updater) applyIncremental(api updateAPI, serverURL, packName string, update *model.IncrementalUpdate, result *UpdateResult, packState *model.PackState) (*UpdateResult, error) {
	incr := update
	packDir := filepath.Join(u.mcDir, "packs", packName)
	_ = packState

	// 删除已移除文件
	for _, relPath := range incr.Removed {
		fullPath := filepath.Join(packDir, relPath)
		if err := os.Remove(fullPath); err != nil {
			if !os.IsNotExist(err) {
				result.Errors = append(result.Errors, fmt.Sprintf("删除 %s 失败: %v", relPath, err))
			}
		}
		result.Deleted++
		logger.Debug("[%s] 已删除: %s", packName, relPath)
	}

	// 处理新增 + 更新
	addedPaths := make(map[string]bool, len(incr.Added))
	for _, a := range incr.Added {
		addedPaths[a.Path] = true
	}

	allChanges := append(incr.Added, incr.Updated...)
	for _, entry := range allChanges {
		fullPath := filepath.Join(packDir, entry.Path)
		isAdd := addedPaths[entry.Path]

		// 查缓存
		cachedPath, err := u.cache.Get(entry.Hash)
		if err == nil && cachedPath != "" {
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("创建目录失败: %v", err))
				continue
			}
			if err := copyFile(cachedPath, fullPath); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("缓存复制 %s 失败: %v", entry.Path, err))
				continue
			}
			result.CacheHits++
			if isAdd {
				result.Added++
			} else {
				result.Updated++
			}
			logger.Debug("[%s] 缓存命中: %s", packName, entry.Path)
			continue
		}

		// 下载
		if err := api.DownloadFile(serverURL, packName, entry.Hash, fullPath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("下载 %s 失败: %v", entry.Path, err))
			continue
		}

		result.Downloaded++
		result.DownloadBytes += entry.Size
		if isAdd {
			result.Added++
		} else {
			result.Updated++
		}

		// 写入缓存
		u.cache.Put(fullPath, entry.Hash)
		logger.Debug("[%s] 已下载: %s", packName, entry.Path)
	}

	// 更新 repo 快照
	if err := u.updatePackSnapshot(packName, update.Version); err != nil {
		logger.Warn("[%s] 快照更新失败(非致命): %v", packName, err)
		result.Errors = append(result.Errors, fmt.Sprintf("快照: %v", err))
	}

	logger.Info("[%s] %s", packName, result.Summary())
	return result, nil
}

// ============================================================
// 全量更新（兜底）
// ============================================================

func (u *Updater) applyFullUpdate(api updateAPI, serverURL, packName string, update *model.IncrementalUpdate, result *UpdateResult) (*UpdateResult, error) {
	// 全量时，需先拉 manifest 或整包 zip
	// 目前全量未做完整实现，先报错提示
	return result, fmt.Errorf("[%s] 全量更新尚未实现，请使用 starter sync 先同步后再 update", packName)
}

// ============================================================
// 多包批量更新
// ============================================================

// UpdateAllPacks 更新所有已启用的包
// 返回每个包各自的更新结果
func (u *Updater) UpdateAllPacks(serverURL string, packs map[string]model.PackState) map[string]*UpdateResult {
	results := make(map[string]*UpdateResult)

	// 先排主包（如果有 server_url 则从 API 获取 primary 标记）
	for name, state := range packs {
		if !state.Enabled {
			continue
		}
		r, err := u.UpdatePack(serverURL, name, &state, false)
		if err != nil {
			logger.Warn("[%s] 更新失败: %v", name, err)
			results[name] = r
			if r == nil {
				results[name] = &UpdateResult{PackName: name, Errors: []string{err.Error()}}
			}
			continue
		}
		results[name] = r
	}

	return results
}

// ============================================================
// repo 快照
// ============================================================

func (u *Updater) updatePackSnapshot(packName, version string) error {
	// 快照基于包目录创建
	snapshotName := fmt.Sprintf("update-%s-%s", packName, version)
	packDir := filepath.Join(u.mcDir, "packs", packName)

	if err := u.repo.Init("packs"); err != nil {
		return fmt.Errorf("初始化 repo 失败: %w", err)
	}

	// 扫描包目录
	var scanDirs []string
	if _, err := os.Stat(filepath.Join(packDir, "mods")); err == nil {
		scanDirs = append(scanDirs, filepath.Join("packs", packName, "mods"))
	}
	if _, err := os.Stat(filepath.Join(packDir, "config")); err == nil {
		scanDirs = append(scanDirs, filepath.Join("packs", packName, "config"))
	}

	manifest, err := u.repo.CreateFullSnapshot(snapshotName, scanDirs, "pack_update", "")
	if err != nil {
		return err
	}
	_ = manifest
	logger.Debug("快照已更新: %s", snapshotName)

	// 后台清理缓存
	go func() {
		refs, err := u.repo.ReferencedHashes()
		if err != nil {
			return
		}
		u.cache.Clean(CleanOptions{
			DryRun:     false,
			MinRefCount: 0,
			KeepHashes: refs,
		})
	}()

	return nil
}

// ============================================================
// 工具函数
// ============================================================

// CheckLocalVersion 检查本地仓库中最新的整合包版本
func (u *Updater) CheckLocalVersion(packName string) string {
	if !u.repo.IsInitialized() || !u.repo.HasSnapshots() {
		return ""
	}
	return u.repo.LatestSnapshot()
}

// DownloadFullPack 全量下载整合包（增量不可用时的兜底方案）
func (u *Updater) DownloadFullPack(fullPackURL, fullPackHash, destDir string) (*UpdateResult, error) {
	result := &UpdateResult{PackName: "full"}

	tmpFile, err := os.CreateTemp("", "mc-pack-full-*.zip")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := downloadToFile(fullPackURL, tmpPath); err != nil {
		return nil, fmt.Errorf("下载全量包失败: %w", err)
	}

	if fullPackHash != "" {
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return nil, err
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != fullPackHash {
			return nil, fmt.Errorf("hash 不匹配")
		}
	}

	if err := extractZip(tmpPath, destDir); err != nil {
		return nil, fmt.Errorf("解压全量包失败: %w", err)
	}

	result.Downloaded = 1
	result.Added = 1
	return result, nil
}

// extractZip 解压 zip 到目标目录
func extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		targetPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			logger.Warn("跳过路径越界的文件: %s", f.Name)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// copyFile 复制文件
