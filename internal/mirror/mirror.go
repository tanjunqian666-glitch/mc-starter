package mirror

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Mirror 镜像配置
// 国内用户访问 Mojang 官方 S3 (Amazon) 带宽极慢，
// BMCLAPI (bangbang93) 是国内最主流的 Minecraft 文件镜像。
//
// 各端点的功能：
//
//	Manifest   — 版本清单 (version_manifest_v2.json)
//	Version    — 版本元数据 (version.json)
//	Asset      — 资源文件 (音效/纹理等，通过 SHA1 前 2 位分目录)
//	Library    — Java 库文件 (Maven 仓库镜像)
//	FabricMeta — Fabric Loader 元数据 API
//	FabricMaven — Fabric Maven 仓库
type Mirror struct {
	Label       string
	Manifest    string
	Version     string
	Asset       string
	Library     string
	FabricMeta  string
	FabricMaven string
}

// DefaultMirrors 各模式镜像映射
// global: Mojang 官方 + Fabric 官方
// china:  BMCLAPI (bangbang93) — 国内最核心的 MC 镜像
var DefaultMirrors = map[string]Mirror{
	"global": {
		Label:       "Mojang",
		Manifest:    "https://piston-meta.mojang.com",
		Version:     "https://piston-meta.mojang.com",
		Asset:       "https://resources.download.minecraft.net",
		Library:     "https://libraries.minecraft.net",
		FabricMeta:  "https://meta.fabricmc.net",
		FabricMaven: "https://maven.fabricmc.net",
	},
	"china": {
		Label:       "BMCLAPI",
		Manifest:    "https://bmclapi2.bangbang93.com",
		Version:     "https://bmclapi2.bangbang93.com",
		Asset:       "https://bmclapi2.bangbang93.com/assets",
		Library:     "https://bmclapi2.bangbang93.com/maven",
		FabricMeta:  "https://bmclapi2.bangbang93.com/fabric-meta",
		FabricMaven: "https://bmclapi2.bangbang93.com/maven",
	},
}

// ============================================================
// 智能镜像选择（参考 PCL DlSourceLauncherOrMetaGet）
//
// PCL 的做法：
//   1. 首次用官方源下载，记录耗时
//   2. <4s 标记为"可优先使用" → 保持官方源
//   3. >4s 切换到 BMCLAPI 镜像
//   4. 维护 DlPreferMojang 全局状态
//
// 我们的实现：
//   1. 在 downloader 层包装，每次 URL 请求前做延迟探针
//   2. 如果 primary 镜像连续 N 次超过阈值 → 切换到 fallback
//   3. 切换后冷却期 (5min) 内不再切换
// ============================================================

// ProbeResult 延迟探测结果
type ProbeResult struct {
	Latency    time.Duration
	PrimaryURL string // 首选 URL（已根据探测结果选择）
	Source     string // "primary" | "fallback"
}

// SmartSelector 智能镜像选择器
type SmartSelector struct {
	mu               sync.RWMutex
	mode             string
	primary          Mirror
	fallback         Mirror
	threshold        time.Duration // 延迟阈值，超时切换
	cooldown         time.Duration // 切换后冷却期
	lastSwitch       time.Time     // 上次切换时间
	usingFallback    bool
	consecutiveSlow  int           // 连续慢请求计数
	slowThreshold    int           // 连续几次慢才切换
	primaryLatencies []time.Duration // 最近 N 次延迟采样
}

// NewSmartSelector 创建智能镜像选择器
// mode: "global" | "china" | "auto"
// threshold: 延迟阈值（默认 4s，PCL 参考值）
func NewSmartSelector(mode string, threshold time.Duration) *SmartSelector {
	if mode == "" || mode == "auto" {
		mode = "global"
	}
	if threshold <= 0 {
		threshold = 4 * time.Second
	}

	primary := Resolve(mode)
	fallback := Resolve("china")
	if mode == "china" {
		fallback = Resolve("global")
	}

	return &SmartSelector{
		mode:             mode,
		primary:          primary,
		fallback:         fallback,
		threshold:        threshold,
		cooldown:         5 * time.Minute,
		slowThreshold:    2,
		primaryLatencies: make([]time.Duration, 0, 10),
	}
}

// ProbeAndSelect 探测 URL 并选择最优镜像
// url: 原始 URL（官方源地址）
// probeURL: 用于探测的轻量 URL（通常与 url 同源，也可用 manifest 端点）
//
// 返回：
//   - selectedURL: 选定的下载 URL（可能是镜像地址）
//   - source: "primary" | "fallback"
//   - latency: 探测延迟
func (s *SmartSelector) ProbeAndSelect(url, probeURL string) (selectedURL string, source string, latency time.Duration) {
	// 检查冷却期
	s.mu.RLock()
	usingFallback := s.usingFallback
	lastSwitch := s.lastSwitch
	mirror := s.primary
	if usingFallback {
		mirror = s.fallback
	}
	s.mu.RUnlock()

	if usingFallback {
		// 检查冷却期是否已过，过期尝试切回 primary
		if time.Since(lastSwitch) > s.cooldown {
			// 尝试探测 primary
			latency = probeLatency(probeURL, 3*time.Second)
			if latency < s.threshold {
				s.mu.Lock()
				s.usingFallback = false
				s.consecutiveSlow = 0
				s.lastSwitch = time.Now()
				s.mu.Unlock()
				return url, "primary", latency
			}
		}
		// 仍用 fallback
		mirrored := rewriteURL(url, mirror)
		return mirrored, "fallback", latency
	}

	// primary 模式下：探测延迟
	latency = probeLatency(probeURL, 3*time.Second)

	s.mu.Lock()
	s.primaryLatencies = append(s.primaryLatencies, latency)
	if len(s.primaryLatencies) > 10 {
		s.primaryLatencies = s.primaryLatencies[1:]
	}

	if latency > s.threshold || latency == 0 {
		s.consecutiveSlow++
	} else {
		s.consecutiveSlow = 0
	}

	if s.consecutiveSlow >= s.slowThreshold {
		s.usingFallback = true
		s.lastSwitch = time.Now()
		s.consecutiveSlow = 0
		mirror = s.fallback
		s.mu.Unlock()

		mirrored := rewriteURL(url, mirror)
		return mirrored, "fallback", latency
	}
	s.mu.Unlock()

	return url, "primary", latency
}

// CurrentSource 返回当前正在使用的镜像源
func (s *SmartSelector) CurrentSource() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.usingFallback {
		return fmt.Sprintf("fallback (%s)", s.fallback.Label)
	}
	return fmt.Sprintf("primary (%s)", s.primary.Label)
}

// ============================================================
// 辅助函数
// ============================================================

// probeLatency 探测 URL 的延迟
// 返回实际耗时，超时返回 0
func probeLatency(url string, timeout time.Duration) time.Duration {
	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Head(url)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return time.Since(start)
}

// rewriteURL 将官方 URL 重写为镜像 URL
// 根据镜像的各个端点类型进行替换
func rewriteURL(originalURL string, m Mirror) string {
	base := bestMatch(originalURL)
	if base == "" {
		return originalURL
	}

	replacement := ""
	switch base {
	case "https://piston-meta.mojang.com":
		replacement = m.Manifest
	case "https://resources.download.minecraft.net":
		replacement = m.Asset
	case "https://libraries.minecraft.net":
		replacement = m.Library
	case "https://meta.fabricmc.net":
		replacement = m.FabricMeta
	case "https://maven.fabricmc.net":
		replacement = m.FabricMaven
	default:
		return originalURL
	}

	return strings.Replace(originalURL, base, replacement, 1)
}

// bestMatch 从 URL 中匹配已知的官方源前缀
func bestMatch(url string) string {
	known := []string{
		"https://piston-meta.mojang.com",
		"https://resources.download.minecraft.net",
		"https://libraries.minecraft.net",
		"https://meta.fabricmc.net",
		"https://maven.fabricmc.net",
	}
	for _, k := range known {
		if strings.HasPrefix(url, k) {
			return k
		}
	}
	return ""
}

// Resolve 根据 mode 获取镜像集
func Resolve(mode string) Mirror {
	if m, ok := DefaultMirrors[mode]; ok {
		return m
	}
	return DefaultMirrors["global"]
}

// AssetURL 构造 asset 对象下载 URL
func AssetURL(m Mirror, hash string) string {
	return strings.Join([]string{m.Asset, hash[:2], hash}, "/")
}

// LibraryURL 构造库文件 URL
func LibraryURL(m Mirror, path string) string {
	return strings.Join([]string{m.Library, path}, "/")
}
