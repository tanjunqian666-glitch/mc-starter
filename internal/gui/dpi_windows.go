//go:build windows

package gui

import "syscall"

// init 在进程启动时告诉 Windows 此进程支持 DPI 感知，
// 避免系统对窗口进行位图缩放导致字体模糊。
// Shcore.dll 的 SetProcessDpiAwareness 比 SetProcessDPIAware 更精确，
// 支持 PROCESS_PER_MONITOR_DPI_AWARE (2)。
func init() {
	shcore := syscall.NewLazyDLL("shcore.dll")
	proc := shcore.NewProc("SetProcessDpiAwareness")
	// PROCESS_PER_MONITOR_DPI_AWARE = 2
	proc.Call(2)
}
