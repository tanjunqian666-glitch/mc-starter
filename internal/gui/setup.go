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
	var nextPB, prevPB, cancelPB *walk.PushButton
	var stepLabel, descLabel *walk.Label
	var stepIndicatorLabel *walk.Label

	// 当前步骤 (0=API, 1=启动器, 2=MC目录, 3=完成)
	step := 0

	serverURL := ""
	launcherPath := ""
	mcDir := ""

	// 各步骤输入控件
	var apiEdit *walk.LineEdit
	var launchEdit *walk.LineEdit
	var mcEdit *walk.LineEdit
	var page0, page1, page2 *walk.Composite

	// 自动检测 & 手动选择
	var detectLaunchBtn, pickLaunchBtn, detectMCBtn, pickMCBtn *walk.PushButton

	// 步骤标题
	stepTitles := []string{
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

	// 显示指定步骤，隐藏其他
	showStep := func(s int) {
		page0.SetVisible(s == 0)
		page1.SetVisible(s == 1)
		page2.SetVisible(s == 2)
		prevPB.SetVisible(s > 0 && s < 3)

		if s < 3 {
			stepLabel.SetText(stepTitles[s])
			descLabel.SetText(stepDesc[s])
			stepIndicatorLabel.SetText(fmt.Sprintf("%d / 3", s+1))
		} else {
			stepLabel.SetText(stepTitles[3])
			descLabel.SetText(stepDesc[3])
			stepIndicatorLabel.SetText("完成")
		}
		nextPB.SetEnabled(true)
	}

	// 手动选择启动器（限定 .exe）
	pickLauncher := func() {
		dlg2 := new(walk.FileDialog)
		dlg2.Title = "选择启动器"
		dlg2.Filter = "启动器程序 (*.exe)|*.exe|所有文件 (*.*)|*.*"
		if ok, _ := dlg2.ShowOpen(dlg); ok {
			launchEdit.SetText(dlg2.FilePath)
		}
	}

	// 手动选择 MC 目录
	pickMCDir := func() {
		dlg2 := new(walk.FileDialog)
		dlg2.Title = "选择 Minecraft 根目录"
		if ok, _ := dlg2.ShowBrowseFolder(dlg); ok {
			mcEdit.SetText(dlg2.FilePath)
		}
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
			Label{AssignTo: &stepLabel, Text: stepTitles[0], Font: Font{PointSize: 10, Bold: true}},
			VSpacer{Size: 4},
			Label{AssignTo: &descLabel, Text: stepDesc[0], Font: Font{PointSize: 9}},
			VSpacer{Size: 8},

			// Step 0: API
			Composite{
				AssignTo: &page0,
				Layout:   VBox{},
				Visible:  true,
				Children: []Widget{
					Label{Text: "服务器 API 地址:"},
					LineEdit{AssignTo: &apiEdit, MinSize: Size{360, 0}},
				},
			},

			// Step 1: 启动器
			Composite{
				AssignTo: &page1,
				Layout:   VBox{},
				Visible:  false,
				Children: []Widget{
					Label{Text: "启动器路径:"},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							LineEdit{AssignTo: &launchEdit, MinSize: Size{260, 0}},
							PushButton{
								AssignTo:  &detectLaunchBtn,
								Text:      "🔍 自动检测",
								OnClicked: func() { go func() { d := detectLauncher(); dlg.Synchronize(func() { launchEdit.SetText(d) }) }() },
							},
							PushButton{
								AssignTo:  &pickLaunchBtn,
								Text:      "📁 手动选择",
								OnClicked: pickLauncher,
							},
						},
					},
				},
			},

			// Step 2: MC 目录
			Composite{
				AssignTo: &page2,
				Layout:   VBox{},
				Visible:  false,
				Children: []Widget{
					Label{Text: "Minecraft 根目录:"},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							LineEdit{AssignTo: &mcEdit, MinSize: Size{260, 0}},
							PushButton{
								AssignTo:  &detectMCBtn,
								Text:      "🔍 自动检测",
								OnClicked: func() { go func() { d := detectMinecraftDir(); dlg.Synchronize(func() { mcEdit.SetText(d) }) }() },
							},
							PushButton{
								AssignTo:  &pickMCBtn,
								Text:      "📁 手动选择",
								OnClicked: pickMCDir,
							},
						},
					},
				},
			},

			VSpacer{Size: 4},
			Label{AssignTo: &stepIndicatorLabel, Text: "1 / 3", Font: Font{PointSize: 9}},

			// 按钮行
			Composite{
				Layout: HBox{},
				Children: []Widget{
					// 上一步（步骤 > 0 且 < 3 时显示）
					PushButton{
						AssignTo: &prevPB,
						Text:     "← 上一步",
						Visible:  false,
						OnClicked: func() {
							if step > 0 && step < 3 {
								step--
								showStep(step)
							}
						},
					},
					HSpacer{},
					PushButton{
						AssignTo: &nextPB,
						Text:     "下一步 →",
						OnClicked: func() {
							switch step {
							case 0:
								url := apiEdit.Text()
								if url == "" {
									walk.MsgBox(dlg, "提示", "请输入服务器 API 地址", walk.MsgBoxOK)
									return
								}
								serverURL = url
								step = 1
								showStep(1)

								// 自动检测启动器
								go func() {
									detected := detectLauncher()
									if detected != "" {
										dlg.Synchronize(func() {
											launchEdit.SetText(detected)
											launcherPath = detected
										})
									}
								}()

							case 1:
								lp := launchEdit.Text()
								// 启动器路径只需是 .exe 文件即可
								if lp != "" {
									ext := filepath.Ext(lp)
									if ext != ".exe" {
										walk.MsgBox(dlg, "提示", "启动器程序必须是 .exe 文件，请重新选择", walk.MsgBoxOK)
										return
									}
									if _, err := os.Stat(lp); os.IsNotExist(err) {
										walk.MsgBox(dlg, "提示", "启动器文件不存在，请重新选择", walk.MsgBoxOK)
										return
									}
								}
								launcherPath = lp
								step = 2
								showStep(2)

								// 自动检测 MC 目录
								go func() {
									detected := detectMinecraftDir()
									if detected != "" {
										dlg.Synchronize(func() {
											mcEdit.SetText(detected)
											mcDir = detected
										})
									}
								}()

							case 2:
								md := mcEdit.Text()
								if md == "" {
									walk.MsgBox(dlg, "提示", "请输入 Minecraft 根目录", walk.MsgBoxOK)
									return
								}
								if _, err := os.Stat(md); os.IsNotExist(err) {
									walk.MsgBox(dlg, "提示", "目录不存在，请重新选择", walk.MsgBoxOK)
									return
								}
								mcDir = md
								step = 3
								showStep(3)
								nextPB.SetText("完成")

							default:
								a.Lock()
								a.localCfg.ServerURL = serverURL
								a.localCfg.Launcher = launcherPath
								a.localCfg.MinecraftDir = mcDir
								a.Unlock()
								a.cfg.SaveLocal(a.localCfg)

								go func() {
									a.refreshServerPacks()
									a.determineInitialPack()
									a.mw.Synchronize(func() { a.refreshUI() })
								}()
								dlg.Accept()
							}
						},
					},
					PushButton{
						AssignTo: &cancelPB,
						Text:     "取消",
						OnClicked: func() {
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

	showStep(0)
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
