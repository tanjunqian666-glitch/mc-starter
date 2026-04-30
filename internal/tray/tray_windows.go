//go:build windows

package tray

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/gege-tlph/mc-starter/internal/repair"
	"github.com/lxn/walk"
)

// WindowsTrayManager Windows 托盘实现（基于 lxn/walk）
type WindowsTrayManager struct {
	mu     sync.Mutex
	ni     *walk.NotifyIcon
	status string // 当前状态文本
	cfgDir string
	mcDir  string
	stopCh chan struct{}
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

	// walk.NewNotifyIcon 需要一个 Form 参数
	mw, err := walk.NewMainWindow()
	if err != nil {
		return fmt.Errorf("创建主窗口失败: %w", err)
	}
	mw.SetVisible(false)

	ni, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return fmt.Errorf("创建通知图标失败: %w", err)
	}

	m.ni = ni

	// 设置图标
	icon, err := walk.Resources.Icon("../img/app.ico")
	if err != nil {
		_ = ni.SetIcon(walk.IconApplication())
	} else {
		_ = ni.SetIcon(icon)
	}

	_ = ni.SetToolTip("MC-Starter")

	if err := m.buildContextMenu(); err != nil {
		return fmt.Errorf("构建右键菜单失败: %w", err)
	}

	_ = ni.SetVisible(true)

	// 左键双击打开目录（使用 MouseDown 事件，因为新版本 MessageClicked 不传按钮类型）
	_ = ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			openExplorer(m.mcDir)
		}
	})

	return nil
}

// buildContextMenu 构建右键菜单
func (m *WindowsTrayManager) buildContextMenu() error {
	menu := m.ni.ContextMenu()
	actions := menu.Actions()

	_ = actions.Add(m.newAction("立即更新", func() {
		m.runUpdate()
	}))

	_ = actions.Add(walk.NewSeparatorAction())

	// 修复子菜单
	repairMenu, err := walk.NewMenu()
	if err != nil {
		return fmt.Errorf("创建修复菜单失败: %w", err)
	}
	rmAction := walk.NewMenuAction(repairMenu)
	rmAction.SetText("修复")
	_ = actions.Add(rmAction)

	rmActions := repairMenu.Actions()
	_ = rmActions.Add(m.newAction("完全修复（清空+重下）", func() { m.runRepair("clean") }))
	_ = rmActions.Add(m.newAction("仅模组", func() { m.runRepair("mods-only") }))
	_ = rmActions.Add(m.newAction("仅配置", func() { m.runRepair("config-only") }))
	_ = rmActions.Add(walk.NewSeparatorAction())
	_ = rmActions.Add(m.newAction("回滚到上一版本", func() { m.runRepair("rollback") }))

	_ = actions.Add(walk.NewSeparatorAction())

	_ = actions.Add(m.newAction("打开 .minecraft", func() {
		openExplorer(m.mcDir)
	}))

	_ = actions.Add(walk.NewSeparatorAction())

	_ = actions.Add(m.newAction("退出", func() {
		m.Stop()
	}))

	return nil
}

// newAction 创建带文本和点击事件的 Action
func (m *WindowsTrayManager) newAction(text string, onClick func()) *walk.Action {
	a := walk.NewAction()
	a.SetText(text)
	a.Triggered().Attach(onClick)
	return a
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

	title := "MC-Starter - 崩溃检测"
	msg := ev.Reason
	if msg == "" {
		msg = "Minecraft 发生了崩溃"
	}
	// walk 新版 ShowCustom 需要 icon 参数，传 nil 用默认
	_ = m.ni.ShowCustom(title, msg, nil)

	m.status = "崩溃: " + ev.Reason
	_ = m.ni.SetToolTip("MC-Starter - " + m.status)
}

// runUpdate 执行更新
func (m *WindowsTrayManager) runUpdate() {
	runStarter(m.cfgDir, "update")
}

// runRepair 执行修复
func (m *WindowsTrayManager) runRepair(action string) {
	args := []string{"repair"}
	switch action {
	case "clean":
		args = append(args, "--clean")
	case "mods-only":
		args = append(args, "--mods-only")
	case "config-only":
		args = append(args, "--config-only")
	case "rollback":
		args = append(args, "--rollback")
	}
	args = append(args, "--headless", "--config", m.cfgDir)
	runStarter(m.cfgDir, args...)
}

// runStarter 启动 starter 子进程
func runStarter(cfgDir string, args ...string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, args...)
	_ = cmd.Start()
}

// openExplorer 打开资源管理器
func openExplorer(path string) {
	_ = exec.Command("explorer", path).Start()
}
