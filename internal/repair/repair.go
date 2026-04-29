package repair

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P2.7 修复命令 — 修复管理器
//
// 职责：
//   1. 修复前自动创建备份
//   2. 按配置清理 mods/config/resourcepacks/saves
//   3. 引导用户进行后续同步
//   4. 支持回滚到备份
//
// 设计原则：
//   - 修复 = 把客户端状态"擦"回服务端定义的纯净状态
//   - 备份 = 擦之前保存用户所有自定义内容
//   - 每次修复前自动创建备份（防手滑）
// ============================================================

// RepairAction 修复操作类型
type RepairAction string

const (
	ActionCleanAll        RepairAction = "clean"          // 完全修复
	ActionModsOnly        RepairAction = "mods-only"      // 仅模组
	ActionConfigOnly      RepairAction = "config-only"    // 仅配置
	ActionLoaderOnly      RepairAction = "loader-only"    // 仅 Loader
	ActionRollback        RepairAction = "rollback"       // 回滚
	ActionListBackups     RepairAction = "list-backups"   // 列出备份
	ActionInteractive     RepairAction = ""               // 空 = 交互模式
)

// RepairConfig 修复配置
type RepairConfig struct {
	Action        RepairAction // 修复操作
	RollbackID    string       // 回滚到指定备份 ID（空=最新）
	MCVersion     string       // MC 版本（loader 安装用）
	LoaderVersion string       // Loader 版本
}

// RepairResult 修复结果
type RepairResult struct {
	Action      RepairAction `json:"action"`
	BackupDir   string       `json:"backup_dir,omitempty"`
	CleanedDirs []string     `json:"cleaned_dirs,omitempty"`
	Restored    int          `json:"restored,omitempty"`
	Errors      []string     `json:"errors,omitempty"`
}

// Repair 执行修复
// mcDir: .minecraft 根目录
// cfg: 修复配置
// 返回: 修复结果
func Repair(mcDir string, cfg RepairConfig) (*RepairResult, error) {
	result := &RepairResult{
		Action: cfg.Action,
	}

	if cfg.Action == ActionRollback {
		return rollback(mcDir, cfg.RollbackID)
	}

	if cfg.Action == ActionListBackups {
		backups, err := ListBackups(mcDir)
		if err != nil {
			return nil, fmt.Errorf("列出备份失败: %w", err)
		}
		if len(backups) == 0 {
			fmt.Println("没有可用的备份")
			return result, nil
		}
		fmt.Printf("可用备份 (%d 个):\n", len(backups))
		for i, b := range backups {
			fmt.Printf("  %d. %s (%s) — 原因=%s, %d 个文件\n",
				i+1, b.ID, b.CreatedAt.Format("2006-01-02 15:04:05"),
				b.Reason, b.FileCount)
		}
		return result, nil
	}

	// Phase 0: 修复前备份
	logger.Info("修复阶段 0: 创建备份...")
	backupOpts := BackupOptions{
		Reason: ReasonRepair,
	}
	backupResult, err := CreateBackup(mcDir, backupOpts)
	if err != nil {
		logger.Warn("创建备份失败: %v", err)
		result.Errors = append(result.Errors, fmt.Sprintf("备份失败: %v", err))
	} else {
		result.BackupDir = backupResult.BackupDir
		logger.Info("备份完成: %s (%d 个文件)", backupResult.BackupDir, backupResult.FileCount)
	}

	// Phase 1: 清理
	cleanMods := cfg.Action == ActionCleanAll || cfg.Action == ActionModsOnly || cfg.Action == ""
	cleanConfig := cfg.Action == ActionCleanAll || cfg.Action == ActionConfigOnly || cfg.Action == ""
	cleanAll := cfg.Action == ActionCleanAll

	// 清理 mods/
	if cleanMods || cleanAll {
		modsDir := filepath.Join(mcDir, "mods")
		if err := cleanDir(modsDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("清理 mods/ 失败: %v", err))
		} else {
			result.CleanedDirs = append(result.CleanedDirs, "mods")
		}
	}

	// 清理 config/
	if cleanConfig || cleanAll {
		configDir := filepath.Join(mcDir, "config")
		if err := cleanDir(configDir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("清理 config/ 失败: %v", err))
		} else {
			result.CleanedDirs = append(result.CleanedDirs, "config")
		}
	}

	// 全量清理 resourcepacks/ 和 shaderpacks/
	if cleanAll {
		for _, d := range []string{"resourcepacks", "shaderpacks"} {
			dir := filepath.Join(mcDir, d)
			if err := cleanDir(dir); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("清理 %s/ 失败: %v", d, err))
			} else {
				result.CleanedDirs = append(result.CleanedDirs, d)
			}
		}
	}

	return result, nil
}

// rollback 回滚到指定备份
func rollback(mcDir string, backupID string) (*RepairResult, error) {
	// 未指定备份 ID 时自动选最新的
	backups, err := ListBackups(mcDir)
	if err != nil {
		return nil, fmt.Errorf("列出备份失败: %w", err)
	}
	if len(backups) == 0 {
		fmt.Println("没有可用的备份可供回滚")
		return &RepairResult{Action: ActionRollback}, nil
	}

	targetID := backupID
	if targetID == "" {
		targetID = backups[0].ID
		logger.Info("未指定备份 ID，使用最新备份: %s", targetID)
	}

	// 查找匹配的备份
	found := false
	for _, b := range backups {
		if b.ID == targetID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("备份 %s 不存在", targetID)
	}

	restored, err := RestoreBackup(mcDir, targetID)
	if err != nil {
		return nil, fmt.Errorf("回滚失败: %w", err)
	}

	return &RepairResult{
		Action:   ActionRollback,
		Restored: restored,
	}, nil
}

// cleanDir 清空目录并重建（幂等）
func cleanDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 目录不存在则直接创建
		return os.MkdirAll(dir, 0755)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("删除 %s 失败: %w", dir, err)
	}
	return os.MkdirAll(dir, 0755)
}

// IsRepairDir 检查目录是否是需要修的内容
func IsRepairDir(name string) bool {
	switch name {
	case "mods", "config", "resourcepacks", "shaderpacks":
		return true
	}
	return false
}
