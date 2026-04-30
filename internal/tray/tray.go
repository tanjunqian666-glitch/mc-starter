// Package tray — 系统托盘图标
//
// P2.13 托盘菜单入口：daemon 模式下最小化到系统托盘
// 右键菜单：更新/修复/查看状态/退出
//
// Windows：使用 lxn/walk 的 NotifyIcon
// 其他平台：无操作 stub

package tray

import "github.com/gege-tlph/mc-starter/internal/repair"

// Manager 托盘管理接口
type Manager interface {
	// Start 启动托盘
	Start() error
	// Stop 停止托盘
	Stop()
	// SetStatus 更新状态文本
	SetStatus(text string)
	// NotifyCrash 崩溃通知
	NotifyCrash(ev repair.CrashEvent)
}
