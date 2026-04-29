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
	"sort"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P1.15 客户端增量更新 — 按 hash 拉单个文件
//
// 职责：
//   1. 拉取服务端 server.json，获取当前版本和增量清单
//   2. 比较本地仓库快照与服务端版本
//   3. 只下载变更的文件（新增/更新），删除已移除的文件
//   4. 利用 CacheStore + files/ 双重缓存避免重复下载
//   5. 更新本地仓库快照链
//
// 与 sync 的关系：
//   - sync（现有）：下载 Mojang 原版文件（assets/libraries/version）
//   - update（新增）：下载整合包模组/配置的增量更新
//   - 两者独立运行，update 依赖 repo 快照存在
// ============================================================

// UpdateResult 更新操作的结果统计
type UpdateResult struct {
	Version       string   `json:"version"`        // 更新到的版本
	FromVersion   string   `json:"from_version"`   // 从哪个版本更新
	Added         int      `json:"added"`          // 新增文件数
	Updated       int      `json:"updated"`        // 更新文件数
	Deleted       int      `json:"deleted"`        // 删除文件数
	Skipped       int      `json:"skipped"`        // 跳过（未变）的文件数
	CacheHits     int      `json:"cache_hits"`     // 缓存命中数
	Downloaded    int      `json:"downloaded"`     // 实际下载数
	DownloadBytes int64    `json:"download_bytes"` // 下载字节数
	Errors        []string `json:"errors,omitempty"`
}

// UpdatedFiles 变更的文件列表（用于显示）
type UpdatedFiles struct {
	Added   []string `json:"added"`
	Updated []string `json:"updated"`
	Deleted []string `json:"deleted"`
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
		return "已是最新版本 ✓"
	}

	cacheStr := ""
	if r.CacheHits > 0 {
		cacheStr = fmt.Sprintf(" (缓存命中 %d 个)", r.CacheHits)
	}

	bytesStr := ""
	if r.DownloadBytes > 0 {
		bytesStr = fmt.Sprintf(", %.1f MB 下载", float64(r.DownloadBytes)/1024/1024)
	}

	return fmt.Sprintf("%s → %s: %s | 下载 %d 个文件%s%s",
		r.FromVersion, r.Version, changeStr, r.Downloaded, cacheStr, bytesStr)
}

// ChangedFiles 返回变更的文件名列表（用于显示）
func (r *UpdateResult) ChangedFiles() *UpdatedFiles {
	// 这里无法精确追踪，结果只是统计摘要
	// 实际的变更文件列表在 update 过程中已逐行打印
	return nil
}

// ============================================================
// 服务端配置扩展 — 增量更新字段
// ============================================================

// ServerUpdateInfo 服务端增量更新配置
// 嵌入到 server.json 的 update 字段
type ServerUpdateInfo struct {
	Mode          string             `json:"mode"`                     // "incremental" | "full"
	Version       string             `json:"version"`                  // 当前版本（如 "v1.2.0"）
	FromVersion   string             `json:"from_version,omitempty"`   // 上一版本（用于增量路径判断）
	MCVersion     string             `json:"mc_version,omitempty"`     // Minecraft 版本
	BaseURL       string             `json:"base_url"`                 // 文件下载基地址
	FilesURL      string             `json:"files_url,omitempty"`      // 文件清单 URL（替代内联）
	Incremental   *IncrementalInfo   `json:"incremental,omitempty"`    // 增量清单（内联）
	FullPack      *FullPackInfo      `json:"full_pack,omitempty"`      // 全量兜底
	ManifestURL   string             `json:"manifest_url,omitempty"`   // 全量清单 URL
}

// IncrementalInfo 增量更新信息
type IncrementalInfo struct {
	Added   []FileChangeEntry `json:"added"`   // 新增的文件
	Updated []FileChangeEntry `json:"updated"` // 更新的文件
	Removed []string          `json:"removed"` // 删除的文件路径列表
}

// FullPackInfo 全量兜底下载信息
type FullPackInfo struct {
	URL  string `json:"url"`  // 全量包下载 URL
	Hash string `json:"hash"` // SHA256 hex
	Size int64  `json:"size"` // 字节数
}

// FileChangeEntry 增量中的单个文件变更
type FileChangeEntry struct {
	Path   string `json:"path"`   // 相对路径（如 "mods/sodium.jar"）
	Hash   string `json:"hash"`   // SHA256 hex
	Size   int64  `json:"size"`   // 字节数
	URL    string `json:"url"`    // 下载 URL
}

// ============================================================
// Updater — 客户端增量更新引擎
// ============================================================

// Updater 客户端增量更新管理器
//
// 使用流程：
//   1. NewUpdater(cfgDir, mcDir) — 创建（自动初始化 CacheStore + LocalRepo）
//   2. FetchUpdateInfo(serverURL) — 拉取服务端更新信息
//   3. CheckLocalVersion() — 检查本地版本
//   4. ApplyUpdate(dryRun) — 应用增量更新
type Updater struct {
	cfgDir        string
	mcDir         string
	cache         *CacheStore
	repo          *LocalRepo
	httpClient    *http.Client
	updateInfo    *ServerUpdateInfo
	localVersion  string // 当前本地版本（从 repo latest snapshot 读取）
}

// NewUpdater 创建增量更新管理器
// cfgDir: 配置目录（config/）
// mcDir: .minecraft 目录
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
func (u *Updater) CacheStore() *CacheStore {
	return u.cache
}

// LocalRepo 返回内部仓库
func (u *Updater) LocalRepo() *LocalRepo {
	return u.repo
}

// UpdateInfo 返回拉取到的服务端更新信息
func (u *Updater) UpdateInfo() *ServerUpdateInfo {
	return u.updateInfo
}

// ============================================================
// 步骤 1: 拉取服务端更新信息
// ============================================================

// FetchUpdateInfo 从服务端拉取增量更新信息
// serverURL: server.json 的完整 URL（如 "https://example.com/mc/server.json"）
// 或本地文件路径（用于开发测试）
func (u *Updater) FetchUpdateInfo(serverURL string) (*ServerUpdateInfo, error) {
	logger.Info("拉取服务端更新信息: %s", serverURL)

	data, err := u.fetchURL(serverURL)
	if err != nil {
		return nil, fmt.Errorf("拉取服务端配置失败: %w", err)
	}

	// 解析顶层 server.json
	var raw struct {
		Update *ServerUpdateInfo `json:"update"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析服务端配置失败: %w", err)
	}

	if raw.Update == nil {
		return nil, fmt.Errorf("服务端配置缺少 update 字段")
	}

	info := raw.Update

	// 验证字段完整性
	if info.Version == "" {
		return nil, fmt.Errorf("服务端 update.version 为空")
	}
	if info.BaseURL == "" && (info.Incremental != nil && len(info.Incremental.Added) > 0) {
		return nil, fmt.Errorf("服务端 update.base_url 为空，无法下载文件")
	}

	// 如果指定了外部清单 URL，拉取补充
	if info.FilesURL != "" {
		logger.Info("拉取外部文件清单: %s", info.FilesURL)
		filesData, err := u.fetchURL(info.FilesURL)
		if err != nil {
			return nil, fmt.Errorf("拉取文件清单失败: %w", err)
		}
		var external IncrFilesPayload
		if err := json.Unmarshal(filesData, &external); err != nil {
			return nil, fmt.Errorf("解析文件清单失败: %w", err)
		}
		// 合并到 Incremental 字段
		if info.Incremental == nil {
			info.Incremental = &IncrementalInfo{}
		}
		info.Incremental.Added = append(info.Incremental.Added, external.Added...)
		info.Incremental.Updated = append(info.Incremental.Updated, external.Updated...)
		info.Incremental.Removed = append(info.Incremental.Removed, external.Removed...)
	}

	u.updateInfo = info
	logger.Info("服务端版本: %s (from: %s, mode: %s)",
		info.Version, info.FromVersion, info.Mode)
	return info, nil
}

// IncrFilesPayload 外部增量文件清单格式
type IncrFilesPayload struct {
	Added   []FileChangeEntry `json:"added"`
	Updated []FileChangeEntry `json:"updated"`
	Removed []string          `json:"removed"`
}

// ============================================================
// 步骤 2: 检查本地版本
// ============================================================

// CheckLocalVersion 检查本地仓库中最新的整合包版本
// 返回空串表示没有本地版本（需全量下载）
func (u *Updater) CheckLocalVersion() string {
	if !u.repo.IsInitialized() || !u.repo.HasSnapshots() {
		logger.Info("本地仓库为空，没有可用的版本信息")
		u.localVersion = ""
		return ""
	}

	latest := u.repo.LatestSnapshot()
	u.localVersion = latest

	logger.Info("本地版本: %s", latest)
	return latest
}

// ============================================================
// 步骤 3: 应用更新
// ============================================================

// ApplyUpdate 执行增量更新
// dryRun: true 则只显示变更，不实际下载和写入
func (u *Updater) ApplyUpdate(dryRun bool) (*UpdateResult, error) {
	if u.updateInfo == nil {
		return nil, fmt.Errorf("请先调用 FetchUpdateInfo 拉取服务端信息")
	}

	result := &UpdateResult{
		Version:     u.updateInfo.Version,
		FromVersion: u.localVersion,
	}

	// 检查版本是否已最新
	if u.updateInfo.Mode == "incremental" && u.localVersion == u.updateInfo.Version {
		logger.Info("已是服务端最新版本 %s，跳过", u.localVersion)
		result.Skipped = -1 // 特殊标记：已最新
		return result, nil
	}

	// 全量模式直接提示
	if u.updateInfo.Mode == "full" || (u.updateInfo.Incremental == nil && u.updateInfo.FullPack != nil) {
		return nil, fmt.Errorf("服务端要求全量更新 (mode=%s)，请使用全量包下载: %s",
			u.updateInfo.Mode, u.updateInfo.FullPack.URL)
	}

	incr := u.updateInfo.Incremental
	if incr == nil {
		// 无增量信息，可能服务端配置不全
		// 如果有 manifest_url 则拉全量清单
		if u.updateInfo.ManifestURL != "" {
			return nil, fmt.Errorf("服务端仅提供全量清单，不支持增量更新: %s", u.updateInfo.ManifestURL)
		}
		// 回退到全量下载
		if u.updateInfo.FullPack != nil {
			return nil, fmt.Errorf("服务端无增量信息，请使用全量包: %s", u.updateInfo.FullPack.URL)
		}
		return result, fmt.Errorf("服务端增量信息为空")
	}

	// =============================================
	// 逐类处理变更
	// =============================================

	// 处理删除
	for _, relPath := range incr.Removed {
		result.Deleted++
		fullPath := filepath.Join(u.mcDir, relPath)

		if dryRun {
			logger.Info("[DRY-RUN] 将删除: %s", relPath)
			continue
		}

		if err := os.Remove(fullPath); err != nil {
			if !os.IsNotExist(err) {
				errMsg := fmt.Sprintf("删除 %s 失败: %v", relPath, err)
				logger.Warn(errMsg)
				result.Errors = append(result.Errors, errMsg)
			}
			// 不存在也算已删除
		}
		logger.Debug("已删除: %s", relPath)
	}

	// 构建 added path 索引（用于 isAdded 判断，避免 O(n²)）
	addedPaths := make(map[string]bool, len(incr.Added))
	for _, a := range incr.Added {
		addedPaths[a.Path] = true
	}

	// 处理新增 + 更新（逻辑相同：下载/缓存到目标位置）
	allChanges := append(incr.Added, incr.Updated...)
	for _, entry := range allChanges {
		fullPath := filepath.Join(u.mcDir, entry.Path)
		isAdd := addedPaths[entry.Path]

		if dryRun {
			hashShort := entry.Hash
			if len(hashShort) > 16 {
				hashShort = hashShort[:16]
			}
			if isAdd {
				logger.Info("[DRY-RUN] 新增: %s (%s, %.1f KB)", entry.Path, hashShort, float64(entry.Size)/1024)
			} else {
				logger.Info("[DRY-RUN] 更新: %s (%s, %.1f KB)", entry.Path, hashShort, float64(entry.Size)/1024)
			}
			result.Skipped++
			continue
		}

		// 尝试从缓存获取
		cachedPath, err := u.cache.Get(entry.Hash)
		if err == nil && cachedPath != "" {
			// 缓存命中，直接复制
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				errMsg := fmt.Sprintf("创建目录 %s 失败: %v", filepath.Dir(fullPath), err)
				logger.Warn(errMsg)
				result.Errors = append(result.Errors, errMsg)
				continue
			}
			if err := copyFile(cachedPath, fullPath); err != nil {
				errMsg := fmt.Sprintf("从缓存复制 %s 失败: %v", entry.Path, err)
				logger.Warn(errMsg)
				result.Errors = append(result.Errors, errMsg)
				continue
			}
			result.CacheHits++
			if isAdd {
				result.Added++
			} else {
				result.Updated++
			}
			logger.Debug("缓存命中: %s", entry.Path)
			continue
		}

		// 缓存未命中，下载
		downloadURL := entry.URL
		if downloadURL == "" {
			// 使用 baseURL + hash 构造下载路径
			downloadURL = fmt.Sprintf("%s/%s", strings.TrimRight(u.updateInfo.BaseURL, "/"), entry.Hash)
		}

		if err := u.downloadFile(downloadURL, fullPath, entry.Hash); err != nil {
			errMsg := fmt.Sprintf("下载 %s 失败: %v", entry.Path, err)
			logger.Warn(errMsg)
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		result.Downloaded++
		result.DownloadBytes += entry.Size

		if isAdd {
			result.Added++
		} else {
			result.Updated++
		}

		// 存入缓存供后续复用
		u.cache.Put(fullPath, entry.Hash)

		logger.Debug("已下载: %s (%s)", entry.Path, downloadURL)
	}

	// =============================================
	// 步骤 4: 更新本地 repo 快照
	// =============================================
	if !dryRun {
		if err := u.updateLocalSnapshot(); err != nil {
			logger.Warn("更新本地快照失败(非致命): %v", err)
			result.Errors = append(result.Errors, fmt.Sprintf("更新快照: %v", err))
		}
	}

	// 统计跳过数（未变动的文件）
	if result.Added == 0 && result.Updated == 0 && result.Deleted == 0 && result.Skipped == 0 && result.Downloaded == 0 {
		changeCount := len(incr.Added) + len(incr.Updated) + len(incr.Removed)
		result.Skipped = changeCount - (result.CacheHits + result.Downloaded)
	}

	logger.Info("更新完成: %s", result.Summary())
	return result, nil
}

// ============================================================
// 辅助: 更新本地快照
// ============================================================

// updateLocalSnapshot 在更新完成后创建/更新本地 repo 快照
func (u *Updater) updateLocalSnapshot() error {
	snapshotName := fmt.Sprintf("update-%s", u.updateInfo.Version)

	// 确保 repo 已初始化
	mcVer := u.updateInfo.MCVersion
	if mcVer == "" {
		mcVer = "unknown"
	}
	if err := u.repo.Init(mcVer); err != nil {
		return fmt.Errorf("初始化 repo 失败: %w", err)
	}

	// 创建快照（扫描 mods + config）
	manifest, err := u.repo.CreateFullSnapshot(snapshotName, []string{"mods", "config"}, "incremental_update", u.localVersion)
	if err != nil {
		return fmt.Errorf("创建快照失败: %w", err)
	}

	_ = manifest
	logger.Info("快照已更新: %s", snapshotName)

	// 清理孤立缓存文件（后台任务，失败不阻塞）
	go func() {
		refs, err := u.repo.ReferencedHashes()
		if err != nil {
			logger.Warn("获取引用 hash 失败(清理跳过): %v", err)
			return
		}
		opts := CleanOptions{
			DryRun:      false,
			MinRefCount: 0,
			KeepHashes:  refs,
		}
		deleted, freed, _ := u.cache.Clean(opts)
		if deleted > 0 {
			logger.Info("缓存清理: 删除 %d 个孤立文件, 释放 %.1f KB", deleted, float64(freed)/1024)
		}
	}()

	return nil
}

// ============================================================
// 辅助: 全量下载兜底
// ============================================================

// DownloadFullPack 全量下载整合包（增量不可用时的兜底方案）
// 下载全量 zip 并解压到 .minecraft 目录
func (u *Updater) DownloadFullPack(dryRun bool) (*UpdateResult, error) {
	if u.updateInfo == nil || u.updateInfo.FullPack == nil {
		return nil, fmt.Errorf("服务端未提供全量包信息")
	}

	result := &UpdateResult{
		Version:     u.updateInfo.Version,
		FromVersion: u.localVersion,
	}

	fullPack := u.updateInfo.FullPack

	if dryRun {
		logger.Info("[DRY-RUN] 将下载全量包: %s (%.1f MB)", fullPack.URL, float64(fullPack.Size)/1024/1024)
		result.Skipped = 1
		return result, nil
	}

	logger.Info("下载全量包: %s", fullPack.URL)

	// 下载到临时文件
	tmpFile, err := os.CreateTemp("", "mc-pack-full-*.zip")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := u.downloadToFile(fullPack.URL, tmpPath); err != nil {
		return nil, fmt.Errorf("下载全量包失败: %w", err)
	}

	// 校验 hash
	if fullPack.Hash != "" {
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return nil, fmt.Errorf("读取临时文件失败: %w", err)
		}
		h := sha256.Sum256(data)
		gotHash := hex.EncodeToString(h[:])
		if gotHash != fullPack.Hash {
			return nil, fmt.Errorf("hash 不匹配: 期望 %s, 实际 %s", fullPack.Hash, gotHash)
		}
	}

	// 解压到 .minecraft（覆盖 mods/config 等目录）
	if err := u.extractZip(tmpPath, u.mcDir); err != nil {
		return nil, fmt.Errorf("解压全量包失败: %w", err)
	}

	// 更新快照
	snapshotName := fmt.Sprintf("full-%s", u.updateInfo.Version)
	mcVer := u.updateInfo.MCVersion
	if mcVer == "" {
		mcVer = "unknown"
	}
	if err := u.repo.Init(mcVer); err != nil {
		logger.Warn("repo 初始化失败(非致命): %v", err)
	} else {
		if _, err := u.repo.CreateFullSnapshot(snapshotName, []string{"mods", "config"}, "full_download", ""); err != nil {
			logger.Warn("创建全量快照失败(非致命): %v", err)
		}
	}

	result.Downloaded = 1 // 1 个 zip
	result.Added = 1      // 全量更新
	logger.Info("全量包下载并解压完成: %s", snapshotName)

	return result, nil
}

// ============================================================
// 辅助: HTTP 文件操作
// ============================================================

// fetchURL 获取 URL 内容
func (u *Updater) fetchURL(url string) ([]byte, error) {
	// 支持本地文件路径（开发测试用）
	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "file://") {
		filePath := strings.TrimPrefix(url, "file://")
		return os.ReadFile(filePath)
	}

	resp, err := u.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s 失败: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	return data, nil
}

// downloadFile 下载文件到指定路径，校验 hash
func (u *Updater) downloadFile(downloadURL, destPath, expectedHash string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	// 下载
	resp, err := u.httpClient.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, downloadURL)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	_ = written

	// 校验 SHA256
	if expectedHash != "" {
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("读取已下载文件失败: %w", err)
		}
		h := sha256.Sum256(data)
		gotHash := hex.EncodeToString(h[:])
		if gotHash != expectedHash {
			return fmt.Errorf("hash 不匹配: 期望 %s, 实际 %s", expectedHash, gotHash)
		}
	}

	// 原子重命名
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}

	return nil
}

// downloadToFile 下载到文件（不校验 hash）
func (u *Updater) downloadToFile(url, destPath string) error {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// extractZip 解压 zip 到目标目录
// 覆盖现有文件，不保留 zip 内的目录结构到目标目录
func (u *Updater) extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		// 跳过目录条目
		if f.FileInfo().IsDir() {
			continue
		}

		targetPath := filepath.Join(destDir, f.Name)

		// 安全检查：防止 zip slip
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			logger.Warn("跳过路径越界的文件: %s", f.Name)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", filepath.Dir(targetPath), err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("打开 zip 条目 %s 失败: %w", f.Name, err)
		}

		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("创建文件 %s 失败: %w", targetPath, err)
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("解压文件 %s 失败: %w", f.Name, err)
		}
	}

	return nil
}

// ============================================================
// 服务端增量清单构建工具
// ============================================================

// BuildServerUpdateInfo 根据本地仓库快照构建服务端增量清单
// 用于服务端生成增量更新配置
// serverVersion: 新版本号
// prevSnapshot: 上一版本的快照名
// newSnapshot: 当前版本的快照名
// baseURL: 文件下载基地址
func (u *Updater) BuildServerUpdateInfo(serverVersion, prevSnapshot, newSnapshot, baseURL string) (*ServerUpdateInfo, error) {
	if prevSnapshot == "" {
		// 首次发布 → 全量模式
		// 这里只返回一个占位，实际应由服务端 pack publish 生成
		return &ServerUpdateInfo{
			Mode:    "full",
			Version: serverVersion,
		}, nil
	}

	// 加载两个快照的 manifest
	prevManifest, err := u.repo.LoadSnapshotManifest(prevSnapshot)
	if err != nil {
		return nil, fmt.Errorf("加载旧快照 %s 失败: %w", prevSnapshot, err)
	}

	newManifest, err := u.repo.LoadSnapshotManifest(newSnapshot)
	if err != nil {
		return nil, fmt.Errorf("加载新快照 %s 失败: %w", newSnapshot, err)
	}

	// 计算差异
	diff := u.repo.ComputeDiff(prevManifest, newManifest)

	info := &ServerUpdateInfo{
		Mode:        "incremental",
		Version:     serverVersion,
		FromVersion: prevSnapshot,
		BaseURL:     baseURL,
		Incremental: &IncrementalInfo{
			Added:   make([]FileChangeEntry, 0),
			Updated: make([]FileChangeEntry, 0),
			Removed: make([]string, 0),
		},
	}

	// 转换 Added
	for _, f := range diff.Added {
		info.Incremental.Added = append(info.Incremental.Added, FileChangeEntry{
			Path: f.RelPath,
			Hash: f.NewHash,
			Size: f.Size,
			URL:  fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), f.NewHash),
		})
	}

	// 转换 Updated
	for _, f := range diff.Updated {
		info.Incremental.Updated = append(info.Incremental.Updated, FileChangeEntry{
			Path: f.RelPath,
			Hash: f.NewHash,
			Size: f.Size,
			URL:  fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), f.NewHash),
		})
	}

	// 排序保证一致性
	sort.Slice(info.Incremental.Added, func(i, j int) bool {
		return info.Incremental.Added[i].Path < info.Incremental.Added[j].Path
	})
	sort.Slice(info.Incremental.Updated, func(i, j int) bool {
		return info.Incremental.Updated[i].Path < info.Incremental.Updated[j].Path
	})
	sort.Strings(info.Incremental.Removed)

	// 转换 Removed
	for _, f := range diff.Deleted {
		info.Incremental.Removed = append(info.Incremental.Removed, f.RelPath)
	}

	return info, nil
}
