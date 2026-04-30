//go:build !windows

package tray

import "github.com/gege-tlph/mc-starter/internal/repair"

// nonWindowsTrayManager 非 Windows 平台空实现
type nonWindowsTrayManager struct{}

// NewManager 创建托盘管理器（非 Windows stub）
func NewManager(cfgDir, mcDir string) Manager {
	return &nonWindowsTrayManager{}
}

func (m *nonWindowsTrayManager) Start() error { return nil }
func (m *nonWindowsTrayManager) Stop()        {}
func (m *nonWindowsTrayManager) SetStatus(text string) {}
func (m *nonWindowsTrayManager) NotifyCrash(ev repair.CrashEvent) {}
