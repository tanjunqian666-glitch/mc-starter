//go:build !windows

package repair

import "fmt"

// messageBoxWindows 非 Windows 平台 stub
func messageBoxWindows(msg string) bool {
	fmt.Printf("\n[崩溃检测] %s\n", msg)
	fmt.Println("[崩溃检测] 使用 `starter repair` 运行修复工具")
	return false
}
