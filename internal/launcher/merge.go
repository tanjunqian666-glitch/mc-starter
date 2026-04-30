package launcher

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// packs/ → versions/ 合并逻辑
//
// 功能：把整合包 pack 中的 mods/config/resourcepacks 等合并到
// MC 版本目录（versions/<name>/），使得 PCL2/HMCL 启动器能识别
// 该版本为一个完整的整合包版本。
// ============================================================

// MergePackToVersion 把指定包的内容合并到 versions/<name>/ 目录
//
// packDir:     .minecraft/packs/<pack-name>/
// versionDir:  .minecraft/versions/<version-name>/
// dryRun:      仅打印不操作
//
// 合并策略：
//   - 从 packs/<name>/ 复制到 versions/<version-name>/ 的子目录
//   - 不删除 version 目录下已有的文件（pack 为增量叠加）
//   - mods/ 同名覆盖（pack 版本优先）
//   - 返回被合并的文件数和错误列表（非致命）
func MergePackToVersion(packDir, versionDir string, dryRun bool) (merged int, errs []error) {
	if _, err := os.Stat(packDir); os.IsNotExist(err) {
		return 0, []error{fmt.Errorf("包目录不存在: %s", packDir)}
	}

	// 要合并的子目录列表
	subDirs := []string{"mods", "config", "resourcepacks", "shaderpacks", "scripts"}

	for _, sub := range subDirs {
		srcDir := filepath.Join(packDir, sub)
		srcInfo, err := os.Stat(srcDir)
		if os.IsNotExist(err) {
			continue // 包中不包含此目录，跳过
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("检查 %s 失败: %w", srcDir, err))
			continue
		}
		if !srcInfo.IsDir() {
			continue
		}

		dstDir := filepath.Join(versionDir, sub)

		if dryRun {
			fmt.Printf("[DRY-RUN] 合并 %s → %s\n", srcDir, dstDir)
			// 计数
			entries, _ := os.ReadDir(srcDir)
			merged += len(entries)
			continue
		}

		// 创建目标目录
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			errs = append(errs, fmt.Errorf("创建目录 %s 失败: %w", dstDir, err))
			continue
		}

		// 复制文件
		entries, readErr := os.ReadDir(srcDir)
		if readErr != nil {
			errs = append(errs, fmt.Errorf("读取 %s 失败: %w", srcDir, readErr))
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue // 暂不递归处理子目录
			}
			srcFile := filepath.Join(srcDir, entry.Name())
			dstFile := filepath.Join(dstDir, entry.Name())

			// 检查是否已存在同名文件（已存在则覆盖，pack 版本优先）
			if existing, err := os.Stat(dstFile); err == nil {
				if existing.Size() == 0 {
					// 空文件也可能是先占位，覆盖
				}
			}

			data, readErr := os.ReadFile(srcFile)
			if readErr != nil {
				errs = append(errs, fmt.Errorf("读取 %s 失败: %w", srcFile, readErr))
				continue
			}
			if writeErr := os.WriteFile(dstFile, data, 0644); writeErr != nil {
				errs = append(errs, fmt.Errorf("写入 %s 失败: %w", dstFile, writeErr))
				continue
			}
			merged++
			logger.Debug("[Merge] %s → %s", srcFile, dstFile)
		}
	}

	return merged, errs
}

// MergeAllPacksToVersion 把多个包合并到一个版本目录
// 按 packs 列表顺序合并，后面的包覆盖前面的同名文件
func MergeAllPacksToVersion(packDirs []string, versionDir string, dryRun bool) (totalMerged int, allErrs []error) {
	for _, packDir := range packDirs {
		n, errs := MergePackToVersion(packDir, versionDir, dryRun)
		totalMerged += n
		allErrs = append(allErrs, errs...)
	}
	return totalMerged, allErrs
}
