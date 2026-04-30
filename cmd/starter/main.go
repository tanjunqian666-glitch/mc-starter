package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/gui"
	"github.com/gege-tlph/mc-starter/internal/launcher"
	"github.com/gege-tlph/mc-starter/internal/logger"
	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/gege-tlph/mc-starter/internal/pack"
	"github.com/gege-tlph/mc-starter/internal/repair"
	"github.com/gege-tlph/mc-starter/internal/tray"
)

var version = "dev"

// parseGlobalFlags 从 os.Args 中提前提取 --config/--verbose/--dry-run 等全局 flag，
// 并返回剥离后的子命令参数列表（os.Args 风格，argv[0] 为程序名）。
// 原理：在标准 flag 解析前手动探测，避免子命令 switch 前无法读取全局选项。
func parseGlobalFlags() (cfgDir string, verbose, headless, dryRun bool, remainingArgs []string) {
	cfgDir = "./config"
	remainingArgs = os.Args

	// 从 os.Args[1:] 中扫描全局 flag 并剥离
	filtered := []string{os.Args[0]} // argv[0] 保留
	skipNext := false
	// 注意：range os.Args[1:] 产生的 i 从 0 开始，对应 os.Args[1]
	for i, a := range os.Args[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		// 仅匹配以 -- 开头的已知全局 flag
		switch {
		case a == "--config" && i+1 < len(os.Args[1:]):
			cfgDir = os.Args[i+2] // os.Args[1:][i+1] = os.Args[i+2]
			skipNext = true
		case a == "--headless":
			headless = true
		case a == "--dry-run":
			dryRun = true
		case a == "--verbose":
			verbose = true
		default:
			filtered = append(filtered, a)
		}
	}
	remainingArgs = filtered
	return
}

func main() {
	cfgDir, verbose, headless, dryRun, args := parseGlobalFlags()

	// P3 自更新: 启动健康检查（新版本首次启动后 10s 健康检测）
	{
		mg := config.New(cfgDir)
		localCfg, loadErr := mg.LoadLocal()
		serverURL := ""
		if loadErr == nil {
			serverURL = localCfg.ServerURL
		}
		localDir := filepath.Join(cfgDir, ".local")
		updater := launcher.NewSelfUpdater(localDir, version, serverURL)
		updater.CheckStartupHealth()
	}

	if len(args) < 2 {
		// 无参数: 双击场景 → 启动 GUI
		if err := gui.Run(cfgDir); err != nil {
			fmt.Fprintf(os.Stderr, "GUI 错误: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cmd := args[1]
	subArgs := args[2:]

	switch cmd {
	case "run":
		run(cfgDir, verbose, headless, dryRun)
	case "init":
		initialize(cfgDir)
	case "check":
		check(cfgDir, verbose)
	case "sync":
		sync(cfgDir, verbose, dryRun)
	case "repair":
		runRepair(subArgs, cfgDir)
	case "update":
		// update 子命令支持 --pack <name> 和 --all
		updateFS := flag.NewFlagSet("update", flag.ExitOnError)
		updatePack := updateFS.String("pack", "", "指定要更新的包名")
		updateAll := updateFS.Bool("all", false, "更新所有已启用的包")
		updateFS.Parse(subArgs)
		if *updatePack != "" {
			handleUpdateMulti(cfgDir, verbose, dryRun, *updatePack, false)
		} else if *updateAll {
			handleUpdateMulti(cfgDir, verbose, dryRun, "", true)
		} else {
			handleUpdate(cfgDir, verbose, dryRun)
		}
	case "backup":
		handleBackup(subArgs)
	case "cache":
		handleCache(subArgs)
	case "pack":
		handlePack(subArgs)
	case "fabric":
		handleFabric(subArgs)
	case "pcl":
		handlePCL(subArgs)
	case "daemon":
		runDaemon(subArgs, cfgDir)
	case "version":
		fmt.Printf("mc-starter %s\n", version)
	case "channel":
		handleChannel(subArgs, cfgDir)
	case "self-update":
		handleSelfUpdate(subArgs, cfgDir)
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
  starter update   增量更新（拉取服务端增量清单，按 hash 下文件）
  starter repair   修复工具
  starter daemon   静默守护模式（后台监控崩溃和日志）
  starter backup   备份管理
    list           列出备份
    restore <name> 恢复指定快照
    create         手动创建备份
    delete <name>  删除快照
  starter cache    缓存管理
    stats          显示缓存统计
    clean [--dry-run] [--min-ref <n>]  清理缓存
    prune [--dry-run]  清理 orphaned 缓存
  starter fabric   安装 Fabric loader
    install <mcVer> [--loader <ver>] [--mirror]
  starter pcl      操作 PCL2
    detect         检测 PCL2.exe 位置
    path <path>    设置 PCL2 路径
  starter self-update  自更新管理
    check              检查更新
    apply              应用已下载的更新
    rollback           回滚到上一个版本
    history            查看更新历史
    channel <name>     切换更新通道 (stable/beta/dev)
  starter version  显示版本信息
  starter channel  频道管理
    list            列出包的频道
    enable          启用频道
    disable         禁用频道
  starter help     显示此帮助

全局选项:
  --config <dir>   配置目录 (默认 ./config)
  --verbose, -v    详细日志
  --headless       静默模式
  --dry-run        仅检查不下载
`))
}

// run 全自动模式（Sprint 10 重写 — C/S 端到端流程）
//
// 流程:
//   1. 读配置 → 检测启动器
//   2. 拉服务端包列表，获取每个包的 (mc_version, loader)
//   3. 对每个已启用的包：
//      a. EnsureVersion(mc_version, loader) → 本地下 MC 本体 + Loader
//      b. UpdatePack → 下自定义内容到 packs/
//      c. MergePackToVersion → packs/ 内容合并到 versions/<name>/
//   4. 通知完成
func run(cfgDir string, verbose bool, headless bool, dryRun bool) {
	logger.Init(verbose)
	logger.Info("run: 全自动模式 (C/S)")
	fmt.Println("=== MC Starter 全自动模式 ===")

	// 1. 初始化配置
	if err := ensureConfig(cfgDir); err != nil {
		logger.Error("配置初始化失败: %v", err)
		fmt.Fprintf(os.Stderr, "run: 配置初始化失败: %v\n", err)
		return
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: 读取配置失败: %v\n", err)
		return
	}

	// 检测启动器
	if localCfg.Launcher == "" {
		localCfg.Launcher = "auto"
	}
	pclDetected := launcher.FindPCL2()
	if pclDetected != nil {
		fmt.Printf("[✓] 检测到启动器: %s\n", pclDetected.Summary())
		localCfg.Launcher = "pcl2"
	} else {
		fmt.Println("[*] 未检测到 PCL2，使用裸启动模式")
	}

	serverURL := localCfg.ServerURL
	if serverURL == "" {
		fmt.Fprintf(os.Stderr, "run: local.json 中缺少 server_url，请先配置\n")
		return
	}

	// 拉服务端包列表 — 获取 (mc_version, loader) 信息
	fmt.Println("\n[1/4] 拉取服务端包列表...")
	if pingErr := mg.Ping(serverURL); pingErr != nil {
		fmt.Fprintf(os.Stderr, "run: 无法连接服务端: %v\n", pingErr)
		return
	}
	packsResp, err := mg.FetchPacks(serverURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run: 拉取包列表失败: %v\n", err)
		return
	}
	fmt.Printf("      服务端 %d 个包\n", len(packsResp.Packs))

	// 同步包列表到本地配置
	for _, p := range packsResp.Packs {
		if _, exists := localCfg.Packs[p.Name]; !exists {
			channelsMap := make(map[string]model.ChannelState)
			for _, ch := range p.Channels {
				channelsMap[ch.Name] = model.ChannelState{
					Enabled: ch.Required,
					Version: "",
				}
			}
			localCfg.Packs[p.Name] = model.PackState{
				Enabled:  p.Primary,
				Status:   "none",
				Dir:      fmt.Sprintf("packs/%s", p.Name),
				Channels: channelsMap,
			}
		}
	}
	mg.SaveLocal(localCfg)

	if localCfg.MinecraftDir == "" {
		fmt.Fprintf(os.Stderr, "run: 未设置 .minecraft 目录，请先配置\n")
		return
	}
	mcDir := localCfg.MinecraftDir
	versionsDir := filepath.Join(mcDir, "versions")
	librariesDir := filepath.Join(mcDir, "libraries")

	// 2-3. 逐包处理
	fmt.Println("\n[2/4] 检查 MC 版本环境...")
	fmt.Println("\n[3/4] 更新整合包内容...")

	updater := launcher.NewUpdater(cfgDir, mcDir, mg)
	processed := 0
	for packName, state := range localCfg.Packs {
		if !state.Enabled {
			logger.Debug("[%s] 已禁用，跳过", packName)
			continue
		}

		fmt.Printf("\n--- 包: %s ---\n", packName)

		// a. 拉包详情获取 mc_version + loader
		detail, detailErr := mg.FetchPackDetail(serverURL, packName)
		mcVersion := ""
		loader := ""
		if detailErr == nil {
			mcVersion = detail.MCVersion
			loader = detail.Loader
		} else {
			logger.Warn("[%s] 拉取详情失败，使用配置中的值时: %v", packName, detailErr)
		}

		// b. 确保 MC 本体 + Loader 已安装
		if mcVersion != "" {
			fmt.Printf("      目标: MC %s", mcVersion)
			if loader != "" {
				fmt.Printf(" + %s", loader)
			}
			fmt.Println()

			if dryRun {
				fmt.Printf("      [DRY-RUN] 跳过 EnsureVersion\n")
			} else {
				ensureReq := launcher.EnsureRequest{
					MCVersion:  mcVersion,
					Loader:     loader,
					VersionDir: versionsDir,
					LibraryDir: librariesDir,
				}
				if err := updater.EnsureVersion(ensureReq); err != nil {
					logger.Error("[%s] EnsureVersion 失败: %v", packName, err)
					fmt.Fprintf(os.Stderr, "      ✗ MC 版本环境准备失败: %v\n", err)
					continue
				}
				fmt.Println("      ✓ MC 版本环境就绪")
			}
		} else {
			fmt.Println("      服务端未指定 MC 版本，跳过 MC 本体下载")
		}

		// c. 更新整合包自定义内容
		if dryRun {
			fmt.Printf("      [DRY-RUN] 跳过 UpdatePack\n")
		} else {
			result, updateErr := updater.UpdatePack(serverURL, packName, &state, false)
			if updateErr != nil {
				logger.Error("[%s] UpdatePack 失败: %v", packName, updateErr)
				fmt.Fprintf(os.Stderr, "      ✗ 更新失败: %v\n", updateErr)
				continue
			}
			fmt.Printf("      ✓ %s\n", result.Summary())
			state.LocalVersion = result.Version
			state.Status = "synced"
			localCfg.Packs[packName] = state
		}

		// d. packs/ → versions/ 合并（创建可被启动器识别的完整版本）
		packDir := filepath.Join(mcDir, "packs", packName)
		// 版本名：使用 pack name 代替 MC version，用于启动器识别
		versionName := fmt.Sprintf("mc-starter-%s", packName)
		versionTargetDir := filepath.Join(versionsDir, versionName)

		if dryRun {
			fmt.Printf("      [DRY-RUN] 合并 %s → %s\n", packDir, versionTargetDir)
		} else {
			merged, mergeErrs := launcher.MergePackToVersion(packDir, versionTargetDir, dryRun)
			if len(mergeErrs) > 0 {
				for _, me := range mergeErrs {
					logger.Warn("[%s] 合并部分失败: %v", packName, me)
				}
			}
			if merged > 0 {
				fmt.Printf("      ✓ 已合并 %d 个文件到 versions/%s/\n", merged, versionName)
				fmt.Printf("        💡 在启动器中选择版本 \"%s\" 启动\n", versionName)
			}
		}

		processed++
		_ = mcVersion
		_ = loader
	}

	if processed == 0 {
		fmt.Println("\n没有已启用的包需要处理")
	} else {
		fmt.Printf("\n✓ 处理完成: %d 个包\n", processed)
	}

	// 保存配置
	mg.SaveLocal(localCfg)

	// P3 自更新标记
	updater2 := launcher.NewSelfUpdater(filepath.Join(cfgDir, ".local"), version, serverURL)
	updater2.MarkStartupOK()

	fmt.Println("\n=== 全自动模式完成 ===")
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
	if err == nil && existing.MinecraftDir != "" {
		fmt.Printf("配置已存在: %s\n", dir)
		fmt.Printf("如需重新初始化，请删除 %s 后重试\n", filepath.Join(dir, "local.json"))
		return
	}

	// 生成默认配置
	local := &model.LocalConfig{
		MinecraftDir: ".minecraft",
		Launcher:    "bare",
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
		// 新版多目录支持
		if localCfg.MinecraftDirs != nil && len(localCfg.MinecraftDirs) > 0 {
			fmt.Printf("    安装目录:\n")
			for packName, mcDir := range localCfg.MinecraftDirs {
				fmt.Printf("      %s → %s\n", packName, mcDir)
			}
		} else {
			fmt.Printf("    安装目录: %s\n", localCfg.MinecraftDir)
		}
		fmt.Printf("    启动器: %s\n", localCfg.Launcher)
	}

	// 2. 尝试拉取版本清单
	manifest, err := mm.Fetch(30 * time.Minute)
	if err != nil {
		fmt.Printf("[✗] 版本清单: %v\n", err)
	} else {
		fmt.Printf("[✓] 版本清单: %d 个版本\n", len(manifest.Versions))
		fmt.Printf("    最新: release=%s  snapshot=%s\n", manifest.Latest.Release, manifest.Latest.Snapshot)
	}

	// 3. 检测启动器
	pclResult := launcher.FindPCL2()
	if pclResult != nil {
		fmt.Printf("[✓] PCL2 启动器: %s (v%s)\n", pclResult.Path, pclResult.Version)
	} else {
		fmt.Println("[*] 未检测到 PCL2")
	}

	// 4. 扫描 MC 目录
	managed, raw := launcher.ResolveMinecraftDirs()
	if len(managed) > 0 {
		fmt.Printf("[✓] 已托管 MC 目录: %d 个\n", len(managed))
		for _, m := range managed {
			packs := "无"
			if len(m.Packs) > 0 {
				packs = strings.Join(m.Packs, ", ")
			}
			fmt.Printf("    - %s (包: %s, 更新: %s)\n", m.Path, packs, m.UpdatedAt)
		}
	}
	if len(raw) > 0 {
		fmt.Printf("[*] 未托管但含 MC 的目录: %d 个\n", len(raw))
		for _, r := range raw {
			fmt.Printf("    - %s\n", r)
		}
	}

	// 5. 检查疑似副本
	packNames := make([]string, 0, len(localCfg.Packs))
	for name := range localCfg.Packs {
		packNames = append(packNames, name)
	}
	for _, packName := range packNames {
		mcDir := localCfg.GetMinecraftDir(packName)
		if mcDir == "" {
			continue
		}
		packsDir := filepath.Join(mcDir, "packs")
		suspects := launcher.FindSuspectedDuplicates(packsDir, packName)
		if len(suspects) > 0 {
			fmt.Printf("[!] 包 %s 发现疑似重复目录:\n", packName)
			for _, s := range suspects {
				fmt.Printf("    - %s\n", s)
			}
		}
	}

	// 6. Java 检测
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
	vc, err := mg.LoadLocalServerConfig()
	var targetVersion string
	if err == nil && vc.ID != "" {
		targetVersion = vc.ID
	} else {
		targetVersion = manifest.Latest.Release
	}

	logger.Info("sync: 目标版本 %s", targetVersion)
	fmt.Printf("sync: 版本清单 (%d 个版本), 目标 %s\n", len(manifest.Versions), targetVersion)

	// 0.5 读取本地配置并查找版本目录
	localCfg, _ := mg.LoadLocal()
	if localCfg != nil && len(localCfg.Packs) == 0 {
		localCfg.Packs = map[string]model.PackState{
			targetVersion: {Enabled: true, Status: "none", Dir: targetVersion},
		}
	}
	managedVersions := make([]string, 0, len(localCfg.Packs))
	for name := range localCfg.Packs {
		managedVersions = append(managedVersions, name)
	}

	finder := launcher.NewVersionFinder(localCfg)
	results := finder.FindManagedVersions(managedVersions)
	versionResult := results[targetVersion]
	var installPath string
	if localCfg != nil && localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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

func runRepair(args []string, cfgDir string) {
	logger.Init(false)

	// 解析 repair 子命令的 flag
	repairFS := flag.NewFlagSet("repair", flag.ExitOnError)
	clean := repairFS.Bool("clean", false, "全量修复：清空 mods/config/resourcepacks 并重新同步")
	modsOnly := repairFS.Bool("mods-only", false, "仅修复模组")
	configOnly := repairFS.Bool("config-only", false, "仅修复配置")
	loaderOnly := repairFS.Bool("loader-only", false, "仅重新安装 Loader")
	rollback := repairFS.Bool("rollback", false, "回滚到备份")
	rollbackID := repairFS.String("rollback-id", "", "回滚到指定备份 ID（不指定则用最新）")
	listBackups := repairFS.Bool("list-backups", false, "列出所有可用备份")
	headless := repairFS.Bool("headless", false, "静默模式（不交互）")
	listPacks := repairFS.Bool("list-packs", false, "列出可修复的包")

	repairFS.Parse(args)

	// 确定操作类型
	action := repair.ActionInteractive // 默认交互模式
	switch {
	case *clean:
		action = repair.ActionCleanAll
	case *modsOnly:
		action = repair.ActionModsOnly
	case *configOnly:
		action = repair.ActionConfigOnly
	case *loaderOnly:
		action = repair.ActionLoaderOnly
	case *rollback:
		action = repair.ActionRollback
	case *listBackups:
		action = repair.ActionListBackups
	}

	_ = headless

	// 加载配置
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	mcDir := mg.GetMinecraftDir(localCfg)

	// 定位要修复的包
	targetPack := ""
	remainingArgs := repairFS.Args()
	if len(remainingArgs) > 0 {
		targetPack = remainingArgs[0]
	}

	if *listPacks {
		fmt.Println("\n=== 可修复的包 ===")
		for name, state := range localCfg.Packs {
			if !state.Enabled {
				continue
			}
			ver := state.LocalVersion
			if ver == "" {
				ver = "(未安装)"
			}
			fmt.Printf("  %s (%s)\n", name, ver)
		}
		return
	}

	// 确定要修复的目录
	if targetPack != "" {
		// 修复指定包
		state, ok := localCfg.Packs[targetPack]
		if !ok {
			fmt.Fprintf(os.Stderr, "包 %s 未在配置中\n", targetPack)
			return
		}
		if !state.Enabled {
			fmt.Fprintf(os.Stderr, "包 %s 已禁用，启用后才能修复\n", targetPack)
			return
		}
		installPath := mg.GetPackWorkDir(mcDir, targetPack)

		if _, statErr := os.Stat(installPath); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "包目录不存在: %s\n", installPath)
			return
		}

		rCfg := repair.RepairConfig{Action: action, RollbackID: *rollbackID}
		if action != repair.ActionListBackups && action != repair.ActionRollback {
			fmt.Printf("\n=== 修复: %s ===\n", targetPack)
		}

		result, err := repair.Repair(installPath, rCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "修复失败: %v\n", err)
			return
		}
		printRepairResult(result, targetPack)
		return
	}

	// 没有指定包 → 修复当前选中的包（或主包）
	// 先找主包
	var primaryName string
	for name, state := range localCfg.Packs {
		if state.Enabled {
			primaryName = name
			break
		}
	}
	if primaryName == "" {
		fmt.Fprintf(os.Stderr, "没有已启用的包\n")
		return
	}
	installPath := mg.GetPackWorkDir(mcDir, primaryName)

	if _, statErr := os.Stat(installPath); os.IsNotExist(statErr) {
		fmt.Fprintf(os.Stderr, ".minecraft 目录不存在: %s\n", installPath)
		return
	}

	rCfg := repair.RepairConfig{Action: action, RollbackID: *rollbackID}
	if action != repair.ActionListBackups && action != repair.ActionRollback {
		fmt.Println("\n=== 修复工具 ===")
	}

	result, err := repair.Repair(installPath, rCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "修复失败: %v\n", err)
		return
	}
	printRepairResult(result, primaryName)

	// P2.11: 修复后自动同步（不适用于回滚/列表/loader-only）
	shouldSync := !*headless && action != repair.ActionRollback && action != repair.ActionListBackups && action != repair.ActionLoaderOnly
	if shouldSync && len(result.CleanedDirs) > 0 {
		fmt.Println("\n▶ 修复完成，正在自动同步...")
		handleUpdateMulti(cfgDir, false, false, targetPack, false)
	}

	// P2.15: 修复后 PCL2 刷新
	if action != repair.ActionListBackups && action != repair.ActionRollback {
		_ = launcher.RefreshPCL2AfterRepair(installPath, targetPack)
	}
}

func printRepairResult(result *repair.RepairResult, packName string) {
	switch result.Action {
	case repair.ActionRollback:
		if result.Restored > 0 {
			fmt.Printf("[✓] %s: 已回滚 %d 个文件\n", packName, result.Restored)
		} else {
			fmt.Println("未执行回滚")
		}

	case repair.ActionListBackups:
		// Repair 内部已输出列表

	case repair.ActionCleanAll, repair.ActionModsOnly, repair.ActionConfigOnly, repair.ActionLoaderOnly, repair.ActionInteractive:
		fmt.Println()
		for _, d := range result.CleanedDirs {
			fmt.Printf("  [✓] %s: 已清理 %s/\n", packName, d)
		}

		if result.BackupDir != "" {
			fmt.Println("\n📦 备份:", result.BackupDir)
		}
		if len(result.Errors) > 0 {
			fmt.Println("\n⚠ 部分操作遇到问题:")
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		if result.Action == repair.ActionLoaderOnly {
			fmt.Printf("\n提示: 请运行 'starter fabric install <mcVer>' 重新安装 %s 的 Loader\n", packName)
		} else {
			fmt.Printf("\n提示: 请运行 'starter update --pack %s' 重新下载模组和配置\n", packName)
		}
		fmt.Println("\n💡 如需回滚: starter repair <包名> --rollback")
	}
}

func handleUpdate(cfgDir string, verbose, dryRun bool) {
	handleUpdateMulti(cfgDir, verbose, dryRun, "", false)
}

func handleUpdateMulti(cfgDir string, verbose, dryRun bool, packName string, updateAll bool) {
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}
	serverURL := localCfg.ServerURL
	if serverURL == "" {
		fmt.Fprintf(os.Stderr, "local.json 中缺少 server_url，请先配置\n")
		return
	}
	mcDir := mg.GetMinecraftDir(localCfg)

	// 指定包 → 只更新一个
	if packName != "" {
		state, ok := localCfg.Packs[packName]
		if !ok {
			fmt.Fprintf(os.Stderr, "包 %s 未在配置中\n", packName)
			return
		}
		if !state.Enabled {
			fmt.Fprintf(os.Stderr, "包 %s 已禁用\n", packName)
			return
		}

		updater := launcher.NewUpdater(cfgDir, mcDir, mg)
		fmt.Printf("\n=== 更新: %s ===\n", packName)
		result, err := updater.UpdatePack(serverURL, packName, &state, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "更新失败: %v\n", err)
			return
		}
		fmt.Printf("[✓] %s\n", result.Summary())
		state.LocalVersion = result.Version
		state.Status = "synced"

		// P6: 同步包详情中的频道信息 + Loader 提示
		detail, detailErr := mg.FetchPackDetail(serverURL, packName)
		if detailErr == nil {
			if len(detail.Channels) > 0 {
				if state.Channels == nil {
					state.Channels = make(map[string]model.ChannelState)
				}
				for _, ch := range detail.Channels {
					if _, ok := state.Channels[ch.Name]; !ok {
						state.Channels[ch.Name] = model.ChannelState{
							Enabled: ch.Required,
							Version: ch.Version,
						}
					}
				}
			}

			// Loader 信息提示
			if detail.MCVersion != "" {
				fmt.Printf("\n📦 %s: 需要 MC %s", packName, detail.MCVersion)
				if detail.Loader != "" {
					fmt.Printf(" + %s", detail.Loader)
				}
				fmt.Println()
				versionName := fmt.Sprintf("mc-starter-%s", packName)
				if detail.Loader != "" {
					fmt.Printf("   💡 运行 `starter run` 自动安装 MC 本体 + %s\n", detail.Loader)
				} else {
					fmt.Printf("   💡 运行 `starter run` 自动安装 MC %s 本体\n", detail.MCVersion)
				}
				fmt.Printf("   💡 之后可在启动器中选 \"%s\" 版本启动\n", versionName)
			}
		}

		localCfg.Packs[packName] = state
		mg.SaveLocal(localCfg)
		return
	}

	// --all 或默认 → 更新所有已启用的包
	fmt.Println("\n=== 检查更新 ===")

	// 先 ping
	if pingErr := mg.Ping(serverURL); pingErr != nil {
		fmt.Fprintf(os.Stderr, "无法连接服务端: %v\n", pingErr)
		return
	}

	// 拉包列表
	packsResp, err := mg.FetchPacks(serverURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "拉取包列表失败: %v\n", err)
		return
	}

	fmt.Printf("服务端包数: %d\n", len(packsResp.Packs))
	for _, p := range packsResp.Packs {
		mark := " "
		if p.Primary {
			mark = "★"
		}
		fmt.Printf("  %s %s (%s)\n", mark, p.DisplayName, p.LatestVersion)
	}

	// 同步包列表到本地配置（P6 扩展：同步频道信息）
	for _, p := range packsResp.Packs {
		existing, exists := localCfg.Packs[p.Name]
		if !exists {
			// 主包自动启用，副包默认禁用
			channelsMap := make(map[string]model.ChannelState)
			if len(p.Channels) > 0 {
				for _, ch := range p.Channels {
					channelsMap[ch.Name] = model.ChannelState{
						Enabled: ch.Required,
						Version: "",
					}
				}
			}
			localCfg.Packs[p.Name] = model.PackState{
				Enabled:  p.Primary,
				Status:   "none",
				Dir:      fmt.Sprintf("packs/%s", p.Name),
				Channels: channelsMap,
			}
		} else if len(p.Channels) > 0 {
			// 已有包，合并/更新频道信息
			if existing.Channels == nil {
				existing.Channels = make(map[string]model.ChannelState)
			}
			for _, ch := range p.Channels {
				if _, ok := existing.Channels[ch.Name]; !ok {
					// 新频道出现
					existing.Channels[ch.Name] = model.ChannelState{
						Enabled: ch.Required,
						Version: "",
					}
				}
			}
			localCfg.Packs[p.Name] = existing
		}
	}
	mg.SaveLocal(localCfg)

	// 更新已启用的包
	updater := launcher.NewUpdater(cfgDir, mcDir, mg)
	results := updater.UpdateAllPacks(serverURL, localCfg.Packs, nil)

	hasNewVersion := false
	for name, r := range results {
		if r == nil {
			continue
		}
		if r.Skipped == -1 {
			continue
		}
		hasNewVersion = true
		fmt.Printf("[✓] %s\n", r.Summary())
		if len(r.Errors) > 0 {
			fmt.Printf("  ⚠ %d 个错误\n", len(r.Errors))
		}
		// 更新本地版本号
		if s, ok := localCfg.Packs[name]; ok {
			s.LocalVersion = r.Version
			s.Status = "synced"
			localCfg.Packs[name] = s
		}
	}

	if !hasNewVersion {
		fmt.Println("所有包已是最新版本 ✓")
	}

	// Loader 信息提示：检查每个已更新包是否需要额外安装 Loader
	fmt.Println()
	for name := range localCfg.Packs {
		detail, detailErr := mg.FetchPackDetail(serverURL, name)
		if detailErr != nil {
			continue
		}
		if detail.MCVersion == "" {
			continue
		}

		fmt.Printf("📦 %s: 需要 MC %s", name, detail.MCVersion)
		if detail.Loader != "" {
			fmt.Printf(" + %s", detail.Loader)
		}
		fmt.Println()

		// 检查是否已安装对应 version
		versionName := fmt.Sprintf("mc-starter-%s", name)
		versionDir := filepath.Join(mcDir, "versions", versionName)
		if _, statErr := os.Stat(versionDir); os.IsNotExist(statErr) {
			fmt.Printf("   ⚠ versions/%s/ 不存在，运行 `starter run` 即可自动安装\n", versionName)
		} else if detail.Loader != "" {
			// 检查 loader 是否就绪（简单检查版本 json 中是否包含 loader 库引用）
			verJSON := filepath.Join(versionDir, fmt.Sprintf("%s.json", versionName))
			if _, statErr := os.Stat(verJSON); os.IsNotExist(statErr) {
				fmt.Printf("   ⚠ 版本 json 缺失，运行 `starter run` 重建\n")
			} else {
				fmt.Printf("   ✓ 版本目录就绪，可在启动器中选 \"%s\" 启动\n", versionName)
			}
		}

		// 提示自动安装
		if detail.Loader != "" {
			// 转成 EnsureRequest 格式
			loaderSpec := fmt.Sprintf("%s-0.16.10", detail.Loader) // 默认版本，实际运行时自动选最新
			fmt.Printf("   💡 如需自动安装 MC 本体 + %s，运行: starter run\n", detail.Loader)
			_ = loaderSpec
		}
	}

	// 有副包可用
	hasInactive := false
	for _, p := range packsResp.Packs {
		if s, ok := localCfg.Packs[p.Name]; ok && !s.Enabled {
			hasInactive = true
			break
		}
	}
	if hasInactive {
		fmt.Println("\n💡 服务端有可用副包，请到托盘菜单启用")
	}

	mg.SaveLocal(localCfg)
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
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
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

// runDaemon 启动静默守护模式
// 用法: starter daemon [--config <dir>] [--poll <间隔>]
func runDaemon(args []string, cfgDir string) {
	logger.Init(false)

	daemonFS := flag.NewFlagSet("daemon", flag.ExitOnError)
	pollInterval := daemonFS.Duration("poll", 5*time.Second, "轮询间隔")
	daemonFS.Parse(args)

	// 加载配置
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}

	installPath := ".minecraft"
	if localCfg.MinecraftDir != "" {
		installPath = localCfg.MinecraftDir
	}

	fmt.Println("\n=== 静默守护 ===")
	fmt.Printf("监控目录: %s\n", installPath)
	fmt.Printf("轮询间隔: %v\n", *pollInterval)
	fmt.Println("按 Ctrl+C 退出守护")

	// 解析 --config 参数（传给修复工具的子进程）
	var daemonCfgArgs []string
	for i, a := range args {
		if a == "--config" && i+1 < len(args) {
			daemonCfgArgs = []string{"--config", args[i+1]}
			break
		}
	}

	// P2.13: 启动系统托盘（Windows 下自动显示托盘图标）
	trayMgr := tray.NewManager(cfgDir, installPath)
	if err := trayMgr.Start(); err != nil {
		logger.Warn("托盘启动失败(非致命): %v", err)
	}
	defer trayMgr.Stop()

	// 创建守护
	d := repair.NewDaemon(repair.DaemonConfig{
		MinecraftDir: installPath,
		PollInterval: *pollInterval,
		OnEvent: func(event repair.DaemonEvent, data interface{}) {
			switch event {
			case repair.EventCrashDetected:
				if ev, ok := data.(repair.CrashEvent); ok {
					fmt.Printf("\n[崩溃检测] %s (%s)\n", ev.Reason, ev.Type)
					fmt.Printf("  文件: %s\n", ev.FilePath)

					// P2.13: 托盘通知
					trayMgr.NotifyCrash(ev)
					trayMgr.SetStatus("崩溃: " + ev.Reason)

					// P2.10: 上传崩溃报告到服务端（静默上传，失败不阻断）
					packName := "main-pack"
					if localCfg != nil {
						for name, st := range localCfg.Packs {
							if st.Enabled {
								packName = name
								break
							}
						}
					}
					go func() {
						resp, uploadErr := repair.CollectAndUpload(
							installPath, cfgDir, packName,
							ev.ExitCode, ev.Reason, nil,
						)
						if uploadErr != nil {
							logger.Warn("崩溃报告上传失败: %v", uploadErr)
						} else if resp != nil {
							logger.Info("崩溃报告已上传 (ticket=%s)", resp.Ticket)
						}
					}()

					// P2.14: 弹窗询问用户是否打开修复工具
					launched, err := repair.PromptCrashRepair(ev, daemonCfgArgs)
					if err != nil {
						fmt.Fprintf(os.Stderr, "\n[崩溃弹窗错误] %v\n", err)
					} else if launched {
						fmt.Println("\n[崩溃处理] 已启动修复工具")
					} else {
						fmt.Println("\n[崩溃处理] 用户忽略，继续监控")
					}
				}
			case repair.EventLogError:
				if s, ok := data.(string); ok {
					fmt.Printf("\n[日志异常] %s\n", s)
				}
			case repair.EventProcessExited:
				fmt.Println("\n[进程退出] 监控目标已退出")
				trayMgr.SetStatus("监控目标已退出")
			case repair.EventMCStarted:
				if p, ok := data.(repair.WatchedProcess); ok {
					fmt.Printf("\n[进程启动] %s (PID=%d)\n", p.Name, p.PID)
					trayMgr.SetStatus("运行中")
				}
			}
		},
	})

	// 启动
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "守护启动失败: %v\n", err)
		return
	}
	defer d.Stop()

	trayMgr.SetStatus("守护中")
	logger.Info("守护已启动，托盘可用")

	// 等待 Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh
	fmt.Println("\n守护已停止")
}

// handleFabric 处理 fabric 子命令
func handleFabric(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(strings.TrimSpace(`
用法:
  starter fabric install <mc版本> [--loader <版本>] [--mirror]

示例:
  starter fabric install 1.20.4          自动选择最新稳定 loader
  starter fabric install 1.20.4 --loader 0.19.2  指定 loader 版本
  starter fabric install 1.20.4 --mirror         启用镜像加速
		`))
		return
	}

	switch args[0] {
	case "install":
		if len(args) < 2 {
			fmt.Println("用法: starter fabric install <mc版本> [--loader <版本>] [--mirror]")
			return
		}

		mcVersion := args[1]
		var loaderVer string
		useMirror := false

		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--loader":
				if i+1 < len(args) {
					loaderVer = args[i+1]
					i++
				}
			case "--mirror":
				useMirror = true
			}
		}

		// 确定目录
		cfgDir := "config"
		mg := config.New(cfgDir)
		localCfg, err := mg.LoadLocal()
		if err != nil {
			fmt.Printf("[!] 未找到 local.json，使用默认路径\n")
		}

		versionsDir := ".minecraft/versions"
		librariesDir := "libraries"
		if localCfg != nil && localCfg.MinecraftDir != "" {
			versionsDir = filepath.Join(localCfg.MinecraftDir, "versions")
		}
		// libraries 通常放在 config/libraries 或 .minecraft/libraries
		librariesDir = filepath.Join(cfgDir, "libraries")

		logger.Init(false)
		logger.Info("Fabric: 安装 mc=%s, loader=%s, mirror=%v", mcVersion, loaderVer, useMirror)

		// 说明：安装流程分两步
		// 1. MC 本体（如果未安装）
		// 2. Fabric loader
		fmt.Printf("\n=== Fabric 安装: %s ===\n", mcVersion)

		// 检查 MC 原版是否已安装
		mcVersionJSON := filepath.Join(versionsDir, mcVersion, fmt.Sprintf("%s.json", mcVersion))
		if _, statErr := os.Stat(mcVersionJSON); os.IsNotExist(statErr) {
			fmt.Printf("[!] MC %s 原版未安装\n", mcVersion)
			fmt.Println("建议先执行: starter sync")
			fmt.Println("（或手动将 .minecraft/versions/ 复制过来）")
			// 不阻断，Fabric 安装器只写 profile JSON 和 libraries
			// MC 版本缺失不影响 Fabric 自己的文件安装
			fmt.Println("[*] 继续安装 Fabric loader（MC 本体需另行同步）")
		} else {
			fmt.Printf("[✓] MC %s 原版已安装\n", mcVersion)
		}

		installer := launcher.NewFabricInstaller(mcVersion, loaderVer, versionsDir, librariesDir)
		if useMirror {
			installer.SetMirror(true)
		}

		result, err := installer.Install()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n[✗] Fabric 安装失败: %v\n", err)
			return
		}

		fmt.Printf("\n[✓] Fabric 安装完成:\n")
		fmt.Printf("    版本 ID:   %s\n", result.VersionID)
		fmt.Printf("    Loader:    %s\n", result.LoaderVersion)
		fmt.Printf("    Libraries: %d 下载, %d 已存在\n", result.Downloaded, result.Skipped)

		// 验证安装
		missing, verifyErr := installer.VerifyInstallation(result.VersionID)
		if verifyErr != nil {
			fmt.Printf("[!] 验证异常: %v\n", verifyErr)
		} else if len(missing) > 0 {
			fmt.Printf("[!] %d 个文件缺失:\n", len(missing))
			for _, m := range missing {
				fmt.Printf("    - %s\n", m)
			}
		} else {
			fmt.Printf("[✓] 安装完整性验证通过\n")
		}

	default:
		fmt.Printf("fabric: unknown subcommand %s\n", args[0])
		fmt.Println("可用: install")
	}
}

// handlePCL 处理 pcl 子命令
func handlePCL(args []string) {
	cfgDir, _, _, _, _ := parseGlobalFlags() // 重新解析，方便读配置

	if len(args) == 0 {
		fmt.Println("pcl: subcommand required (detect | set-dir | path)")
		fmt.Println("  detect         检测启动器和 MC 目录")
		fmt.Println("  set-dir <包名> <序号>  为指定包选择目录")
		fmt.Println("  path <路径>    手动指定启动器路径")
		return
	}
	switch args[0] {
	case "detect":
		handlePCLDetect(cfgDir)
	case "set-dir":
		if len(args) < 3 {
			fmt.Println("pcl set-dir: 需要包名和目录序号")
			fmt.Println("用法: starter pcl set-dir <包名> <序号>")
			return
		}
		handlePCLSetDir(cfgDir, args[1], args[2])
	case "path":
		if len(args) < 2 {
			fmt.Println("pcl path: path required")
			return
		}
		fmt.Printf("pcl path: set to %s\n", args[1])
	default:
		fmt.Printf("pcl: unknown subcommand %s\n", args[0])
	}
}

// handlePCLDetect 检测启动器和 MC 目录
func handlePCLDetect(cfgDir string) {
	pclResult := launcher.FindPCL2()

	fmt.Println("=== 启动器检测 ===")
	if pclResult != nil {
		fmt.Printf("[✓] PCL2: %s (v%s, 检测级别 %d)\n", pclResult.Path, pclResult.Version, pclResult.Level)
	} else {
		fmt.Println("[*] 未检测到 PCL2")
	}

	// 读取 PCL.ini 配置
	managed, raw := launcher.ResolveMinecraftDirs()
	fmt.Println("\n=== Minecraft 目录 ===")
	allDirs := append([]launcher.ManagedMCDir{}, managed...)
	idx := 1
	for _, m := range managed {
		packs := "无"
		if len(m.Packs) > 0 {
			packs = strings.Join(m.Packs, ", ")
		}
		fmt.Printf("  %d. %s [已托管] (包: %s)\n", idx, m.Path, packs)
		idx++
	}
	for _, r := range raw {
		fmt.Printf("  %d. %s [候选]\n", idx, r)
		allDirs = append(allDirs, launcher.ManagedMCDir{Path: r})
		idx++
	}

	if len(allDirs) == 0 {
		fmt.Println("  (未找到任何 Minecraft 目录)")
		return
	}

	// 显示当前配置
	mg := config.New(cfgDir)
	localCfg, _ := mg.LoadLocal()
	fmt.Println("\n=== 当前配置 ===")
	for packName := range localCfg.Packs {
		mcDir := localCfg.GetMinecraftDir(packName)
		if mcDir == "" {
			fmt.Printf("  %s → (未选择)\n", packName)
		} else {
			fmt.Printf("  %s → %s\n", packName, mcDir)
		}
	}
	if len(localCfg.Packs) == 0 {
		fmt.Println("  (尚无管理的包)")
		fmt.Println("  [提示]: 运行 starter sync 后会注册管理的包")
	}
	fmt.Println("\n使用 starter pcl set-dir <包名> <序号> 为指定包选择目录")
}

// handlePCLSetDir 为指定包设置目录
func handlePCLSetDir(cfgDir, packName, indexStr string) {
	managed, raw := launcher.ResolveMinecraftDirs()
	allDirs := append([]launcher.ManagedMCDir{}, managed...)
	for _, r := range raw {
		allDirs = append(allDirs, launcher.ManagedMCDir{Path: r})
	}

	var idx int
	if _, err := fmt.Sscanf(indexStr, "%d", &idx); err != nil || idx < 1 || idx > len(allDirs) {
		fmt.Printf("无效序号: %s (有效范围 1-%d)\n", indexStr, len(allDirs))
		return
	}

	selectedPath := allDirs[idx-1].Path
	mg := config.New(cfgDir)
	localCfg, _ := mg.LoadLocal()
	localCfg.SetMinecraftDir(packName, selectedPath)
	if err := mg.SaveLocal(localCfg); err != nil {
		fmt.Printf("[✗] 保存配置失败: %v\n", err)
		return
	}
	fmt.Printf("[✓] 包 %s → %s\n", packName, selectedPath)
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

// ==== 频道管理 (P6) ====

// handleChannel 处理 channel 子命令
func handleChannel(args []string, cfgDir string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(strings.TrimSpace(`
用法:
  starter channel list [--pack <包名>]         列出所有已启用包的频道状态
  starter channel enable <包名> --channel <频道名>  启用频道
  starter channel disable <包名> --channel <频道名> 禁用频道

示例:
  starter channel list                           列出所有包的频道
  starter channel list --pack main-pack          只列 main-pack 的频道
  starter channel enable main-pack --channel shaderpacks  启用光影包频道
  starter channel disable main-pack --channel shaderpacks 禁用光影包频道
		`))
		return
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}

	switch args[0] {
	case "list":
		handleChannelList(args[1:], mg, localCfg)
	case "enable":
		handleChannelEnable(args[1:], mg, localCfg)
	case "disable":
		handleChannelDisable(args[1:], mg, localCfg)
	default:
		fmt.Printf("channel: unknown subcommand %s\n", args[0])
		fmt.Println("可用: list, enable, disable")
	}
}

func handleChannelList(args []string, mg *config.Manager, localCfg *model.LocalConfig) {
	fs := flag.NewFlagSet("channel list", flag.ExitOnError)
	packFilter := fs.String("pack", "", "指定包名")
	fs.Parse(args)

	hasPrint := false
	for packName, state := range localCfg.Packs {
		if *packFilter != "" && packName != *packFilter {
			continue
		}

		if !hasPrint {
			hasPrint = true
		}

		ver := state.LocalVersion
		if ver == "" {
			ver = "(未安装)"
		}

		fmt.Printf("\n包: %s (%s)\n", packName, ver)

		if len(state.Channels) == 0 {
			fmt.Println("  (无频道信息，使用默认同步)")
			continue
		}

		for chName, chState := range state.Channels {
			verStr := chState.Version
			if verStr == "" {
				verStr = "(未安装)"
			}
			if chState.Enabled {
				fmt.Printf("  ☑ %s  [%s]\n", chName, verStr)
			} else {
				fmt.Printf("  ☐ %s  [%s]\n", chName, verStr)
			}
		}
	}

	if !hasPrint {
		fmt.Println("没有已配置的包")
	}
}

func handleChannelEnable(args []string, mg *config.Manager, localCfg *model.LocalConfig) {
	fs := flag.NewFlagSet("channel enable", flag.ExitOnError)
	channelName := fs.String("channel", "", "频道名")
	fs.Parse(args)

	if fs.NArg() < 1 || *channelName == "" {
		fmt.Println("用法: starter channel enable <包名> --channel <频道名>")
		return
	}

	packName := fs.Arg(0)
	state, ok := localCfg.Packs[packName]
	if !ok {
		fmt.Fprintf(os.Stderr, "包 %s 未在配置中\n", packName)
		return
	}

	if state.Channels == nil {
		state.Channels = make(map[string]model.ChannelState)
	}

	ch, exists := state.Channels[*channelName]
	if !exists {
		fmt.Fprintf(os.Stderr, "频道 %s 未在包 %s 中定义\n", *channelName, packName)
		return
	}

	if ch.Enabled {
		fmt.Printf("频道 %s 已启用\n", *channelName)
		return
	}

	ch.Enabled = true
	state.Channels[*channelName] = ch
	localCfg.Packs[packName] = state

	if err := mg.SaveLocal(localCfg); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置失败: %v\n", err)
		return
	}

	fmt.Printf("[✓] 频道 %s 已启用，下次 update 将同步该频道\n", *channelName)
	fmt.Println("运行 `starter update` 立即同步")
}

func handleChannelDisable(args []string, mg *config.Manager, localCfg *model.LocalConfig) {
	fs := flag.NewFlagSet("channel disable", flag.ExitOnError)
	channelName := fs.String("channel", "", "频道名")
	fs.Parse(args)

	if fs.NArg() < 1 || *channelName == "" {
		fmt.Println("用法: starter channel disable <包名> --channel <频道名>")
		return
	}

	packName := fs.Arg(0)
	state, ok := localCfg.Packs[packName]
	if !ok {
		fmt.Fprintf(os.Stderr, "包 %s 未在配置中\n", packName)
		return
	}

	if state.Channels == nil {
		state.Channels = make(map[string]model.ChannelState)
	}

	ch, exists := state.Channels[*channelName]
	if !exists {
		fmt.Fprintf(os.Stderr, "频道 %s 未在包 %s 中定义\n", *channelName, packName)
		return
	}

	if !ch.Enabled {
		fmt.Printf("频道 %s 已禁用\n", *channelName)
		return
	}

	ch.Enabled = false
	state.Channels[*channelName] = ch
	localCfg.Packs[packName] = state

	if err := mg.SaveLocal(localCfg); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置失败: %v\n", err)
		return
	}

	fmt.Printf("[✓] 频道 %s 已禁用，下次 update 将跳过该频道\n", *channelName)
}

// ==== 自更新 (P3) ====

// handleSelfUpdate 处理 self-update 子命令
func handleSelfUpdate(args []string, cfgDir string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(strings.TrimSpace(`
用法:
  starter self-update check             检查更新
  starter self-update apply             应用已下载的更新并重启
  starter self-update rollback          回滚到上一个版本
  starter self-update history           查看更新历史
  starter self-update channel <name>    切换更新通道 (stable, beta, dev)
		`))
		return
	}

	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		return
	}

	localDir := filepath.Join(cfgDir, ".local")
	serverURL := localCfg.ServerURL
	if serverURL == "" {
		fmt.Fprintf(os.Stderr, "local.json 中缺少 server_url，请先配置\n")
		return
	}

	updater := launcher.NewSelfUpdater(localDir, version, serverURL)

	switch args[0] {
	case "check":
		handleSelfUpdateCheck(updater)
	case "apply":
		handleSelfUpdateApply(updater)
	case "rollback":
		handleSelfUpdateRollback(updater)
	case "history":
		handleSelfUpdateHistory(updater)
	case "channel":
		handleSelfUpdateChannel(args[1:], updater)
	default:
		fmt.Printf("self-update: unknown subcommand %s\n", args[0])
		fmt.Println("可用: check, apply, rollback, history, channel")
	}
}

func handleSelfUpdateCheck(updater *launcher.SelfUpdater) {
	meta, available, err := updater.CheckUpdate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "检查更新失败: %v\n", err)
		return
	}

	fmt.Printf("当前版本: %s\n", updater.Version)
	if !available {
		fmt.Println("已是最新版本 ✓")
		return
	}

	fmt.Println("\n📦 发现新版本:")
	fmt.Printf("  版本: %s (%s)\n", meta.Version, meta.Channel)
	fmt.Printf("  发布: %s\n", meta.ReleaseDate)
	if len(meta.Changelog) > 0 {
		fmt.Println("  更新内容:")
		for _, line := range meta.Changelog {
			fmt.Printf("    • %s\n", line)
		}
	}

	if updater.IsCriticalUpdate(meta) {
		fmt.Println("\n⚠ 当前版本过旧，需要强制更新")
	}

	fmt.Println("\n▶ 正在下载更新...")
	if err := updater.DownloadUpdate(meta); err != nil {
		fmt.Fprintf(os.Stderr, "下载更新失败: %v\n", err)
		return
	}
	fmt.Println("✅ 更新已下载，运行 `starter self-update apply` 应用更新")
}

func handleSelfUpdateApply(updater *launcher.SelfUpdater) {
	result, err := updater.ApplyUpdate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "应用更新失败: %v\n", err)
		return
	}
	_ = result
}

func handleSelfUpdateRollback(updater *launcher.SelfUpdater) {
	if err := updater.Rollback(); err != nil {
		fmt.Fprintf(os.Stderr, "回滚失败: %v\n", err)
		return
	}
	fmt.Println("已回滚到上一个版本，请重启 starter")

	if runtime.GOOS != "windows" {
		exe, _ := os.Executable()
		if _, statErr := os.Stat(exe); statErr == nil {
			fmt.Println("正在重启...")
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Start()
			os.Exit(0)
		}
	}
}

func handleSelfUpdateHistory(updater *launcher.SelfUpdater) {
	entries, err := updater.GetUpdateHistory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取更新历史失败: %v\n", err)
		return
	}
	fmt.Println(launcher.FormatUpdateHistory(entries))
}

func handleSelfUpdateChannel(args []string, updater *launcher.SelfUpdater) {
	if len(args) == 0 {
		fmt.Printf("当前通道: %s\n", updater.Channel)
		fmt.Println("可用通道: stable, beta, dev")
		return
	}

	ch := args[0]
	if err := updater.SetChannelStr(ch); err != nil {
		fmt.Fprintf(os.Stderr, "切换通道失败: %v\n", err)
		return
	}
	fmt.Printf("已切换到通道: %s\n", ch)
	fmt.Println("下次检查更新时将使用新通道")
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
			MinecraftDir: ".minecraft",
			Launcher:    "bare",
			Username:    "Player",
		}
		if err := mg.SaveLocal(local); err != nil {
			return fmt.Errorf("创建默认配置: %w", err)
		}
		logger.Info("已创建默认配置: %s", cfgDir)
	}
	return nil
}
