// DoRepairCleanAll 全量修复：备份 → 清理 → 全量拉取最新模组 + 更新版本号到最新
// 用户不用再回主界面点安装，一次完成
func (o *Orchestrator) DoRepairCleanAll(mcDir string, withBackup bool) {
	o.vm.state.Transition(StateRepairing)

	go func() {
		packName := o.vm.SelectedPack()
		if packName == "" {
			o.eb.EmitError("全量修复", "未选中版本", nil)
			o.vm.MarkIdle()
			return
		}

		localCfg := o.vm.LocalConfig()
		serverURL := localCfg.ServerURL
		if serverURL == "" {
			o.eb.EmitError("全量修复", "未配置服务端地址", nil)
			o.vm.MarkIdle()
			return
		}

		// 1. 备份
		if withBackup {
			o.eb.EmitProgress(5, "正在备份用户数据...")
			backupResult, err := repair.CreateBackup(mcDir, repair.BackupOptions{Reason: repair.ReasonRepair})
			if err != nil {
				o.eb.EmitLog("warn", fmt.Sprintf("备份失败（继续执行修复）: %v", err))
			} else {
				o.eb.EmitLog("info", fmt.Sprintf("备份完成: %s (%d 文件)", backupResult.BackupDir, backupResult.FileCount))
			}
		}

		// 2. 清理
		o.eb.EmitProgress(20, "正在清理...")
		repairResult, err := repair.Repair(mcDir, repair.RepairConfig{Action: repair.ActionCleanAll})
		if err != nil {
			o.eb.EmitError("全量修复", fmt.Sprintf("清理失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}
		o.eb.EmitLog("info", fmt.Sprintf("已清理: %s", strings.Join(repairResult.CleanedDirs, ", ")))

		// 3. 获取最新版本信息
		o.eb.EmitProgress(40, "正在获取版本信息...")
		packState, _ := localCfg.Packs[packName]
		update, err := o.cfg.FetchUpdate(serverURL, packName, "", nil)
		if err != nil {
			o.eb.EmitError("全量修复", fmt.Sprintf("获取版本信息失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}

		updater := launcher.NewUpdater(o.cfgDir, mcDir, o.cfg)
		mcVersion := update.MCVersion
		loaderSpec := update.Loader
		if mcVersion != "" {
			o.eb.EmitProgress(50, "正在安装 MC...")
			versionDir := filepath.Join(mcDir, "versions")
			libraryDir := filepath.Join(mcDir, "libraries")
			_, _ = updater.EnsureVersion(launcher.EnsureRequest{
				MCVersion:  mcVersion,
				Loader:     loaderSpec,
				VersionDir: versionDir,
				LibraryDir: libraryDir,
			})
		}

		// 4. 全量拉取最新模组
		o.eb.EmitProgress(60, "正在下载模组文件...")
		result, err := updater.UpdatePack(serverURL, packName, &packState, true)
		if err != nil {
			o.eb.EmitError("全量修复", fmt.Sprintf("下载模组文件失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}

		// 5. 更新版本号到最新
		newVersion := result.Version
		o.vm.MarkSyncDone(newVersion)
		localCfg.Packs[packName] = model.PackState{
			Enabled:      true,
			Status:       "synced",
			LocalVersion: newVersion,
			Dir:          fmt.Sprintf("packs/%s", packName),
		}
		_ = o.cfg.SaveLocal(localCfg)

		o.eb.EmitProgress(100, "完成")
		o.eb.EmitLog("info", fmt.Sprintf("全量修复完成 → %s %s", packName, newVersion))
		o.vm.MarkIdle()
	}()
}

// DoRepairMC 修复 MC 本体：重新安装最新 MC + Loader
// 模组文件不动，版本号不更新
func (o *Orchestrator) DoRepairMC(mcDir string, withBackup bool) {
	o.vm.state.Transition(StateRepairing)

	go func() {
		packName := o.vm.SelectedPack()
		if packName == "" {
			o.eb.EmitError("修复", "未选中版本", nil)
			o.vm.MarkIdle()
			return
		}

		localCfg := o.vm.LocalConfig()
		serverURL := localCfg.ServerURL
		if serverURL == "" {
			o.eb.EmitError("修复", "未配置服务端地址", nil)
			o.vm.MarkIdle()
			return
		}

		// 备份（如需要）
		if withBackup {
			o.eb.EmitProgress(0, "正在备份...")
			_, err := repair.CreateBackup(mcDir, repair.BackupOptions{Reason: repair.ReasonRepair})
			if err != nil {
				o.eb.EmitLog("warn", fmt.Sprintf("备份失败（继续执行修复）: %v", err))
			} else {
				o.eb.EmitLog("info", "备份完成")
			}
		}

		// 获取 MC 版本和 Loader 信息
		o.eb.EmitProgress(20, "正在获取版本信息...")

		packState, _ := localCfg.Packs[packName]

		// 用空版本号查最新版信息
		update, err := o.cfg.FetchUpdate(serverURL, packName, "", nil)
		if err != nil {
			o.eb.EmitError("MC 修复", fmt.Sprintf("获取最新版本信息失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}
		_ = packState

		// 重新安装最新 MC + Loader（不管旧版本，直接装最新）
		o.eb.EmitProgress(40, "正在重新安装最新版 MC...")
		versionDir := filepath.Join(mcDir, "versions")
		libraryDir := filepath.Join(mcDir, "libraries")

		updater := launcher.NewUpdater(o.cfgDir, mcDir, o.cfg)
		mcVersion := update.MCVersion
		loaderSpec := update.Loader
		if mcVersion == "" {
			o.eb.EmitError("MC 修复", "服务端未返回 MC 版本信息", nil)
			o.vm.MarkIdle()
			return
		}
		o.eb.EmitLog("info", fmt.Sprintf("将安装最新 MC %s%s", mcVersion,
			map[bool]string{true: " + " + loaderSpec, false: ""}[loaderSpec != ""]))

		ensureResult, err := updater.EnsureVersion(launcher.EnsureRequest{
			MCVersion:  mcVersion,
			Loader:     loaderSpec,
			VersionDir: versionDir,
			LibraryDir: libraryDir,
		})
		if err != nil {
			o.eb.EmitError("MC 修复", fmt.Sprintf("MC 修复失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}

		o.eb.EmitProgress(100, "MC 本体修复完成")
		o.eb.EmitLog("info", fmt.Sprintf("MC 修复完成: 版本ID=%s（模组文件不变）", ensureResult.VersionID))
		o.vm.MarkIdle()
	}()
}

// DoRepairModsSync 模组同步：清空 mods/ → 全量拉取最新版 → 更新版本号到最新
func (o *Orchestrator) DoRepairModsSync(mcDir string, withBackup bool) {
	o.vm.state.Transition(StateRepairing)

	go func() {
		packName := o.vm.SelectedPack()
		if packName == "" {
			o.eb.EmitError("模组同步", "未选中版本", nil)
			o.vm.MarkIdle()
			return
		}

		localCfg := o.vm.LocalConfig()
		serverURL := localCfg.ServerURL
		if serverURL == "" {
			o.eb.EmitError("模组同步", "未配置服务端地址", nil)
			o.vm.MarkIdle()
			return
		}

		// 备份
		if withBackup {
			o.eb.EmitProgress(5, "正在备份 mods 目录...")
			_, err := repair.CreateBackup(mcDir, repair.BackupOptions{Reason: repair.ReasonRepair})
			if err != nil {
				o.eb.EmitLog("warn", fmt.Sprintf("备份失败（继续执行同步）: %v", err))
			} else {
				o.eb.EmitLog("info", "备份完成")
			}
		}

		// 清理 mods/
		o.eb.EmitProgress(15, "正在清理 mods 目录...")
		modsDir := filepath.Join(mcDir, "mods")
		_ = removeContents(modsDir)

		// 全量拉取最新
		o.eb.EmitProgress(30, "正在拉取最新模组...")
		updater := launcher.NewUpdater(o.cfgDir, mcDir, o.cfg)
		packState, _ := localCfg.Packs[packName]

		result, err := updater.UpdatePack(serverURL, packName, &packState, true)
		if err != nil {
			o.eb.EmitError("模组同步", fmt.Sprintf("拉取模组失败: %v", err), err)
			o.vm.MarkIdle()
			return
		}

		// 更新版本号到最新
		newVersion := result.Version
		localCfg.Packs[packName] = model.PackState{
			Enabled:      true,
			Status:       "synced",
			LocalVersion: newVersion,
			Dir:          fmt.Sprintf("packs/%s", packName),
		}
		_ = o.cfg.SaveLocal(localCfg)
		o.vm.MarkSyncDone(newVersion)

		o.eb.EmitProgress(100, "模组同步完成")
		o.eb.EmitLog("info", fmt.Sprintf("模组同步完成 → %s %s", packName, newVersion))
		o.vm.MarkIdle()
	}()
}

// DoCrashLogUpload 上传崩溃日志
func (o *Orchestrator) DoCrashLogUpload(mcDir string) {
	go func() {
		packName := o.vm.SelectedPack()
		if packName == "" {
			o.eb.EmitError("崩溃日志上传", "未选中版本", nil)
			return
		}

		localCfg := o.vm.LocalConfig()
		serverURL := localCfg.ServerURL
		if serverURL == "" {
			o.eb.EmitError("崩溃日志上传", "未配置服务端地址", nil)
			return
		}

		// 获取包详情中的 MC 版本信息
		var mcVersion string
		update, err := o.cfg.FetchUpdate(serverURL, packName, "", nil)
		if err == nil {
			mcVersion = update.MCVersion
		}

		opts := &repair.UploadOptions{
			MCVersion: mcVersion,
		}

		response, err := repair.CollectAndUpload(mcDir, o.cfgDir, packName, 0, "user requested upload", opts)
		if err != nil {
			o.eb.EmitError("崩溃日志上传", fmt.Sprintf("上传失败: %v", err), err)
			return
		}

		o.eb.EmitLog("info", fmt.Sprintf("崩溃日志上传成功: 状态=%s, 工单=%s", response.Status, response.Ticket))
	}()
}

// DoRestoreBackup 恢复指定备份
func (o *Orchestrator) DoRestoreBackup(mcDir, backupID string) {
	o.vm.state.Transition(StateRepairing)

	go func() {
		result, err := repair.Repair(mcDir, repair.RepairConfig{
			Action:     repair.ActionRollback,
			RollbackID: backupID,
		})
		if err != nil {
			o.eb.EmitError("恢复备份", fmt.Sprintf("恢复失败: %v", err), err)
		} else {
			o.eb.EmitLog("info", fmt.Sprintf("备份恢复完成: 恢复 %d 个文件", result.Restored))
		}
		o.vm.MarkIdle()
	}()
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
// 辅助
// ============================================================

// removeContents 清空目录下的所有内容（保留目录本身）
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0755)
		}
		return err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// ListBackups 获取可用备份列表（简化，返回备份 ID 列表）
func (o *Orchestrator) ListBackups(mcDir string) ([]repair.BackupListEntry, error) {
	return repair.ListBackups(mcDir)
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

// ============================================================
// EnsureVersion 快捷方法 — 供外部直接调用（修复窗口等）
// ============================================================

// EnsureMCVersion 安装指定 MC+Loader（异步报告进度）
func (o *Orchestrator) EnsureMCVersion(mcDir, mcVersion, loaderSpec string) error {
	versionDir := filepath.Join(mcDir, "versions")
	libraryDir := filepath.Join(mcDir, "libraries")
	updater := launcher.NewUpdater(o.cfgDir, mcDir, o.cfg)

	ensureReq := launcher.EnsureRequest{
		MCVersion:  mcVersion,
		Loader:     loaderSpec,
		VersionDir: versionDir,
		LibraryDir: libraryDir,
	}

	_, err := updater.EnsureVersion(ensureReq)
	return err
}

// ============================================================
// 版本信息获取
// ============================================================

// FetchPackDetail 获取包详情
func (o *Orchestrator) FetchPackDetail(packName string) (*model.PackDetail, error) {
	localCfg := o.vm.LocalConfig()
	if localCfg.ServerURL == "" {
		return nil, fmt.Errorf("未配置服务端地址")
	}
	return o.cfg.FetchPackDetail(localCfg.ServerURL, packName)
}

// ============================================================
// doRepair 通用修复执行（异步）
// ============================================================

func (o *Orchestrator) doRepair(mcDir string, cfg repair.RepairConfig, withBackup bool) {
	// 带备份的修复
	if withBackup {
		o.eb.EmitProgress(5, "正在备份用户数据...")
		result, err := repair.CreateBackup(mcDir, repair.BackupOptions{Reason: repair.ReasonRepair})
		if err != nil {
			o.eb.EmitLog("warn", fmt.Sprintf("备份失败（继续执行修复）: %v", err))
		} else {
			o.eb.EmitLog("info", fmt.Sprintf("备份完成: %s (%d 文件)", result.BackupDir, result.FileCount))
		}
	}

	// 执行修复
	o.eb.EmitProgress(30, "正在清理...")
	repairResult, err := repair.Repair(mcDir, cfg)
	if err != nil {
		o.eb.EmitError("修复", fmt.Sprintf("修复失败: %v", err), err)
		o.vm.MarkIdle()
		return
	}

	// 检查清理结果
	if len(repairResult.Errors) > 0 {
		o.eb.EmitLog("warn", fmt.Sprintf("修复过程中有 %d 个错误:\n%s",
			len(repairResult.Errors),
			strings.Join(repairResult.Errors, "\n")))
	}

	cleanedDirs := repairResult.CleanedDirs
	o.eb.EmitProgress(80, "清理完成")
	if len(cleanedDirs) > 0 {
		o.eb.EmitLog("info", fmt.Sprintf("已清理: %s", strings.Join(cleanedDirs, ", ")))
	}

	// 全量修复完成 → 提示用户点安装
	o.eb.EmitProgress(100, "清理完成")
	o.eb.EmitLog("info", "全量修复完成，请返回主窗口安装版本")
	o.vm.MarkIdle()
}
