// Package gui — mc-starter Windows GUI (Walk)
//
// G.5 简化版 app.go — 只负责 UI 布局和控件绑定
// 数据从 ViewModel 来，操作调 Orchestrator
//
// 核心流程：
//   首次打开 → 配置向导
//   日常使用 → 选版本 → 更新按钮状态自动跟随
//   有更新   → 点更新 → Orchestrator 调度 → EventBus 事件驱动 UI 刷新
//
// 小工具风格：固定大小窗口，无缩放

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// App 全局 GUI 应用状态
// 控件字段直接用 Walk 绑定，数据走 ViewModel，操作走 Orchestrator
type App struct {
	mu sync.Mutex // settings.go/setup.go 中 Lock/Unlock 使用

	cfgDir string

	// 三层（初始化后不可变）
	vm  *ViewModel
	eb  *EventBus
	orc *Orchestrator

	// Walk 控件引用（运行时绑定）
	mw        *walk.MainWindow
	packCB    *walk.ComboBox
	openBtn   *walk.PushButton
	updateBtn *walk.PushButton
	statusBar *walk.Label
	verBar    *walk.Label // 版本状态栏
	progress  *walk.ProgressBar
	progressLabel *walk.Label
	cancelBtn *walk.PushButton

	// 数据（仅供 settings.go / setup.go 兼容，后续可移除）
	cfg      *config.Manager
	localCfg *model.LocalConfig
}

// Run 启动 GUI
// 首次运行：先建主窗口 → mw.Run()（此时消息循环可用）
//   → 在 Starting 事件中弹向导（模态，阻塞主窗口直到完成）
// 再次运行：直接显示
func Run(cfgDir string) error {
	// 1. 初始化三层
	vm := NewViewModel(cfgDir)
	eb := NewEventBus(64)
	vm.SetEventBus(eb)

	isFirstRun := vm.Init()

	orc := NewOrchestrator(cfgDir, vm, eb)

	app := &App{
		cfgDir:   cfgDir,
		vm:       vm,
		eb:       eb,
		orc:      orc,
		cfg:      config.New(cfgDir),
		localCfg: vm.LocalConfig(),
	}

	debugLog("Run(cfgDir=%s) isFirstRun=%v", cfgDir, isFirstRun)

	// 2. 启动 EventBus 分发
	go eb.Run()

	// 3. 构建主窗口
	debugLog("buildUI start")
	app.buildUI()
	debugLog("buildUI done")

	// 4. 首次运行：在 mw.Starting 事件里弹向导
	//    mw.Run() 启动消息循环后立即触发 Starting
	//    向导作为模态 Dialog 在消息循环中运行
	if isFirstRun {
		app.mw.Starting().Attach(func() {
			if err := runSetupWizard(app); err != nil {
				// 向导取消/未完成 → 关闭主窗口退出
				app.mw.Close()
				return
			}
			// 向导完成 → 重新加载配置并刷新 UI
			vm.reloadConfig()
			// 向导后才拉服务端列表，需要重新刷新
			vm.DetermineInitialPack()
			app.refreshUI()
		})
	}

	// 5. 初始选中版本 + UI 刷新（非首次直接生效，首次等待 Starting 中向导完成）
	vm.DetermineInitialPack()
	app.refreshUI()
	debugLog("refreshUI done")

	// 6. 启动 EventBus 事件循环
	go app.eventLoop()

	debugLog("about to call mw.Run()")
	app.mw.Run()
	debugLog("mw.Run() returned — GUI exiting")
	return nil
}

// buildUI 构建主窗口布局
func (a *App) buildUI() {
	mw := new(walk.MainWindow)

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "MC Starter",
		MinSize:  Size{380, 230},
		MaxSize:  Size{380, 230},
		Size:     Size{380, 230},
		Layout:   VBox{MarginsZero: false, Margins: Margins{10, 10, 10, 10}},
		Children: []Widget{
			// 标题栏 + 设置按钮
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "MC Starter", Font: Font{PointSize: 11, Bold: true}},
					HSpacer{},
					PushButton{
						Text:     "⚙",
						MinSize:  Size{28, 0},
						MaxSize:  Size{28, 0},
						OnClicked: func() {
							a.openSettings()
						},
					},
				},
			},
			// 版本选择下拉
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "版本:"},
					ComboBox{
						AssignTo: &a.packCB,
						Model:    make([]string, 0),
						OnCurrentIndexChanged: func() {
							a.onPackSelected()
						},
					},
				},
			},
			// 操作按钮行
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						AssignTo: &a.openBtn,
						Text:     "📂 打开启动器",
						MinSize:  Size{140, 0},
						OnClicked: func() {
							a.openLauncher()
						},
					},
					PushButton{
						AssignTo: &a.updateBtn,
						Text:     "📥 安装",
						MinSize:  Size{100, 0},
						Enabled:  false,
						OnClicked: func() {
							a.orc.UpdateOrInstall()
						},
					},
				},
			},
			// 进度条行
			Composite{
				Layout: HBox{},
				Visible: false,
				Children: []Widget{
					ProgressBar{
						AssignTo: &a.progress,
						MinSize:  Size{200, 0},
					},
					Label{
						AssignTo: &a.progressLabel,
						MinSize:  Size{60, 0},
					},
					PushButton{
						AssignTo: &a.cancelBtn,
						Text:     "取消",
						OnClicked: func() {
							a.orc.Cancel()
							a.cancelBtn.SetEnabled(false)
						},
					},
				},
			},
			// 版本状态栏
			Label{
				AssignTo: &a.verBar,
				Text:     "",
				Font:     Font{PointSize: 9},
				TextColor: walk.RGB(100, 100, 100),
			},
			// 状态
			Label{
				AssignTo: &a.statusBar,
				Text:     "就绪",
				TextColor: walk.RGB(0, 170, 0),
			},
		},
	}.Create()); err != nil {
		panic(err)
	}
	a.mw = mw // AssignTo 更新了局部指针，这里同步到 App 字段
}

// ============================================================
// 事件循环（EventBus 驱动 UI 刷新）
// ============================================================

func (a *App) eventLoop() {
	ch := a.eb.Subscribe()
	defer a.eb.Unsubscribe(ch)

	for evt := range ch {
		switch evt.Type {
		case EvtProgress:
			if data, ok := evt.Data.(ProgressData); ok {
				a.mw.Synchronize(func() {
					a.progress.SetValue(data.Percent)
					a.progressLabel.SetText(fmt.Sprintf("%d%%", data.Percent))
				})
			}

		case EvtStateChange:
			a.mw.Synchronize(func() {
				a.refreshUI()
			})

		case EvtError:
			if data, ok := evt.Data.(ErrorData); ok {
				a.mw.Synchronize(func() {
					a.statusBar.SetTextColor(walk.RGB(200, 50, 50))
					a.statusBar.SetText(fmt.Sprintf("错误: %s", data.Message))
				})
			}

		case EvtLog:
			// 日志暂不显示在 UI 上

		case EvtSyncDone:
			if data, ok := evt.Data.(SyncDoneData); ok {
				a.mw.Synchronize(func() {
					if data.Err != nil {
						walk.MsgBox(a.mw, "同步失败",
							fmt.Sprintf("%s 同步失败: %v", data.PackName, data.Err), walk.MsgBoxOK)
					} else {
						walk.MsgBox(a.mw, "完成",
							fmt.Sprintf("%s 已更新到 %s", data.PackName, data.NewVersion), walk.MsgBoxOK)
					}
					a.refreshUI()
				})
			}

		case EvtPackList:
			a.mw.Synchronize(func() {
				a.refreshUI()
			})
		}
	}
}

// ============================================================
// UI 刷新（根据 ViewModel 状态更新控件）
// ============================================================

// refreshUI 刷新所有 Walk 控件状态
func (a *App) refreshUI() {
	// 确保当前选中的版本未被禁用
	a.ensureValidSelection()

	status := a.vm.CurrentPackStatus()

	// 下拉列表
	if a.packCB != nil {
		names := a.vm.PackNames()
		a.packCB.SetModel(names)

		// 保持选中项一致
		a.syncPackCBSelection()
	}

	// 版本状态栏
	if a.verBar != nil {
		a.verBar.SetText(a.vm.VersionBarText())
	}

	// 更新按钮
	if a.updateBtn != nil {
		a.updateBtn.SetText(status.UpdateBtnText)
		a.updateBtn.SetEnabled(status.UpdateEnabled)
	}

	// 打开启动器按钮
	if a.openBtn != nil {
		a.openBtn.SetEnabled(!a.vm.StateMachine().IsBusy())
	}

	// 状态栏
	if a.statusBar != nil {
		a.statusBar.SetText(status.StatusText)
		switch status.StatusColor {
		case StatusGreen:
			a.statusBar.SetTextColor(walk.RGB(0, 170, 0))
		case StatusOrange:
			a.statusBar.SetTextColor(walk.RGB(200, 150, 0))
		case StatusRed:
			a.statusBar.SetTextColor(walk.RGB(200, 50, 50))
		default:
			a.statusBar.SetTextColor(walk.RGB(100, 100, 100))
		}
	}

	// 进度行可见性
	busy := a.vm.StateMachine().IsBusy()

	// 取消按钮状态
	if a.cancelBtn != nil {
		a.cancelBtn.SetEnabled(busy)
	}
}

// ============================================================
// 版本选择
// ============================================================

// ensureValidSelection 确保当前选中版本未被禁用
// 如果被禁用，自动选主版本或第一个可用版本
func (a *App) ensureValidSelection() {
	selected := a.vm.SelectedPack()
	packs := a.vm.ServerPacks()

	// 没选中任何版本
	if selected == "" {
		a.vm.DetermineInitialPack()
		return
	}

	// 检查选中版本是否存在且未被禁用
	valid := false
	for _, p := range packs {
		if p.Name == selected {
			valid = true
			break
		}
	}
	if valid {
		return
	}

	// 选中版本不可用（可能被禁用了），自动切换到主版本
	a.vm.DetermineInitialPack()
}

// syncPackCBSelection 同步下拉框选中索引与 ViewModel 的 selectedPack
func (a *App) syncPackCBSelection() {
	if a.packCB == nil {
		return
	}
	selected := a.vm.SelectedPack()
	packs := a.vm.ServerPacksFiltered()
	for i, p := range packs {
		if p.Name == selected {
			if a.packCB.CurrentIndex() != i {
				a.packCB.SetCurrentIndex(i)
			}
			return
		}
	}
	// 没找到对应项，选中第一个
	if len(packs) > 0 {
		a.packCB.SetCurrentIndex(0)
	}
}

func (a *App) onPackSelected() {
	if a.packCB == nil {
		return
	}
	idx := a.packCB.CurrentIndex()
	packs := a.vm.ServerPacksFiltered()
	if idx < 0 || idx >= len(packs) {
		return
	}

	p := packs[idx]
	a.vm.SelectPack(p.Name)
	a.refreshUI()
}

// ============================================================
// 操作
// ============================================================

func (a *App) openSettings() {
	showSettings(a)
}

func (a *App) openLauncher() {
	if a.vm.StateMachine().IsBusy() {
		return
	}
	packName := a.vm.SelectedPack()
	if packName == "" {
		return
	}

	status := a.vm.CurrentPackStatus()
	if !status.IsInstalled {
		walk.MsgBox(a.mw, "提示", "请先点击安装此版本", walk.MsgBoxOK)
		return
	}

	localCfg := a.vm.LocalConfig()
	if localCfg.Launcher == "" {
		walk.MsgBox(a.mw, "提示", "未配置启动器路径，请点击右上角「⚙」进行设置", walk.MsgBoxOK)
		return
	}

	// 通过 Orchestrator 校验路径，实际启动
	if err := a.orc.OpenLauncher(); err != nil {
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("打开启动器失败: %v", err), walk.MsgBoxOK)
		return
	}

	// 执行实际启动
	if err := openLauncherExternal(localCfg.Launcher); err != nil {
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("启动启动器失败: %v", err), walk.MsgBoxOK)
	}
}

// openLauncherExternal 启动外部启动器（不阻塞）
func openLauncherExternal(launcherPath string) error {
	if launcherPath == "" {
		return fmt.Errorf("启动器路径为空")
	}
	if _, err := os.Stat(launcherPath); os.IsNotExist(err) {
		return fmt.Errorf("启动器文件不存在: %s", launcherPath)
	}
	cmd := exec.Command(launcherPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动启动器失败: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ============================================================
// 设置/向导依赖的兼容方法
// settings.go 和 setup.go 还需要这些
// ============================================================

// refreshServerPacks 刷新服务端版本列表（设置保存后调用）
func (a *App) refreshServerPacks() {
	a.vm.RefreshPacks()
}

// determineInitialPack 确定初始选中版本（设置保存后调用）
func (a *App) determineInitialPack() {
	a.vm.DetermineInitialPack()
}

// RunGUI 兼容性入口
func RunGUI(cfgDir string) error {
	return Run(cfgDir)
}
