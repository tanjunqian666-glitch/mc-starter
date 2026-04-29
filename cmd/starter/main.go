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
	"github.com/gege-tlph/mc-starter/internal/tui"
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
		// 无参数: 双击场景 → 启动 TUI 全自动
		if err := tui.RunTUI("./config", false); err != nil {
			fmt.Fprintf(os.Stderr, "TUI 错误: %v\n", err)
			os.Exit(1)
		}
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

// run 全自动模式: 检测 → 同步 → 拉起启动器
func run(cfgDir string, verbose bool, headless bool, dryRun bool) {
	logger.Init(verbose)
	logger.Info("run: 全自动模式")
	fmt.Println("=== 全自动模式 ===")

	// 1. 初始化配置（如果不存在则创建）
	if err := ensureConfig(cfgDir); err != nil {
		logger.Error("配置初始化失败: %v", err)
		fmt.Fprintf(os.Stderr, "run: 配置初始化失败: %v\n", err)
		return
	}

	// 2. 读取本地配置，查找 PCL2 和 .minecraft 配置
	mg := config.New(cfgDir)
	localCfg, _ := mg.LoadLocal()
	if localCfg != nil && localCfg.Launcher == "" {
		localCfg.Launcher = "auto"
	}

	// 检测启动器
	pclDetected := launcher.FindPCL2()
	if pclDetected != nil {
		fmt.Printf("[✓] 检测到 PCL2: %s\n", pclDetected.Summary())
	} else {
		fmt.Println("[*] 未检测到 PCL2，使用裸启动模式")
	}

	// 3. 读取服务端配置，确定目标版本
	serverCfg, err := mg.LoadServer()
	var targetVersion string
	if err == nil && serverCfg.Version.ID != "" {
		targetVersion = serverCfg.Version.ID
		fmt.Printf("[✓] 目标版本: %s (来自 server.json)\n", targetVersion)
	} else {
		// 没有 server.json，拉 manifest 用最新 release
		manifestDir := filepath.Join(cfgDir, ".cache", "manifest")
		mm := launcher.NewVersionManifestManager(manifestDir)
		manifest, fetchErr := mm.Fetch(30 * time.Minute)
		if fetchErr != nil {
			logger.Error("版本清单拉取失败: %v", fetchErr)
			fmt.Fprintf(os.Stderr, "run: 版本清单拉取失败: %v\n", fetchErr)
			return
		}
		targetVersion = manifest.Latest.Release
		fmt.Printf("[*] 目标版本: %s (最新正式版, 无 server.json)\n", targetVersion)
	}

	if dryRun {
		fmt.Printf("[DRY-RUN] 将启动版本: %s\n", targetVersion)
		return
	}

	// 4. 查找本地版本目录
	if localCfg != nil && localCfg.ManagedVersions == nil {
		localCfg.ManagedVersions = []string{targetVersion}
	}
	finder := launcher.NewVersionFinder(localCfg)
	results := finder.FindManagedVersions(localCfg.ManagedVersions)
	versionResult := results[targetVersion]

	if versionResult == nil || !versionResult.Found {
		// 首次启动: 版本未安装 → 执行同步
		fmt.Printf("[*] 版本 %s 未安装, 首次启动自动同步...\n", targetVersion)
		sync(cfgDir, verbose, false)
	} else {
		from := "路径扫描"
		if versionResult.FromPCL {
			from = "PCL配置"
		}
		fmt.Printf("[✓] 版本 %s 已安装于 %s (来自 %s)\n",
			targetVersion, versionResult.VersionDir, from)
	}

	// TODO: 5. 拉起启动器（启动游戏）
	logger.Info("run: 同步完成, 等待启动器拉起功能")
	fmt.Println("run: 同步完成，启动器拉起功能开发中")
	logger.Info("run: 全自动模式完成")
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
	var is *launcher.IncrementalSync

	// 0. 拉取版本清单 + 确定目标版本
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

	// 0.5 读取本地配置并查找版本目录
	localCfg, _ := mg.LoadLocal()
	if localCfg != nil && localCfg.ManagedVersions == nil {
		localCfg.ManagedVersions = []string{targetVersion}
	}

	finder := launcher.NewVersionFinder(localCfg)
	results := finder.FindManagedVersions(localCfg.ManagedVersions)
	versionResult := results[targetVersion]
	var installPath string
	if localCfg != nil && localCfg.InstallPath != "" {
		installPath = localCfg.InstallPath
	}

	if versionResult == nil || !versionResult.Found {
		logger.Info("版本 %s 未在本地安装, 将执行全量同步", targetVersion)
		fmt.Printf("[*] 版本 %s 未在本地找到, 将执行全量安装\n", targetVersion)
		if installPath == "" {
			installPath = ".minecraft"
		}
	} else {
		from := "路径扫描"
		if versionResult.FromPCL {
			from = "PCL配置"
		}
		fmt.Printf("[✓] 版本 %s 已安装于 %s (来自 %s)\n",
			targetVersion, versionResult.VersionDir, from)
		installPath = versionResult.MinecraftDir
	}

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
		is = launcher.NewIncrementalSync(cfgDir, installPath)

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

		// 初始化 IncrementalSync
		is = launcher.NewIncrementalSync(cfgDir, installPath)

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
		is = launcher.NewIncrementalSync(cfgDir, installPath)

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

// TODO: 全部功能开发完毕后再实现。包括：
//   - pcl detect: 调用 FindPCL2() 并写入 local.json pcl2_path
//   - pcl path <path>: 验证路径有效性后写入 local.json pcl2_path
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
		fmt.Println("pack: subcommand required (import | publish | diff | list)")
		return
	}
	switch args[0] {
	case "import":
		handlePackImport(args[1:])
	case "publish":
		handlePackPublish(args[1:])
	case "diff":
		handlePackDiff(args[1:])
	case "list":
		handlePackList(args[1:])
	default:
		fmt.Printf("pack: unknown subcommand %s\n", args[0])
	}
}

func handlePackImport(args []string) {
	fs := flag.NewFlagSet("pack import", flag.ExitOnError)
	repoDir := fs.String("repo", "./publish", "发布仓库目录")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Println("用法: starter pack import <zip_path> [--repo <dir>]")
		fmt.Println("示例: starter pack import ./cjc-pack-v1.2.0.zip --repo /data/mc-starter/repo")
		return
	}

	zipPath := fs.Arg(0)
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "文件不存在: %s\n", zipPath)
		return
	}

	// 确保 repo 存在
	if err := pack.EnsureRepo(*repoDir); err != nil {
		fmt.Fprintf(os.Stderr, "初始化仓库失败: %v\n", err)
		return
	}

	result, err := pack.ImportZip(zipPath, *repoDir, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "导入失败: %v\n", err)
		return
	}

	fmt.Println("\n=== 导入结果 ===")
	fmt.Printf("zip: %s (%d 个文件, %.1f MB)\n", zipPath, result.Manifest.FileCount, float64(result.Manifest.TotalSize)/1024/1024)
	if result.MCVersion != "" {
		fmt.Printf("MC 版本: %s\n", result.MCVersion)
	}
	if result.Loader != "" {
		fmt.Printf("Loader: %s\n", result.Loader)
	}
	fmt.Printf("版本: %s\n", result.Version)

	if result.Diff != nil {
		fmt.Printf("\n差异对比 (相对 %s):\n", result.PrevVersion)
		if len(result.Diff.Added) > 0 {
			for _, e := range result.Diff.Added[:minInt(len(result.Diff.Added), 5)] {
				fmt.Printf("  + %s (%.1f KB)\n", e.Path, float64(e.Size)/1024)
			}
			if len(result.Diff.Added) > 5 {
				fmt.Printf("  ... (+%d more)\n", len(result.Diff.Added)-5)
			}
		}
		if len(result.Diff.Removed) > 0 {
			for _, e := range result.Diff.Removed[:minInt(len(result.Diff.Removed), 5)] {
				fmt.Printf("  - %s (%.1f KB)\n", e.Path, float64(e.Size)/1024)
			}
			if len(result.Diff.Removed) > 5 {
				fmt.Printf("  ... (-%d more)\n", len(result.Diff.Removed)-5)
			}
		}
		if len(result.Diff.Updated) > 0 {
			for _, e := range result.Diff.Updated[:minInt(len(result.Diff.Updated), 5)] {
				fmt.Printf("  ~ %s (%.1f KB)\n", e.Path, float64(e.Size)/1024)
			}
			if len(result.Diff.Updated) > 5 {
				fmt.Printf("  ... (~%d more)\n", len(result.Diff.Updated)-5)
			}
		}
		diffBytes := result.Diff.TotalDiffBytes()
		fmt.Printf("\n增量大小: %.1f MB (全量 %.1f MB 的 %.0f%%)\n",
			float64(diffBytes)/1024/1024,
			float64(result.Manifest.TotalSize)/1024/1024,
			float64(diffBytes)/float64(result.Manifest.TotalSize)*100)
	}

	fmt.Printf("\n状态: [draft] %s\n", result.Version)
	fmt.Println("\n运行 `starter pack publish` 确认发布")
}

func handlePackPublish(args []string) {
	fs := flag.NewFlagSet("pack publish", flag.ExitOnError)
	repoDir := fs.String("repo", "./publish", "发布仓库目录")
	version := fs.String("version", "", "版本号（空=发布最新 draft）")
	message := fs.String("message", "", "发布说明")
	fs.Parse(args)

	if err := pack.EnsureRepo(*repoDir); err != nil {
		fmt.Fprintf(os.Stderr, "初始化仓库失败: %v\n", err)
		return
	}

	if err := pack.PublishDraft(*repoDir, *version, *message); err != nil {
		fmt.Fprintf(os.Stderr, "发布失败: %v\n", err)
		return
	}

	fmt.Println("[✓] 发布成功")
}

func handlePackDiff(args []string) {
	fs := flag.NewFlagSet("pack diff", flag.ExitOnError)
	repoDir := fs.String("repo", "./publish", "发布仓库目录")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Println("用法: starter pack diff <v1> <v2> [--repo <dir>]")
		fmt.Println("示例: starter pack diff v1.0.0 v1.1.0 --repo /data/mc-starter/repo")
		return
	}

	fromVer, toVer := fs.Arg(0), fs.Arg(1)

	diff, err := pack.DiffVersions(*repoDir, fromVer, toVer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 diff 失败: %v\n", err)
		return
	}

	fmt.Printf("差异 %s → %s:\n", fromVer, toVer)
	fmt.Printf("  %s\n", diff.Summary())
	if len(diff.Added) > 0 {
		fmt.Println("  新增:")
		for _, e := range diff.Added {
			fmt.Printf("    + %s\n", e.Path)
		}
	}
	if len(diff.Removed) > 0 {
		fmt.Println("  删除:")
		for _, e := range diff.Removed {
			fmt.Printf("    - %s\n", e.Path)
		}
	}
	if len(diff.Updated) > 0 {
		fmt.Println("  更新:")
		for _, e := range diff.Updated {
			fmt.Printf("    ~ %s\n", e.Path)
		}
	}
}

func handlePackList(args []string) {
	fs := flag.NewFlagSet("pack list", flag.ExitOnError)
	repoDir := fs.String("repo", "./publish", "发布仓库目录")
	fs.Parse(args)

	drafts, published, err := pack.ListVersions(*repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出版本失败: %v\n", err)
		return
	}

	fmt.Println("=== 版本历史 ===")
	if len(drafts) == 0 && len(published) == 0 {
		fmt.Println("(空)")
		return
	}

	for _, v := range published {
		fmt.Printf("  [published] %s\n", v)
	}
	for _, v := range drafts {
		fmt.Printf("  [draft]     %s\n", v)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
