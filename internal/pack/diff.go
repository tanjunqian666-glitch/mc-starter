package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P1.13 — zip 内容差异同步
//
// 对比已解压的 zip 文件与目标目录（.minecraft），输出差异：
//   - Added:    zip 中有但目录中没有的
//   - Updated:  文件名相同但 hash 不同的
//   - Deleted:  目录中有但 zip 中没有的（仅限同步范围内的文件）
//   - Unchanged: 完全一致的
// ============================================================

// DiffEntry 单个文件的差异
type DiffEntry struct {
	RelPath  string // 相对路径
	Status   string // "added" | "updated" | "deleted" | "unchanged"
	NewSHA1  string // 新文件 SHA1（删除时为空）
	OldSHA1  string // 旧文件 SHA1（新增时为空）
	Size     int64  // 文件大小
}

// DiffResult 差异计算结果
type DiffResult struct {
	Added      []DiffEntry
	Updated    []DiffEntry
	Deleted    []DiffEntry
	Unchanged  int
	TotalBytes int64 // 需要下载/复制的总字节数
}

// ComputeDiff 计算两个文件集合的差异
//
// 参数:
//   - zipEntries:  zip 包中的文件清单
//   - targetDir:   本地目标目录（如 .minecraft）
//   - prefix:      需要对比的目录前缀（如 "mods", "config"），空则对比所有
//
// 返回:
//   - DiffResult: 差异结果
func ComputeDiff(zipEntries []ZipEntry, targetDir string, prefix string) *DiffResult {
	result := &DiffResult{}

	// 构建 target 的文件索引（relPath → hash）
	targetFiles := buildTargetIndex(targetDir, prefix)

	// 构建 zip 的文件索引
	zipFiles := make(map[string]ZipEntry)
	for _, entry := range zipEntries {
		if prefix == "" || strings.HasPrefix(entry.RelPath, prefix) {
			zipFiles[entry.RelPath] = entry
		}
	}

	// 遍历 zip 文件集合，与 target 对比
	for relPath, zipEntry := range zipFiles {
		targetHash, exists := targetFiles[relPath]
		if !exists {
			result.Added = append(result.Added, DiffEntry{
				RelPath: relPath,
				Status:  "added",
				NewSHA1: zipEntry.SHA1,
				Size:    zipEntry.Size,
			})
			result.TotalBytes += zipEntry.Size
		} else if targetHash != zipEntry.SHA1 {
			result.Updated = append(result.Updated, DiffEntry{
				RelPath: relPath,
				Status:  "updated",
				NewSHA1: zipEntry.SHA1,
				OldSHA1: targetHash,
				Size:    zipEntry.Size,
			})
			result.TotalBytes += zipEntry.Size
		} else {
			result.Unchanged++
		}
	}

	// 遍历 target 文件集合，找出 zip 中没有的（已删除）
	for relPath := range targetFiles {
		if _, exists := zipFiles[relPath]; !exists {
			result.Deleted = append(result.Deleted, DiffEntry{
				RelPath: relPath,
				Status:  "deleted",
			})
		}
	}

	return result
}

// ApplyDiff 将差异应用到目标目录
//
// 参数:
//   - diff:       差异结果
//   - zipEntries: zip 包文件清单（用于获取源文件路径）
//   - targetDir:  目标目录
//   - dryRun:     仅显示不执行
//
// 返回: 成功应用的文件数
func ApplyDiff(diff *DiffResult, zipEntries []ZipEntry, targetDir string, dryRun bool) int {
	// 构建 zip 文件索引（relPath → ZipEntry）
	zipIndex := make(map[string]ZipEntry)
	for _, entry := range zipEntries {
		zipIndex[entry.RelPath] = entry
	}

	applied := 0

	// 处理新增
	for _, entry := range diff.Added {
		src, ok := zipIndex[entry.RelPath]
		if !ok {
			logger.Warn("diff: 新增 %s 的 zip 源文件未找到", entry.RelPath)
			continue
		}
		dst := filepath.Join(targetDir, entry.RelPath)
		if dryRun {
			logger.Info("[DRY-RUN] 新增: %s (%d KB)", entry.RelPath, src.Size/1024)
			applied++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			logger.Warn("diff: 创建目录 %s 失败: %v", filepath.Dir(dst), err)
			continue
		}
		if err := copyFile(src.TempPath, dst); err != nil {
			logger.Warn("diff: 复制 %s 失败: %v", entry.RelPath, err)
			continue
		}
		applied++
		logger.Debug("diff: 新增 %s", entry.RelPath)
	}

	// 处理更新
	for _, entry := range diff.Updated {
		src, ok := zipIndex[entry.RelPath]
		if !ok {
			logger.Warn("diff: 更新 %s 的 zip 源文件未找到", entry.RelPath)
			continue
		}
		dst := filepath.Join(targetDir, entry.RelPath)
		if dryRun {
			logger.Info("[DRY-RUN] 更新: %s (%d KB)", entry.RelPath, src.Size/1024)
			applied++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			logger.Warn("diff: 创建目录 %s 失败: %v", filepath.Dir(dst), err)
			continue
		}
		if err := copyFile(src.TempPath, dst); err != nil {
			logger.Warn("diff: 更新 %s 失败: %v", entry.RelPath, err)
			continue
		}
		applied++
		logger.Debug("diff: 更新 %s", entry.RelPath)
	}

	// 处理删除
	for _, entry := range diff.Deleted {
		dst := filepath.Join(targetDir, entry.RelPath)
		if dryRun {
			logger.Info("[DRY-RUN] 删除: %s", entry.RelPath)
			applied++
			continue
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			logger.Warn("diff: 删除 %s 失败: %v", entry.RelPath, err)
			continue
		}
		applied++
		logger.Debug("diff: 删除 %s", entry.RelPath)
	}

	return applied
}

// Summary 返回差异的文本摘要
func (d *DiffResult) Summary() string {
	parts := []string{
		fmt.Sprintf("+%d 新增", len(d.Added)),
		fmt.Sprintf("~%d 更新", len(d.Updated)),
		fmt.Sprintf("-%d 删除", len(d.Deleted)),
		fmt.Sprintf("=%d 未变", d.Unchanged),
	}
	if d.TotalBytes > 0 {
		parts = append(parts, fmt.Sprintf("(%d KB 需同步)", d.TotalBytes/1024))
	}
	return strings.Join(parts, " · ")
}

// HasChanges 是否有实际变更
func (d *DiffResult) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Updated) > 0 || len(d.Deleted) > 0
}

// ============================================================
// 辅助函数
// ============================================================

// buildTargetIndex 构建目标目录的文件索引（relPath → SHA1）
func buildTargetIndex(targetDir string, prefix string) map[string]string {
	index := make(map[string]string)

	walkPrefix := targetDir
	if prefix != "" {
		walkPrefix = filepath.Join(targetDir, prefix)
	}

	filepath.Walk(walkPrefix, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(targetDir, path)
		if err != nil {
			return nil
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		if prefix == "" || strings.HasPrefix(relPath, prefix) {
			hash, _, err := computeHashes(path)
			if err == nil {
				index[relPath] = hash
			}
		}
		return nil
	})

	return index
}

// copyFile 复制文件（与 repo.go 中的 copyFile 相同逻辑，避免跨包依赖）
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
