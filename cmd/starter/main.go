package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/gege-tlph/mc-starter/internal/pack"
)

var version = "dev"

func main() {
	fs := flag.NewFlagSet("starter", flag.ExitOnError)
	cfgDir := fs.String("config", "./config", "配置目录")
	verbose := fs.Bool("verbose", false, "详细日志")
	verboseShort := fs.Bool("v", false, "详细日志")
	headless := fs.Bool("headless", false, "静默模式")
	dryRun := fs.Bool("dry-run", false, "仅检查不下载")

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "run":
		fs.Parse(os.Args[2:])
		run(*cfgDir, *verbose || *verboseShort, *headless, *dryRun)
	case "init":
		fs.Parse(os.Args[2:])
		initialize(*cfgDir)
	case "check":
		fs.Parse(os.Args[2:])
		check(*cfgDir, *verbose || *verboseShort)
	case "sync":
		fs.Parse(os.Args[2:])
		sync(*cfgDir, *verbose || *verboseShort, *dryRun)
	case "repair":
		fs.Parse(os.Args[2:])
		repair(*cfgDir, *headless)
	case "backup":
		handleBackup(os.Args[2:])
	case "cache":
		handleCache(os.Args[2:])
	case "pack":
		handlePack(os.Args[2:])
	case "pcl":
		handlePCL(os.Args[2:])
	case "version":
		fmt.Printf("mc-starter %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(strings.TrimSpace(`
mc-starter — Windows 版 Minecraft 版本管理 & 整合包更新器

用法:
  starter run      全自动: 检测 -> 同步 -> 拉起 PCL2
  starter init     初始化本地配置
  starter check    检查 Java / PCL2 / 配置完整性
  starter sync     仅同步版本 + 模组
  starter repair   修复工具
  starter backup   备份管理
    list           列出备份
    restore <name> 恢复指定快照
    create         手动创建备份
    delete <name>  删除快照
  starter cache    缓存管理
    stats          显示缓存统计
    clean [--dry-run] [--min-ref <n>]  清理缓存
    prune [--dry-run]  清理 orphaned 缓存
  starter pcl      操作 PCL2
    detect         检测 PCL2.exe 位置
    path <path>    设置 PCL2 路径
  starter version  显示版本信息
  starter help     显示此帮助

全局选项:
  --config <dir>   配置目录 (默认 ./config)
  --verbose, -v    详细日志
  --headless       静默模式
  --dry-run        仅检查不下载
`))
}

func run(cfgDir string, verbose bool, headless bool, dryRun bool) {
	logger.Init(verbose)
	logger.Info("run: 全自动模式")

	// 1. 初始化配置（如果不存在则创建）
	if err := ensureConfig(cfgDir); err != nil {
		logger.Error("配置初始化失败: %v", err)
		return
	}

	// 2. 拉取版本清单
	manifestDir := filepath.Join(cfgDir, ".cache", "manifest")
	mm := launcher.NewVersionManifestManager(manifestDir)
	manifest, err := mm.Fetch(30 * time.Minute)
	if err != nil {
		logger.Error("版本清单拉取失败: %v", err)
		return
	}

	if !headless {
		fmt.Printf("最新版本: release=%s  snapshot=%s\n", manifest.Latest.Release, manifest.Latest.Snapshot)
		fmt.Printf("共 %d 个版本可用\n", len(manifest.Versions))
	}

	// TODO: 后续步骤 — 读 server.json → 确定目标版本 → 下载
	logger.Info("run: 版本清单就绪, 等待 P1 功能完善")
	fmt.Println("run: 版本清单已同步，后续功能开发中")
}

func initialize(cfgDir string) {
	logger.Init(false)

	dir, err := filepath.Abs(cfgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取配置目录失败: %v\n", err)
		return
	}

	mg := config.New(dir)

	// 读取现有的，如果有则提示
	existing, err := mg.LoadLocal()
	if err == nil && existing.InstallPath != "" {
		fmt.Printf("配置已存在: %s\n", dir)
		fmt.Printf("如需重新初始化，请删除 %s 后重试\n", filepath.Join(dir, "local.json"))
		return
	}

	// 生成默认配置
	local := &model.LocalConfig{
		InstallPath: ".minecraft",
		Launcher:    "bare",
		Memory:      4096,
		Username:    "Player",
	}

	if err := mg.SaveLocal(local); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置失败: %v\n", err)
		return
	}

	fmt.Printf("初始化完成: %s\n", dir)
	fmt.Println("请编辑 local.json 修改安装路径等配置")
}

func check(cfgDir string, verbose bool) {
	logger.Init(verbose)
	logger.Info("check: 系统检查")

	manifestDir := filepath.Join(cfgDir, ".cache", "manifest")
	mm := launcher.NewVersionManifestManager(manifestDir)

	fmt.Println("=== 系统检查 ===")

	// 1. 检查配置
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Printf("[✗] 本地配置: %v\n", err)
	} else {
		fmt.Printf("[✓] 本地配置: %s\n", cfgDir)
		fmt.Printf("    安装目录: %s\n", localCfg.InstallPath)
		fmt.Printf("    启动器: %s\n", localCfg.Launcher)
		fmt.Printf("    内存: %d MB\n", localCfg.Memory)
	}

	// 2. 尝试拉取版本清单
	manifest, err := mm.Fetch(30 * time.Minute)
	if err != nil {
		fmt.Printf("[✗] 版本清单: %v\n", err)
	} else {
		fmt.Printf("[✓] 版本清单: %d 个版本\n", len(manifest.Versions))
		fmt.Printf("    最新: release=%s  snapshot=%s\n", manifest.Latest.Release, manifest.Latest.Snapshot)
	}

	// 3. 检查安装目录
	if localCfg != nil && localCfg.InstallPath != "" {
		if info, err := os.Stat(localCfg.InstallPath); err == nil {
			fmt.Printf("[✓] 安装目录: %s (%d MB 可用)\n", localCfg.InstallPath, info.Size()/1024/1024)
		} else {
			fmt.Printf("[!] 安装目录不存在: %s\n", localCfg.InstallPath)
		}
	}

	// TODO: Java 检测（P3）
	fmt.Println("[…] Java 检测: 待实现 (P3)")
}

func sync(cfgDir string, verbose bool, dryRun bool) {
	logger.Init(verbose)
	logger.Info("sync: 开始同步")

	cacheDir := filepath.Join(cfgDir, ".cache")
	manifestDir := filepath.Join(cacheDir, "manifest")
	versionsDir := filepath.Join(cacheDir, "versions")
	jarDir := filepath.Join(cfgDir, "jars")

	mm := launcher.NewVersionManifestManager(manifestDir)
	vm := launcher.NewVersionMetaManager(versionsDir, mm)

	// 0. 拉取最新版本清单 + 确定目标版本（断点恢复的版本从目标版本匹配）
	manifest, err := mm.Fetch(30 * time.Minute)
	if err != nil {
		logger.Error("版本清单拉取失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: 版本清单拉取失败: %v\n", err)
		return
	}

	mg := config.New(cfgDir)
	serverCfg, err := mg.LoadServer()
	var targetVersion string
	if err == nil && serverCfg.Version.ID != "" {
		targetVersion = serverCfg.Version.ID
	} else {
		targetVersion = manifest.Latest.Release
	}

	logger.Info("sync: 目标版本 %s", targetVersion)

	fmt.Printf("sync: 版本清单 (%d 个版本), 目标 %s\n", len(manifest.Versions), targetVersion)

	// 1. 尝试断点恢复：读取之前的 sync_state.json
	state := launcher.LoadSyncState(cacheDir, targetVersion)
	if state != nil {
		if state.IsStale() {
			logger.Info("sync 状态已过期(>1h)，从头开始")
			state.Reset()
		} else {
			fmt.Printf("[*] 断点恢复: 已完成 %d 个阶段, 从断点继续\n", len(state.Completed))
		}
	} else {
		state = launcher.NewSyncState(cacheDir, targetVersion)
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] 将同步版本 %s\n", targetVersion)
		return
	}

	fmt.Printf("\n=== 同步版本 %s ===\n", targetVersion)

	// 获取版本元数据（阶段 3/4 共用）
	meta, err := vm.Fetch(targetVersion)
	if err != nil {
		logger.Error("获取版本元数据失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: 获取 %s 元数据失败: %v\n", targetVersion, err)
		return
	}

	// =============================================
	// 4. 版本元数据
	// =============================================
	if !state.HasCompleted(launcher.PhaseVersionMeta) {
		fmt.Printf("[✓] 版本元数据: %s (type=%s)\n", meta.ID, meta.Type)
		fmt.Printf("    mainClass: %s\n", meta.MainClass)
		fmt.Printf("    assets: %s\n", meta.Assets)
		if meta.Downloads != nil && meta.Downloads.Client != nil {
			fmt.Printf("    client.jar: %d MB (SHA1: %s)\n",
				meta.Downloads.Client.Size/1024/1024,
				meta.Downloads.Client.Sha1[:12]+"...")
		}
		state.MarkCompleted(launcher.PhaseVersionMeta)
	} else {
		fmt.Printf("[*] 跳过: 版本元数据已获取\n")
	}

	// =============================================
	// 5. client.jar 下载（启用增量缓存: 先查 CacheStore）
	// =============================================
	if !state.HasCompleted(launcher.PhaseClientJar) {
		// 读取 local config 获取安装目录
		localCfg, _ := mg.LoadLocal()
		installPath := ".minecraft"
		if localCfg != nil && localCfg.InstallPath != "" {
			installPath = localCfg.InstallPath
		}
		is := launcher.NewIncrementalSync(cfgDir, installPath)

		jarPath := ""
		if meta.Downloads != nil && meta.Downloads.Client != nil {
			// 尝试从缓存获取
			clientSHA1 := meta.Downloads.Client.Sha1
			cachedJar := is.TryCacheClientJar(clientSHA1)
			if cachedJar != "" {
				// 复制到目标路径
				jarPath = filepath.Join(jarDir, fmt.Sprintf("%s.jar", meta.ID))
				if err := os.MkdirAll(filepath.Dir(jarPath), 0755); err == nil {
					data, readErr := os.ReadFile(cachedJar)
					if readErr == nil {
						if writeErr := os.WriteFile(jarPath, data, 0644); writeErr == nil {
							fmt.Printf("[✓] client.jar (缓存): %s\n", jarPath)
						} else {
							cachedJar = ""
						}
					} else {
						cachedJar = ""
					}
				}
			}
		}

		if jarPath == "" {
			jarPath, err = vm.DownloadClientJar(meta, jarDir)
			if err != nil {
				logger.Error("下载 client.jar 失败: %v", err)
				fmt.Fprintf(os.Stderr, "sync: client.jar 下载失败: %v\n", err)
				return
			}
			// 缓存下载的 client.jar
			if meta.Downloads != nil && meta.Downloads.Client != nil {
				is.CacheClientJar(meta.Downloads.Client.Sha1, jarPath)
			}
		}
		_ = is
		fmt.Printf("[✓] client.jar: %s\n", jarPath)
		state.MarkCompleted(launcher.PhaseClientJar)
	} else {
		fmt.Printf("[*] 跳过: client.jar 已下载\n")
	}

	// =============================================
	// 6. Asset 索引同步
	// =============================================
	assetsDir := filepath.Join(cfgDir, "assets")
	am := launcher.NewAssetManager(cacheDir, assetsDir, mm, vm)

	if !state.HasCompleted(launcher.PhaseAssetIndex) {
		assetIdx, err := am.FetchIndex(targetVersion)
		if err != nil {
			logger.Error("Asset 索引拉取失败: %v", err)
			fmt.Fprintf(os.Stderr, "sync: Asset 索引拉取失败: %v\n", err)
			return
		}

		stats := am.Statistics(assetIdx)
		fmt.Printf("[✓] Asset 索引: %s (%d 个文件, 总计 %d MB, 平均 %.1f KB)\n",
			meta.Assets, stats.TotalFiles, stats.TotalSize/1024/1024, stats.AvgSize/1024)
		state.MarkCompleted(launcher.PhaseAssetIndex)
	} else {
		fmt.Printf("[*] 跳过: Asset 索引已同步\n")
	}

	// =============================================
	// 7. Asset 文件下载（增量: 先查 CacheStore 再并发下载）
	// =============================================
	if !state.HasCompleted(launcher.PhaseAssetFiles) {
		assetIdx, err := am.FetchIndex(targetVersion)
		if err != nil {
			logger.Error("Asset 索引拉取失败(阶段4): %v", err)
			fmt.Fprintf(os.Stderr, "sync: Asset 索引拉取失败: %v\n", err)
			return
		}

		// 初始化 IncrementalSync（如果尚未初始化）
		localCfg, _ := mg.LoadLocal()
		installPath := ".minecraft"
		if localCfg != nil && localCfg.InstallPath != "" {
			installPath = localCfg.InstallPath
		}
		is := launcher.NewIncrementalSync(cfgDir, installPath)
		_ = is

		assetFiles := am.ListObjects(assetIdx)
		logger.Info("开始 Asset 文件下载 (8 workers, %d 个文件, 增量缓存)...", len(assetFiles))

		type assetResult struct {
			downloaded int
			cached     int
			skipped    int
			failed     int
		}
		resultCh := make(chan assetResult, len(assetFiles))
		sem := make(chan struct{}, 8)
		// 并发下载 Asset 文件，使用 worker pool 模式：
		//   - sem (channel) 作为信号量，同时最多 8 个 goroutine 持有 slot
		//   - 每个文件先查 CacheStore，命中则直接复制；未命中则下载并存入缓存

		for _, obj := range assetFiles {
			go func(vpath, hash string) {
				sem <- struct{}{}
				defer func() { <-sem }()

				localPath := am.AssetObjectPath(hash)

				// step 1: 检查本地磁盘
				if _, err := os.Stat(localPath); err == nil {
					resultCh <- assetResult{skipped: 1}
					return
				}

				// step 2: 检查 CacheStore
				if is.AssetFromCache(hash, localPath) {
					resultCh <- assetResult{cached: 1}
					return
				}

				// step 3: 下载
				if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
					logger.Warn("创建 Asset 目录失败: %v", err)
					resultCh <- assetResult{failed: 1}
					return
				}
				if err := am.DownloadFile(hash, localPath); err != nil {
					logger.Warn("下载 Asset 失败: %s (%v)", vpath, err)
					resultCh <- assetResult{failed: 1}
					return
				}

				// 存入 CacheStore
				is.StoreAsset(hash, localPath)
				resultCh <- assetResult{downloaded: 1}
			}(obj.VirtualPath, obj.Hash)
		}

		var totalDownloaded, totalCached, totalSkipped, totalFailed int
		for i := 0; i < len(assetFiles); i++ {
			r := <-resultCh
			totalDownloaded += r.downloaded
			totalCached += r.cached
			totalSkipped += r.skipped
			totalFailed += r.failed
		}
		fmt.Printf("[✓] Asset 文件: %d 下载, %d 缓存命中, %d 已存在, %d 失败\n",
			totalDownloaded, totalCached, totalSkipped, totalFailed)

		state.SetAssetCount(totalDownloaded + totalCached)
		state.MarkCompleted(launcher.PhaseAssetFiles)
	} else {
		fmt.Printf("[*] 跳过: Asset 文件已下载\n")
	}

	// =============================================
	// 8. Libraries 下载（增量: 先查 CacheStore 再批量下载）
	// =============================================
	if !state.HasCompleted(launcher.PhaseLibraries) || !state.HasCompleted(launcher.PhaseNatives) {
		libraryDir := filepath.Join(cfgDir, "libraries")
		nativesDir := filepath.Join(cfgDir, "versions", targetVersion, "natives")
		lm := launcher.NewLibraryManager(libraryDir, nativesDir)

		// 初始化 IncrementalSync
		localCfg, _ := mg.LoadLocal()
		installPath := ".minecraft"
		if localCfg != nil && localCfg.InstallPath != "" {
			installPath = localCfg.InstallPath
		}
		is := launcher.NewIncrementalSync(cfgDir, installPath)

		fmt.Printf("\n=== 同步 Libraries ===\n")
		resolvedLibs, err := vm.ResolveLibraries(meta, filepath.Join(cfgDir, "versions"))
		if err != nil {
			logger.Error("解析 Libraries 失败: %v", err)
			fmt.Fprintf(os.Stderr, "sync: Libraries 解析失败: %v\n", err)
		} else if len(resolvedLibs) > 0 {
			libFiles := lm.ResolveToFiles(resolvedLibs)
			fmt.Printf("Libraries 条目: %d（解析为 %d 个文件）\n", len(resolvedLibs), len(libFiles))

			if !state.HasCompleted(launcher.PhaseLibraries) {
				// 拆分为"需下载"和"从缓存复制"
				toDownload, fromCache := is.ConsumeLibraryFiles(libFiles)
				fmt.Printf(" 缓存命中: %d 个文件\n", fromCache)

				// 下载剩余文件（跳过已存在的）
				downloaded, skipped, failed := lm.DownloadFiles(toDownload)
				fmt.Printf("[✓] Libraries 下载: %d 下载, %d 已存在, %d 失败\n", downloaded, skipped, failed)

				// 将新下载的存入缓存
				for _, f := range toDownload {
					if f.SHA1 != "" {
						is.StoreLibrary(f.SHA1, f.LocalPath)
					}
				}

				state.SetLibraryCount(downloaded + fromCache)
				state.MarkCompleted(launcher.PhaseLibraries)
			} else {
				fmt.Printf("[*] 跳过: Libraries 已下载\n")
			}

			if !state.HasCompleted(launcher.PhaseNatives) {
				extracted, extractErrs := lm.ExtractNativesFromFiles(libFiles)
				if len(extractErrs) > 0 {
					for _, e := range extractErrs {
						logger.Warn("natives 解压错误: %v", e)
					}
				}
				fmt.Printf("[✓] Natives 解压: %d 完成, %d 错误\n", extracted, len(extractErrs))
				state.MarkCompleted(launcher.PhaseNatives)
			} else {
				fmt.Printf("[*] 跳过: Natives 已解压\n")
			}
		} else {
			fmt.Println("[!] 该版本没有 Libraries 信息")
			state.MarkCompleted(launcher.PhaseLibraries)
			state.MarkCompleted(launcher.PhaseNatives)
		}
	} else {
		fmt.Printf("[*] 跳过: Libraries 和 Natives 已完成\n")
	}

	// =============================================
	// 9. 增量同步收尾 — 创建/更新 repo 快照
	// =============================================
	{
		localCfg, _ := mg.LoadLocal()
		installPath := ".minecraft"
		if localCfg != nil && localCfg.InstallPath != "" {
			installPath = localCfg.InstallPath
		}

		is := launcher.NewIncrementalSync(cfgDir, installPath)

		// 初始化 repo
		if err := is.EnsureRepo(targetVersion); err != nil {
			logger.Warn("repo 初始化失败(非致命): %v", err)
		} else {
			// 检查是否有旧快照，有则做增量差异
			if is.LocalRepo().HasSnapshots() {
				latestName := is.LocalRepo().LatestSnapshot()
				logger.Info("检测到旧快照: %s, 计算增量差异", latestName)

				snapshotName := fmt.Sprintf("sync-%s", time.Now().Format("20060102-150405"))
				diff, err := is.DiffSinceSnapshot(latestName, []string{"mods", "config"})
				if err != nil {
					logger.Warn("增量差异计算失败(非致命): %v", err)
				} else if diff != nil && (len(diff.Added) > 0 || len(diff.Updated) > 0 || len(diff.Deleted) > 0) {
					fmt.Printf("[Δ] 增量变化: +%d, ~%d, -%d, =%d\n",
						len(diff.Added), len(diff.Updated), len(diff.Deleted), diff.Unchanged)

					// 创建增量快照
					if len(diff.Added) > 0 || len(diff.Updated) > 0 {
						if err := is.CreateSyncSnapshot(snapshotName, []string{"mods", "config"}); err != nil {
							logger.Warn("创建增量快照失败(非致命): %v", err)
						} else {
							fmt.Printf("[✓] 增量快照: %s\n", snapshotName)
						}
					}
				} else {
					fmt.Printf("[Δ] 无增量变化 (已是最新)\n")
				}
			} else {
				// 无旧快照，创建全量快照
				snapshotName := fmt.Sprintf("initial-%s", time.Now().Format("20060102-150405"))
				if err := is.CreateSyncSnapshot(snapshotName, []string{"mods", "config"}); err != nil {
					logger.Warn("创建全量快照失败(非致命): %v", err)
				} else {
					fmt.Printf("[✓] 全量快照: %s\n", snapshotName)
				}
			}

			logger.Debug("repo stats: %s", is.SyncStats())
		}
	}

	// 标记全部完成并清理状态文件
	state.MarkCompleted(launcher.PhaseComplete)
	state.Remove()

	logger.Info("sync: 完成")
	fmt.Printf("\nsync: 完成\n")
}

func repair(cfgDir string, headless bool) {
	logger.Init(false)
	fmt.Println("repair: not yet implemented")
}

func handleBackup(args []string) {
	if len(args) == 0 {
		fmt.Println("backup: subcommand required (list | restore | create | delete)")
		return
	}
	switch args[0] {
	case "list":
		handleBackupList(args[1:])
	case "restore":
		handleBackupRestore(args[1:])
	case "create":
		handleBackupCreate(args[1:])
	case "delete":
		handleBackupDelete(args[1:])
	default:
		fmt.Printf("backup: unknown subcommand %s\n", args[0])
	}
}

func handleBackupList(args []string) {
	cfgDir := "config"
	if len(args) >= 2 && args[0] == "--config" {
		cfgDir = args[1]
	}
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	installPath := ".minecraft"
	if localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	repo := launcher.NewLocalRepo(installPath)
	if !repo.HasSnapshots() {
		fmt.Println("没有快照")
		return
	}

	snapshots, err := repo.ListSnapshots()
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出快照失败: %v\n", err)
		return
	}

	fmt.Printf("快照列表 (%d 个):\n", len(snapshots))
	for i, name := range snapshots {
		meta, err := repo.LoadSnapshotMeta(name)
		if err != nil {
			fmt.Printf("  %d. %s (读取失败: %v)\n", i+1, name, err)
			continue
		}
		fmt.Printf("  %d. %s — %d 个文件, %.1f MB, %s\n",
			i+1, name, meta.FileCount, float64(meta.TotalSize)/1024/1024,
			meta.CreatedAt.Format("2006-01-02 15:04:05"))
	}
}

func handleBackupRestore(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: starter backup restore <snapshot_name> [--config <dir>]")
		return
	}
	snapshotName := args[0]
	cfgDir := "config"
	for i := 1; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			cfgDir = args[i+1]
			i++
		}
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	installPath := ".minecraft"
	if localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	repo := launcher.NewLocalRepo(installPath)
	if !repo.HasSnapshot(snapshotName) {
		fmt.Fprintf(os.Stderr, "快照 %s 不存在\n", snapshotName)
		return
	}

	fmt.Printf("恢复快照 %s 到 %s ...\n", snapshotName, installPath)
	if err := repo.RestoreSnapshot(snapshotName, installPath); err != nil {
		fmt.Fprintf(os.Stderr, "恢复失败: %v\n", err)
		return
	}
	fmt.Printf("[✓] 快照 %s 已恢复\n", snapshotName)
}

func handleBackupCreate(args []string) {
	cfgDir := "config"
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			cfgDir = args[i+1]
			i++
		}
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	installPath := ".minecraft"
	if localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	repo := launcher.NewLocalRepo(installPath)
	if !repo.IsInitialized() {
		repo.Init("unknown")
	}

	is := launcher.NewIncrementalSync(cfgDir, installPath)
	snapshotName := fmt.Sprintf("manual-%s", time.Now().Format("20060102-150405"))

	if err := is.CreateSyncSnapshot(snapshotName, []string{"mods", "config"}); err != nil {
		fmt.Fprintf(os.Stderr, "创建备份失败: %v\n", err)
		return
	}

	meta, _ := repo.LoadSnapshotMeta(snapshotName)
	if meta != nil {
		fmt.Printf("[✓] 备份 %s 已创建: %d 个文件, %.1f MB\n",
			snapshotName, meta.FileCount, float64(meta.TotalSize)/1024/1024)
	} else {
		fmt.Printf("[✓] 备份 %s 已创建\n", snapshotName)
	}
}

func handleBackupDelete(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: starter backup delete <snapshot_name> [--config <dir>]")
		return
	}
	snapshotName := args[0]
	cfgDir := "config"
	for i := 1; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			cfgDir = args[i+1]
			i++
		}
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	installPath := ".minecraft"
	if localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	repo := launcher.NewLocalRepo(installPath)
	if !repo.HasSnapshot(snapshotName) {
		fmt.Fprintf(os.Stderr, "快照 %s 不存在\n", snapshotName)
		return
	}

	if err := repo.DeleteSnapshot(snapshotName); err != nil {
		fmt.Fprintf(os.Stderr, "删除失败: %v\n", err)
		return
	}
	fmt.Printf("[✓] 快照 %s 已删除\n", snapshotName)
}

func handleCache(args []string) {
	if len(args) == 0 {
		fmt.Println("cache: subcommand required (stats | clean | prune)")
		return
	}

	cfgDir := "config"
	// 从 args 中解析 --config
	remaining := args
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--config" && i+1 < len(remaining) {
			cfgDir = remaining[i+1]
			i++
		}
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	installPath := ".minecraft"
	if localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	switch args[0] {
	case "stats":
		cacheDir := filepath.Join(cfgDir, ".cache", "mc_cache")
		cs := launcher.NewCacheStore(cacheDir)
		fmt.Println(cs.Stats())

	case "clean":
		dryRun := false
		minRef := 0
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--dry-run":
				dryRun = true
			case "--min-ref":
				if i+1 < len(args) {
					fmt.Sscanf(args[i+1], "%d", &minRef)
					i++
				}
			}
		}
		cacheDir := filepath.Join(cfgDir, ".cache", "mc_cache")
		cs := launcher.NewCacheStore(cacheDir)
		opts := launcher.CleanOptions{
			DryRun:      dryRun,
			MinRefCount: minRef,
		}
		deleted, freed, errs := cs.Clean(opts)
		if len(errs) > 0 {
			for _, e := range errs {
				logger.Warn("清理错误: %v", e)
			}
		}
		if dryRun {
			fmt.Printf("[DRY-RUN] 将删除 %d 个文件, 释放 %.1f KB\n", deleted, float64(freed)/1024)
		} else {
			fmt.Printf("[✓] 缓存清理: 删除 %d 个文件, 释放 %.1f KB\n", deleted, float64(freed)/1024)
		}

	case "prune":
		dryRun := false
		for i := 1; i < len(args); i++ {
			if args[i] == "--dry-run" {
				dryRun = true
			}
		}
		is := launcher.NewIncrementalSync(cfgDir, installPath)
		deleted, freed, errs := is.CleanOrphaned(dryRun)
		if len(errs) > 0 {
			for _, e := range errs {
				logger.Warn("prune 错误: %v", e)
			}
		}
		if dryRun {
			fmt.Printf("[DRY-RUN] 将删除 %d 个 orphaned 文件, 释放 %.1f KB\n", deleted, float64(freed)/1024)
		} else {
			fmt.Printf("[✓] Orphaned 清理: 删除 %d 个文件, 释放 %.1f KB\n", deleted, float64(freed)/1024)
		}

	default:
		fmt.Printf("cache: unknown subcommand %s\n", args[0])
	}
}

func handlePCL(args []string) {
	if len(args) == 0 {
		fmt.Println("pcl: subcommand required (detect | path)")
		return
	}
	switch args[0] {
	case "detect":
		fmt.Println("pcl detect: not yet implemented")
	case "path":
		if len(args) < 2 {
			fmt.Println("pcl path: path required")
			return
		}
		fmt.Printf("pcl path: set to %s (not yet implemented)\n", args[1])
	default:
		fmt.Printf("pcl: unknown subcommand %s\n", args[0])
	}
}

func handlePack(args []string) {
	if len(args) == 0 {
		fmt.Println("pack: subcommand required (sync | diff)")
		return
	}
	switch args[0] {
	case "sync":
		handlePackSync(args[1:])
	case "diff":
		handlePackDiff(args[1:])
	case "extract":
		handlePackExtract(args[1:])
	default:
		fmt.Printf("pack: unknown subcommand %s\n", args[0])
	}
}

func handlePackSync(args []string) {
	fs := flag.NewFlagSet("pack sync", flag.ExitOnError)
	cfgDir := fs.String("config", "./config", "配置目录")
	verbose := fs.Bool("verbose", false, "详细日志")
	dryRun := fs.Bool("dry-run", false, "仅检查不下载")
	hash := fs.String("hash", "", "期望的 SHA256 校验值")
	fs.Parse(args)

	if *verbose {
		logger.Init(true)
	}

	// 读取 server.json
	mg := config.New(*cfgDir)
	serverCfg, err := mg.LoadServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 server.json 失败: %v\n", err)
		fmt.Println("请先创建 config/server.json")
		return
	}

	if len(serverCfg.Modpacks) == 0 {
		fmt.Println("server.json 中未配置 modpacks")
		return
	}

	// 读取 install path
	localCfg, _ := mg.LoadLocal()
	installDir := ".minecraft"
	if localCfg.InstallPath != "" {
		installDir = localCfg.InstallPath
	}

	sm := pack.NewSyncManager()

	for _, mp := range serverCfg.Modpacks {
		if mp.Source != "url" || mp.Slug == "" {
			continue
		}
		// 根据 Source 确定 URL: "url" 类型从 Files[0] 取 URL
		sourceURL := ""
		if len(mp.Files) > 0 {
			sourceURL = mp.Files[0]
		}
		if sourceURL == "" {
			fmt.Printf("[!] modpack %s: 未配置 source URL\n", mp.Slug)
			continue
		}

		effectiveHash := *hash
		if effectiveHash == "" && serverCfg.SelfUpdate != nil && serverCfg.SelfUpdate.Version != "" {
			effectiveHash = serverCfg.SelfUpdate.Version
		}

		fmt.Printf("\n=== 同步整合包: %s ===\n", mp.Slug)
		logger.Info("pack sync: %s (%s)", mp.Slug, sourceURL)

		if *dryRun {
			// dry-run 只下载+计算，不应用
			tempDir := filepath.Join(installDir, ".starter_cache", "pack", mp.Slug)
			handler := pack.NewZipHandler()
			if effectiveHash != "" {
				handler.WithHash(effectiveHash)
			}
			result, err := handler.DownloadAndExtract(sourceURL, tempDir, mp.Slug)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[✗] %s: %v\n", mp.Slug, err)
				continue
			}
			defer result.Cleanup()

			fmt.Printf("  解压了 %d 个文件\n", len(result.Entries))

			// 如果没有指定 targets，默认 mods 和 config
			targets := []string{"mods", "config"}
			for _, target := range targets {
				diff := pack.ComputeDiff(result.Entries, installDir, target)
				if diff.HasChanges() {
					fmt.Printf("  [%s] %s\n", target, diff.Summary())
					pack.PrintPendingSyncDiff(map[string]*pack.DiffResult{target: diff})
				} else {
					fmt.Printf("  [%s] 已是最新\n", target)
				}
			}
			continue
		}

		// 执行同步
		syncResult, err := sm.SyncFromURL(sourceURL, installDir, mp.Slug, []string{"mods", "config"}, effectiveHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[✗] %s 同步失败: %v\n", mp.Slug, err)
			continue
		}
		fmt.Printf("  %s\n", syncResult.Summary())
	}

	fmt.Println("\npack sync: 完成")
}

func handlePackDiff(args []string) {
	fs := flag.NewFlagSet("pack diff", flag.ExitOnError)
	cfgDir := fs.String("config", "./config", "配置目录")
	zipPath := fs.String("zip", "", "本地 zip 文件路径")
	fs.Parse(args)

	if *zipPath == "" {
		fmt.Println("用法: starter pack diff --zip <path> [--config <dir>]")
		return
	}

	mg := config.New(*cfgDir)
	localCfg, _ := mg.LoadLocal()
	installDir := ".minecraft"
	if localCfg.InstallPath != "" {
		installDir = localCfg.InstallPath
	}

	handler := pack.NewZipHandler()
	tempDir := filepath.Join(installDir, ".starter_cache", "pack", "diff-tmp")
	result, err := handler.ExtractExisting(*zipPath, tempDir, "diff")
	if err != nil {
		fmt.Fprintf(os.Stderr, "解压失败: %v\n", err)
		return
	}
	defer result.Cleanup()

	fmt.Printf("zip 文件: %s (%d 个文件)\n", *zipPath, len(result.Entries))

	for _, target := range []string{"mods", "config"} {
		diff := pack.ComputeDiff(result.Entries, installDir, target)
		fmt.Printf("\n=== [%s] %s ===\n", target, diff.Summary())
		if diff.HasChanges() {
			pack.PrintPendingSyncDiff(map[string]*pack.DiffResult{target: diff})
		} else {
			fmt.Println("  无变更")
		}
	}
}

func handlePackExtract(args []string) {
	fs := flag.NewFlagSet("pack extract", flag.ExitOnError)
	zipPath := fs.String("zip", "", "本地 zip 文件路径")
	outDir := fs.String("out", "", "输出目录（默认 zip 同级）")
	fs.Parse(args)

	if *zipPath == "" {
		fmt.Println("用法: starter pack extract --zip <path> [--out <dir>]")
		return
	}

	dest := *outDir
	if dest == "" {
		dest = filepath.Join(filepath.Dir(*zipPath), "extracted")
	}

	handler := pack.NewZipHandler()
	result, err := handler.ExtractExisting(*zipPath, dest, "extract")
	if err != nil {
		fmt.Fprintf(os.Stderr, "解压失败: %v\n", err)
		return
	}
	defer result.Cleanup()

	fmt.Printf("解压完成: %d 个文件 → %s\n", len(result.Entries), result.TempRoot)

	// 列一些文件
	for _, entry := range result.Entries[:min(len(result.Entries), 10)] {
		fmt.Printf("  %s (%d KB)\n", entry.RelPath, entry.Size/1024)
	}
	if len(result.Entries) > 10 {
		fmt.Printf("  ... (%d more)\n", len(result.Entries)-10)
	}
}

// ensureConfig 确保配置文件存在，不存在则创建默认配置
func ensureConfig(cfgDir string) error {
	mg := config.New(cfgDir)
	_, err := mg.LoadLocal()
	if err != nil {
		// 生成默认配置
		local := &model.LocalConfig{
			InstallPath: ".minecraft",
			Launcher:    "bare",
			Memory:      4096,
			Username:    "Player",
		}
		if err := mg.SaveLocal(local); err != nil {
			return fmt.Errorf("创建默认配置: %w", err)
		}
		logger.Info("已创建默认配置: %s", cfgDir)
	}
	return nil
}
