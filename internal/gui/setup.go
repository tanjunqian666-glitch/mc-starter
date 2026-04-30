package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// ============================================================
// 首次配置向导
// ============================================================

// runSetupWizard 首次启动时弹出配置向导
func runSetupWizard(a *App) {
	var dlg *walk.Dialog
	var nextPB, cancelPB *walk.PushButton

	// 当前步骤 (0=API, 1=启动器, 2=MC目录, 3=完成)
	step := 0

	serverURL := ""
	launcherPath := ""
	mcDir := ""

	// 步骤描述
	stepLabels := []string{
		"第 1 步: 填写服务器 API 地址",
		"第 2 步: 设置启动器路径",
		"第 3 步: 设置 Minecraft 根目录",
		"配置完成",
	}
	stepDesc := []string{
		"请向管理员索取 API 地址并填入下方。",
		"更新器会自动搜索 PCL2 或 HMCL，也可以手动选择。",
		"更新器会自动搜索 .minecraft 目录，也可以手动选择。",
		"所有配置已完成，点击完成即可开始使用。",
	}

	var stepLabel *walk.Label
	var descLabel *walk.Label
	var apiEdit *walk.LineEdit
	var launchEdit *walk.LineEdit
	var mcEdit *walk.LineEdit
	var detectLaunchBtn *walk.PushButton
	var pickLaunchBtn *walk.PushButton
	var detectMCBtn *walk.PushButton
	var pickMCBtn *walk.PushButton

	// 各步骤页面的 Widget 列表（动态显示/隐藏）
	pageWidgets := make([][]Widget, 3)

	// step 0: API
	pageWidgets[0] = []Widget{
		Label{Text: "服务器 API 地址:"},
		LineEdit{AssignTo: &apiEdit, MinSize: Size{360, 0}},
	}

	// step 1: 启动器
	pageWidgets[1] = []Widget{
		Label{Text: "启动器路径:"},
		Composite{
			Layout: HBox{},
			Children: []Widget{
				LineEdit{AssignTo: &launchEdit, MinSize: Size{260, 0}},
				PushButton{AssignTo: &detectLaunchBtn, Text: "🔍 自动检测"},
				PushButton{AssignTo: &pickLaunchBtn, Text: "📁 手动选择"},
			},
		},
	}

	// step 2: MC 目录
	pageWidgets[2] = []Widget{
		Label{Text: "Minecraft 根目录:"},
		Composite{
			Layout: HBox{},
			Children: []Widget{
				LineEdit{AssignTo: &mcEdit, MinSize: Size{260, 0}},
				PushButton{AssignTo: &detectMCBtn, Text: "🔍 自动检测"},
				PushButton{AssignTo: &pickMCBtn, Text: "📁 手动选择"},
			},
		},
	}

	if err := (Dialog{
		AssignTo:      &dlg,
		Title:         "首次配置向导",
		MinSize:       Size{450, 280},
		Size:          Size{450, 280},
		Layout:        VBox{Margins: Margins{10, 10, 10, 10}},
		DefaultButton: &nextPB,
		CancelButton:  &cancelPB,
		Children: []Widget{
			// 标题
			Label{AssignTo: &stepLabel, Text: stepLabels[0], Font: Font{PointSize: 10, Bold: true}},
			VSpacer{Size: 4},
			// 描述
			Label{AssignTo: &descLabel, Text: stepDesc[0], Font: Font{PointSize: 8}},
			VSpacer{Size: 8},

			// 步骤页面容器
			Composite{
				Layout: VBox{},
				Children: []Widget{
					// Step 0: API
					Composite{Layout: VBox{}, Children: pageWidgets[0]},
					// Step 1: 启动器
					Composite{Layout: VBox{}, Children: pageWidgets[1]},
					// Step 2: MC 目录
					Composite{Layout: VBox{}, Children: pageWidgets[2]},
				},
			},

			VSpacer{Size: 4},

			// step 指示器
			Label{Text: "1 / 3", Font: Font{PointSize: 8}},

			// 按钮行
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &nextPB,
						Text:     "下一步 →",
						OnClicked: func() {
							switch step {
							case 0:
								// 验证 API
								if apiEdit.Text() == "" {
									walk.MsgBox(dlg, "提示", "请输入服务器 API 地址", walk.MsgBoxOK)
									return
								}
								serverURL = apiEdit.Text()
								step = 1

							case 1:
								launcherPath = launchEdit.Text()
								step = 2

							case 2:
								mcDir = mcEdit.Text()
								step = 3

							default:
								return
							}

							if step < 3 {
								stepLabel.SetText(stepLabels[step])
								descLabel.SetText(stepDesc[step])
								// 自动执行检测
								switch step {
								case 1:
									go func() {
										detected := detectLauncher()
										if detected != "" {
											dlg.Synchronize(func() {
												launchEdit.SetText(detected)
												launcherPath = detected
											})
										}
									}()
								case 2:
									go func() {
										detected := detectMinecraftDir()
										if detected != "" {
											dlg.Synchronize(func() {
												mcEdit.SetText(detected)
												mcDir = detected
											})
										}
									}()
								}
							} else {
								// 完成 — 保存配置
								a.Lock()
								a.localCfg.ServerURL = serverURL
								a.localCfg.Launcher = launcherPath
								a.localCfg.MinecraftDir = mcDir
								a.Unlock()
								a.cfg.SaveLocal(a.localCfg)

								// 拉取服务端包列表
								go func() {
									a.refreshServerPacks()
									a.determineInitialPack()
									a.mw.Synchronize(func() {
										a.refreshUI()
									})
								}()

								dlg.Accept()
							}
						},
					},
					PushButton{
						AssignTo: &cancelPB,
						Text:     "取消",
						OnClicked: func() {
							// 取消向导也允许使用（有些配置可能已保存）
							dlg.Cancel()
						},
					},
				},
			},
		},
	}.Create(a.mw)); err != nil {
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("启动配置向导失败: %v", err), walk.MsgBoxOK)
		return
	}

	// 首次打开自动触发启动器检测
	go func() {
		detected := detectLauncher()
		if detected != "" {
			dlg.Synchronize(func() {
				launchEdit.SetText(detected)
				launcherPath = detected
			})
		}
	}()

	dlg.Run()
}

// ============================================================
// 自动检测工具函数
// ============================================================

// detectLauncher 自动搜索 PCL2/HMCL
func detectLauncher() string {
	result := launcher.FindPCL2()
	if result != nil {
		return filepath.Join(result.PCLDir, "PCL2.exe")
	}
	// 也搜一下 HMCL
	// 目前 FindPCL2 已覆盖常见路径
	return ""
}

// detectMinecraftDir 自动搜索 .minecraft 目录
func detectMinecraftDir() string {
	// 搜索常见位置
	candidates := []string{
		// APPDATA/.minecraft
		filepath.Join(os.Getenv("APPDATA"), ".minecraft"),
		filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", ".minecraft"),
		// PCL2 同目录
		filepath.Join(".minecraft"),
	}

	for _, dir := range candidates {
		verDir := filepath.Join(dir, "versions")
		if info, err := os.Stat(verDir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// pickFile 打开文件选择对话框
func pickFile(owner walk.Form, title, filter string) string {
	dlg := new(walk.FileDialog)
	dlg.Title = title
	dlg.Filter = filter

	if ok, _ := dlg.ShowOpen(owner); ok {
		return dlg.FilePath
	}
	return ""
}

// pickDir 打开目录选择对话框
func pickDir(owner walk.Form, title string) string {
	dlg := new(walk.FileDialog)
	dlg.Title = title

	if ok, _ := dlg.ShowBrowseFolder(owner); ok {
		return dlg.FilePath
	}
	return ""
}
