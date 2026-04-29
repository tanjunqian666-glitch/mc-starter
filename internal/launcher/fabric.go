package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gege-tlph/mc-starter/internal/downloader"
	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ──────────────────────────────────────────────
// Fabric 安装器
// ──────────────────────────────────────────────
//
// 设计思路（复用 P1 的解析/下载分离模式）：
//
//  1. 通过 Fabric meta API 获取 loader 版本列表（v2/versions/loader/<mcVer>）
//  2. 获取 profile JSON（v2/versions/loader/<mcVer>/<loaderVer>/profile/json）
//  3. profile 本身就是标准 VersionMeta JSON，inheritsFrom 指向 MC 版本
//  4. 解析 profile → 下载 libraries（复用 LibraryManager.ResolveToFiles）
//  5. 将 profile JSON 写入 versions/<fabric-id>/<fabric-id>.json
//     这样 mc-starter 启动时能用标准版本加载流程找到它
//
// 参考 Fabric meta API:
//   - GET /v2/versions/loader/<mcVersion>          → loader+intermediary 列表
//   - GET /v2/versions/loader/<mcVersion>/<loader>  → loader+intermediary+launcherMeta
//   - GET /v2/versions/loader/<mcVersion>/<loader>/profile/json → VersionMeta JSON

// FabricURLs Fabric meta API 地址
var FabricURLs = struct {
	Direct string // Fabric 官方
	Mirror string // BMCLAPI 镜像
}{
	Direct: "https://meta.fabricmc.net/v2",
	Mirror: "https://bmclapi2.bangbang93.com/fabric/meta/v2",
}

// FabricLoaderEntry loader 版本条目
type FabricLoaderEntry struct {
	Loader struct {
		Separator string `json:"separator"`
		Build     int    `json:"build"`
		Maven     string `json:"maven"`
		Version   string `json:"version"`
		Stable    bool   `json:"stable"`
	} `json:"loader"`
	Intermediary struct {
		Maven   string `json:"maven"`
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"intermediary"`
	LauncherMeta struct {
		Version       int `json:"version"`
		MinJavaVerion int `json:"min_java_version,omitempty"`
		Libraries     struct {
			Client []FabricLibraryEntry `json:"client"`
			Common []FabricLibraryEntry `json:"common"`
			Server []FabricLibraryEntry `json:"server"`
		} `json:"libraries"`
		MainClass struct {
			Client string `json:"client"`
			Server string `json:"server"`
		} `json:"mainClass,omitempty"`
	} `json:"launcherMeta"`
}

// FabricLibraryEntry Fabric 元数据中的库条目（同 profile 格式）
type FabricLibraryEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	MD5  string `json:"md5,omitempty"`
	SHA1 string `json:"sha1,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// FabricInstaller Fabric 安装器
//
// 职责：
//  1. 从 meta API 获取 loader 版本信息
//  2. 生成 profile JSON（一个继承了 MC 版本的 VersionMeta）
//  3. 下载 Fabric 所需的 libraries
//  4. 将 profile 写入 versions 目录
//
// 不包含：
//   - MC 本体版本下载（调用 sync 即可）
//   - 其他 mod loader（Forge/Quilt 有各自 meta API）
type FabricInstaller struct {
	mcVersion    string  // 目标 MC 版本
	loaderVer    string  // Fabric loader 版本（空=自动选最新稳定）
	versionsDir  string  // versions/ 目录
	librariesDir string  // libraries/ 目录
	nativesDir   string  // natives/ 目录
	dl           *downloader.Downloader
	httpClient   *http.Client
	baseURL      string  // meta API 基 URL
	mirror       bool
}

// NewFabricInstaller 创建 Fabric 安装器
func NewFabricInstaller(mcVersion string, loaderVer string, versionsDir, librariesDir string) *FabricInstaller {
	return &FabricInstaller{
		mcVersion:    mcVersion,
		loaderVer:    loaderVer,
		versionsDir:  versionsDir,
		librariesDir: librariesDir,
		nativesDir:   filepath.Join(filepath.Dir(versionsDir), "versions"),
		dl:           downloader.New(),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		baseURL:      FabricURLs.Direct,
	}
}

// SetMirror 启用镜像加速
func (f *FabricInstaller) SetMirror(enabled bool) {
	f.mirror = enabled
	if enabled {
		f.baseURL = FabricURLs.Mirror
	}
}

// SetBaseURL 手动设置 API 基 URL
func (f *FabricInstaller) SetBaseURL(baseURL string) {
	f.baseURL = baseURL
}

// FabricResult 安装结果
type FabricResult struct {
	VersionID     string // 生成的版本 ID（如 "fabric-loader-0.19.2-1.20.4"）
	LoaderVersion string // Fabric loader 版本号
	Downloaded    int    // 下载的文件数
	Skipped       int    // 已存在的文件数
	LibrariesDir  string // libraries 目录路径
}

// fetchJSON GET 请求并解析 JSON
func (f *FabricInstaller) fetchJSON(requestURL string, target interface{}) error {
	resp, err := f.httpClient.Get(requestURL)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d (GET %s)", resp.StatusCode, requestURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w", err)
	}
	return nil
}

// SelectLatestLoader 获取指定 MC 版本的最新稳定 loader 版本号
func (f *FabricInstaller) SelectLatestLoader() (string, error) {
	metaURL := fmt.Sprintf("%s/versions/loader/%s", f.baseURL, f.mcVersion)
	logger.Debug("Fabric: 获取 loader 列表 %s", metaURL)

	var entries []FabricLoaderEntry
	if err := f.fetchJSON(metaURL, &entries); err != nil {
		return "", fmt.Errorf("获取 Fabric loader 列表失败: %w", err)
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("MC 版本 %s 没有可用的 Fabric loader", f.mcVersion)
	}

	// 优先选择 stable=true 的最新版
	var latestStable string
	var latestAny string
	for _, e := range entries {
		if latestAny == "" {
			latestAny = e.Loader.Version
		}
		if e.Loader.Stable {
			latestStable = e.Loader.Version
		}
	}

	if latestStable != "" {
		logger.Debug("Fabric: 选中最新稳定 loader %s", latestStable)
		return latestStable, nil
	}
	logger.Debug("Fabric: 无稳定版，使用最新版 %s", latestAny)
	return latestAny, nil
}

// Install 安装 Fabric loader
//
// 执行流程：
//  1. 确定 loader 版本（自动或指定）
//  2. 拉取 profile JSON
//  3. 写入 version JSON 到 versions/ 目录
//  4. 下载 Fabric libraries
func (f *FabricInstaller) Install() (*FabricResult, error) {
	result := &FabricResult{}

	// Step 1: 确定 loader 版本
	if f.loaderVer == "" {
		latest, err := f.SelectLatestLoader()
		if err != nil {
			return nil, fmt.Errorf("Fabric: 自动选择 loader 版本失败: %w", err)
		}
		f.loaderVer = latest
	}

	versionID := fmt.Sprintf("fabric-loader-%s-%s", f.loaderVer, f.mcVersion)
	result.VersionID = versionID
	result.LoaderVersion = f.loaderVer
	logger.Info("Fabric: 安装版本 %s (loader=%s, mc=%s)", versionID, f.loaderVer, f.mcVersion)

	// Step 2: 拉取 profile JSON
	profileURL := fmt.Sprintf("%s/versions/loader/%s/%s/profile/json",
		f.baseURL, f.mcVersion, url.PathEscape(f.loaderVer))
	logger.Debug("Fabric: 拉取 profile %s", profileURL)

	resp, err := f.httpClient.Get(profileURL)
	if err != nil {
		return nil, fmt.Errorf("Fabric: 获取 profile 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Fabric: profile HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Fabric: 读取 profile 响应失败: %w", err)
	}

	var profile VersionMeta
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("Fabric: 解析 profile 失败: %w", err)
	}

	// 验证 profile 结构
	if profile.ID == "" {
		return nil, fmt.Errorf("Fabric: profile 缺少 id 字段")
	}
	if profile.InheritsFrom == "" {
		return nil, fmt.Errorf("Fabric: profile 缺少 inheritsFrom 字段（预期指向 MC 版本 %s）", f.mcVersion)
	}
	logger.Info("Fabric: profile ID=%s, inheritsFrom=%s, mainClass=%s",
		profile.ID, profile.InheritsFrom, profile.MainClass)

	// Step 3: 写入 version JSON
	versionDir := filepath.Join(f.versionsDir, profile.ID)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return nil, fmt.Errorf("Fabric: 创建 version 目录失败: %w", err)
	}

	versionJSONPath := filepath.Join(versionDir, fmt.Sprintf("%s.json", profile.ID))
	if err := os.WriteFile(versionJSONPath, data, 0644); err != nil {
		return nil, fmt.Errorf("Fabric: 写入 version JSON 失败: %w", err)
	}
	fmt.Printf("[✓] Fabric profile: %s\n", versionJSONPath)

	// Step 4: 下载 Fabric libraries
	// profile.Libraries 中的条目没有 downloads.artifact，
	// 改用 name+url 拼 Maven URL — LibraryManager.ResolveToFiles 已支持这种格式
	// fabric 条目没有 Rules 和 natives，所以 ResolveToFiles 直接包含所有
	lm := NewLibraryManager(f.librariesDir, f.nativesDir)
	libFiles := lm.ResolveToFiles(profile.Libraries)

	logger.Info("Fabric: %d 个 libraries 需下载", len(libFiles))
	if len(libFiles) > 0 {
		now := time.Now()
		downloaded, skipped, failed := lm.DownloadFiles(libFiles)
		elapsed := time.Since(now)
		result.Downloaded = downloaded
		result.Skipped = skipped

		if failed > 0 {
			logger.Error("Fabric: %d 个 libraries 下载失败", failed)
			fmt.Printf("[!] Fabric libraries: %d 下载, %d 已存在, %d 失败 (%.1fs)\n",
				downloaded, skipped, failed, elapsed.Seconds())
		} else {
			fmt.Printf("[✓] Fabric libraries: %d 下载, %d 已存在 (%.1fs)\n",
				downloaded, skipped, elapsed.Seconds())
		}

		if failed > 0 && downloaded == 0 {
			return nil, fmt.Errorf("Fabric: 全部 libraries 下载失败，安装不完整")
		}
	} else {
		fmt.Println("[*] Fabric: 无额外 libraries 需要下载")
	}

	result.LibrariesDir = f.librariesDir
	logger.Info("Fabric: %s 安装完成", versionID)
	return result, nil
}

// VerifyInstallation 验证 Fabric 安装完整性
//
// 检查：
//  1. version JSON 是否存在
//  2. 所有 libraries 文件是否存在
//  3. 对应的 MC 版本是否已安装
func (f *FabricInstaller) VerifyInstallation(versionID string) ([]string, error) {
	var missing []string

	versionJSON := filepath.Join(f.versionsDir, versionID, fmt.Sprintf("%s.json", versionID))
	if _, err := os.Stat(versionJSON); os.IsNotExist(err) {
		missing = append(missing, versionJSON)
		return missing, nil
	}

	data, err := os.ReadFile(versionJSON)
	if err != nil {
		return missing, fmt.Errorf("读取 version JSON 失败: %w", err)
	}

	var profile VersionMeta
	if err := json.Unmarshal(data, &profile); err != nil {
		return missing, fmt.Errorf("解析 version JSON 失败: %w", err)
	}

	// 检查 libraries 文件
	lm := NewLibraryManager(f.librariesDir, f.nativesDir)
	for _, lib := range profile.Libraries {
		if !ShouldInclude(lib.Rules) {
			continue
		}
		coords := ParseMavenCoords(lib.Name)
		if coords == nil {
			continue
		}
		localPath := lm.MavenLocalPath(coords)
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			missing = append(missing, localPath)
		}
	}

	// 检查 MC 原版
	if profile.InheritsFrom != "" {
		mcVersionJSON := filepath.Join(f.versionsDir, profile.InheritsFrom, fmt.Sprintf("%s.json", profile.InheritsFrom))
		if _, err := os.Stat(mcVersionJSON); os.IsNotExist(err) {
			logger.Warn("Fabric: MC 原版 %s 未安装（需先运行 sync）", profile.InheritsFrom)
		}
	}

	return missing, nil
}

// FabricInstallOptions 安装选项
type FabricInstallOptions struct {
	MCVersion    string // MC 版本（必填）
	LoaderVer    string // Loader 版本（空=自动选择最新稳定）
	VersionsDir  string // versions/ 目录（必填）
	LibrariesDir string // libraries 目录（必填）
	Mirror       bool   // 使用镜像加速
}

// InstallFabric 一站式 Fabric 安装
func InstallFabric(opts FabricInstallOptions) (*FabricResult, error) {
	installer := NewFabricInstaller(opts.MCVersion, opts.LoaderVer, opts.VersionsDir, opts.LibrariesDir)
	if opts.Mirror {
		installer.SetMirror(true)
	}
	return installer.Install()
}

// FabricProfilePath fabric profile JSON 的本地路径
func FabricProfilePath(versionsDir, versionID string) string {
	return filepath.Join(versionsDir, versionID, fmt.Sprintf("%s.json", versionID))
}

// FabricVersionID 生成 Fabric 版本 ID
func FabricVersionID(loaderVer, mcVersion string) string {
	return fmt.Sprintf("fabric-loader-%s-%s", loaderVer, mcVersion)
}

// IsFabricVersion 判断版本 ID 是否为 Fabric
func IsFabricVersion(versionID string) bool {
	return len(versionID) > 14 && versionID[:14] == "fabric-loader-"
}

// EnsureFabricLibraryFiles 重新补下载缺失的 Fabric library 文件
func (f *FabricInstaller) EnsureFabricLibraryFiles(versionID string) error {
	missing, err := f.VerifyInstallation(versionID)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}

	logger.Info("Fabric: %d 个 library 文件缺失，尝试补下载", len(missing))
	versionJSON := filepath.Join(f.versionsDir, versionID, fmt.Sprintf("%s.json", versionID))
	data, err := os.ReadFile(versionJSON)
	if err != nil {
		return fmt.Errorf("读取 version JSON 失败: %w", err)
	}
	var profile VersionMeta
	if err := json.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("解析 version JSON 失败: %w", err)
	}

	lm := NewLibraryManager(f.librariesDir, f.nativesDir)
	libFiles := lm.ResolveToFiles(profile.Libraries)
	_, skipped, failed := lm.DownloadFiles(libFiles)
	logger.Info("Fabric: 补下载完成 — %d 跳过(已存在), %d 失败", skipped, failed)
	if failed > 0 {
		return fmt.Errorf("Fabric: %d 个 library 仍然下载失败", failed)
	}
	return nil
}
