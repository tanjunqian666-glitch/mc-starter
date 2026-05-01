package gui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// mcDirItem 下拉框列表项
type mcDirItem struct {
	Label string // 显示文本
	Path  string // 实际路径
}

// subPackUI 副版本 UI 控件集合
type subPackUI struct {
	packName    string
	cb          *walk.CheckBox
	cbPtr       **walk.CheckBox // AssignTo 指针的指针（Create 后解引用得实际 cb）
	mcRow       *walk.Composite
	mcCB        *walk.ComboBox
	mcCBPtr     **walk.ComboBox
	dirLabel    *walk.Label
	reloadBtn   *walk.PushButton

	// MC 目录下拉数据
	items  []mcDirItem
}

// showSettings 打开设置弹窗
func showSettings(a *App) {
	debugLog("showSettings start")
	var dlg *walk.Dialog
	var acceptPB, cancelPB *walk.PushButton
	var serverEdit, launchEdit *walk.LineEdit

	localCfg := a.vm.LocalConfig()

	debugLog("showSettings: localCfg.ServerURL=%q Launcher=%q", localCfg.ServerURL, localCfg.Launcher)

	// 主 MC 目录
	var mcDirItems []mcDirItem
	var mcDirCB *walk.ComboBox
	var mcDirPicked string

	// 副版本 UI 控件（dialog Create 后初始化）
	var subUIs []*subPackUI

	// 缓存 MC 目录扫描结果（避免每次刷新都搜 PCL）
	var mcDirCache []mcDirItem
	scanMCDirs := func() []mcDirItem {
		if mcDirCache != nil {
			return mcDirCache
		}
		dirs := launcher.FindMinecraftDirs()
		managed, raw := launcher.IsManagedDirs(dirs)
		seen := make(map[string]bool)
		var items []mcDirItem

		for _, m := range managed {
			if seen[m.Path] {
				continue
			}
			seen[m.Path] = true
			suffix := "[已托管]"
			if len(m.Packs) > 0 {
				suffix = fmt.Sprintf("[已托管: %s]", strings.Join(m.Packs, ", "))
			}
			items = append(items, mcDirItem{
				Label: m.Path + "  " + suffix,
				Path:  m.Path,
			})
		}
		for _, r := range raw {
			if seen[r] {
				continue
			}
			seen[r] = true
			items = append(items, mcDirItem{
				Label: r + "  [未托管]",
				Path:  r,
			})
		}
		if len(items) == 0 {
			items = append(items, mcDirItem{
				Label: "(未检测到，请手动选择)",
				Path:  "",
			})
		}
		mcDirCache = items
		return items
	}

	// 刷新 MC 目录列表（带缓存）
	refreshMCDirItems := func() []mcDirItem {
		return scanMCDirs()
	}

	// 给 ComboBox 设置模型并尝试选中指定路径
	// 如果 preferPath 不在 items 中，自动添加（保证已保存路径始终可选）
	setupMCCB := func(cb *walk.ComboBox, items []mcDirItem, preferPath string) string {
		labels := make([]string, len(items))
		for i, item := range items {
			labels[i] = item.Label
		}
		cb.SetModel(labels)
		if len(items) == 0 {
			return ""
		}
		if preferPath != "" {
			for i, item := range items {
				if item.Path == preferPath {
					cb.SetCurrentIndex(i)
					return items[i].Path
				}
			}
			// 已保存路径不在扫描结果中时，添加到列表头部
			newItem := mcDirItem{Label: preferPath + "  [已保存]", Path: preferPath}
			items = append([]mcDirItem{newItem}, items...)
			newLabels := make([]string, len(items))
			for i, item := range items {
				newLabels[i] = item.Label
			}
			cb.SetModel(newLabels)
			cb.SetCurrentIndex(0)
			return preferPath
		}
		cb.SetCurrentIndex(0)
		return items[0].Path
	}

	// 刷新主 MC 目录下拉
	refreshMainMCDir := func() {
		mcDirItems = refreshMCDirItems()
		preferPath := localCfg.GetMinecraftDir("")
		mcDirPicked = setupMCCB(mcDirCB, mcDirItems, preferPath)
	}

	// 刷新指定副包的 MC 目录下拉
	refreshSubMCDir := func(ui *subPackUI) {
		if ui == nil || ui.mcCB == nil {
			return
		}
		items := refreshMCDirItems()
		ui.items = make([]mcDirItem, len(items))
		for i, item := range items {
			ui.items[i] = mcDirItem{Label: item.Label, Path: item.Path}
		}
		labels := make([]string, len(items))
		for i, item := range items {
			labels[i] = item.Label
		}
		ui.mcCB.SetModel(labels)

		preferPath := localCfg.GetMinecraftDir(ui.packName)
		if preferPath != "" {
			for i, item := range items {
				if item.Path == preferPath {
					ui.mcCB.SetCurrentIndex(i)
					if ui.dirLabel != nil {
						ui.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", item.Path))
					}
					return
				}
			}
			// 已保存路径不在扫描结果中，添加到列表头部
			newItem := mcDirItem{Label: preferPath + "  [已保存]", Path: preferPath}
			ui.items = append([]mcDirItem{newItem}, ui.items...)
			newLabels := make([]string, len(ui.items))
			for i, item := range ui.items {
				newLabels[i] = item.Label
			}
			ui.mcCB.SetModel(newLabels)
			ui.mcCB.SetCurrentIndex(0)
			if ui.dirLabel != nil {
				ui.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", preferPath))
			}
			return
		}
		if len(items) > 0 {
			ui.mcCB.SetCurrentIndex(0)
			if ui.dirLabel != nil && items[0].Path != "" {
				ui.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", items[0].Path))
			}
		}
	}

	// 手动选择目录，添加到列表并选中
	pickDir := func(owner walk.Form, cb *walk.ComboBox, items *[]mcDirItem, currentPtr *string) {
		fd := new(walk.FileDialog)
		fd.Title = "选择 Minecraft 根目录"
		if ok, _ := fd.ShowBrowseFolder(owner); ok {
			picked := fd.FilePath
			for i, item := range *items {
				if item.Path == picked {
					cb.SetCurrentIndex(i)
					*currentPtr = picked
					return
				}
			}
			*items = append([]mcDirItem{{Label: picked + "  [手动添加]", Path: picked}}, *items...)
			labels := make([]string, len(*items))
			for i, item := range *items {
				labels[i] = item.Label
			}
			cb.SetModel(labels)
			cb.SetCurrentIndex(0)
			*currentPtr = picked
		}
	}

	// ============================================================
	// 构建副版本 UI 声明
	// ============================================================

	subPackChildren := buildSubPackUI(a, localCfg, &subUIs, refreshMCDirItems, setupMCCB, refreshSubMCDir)

	// ============================================================
	// Dialog 声明
	// ============================================================

	if err := (Dialog{
		AssignTo:      &dlg,
		Title:         "设置",
		MinSize:       Size{500, 420},
		Size:          Size{500, 420},
		Layout:        VBox{Margins: Margins{10, 10, 10, 10}},
		DefaultButton: &acceptPB,
		CancelButton:  &cancelPB,
		Children: []Widget{
			Label{Text: "设置", Font: Font{PointSize: 10, Bold: true}},

			// 服务器 API
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "服务器 API:", MinSize: Size{90, 0}},
					LineEdit{AssignTo: &serverEdit, MinSize: Size{300, 0}},
				},
			},
			VSpacer{Size: 4},

			// 启动器路径
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "启动器路径:", MinSize: Size{90, 0}},
					LineEdit{AssignTo: &launchEdit, MinSize: Size{240, 0}},
					PushButton{
						Text:     "🔍",
						MinSize:  Size{28, 0},
						MaxSize:  Size{28, 0},
						OnClicked: func() {
							detected := detectLauncher()
							if detected != "" {
								if launchEdit != nil {
									launchEdit.SetText(detected)
								}
							} else {
								walk.MsgBox(dlg, "未检测到", "未找到 PCL2 或 HMCL，请手动选择", walk.MsgBoxOK)
							}
						},
					},
					PushButton{
						Text:     "📁",
						MinSize:  Size{28, 0},
						MaxSize:  Size{28, 0},
						OnClicked: func() {
							picked := pickFile(dlg, "选择启动器", "可执行文件 (*.exe)|*.exe")
							if picked != "" {
								if launchEdit != nil {
									launchEdit.SetText(picked)
								}
							}
						},
					},
				},
			},
			VSpacer{Size: 4},

			// MC 根目录（主版本）
			Composite{
				Layout: VBox{},
				Children: []Widget{
					Composite{
						Layout: HBox{},
						Children: []Widget{
							Label{Text: "MC 根目录 (主):", MinSize: Size{90, 0}},
						},
					},
					Composite{
						Layout: HBox{},
						Children: []Widget{
							ComboBox{AssignTo: &mcDirCB, MinSize: Size{360, 0}, OnCurrentIndexChanged: func() {
								idx := mcDirCB.CurrentIndex()
								if idx >= 0 && idx < len(mcDirItems) {
									mcDirPicked = mcDirItems[idx].Path
								}
							}},
							PushButton{
								Text:     "📁",
								MinSize:  Size{28, 0},
								MaxSize:  Size{28, 0},
								OnClicked: func() {
									pickDir(dlg, mcDirCB, &mcDirItems, &mcDirPicked)
								},
							},
							PushButton{
								Text:     "🔍",
								MinSize:  Size{28, 0},
								MaxSize:  Size{28, 0},
								OnClicked: refreshMainMCDir,
							},
						},
					},
				},
			},
			VSpacer{Size: 4},

			// 副包列表
			GroupBox{
				Title:  "副版本",
				Layout: VBox{},
				Children: []Widget{
					Label{Text: "勾选启用/禁用副版本（独立的完整整合包）:", Font: Font{PointSize: 9}},
					subPackChildren,
				},
			},

			// 按钮行
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &acceptPB,
						Text:     "保存",
						OnClicked: func() {
							// 从输入框读取当前值（AssignTo 不自动同步值到变量）
							curServer := serverEdit.Text()
							curLauncher := launchEdit.Text()
							// 相对路径转绝对路径（避免下次启动时 exec.Command 找不到）
							if curLauncher != "" && !filepath.IsAbs(curLauncher) {
								if abs, err := filepath.Abs(curLauncher); err == nil {
									curLauncher = abs
								}
							}

							if curServer == "" {
								walk.MsgBox(dlg, "提示", "请输入服务器 API 地址", walk.MsgBoxOK)
								return
							}

							// 收集主版本 MC 目录
							if mcDirPicked != "" {
								localCfg.SetMinecraftDir("", mcDirPicked)
							}

							// 收集副包 MC 目录（从下拉框读取）
							for _, ui := range subUIs {
								if ui.cb.Checked() {
									idx := ui.mcCB.CurrentIndex()
									if idx >= 0 && idx < len(ui.items) && ui.items[idx].Path != "" {
										localCfg.SetMinecraftDir(ui.packName, ui.items[idx].Path)
									}
								}
							}

							localCfg.Launcher = curLauncher
							localCfg.ServerURL = curServer

							if err := a.vm.SaveLocalConfig(localCfg); err != nil {
								walk.MsgBox(dlg, "错误", fmt.Sprintf("保存配置失败: %v", err), walk.MsgBoxOK)
								return
							}

							// 不需要手动写 a.localCfg，vm.SaveLocalConfig 已自动更新内部引用

							go func() {
								a.refreshServerPacks()
								a.determineInitialPack()
								a.mw.Synchronize(func() {
									a.refreshUI()
								})
							}()

							dlg.Accept()
						},
					},
					PushButton{
						AssignTo: &cancelPB,
						Text:     "取消",
						OnClicked: func() { dlg.Cancel() },
					},
				},
			},
		},
	}.Create(a.mw)); err != nil {
		debugLog("showSettings Dialog.Create failed: %v", err)
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("打开设置失败: %v", err), walk.MsgBoxOK)
		return
	}
	debugLog("showSettings Dialog.Create OK")

	// 预填数据到输入框
	if serverEdit != nil {
		serverEdit.SetText(localCfg.ServerURL)
	}
	if launchEdit != nil {
		launchEdit.SetText(localCfg.Launcher)
	}

	// 弹窗创建后，初始化 MC 目录下拉
	debugLog("showSettings: refreshMainMCDir start")
	refreshMainMCDir()
	debugLog("showSettings: refreshMainMCDir done")

	// 解引用 cbPtr/mcCBPtr → cb/mcCB（walk Create 后才会填充 AssignTo 指针）
	for _, ui := range subUIs {
		if ui.cbPtr != nil {
			ui.cb = *ui.cbPtr
		}
		if ui.mcCBPtr != nil {
			ui.mcCB = *ui.mcCBPtr
		}
	}

	// 初始化副版本下拉
	for i, ui := range subUIs {
		debugLog("showSettings: subUI[%d] pack=%s checked=%v", i, ui.packName, ui.cb.Checked())
		if ui.cb.Checked() {
			debugLog("showSettings: refreshSubMCDir for %s", ui.packName)
			refreshSubMCDir(ui)
		}
	}

	debugLog("showSettings: about to call dlg.Run()")
	dlg.Run()
	debugLog("showSettings: dlg.Run() returned")
}

// buildSubPackUI 构建副版本 UI 控件数组（dialog Create 前调用）
// 返回 Composite 的 Children，同时填充 subUIs 以便后续 SetModel
func buildSubPackUI(a *App, localCfg *model.LocalConfig, subUIs *[]*subPackUI,
	refreshItems func() []mcDirItem,
	setupCB func(*walk.ComboBox, []mcDirItem, string) string,
	refreshSub func(*subPackUI),
) Widget {

	// 收集所有副版本
	var subPacks []model.PackInfo
	for _, p := range a.vm.ServerPacks() {
		if !p.Primary {
			subPacks = append(subPacks, p)
		}
	}

	if len(subPacks) == 0 {
		return Composite{
			Layout: VBox{},
			Children: []Widget{
				Label{Text: "(无可用的副版本)", Font: Font{PointSize: 9}},
			},
		}
	}

	children := make([]Widget, 0, len(subPacks)*3)

	for _, p := range subPacks {
		p := p

		checked := false
		if localCfg != nil {
			if s, ok := localCfg.Packs[p.Name]; ok {
				checked = s.Enabled
			}
		}

		// 预构建 MC 目录数据
		mcItems := refreshItems()

		ui := &subPackUI{
			packName: p.Name,
			items:    make([]mcDirItem, len(mcItems)),
		}
		for i, item := range mcItems {
			ui.items[i] = mcDirItem{Label: item.Label, Path: item.Path}
		}
		*subUIs = append(*subUIs, ui)

		var cb *walk.CheckBox
		var mcRow *walk.Composite
		var dirCB *walk.ComboBox
		var dirLabel *walk.Label
		var reloadBtn *walk.PushButton

		// 勾选框（toggle 副包 MC 目录行可见性）
		children = append(children, CheckBox{
			AssignTo: &cb,
			Text:     fmt.Sprintf("%s (%s)", p.DisplayName, p.LatestVersion),
			Checked:  checked,
			OnCheckedChanged: func() {
				enabled := cb.Checked()

				// 更新 localCfg
				s, ok := localCfg.Packs[p.Name]
				if !ok {
					s = model.PackState{
						Enabled: false,
						Status:  "none",
						Dir:     fmt.Sprintf("packs/%s", p.Name),
					}
				}
				s.Enabled = enabled
				localCfg.Packs[p.Name] = s

				// 显示/隐藏 MC 目录行
				if mcRow != nil {
					mcRow.SetVisible(enabled)
				}

				// 勾选时自动刷新目录下拉
				if enabled {
					refreshSub(ui)
				}
			},
		})
		// 存指针的指针：walk Create 后会填充 *cb，Create 后 *ui.cbPtr 就是控件的指针
		ui.cbPtr = &cb

		// MC 目录行（默认按 checked 显隐）
		children = append(children, Composite{
			AssignTo: &mcRow,
			Layout:   HBox{Margins: Margins{Left: 20}},
			Visible:  checked,
			Children: []Widget{
				Label{Text: "  MC 目录 (副):", Font: Font{PointSize: 9}},
				ComboBox{
					AssignTo: &dirCB,
					MinSize:  Size{280, 0},
					OnCurrentIndexChanged: func() {
						idx := dirCB.CurrentIndex()
						if idx >= 0 && idx < len(ui.items) {
							if dirLabel != nil {
								dirLabel.SetText(fmt.Sprintf("MC 目录: %s", ui.items[idx].Path))
							}
						}
					},
				},
				PushButton{
					AssignTo: &reloadBtn,
					Text:     "🔍",
					MinSize:  Size{28, 0},
					MaxSize:  Size{28, 0},
					OnClicked: func() { refreshSub(ui) },
				},
			},
		})
		ui.mcRow = mcRow
		ui.mcCBPtr = &dirCB
		ui.reloadBtn = reloadBtn

		// MC 目录路径标签
		children = append(children, Label{
			AssignTo: &dirLabel,
			Text:     "",
			Font:     Font{PointSize: 9},
			TextColor: walk.RGB(100, 100, 100),
		})
		ui.dirLabel = dirLabel

		// ComboBox 模型不在声明中设置（walk 在 Create 前设置 Model 会 panic）
		// 在 create 后通过 Synchronize 设置
	}

	return Composite{
		Layout: VBox{},
		Children: children,
	}
}
