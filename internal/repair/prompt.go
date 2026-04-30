// Package repair — mc-starter 修复器
//
// P2.14 崩溃弹窗询问 — 检测到托管版本崩溃时询问用户是否打开修复工具

package repair

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PromptCrashRepair 检测到崩溃后，询问用户是否打开修复工具
// 返回 true 表示用户要修复，(launched, error) — launched 表示修复进程已启动
// 在 headless/无 GUI 环境直接返回 (false, nil)
func PromptCrashRepair(ev CrashEvent, startupArgs []string) (launched bool, err error) {
	msg := buildMessage(ev)

	if !messageBoxWindows(msg) {
		return false, nil // 用户点"否"
	}

	// 用户点"是" — 启动修复工具
	if err := launchRepairTool(startupArgs); err != nil {
		return false, fmt.Errorf("启动修复工具失败: %w", err)
	}

	return true, nil
}

// launchRepairTool 启动修复工具自身（starter repair）
func launchRepairTool(startupArgs []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位自身路径: %w", err)
	}

	repairArgs := []string{"repair"}
	repairArgs = append(repairArgs, startupArgs...)

	cmd := exec.Command(exe, repairArgs...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec.Start: %w", err)
	}
	// 分离子进程，不等待
	return nil
}

// buildMessage 构建弹窗消息文本
func buildMessage(ev CrashEvent) string {
	var lines []string
	lines = append(lines, "检测到更新器托管的 Minecraft 版本发生了崩溃。")
	if ev.Reason != "" {
		lines = append(lines, "")
		lines = append(lines, "崩溃原因: "+ev.Reason)
	}
	lines = append(lines, "")
	lines = append(lines, "是否打开修复工具？")
	return strings.Join(lines, "\n")
}
