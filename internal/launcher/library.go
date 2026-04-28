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
)

// LibraryManager Libraries 下载管理器
//
// Minecraft 的 libraries 是 version.json 中声明的 Java 依赖库。
// 每个 LibraryEntry 包含 Maven 坐标、下载信息、平台规则等。
// 处理流程:
//   1. 解析 Maven 坐标 → 下载 URL
//   2. 匹配 rules → 判断当前平台是否需要该库
//   3. 下载 artifact JAR 或 natives JAR
//   4. 对于 natives，解压提取 .dll/.so/.dylib
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

// MavenArtifact Maven 坐标解析结果
type MavenArtifact struct {
	Group     string // group
	Artifact  string // artifact ID
	Version   string // version
	Classifier string // classifier（可选，如 "natives-windows"）
	Extension string // 扩展名（默认 "jar"）
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
//   base="https://libraries.minecraft.net"
//   coords="org.lwjgl:lwjgl:3.3.1"
//   → https://libraries.minecraft.net/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar
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
// 规则逻辑:
//   - 空 rules → 总是包含
//   - rules 数组中所有条目都匹配（AND 逻辑）
//   - 每个条目的 action 为 "allow" 表示匹配时通过，"disallow" 表示匹配时排除
//   - 如果没有任意 allow 规则匹配，默认不允许
//
// Windows-only 项目简化处理: 只匹配 os.name == "windows" 的规则
func ShouldInclude(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}

	// 检查是否有任何规则匹配当前平台
	// 空 rules = 全部包含
	// 有 rules = 逐条计算 allow/disallow
	var matchedAllow bool

	for _, rule := range rules {
		if rule.OS == nil || rule.OS.Name == "" {
			// 无 OS 限制的规则：匹配所有平台
			if rule.Action == "allow" {
				matchedAllow = true
			} else if rule.Action == "disallow" {
				return false
			}
			continue
		}

		// 检查 OS 是否匹配
		osName := normalizeOS(rule.OS.Name)
		if osName == "windows" {
			if rule.Action == "allow" {
				matchedAllow = true
			} else if rule.Action == "disallow" {
				return false
			}
		}
	}

	// 如果有 allow 规则且至少有一个匹配到了，则包含
	return matchedAllow
	// 注意: 这个简化实现对于纯 Windows 场景够用。
	// 如果要支持 macOS/Linux，需要用 runtime.GOOS 做实际匹配。
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
func (m *LibraryManager) DownloadLibrary(lib LibraryEntry) (string, bool, error) {
	// 检查 rules
	if !ShouldInclude(lib.Rules) {
		return "", false, nil // 跳过，不是错误
	}

	// 确定下载 URL 和本地路径
	var (
		downloadURL string
		localPath   string
		isNative    bool
	)

	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		// Mojang 格式: 直接使用 downloads.artifact.url
		downloadURL = lib.Downloads.Artifact.URL
		coords := ParseMavenCoords(lib.Name)
		if coords != nil {
			localPath = m.MavenLocalPath(coords)
		} else {
			return "", false, fmt.Errorf("无法解析 Maven 坐标: %s", lib.Name)
		}
	} else if lib.URL != "" {
		// Fabric 格式: 通过 name+url 拼 Maven 路径
		coords := ParseMavenCoords(lib.Name)
		if coords == nil {
			return "", false, fmt.Errorf("无法解析 Fabric Maven 坐标: %s", lib.Name)
		}
		downloadURL = MavenURL(lib.URL, coords)
		localPath = m.MavenLocalPath(coords)
	} else {
		// 没有下载信息，跳过
		logger.Debug("跳过无下载信息的库: %s", lib.Name)
		return "", false, nil
	}

	// 如果已有则跳过
	if _, err := os.Stat(localPath); err == nil {
		logger.Debug("库文件已存在: %s", filepath.Base(localPath))
		return localPath, false, nil
	}

	// 下载
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", false, fmt.Errorf("创建库目录失败: %w", err)
	}

	logger.Debug("下载库: %s", filepath.Base(localPath))
	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		// Mojang 格式: 传 SHA1 做校验
		if err := m.dl.File(downloadURL, localPath, ""); err != nil {
			return "", false, fmt.Errorf("下载库失败(%s): %w", lib.Name, err)
		}
		// SHA1 校验
		if lib.Downloads.Artifact.Sha1 != "" {
			if ok, err := verifySHA1(localPath, lib.Downloads.Artifact.Sha1); err != nil {
				return "", false, fmt.Errorf("SHA1 校验失败(%s): %w", lib.Name, err)
			} else if !ok {
				os.Remove(localPath)
				return "", false, fmt.Errorf("SHA1 不匹配(%s)", lib.Name)
			}
		}
	} else {
		// Fabric 格式: 无 SHA1 信息，直接下载
		if err := m.dl.File(downloadURL, localPath, ""); err != nil {
			return "", false, fmt.Errorf("下载 Fabric 库失败(%s): %w", lib.Name, err)
		}
	}

	return localPath, isNative, nil
}

// DownloadLibraries 批量下载库文件
// 返回: 成功列表、失败列表
func (m *LibraryManager) DownloadLibraries(libraries []LibraryEntry) ([]string, []error) {
	var success []string
	var fails []error

	for _, lib := range libraries {
		path, _, err := m.DownloadLibrary(lib)
		if err != nil {
			logger.Warn("库下载失败: %v", err)
			fails = append(fails, err)
			continue
		}
		if path != "" {
			success = append(success, path)
		}
	}

	return success, fails
}

// DownloadNatives 下载并解压 natives
// Natives 是平台特定的动态库 (.dll/.so/.dylib)，需要从 JAR 中解压出来
// 处理流程:
//   1. 检查 lib.natives[windiws] 确定 classifier（如 "natives-windows"）
//   2. 从 downloads.classifiers[classifier] 获取下载信息
//   3. 下载 JAR 到临时路径
//   4. 解压 JAR 中的 .dll/.so/.dylib 到 nativesDir
//   5. 删除临时 JAR
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
	defer reader.Close()

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

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
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
// 在 Windows 上用 ";" 分隔，其他系统用 ":" （但我们 Windows only）
func BuildClasspath(libraryPaths []string, clientJar string) string {
	separator := string(filepath.ListSeparator) // Windows: ";", others: ":"
	allPaths := append(libraryPaths, clientJar)
	return strings.Join(allPaths, separator)
}

// GetNativesDir 返回 natives 目录路径
func (m *LibraryManager) GetNativesDir() string {
	return m.nativesDir
}

// SetDownloadDir 设置自定义下载目录（Fabric 等额外库）
func (m *LibraryManager) SetDownloadDir(dir string) {
	m.downloadDir = dir
}

// CurrentOS 返回当前操作系统名称（小写），匹配 rules 中的 os.name
func CurrentOS() string {
	return runtime.GOOS
}
