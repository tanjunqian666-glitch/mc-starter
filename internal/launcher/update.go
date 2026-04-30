package launcher

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/config"
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
	api        *config.Manager
}

// NewUpdater 创建增量更新管理器
func NewUpdater(cfgDir, mcDir string, api *config.Manager) *Updater {
	cacheDir := filepath.Join(cfgDir, ".cache", "mc_cache")
	cache := NewCacheStore(cacheDir)
	cache.Init()

	repo := NewLocalRepo(mcDir)

	if api == nil {
		api = config.New(cfgDir)
	}

	return &Updater{
		cfgDir: cfgDir,
		mcDir:  mcDir,
		cache:  cache,
		repo:   repo,
		api:    api,
	}
}

// CacheStore 返回内部缓存
func (u *Updater) CacheStore() *CacheStore { return u.cache }

// LocalRepo 返回内部仓库
func (u *Updater) LocalRepo() *LocalRepo { return u.repo }

// ============================================================
// 核心：更新单个包
// ============================================================

// UpdatePack 更新指定包（P6 扩展：支持频道）
// packState: 本地包状态（含版本号和频道状态），nil 表示首次安装
// forceFull: 强制全量更新
func (u *Updater) UpdatePack(serverURL, packName string, packState *model.PackState, forceFull bool) (*UpdateResult, error) {
	result := &UpdateResult{PackName: packName}

	// 1. 确定已启用的频道列表
	enabledChannels := u.getEnabledChannels(packState)
	localVer := ""
	if packState != nil && packState.LocalVersion != "" {
		localVer = packState.LocalVersion
	}
	result.FromVersion = localVer

	// 2. 拉取增量信息（带上已启用的频道）
	update, err := u.api.FetchUpdate(serverURL, packName, localVer, enabledChannels)
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
		return u.applyFullUpdate(serverURL, packName, update, result)
	}

	// 3. 增量模式
	return u.applyIncremental(serverURL, packName, update, result, packState)
}

// getEnabledChannels 从包状态中提取已启用的频道名列表
func (u *Updater) getEnabledChannels(packState *model.PackState) []string {
	if packState == nil || len(packState.Channels) == 0 {
		return nil
	}
	var channels []string
	for name, ch := range packState.Channels {
		if ch.Enabled {
			channels = append(channels, name)
		}
	}
	return channels
}

// ============================================================
// 增量更新
// ============================================================

func (u *Updater) applyIncremental(serverURL, packName string, update *model.IncrementalUpdate, result *UpdateResult, packState *model.PackState) (*UpdateResult, error) {
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
		if err := u.api.DownloadFile(serverURL, packName, entry.Hash, fullPath); err != nil {
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
// 全量更新（首次部署 / 强制全量）
// ============================================================

func (u *Updater) applyFullUpdate(serverURL, packName string, update *model.IncrementalUpdate, result *UpdateResult) (*UpdateResult, error) {
	packDir := filepath.Join(u.mcDir, "packs", packName)

	// 1. 拉起整包 zip
	baseURL := strings.TrimRight(serverURL, "/")
	fullURL := fmt.Sprintf("%s/api/v1/packs/%s/download",
		baseURL, packName)

	tmpFile, err := os.CreateTemp("", "mc-pack-full-*.zip")
	if err != nil {
		return nil, fmt.Errorf("[%s] 创建临时文件失败: %w", packName, err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	logger.Info("[%s] 全量下载: %s", packName, fullURL)
	resp, err := u.api.HTTPGet(fullURL)
	if err != nil {
		return nil, fmt.Errorf("[%s] 下载全量包失败: %w", packName, err)
	}
	defer resp.Body.Close()

	out, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("[%s] 创建本地文件失败: %w", packName, err)
	}
	n, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return nil, fmt.Errorf("[%s] 下载中断: %w", packName, err)
	}
	result.DownloadBytes = n

	// 2. 确保目标目录存在
	if err := os.MkdirAll(packDir, 0755); err != nil {
		return nil, fmt.Errorf("[%s] 创建目录失败: %w", packName, err)
	}

	// 3. 解压到 packDir，自动 strip overrides/ 前缀
	reader, err := zip.OpenReader(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("[%s] 打开全量包失败: %w", packName, err)
	}
	defer reader.Close()

	extractCount := 0
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		relPath := normalizeFullPath(f.Name) // strip overrides/
		targetPath := filepath.Join(packDir, relPath)
		// 路径越界保护
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(packDir)+string(os.PathSeparator)) {
			logger.Warn("[%s] 跳过路径越界的文件: %s", packName, f.Name)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("创建目录 %s 失败: %v", relPath, err))
			continue
		}
		rc, err := f.Open()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("打开 %s 失败: %v", f.Name, err))
			continue
		}
		w, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			result.Errors = append(result.Errors, fmt.Sprintf("写入 %s 失败: %v", relPath, err))
			continue
		}
		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("写入 %s 失败: %v", relPath, err))
			continue
		}
		extractCount++
		result.Added++
	}
	result.Downloaded = extractCount

	// 4. 创建快照
	if err := u.updatePackSnapshot(packName, update.Version); err != nil {
		logger.Warn("[%s] 快照更新失败(非致命): %v", packName, err)
	}

	logger.Info("[%s] 全量更新完成: %d 个文件 -> %s", packName, extractCount, packDir)
	return result, nil
}

// normalizeFullPath 处理 modrinth 包路径
// modrinth.index.json 包根目录的 zip 会用 overrides/ 前缀
// 解压时需要 strip 这个前缀，使文件直接放到 packDir 根目录
func normalizeFullPath(zipPath string) string {
	// 去除路径中的 overrides/ 前缀
	cleaned := filepath.ToSlash(zipPath)
	if strings.HasPrefix(cleaned, "overrides/") {
		cleaned = cleaned[len("overrides/"):]
	}
	// modrinth.index.json 也属于包根，不覆盖
	return cleaned
}

// ============================================================
// 多包批量更新
// ============================================================

// UpdateAllPacks 更新所有已启用的包（P6 扩展：同步各包频道信息到本地配置）
// 返回每个包各自的更新结果
// 新增配置同步回调：用于更新本地配置中的频道信息
func (u *Updater) UpdateAllPacks(serverURL string, packs map[string]model.PackState, onChannels func(packName string, channels []model.ChannelInfo)) map[string]*UpdateResult {
	results := make(map[string]*UpdateResult)

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

	// 用 u.api 的 HTTP client 下载
	resp, err := u.api.HTTPGet(fullPackURL)
	if err != nil {
		return nil, fmt.Errorf("下载全量包失败: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(tmpPath)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return nil, err
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

// ============================================================
// MC 版本 + Loader 安装编排（Sprint 10 新增）
// ============================================================

// EnsureRequest 描述需要确保的 MC 版本环境
type EnsureRequest struct {
	MCVersion  string // 如 "1.21.1"
	Loader     string // 如 "fabric-0.16.10" 或 ""（纯原版）
	VersionDir string // versions/ 目录路径
	LibraryDir string // libraries/ 目录路径
}

// EnsureVersion 确保指定 MC 版本 + Loader 已安装
// 内部调用 sync 组件 + fabric install，对外是一条原子操作
func (u *Updater) EnsureVersion(req EnsureRequest) error {
	// 1. 确保 MC 本体已安装（client.jar + libraries）
	vm := u.newVersionMetaManager(req.MCVersion)
	meta, err := vm.Fetch(req.MCVersion)
	if err != nil {
		return fmt.Errorf("获取 %s 版本元数据失败: %w", req.MCVersion, err)
	}

	// 下载 client.jar
	jarPath := filepath.Join(req.VersionDir, req.MCVersion, fmt.Sprintf("%s.jar", req.MCVersion))
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		logger.Info("[Ensure] 下载 client.jar: %s", req.MCVersion)
		if err := os.MkdirAll(filepath.Dir(jarPath), 0755); err != nil {
			return fmt.Errorf("创建版本目录失败: %w", err)
		}
		downloaded, err := vm.DownloadClientJar(meta, filepath.Dir(filepath.Dir(jarPath)))
		if err != nil {
			return fmt.Errorf("下载 client.jar 失败: %w", err)
		}
		jarPath = downloaded
		logger.Info("[Ensure] client.jar 下载完成: %s", jarPath)
	} else {
		logger.Info("[Ensure] client.jar 已存在: %s", jarPath)
	}

	// 下载 libraries
	lm := libraryManager(u.cfgDir, req.LibraryDir)
	resolvedLibs, err := vm.ResolveLibraries(meta, req.VersionDir)
	if err != nil {
		logger.Warn("[Ensure] 解析 libraries 失败（非致命）: %v", err)
	} else if len(resolvedLibs) > 0 {
		libFiles := lm.ResolveToFiles(resolvedLibs)
		toDownload, _ := partitionLibFiles(libFiles) // 只下载不存在的
		downloaded, skipped, failed := lm.DownloadFiles(toDownload)
		logger.Info("[Ensure] Libraries: %d 下载, %d 已存在, %d 失败", downloaded, skipped, failed)
		extracted, _ := lm.ExtractNativesFromFiles(libFiles)
		logger.Info("[Ensure] Natives 解压完成: %d 文件", extracted)
	}

	// 2. 如果需要 Loader，安装
	if req.Loader != "" {
		loaderType, loaderVer := parseLoaderSpec(req.Loader)
		switch loaderType {
		case "fabric":
			if err := u.ensureFabric(req, meta, loaderVer); err != nil {
				return fmt.Errorf("安装 Fabric %s/%s 失败: %w", req.MCVersion, loaderVer, err)
			}
		case "forge", "neoforge":
			// TODO: Forge/NeoForge 支持
			logger.Warn("[Ensure] %s loader 安装尚未实现，跳过", loaderType)
		default:
			logger.Warn("[Ensure] 未知 loader 类型: %s，跳过", loaderType)
		}
	}

	logger.Info("[Ensure] MC %s%s 安装完成", req.MCVersion,
		map[bool]string{true: " + " + req.Loader, false: ""}[req.Loader != ""])
	return nil
}

// newVersionMetaManager 创建临时的 VersionMetaManager
func (u *Updater) newVersionMetaManager(version string) *VersionMetaManager {
	manifestDir := filepath.Join(u.cfgDir, ".cache", "manifest")
	versionsDir := filepath.Join(u.cfgDir, ".cache", "versions")
	mm := NewVersionManifestManager(manifestDir)
	return NewVersionMetaManager(versionsDir, mm)
}

// libraryManager 创建临时的 LibraryManager
func libraryManager(cfgDir, libraryDir string) *LibraryManager {
	nativesDir := filepath.Join(cfgDir, "versions", "_natives")
	return NewLibraryManager(libraryDir, nativesDir)
}

// parseLoaderSpec 解析 loader 规格 "fabric-0.16.10" → ("fabric", "0.16.10")
func parseLoaderSpec(spec string) (loaderType, loaderVer string) {
	idx := strings.Index(spec, "-")
	if idx < 0 {
		return spec, ""
	}
	return spec[:idx], spec[idx+1:]
}

// ensureFabric 安装 Fabric loader
func (u *Updater) ensureFabric(req EnsureRequest, meta *VersionMeta, loaderVer string) error {
	// 如果未指定 loader 版本，自动选最新
	installer := NewFabricInstaller(req.MCVersion, loaderVer, req.VersionDir, req.LibraryDir)
	installer.SetMirror(true)

	if loaderVer == "" {
		selected, err := installer.SelectLatestLoader()
		if err != nil {
			return fmt.Errorf("获取最新 Fabric loader 版本失败: %w", err)
		}
		loaderVer = selected
		logger.Info("[Ensure] 自动选择 Fabric loader: %s", loaderVer)
		// 重建 installer
		installer = NewFabricInstaller(req.MCVersion, loaderVer, req.VersionDir, req.LibraryDir)
		installer.SetMirror(true)
	}

	result, err := installer.Install()
	if err != nil {
		return fmt.Errorf("Fabric 安装失败: %w", err)
	}
	logger.Info("[Ensure] Fabric %s-%s 安装完成（%d 下载，%d 已存在）",
		req.MCVersion, result.LoaderVersion, result.Downloaded, result.Skipped)

	// 验证
	missing, _ := installer.VerifyInstallation(result.VersionID)
	if len(missing) > 0 {
		logger.Warn("[Ensure] Fabric 安装后 %d 个文件缺失: %v", len(missing), missing)
	}
	return nil
}

// partitionLibFiles 将库文件列表分为"需下载"和"已存在"
func partitionLibFiles(files []model.LibraryFile) (toDownload []model.LibraryFile, cached int) {
	for _, f := range files {
		if _, err := os.Stat(f.LocalPath); os.IsNotExist(err) {
			toDownload = append(toDownload, f)
		} else {
			cached++
		}
	}
	return
}
