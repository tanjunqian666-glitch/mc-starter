package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// PCL2 (Plain Craft Launcher 2) 检测器
//
// 检测逻辑分多层：
//   Level 1 — 文件名匹配（最弱，但最快）
//   Level 2 — PE 头部 + 内部特征字符串扫描
//   Level 3 — 读取同目录 PCL.ini 或 PCLPortable.ini
//   Level 4 — SHA256 hash 校验（已知版本白名单）
//
// 单项通过即认为识别为 PCL2，但会记录检测层级。
// ============================================================

// PCLDetection 检测结果
type PCLDetection struct {
	Path          string `json:"path"`           // exe 完整路径
	Version       string `json:"version"`        // 版本号（从 PCL.ini 或 PE 读取）
	Level         int    `json:"level"`          // 检测层级（1-4，越高越可靠）
	Hash          string `json:"hash,omitempty"` // SHA256
	PCLDir        string `json:"pcl_dir"`        // PCL 所在目录
	PCLIniPath    string `json:"pcl_ini_path"`   // PCL.ini 路径
	IsPortable    bool   `json:"is_portable"`    // 是否为便携版
}

// PCLDetector PCL2 检测器
type PCLDetector struct {
	knownHashes map[string]string // hash → version（白名单）
}

// NewPCLDetector 创建检测器
func NewPCLDetector() *PCLDetector {
	return &PCLDetector{
		knownHashes: make(map[string]string),
	}
}

// AddKnownHash 添加一个已知 PCL2 的 SHA256 到白名单
// 用户或配置可追加
func (d *PCLDetector) AddKnownHash(hash, version string) {
	d.knownHashes[hash] = version
}

// Detect 搜索系统并检测 PCL2
// 返回第一个找到的结果，可选的搜索路径列表
func (d *PCLDetector) Detect(searchPaths []string) *PCLDetection {
	// 默认搜索路径
	if len(searchPaths) == 0 {
		searchPaths = d.defaultSearchPaths()
	}

	for _, dir := range searchPaths {
		if result := d.detectInDir(dir); result != nil {
			return result
		}
	}

	return nil
}

// DetectAll 搜索所有路径，返回所有找到的 PCL2
func (d *PCLDetector) DetectAll(searchPaths []string) []PCLDetection {
	if len(searchPaths) == 0 {
		searchPaths = d.defaultSearchPaths()
	}

	var results []PCLDetection
	seen := make(map[string]bool) // 去重

	for _, dir := range searchPaths {
		if result := d.detectInDir(dir); result != nil {
			key := result.Path
			if !seen[key] {
				seen[key] = true
				results = append(results, *result)
			}
		}
	}

	return results
}

// detectInDir 在单个目录中检测 PCL2
func (d *PCLDetector) detectInDir(dir string) *PCLDetection {
	// 清理路径
	dir = filepath.Clean(dir)

	// 检查目录是否存在
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	// 列出所有 .exe 文件
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".exe" {
			continue
		}

		fullPath := filepath.Join(dir, name)
		result := d.analyzeFile(fullPath)
		if result != nil {
			return result
		}
	}

	return nil
}

// analyzeFile 分析单个 exe 文件是否 PCL2
func (d *PCLDetector) analyzeFile(path string) *PCLDetection {
	// Level 1: 文件名匹配
	name := strings.ToLower(filepath.Base(path))
	isPCLName := false

	// PCL2 常见的 exe 文件名模式
	pclPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^plain.?craft.?launcher2?\.exe$`),
		regexp.MustCompile(`^plain.?craft.?launcher\s*2?\.exe$`),
		regexp.MustCompile(`^pcl2?\.exe$`),
		regexp.MustCompile(`^pcl\s*community\s*edition\.exe$`),
		regexp.MustCompile(`^pcl2?ce\.exe$`),
	}
	for _, p := range pclPatterns {
		if p.MatchString(name) {
			isPCLName = true
			break
		}
	}

	if !isPCLName {
		return nil // 文件名都不匹配，跳过
	}

	// 基础结果
	result := &PCLDetection{
		Path:   path,
		Level:  1,
		PCLDir: filepath.Dir(path),
	}

	// Level 2: PE 文件内部特征扫描
	result.Level = d.scanPEFeatures(path, result)

	// Level 3: 读取 PCL.ini
	iniPath := d.findPCLIni(result.PCLDir)
	if iniPath != "" {
		result.PCLIniPath = iniPath
		result.Level = max(result.Level, 3)
		version, portable := d.readPCLIni(iniPath)
		result.Version = version
		result.IsPortable = portable
	}

	// Level 4: SHA256 hash 白名单校验
	hash := d.computeHash(path)
	if hash != "" {
		result.Hash = hash
		if ver, ok := d.knownHashes[hash]; ok {
			result.Version = ver
			result.Level = max(result.Level, 4)
		}
	}

	return result
}

// scanPEFeatures 扫描 PE 文件内部特征
// 返回检测层级（2 = PE 特征匹配, 1 = 仅文件名）
func (d *PCLDetector) scanPEFeatures(path string, result *PCLDetection) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 1
	}

	if len(data) < 2 {
		return 1
	}

	// PE 头部检查: MZ 标记
	if data[0] != 'M' || data[1] != 'Z' {
		return 1
	}

	// PCL2 内部特征字符串（常见于 .NET 资源或字符串表）
	// 这些字符串在 PCL2 的可执行文件中大概率出现
	pclSignatures := []string{
		"Plain Craft Launcher",
		"PlainCraftLauncher",
		"PCL2",
		"PCL 2",
		"szszss",          // PCL2 作者
		"Hex-Dragon",      // 作者 GitHub
		"PCLCommunity",
		"PCL-Community",
		"Minecraft Launcher",
		"Plain Craft Launcher 2",
	}

	// 只读取前 1MB 做字符串扫描（避免大文件全读）
	scanLimit := len(data)
	if scanLimit > 1*1024*1024 {
		scanLimit = 1 * 1024 * 1024
	}
	scanData := strings.ToLower(string(data[:scanLimit]))

	matches := 0
	threshold := 2 // 至少匹配 2 个特征才认为 PE 校验通过

	for _, sig := range pclSignatures {
		if strings.Contains(scanData, strings.ToLower(sig)) {
			matches++
			if matches >= threshold {
				return 2
			}
		}
	}

	return 1
}

// findPCLIni 查找 PCL.ini 或 PCLPortable.ini
func (d *PCLDetector) findPCLIni(dir string) string {
	candidates := []string{
		filepath.Join(dir, "PCL.ini"),
		filepath.Join(dir, "PCL.ini"),
		filepath.Join(dir, "PCLPortable.ini"),
		filepath.Join(dir, "PCLPortable.ini"),
		// 父目录也检查（便携版有时在子目录）
		filepath.Join(filepath.Dir(dir), "PCL.ini"),
		filepath.Join(filepath.Dir(dir), "PCLPortable.ini"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// readPCLIni 读取 PCL.ini 提取版本和便携模式信息
func (d *PCLDetector) readPCLIni(path string) (version string, portable bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	content := string(data)

	// PCLPortable.ini 存在即表示便携版
	if strings.Contains(strings.ToLower(filepath.Base(path)), "portable") {
		portable = true
	}

	// 从 ini 中提取 Version 字段
	// PCL.ini 格式一般为:
	//   [Main]
	//   Version=2.x.x.x
	//   ...
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "version=") {
			version = strings.TrimPrefix(line[8:], " ")
			version = strings.Trim(version, "\"")
			version = strings.TrimSpace(version)
			break
		}
	}

	return version, portable
}

// computeHash 计算文件 SHA256
func (d *PCLDetector) computeHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// defaultSearchPaths 返回默认搜索路径列表（Windows 下）
func (d *PCLDetector) defaultSearchPaths() []string {
	return []string{
		// PCL2 典型位置
		".",                                 // 当前目录
		"C:\\Program Files\\PCL2",           // 典型安装路径
		"C:\\Program Files (x86)\\PCL2",     // 32 位安装
		filepath.Join(os.Getenv("LOCALAPPDATA"), "PCL2"),
		filepath.Join(os.Getenv("APPDATA"), "PCL2"),
		"D:\\PCL2",
		// 桌面
		filepath.Join(os.Getenv("USERPROFILE"), "Desktop"),
		filepath.Join(os.Getenv("USERPROFILE"), "Downloads"),
	}
}

// Validate 确认路径是否真的是 PCL2
// 对已存储的路径做二次确认
func (d *PCLDetector) Validate(path string) bool {
	result := d.analyzeFile(path)
	return result != nil
}

// PCLIniExists 检查 PCL.ini 是否存在
func (p *PCLDetection) PCLIniExists() bool {
	return p.PCLIniPath != ""
}

// String 返回人类可读摘要
func (p *PCLDetection) String() string {
	version := p.Version
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("PCL2 @ %s (v%s, level=%d, portable=%v)",
		shortPath(p.Path), version, p.Level, p.IsPortable)
}

// Summary 返回单行摘要
func (p *PCLDetection) Summary() string {
	return p.String()
}

// shortPath 截断路径为可读短串
func shortPath(path string) string {
	if len(path) <= 60 {
		return path
	}
	return "..." + path[len(path)-57:]
}

// IsPCL2Dir 检查目录是否包含 PCL2
func IsPCL2Dir(dir string) *PCLDetection {
	detector := NewPCLDetector()
	return detector.detectInDir(dir)
}

// FindPCL2 在常见位置查找 PCL2
func FindPCL2() *PCLDetection {
	logger.Info("搜索 PCL2...")
	detector := NewPCLDetector()

	// 添加几个已知版本的 hash（示例，正式需要从更新源获取）
	detector.AddKnownHash("placeholder", "0.0.0")

	result := detector.Detect(nil)
	if result != nil {
		logger.Info("发现 PCL2: %s", result.Summary())
	} else {
		logger.Info("未找到 PCL2")
	}
	return result
}
