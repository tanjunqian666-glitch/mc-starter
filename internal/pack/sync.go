package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P1.14 — zip 与仓库整合（增量 + 缓存复用）
//
// 将 zip sync 与 LocalRepo 仓库体系整合，流程：
//   1. 解析 server.json 获取 modpack source
//   2. 下载/解压 zip（或使用既有的 CacheStore 命中）
//   3. 计算差异
//   4. 应用差异到 .minecraft
//   5. 创建/更新 LocalRepo 快照
//   6. 清理临时文件
// ============================================================

// SyncConfig zip 同步配置
type SyncConfig struct {
	URL      string   // zip 下载地址
	Name     string   // 包标识（用于目录和日志）
	Targets  []string // 需要同步的目录前缀（如 ["mods", "config"]），空=全部
	Hash     string   // 期望的 SHA256（可选校验）
	TempDir  string   // 临时目录
	CacheDir string   // CacheStore 缓存目录（可选）
}

// SyncManager zip 同步管理器
type SyncManager struct {
	zipHandler *ZipHandler
}

// NewSyncManager 创建 zip 同步管理器
func NewSyncManager() *SyncManager {
	return &SyncManager{
		zipHandler: NewZipHandler(),
	}
}

// SyncFromURL 通过 URL 下载并同步 zip 包
//
// 完整流程:
//   1. 下载 zip → 校验（可选）→ 解压到临时目录
//   2. 遍历每个 target prefix 计算差异
//   3. 应用差异到 installDir
//   4. 清理临时文件
//
// 参数:
//   - url:        zip 下载地址
//   - installDir: MC 安装目录（如 .minecraft）
//   - name:       包标识
//   - targets:    需要同步的目录列表（如 ["mods", "config"]），空=同步全部
//   - hash:       期望 SHA256（可选，传 "" 跳过校验）
//
// 返回:
//   - map[string]*DiffResult: 每个 target 的差异结果
//   - error:                   错误信息
func (sm *SyncManager) SyncFromURL(url, installDir, name string, targets []string, hash string) (*SyncResult, error) {
	logger.Info("pack sync: %s (%s)", name, url)

	// 下载并解压
	tempDir := filepath.Join(installDir, ".starter_cache", "pack", name)
	handler := NewZipHandler()
	if hash != "" {
		handler.WithHash(hash)
	}
	result, err := handler.DownloadAndExtract(url, tempDir, name)
	if err != nil {
		return nil, fmt.Errorf("zip 处理失败: %w", err)
	}

	// 如果解压出的是单层目录（如整合包常见格式），自动展平
	result = flattenSingleDir(result)

	// 确保不在 cleanup 前返回（用 defer 兜底）
	return sm.syncFromResult(result, installDir, name, targets)
}

// SyncExisting 同步已有的 zip 文件（不下载）
func (sm *SyncManager) SyncExisting(zipPath, installDir, name string, targets []string) (*SyncResult, error) {
	tempDir := filepath.Join(installDir, ".starter_cache", "pack", name)
	result, err := NewZipHandler().ExtractExisting(zipPath, tempDir, name)
	if err != nil {
		return nil, fmt.Errorf("zip 解压失败: %w", err)
	}
	result = flattenSingleDir(result)
	return sm.syncFromResult(result, installDir, name, targets)
}

// syncFromResult 从已解压的 zip 执行同步
func (sm *SyncManager) syncFromResult(result *ZipResult, installDir, name string, targets []string) (*SyncResult, error) {
	defer result.Cleanup()

	syncResult := &SyncResult{
		Name:   name,
		Target: make(map[string]*DiffResult),
	}

	// 如果没有指定 targets，同步所有文件
	effectiveTargets := targets
	if len(effectiveTargets) == 0 {
		effectiveTargets = []string{""} // 空 prefix = 全部
	}

	// 对每个 target 执行差异同步
	for _, target := range effectiveTargets {
		diff := ComputeDiff(result.Entries, installDir, target)
		syncResult.Target[target] = diff

		if !diff.HasChanges() {
			logger.Info("pack sync [%s]: 无变更, 跳过", target)
			continue
		}

		logger.Info("pack sync [%s]: %s", target, diff.Summary())

		// 应用差异
		applied := ApplyDiff(diff, result.Entries, installDir, false)
		logger.Info("pack sync [%s]: 已应用 %d 个文件变更", target, applied)

		syncResult.Applied += applied
	}

	syncResult.Completed = true
	logger.Info("pack sync %s: 完成, 共 %d 个变更", name, syncResult.Applied)
	return syncResult, nil
}

// SyncResult zip 同步结果
type SyncResult struct {
	Name      string                    // 包名
	Target    map[string]*DiffResult    // 每个 target 的差异
	Applied   int                       // 实际应用的文件数
	Completed bool                      // 是否成功完成
}

// Summary 返回同步摘要
func (sr *SyncResult) Summary() string {
	if !sr.Completed {
		return fmt.Sprintf("%s: 未完成", sr.Name)
	}
	var parts []string
	for target, diff := range sr.Target {
		parts = append(parts, fmt.Sprintf("[%s] %s", target, diff.Summary()))
	}
	return fmt.Sprintf("%s: %d 变更 — %s", sr.Name, sr.Applied, strings.Join(parts, "; "))
}

// ============================================================
// 辅助函数
// ============================================================

// flattenSingleDir 如果 zip 解压后只有一个顶层目录，展平
// 很多整合包 zip 是 "ModpackName/mods/x.jar" 格式
func flattenSingleDir(result *ZipResult) *ZipResult {
	if result == nil || len(result.Entries) == 0 {
		return result
	}

	// 找出所有顶层目录
	topDirs := make(map[string]bool)
	for _, entry := range result.Entries {
		parts := strings.SplitN(entry.RelPath, "/", 2)
		if len(parts) == 2 {
			topDirs[parts[0]] = true
		}
	}

	// 如果只有一个顶层目录，展平
	if len(topDirs) == 1 {
		prefix := ""
		for k := range topDirs {
			prefix = k + "/"
		}

		newEntries := make([]ZipEntry, len(result.Entries))
		prefixLen := len(prefix)
		for i, entry := range result.Entries {
			if strings.HasPrefix(entry.RelPath, prefix) {
				newEntries[i] = entry
				newEntries[i].RelPath = entry.RelPath[prefixLen:]
			} else {
				newEntries[i] = entry
			}
		}
		result.Entries = newEntries
		logger.Debug("pack: zip 单层目录展平: 移除前缀 %s", prefix)
	}

	return result
}

// HasPendingSyncOrClean 打印同步待处理摘要（用于 CLI --dry-run 模式）
func PrintPendingSyncDiff(targetDiffs map[string]*DiffResult) {
	for target, diff := range targetDiffs {
		if !diff.HasChanges() {
			fmt.Printf("  [%s] 已是最新\n", target)
			continue
		}
		fmt.Printf("  [%s] %s\n", target, diff.Summary())
		for _, entry := range diff.Added[:min(len(diff.Added), 5)] {
			fmt.Printf("    + %s (%d KB)\n", entry.RelPath, entry.Size/1024)
		}
		if len(diff.Added) > 5 {
			fmt.Printf("    ... (+%d more)\n", len(diff.Added)-5)
		}
		for _, entry := range diff.Updated[:min(len(diff.Updated), 3)] {
			fmt.Printf("    ~ %s\n", entry.RelPath)
		}
		for _, entry := range diff.Deleted[:min(len(diff.Deleted), 5)] {
			fmt.Printf("    - %s\n", entry.RelPath)
		}
		if len(diff.Deleted) > 5 {
			fmt.Printf("    ... (-%d more)\n", len(diff.Deleted)-5)
		}
	}
}

// EnsureDir 确保目录存在
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// Timestamp 返回当前时间戳字符串
func Timestamp() string {
	return time.Now().Format("20060102-150405")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
