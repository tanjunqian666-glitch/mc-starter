//go:build windows

package repair

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// user32.dll 的 MessageBoxW
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
	procGetActiveWin = user32.NewProc("GetActiveWindow")
	// 使用 GetDesktopWindow 作为父窗口（没有主窗口时也能弹）
	procGetDesktopWin = user32.NewProc("GetDesktopWindow")
)

// MessageBox 按钮类型
const (
	mbOKCancel = 1
	mbYesNo    = 4
	mbIconStop = 16
	mbIconInfo = 64
)

// MessageBox 返回值
const (
	idYes    = 6
	idNo     = 7
	idOK     = 1
	idCancel = 2
)

// messageBoxWindows 弹 Windows 原生 MessageBox，询问用户是否修复
// 返回 true = 用户点击"是"
func messageBoxWindows(msg string) bool {
	// 窗口标题
	title := "MC Starter - 崩溃检测"

	// 编码为 UTF-16
	uTitle := utf16.Encode([]rune(title + "\x00"))
	uMsg := utf16.Encode([]rune(msg + "\x00"))

	// 父窗口: 用桌面窗口（确保一定能弹出来）
	hwnd, _, _ := procGetDesktopWin.Call()

	// 按钮: 是(Y) / 否(N) + 感叹号图标
	// MB_YESNO (4) | MB_ICONWARNING (0x30) | MB_SYSTEMMODAL (0x1000)
	uType := uintptr(4 | 0x30 | 0x1000)

	ret, _, _ := procMessageBoxW.Call(
		hwnd,
		uintptr(unsafe.Pointer(&uMsg[0])),
		uintptr(unsafe.Pointer(&uTitle[0])),
		uType,
	)

	return ret == idYes
}
