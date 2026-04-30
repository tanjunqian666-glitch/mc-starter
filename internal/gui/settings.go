package gui

import (
	"fmt"

	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// showSettings 打开设置弹窗
func showSettings(a *App) {
	var dlg *walk.Dialog
	var acceptPB, cancelPB *walk.PushButton

	mcDir := a.localCfg.MinecraftDir
	launcherPath := a.localCfg.Launcher
	serverURL := a.localCfg.ServerURL

	if err := (Dialog{
		AssignTo:      &dlg,
		Title:         "设置",
		MinSize:       Size{420, 320},
		Size:          Size{420, 320},
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
					LineEdit{Text: &serverURL, MinSize: Size{280, 0}},
				},
			},
			VSpacer{Size: 4},

			// 启动器路径
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "启动器路径:", MinSize: Size{90, 0}},
					LineEdit{Text: &launcherPath, MinSize: Size{220, 0}},
					PushButton{
						Text:  "🔍",
						Width: 28,
						OnClicked: func() {
							// 复用 pcl_detect 的逻辑
							detected := detectLauncher()
							if detected != "" {
								launcherPath = detected
								// 通知 LineEdit 刷新
								dlg.Synchronize(func() {
									// walk 的 LineEdit 通过指针绑定自动同步
								})
							} else {
								walk.MsgBox(dlg, "未检测到", "未找到 PCL2 或 HMCL，请手动选择")
							}
						},
					},
					PushButton{
						Text:  "📁",
						Width: 28,
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

			// Minecraft 根目录
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Minecraft 根目录:", MinSize: Size{90, 0}},
					LineEdit{Text: &mcDir, MinSize: Size{220, 0}},
					PushButton{
						Text:  "🔍",
						Width: 28,
						OnClicked: func() {
							detected := detectMinecraftDir()
							if detected != "" {
								mcDir = detected
							} else {
								walk.MsgBox(dlg, "未检测到", "未找到 .minecraft 目录，请手动选择")
							}
						},
					},
					PushButton{
						Text:  "📁",
						Width: 28,
						OnClicked: func() {
							picked := pickDir(dlg, "选择 Minecraft 根目录")
							if picked != "" {
								mcDir = picked
							}
						},
					},
				},
			},
			VSpacer{Size: 8},

			// 副包列表
			GroupBox{
				Title:  "副版本",
				Layout: VBox{},
				Children: []Widget{
					Label{Text: "勾选启用副版本（独立的完整整合包）:", Font: Font{PointSize: 8}},
					packList(a),
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
							// 验证必需字段
							if serverURL == "" {
								walk.MsgBox(dlg, "提示", "请输入服务器 API 地址")
								return
							}

							a.Lock()
							a.localCfg.MinecraftDir = mcDir
							a.localCfg.Launcher = launcherPath
							a.localCfg.ServerURL = serverURL
							a.Unlock()

							if err := a.cfg.SaveLocal(a.localCfg); err != nil {
								walk.MsgBox(dlg, "错误", fmt.Sprintf("保存配置失败: %v", err))
								return
							}

							// 保存后重新拉取服务端包列表
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
		walk.MsgBox(a.mw, "错误", fmt.Sprintf("打开设置失败: %v", err))
		return
	}

	dlg.Run()
}

// packList 返回副版本勾选框列表
func packList(a *App) Widget {
	children := make([]Widget, 0, len(a.serverPacks))
	for _, p := range a.serverPacks {
		p := p // capture
		checked := false
		if a.localCfg != nil {
			if s, ok := a.localCfg.Packs[p.Name]; ok {
				checked = s.Enabled
			}
		}
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
	}

	return Composite{
		Layout: VBox{},
		Children: children,
	}
}
