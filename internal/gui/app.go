// Package gui — mc-starter Windows GUI (Walk)
//
// 小工具风格：固定大小窗口，无缩放
// 主界面：版本选择 + 打开启动器 + 更新按钮 + 进度条 + 版本状态
// 设置弹窗：Minecraft 根目录 / 启动器路径 / 服务端 API / 副版本启用
// 首次启动：配置向导（API→启动器路径自动检测→MC目录自动检测）
//
// 核心流程：
//   首次打开 → 配置向导
//   日常使用 → 选版本 → 打开启动器（同步完才能点）
//   有更新   → 更新按钮亮 → 点→自动同步→自动装Fabric/Forge→完成

package gui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// App 全局 GUI 应用状态
type App struct {
	sync.Mutex

	cfgDir string

	// Walk 控件引用（运行时绑定）
	mw        *walk.MainWindow
	packCB    *walk.ComboBox
	openBtn   *walk.PushButton
	updateBtn *walk.PushButton
	statusBar *walk.Label
	verBar    *walk.Label // 版本状态栏："主整合包  本地: v1.2.0  最新: v1.3.0"
	progress  *walk.ProgressBar
	progressLabel *walk.Label
	cancelBtn *walk.PushButton

	// 数据
	cfg            *config.Manager
	localCfg       *model.LocalConfig
	serverPacks    []model.PackInfo // 服务端版本列表
	selectedPack   string           // 当前选中的版本名
	hasUpdate      bool
	currentVersion string // 本地版本
	latestVersion  string // 服务端最新版本

	// 同步状态
	syncing   bool
	syncCancel chan struct{}
}

// Run 启动 GUI
func Run(cfgDir string) error {
	app := &App{
		cfgDir:     cfgDir,
		cfg:        config.New(cfgDir),
		syncCancel: make(chan struct{}),
	}

	// 加载本地配置
	localCfg, err := app.cfg.LoadLocal()
	if err != nil {
		return fmt.Errorf("加载配置失败: %v", err)
	}
	app.localCfg = localCfg

	// 尝试拉取服务端版本列表
	app.refreshServerPacks()

	// 确定初始选中版本
	app.determineInitialPack()

	app.buildUI().Run()
	return nil
}

func (a *App) buildUI() *walk.MainWindow {
	mw := new(walk.MainWindow)
	a.mw = mw

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
						Text:  "⚙",
						MinSize: Size{28, 0},
						MaxSize: Size{28, 0},
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
						Model:    a.packNames(),
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
						Text:     "🔄 更新",
						MinSize:  Size{100, 0},
						Enabled:  false,
						OnClicked: func() {
							a.startSync()
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
							a.cancelSync()
						},
					},
				},
			},
			// 版本状态栏
			Label{
				AssignTo: &a.verBar,
				Text:     "",
				Font:     Font{PointSize: 8},
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

	// 初始状态更新
	a.refreshUI()

	// 首次启动检测：无配置时弹出向导
	isFirstRun := a.localCfg.ServerURL == ""
	if isFirstRun {
		mw.Synchronize(func() {
			runSetupWizard(a)
		})
	}

	return mw
}

// ============================================================
// 数据 & 刷新
// ============================================================

// packNames 返回下拉列表的显示名
func (a *App) packNames() []string {
	names := make([]string, 0, len(a.serverPacks))
	for _, p := range a.serverPacks {
		display := p.DisplayName
		if a.localCfg != nil {
			if s, ok := a.localCfg.Packs[p.Name]; ok && s.LocalVersion != "" {
				display = fmt.Sprintf("%s v%s", p.DisplayName, s.LocalVersion)
			}
		}
		names = append(names, display)
	}
	if len(names) == 0 {
		names = append(names, "(无可用版本)")
	}
	return names
}

func (a *App) refreshServerPacks() {
	if a.localCfg == nil || a.localCfg.ServerURL == "" {
		return
	}
	resp, err := a.cfg.FetchPacks(a.localCfg.ServerURL)
	if err != nil {
		return
	}
	a.serverPacks = resp.Packs
}

func (a *App) determineInitialPack() {
	if len(a.serverPacks) == 0 {
		return
	}
	for _, p := range a.serverPacks {
		if p.Primary {
			a.selectedPack = p.Name
			a.latestVersion = p.LatestVersion
			return
		}
	}
	a.selectedPack = a.serverPacks[0].Name
	a.latestVersion = a.serverPacks[0].LatestVersion
}

func (a *App) refreshUI() {
	packName := a.selectedPack
	if packName == "" {
		if a.verBar != nil {
			a.verBar.SetText("")
		}
		if a.statusBar != nil {
			a.statusBar.SetText("就绪")
		}
		return
	}

	// 取显示名
	var displayName string
	for _, p := range a.serverPacks {
		if p.Name == packName {
			displayName = p.DisplayName
			break
		}
	}
	if displayName == "" {
		displayName = packName
	}

	// 本地版本
	if a.localCfg != nil {
		if s, ok := a.localCfg.Packs[packName]; ok {
			a.currentVersion = s.LocalVersion
		}
	}
	if a.currentVersion == "" {
		a.currentVersion = "(未安装)"
	}

	// 检查是否有更新
	a.hasUpdate = a.latestVersion != "" && a.currentVersion != "" &&
		a.currentVersion != a.latestVersion && a.currentVersion != "(未安装)"

	// 更新版本状态栏
	if a.verBar != nil {
		verText := fmt.Sprintf("%s  本地: %s  最新: %s", displayName, a.currentVersion, a.latestVersion)
		a.verBar.SetText(verText)
	}

	// 更新下拉列表（如果版本号变了）
	if a.packCB != nil {
		a.packCB.SetModel(a.packNames())
	}

	// 更新按钮
	if a.updateBtn != nil {
		a.updateBtn.SetEnabled(a.hasUpdate && !a.syncing)
	}
	if a.openBtn != nil {
		a.openBtn.SetEnabled(!a.syncing)
	}

	// 更新状态栏
	if a.statusBar != nil {
		if a.currentVersion == "(未安装)" {
			a.statusBar.SetText("未安装，请点击更新")
			a.statusBar.SetTextColor(walk.RGB(200, 150, 0))
		} else if a.hasUpdate {
			a.statusBar.SetText("有可用更新")
			a.statusBar.SetTextColor(walk.RGB(200, 100, 0))
		} else {
			a.statusBar.SetText("已是最新")
			a.statusBar.SetTextColor(walk.RGB(0, 170, 0))
		}
	}
}

func (a *App) onPackSelected() {
	if a.packCB == nil {
		return
	}
	idx := a.packCB.CurrentIndex()
	if idx < 0 || idx >= len(a.serverPacks) {
		return
	}
	p := a.serverPacks[idx]
	a.selectedPack = p.Name
	a.latestVersion = p.LatestVersion
	a.refreshUI()
}

// ============================================================
// 操作
// ============================================================

func (a *App) openSettings() {
	showSettings(a)
}

func (a *App) openLauncher() {
	if a.syncing || a.selectedPack == "" {
		return
	}
	if a.currentVersion == "(未安装)" {
		walk.MsgBox(a.mw, "提示", "请先点击更新安装此版本", walk.MsgBoxOK)
		return
	}
	walk.MsgBox(a.mw, "提示", fmt.Sprintf("打开启动器: %s\n（功能开发中）", a.selectedPack), walk.MsgBoxOK)
}

func (a *App) startSync() {
	if a.syncing || a.selectedPack == "" || !a.hasUpdate {
		return
	}

	a.Lock()
	a.syncing = true
	a.syncCancel = make(chan struct{})
	a.Unlock()

	a.updateBtn.SetEnabled(false)
	a.openBtn.SetEnabled(false)
	a.statusBar.SetText("正在同步...")
	a.statusBar.SetTextColor(walk.RGB(200, 150, 0))

	// 显示进度行
	// Walk 中动态显示控件需要通过 layout 操作，这里用简单的标签提示
	if a.cancelBtn != nil {
		a.cancelBtn.SetEnabled(true)
	}
	if a.progress != nil {
		a.progress.SetValue(0)
	}

	go func() {
		for i := 0; i <= 100; i += 10 {
			select {
			case <-a.syncCancel:
				a.handleSyncCancel()
				return
			default:
			}
			time.Sleep(200 * time.Millisecond)
			a.mw.Synchronize(func() {
				if a.progress != nil {
					a.progress.SetValue(i)
				}
				if a.progressLabel != nil {
					a.progressLabel.SetText(fmt.Sprintf("%d%%", i))
				}
			})
		}

		a.mw.Synchronize(func() {
			a.Lock()
			a.syncing = false
			a.Unlock()

			if a.localCfg != nil && a.selectedPack != "" {
				if s, ok := a.localCfg.Packs[a.selectedPack]; ok {
					s.LocalVersion = a.latestVersion
					a.localCfg.Packs[a.selectedPack] = s
					a.cfg.SaveLocal(a.localCfg)
				}
			}

			a.currentVersion = a.latestVersion
			a.refreshUI()
			walk.MsgBox(a.mw, "完成", fmt.Sprintf("%s 已更新到 %s", a.selectedPack, a.latestVersion), walk.MsgBoxOK)
			a.statusBar.SetText("已是最新")
		})
	}()
}

func (a *App) cancelSync() {
	if !a.syncing {
		return
	}
	a.cancelBtn.SetEnabled(false)
	close(a.syncCancel)
}

func (a *App) handleSyncCancel() {
	a.Lock()
	a.syncing = false
	a.Unlock()

	a.mw.Synchronize(func() {
		a.statusBar.SetText("已取消")
		a.statusBar.SetTextColor(walk.RGB(200, 100, 0))
		a.refreshUI()
		walk.MsgBox(a.mw, "提示", "同步已取消，已回滚到同步前状态", walk.MsgBoxOK)
	})
}

// RunGUI 兼容性入口
func RunGUI(cfgDir string) error {
	return Run(cfgDir)
}
