package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/server"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "start":
		cmdStart(args)
	case "init":
		cmdInit(args)
	case "check":
		cmdCheck(args)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`mc-starter-server — Minecraft 整合包更新服务端

用法:
  mc-starter-server start [--config <path>]  启动服务
  mc-starter-server init   [--config <path>]  生成默认配置
  mc-starter-server check  [--config <path>]  检查配置

默认配置文件路径: ./server.yml`)
}

func parseStartFlags(args []string) (string, bool) {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	configPath := fs.String("config", "server.yml", "配置文件路径")
	fs.Parse(args)
	return *configPath, true
}

func cmdStart(args []string) {
	configPath, _ := parseStartFlags(args)

	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("初始化服务失败: %v", err)
	}

	log.Printf("mc-starter-server v1.0.0 启动中...")
	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("服务异常退出: %v", err)
	}
}

func cmdInit(args []string) {
	configPath := "server.yml"
	if len(args) > 0 && args[0] == "--config" && len(args) > 1 {
		configPath = args[1]
	}

	// 如果文件已存在则询问覆盖
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("配置文件 %s 已存在，覆盖? [y/N]: ", configPath)
		var resp string
		fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" {
			fmt.Println("已取消")
			os.Exit(0)
		}
	}

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if dir != "." {
		os.MkdirAll(dir, 0755)
	}

	cfg := server.DefaultConfig()
	if err := server.SaveConfig(configPath, cfg); err != nil {
		log.Fatalf("保存配置失败: %v", err)
	}

	fmt.Printf("配置文件已生成: %s\n", configPath)
	fmt.Println("请编辑配置文件中的 admin_token 后启动服务。")
}

func cmdCheck(args []string) {
	configPath := "server.yml"
	if len(args) > 0 && args[0] == "--config" && len(args) > 1 {
		configPath = args[1]
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("配置文件不存在: %s (可使用 'init' 命令生成)", configPath)
	}

	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}

	fmt.Println("配置校验通过!")
	fmt.Printf("  监听地址: %s\n", cfg.ListenAddr())
	fmt.Printf("  TLS: %v\n", cfg.Server.TLSEnabled)
	fmt.Printf("  认证: %v\n", cfg.Auth.Enabled)
	fmt.Printf("  数据目录: %s\n", cfg.Storage.DataDir)
	fmt.Printf("  Packs 目录: %s\n", cfg.Storage.PacksDir)
}
