//go:build windows

package tray

import (
	"fmt"
	"sync"

	"github.com/gege-tlph/mc-starter/internal/repair"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// WindowsTrayManager Windows 托盘实现（基于 lxn/walk）
type WindowsTrayManager struct {
	mu       sync.Mutex
	ni       *walk.NotifyIcon
	status   string // 当前状态文本
	cfgDir   string
	mcDir    string
	stopCh   chan struct{}
}

// NewManager 创建托盘管理器
func NewManager(cfgDir, mcDir string) Manager {
	return &WindowsTrayManager{
		cfgDir: cfgDir,
		mcDir:  mcDir,
		stopCh: make(chan struct{}),
	}
}

// Start 启动托盘图标
func (m *WindowsTrayManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ni != nil {
		return nil // 已启动
	}

	// 创建 NotifyIcon
	ni, err := walk.NewNotifyIcon()
	if err != nil {
		return fmt.Errorf("创建通知图标失败: %w", err)
	}

	m.ni = ni

	// 设置图标（使用默认应用图标）
	icon, err := walk.Resources.Icon("../img/app.ico")
	if err != nil {
		// 没有自定义图标时用系统图标
		_ = m.ni.SetIcon(win.LoadIcon(0, win.MakeIntResource(win.IDI_APPLICATION)))
	} else {
		_ = m.ni.SetIcon(icon)
	}

	// 设置悬停提示
	_ = m.ni.SetToolTip("MC-Starter")

	// 创建右键菜单
	_ = m.ni.ContextMenu().Actions().Add(walk.NewAction("立即更新", func() {
		m.runUpdate()
	}))

	_ = m.ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())

	// 修复子菜单
	repairMenu := walk.NewMenu()
	_ = m.ni.ContextMenu().Actions().Add(walk.NewMenuAction(repairMenu, "修复"))
	repairMenu.Actions().Add(walk.NewAction("完全修复（清空+重下）", func() {
		m.runRepair("clean")
	}))
	repairMenu.Actions().Add(walk.NewAction("仅模组", func() {
		m.runRepair("mods-only")
	}))
	repairMenu.Actions().Add(walk.NewAction("仅配置", func() {
		m.runRepair("config-only")
	}))
	repairMenu.Actions().Add(walk.NewSeparatorAction())
	repairMenu.Actions().Add(walk.NewAction("回滚到上一版本", func() {
		m.runRepair("rollback")
	}))

	_ = m.ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())

	// 打开游戏目录
	_ = m.ni.ContextMenu().Actions().Add(walk.NewAction("打开 .minecraft", func() {
		walk.Run("explorer", m.mcDir)
	}))

	_ = m.ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())

	// 退出
	_ = m.ni.ContextMenu().Actions().Add(walk.NewAction("退出", func() {
		m.Stop()
	}))

	// 设置可见
	_ = m.ni.SetVisible(true)

	// 左键双击恢复（目前无窗口可恢复，当作打开 .minecraft）
	_ = m.ni.MessageClicked().Attach(func(btn walk.NotifyIconMessageButton) {
		if btn == walk.NotifyIconButtonLeft {
			walk.Run("explorer", m.mcDir)
		}
	})

	return nil
}

// Stop 停止托盘
func (m *WindowsTrayManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ni == nil {
		return
	}

	_ = m.ni.SetVisible(false)
	m.ni.Dispose()
	m.ni = nil

	close(m.stopCh)
}

// SetStatus 更新悬停提示状态
func (m *WindowsTrayManager) SetStatus(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = text
	if m.ni != nil {
		_ = m.ni.SetToolTip("MC-Starter - " + text)
	}
}

// NotifyCrash 崩溃通知
func (m *WindowsTrayManager) NotifyCrash(ev repair.CrashEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ni == nil {
		return
	}

	// 弹气泡通知
	title := "MC-Starter - 崩溃检测"
	msg := ev.Reason
	if msg == "" {
		msg = "Minecraft 发生了崩溃"
	}
	_ = m.ni.ShowCustom(title, msg)

	// 更新状态
	m.status = "崩溃: " + ev.Reason
	_ = m.ni.SetToolTip("MC-Starter - " + m.status)
}

// runUpdate 执行更新
func (m *WindowsTrayManager) runUpdate() {
	// 通过启动子进程执行，避免阻塞托盘
	exe, _ := walk.Executable()
	_ = walk.Run(exe, "update", "--config", m.cfgDir)
}

// runRepair 执行修复
func (m *WindowsTrayManager) runRepair(action string) {
	exe, _ := walk.Executable()
	switch action {
	case "clean":
		_ = walk.Run(exe, "repair", "--clean", "--headless", "--config", m.cfgDir)
	case "mods-only":
		_ = walk.Run(exe, "repair", "--mods-only", "--headless", "--config", m.cfgDir)
	case "config-only":
		_ = walk.Run(exe, "repair", "--config-only", "--headless", "--config", m.cfgDir)
	case "rollback":
		_ = walk.Run(exe, "repair", "--rollback", "--config", m.cfgDir)
	}
}
