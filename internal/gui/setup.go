package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// ============================================================
// 首次配置向导
// ============================================================

// runSetupWizard 首次启动时弹出配置向导
// 返回 nil 表示配置完成，返回 error 表示用户取消或出错
func runSetupWizard(a *App) error {
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
	var mcCB *walk.ComboBox     // MC 目录下拉框
	var page0, page1, page2 *walk.Composite

	// 自动检测 & 手动选择
	var detectLaunchBtn, pickLaunchBtn, detectMCBtn, pickMCBtn *walk.PushButton

	// MC 目录下拉模型 — 使用包级 mcDirItem 类型
	//（定义见 settings.go）
	var mcDirItems []mcDirItem

	// 刷新 MC 目录下拉列表
	refreshMCDirs := func() {
		dirs := launcher.FindMinecraftDirs()
		managed, raw := launcher.IsManagedDirs(dirs)

		// 收集所有可用的目录，去重
		seen := make(map[string]bool)
		mcDirItems = nil

		// 已托管的优先
		for _, m := range managed {
			if seen[m.Path] {
				continue
			}
			seen[m.Path] = true
			suffix := "[已托管]"
			if len(m.Packs) > 0 {
				suffix = fmt.Sprintf("[已托管: %s]", strings.Join(m.Packs, ", "))
			}
			mcDirItems = append(mcDirItems, mcDirItem{
				Label: m.Path + "  " + suffix,
				Path:  m.Path,
			})
		}

		// 未托管的
		for _, r := range raw {
			if seen[r] {
				continue
			}
			seen[r] = true
			mcDirItems = append(mcDirItems, mcDirItem{
				Label: r + "  [未托管]",
				Path:  r,
			})
		}

		// 回退：如果什么都没检测到，给个手动输入提示
		if len(mcDirItems) == 0 {
			mcDirItems = append(mcDirItems, mcDirItem{
				Label: "(未检测到，请手动选择)",
				Path:  "",
			})
		}

		// 更新 ComboBox 模型
		labels := make([]string, len(mcDirItems))
		for i, item := range mcDirItems {
			labels[i] = item.Label
		}
		mcCB.SetModel(labels)

		// 默认选中第一个已托管的，否则选第一个
		mcCB.SetCurrentIndex(0)
		if len(mcDirItems) > 0 {
			mcDir = mcDirItems[0].Path
		}
	}

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
			picked := dlg2.FilePath
			// 检查是否已在列表中
			for i, item := range mcDirItems {
				if item.Path == picked {
					mcCB.SetCurrentIndex(i)
					mcDir = picked
					return
				}
			}
			// 不在列表中则添加到最前面并选中
			newItems := make([]mcDirItem, 0, len(mcDirItems)+1)
			newItems = append(newItems, mcDirItem{Label: picked + "  [手动添加]", Path: picked})
			newItems = append(newItems, mcDirItems...)
			mcDirItems = newItems
			labels := make([]string, len(mcDirItems))
			for i, item := range mcDirItems {
				labels[i] = item.Label
			}
			mcCB.SetModel(labels)
			mcCB.SetCurrentIndex(0)
			mcDir = picked
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

			// Step 2: MC 目录（下拉框选择）
			Composite{
				AssignTo: &page2,
				Layout:   VBox{},
				Visible:  false,
				Children: []Widget{
					Label{Text: "Minecraft 根目录（自动检测）："},
					ComboBox{
						AssignTo: &mcCB,
						MinSize:  Size{360, 0},
						OnCurrentIndexChanged: func() {
							idx := mcCB.CurrentIndex()
							if idx >= 0 && idx < len(mcDirItems) {
								mcDir = mcDirItems[idx].Path
							}
						},
					},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							PushButton{
								AssignTo:  &detectMCBtn,
								Text:      "🔍 刷新检测",
								OnClicked: func() { refreshMCDirs() },
							},
							PushButton{
								AssignTo:  &pickMCBtn,
								Text:      "📁 手动选择（不在列表中时）",
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

												// 自动刷新 MC 目录下拉列表
								refreshMCDirs()

							case 2:
								if mcDir == "" {
									// 尝试从下拉框取
									idx := mcCB.CurrentIndex()
									if idx >= 0 && idx < len(mcDirItems) {
										mcDir = mcDirItems[idx].Path
									}
								}
								if mcDir == "" {
									walk.MsgBox(dlg, "提示", "请选择或手动输入 Minecraft 根目录", walk.MsgBoxOK)
									return
								}
								if _, err := os.Stat(mcDir); os.IsNotExist(err) {
									walk.MsgBox(dlg, "提示", "目录不存在，请重新选择", walk.MsgBoxOK)
									return
								}
								step = 3
								showStep(3)
								nextPB.SetText("完成")

							default:
								cfg := a.vm.LocalConfig()
								cfg.ServerURL = serverURL
								cfg.Launcher = launcherPath
								cfg.SetMinecraftDir("", mcDir)
								a.vm.SaveLocalConfig(cfg)

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
		walk.MsgBox(nil, "错误", fmt.Sprintf("启动配置向导失败: %v", err), walk.MsgBoxOK)
		return fmt.Errorf("向导创建失败: %w", err)
	}

	showStep(0)
	res := dlg.Run()

	if res != walk.DlgCmdOK {
		return fmt.Errorf("用户取消")
	}
	return nil
}

// ============================================================
// 自动检测工具函数
// ============================================================

// detectLauncher 自动搜索 PCL2/HMCL
func detectLauncher() string {
	result := launcher.FindPCL2()
	if result != nil {
		return result.Path
	}
	// 也搜一下 HMCL
	// 目前 FindPCL2 已覆盖常见路径
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
