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
    restore <id>   恢复备份
    create         手动创建备份
    delete <id>    删除备份
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

	// 1. 拉取最新版本清单
	manifest, err := mm.Fetch(30 * time.Minute)
	if err != nil {
		logger.Error("版本清单拉取失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: 版本清单拉取失败: %v\n", err)
		return
	}

	fmt.Printf("sync: 版本清单已同步 (%d 个版本)\n", len(manifest.Versions))
	fmt.Printf("      最新 release: %s\n", manifest.Latest.Release)

	// 2. 确定目标版本
	mg := config.New(cfgDir)
	serverCfg, err := mg.LoadServer()
	var targetVersion string
	if err == nil && serverCfg.Version.ID != "" {
		targetVersion = serverCfg.Version.ID
	} else {
		targetVersion = manifest.Latest.Release
		fmt.Printf("sync: 未指定版本，使用最新 release: %s\n", targetVersion)
	}

	logger.Info("sync: 目标版本 %s", targetVersion)

	// 3. 下载 version.json（版本元数据）
	fmt.Printf("\n=== 同步版本 %s ===\n", targetVersion)

	if dryRun {
		fmt.Printf("[DRY-RUN] 将下载 version.json + client.jar\n")
		return
	}

	// 4. 获取版本元数据
	meta, err := vm.Fetch(targetVersion)
	if err != nil {
		logger.Error("获取版本元数据失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: 获取 %s 元数据失败: %v\n", targetVersion, err)
		return
	}

	fmt.Printf("[✓] 版本元数据: %s (type=%s)\n", meta.ID, meta.Type)
	fmt.Printf("    mainClass: %s\n", meta.MainClass)
	fmt.Printf("    assets: %s\n", meta.Assets)

	if meta.Downloads != nil && meta.Downloads.Client != nil {
		fmt.Printf("    client.jar: %d MB (SHA1: %s)\n",
			meta.Downloads.Client.Size/1024/1024,
			meta.Downloads.Client.Sha1[:12]+"...")
	}

	// 5. 下载 client.jar
	jarPath, err := vm.DownloadClientJar(meta, jarDir)
	if err != nil {
		logger.Error("下载 client.jar 失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: client.jar 下载失败: %v\n", err)
		return
	}

	fmt.Printf("[✓] client.jar: %s\n", jarPath)

	// 6. Asset 索引同步
	assetsDir := filepath.Join(cfgDir, "assets")
	am := launcher.NewAssetManager(cacheDir, assetsDir, mm, vm)

	assetIdx, err := am.FetchIndex(targetVersion)
	if err != nil {
		logger.Error("Asset 索引拉取失败: %v", err)
		fmt.Fprintf(os.Stderr, "sync: Asset 索引拉取失败: %v\n", err)
		return
	}

	stats := am.Statistics(assetIdx)
	fmt.Printf("[✓] Asset 索引: %s (%d 个文件, 总计 %d MB, 平均 %.1f KB)\n",
		meta.Assets, stats.TotalFiles, stats.TotalSize/1024/1024, stats.AvgSize/1024)

	// 7. Asset 文件下载（并发，8 个 worker）
	logger.Info("开始 Asset 文件下载 (8 workers)...")
	assetFiles := am.ListObjects(assetIdx)
	type assetResult struct {
		downloaded int
		skipped    int
		failed     int
	}
	resultCh := make(chan assetResult, len(assetFiles))
	sem := make(chan struct{}, 8)
	// 并发下载 Asset 文件，使用 worker pool 模式：
	//   - sem (channel) 作为信号量，同时最多 8 个 goroutine 持有 slot
	//   - resultCh 收集每个文件的结果（下载/跳过/失败）
	//   - 主 goroutine 通过 for-range resultCh 等待所有任务完成并汇总
	// 选择 8 并发是因为:
	//   - BMCLAPI 镜像对并发连接数有一定容忍度
	//   - 太多并发可能导致本机文件系统 I/O 瓶颈
	//   - 4746 个文件逐一下载太慢（约 30min），8 并发可压到 ~5min

	for _, obj := range assetFiles {
		go func(vpath, hash string) {
			sem <- struct{}{}
			defer func() { <-sem }()

			localPath := am.AssetObjectPath(hash)
			if _, err := os.Stat(localPath); err == nil {
				resultCh <- assetResult{skipped: 1}
				return
			}
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
			resultCh <- assetResult{downloaded: 1}
		}(obj.VirtualPath, obj.Hash)
	}

	var totalDownloaded, totalSkipped, totalFailed int
	for i := 0; i < len(assetFiles); i++ {
		r := <-resultCh
		totalDownloaded += r.downloaded
		totalSkipped += r.skipped
		totalFailed += r.failed
	}
	fmt.Printf("[✓] Asset 文件: %d 下载, %d 已存在, %d 失败\n", totalDownloaded, totalSkipped, totalFailed)

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
		fmt.Println("backup list: not yet implemented")
	case "restore":
		fmt.Println("backup restore: not yet implemented")
	case "create":
		fmt.Println("backup create: not yet implemented")
	case "delete":
		fmt.Println("backup delete: not yet implemented")
	default:
		fmt.Printf("backup: unknown subcommand %s\n", args[0])
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
