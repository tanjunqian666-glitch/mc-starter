package launcher

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// 版本目录查找器
//
// 查找流程:
//   1. 尝试检测 PCL2 并读取其 LaunchFolders → 拿到 .minecraft 目录列表
//   2. 对每个 .minecraft/versions/{versionName}/ 检查 version.json 是否存在
//   3. 如果 repo 里的版本名存在于多个 .minecraft 下，选文件最新的
//   4. 如果没有 PCL2，或 PCL2 没有有效配置，回退到默认路径
// ============================================================

// VersionFindResult 单个版本的查找结果
type VersionFindResult struct {
	VersionName  string    `json:"version_name"`  // 版本标识名
	VersionDir   string    `json:"version_dir"`   // versions/{name}/ 目录
	MinecraftDir string    `json:"minecraft_dir"` // .minecraft 根目录
	Found        bool      `json:"found"`         // 是否找到
	ModTime      time.Time `json:"mod_time"`      // version.json 修改时间（用于选最新）
	FromPCL      bool      `json:"from_pcl"`      // 是否来自 PCL 配置
}

// VersionFinder 版本查找器
type VersionFinder struct {
	localCfg *model.LocalConfig
}

// NewVersionFinder 创建查找器
func NewVersionFinder(cfg *model.LocalConfig) *VersionFinder {
	return &VersionFinder{localCfg: cfg}
}

// FindManagedVersions 查找所有受管版本目录
// versionNames: 需要查找的版本名列表（来自 repo 配置）
// 返回 map[versionName]*VersionFindResult
func (f *VersionFinder) FindManagedVersions(versionNames []string) map[string]*VersionFindResult {
	if len(versionNames) == 0 {
		return nil
	}

	results := make(map[string]*VersionFindResult)
	remaining := make(map[string]bool) // 还没找到的版本名
	for _, name := range versionNames {
		remaining[name] = true
	}

	// Phase 1: 先查找 PCL2
	if f.localCfg == nil || f.localCfg.Launcher == "" || f.localCfg.Launcher == "pcl2" || f.localCfg.Launcher == "auto" {
		f.findViaPCL(remaining, results)
	}

	// Phase 2: 回退到旧扫描逻辑（适用于裸启动/无 PCL/其他启动器）
	if len(remaining) > 0 {
		f.findViaFallback(remaining, results)
	}

	return results
}

// findViaPCL 通过 PCL2 配置查找
func (f *VersionFinder) findViaPCL(remaining map[string]bool, results map[string]*VersionFindResult) {
	pclResult := FindPCL2()
	if pclResult == nil {
		logger.Debug("未检测到 PCL2，跳过 PCL 配置查找")
		return
	}

	// 读取 PCL 配置拿到 .minecraft 目录列表
	cfg, err := pclResult.ReadPCLConfig()
	if err != nil {
		logger.Debug("读取 PCL 配置失败: %v", err)
		return
	}

	if len(cfg.MinecraftDirs) == 0 {
		logger.Debug("PCL 配置中没有有效的 .minecraft 目录")
		return
	}

	logger.Info("从 PCL 配置读取到 %d 个 .minecraft 目录", len(cfg.MinecraftDirs))

	// 对每个 .minecraft，检查各个版本是否存在
	for _, mcDir := range cfg.MinecraftDirs {
		if len(remaining) == 0 {
			break
		}

		versionsDir := filepath.Join(mcDir, "versions")
		entries, err := os.ReadDir(versionsDir)
		if err != nil {
			continue
		}

		// 构建目录名 → 版本的快速查找表
		dirNames := make(map[string]bool)
		for _, e := range entries {
			if e.IsDir() {
				dirNames[e.Name()] = true
			}
		}

		for name := range remaining {
			if !dirNames[name] {
				continue
			}

			versionDir := filepath.Join(versionsDir, name)
			vj := filepath.Join(versionDir, "version.json")

			info, err := os.Stat(vj)
			if err != nil {
				continue
			}

			result := &VersionFindResult{
				VersionName:  name,
				VersionDir:   versionDir,
				MinecraftDir: mcDir,
				Found:        true,
				ModTime:      info.ModTime(),
				FromPCL:      true,
			}

			// 如果之前已找到该版本，选更新日期更晚的
			if existing, ok := results[name]; ok {
				if result.ModTime.After(existing.ModTime) {
					results[name] = result
				}
			} else {
				results[name] = result
				delete(remaining, name)
			}
		}
	}
}

// findViaFallback 回退到全量扫描
// 收集以下来源的 .minecraft 目录：
//  1. ResolveMinecraftDirs（PCL 配置 + 默认路径）
//  2. LocalConfig 中已记录的目录（GetMinecraftDir）
func (f *VersionFinder) findViaFallback(remaining map[string]bool, results map[string]*VersionFindResult) {
	var mcDirs []string
	seen := make(map[string]bool)

	// 1. ResolveMinecraftDirs（PCL + 默认路径）
	managed, raw := ResolveMinecraftDirs()
	for _, m := range managed {
		if !seen[m.Path] {
			seen[m.Path] = true
			mcDirs = append(mcDirs, m.Path)
		}
	}
	for _, d := range raw {
		if !seen[d] {
			seen[d] = true
			mcDirs = append(mcDirs, d)
		}
	}

	// 2. LocalConfig 中记录的目录（可能不在标准路径上）
	if f.localCfg != nil {
		mcDir := f.localCfg.GetMinecraftDir("")
		if mcDir != "" && !seen[mcDir] {
			seen[mcDir] = true
			mcDirs = append(mcDirs, mcDir)
		}
	}

	if len(mcDirs) == 0 {
		logger.Debug("回退扫描: 未找到任何有效的 .minecraft 目录")
		return
	}

	logger.Debug("回退扫描 %d 个 .minecraft 目录", len(mcDirs))

	for _, mcDir := range mcDirs {
		if len(remaining) == 0 {
			break
		}

		versionsDir := filepath.Join(mcDir, "versions")
		entries, err := os.ReadDir(versionsDir)
		if err != nil {
			continue
		}

		dirNames := make(map[string]bool)
		for _, e := range entries {
			if e.IsDir() {
				dirNames[e.Name()] = true
			}
		}

		for name := range remaining {
			if !dirNames[name] {
				continue
			}

			versionDir := filepath.Join(versionsDir, name)
			vj := filepath.Join(versionDir, "version.json")

			info, err := os.Stat(vj)
			if err != nil {
				continue
			}

			result := &VersionFindResult{
				VersionName:  name,
				VersionDir:   versionDir,
				MinecraftDir: mcDir,
				Found:        true,
				ModTime:      info.ModTime(),
				FromPCL:      false,
			}

			if existing, ok := results[name]; ok {
				if result.ModTime.After(existing.ModTime) {
					results[name] = result
				}
			} else {
				results[name] = result
				delete(remaining, name)
			}
		}
	}

	if len(remaining) > 0 {
		var ks []string
		for k := range remaining {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		logger.Info("以下版本未在用户电脑中找到: %v", ks)
	}
}

// FindVersionDir 查找单个版本的目录
// 返回 versionDir 路径（可能为空字符串）
func FindVersionDir(cfg *model.LocalConfig, versionName string) string {
	finder := NewVersionFinder(cfg)
	results := finder.FindManagedVersions([]string{versionName})
	if r, ok := results[versionName]; ok && r.Found {
		return r.VersionDir
	}
	return ""
}

// FindMinecraftDirs 查找用户电脑上的所有 .minecraft 目录
// 自动尝试 PCL 配置和默认路径
// 底层复用 ResolveMinecraftDirs（repo.go）的完整扫描逻辑
func FindMinecraftDirs() []string {
	managed, raw := ResolveMinecraftDirs()
	var all []string
	seen := make(map[string]bool)
	for _, m := range managed {
		if !seen[m.Path] {
			seen[m.Path] = true
			all = append(all, m.Path)
		}
	}
	for _, d := range raw {
		if !seen[d] {
			seen[d] = true
			all = append(all, d)
		}
	}
	return all
}

// VersionDirExists 检查某个版本的目录是否存在
func VersionDirExists(mcDir, versionName string) bool {
	vj := filepath.Join(mcDir, "versions", versionName, "version.json")
	_, err := os.Stat(vj)
	return err == nil
}

// FindLatestVersionDir 在多个 .minecraft 中找最新的版本
func FindLatestVersionDir(mcDirs []string, versionName string) string {
	var bestDir string
	var bestTime time.Time

	for _, mcDir := range mcDirs {
		vj := filepath.Join(mcDir, "versions", versionName, "version.json")
		info, err := os.Stat(vj)
		if err != nil {
			continue
		}

		t := info.ModTime()
		if t.After(bestTime) {
			bestTime = t
			bestDir = filepath.Join(mcDir, "versions", versionName)
		}
	}

	return bestDir
}

