package launcher

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/downloader"
	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// LibraryManager Libraries 下载管理器
//
// Minecraft 的 libraries 是 version.json 中声明的 Java 依赖库。
// 每个 LibraryEntry 包含 Maven 坐标、下载信息、平台规则等。
// 处理流程:
//  1. 解析 Maven 坐标 → 下载 URL
//  2. 匹配 rules → 判断当前平台是否需要该库
//  3. 下载 artifact JAR 或 natives JAR
//  4. 对于 natives，解压提取 .dll/.so/.dylib
//
// Maven 坐标格式: "group:artifact:version"（有时带 classifier）
// 下载 URL: {maven_url}/{group_path}/{artifact}/{version}/{artifact}-{version}(-classifier).jar
type LibraryManager struct {
	libraryDir  string // libraries 存放目录
	nativesDir  string // natives 解压目录
	downloadDir string // 自定义下载目录（用于 Fabric 等 Maven 仓库）
	dl          *downloader.Downloader
}

// NewLibraryManager 创建 Libraries 管理器
// libraryDir: libraries/ 存放目录
// nativesDir: natives/ 解压目录（会传入 ${natives_directory}）
func NewLibraryManager(libraryDir, nativesDir string) *LibraryManager {
	return &LibraryManager{
		libraryDir: libraryDir,
		nativesDir: nativesDir,
		dl:         downloader.New(),
	}
}

// ResolveToFiles 解析 LibraryEntry 列表为统一的 LibraryFile 列表
// 参考 PCL 的 McLibListGet：解析（rules 过滤 + 路径转换）→ 下载（批量处理）
// 返回结果已通过 rules 过滤，包含普通库和 natives 条目（natives 以 IsNative=true 标记）。
func (m *LibraryManager) ResolveToFiles(libraries []LibraryEntry) []model.LibraryFile {
	var files []model.LibraryFile

	for _, lib := range libraries {
		if !ShouldInclude(lib.Rules) {
			continue
		}

		coords := ParseMavenCoords(lib.Name)
		if coords == nil {
			logger.Debug("跳过无法解析的库: %s", lib.Name)
			continue
		}

		// 处理主 artifact
		if lib.Downloads != nil && lib.Downloads.Artifact != nil {
			files = append(files, model.LibraryFile{
				LocalPath:    m.MavenLocalPath(coords),
				URL:          lib.Downloads.Artifact.URL,
				SHA1:         lib.Downloads.Artifact.Sha1,
				Size:         lib.Downloads.Artifact.Size,
				IsNative:     false,
				OriginalName: lib.Name,
			})
		} else if lib.URL != "" {
			// Fabric 格式：无 downloads.artifact，用 name+url 拼
			dlURL := MavenURL(lib.URL, coords)
			files = append(files, model.LibraryFile{
				LocalPath:    m.MavenLocalPath(coords),
				URL:          dlURL,
				IsNative:     false,
				OriginalName: lib.Name,
			})
		} else {
			logger.Debug("跳过无下载信息的库: %s", lib.Name)
		}

		// 处理 natives
		if lib.Natives != nil {
			nativeFiles := m.resolveNatives(lib, coords)
			files = append(files, nativeFiles...)
		}
	}

	return files
}

// resolveNatives 解析单个 LibraryEntry 中的 natives 条目
func (m *LibraryManager) resolveNatives(lib LibraryEntry, coords *MavenArtifact) []model.LibraryFile {
	if lib.Natives == nil {
		return nil
	}

	classifier, ok := lib.Natives["windows"]
	if !ok {
		return nil
	}

	if lib.Downloads == nil || lib.Downloads.Classifiers == nil {
		return nil
	}

	entry, ok := lib.Downloads.Classifiers[classifier]
	if !ok {
		// 模糊匹配 fallback
		for c := range lib.Downloads.Classifiers {
			if strings.Contains(c, "windows") || strings.Contains(c, "natives") {
				entry = lib.Downloads.Classifiers[c]
				classifier = c
				ok = true
				break
			}
		}
		if !ok {
			return nil
		}
	}

	// natives 的本地路径 = artifacts 路径 + classifier
	nativeCoords := *coords
	nativeCoords.Classifier = classifier
	nativePath := m.MavenLocalPath(&nativeCoords)

	var dlURL string
	if entry.URL != "" {
		dlURL = entry.URL
	} else {
		dlURL = MavenURL("https://libraries.minecraft.net", &nativeCoords)
	}

	return []model.LibraryFile{{
		LocalPath:    nativePath,
		URL:          dlURL,
		SHA1:         entry.Sha1,
		Size:         entry.Size,
		IsNative:     true,
		OriginalName: lib.Name,
	}}
}

// MavenArtifact Maven 坐标解析结果
type MavenArtifact struct {
	Group      string // group
	Artifact   string // artifact ID
	Version    string // version
	Classifier string // classifier（可选，如 "natives-windows"）
	Extension  string // 扩展名（默认 "jar"）
}

// ParseMavenCoords 解析 Maven 坐标字符串
// 格式: "group:artifact:version" 或 "group:artifact:version:classifier"
// 例: "org.lwjgl:lwjgl:3.3.1" 或 "org.lwjgl:lwjgl-platform:3.3.1:natives-windows"
func ParseMavenCoords(name string) *MavenArtifact {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return nil
	}

	result := &MavenArtifact{
		Group:     parts[0],
		Artifact:  parts[1],
		Version:   parts[2],
		Extension: "jar",
	}

	if len(parts) >= 4 {
		result.Classifier = parts[3]
	}

	return result
}

// MavenURL 从 Maven 坐标和仓库基 URL 生成下载地址
// 路径规则: {base_url}/{group_path}/{artifact}/{version}/{artifact}-{version}(-{classifier}).{ext}
// 例:
//
//	base="https://libraries.minecraft.net"
//	coords="org.lwjgl:lwjgl:3.3.1"
//	→ https://libraries.minecraft.net/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar
func MavenURL(baseURL string, coords *MavenArtifact) string {
	groupPath := strings.ReplaceAll(coords.Group, ".", "/")
	ext := coords.Extension
	if ext == "" {
		ext = "jar"
	}
	filename := fmt.Sprintf("%s-%s.%s", coords.Artifact, coords.Version, ext)
	if coords.Classifier != "" {
		filename = fmt.Sprintf("%s-%s-%s.%s", coords.Artifact, coords.Version, coords.Classifier, ext)
	}

	return fmt.Sprintf("%s/%s/%s/%s/%s",
		strings.TrimRight(baseURL, "/"),
		groupPath,
		coords.Artifact,
		coords.Version,
		filename,
	)
}

// MavenLocalPath 从 Maven 坐标生成本地存储路径
// 镜像: {libraryDir}/{group_path}/{artifact}/{version}/{filename}
func (m *LibraryManager) MavenLocalPath(coords *MavenArtifact) string {
	groupPath := strings.ReplaceAll(coords.Group, ".", "/")
	ext := coords.Extension
	if ext == "" {
		ext = "jar"
	}
	var filename string
	if coords.Classifier != "" {
		filename = fmt.Sprintf("%s-%s-%s.%s", coords.Artifact, coords.Version, coords.Classifier, ext)
	} else {
		filename = fmt.Sprintf("%s-%s.%s", coords.Artifact, coords.Version, ext)
	}

	return filepath.Join(m.libraryDir, groupPath, coords.Artifact, coords.Version, filename)
}

// ShouldInclude 根据 rules 判断当前平台是否应包含该库
//
// 规则逻辑（参考 PCL McJsonRuleCheck）：
//   - 空 rules → 总是包含
//   - 存在 allow 规则 → 至少一个 allow 匹配才算通过
//   - 存在 disallow 规则 → 任一 disallow 匹配就拒绝（优先于 allow）
//   - 不存在任何 allow 规则但有 disallow → 不匹配 disallow 的都通过
//
// 匹配维度：
//   - os.name: "windows" / "osx" / "linux"
//   - os.version: 正则匹配（如 "10\\.0\\..*"）
//   - os.arch: "x86" = 32位，空/其他 = 64位
//   - features.is_demo_user: true = 仅 Demo 用户匹配，false 或 nil = 跳过
func ShouldInclude(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}

	var (
		hasAllow       bool
		matchedAllow   bool
		matchedDisallow bool
	)

	for _, rule := range rules {
		matched := ruleMatches(rule)

		if rule.Action == "allow" {
			hasAllow = true
			if matched {
				matchedAllow = true
			}
		} else if rule.Action == "disallow" {
			if matched {
				matchedDisallow = true
			}
		}
	}

	// disallow 优先：任一匹配就拒绝
	if matchedDisallow {
		return false
	}

	// 有 allow 规则：至少一个匹配
	if hasAllow {
		return matchedAllow
	}

	// 只有 disallow 且未匹配 → 允许
	return true
}

// ruleMatches 判断单条规则在当前平台是否匹配
func ruleMatches(rule Rule) bool {
	// 无任何条件：表示"无条件匹配"
	if rule.OS == nil && rule.Features == nil {
		return true
	}

	// OS 条件
	if rule.OS != nil {
		if !matchOS(rule.OS) {
			return false
		}
	}

	// Features 条件
	if rule.Features != nil {
		if !matchFeatures(rule.Features) {
			return false
		}
	}

	return true
}

// matchOS 判断 OS 规则是否匹配当前平台
func matchOS(rule *OSRule) bool {
	// 名称匹配
	if rule.Name != "" {
		osName := currentOSName()
		target := normalizeOS(rule.Name)
		if osName != target {
			return false
		}
	}

	// 版本正则匹配
	if rule.Version != "" {
	}

	// 架构匹配
	if rule.Arch != "" {
		if !archMatches(rule.Arch) {
			return false
		}
	}

	return true
}

// matchFeatures 判断 features 规则是否匹配
func matchFeatures(features *RuleFeatures) bool {
	if features == nil {
		return true
	}
	if features.IsDemoUser != nil && *features.IsDemoUser {
		return false // 我们不是 Demo 用户，所以要求 is_demo_user=true 时不匹配
	}
	return true
}

// versionMatches 匹配 OS 版本正则
// 现阶段简化处理：在 Windows 上始终返回 true
func versionMatches(pattern string) bool {
	_ = pattern
	return true
}

// archMatches 匹配架构条件
// "x86" = 要求是 32 位系统
func archMatches(arch string) bool {
	if arch == "x86" {
		return false // 本项目是 64 位
	}
	return true
}

// currentOSName 返回当前操作系统名称（小写）
func currentOSName() string {
	return runtime.GOOS
}

// normalizeOS 标准化 OS 名称
func normalizeOS(name string) string {
	switch strings.ToLower(name) {
	case "windows", "win":
		return "windows"
	case "osx", "mac", "macos":
		return "osx"
	case "linux":
		return "linux"
	default:
		return name
	}
}

// DownloadLibrary 下载单个库文件（根据 rules 判断是否需要）
// 返回: 下载到的本地路径，是否需要本机处理，错误
// Mojang 格式的 library: 通过 Downloads.Artifact 直接下载
// Fabric 格式的 library: 通过 name+url Maven 下载
// ResolveLibrary 将单个 LibraryEntry 解析为 LibraryFile（不下载）
//
// 职责分离（参考 PCL McLibToken）：
//   - ResolveLibrary: 解析 rules + Maven 坐标 + 下载信息 → LibraryFile
//   - 下载：由调用方通过 Downloader 或批量流程处理
//
// 返回解析后的文件列表（可能含 natives）
func (m *LibraryManager) ResolveLibrary(lib LibraryEntry) []model.LibraryFile {
	if !ShouldInclude(lib.Rules) {
		return nil
	}

	coords := ParseMavenCoords(lib.Name)
	if coords == nil {
		logger.Debug("跳过无法解析的库: %s", lib.Name)
		return nil
	}

	var files []model.LibraryFile

	// 主 artifact
	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		files = append(files, model.LibraryFile{
			LocalPath:    m.MavenLocalPath(coords),
			URL:          lib.Downloads.Artifact.URL,
			SHA1:         lib.Downloads.Artifact.Sha1,
			Size:         lib.Downloads.Artifact.Size,
			IsNative:     false,
			OriginalName: lib.Name,
		})
	} else if lib.URL != "" {
		dlURL := MavenURL(lib.URL, coords)
		files = append(files, model.LibraryFile{
			LocalPath:    m.MavenLocalPath(coords),
			URL:          dlURL,
			IsNative:     false,
			OriginalName: lib.Name,
		})
	}

	// natives
	if lib.Natives != nil {
		nativeFiles := m.resolveNatives(lib, coords)
		files = append(files, nativeFiles...)
	}

	return files
}

// DownloadLibraries 批量下载库文件
// 使用两阶段流程：ResolveLibrary → DownloadFiles
// 返回: 成功列表、失败列表
func (m *LibraryManager) DownloadLibraries(libraries []LibraryEntry) ([]string, []error) {
	// 阶段1：解析所有 libraries
	var allFiles []model.LibraryFile
	for _, lib := range libraries {
		files := m.ResolveLibrary(lib)
		allFiles = append(allFiles, files...)
	}

	// 阶段2：批量下载
	var success []string
	var fails []error
	for _, f := range allFiles {
		if f.SHA1 != "" {
			if ok, _ := verifySHA1(f.LocalPath, f.SHA1); ok {
				success = append(success, f.LocalPath)
				continue
			}
		} else if _, err := os.Stat(f.LocalPath); err == nil {
			success = append(success, f.LocalPath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(f.LocalPath), 0755); err != nil {
			fails = append(fails, fmt.Errorf("创建目录失败: %s (%w)", f.LocalPath, err))
			continue
		}
		if err := m.dl.File(f.URL, f.LocalPath, ""); err != nil {
			fails = append(fails, fmt.Errorf("下载失败: %s (%w)", f.OriginalName, err))
			continue
		}
		if f.SHA1 != "" {
			if ok, _ := verifySHA1(f.LocalPath, f.SHA1); !ok {
				os.Remove(f.LocalPath)
				fails = append(fails, fmt.Errorf("SHA1 校验失败: %s", f.OriginalName))
				continue
			}
		}
		success = append(success, f.LocalPath)
	}

	return success, fails
}

// DownloadNatives 下载并解压 natives
// Natives 是平台特定的动态库 (.dll/.so/.dylib)，需要从 JAR 中解压出来
// 处理流程:
//  1. 检查 lib.natives[windiws] 确定 classifier（如 "natives-windows"）
//  2. 从 downloads.classifiers[classifier] 获取下载信息
//  3. 下载 JAR 到临时路径
//  4. 解压 JAR 中的 .dll/.so/.dylib 到 nativesDir
//  5. 删除临时 JAR
func (m *LibraryManager) DownloadNatives(lib LibraryEntry) error {
	if lib.Natives == nil {
		return nil
	}

	// Windows-only: 只处理 windows natives
	nativeClassifier, ok := lib.Natives["windows"]
	if !ok {
		return nil
	}

	// 同时处理 "osx" 和 "linux" 的 fallback 也兼容
	// 但 Windows-only 项目只取 windows
	_ = nativeClassifier

	// 根据 Mojang 格式和 Fabric 格式不同方式获取 classifier
	// Mojang 格式: lib.Natives["windows"] → "natives-windows"
	// Fabric 格式: 无 Natives 字段，通过 classifiers 直接判断
	classifier := lib.Natives["windows"]

	if lib.Downloads == nil || lib.Downloads.Classifiers == nil {
		return nil
	}

	entry, ok := lib.Downloads.Classifiers[classifier]
	if !ok {
		// 尝试找其他可能的名字
		for c := range lib.Downloads.Classifiers {
			if strings.Contains(c, "windows") || strings.Contains(c, "natives") {
				entry = lib.Downloads.Classifiers[c]
				classifier = c
				ok = true
				break
			}
		}
		if !ok {
			logger.Debug("未找到 Windows natives: %s", lib.Name)
			return nil
		}
	}

	// 生成临时下载路径
	coords := ParseMavenCoords(lib.Name)
	if coords == nil {
		return fmt.Errorf("无法解析 natives Maven 坐标: %s", lib.Name)
	}
	coords.Classifier = classifier
	tmpJar := m.MavenLocalPath(coords) + ".tmp"

	// 检查是否已解压
	extractedMarker := m.MavenLocalPath(coords) + ".extracted"
	if _, err := os.Stat(extractedMarker); err == nil {
		logger.Debug("natives 已解压: %s", lib.Name)
		return nil
	}

	// 下载 natives JAR
	var downloadURL string
	if entry.URL != "" {
		downloadURL = entry.URL
	} else {
		downloadURL = MavenURL("https://libraries.minecraft.net", coords)
	}

	logger.Debug("下载 natives: %s", filepath.Base(tmpJar))
	if err := m.dl.File(downloadURL, tmpJar, ""); err != nil {
		return fmt.Errorf("下载 natives JAR 失败(%s): %w", lib.Name, err)
	}

	// 解压 natives
	if err := os.MkdirAll(m.nativesDir, 0755); err != nil {
		os.Remove(tmpJar)
		return fmt.Errorf("创建 natives 目录失败: %w", err)
	}

	if err := extractNatives(tmpJar, m.nativesDir); err != nil {
		os.Remove(tmpJar)
		return fmt.Errorf("解压 natives 失败(%s): %w", lib.Name, err)
	}

	// 标记已解压
	if err := os.WriteFile(extractedMarker, []byte{}, 0644); err != nil {
		logger.Warn("写入 natives 标记失败: %v", err)
	}

	// 清理临时 JAR 文件
	os.Remove(tmpJar)

	logger.Debug("natives 解压完成: %s → %s", lib.Name, m.nativesDir)
	return nil
}

// extractNatives 从 JAR 文件中提取 .dll/.so/.dylib 文件
// JAR 本质上是 ZIP 格式，遍历其中的条目找到平台动态库
func extractNatives(jarPath, destDir string) error {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("打开 JAR 失败: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Warn("关闭 JAR ZIP reader 失败: %v", err)
		}
	}()

	var extracted int
	for _, f := range reader.File {
		// 只提取动态库文件和 META-INF（签名用）的排除在 extractNativeLib 中处理
		// 实际只提取扩展名为 dll/so/dylib 的文件
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext != ".dll" && ext != ".so" && ext != ".dylib" && ext != ".jnilib" {
			continue
		}

		// 跳过 META-INF 目录
		if strings.HasPrefix(f.Name, "META-INF/") || strings.HasPrefix(f.Name, "META-INF\\") {
			continue
		}

		destPath := filepath.Join(destDir, filepath.Base(f.Name))

		// 避免路径穿越
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)) {
			continue
		}

		if err := extractZipFile(f, destPath); err != nil {
			return fmt.Errorf("解压 native 文件失败(%s): %w", f.Name, err)
		}
		extracted++
	}

	if extracted == 0 {
		logger.Debug("JAR 中未找到 native 文件: %s", jarPath)
	}

	return nil
}

// extractZipFile 从 ZIP 条目中提取单个文件
func extractZipFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if dirErr := os.MkdirAll(filepath.Dir(destPath), 0755); dirErr != nil {
		return err
	}

	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

// BuildClasspath 构建 classpath 字符串
// classpath 是启动 Java 时的 -cp 参数，包含:
//   - 所有 libraries 的 JAR 路径
//   - client.jar 的路径
//
// 在 Windows 上用 ";" 分隔，其他系统用 ":" （但我们 Windows only）
func BuildClasspath(libraryPaths []string, clientJar string) string {
	separator := string(filepath.ListSeparator) // Windows: ";", others: ":"
	allPaths := append(libraryPaths, clientJar)
	return strings.Join(allPaths, separator)
}

// DownloadFiles 批量下载 LibraryFile 列表
// 返回：下载成功数、失败数的统计数据
// 会跳过已存在且 SHA1 匹配的文件
func (m *LibraryManager) DownloadFiles(files []model.LibraryFile) (downloaded, skipped, failed int) {
	for _, f := range files {
		// 已存在且 SHA1 匹配则跳过
		if f.SHA1 != "" {
			if ok, _ := verifySHA1(f.LocalPath, f.SHA1); ok {
				skipped++
				continue
			}
		} else if _, err := os.Stat(f.LocalPath); err == nil {
			skipped++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(f.LocalPath), 0755); err != nil {
			logger.Warn("创建目录失败: %s (%v)", filepath.Dir(f.LocalPath), err)
			failed++
			continue
		}

		if err := m.dl.File(f.URL, f.LocalPath, ""); err != nil {
			logger.Warn("下载失败: %s (%v)", f.OriginalName, err)
			failed++
			continue
		}

		// SHA1 校验
		if f.SHA1 != "" {
			if ok, _ := verifySHA1(f.LocalPath, f.SHA1); !ok {
				os.Remove(f.LocalPath)
				logger.Warn("SHA1 校验失败: %s", f.OriginalName)
				failed++
				continue
			}
		}

		downloaded++
	}
	return
}

// ExtractNativesFromFiles 从 LibraryFile 列表中提取 natives 并解压
// 只处理 IsNative=true 且本地 JAR 存在的条目
func (m *LibraryManager) ExtractNativesFromFiles(files []model.LibraryFile) (extracted int, errs []error) {
	for _, f := range files {
		if !f.IsNative {
			continue
		}

		// 检查标记
		extractedMarker := f.LocalPath + ".extracted"
		if _, err := os.Stat(extractedMarker); err == nil {
			continue
		}

		if err := os.MkdirAll(m.nativesDir, 0755); err != nil {
			errs = append(errs, fmt.Errorf("创建 natives 目录失败: %w", err))
			continue
		}

		if err := extractNatives(f.LocalPath, m.nativesDir); err != nil {
			errs = append(errs, fmt.Errorf("解压 natives 失败(%s): %w", f.OriginalName, err))
			continue
		}

		// 标记已解压
		os.WriteFile(extractedMarker, []byte{}, 0644)
		extracted++
	}
	return
}

// GetNativesDir 返回 natives 目录路径
func (m *LibraryManager) GetNativesDir() string {
	return m.nativesDir
}

// SetDownloadDir 设置自定义下载目录（Fabric 等额外库）
func (m *LibraryManager) SetDownloadDir(dir string) {
	m.downloadDir = dir
}
