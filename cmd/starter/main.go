package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
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
	fmt.Println("run: not yet implemented")
}

func initialize(cfgDir string) {
	fmt.Println("init: not yet implemented")
}

func check(cfgDir string, verbose bool) {
	fmt.Println("check: not yet implemented")
}

func sync(cfgDir string, verbose bool, dryRun bool) {
	fmt.Println("sync: not yet implemented")
}

func repair(cfgDir string, headless bool) {
	fmt.Println("repair: not yet implemented")
}

func handleBackup(args []string) {
	fmt.Println("backup: not yet implemented")
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
