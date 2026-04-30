package gui

import (
	"fmt"
	"strings"
	"sync"

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

// settingsPackMCInfo 副包关联的 MC 目录下拉状态
type settingsPackMCInfo struct {
	items    []mcDirItem
	cb       *walk.ComboBox
	dirLabel *walk.Label // 显示当前 MC 路径的标签
}

// showSettings 打开设置弹窗
func showSettings(a *App) {
	var dlg *walk.Dialog
	var acceptPB, cancelPB *walk.PushButton

	serverURL := a.localCfg.ServerURL
	launcherPath := a.localCfg.Launcher

	// 主 MC 目录
	var mcDirItems []mcDirItem
	var mcDirCB *walk.ComboBox
	var mcDirPicked string // 选中值

	// 副包 MC 目录状态
	packMCInfo := make(map[string]*settingsPackMCInfo)
	var pmu sync.Mutex

	// 刷新 MC 目录列表（通用）
	refreshMCDirItems := func() []mcDirItem {
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
		return items
	}

	// 给 ComboBox 设置模型并尝试选中指定路径
	setupMCCB := func(cb *walk.ComboBox, items []mcDirItem, preferPath string) string {
		labels := make([]string, len(items))
		for i, item := range items {
			labels[i] = item.Label
		}
		cb.SetModel(labels)
		if cb.Model() == nil || len(items) == 0 {
			return ""
		}
		// 优先选中 preferPath
		if preferPath != "" {
			for i, item := range items {
				if item.Path == preferPath {
					cb.SetCurrentIndex(i)
					return items[i].Path
				}
			}
		}
		// 否则选第一个
		cb.SetCurrentIndex(0)
		return items[0].Path
	}

	// 刷新主 MC 目录下拉
	refreshMainMCDir := func() {
		mcDirItems = refreshMCDirItems()
		preferPath := a.localCfg.GetMinecraftDir("")
		mcDirPicked = setupMCCB(mcDirCB, mcDirItems, preferPath)
	}

	// 刷新指定副包的 MC 目录下拉
	refreshPackMCDir := func(packName string) {
		pmu.Lock()
		info := packMCInfo[packName]
		pmu.Unlock()
		if info == nil {
			return
		}

		items := refreshMCDirItems()
		info.items = make([]mcDirItem, len(items))
		for i, item := range items {
			info.items[i] = mcDirItem{Label: item.Label, Path: item.Path}
		}

		labels := make([]string, len(items))
		for i, item := range items {
			labels[i] = item.Label
		}
		info.cb.SetModel(labels)

		preferPath := a.localCfg.GetMinecraftDir(packName)
		if preferPath != "" {
			for i, item := range items {
				if item.Path == preferPath {
					info.cb.SetCurrentIndex(i)
					if info.dirLabel != nil {
						info.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", item.Path))
					}
					return
				}
			}
		}
		if len(items) > 0 {
			info.cb.SetCurrentIndex(0)
			if info.dirLabel != nil && items[0].Path != "" {
				info.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", items[0].Path))
			}
		}
	}

	// 手动选择目录，添加到列表并选中
	pickDir := func(owner walk.Form, cb *walk.ComboBox, items *[]mcDirItem, currentPtr *string) {
		fd := new(walk.FileDialog)
		fd.Title = "选择 Minecraft 根目录"
		if ok, _ := fd.ShowBrowseFolder(owner); ok {
			picked := fd.FilePath
			// 检查是否已在列表
			for i, item := range *items {
				if item.Path == picked {
					cb.SetCurrentIndex(i)
					*currentPtr = picked
					return
				}
			}
			// 添加并选中
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

	// 构建设置弹窗
	if err := (Dialog{
		AssignTo:      &dlg,
		Title:         "设置",
		MinSize:       Size{500, 400},
		Size:          Size{500, 400},
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
					LineEdit{Text: &serverURL, MinSize: Size{300, 0}},
				},
			},
			VSpacer{Size: 4},

			// 启动器路径
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "启动器路径:", MinSize: Size{90, 0}},
					LineEdit{Text: &launcherPath, MinSize: Size{240, 0}},
					PushButton{
						Text:     "🔍",
						MinSize:  Size{28, 0},
						MaxSize:  Size{28, 0},
						OnClicked: func() {
							detected := detectLauncher()
							if detected != "" {
								launcherPath = detected
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
								launcherPath = picked
							}
						},
					},
				},
			},
			VSpacer{Size: 4},

			// Minecraft 根目录（主版本）
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
					settingsPackList(a, dlg, packMCInfo, &pmu, refreshMCDirItems, setupMCCB, pickDir, refreshPackMCDir),
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
							if serverURL == "" {
								walk.MsgBox(dlg, "提示", "请输入服务器 API 地址", walk.MsgBoxOK)
								return
							}

							// 收集主版本 MC 目录
							if mcDirPicked != "" {
								a.localCfg.SetMinecraftDir("", mcDirPicked)
							}

							// 收集副包 MC 目录
							pmu.Lock()
							for packName, info := range packMCInfo {
								idx := info.cb.CurrentIndex()
								if idx >= 0 && idx < len(info.items) && info.items[idx].Path != "" {
									a.localCfg.SetMinecraftDir(packName, info.items[idx].Path)
								}
							}
							pmu.Unlock()

							a.Lock()
							a.localCfg.Launcher = launcherPath
							a.localCfg.ServerURL = serverURL
							a.Unlock()

							if err := a.cfg.SaveLocal(a.localCfg); err != nil {
								walk.MsgBox(dlg, "错误", fmt.Sprintf("保存配置失败: %v", err), walk.MsgBoxOK)
								return
							}

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
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("打开设置失败: %v", err), walk.MsgBoxOK)
		return
	}

	// 弹窗创建成功后，初始化 MC 目录下拉
	refreshMainMCDir()

	dlg.Run()
}

// settingsPackList 返回副版本列表（含独立的 MC 目录下拉）
func settingsPackList(a *App, dlg *walk.Dialog, packMCInfo map[string]*settingsPackMCInfo,
	pmu *sync.Mutex,
	refreshItems func() []mcDirItem,
	setupCB func(*walk.ComboBox, []mcDirItem, string) string,
	pickDirFn func(walk.Form, *walk.ComboBox, *[]mcDirItem, *string),
	refreshPack func(string),
) Widget {

	children := make([]Widget, 0, len(a.serverPacks)*3)

	for _, p := range a.serverPacks {
		p := p // capture

		checked := false
		if a.localCfg != nil {
			if s, ok := a.localCfg.Packs[p.Name]; ok {
				checked = s.Enabled
			}
		}

		// 勾选框
		cb := new(walk.CheckBox)
		children = append(children, CheckBox{
			AssignTo: &cb,
			Text:     fmt.Sprintf("%s (%s)", p.DisplayName, p.LatestVersion),
			Checked:  checked,
			OnCheckedChanged: func() {
				if a.localCfg != nil {
					a.Lock()
					s, ok := a.localCfg.Packs[p.Name]
					if !ok {
						s = model.PackState{
							Enabled: false,
							Status:  "none",
							Dir:     fmt.Sprintf("packs/%s", p.Name),
						}
					}
					s.Enabled = cb.Checked()
					a.localCfg.Packs[p.Name] = s
					a.Unlock()
				}
			},
		})

		// 启用状态下显示 MC 目录下拉
		if checked {
			info := &settingsPackMCInfo{}
			pmu.Lock()
			packMCInfo[p.Name] = info
			pmu.Unlock()

			var dirCB *walk.ComboBox
			var dirLabel *walk.Label
			info.cb = dirCB

			items := refreshItems()
			info.items = make([]mcDirItem, len(items))
			for i, item := range items {
				info.items[i] = mcDirItem{Label: item.Label, Path: item.Path}
			}

			preferPath := a.localCfg.GetMinecraftDir(p.Name)

			children = append(children, Composite{
				Layout: HBox{Margins: Margins{Left: 20}},
				Children: []Widget{
					Label{Text: fmt.Sprintf("  MC 目录 (副):"), Font: Font{PointSize: 9}},
					ComboBox{
						AssignTo: &dirCB,
						MinSize:  Size{280, 0},
						OnCurrentIndexChanged: func() {
							idx := dirCB.CurrentIndex()
							pmu.Lock()
							info := packMCInfo[p.Name]
							pmu.Unlock()
							if info != nil && idx >= 0 && idx < len(info.items) {
								if info.dirLabel != nil {
									info.dirLabel.SetText(fmt.Sprintf("MC 目录: %s", info.items[idx].Path))
								}
							}
						},
					},
					PushButton{
						Text:     "🔍",
						MinSize:  Size{28, 0},
						MaxSize:  Size{28, 0},
						OnClicked: func() { refreshPack(p.Name) },
					},
				},
			})

			// 路径标签
			children = append(children, Label{
				AssignTo: &dirLabel,
				Text:     "",
				Font:     Font{PointSize: 9},
				TextColor: walk.RGB(100, 100, 100),
			})

			info.dirLabel = dirLabel
			info.cb = dirCB

			setupCB(dirCB, items, preferPath)
			if preferPath != "" {
				dirLabel.SetText(fmt.Sprintf("MC 目录: %s", preferPath))
			}
		}
	}

	if len(children) == 0 {
		children = append(children, Label{Text: "(无可用的副版本)", Font: Font{PointSize: 9}})
	}

	return Composite{
		Layout: VBox{},
		Children: children,
	}
}
