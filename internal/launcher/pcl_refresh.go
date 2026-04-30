// Package launcher — PCL2 刷新
//
// P2.15 修复后 PCL2 刷新：确保修复/更新后 PCL2 能看到最新版本

package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RefreshPCL2AfterRepair 修复后刷新 PCL2 配置
// 确保 PCL2 能检测到已更新/修复的版本
// mcDir: .minecraft 目录
// versionName: 版本名称（如 "fabric-loader-0.15.11-1.20.4"）
//
// 操作：
//   1. 写 launcher_profiles.json 标记版本可用
//   2. 如果有 PCL.ini，刷新其中的版本列表缓存
func RefreshPCL2AfterRepair(mcDir, versionName string) error {
	var lastErr error

	// 1. 写 launcher_profiles.json
	if err := refreshProfilesJSON(mcDir, versionName); err != nil {
		lastErr = err
	}

	// 2. 尝试刷新 PCL.ini 缓存（如果有）
	if pcl := FindPCL2(); pcl != nil && pcl.PCLIniExists() {
		if err := refreshPCLIniCache(pcl); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// refreshProfilesJSON 写 launcher_profiles.json 标记版本
// 这是 Mojang 启动器和 PCL2 都会读取的配置文件
func refreshProfilesJSON(mcDir, versionName string) error {
	profilesPath := filepath.Join(mcDir, "launcher_profiles.json")

	// 尝试读取现有配置
	var profiles map[string]interface{}
	data, err := os.ReadFile(profilesPath)
	if err == nil {
		json.Unmarshal(data, &profiles)
	}

	// 初始化结构
	if profiles == nil {
		profiles = make(map[string]interface{})
	}

	// 确保 profiles 字段存在
	if _, ok := profiles["profiles"]; !ok {
		profiles["profiles"] = make(map[string]interface{})
	}
	profs, _ := profiles["profiles"].(map[string]interface{})
	if profs == nil {
		profs = make(map[string]interface{})
		profiles["profiles"] = profs
	}

	// 确保版本存在于 profiles 中
	if _, ok := profs[versionName]; !ok {
		profs[versionName] = map[string]interface{}{
			"name":         versionName,
			"type":         "custom",
			"created":      time.Now().UTC().Format(time.RFC3339),
			"lastUsed":     time.Now().UTC().Format(time.RFC3339),
			"icon":         "Furnace",
			"lastVersionId": versionName,
		}
	}

	// 写回文件
	data, err = json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 launcher_profiles.json 失败: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(profilesPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpPath := profilesPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}

	return os.Rename(tmpPath, profilesPath)
}

// refreshPCLIniCache 刷新 PCL.ini 的版本缓存
// PCL2 会缓存版本列表，不刷新可能导致检测不到新安装的版本
func refreshPCLIniCache(pcl *PCLDetection) error {
	if pcl.PCLIniPath == "" {
		return nil
	}

	data, err := os.ReadFile(pcl.PCLIniPath)
	if err != nil {
		return fmt.Errorf("读取 PCL.ini 失败: %w", err)
	}

	content := string(data)

	// 删除版本缓存相关标记（PCL2 会在下次启动时重新扫描 versions/ 目录）
	// VersionExcludes 和 CustomVersion 等缓存标记
	modified := false

	// 移除 VersionRefreshTime（强制下次启动刷新）
	lines := make([]string, 0)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// 跳过版本缓存相关行
		if strings.HasPrefix(strings.ToLower(trimmed), "versionrefreshtime=") {
			modified = true
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "versionexcludes=") {
			modified = true
			continue
		}
		lines = append(lines, line)
	}

	if modified {
		newContent := strings.Join(lines, "\n")
		tmpPath := pcl.PCLIniPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("写 PCL.ini 临时文件失败: %w", err)
		}
		return os.Rename(tmpPath, pcl.PCLIniPath)
	}

	return nil
}
