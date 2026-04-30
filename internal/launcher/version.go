package launcher

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gege-tlph/mc-starter/internal/downloader"
	"github.com/gege-tlph/mc-starter/internal/logger"
)

// Downloads 版本下载信息
type Downloads struct {
	Client    *DownloadEntry `json:"client,omitempty"`
	ClientMap *DownloadEntry `json:"client_mappings,omitempty"`
	Server    *DownloadEntry `json:"server,omitempty"`
	ServerMap *DownloadEntry `json:"server_mappings,omitempty"`
}

// DownloadEntry 单个下载项
type DownloadEntry struct {
	Sha1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Path string `json:"path,omitempty"`
}

// VersionMeta 版本元数据（version.json 的核心字段）
type VersionMeta struct {
	ID                 string         `json:"id"`
	Type               string         `json:"type"`
	MainClass          string         `json:"mainClass"`
	MinimumLauncherVer int            `json:"minimumLauncherVersion"`
	Assets             string         `json:"assets"`
	AssetIndex         *AssetIndexRef `json:"assetIndex"`
	Downloads          *Downloads     `json:"downloads"`
	Libraries          []LibraryEntry `json:"libraries"`
	MinecraftArguments string         `json:"minecraftArguments,omitempty"`
	Arguments          *Arguments     `json:"arguments,omitempty"`
	InheritsFrom       string         `json:"inheritsFrom,omitempty"`
	ReleaseTime        string         `json:"releaseTime,omitempty"`
	JavaVersion        *JavaVersion   `json:"javaVersion,omitempty"`
	Logging            *LoggingConfig `json:"logging,omitempty"`
}

// JavaVersion 描述所需 Java 版本
type JavaVersion struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Client *LoggingClient `json:"client,omitempty"`
}

// LoggingClient 客户端日志配置
type LoggingClient struct {
	Argument string            `json:"argument"`
	File     *LoggingFileEntry `json:"file,omitempty"`
	Type     string            `json:"type"`
}

// LoggingFileEntry 日志文件描述
type LoggingFileEntry struct {
	ID   string `json:"id"`
	Sha1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// AssetIndexRef 资源索引引用
// 每个 MC 版本有一个 asset index，描述该版本所有资源文件（音效/纹理/UI）的 hash 和路径
// 详见: asset.go 中的 AssetManager
type AssetIndexRef struct {
	ID        string `json:"id"`        // 索引 ID，通常是小版本号如 "13", "30"
	Sha1      string `json:"sha1"`      // 索引 JSON 文件本身的 SHA1
	Size      int64  `json:"size"`      // 索引 JSON 文件大小
	TotalSize int64  `json:"totalSize"` // 索引中所有 Asset 文件的总大小
	URL       string `json:"url"`       // 索引 JSON 的下载地址
}

// LibraryEntry 库文件条目
// Minecraft 依赖大量第三方 Java 库，通过 version.json 的 libraries 数组描述。
// 每个条目可以包含:
//   - name: Maven 坐标 (格式: "group:artifact:version")
//   - downloads.artifact: 主 JAR 文件下载信息（含 SHA1 + URL）
//   - downloads.classifiers: 平台特定文件（如 natives-windows.jar）
//   - rules: 平台匹配规则（如仅在 Windows 或 Linux 下使用该库）
//   - natives: 声明哪些 classifier 是本机库（需要解压提取 .dll/.so）
//
// Maven 坐标转下载 URL 规则:
//
//	group 中的 "." → "/"
//	最终 URL: {maven_url}/{group_path}/{artifact}/{version}/{artifact}-{version}.jar
//	例: "org.lwjgl:lwjgl:3.3.1" → org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar
type LibraryEntry struct {
	Name      string            `json:"name"`          // Maven 坐标
	URL       string            `json:"url,omitempty"` // Maven 仓库基 URL（Fabric 格式用到）
	Downloads *LibraryDownloads `json:"downloads,omitempty"`
	Rules     []Rule            `json:"rules,omitempty"`   // 空 = 全部平台都适用
	Natives   map[string]string `json:"natives,omitempty"` // e.g. {"windows": "natives-windows"}
}

// LibraryDownloads 库文件下载信息
type LibraryDownloads struct {
	Artifact    *DownloadEntry           `json:"artifact,omitempty"`
	Classifiers map[string]DownloadEntry `json:"classifiers,omitempty"`
}

// Rule 操作系统规则
// 用于条件包含/排除库文件和启动参数。
// Action 为 "allow" 表示匹配则该规则适用，"disallow" 表示匹配则不适用。
// 多个 rules 之间是 OR 关系（任一匹配即生效）。
//
// 典型用法:
//
//	{"action": "allow", "os": {"name": "windows"}}  // 仅 Windows
//	{"action": "disallow", "os": {"name": "osx"}}    // 排除 macOS
//	{"action": "allow"}                               // 所有平台
//	空 rules 数组 = 所有平台都适用（无限制）
//
// 注意：Mojang 的 rules 支持 os.version 正则匹配（如 "10\\.0\\..*"）、
// os.arch 架构判断（"x86" vs 默认 64 位）、以及 features 标签（如 is_demo_user）。
// 参考 PCL 的 McJsonRuleCheck 实现。
type Rule struct {
	Action   string        `json:"action"`
	OS       *OSRule       `json:"os,omitempty"`
	Features *RuleFeatures `json:"features,omitempty"`
}

// OSRule 操作系统匹配规则
type OSRule struct {
	Name    string `json:"name,omitempty"`    // "windows" | "osx" | "linux" | ""
	Version string `json:"version,omitempty"` // 版本正则，如 "10\\.0\\..*"
	Arch    string `json:"arch,omitempty"`    // "x86" 表示 32 位
}

// RuleFeatures 特性标签规则
// 目前已知的 features:
//
//	is_demo_user — 反选（非 Demo 用户才匹配）
//	has_custom_resolution — 通常忽略
//	quick_play* — PCL 选择始终不匹配
type RuleFeatures struct {
	IsDemoUser *bool `json:"is_demo_user,omitempty"`
	// 其他 features 通过 Raw 保留，用不上时可以忽略
}

// Arguments 启动参数
// 新版 version.json (>=1.13) 使用 arguments 字段替代 mineshaftArguments
// arguments.game 和 arguments.jvm 都是数组，元素可以是字符串或 Rule 对象
//
// 启动格式:
//
//	java [JVM args] mainClass [game args]
//
// 需要替换的占位符:
//
//	JVM: ${natives_directory}, ${classpath}, ${launcher_name}, ...
//	Game: ${auth_player_name}, ${version_name}, ${assets_root}, ...
type Arguments struct {
	Game []interface{} `json:"game,omitempty"`
	JVM  []interface{} `json:"jvm,omitempty"`
}

// VersionMetaManager 版本元数据管理器
type VersionMetaManager struct {
	cacheDir string
	dl       *downloader.Downloader
	mm       *VersionManifestManager
}

// NewVersionMetaManager 创建版本元数据管理器
func NewVersionMetaManager(cacheDir string, mm *VersionManifestManager) *VersionMetaManager {
	return &VersionMetaManager{
		cacheDir: cacheDir,
		dl:       downloader.New(),
		mm:       mm,
	}
}

// Fetch 获取指定版本 ID 的元数据
// 优先从缓存加载，过期则重新拉取
func (m *VersionMetaManager) Fetch(versionID string) (*VersionMeta, error) {
	// 先在清单中找这个版本的 URL
	var versionURL string
	if m.mm != nil {
		entry := m.mm.FindVersion(versionID)
		if entry != nil {
			versionURL = entry.URL
		}
	}

	if versionURL == "" {
		return nil, fmt.Errorf("版本 %s 未在清单中找到", versionID)
	}

	// 尝试从缓存加载
	cachePath := m.cachePath(versionID)
	if meta, err := m.loadVersionMeta(cachePath); err == nil {
		logger.Debug("版本元数据缓存命中: %s", versionID)
		return meta, nil
	}

	// 下载 version.json
	tmpDir := filepath.Join(m.cacheDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("version_%s_%d.json", versionID, time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	logger.Info("下载版本元数据: %s", versionID)
	if err := m.dl.File(versionURL, tmpFile, ""); err != nil {
		return nil, fmt.Errorf("下载版本元数据失败: %w", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("读取版本元数据失败: %w", err)
	}

	meta, err := parseVersionMeta(data)
	if err != nil {
		return nil, err
	}

	// 写入缓存
	m.saveVersionMeta(cachePath, data)

	return meta, nil
}

// DownloadClientJar 下载指定版本的 client.jar
// 返回下载后的完整路径
func (m *VersionMetaManager) DownloadClientJar(meta *VersionMeta, destDir string) (string, error) {
	if meta.Downloads == nil || meta.Downloads.Client == nil {
		return "", fmt.Errorf("版本 %s 没有 client.jar 下载信息", meta.ID)
	}

	client := meta.Downloads.Client
	destPath := filepath.Join(destDir, fmt.Sprintf("%s.jar", meta.ID))

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 已存在且 SHA1 匹配则跳过
	if _, err := os.Stat(destPath); err == nil {
		if ok, _ := verifySHA1(destPath, client.Sha1); ok {
			logger.Info("client.jar 已存在且校验通过: %s", destPath)
			return destPath, nil
		}
		logger.Info("client.jar 校验失败，重新下载: %s", meta.ID)
	} else {
		logger.Info("下载 client.jar: %s (%d MB)", meta.ID, client.Size/1024/1024)
	}

	// 先写入临时文件
	if err := m.dl.File(client.URL, destPath, ""); err != nil {
		return "", fmt.Errorf("下载 client.jar 失败: %w", err)
	}

	// SHA1 校验
	if ok, err := verifySHA1(destPath, client.Sha1); err != nil {
		return "", fmt.Errorf("SHA1 校验执行失败: %w", err)
	} else if !ok {
		os.Remove(destPath)
		return "", fmt.Errorf("client.jar SHA1 校验不匹配")
	}

	logger.Info("client.jar 下载完成")
	return destPath, nil
}

// cachePath 生成缓存文件路径
func (m *VersionMetaManager) cachePath(versionID string) string {
	return filepath.Join(m.cacheDir, fmt.Sprintf("%s.json", versionID))
}

// loadVersionMeta 从缓存加载版本元数据
func (m *VersionMetaManager) loadVersionMeta(path string) (*VersionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// 缓存 1 小时
	if time.Since(fi.ModTime()) > 1*time.Hour {
		return nil, fmt.Errorf("缓存过期")
	}

	return parseVersionMeta(data)
}

// saveVersionMeta 缓存版本元数据
func (m *VersionMetaManager) saveVersionMeta(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logger.Warn("创建版本缓存目录失败: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logger.Warn("写入版本缓存失败: %v", err)
	}
}

// parseVersionMeta 解析 version.json
func parseVersionMeta(data []byte) (*VersionMeta, error) {
	var meta VersionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析 version.json 失败: %w", err)
	}
	if meta.ID == "" {
		return nil, fmt.Errorf("version.json 缺少 id 字段")
	}
	return &meta, nil
}

// ResolveLibraries 递归解析库文件列表（处理 inheritsFrom 继承）
//
// Minecraft 的加载器版本（Fabric/Forge/NeoForge）通常不直接列出所有依赖，
// 而是通过 inheritsFrom 指向原始版本。这个方法递归拉取父版本的 libraries，
// 子版本的库排在前面（Java classpath 顺序语义：先出现的优先）。
//
// 返回的已通过 rules 过滤且去重的库条目。
// versionDir: 父版本的版本目录，用于加载继承的 version.json
func (m *VersionMetaManager) ResolveLibraries(meta *VersionMeta, versionDir string) ([]LibraryEntry, error) {
	var allLibs []LibraryEntry
	seen := make(map[string]bool)

	// 递归解析
	var resolve func(meta *VersionMeta) error
	resolve = func(meta *VersionMeta) error {
		for _, lib := range meta.Libraries {
			// rules 过滤
			if !ShouldInclude(lib.Rules) {
				continue
			}

			// 去重：以 group:artifact 为 key
			coords := ParseMavenCoords(lib.Name)
			if coords != nil {
				key := coords.Group + ":" + coords.Artifact
				if seen[key] {
					continue
				}
				seen[key] = true
			}

			allLibs = append(allLibs, lib)
		}

		// 处理继承
		if meta.InheritsFrom != "" {
			parentMeta, err := m.Fetch(meta.InheritsFrom)
			if err != nil {
				return fmt.Errorf("获取继承版本 %s 元数据失败: %w", meta.InheritsFrom, err)
			}
			return resolve(parentMeta)
		}
		return nil
	}

	if err := resolve(meta); err != nil {
		return nil, err
	}

	return allLibs, nil
}

// verifySHA1 校验文件 SHA1
func verifySHA1(path, expected string) (bool, error) {
	if expected == "" {
		return true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == expected, nil
}
