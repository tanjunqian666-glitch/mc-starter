// Package gui — Repair Window
//
// G.8 修复工具窗口
//
// 布局：
//   ┌──── 修复工具 ────────────────────────────┐
//   │  已完成: 35%                              │
//   │  ─────────────────────────────────────    │
//   │  [🔧 执行全量修复]                        │
//   │  [⚡ 执行 MC 修复]                        │
//   │  [📦 执行模组同步]                        │
//   │  [📤 上传崩溃日志]                        │
//   │  ─────────────────────────────────────    │
//   │  备份: [05/01/26 22:30  ▼]               │
//   │  [🔄 恢复选中备份]                        │
//   └───────────────────────────────────────────┘
//
// 规则：
//   - 4 个修复按钮 + 恢复按钮互斥：任意操作进行时全部禁用
//   - 完成 / 失败后重新启用
//   - 点击修复按钮时弹窗问 "是否备份？"
//   - 崩溃日志上传不上进度（小文件，瞬间完成）

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/gege-tlph/mc-starter/internal/repair"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// repairWindowState 修复窗口状态
type repairWindowState struct {
	vm  *ViewModel
	app *App // 通过 App 获取配置（简化对接，未来可改为只传 orc）
	orc *Orchestrator

	dlg *walk.Dialog

	// 控件
	cleanAllBtn    *walk.PushButton
	mcRepairBtn    *walk.PushButton
	modSyncBtn     *walk.PushButton
	crashUploadBtn *walk.PushButton
	restoreBtn     *walk.PushButton
	backupCB       *walk.ComboBox
	progressLabel  *walk.Label
	progressBar    *walk.ProgressBar

	// 备份下拉数据
	backupItems []repair.BackupListEntry
	selectedBackupID string

	mu   sync.Mutex
	busy bool
}

// showRepairWindow 打开修复工具窗口
func showRepairWindow(app *App, vm *ViewModel, orc *Orchestrator) {
	state := &repairWindowState{
		vm:  vm,
		app: app,
		orc: orc,
	}

	mw := app.mw
	if mw == nil {
		return
	}

	var dlg *walk.Dialog
	state.dlg = &walk.Dialog{}

	if err := (Dialog{
		AssignTo: &dlg,
		Title:    "修复工具",
		MinSize:  Size{420, 340},
		Size:     Size{420, 340},
		Layout:   VBox{Margins: Margins{10, 10, 10, 10}},
		Children: []Widget{
			// 当前版本提示
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{
						Text:     "当前版本: ",
						Font:     Font{PointSize: 9},
						TextColor: walk.RGB(100, 100, 100),
					},
					Label{
						Text:     currentVersionText(vm),
						Font:     Font{PointSize: 9, Bold: true},
						TextColor: walk.RGB(80, 80, 80),
					},
				},
			},
			// 进度显示（Label 方式，不用进度条）
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{
						Text:     "已完成: ",
						Font:     Font{PointSize: 9},
					},
					Label{
						AssignTo: &state.progressLabel,
						Text:     "",
						Font:     Font{PointSize: 9, Bold: true},
					},
					ProgressBar{
						AssignTo: &state.progressBar,
						MinSize:  Size{100, 0},
						Visible:  false,
					},
				},
			},
			// 分隔线
			Label{Text: "────────────────────────", Font: Font{PointSize: 8}, TextColor: walk.RGB(180, 180, 180)},
			// 全量修复
			PushButton{
				AssignTo: &state.cleanAllBtn,
				Text:     "🔧 执行全量修复",
				OnClicked: func() {
					state.startRepair(actionCleanAll)
				},
			},
			// MC 本体修复
			PushButton{
				AssignTo: &state.mcRepairBtn,
				Text:     "⚡ 执行 MC 修复",
				OnClicked: func() {
					state.startRepair(actionMCRepair)
				},
			},
			// 模组同步
			PushButton{
				AssignTo: &state.modSyncBtn,
				Text:     "📦 执行模组同步",
				OnClicked: func() {
					state.startRepair(actionModSync)
				},
			},
			// 崩溃日志上传
			PushButton{
				AssignTo: &state.crashUploadBtn,
				Text:     "📤 上传崩溃日志",
				OnClicked: func() {
					state.startRepair(actionCrashUpload)
				},
			},
			// 分隔线
			Label{Text: "────────────────────────", Font: Font{PointSize: 8}, TextColor: walk.RGB(180, 180, 180)},
			// 备份区域
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "备份:", MinSize: Size{40, 0}},
					ComboBox{
						AssignTo: &state.backupCB,
						MinSize:  Size{200, 0},
						OnCurrentIndexChanged: func() {
							idx := state.backupCB.CurrentIndex()
							if idx >= 0 && idx < len(state.backupItems) {
								state.selectedBackupID = state.backupItems[idx].ID
							}
						},
					},
					PushButton{
						AssignTo: &state.restoreBtn,
						Text:     "🔄 恢复选中备份",
						OnClicked: func() {
							state.startRestore()
						},
					},
				},
			},
		},
	}.Create(mw)); err != nil {
		walk.MsgBox(mw, "错误", fmt.Sprintf("打开修复工具失败: %v", err), walk.MsgBoxOK)
		return
	}

	// 刷新备份列表
	state.refreshBackups()

	state.refreshUI()

	if err := dlg.Run(); err != nil {
		walk.MsgBox(mw, "错误", fmt.Sprintf("打开修复工具失败: %v", err), walk.MsgBoxOK)
	}
}

// ============================================================
// 修复操作类型（内部枚举）
// ============================================================

type repairAction int

const (
	actionCleanAll repairAction = iota
	actionMCRepair
	actionModSync
	actionCrashUpload
)

// ============================================================
// 修复入口
// ============================================================

func (rs *repairWindowState) startRepair(action repairAction) {
	rs.mu.Lock()
	if rs.busy {
		rs.mu.Unlock()
		return
	}
	rs.busy = true
	rs.mu.Unlock()

	rs.refreshUI()

	// 所有操作先弹窗确认，用用户能看懂的语言
	confirmTitle, confirmText := getRepairConfirm(action)
	result, dlgErr := walk.MsgBox(rs.dlg, confirmTitle, confirmText, walk.MsgBoxYesNo)
	if dlgErr != nil || result != walk.DlgCmdYes {
		rs.setDone()
		return
	}

	localCfg := rs.vm.LocalConfig()
	mcDir := localCfg.GetMinecraftDir(rs.vm.SelectedPack())
	if mcDir == "" {
		walk.MsgBox(rs.dlg, "错误", "未配置 Minecraft 目录，请先在设置中配置", walk.MsgBoxOK)
		rs.setDone()
		return
	}

	// 非崩溃日志的操作，再问是否备份
	if action != actionCrashUpload {
		withBackup := true
		backupResult, _ := walk.MsgBox(rs.dlg, "备份", "是否先备份当前用户数据（存档、截图等）？", walk.MsgBoxYesNo)
		if backupResult == walk.DlgCmdNo {
			withBackup = false
		}

		switch action {
		case actionCleanAll:
			rs.orc.DoRepairCleanAll(mcDir, withBackup)
		case actionMCRepair:
			rs.orc.DoRepairMC(mcDir, withBackup)
		case actionModSync:
			rs.orc.DoRepairModsSync(mcDir, withBackup)
		}

		rs.setProgress("正在执行...")
		go rs.waitRepairDone()
	} else {
		// 崩溃日志直接上传
		rs.setProgress("正在上传崩溃日志...")
		rs.orc.DoCrashLogUpload(mcDir)
		go rs.waitCrashUpload()
	}
}

func (rs *repairWindowState) startRestore() {
	rs.mu.Lock()
	if rs.busy {
		rs.mu.Unlock()
		return
	}
	if rs.selectedBackupID == "" {
		walk.MsgBox(rs.dlg, "提示", "请先选择要恢复的备份", walk.MsgBoxOK)
		rs.mu.Unlock()
		return
	}

	// 先弹窗确认
	var backupDesc string
	for _, b := range rs.backupItems {
		if b.ID == rs.selectedBackupID {
			timeStr := b.CreatedAt.Format("2006-01-02 15:04")
			backupDesc = fmt.Sprintf("备份时间: %s\n文件数: %d\n", timeStr, b.FileCount)
			break
		}
	}
	confirmText := backupDesc + "\n恢复后将替换当前的 mods、config、存档等文件。\n确定恢复吗？"
	confirmResult, confirmErr := walk.MsgBox(rs.dlg, "恢复备份", confirmText, walk.MsgBoxYesNo)
	if confirmErr != nil || confirmResult != walk.DlgCmdYes {
		return
	}

	backupID := rs.selectedBackupID
	rs.busy = true
	rs.mu.Unlock()

	rs.refreshUI()
	rs.setProgress("正在恢复备份...")

	localCfg := rs.vm.LocalConfig()
	mcDir := localCfg.GetMinecraftDir(rs.vm.SelectedPack())
	if mcDir == "" {
		walk.MsgBox(rs.dlg, "错误", "未配置 Minecraft 目录", walk.MsgBoxOK)
		rs.setDone()
		return
	}

	rs.orc.DoRestoreBackup(mcDir, backupID)
	go rs.waitRepairDone()
}

func getRepairConfirm(action repairAction) (string, string) {
	switch action {
	case actionCleanAll:
		return "确认全量修复",
			"将清理模组、配置、资源包和光影，然后从服务器重新下载全部文件。\n\n" +
				"会保留你的存档和截图，版本会自动更新到最新。\n\n确定执行吗？"
	case actionMCRepair:
		return "确认 MC 修复",
			"将重新安装 Minecraft 本体和模组加载器，版本会与服务端同步。\n\n" +
				"模组文件不会变动。确定执行吗？"
	case actionModSync:
		return "确认模组同步",
			"将清空 mods 文件夹，从服务器拉取最新模组，版本会自动更新到最新。\n\n" +
				"确定执行吗？"
	case actionCrashUpload:
		return "确认上传",
			"将收集最近的崩溃报告并上传到服务器，方便排查问题。\n\n确定执行吗？"
	default:
		return "确认", "确定执行这个操作吗？"
	}
}

// ============================================================
// 等待完成（EventBus 事件监听）
// ============================================================

func (rs *repairWindowState) waitRepairDone() {
	// 简单轮询 ViewModel 状态机：等回到 Idle/Error
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		cur := rs.vm.StateMachine().Current()
		if !isBusyState(cur) {
			// 回到空闲状态
			rs.dlg.Synchronize(func() {
				rs.setDone()
				rs.refreshBackups()
			})
			return
		}

		// 取进度更新到 UI
		percent, phase := rs.vm.Progress()
		rs.dlg.Synchronize(func() {
			if phase != "" {
				rs.setProgressLabel(percent)
			}
		})
	}
}

func (rs *repairWindowState) waitCrashUpload() {
	// 崩溃日志体积小，等几秒后认为完成
	time.Sleep(2 * time.Second)
	rs.dlg.Synchronize(func() {
		rs.setDone()
	})
}

// isBusyState 判断状态机是否在忙碌中
func isBusyState(s AppState) bool {
	switch s {
	case StateIdle, StateDone, StateError, StateCancelled:
		return false
	default:
		return true
	}
}

// ============================================================
// UI 刷新
// ============================================================

func (rs *repairWindowState) refreshUI() {
	rs.mu.Lock()
	b := rs.busy
	rs.mu.Unlock()

	// 所有按钮的 Enabled 同步
	for _, btn := range []*walk.PushButton{
		rs.cleanAllBtn, rs.mcRepairBtn, rs.modSyncBtn,
		rs.crashUploadBtn, rs.restoreBtn,
	} {
		if btn != nil {
			btn.SetEnabled(!b)
		}
	}
	// 备份下拉也禁用
	if rs.backupCB != nil {
		rs.backupCB.SetEnabled(!b)
	}
}

// currentVersionText 获取当前选中版本的显示文字
func currentVersionText(vm *ViewModel) string {
	status := vm.CurrentPackStatus()
	if status.DisplayName == "" {
		return "(未选择版本)"
	}
	if status.CurrentVersion == "" || status.CurrentVersion == "(未安装)" {
		return fmt.Sprintf("%s (未安装)", status.DisplayName)
	}
	return fmt.Sprintf("%s v%s", status.DisplayName, status.CurrentVersion)
}

func (rs *repairWindowState) setProgress(text string) {
	if rs.progressLabel != nil {
		rs.progressLabel.SetText(text)
	}
}

func (rs *repairWindowState) setProgressLabel(percent int) {
	if rs.progressLabel != nil {
		rs.progressLabel.SetText(fmt.Sprintf("已完成: %d%%", percent))
	}
}

func (rs *repairWindowState) setDone() {
	rs.mu.Lock()
	rs.busy = false
	rs.mu.Unlock()

	rs.refreshUI()
	if rs.progressLabel != nil {
		rs.progressLabel.SetText("")
	}
}

// ============================================================
// 备份下拉刷新
// ============================================================

func (rs *repairWindowState) refreshBackups() {
	localCfg := rs.vm.LocalConfig()
	mcDir := localCfg.GetMinecraftDir(rs.vm.SelectedPack())
	if mcDir == "" {
		return
	}

	backups, err := repair.ListBackups(mcDir)
	if err != nil {
		// 忽略
		rs.backupItems = nil
	} else {
		// 只展示最近 5 个
		if len(backups) > 5 {
			backups = backups[:5]
		}
		rs.backupItems = backups
	}

	if rs.backupCB == nil {
		return
	}

	if len(rs.backupItems) == 0 {
		rs.backupCB.SetModel([]string{"(无可用备份)"})
		rs.selectedBackupID = ""
		return
	}

	labels := make([]string, len(rs.backupItems))
	for i, b := range rs.backupItems {
		timeStr := b.CreatedAt.Format("01/02/06 15:04")
		info := timeStr
		if b.FileCount > 0 {
			info += fmt.Sprintf(" (%d 文件)", b.FileCount)
		}
		if b.Reason != "" {
			info += fmt.Sprintf(" [%s]", string(b.Reason))
		}
		labels[i] = info
	}
	rs.backupCB.SetModel(labels)
	if len(labels) > 0 {
		rs.backupCB.SetCurrentIndex(0)
		rs.selectedBackupID = rs.backupItems[0].ID
	}
}

// ============================================================
// 打开启动器（供 app.go OpenLauncher 使用）
// ============================================================

// openLauncherExternal 外部调用，通过 Orchestrator 校验后执行 exec.Command
func openLauncherExternal(launcherPath string) error {
	if launcherPath == "" {
		return fmt.Errorf("启动器路径为空")
	}
	if _, err := os.Stat(launcherPath); os.IsNotExist(err) {
		return fmt.Errorf("启动器文件不存在: %s", launcherPath)
	}

	// 启动外部 exe（不阻塞）
	cmd := exec.Command(launcherPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动启动器失败: %v", err)
	}

	// 分离进程后不等待
	go func() {
		_ = cmd.Wait()
	}()

	return nil
}

// ============================================================
// 路径检测
// ============================================================

// hasExeExt 检查文件名是否以 .exe 结尾
func hasExeExt(fileName string) bool {
	return strings.HasSuffix(strings.ToLower(fileName), ".exe")
}

// getMCDirForRepair 获取修复工具使用的 MC 目录
// 优先使用当前选中包专用的 MC 目录，回退到 _default
func getMCDirForRepair(localCfg *model.LocalConfig, packName string) string {
	if localCfg == nil {
		return ""
	}
	// 先找包专用目录
	if d, ok := localCfg.MinecraftDirs[packName]; ok && d != "" {
		return d
	}
	// 再找 _default
	if d, ok := localCfg.MinecraftDirs["_default"]; ok && d != "" {
		return d
	}
	// 兼容旧字段
	return localCfg.MinecraftDir
}
