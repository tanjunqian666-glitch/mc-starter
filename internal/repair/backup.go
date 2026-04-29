package repair

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P2.6 备份系统 — 修复前自动创建、手动创建、回滚、清理
//
// 与 repo 快照（starter_repo/）的区别：
//   - repo 快照：面向整合包 mods/config 版本管理，用于差异同步和回滚
//   - 通用备份（本文件）：面向用户数据保护，备份 saves/screenshots/options.txt 等，
//     在 repair / 升级前自动创建
//
// 存储结构：
//   {mcDir}/starter_backups/
//   ├── backup_20260429_022800/
//   │   ├── meta.json
//   │   ├── mods/
//   │   ├── config/
//   │   ├── saves/
//   │   ├── resourcepacks/
//   │   ├── shaderpacks/
//   │   ├── screenshots/
//   │   └── options.txt
//   ├── backup_20260428_153000/
//   └── ...
// ============================================================

const (
	// BackupDirName starter_backups 目录名
	BackupDirName = "starter_backups"
	// DefaultMaxBackups 默认最大保留备份数
	DefaultMaxBackups = 5
	// DefaultMaxBackupSize 默认最大备份总大小（2GB）
	DefaultMaxBackupSize = 2 * 1024 * 1024 * 1024 // 2GB
)

// BackupReason 备份原因
type BackupReason string

const (
	ReasonRepair  BackupReason = "repair"   // 修复前创建
	ReasonUpgrade BackupReason = "upgrade"  // 版本升级前创建
	ReasonManual  BackupReason = "manual"   // 用户手动创建
	ReasonPreSync BackupReason = "pre-sync" // 首次同步前（已有 .minecraft）
)

// BackupMeta 备份元信息
type BackupMeta struct {
	ID          string       `json:"id"`          // 时间戳，如 "20260429_022800"
	CreatedAt   time.Time    `json:"created_at"`  // 创建时间
	Reason      BackupReason `json:"reason"`      // 创建原因
	MCVersion   string       `json:"mc_version,omitempty"`
	ModCount    int          `json:"mod_count,omitempty"`
	ConfigCount int          `json:"config_count,omitempty"`
	SaveCount   int          `json:"save_count,omitempty"`
	SizeBytes   int64        `json:"size_bytes"` // 备份总大小
	FileCount   int          `json:"file_count"` // 文件总数
}

// BackupOptions 创建备份的选项
type BackupOptions struct {
	Reason    BackupReason // 备份原因
	MCVersion string      // 关联的 MC 版本（可选）
	MaxKeep   int         // 最大保留备份数（0=DefaultMaxBackups）
}

// BackupListEntry 备份列表中的一项
type BackupListEntry struct {
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	Reason    BackupReason `json:"reason"`
	ModCount  int          `json:"mod_count"`
	SaveCount int          `json:"save_count"`
	SizeBytes int64        `json:"size_bytes"`
	FileCount int          `json:"file_count"`
}

// BackupResult 创建备份的结果
type BackupResult struct {
	BackupDir string // 备份目录路径
	FileCount int    // 备份的文件数
	SizeBytes int64  // 备份大小
}

// BackedUpItem 已备份的项目统计
type BackedUpItem struct {
	Name  string
	Files int
	Size  int64
}

// CreateBackup 创建通用备份
// 备份目录下需要保护的用户数据，不包括 versions/assets/libraries
// mcDir: .minecraft 根目录
// opts: 备份选项（留空使用默认值）
// 返回: 备份结果
func CreateBackup(mcDir string, opts BackupOptions) (*BackupResult, error) {
	backupRoot := filepath.Join(mcDir, BackupDirName)
	if err := os.MkdirAll(backupRoot, 0755); err != nil {
		return nil, fmt.Errorf("创建备份根目录失败: %w", err)
	}

	// 生成备份 ID
	id := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupRoot, "backup_"+id)
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("创建备份目录 %s 失败: %w", backupPath, err)
	}

	// 定义要备份的条目
	items := []struct {
		RelPath  string // 相对于 mcDir 的路径
		Required bool   // 是否存在检查
		IsDir    bool   // 是否是目录
	}{
		{RelPath: "mods", IsDir: true},
		{RelPath: "config", IsDir: true},
		{RelPath: "saves", IsDir: true},
		{RelPath: "resourcepacks", IsDir: true},
		{RelPath: "shaderpacks", IsDir: true},
		{RelPath: "screenshots", IsDir: true},
		{RelPath: "options.txt", IsDir: false},
	}

	var totalFiles, modCount, configCount, saveCount int
	var totalSize int64
	var backedItems []BackedUpItem

	for _, item := range items {
		srcPath := filepath.Join(mcDir, item.RelPath)
		dstPath := filepath.Join(backupPath, item.RelPath)

		// 检查源是否存在
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			if item.Required {
				logger.Warn("备份: 必需的 %s 不存在，跳过", item.RelPath)
			}
			continue
		}

		var files int
		var size int64
		var err error

		if item.IsDir {
			files, size, err = copyDir(srcPath, dstPath)
		} else {
			err = copyFile(srcPath, dstPath)
			if err == nil {
				files = 1
				if fi, statErr := os.Stat(srcPath); statErr == nil {
					size = fi.Size()
				}
			}
		}

		if err != nil {
			logger.Warn("备份 %s 失败: %v", item.RelPath, err)
			continue
		}

		totalFiles += files
		totalSize += size
		backedItems = append(backedItems, BackedUpItem{
			Name:  item.RelPath,
			Files: files,
			Size:  size,
		})

		// 分类统计
		switch item.RelPath {
		case "mods":
			modCount = files
		case "config":
			configCount = files
		case "saves":
			saveCount = files
		}
	}

	// 写入元数据
	meta := BackupMeta{
		ID:          id,
		CreatedAt:   time.Now(),
		Reason:      opts.Reason,
		MCVersion:   opts.MCVersion,
		ModCount:    modCount,
		ConfigCount: configCount,
		SaveCount:   saveCount,
		SizeBytes:   totalSize,
		FileCount:   totalFiles,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化元数据失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupPath, "meta.json"), metaBytes, 0644); err != nil {
		return nil, fmt.Errorf("写入元数据失败: %w", err)
	}

	// 清理旧备份
	maxKeep := opts.MaxKeep
	if maxKeep <= 0 {
		maxKeep = DefaultMaxBackups
	}
	removed := cleanupOldBackups(backupRoot, maxKeep)
	if removed > 0 {
		logger.Info("已清理 %d 个旧备份", removed)
	}

	logger.Info("备份 %s 创建完成: %d 个文件, %.1f MB, 原因=%s",
		id, totalFiles, float64(totalSize)/1024/1024, opts.Reason)

	return &BackupResult{
		BackupDir: backupPath,
		FileCount: totalFiles,
		SizeBytes: totalSize,
	}, nil
}

// ListBackups 列出所有备份
// mcDir: .minecraft 根目录
// 返回: 按时间倒序（最新在前）的备份列表
func ListBackups(mcDir string) ([]BackupListEntry, error) {
	backupRoot := filepath.Join(mcDir, BackupDirName)
	if _, err := os.Stat(backupRoot); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}

	var backups []BackupListEntry
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "backup_") {
			continue
		}

		metaPath := filepath.Join(backupRoot, e.Name(), "meta.json")
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			// 没有 meta.json 仍显示为未知备份
			backups = append(backups, BackupListEntry{
				ID: strings.TrimPrefix(e.Name(), "backup_"),
			})
			continue
		}

		var meta BackupMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			backups = append(backups, BackupListEntry{
				ID: strings.TrimPrefix(e.Name(), "backup_"),
			})
			continue
		}

		backups = append(backups, BackupListEntry{
			ID:        meta.ID,
			CreatedAt: meta.CreatedAt,
			Reason:    meta.Reason,
			ModCount:  meta.ModCount,
			SaveCount: meta.SaveCount,
			SizeBytes: meta.SizeBytes,
			FileCount: meta.FileCount,
		})
	}

	// 按时间倒序（最新在前）
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// RestoreBackup 恢复指定备份
// mcDir: .minecraft 根目录
// backupID: 备份 ID（时间戳）
// 返回: 恢复的文件数
func RestoreBackup(mcDir string, backupID string) (int, error) {
	backupDir := filepath.Join(mcDir, BackupDirName, "backup_"+backupID)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return 0, fmt.Errorf("备份 %s 不存在", backupID)
	}

	// 恢复前自动备份当前状态（防手滑）
	preRestoreOpts := BackupOptions{
		Reason: ReasonUpgrade, // 等于是升级前备份
	}
	if _, err := CreateBackup(mcDir, preRestoreOpts); err != nil {
		logger.Warn("恢复前创建临时备份失败: %v", err)
		// 不阻断恢复流程
	}

	// 要恢复的目录/文件
	restoreItems := []string{
		"mods", "config", "saves", "resourcepacks",
		"shaderpacks", "screenshots", "options.txt",
	}

	var restored int
	for _, relPath := range restoreItems {
		srcPath := filepath.Join(backupDir, relPath)
		dstPath := filepath.Join(mcDir, relPath)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue
		}

		// 删除目标
		if err := os.RemoveAll(dstPath); err != nil {
			logger.Warn("恢复: 删除 %s 失败: %v", relPath, err)
			continue
		}

		// 如果目标是 options.txt（文件），直接复制
		if relPath == "options.txt" {
			if err := copyFile(srcPath, dstPath); err != nil {
				logger.Warn("恢复: 复制 %s 失败: %v", relPath, err)
				continue
			}
			restored++
			continue
		}

		// 如果是目录，重新创建并复制
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			logger.Warn("恢复: 创建父目录 %s 失败: %v", relPath, err)
			continue
		}

		files, _, err := copyDir(srcPath, dstPath)
		if err != nil {
			logger.Warn("恢复: 复制目录 %s 失败: %v", relPath, err)
			continue
		}
		restored += files
	}

	logger.Info("备份 %s 恢复完成: %d 个文件", backupID, restored)
	return restored, nil
}

// DeleteBackup 删除指定备份
func DeleteBackup(mcDir string, backupID string) error {
	backupDir := filepath.Join(mcDir, BackupDirName, "backup_"+backupID)
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return fmt.Errorf("备份 %s 不存在", backupID)
	}

	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("删除备份 %s 失败: %w", backupID, err)
	}

	logger.Info("备份 %s 已删除", backupID)
	return nil
}

// cleanupOldBackups 清理旧备份，保留最近 maxKeep 个
// 返回删除的备份数
func cleanupOldBackups(backupRoot string, maxKeep int) int {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return 0
	}

	var backupDirs []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "backup_") {
			backupDirs = append(backupDirs, e)
		}
	}

	if len(backupDirs) <= maxKeep {
		return 0
	}

	// 按名称排序（名称是时间戳，天然有序，旧的在前面）
	sort.Slice(backupDirs, func(i, j int) bool {
		return backupDirs[i].Name() < backupDirs[j].Name()
	})

	// 检查总大小是否超限
	totalSize := int64(0)
	for _, e := range backupDirs {
		totalSize += dirSize(filepath.Join(backupRoot, e.Name()))
	}

	removed := 0
	// 超过数量限制，从最旧的开始删
	for len(backupDirs) > maxKeep {
		old := backupDirs[0]
		oldPath := filepath.Join(backupRoot, old.Name())
		if err := os.RemoveAll(oldPath); err != nil {
			logger.Warn("清理旧备份 %s 失败: %v", old.Name(), err)
			backupDirs = backupDirs[1:]
			continue
		}
		logger.Debug("已删除旧备份: %s", old.Name())
		removed++
		backupDirs = backupDirs[1:]
		totalSize = 0
		for _, e := range backupDirs {
			totalSize += dirSize(filepath.Join(backupRoot, e.Name()))
		}
	}

	// 如果总大小超过 2GB，额外清理
	if totalSize > DefaultMaxBackupSize {
		for totalSize > DefaultMaxBackupSize && len(backupDirs) > 1 {
			old := backupDirs[0]
			oldPath := filepath.Join(backupRoot, old.Name())
			oldSize := dirSize(oldPath)
			if err := os.RemoveAll(oldPath); err != nil {
				logger.Warn("清理超大备份 %s 失败: %v", old.Name(), err)
				break
			}
			logger.Info("备份总大小超限 (%.1f GB)，已删除旧备份: %s",
				float64(totalSize)/1024/1024/1024, old.Name())
			removed++
			backupDirs = backupDirs[1:]
			totalSize -= oldSize
		}
	}

	return removed
}

// GetBackupSizeTotal 计算备份目录总大小
func GetBackupSizeTotal(mcDir string) int64 {
	backupRoot := filepath.Join(mcDir, BackupDirName)
	if _, err := os.Stat(backupRoot); os.IsNotExist(err) {
		return 0
	}
	return dirSize(backupRoot)
}

// ============================================================
// 文件操作辅助
// ============================================================

// copyDir 递归复制目录
// src: 源目录路径
// dst: 目标目录路径
// 返回: 复制的文件数、总大小、错误
func copyDir(src, dst string) (int, int64, error) {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return 0, 0, fmt.Errorf("创建目录 %s 失败: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, 0, fmt.Errorf("读取源目录 %s 失败: %w", src, err)
	}

	var totalFiles int
	var totalSize int64

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			files, size, err := copyDir(srcPath, dstPath)
			if err != nil {
				return totalFiles, totalSize, err
			}
			totalFiles += files
			totalSize += size
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				logger.Warn("复制文件 %s 失败: %v", srcPath, err)
				continue
			}
			totalFiles++
			if fi, statErr := os.Stat(srcPath); statErr == nil {
				totalSize += fi.Size()
			}
		}
	}

	return totalFiles, totalSize, nil
}

// copyFile 复制单个文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件 %s 失败: %w", src, err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("创建目标目录 %s 失败: %w", filepath.Dir(dst), err)
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件 %s 失败: %w", dst, err)
	}
	defer dstFile.Close()

	written, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("复制文件内容 %s -> %s 失败: %w", src, dst, err)
	}

	// 保留文件权限
	if fi, statErr := os.Stat(src); statErr == nil {
		_ = os.Chmod(dst, fi.Mode())
	}

	_ = written // 忽略写入字节数
	return nil
}

// dirSize 递归计算目录总大小
func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过访问错误
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
