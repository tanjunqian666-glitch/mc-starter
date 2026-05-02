// Package gui — Orchestrator module
//
// G.4 Orchestrator — 流程调度层
//
// 职责：将 ViewModel 的操作请求转化为 Core Services 的实际调用，
//       并通过 EventBus 发射进度、日志、错误、状态变更事件。
//
// 设计原则：
//   - Orchestrator 不直接操作 Walk 控件
//   - Orchestrator 通过传入 ViewModel 的引用更新版本状态
//   - 所有耗时操作在 goroutine 中执行，EventBus 回调由 ViewModel 同步到 UI
//   - Orchestrator 只负责"调度"，不负责"做"（具体工作交给 launcher/config）
//
// 流程示例：
//   1. app.go: 用户点「安装/更新」→ 调用 orch.UpdateOrInstall()
//   2. Orch:  检查本地版本 → 决定增量/全量 → 发射进度事件
//   3. Orch:  调用 updater.EnsureVersion() → 安装 MC+Loader
//   4. Orch:  调用 updater.UpdatePack() → 同步文件
//   5. Orch:  保存最新版本号到 ViewModel → 发射 SyncDone 事件
//   6. ViewModel: 更新状态 → UI 自动刷新
//
// 注意：修复相关的函数（DoRepair*, doRepair, ListBackups 等）
// 已移至 internal/v2/gui/，v2 迭代时移回。

package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// Orchestrator
// ============================================================

// Orchestrator 流程调度器
type Orchestrator struct {
	cfgDir string
	vm     *ViewModel
	eb     *EventBus

	// 运行时缓存（避免重复创建）
	cfg *config.Manager

	// 取消信号：UpdateOrInstall 操作取消
	cancelCh chan struct{}
}

// NewOrchestrator 创建调度器
// cfgDir: 配置目录
// vm: ViewModel 引用（用于更新版本状态）
// eb: EventBus（用于发射事件）
func NewOrchestrator(cfgDir string, vm *ViewModel, eb *EventBus) *Orchestrator {
	return &Orchestrator{
		cfgDir:   cfgDir,
		vm:       vm,
		eb:       eb,
		cfg:      vm.ConfigManager(),
		cancelCh: make(chan struct{}),
	}
}

// ============================================================
// 取消
// ============================================================

// Cancel 取消正在执行的 UpdateOrInstall 操作
func (o *Orchestrator) Cancel() {
	select {
	case <-o.cancelCh:
		// 已关闭
	default:
		close(o.cancelCh)
	}
	o.cancelCh = make(chan struct{})
}

// ============================================================
// 更新/安装
// ============================================================

// UpdateOrInstall 安装或更新当前选中的版本
//
// 流程：
//  1. 检查本地版本 → 决定全量/增量
//  2. 获取包详情（含 MC 版本、Loader 规格）
//  3. 调用 EnsureVersion() 安装 MC+Loader
//  4. 调用 UpdatePack() 同步模组文件
//  5. 保存本地版本号 → 发送 SyncDone 事件
//
// 本方法在单独的 goroutine 中执行，不阻塞调用方。
func (o *Orchestrator) UpdateOrInstall() {
	packName := o.vm.SelectedPack()
	if packName == "" {
		o.eb.EmitError("同步", "未选中版本，请先选择一个版本", nil)
		return
	}

	if !o.vm.CanSync() {
		o.eb.EmitLog("warn", "当前状态不允许同步（可能已在同步或已是最新）")
		return
	}

	// 标记同步开始
	o.vm.MarkSyncStart()

	// 获取本地配置快照
	localCfg := o.vm.LocalConfig()
	serverURL := localCfg.ServerURL
	if serverURL == "" {
		o.eb.EmitError("同步", "未配置服务端地址，请先完成设置", nil)
		o.vm.MarkSyncError()
		return
	}

	mcDir := localCfg.GetMinecraftDir(packName)
	if mcDir == "" {
		o.eb.EmitError("同步", "未配置 Minecraft 目录，请先完成设置", nil)
		o.vm.MarkSyncError()
		return
	}

	syncType := "增量更新"
	if o.vm.SyncType() == "install" {
		syncType = "首次安装"
	}
	o.eb.EmitProgress(0, fmt.Sprintf("开始%s...", syncType))

	// 在 goroutine 中执行耗时操作
	go o.doUpdate(packName, serverURL, mcDir, localCfg)
}

// doUpdate 实际执行更新/安装（在 goroutine 中运行）
func (o *Orchestrator) doUpdate(packName, serverURL, mcDir string, localCfg *model.LocalConfig) {
	// 1. 获取包详情（含 MC 版本、Loader 规格）
	o.eb.EmitProgress(5, "正在获取版本信息...")

	detail, err := o.cfg.FetchPackDetail(serverURL, packName)
	if err != nil {
		o.eb.EmitError("同步", fmt.Sprintf("获取版本信息失败: %v", err), err)
		o.vm.MarkSyncError()
		o.eb.EmitSyncDone(packName, "", "", false, err)
		return
	}

	mcVersion := detail.Name // 服务端指定 MC 版本（从 pack detail 获取）
	loaderSpec := ""          // 如 "fabric-0.16.10"

	// 尝试通过 FetchUpdate 获取 MC 版本和 Loader 信息
	packState, _ := localCfg.Packs[packName]
	update, err := o.cfg.FetchUpdate(serverURL, packName, packState.LocalVersion, nil)
	if err == nil {
		if update.MCVersion != "" {
			mcVersion = update.MCVersion
		}
		if update.Loader != "" {
			loaderSpec = update.Loader
		}
	} else {
		o.eb.EmitLog("warn", fmt.Sprintf("获取增量信息失败（将使用默认 MC 版本）: %v", err))
	}

	// 2. 创建 Updater
	updater := launcher.NewUpdater(o.cfgDir, mcDir, o.cfg)

	// 3. 安装 MC + Loader（EnsureVersion）
	o.eb.EmitProgress(10, "正在准备安装环境...")

	// 确定版本目录和库目录
	versionDir := filepath.Join(mcDir, "versions")
	libraryDir := filepath.Join(mcDir, "libraries")

	// 如果服务端返回了 MC 版本信息，安装 MC+Loader
	if mcVersion != "" {
		o.eb.EmitProgress(15, "正在检查 MC 版本安装...")

		ensureReq := launcher.EnsureRequest{
			MCVersion:  mcVersion,
			Loader:     loaderSpec,
			VersionDir: versionDir,
			LibraryDir: libraryDir,
		}

		ensureResult, err := updater.EnsureVersion(ensureReq)
		if err != nil {
			o.eb.EmitLog("warn", fmt.Sprintf("MC+Loader 安装出现错误（非致命）: %v", err))
			// 继续执行——就算是 fabric 安装出问题，模组文件仍可同步
		} else {
			o.eb.EmitLog("info", fmt.Sprintf("MC %s%s 安装完成 → 版本ID=%s",
				mcVersion,
				map[bool]string{true: " + " + loaderSpec, false: ""}[loaderSpec != ""],
				ensureResult.VersionID))
		}

		o.eb.EmitProgress(30, "MC 环境准备完成")
	}

	// 4. 同步整合包文件
	o.eb.EmitProgress(35, "正在下载模组文件...")

	forceFull := o.vm.SyncType() == "install"
	// 将 PackState 转为指针传递给 UpdatePack
	var packStatePtr *model.PackState
	if ps, ok := localCfg.Packs[packName]; ok {
		packStatePtr = &ps
	}

	result, err := updater.UpdatePack(serverURL, packName, packStatePtr, forceFull)
	if err != nil {
		o.eb.EmitError("同步", fmt.Sprintf("下载模组文件失败: %v", err), err)
		o.vm.MarkSyncError()
		o.eb.EmitSyncDone(packName, "", "", false, err)
		return
	}

	// 5. 下载完成，写入版本信息
	newVersion := result.Version
	o.vm.MarkSyncDone(newVersion)

	o.eb.EmitProgress(100, "完成")

	// 保存本地配置
	localCfg.Packs[packName] = model.PackState{
		Enabled:      true,
		Status:       "synced",
		LocalVersion: newVersion,
		Dir:          fmt.Sprintf("packs/%s", packName),
	}
	if err := o.cfg.SaveLocal(localCfg); err != nil {
		o.eb.EmitLog("warn", fmt.Sprintf("保存本地配置失败: %v", err))
	}

	// 发送完成事件
	oldVersion := o.vm.currentVersion
	o.eb.EmitSyncDone(packName, newVersion, oldVersion, forceFull, nil)
	o.eb.EmitLog("info", fmt.Sprintf("同步完成: %s %s → %s (%s)", packName, oldVersion, newVersion, result.Summary()))
}

// ============================================================
// 打开启动器
// ============================================================

// OpenLauncher 打开 PCL2/HMCL 启动器
func (o *Orchestrator) OpenLauncher() error {
	localCfg := o.vm.LocalConfig()
	launcherPath := localCfg.Launcher
	if launcherPath == "" || launcherPath == "bare" {
		return fmt.Errorf("未配置启动器路径")
	}

	// 检查启动器文件是否存在
	if _, err := os.Stat(launcherPath); os.IsNotExist(err) {
		return fmt.Errorf("启动器文件不存在: %s", launcherPath)
	}

	// 通过 EventBus 发射日志（方便追踪）
	o.eb.EmitLog("info", fmt.Sprintf("打开启动器: %s", launcherPath))

	// 返回 nil 表示"应该尝试打开"，调用方处理具体 os/exec
	return nil
}

// ============================================================
// 配置
// ============================================================

// PingServer 测试服务端是否可达
func (o *Orchestrator) PingServer() error {
	localCfg := o.vm.LocalConfig()
	if localCfg.ServerURL == "" {
		return fmt.Errorf("未配置服务端地址")
	}
	return o.cfg.Ping(localCfg.ServerURL)
}

// ResetCancel 重置取消信号（可在新操作开始前调用）
func (o *Orchestrator) ResetCancel() {
	select {
	case <-o.cancelCh:
		o.cancelCh = make(chan struct{})
	default:
		// 未关闭过，不需要重置
	}
}
