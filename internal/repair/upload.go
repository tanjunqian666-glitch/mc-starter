// Package repair — 崩溃报告上传
//
// P2.10 崩溃报告上传：检测到崩溃后收集信息上传到 mc-starter-server API

package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// Uploader 崩溃报告上传器
type Uploader struct {
	cfgDir   string
	packName string
	mg       *config.Manager
}

// NewUploader 创建上传器
// cfgDir: config 目录
// packName: 关联的包名
func NewUploader(cfgDir, packName string) *Uploader {
	return &Uploader{
		cfgDir:   cfgDir,
		packName: packName,
		mg:       config.New(cfgDir),
	}
}

// UploadOptions 上传选项
type UploadOptions struct {
	MCVersion     string // MC 版本号
	LoaderType    string // fabric/forge/vanilla
	LoaderVersion string // Loader 版本
	ModList       []model.Mod // 模组列表
	ConfigHash    string // 同步时的配置 hash
}

// CollectAndUpload 收集崩溃报告并上传
// mcDir: .minecraft 目录
// exitCode: 进程退出码
// errMsg: 崩溃描述
// opts: 上传选项（可选）
// 返回: 上传响应（含工单号），错误
func CollectAndUpload(mcDir, cfgDir, packName string, exitCode int, errMsg string, opts *UploadOptions) (*model.CrashReportUploadResponse, error) {
	report := model.NewCrashReport(errMsg, exitCode)

	// 收集信息
	collectLogTail(mcDir, &report)
	collectCrashReport(mcDir, &report)
	collectJVMError(mcDir, &report)

	// 可选信息
	if opts != nil {
		report.MCVersion = opts.MCVersion
		report.LoaderType = opts.LoaderType
		report.LoaderVersion = opts.LoaderVersion
		report.ModList = opts.ModList
		report.ConfigHash = opts.ConfigHash
	}

	// 读取配置获取 server URL
	mg := config.New(cfgDir)
	localCfg, err := mg.LoadLocal()
	if err != nil || localCfg.ServerURL == "" {
		return nil, fmt.Errorf("服务端未配置: %w", err)
	}

	// 上传
	return mg.PostCrashReport(localCfg.ServerURL, packName, report)
}

// UploadFromCrashEvent 从崩溃事件对象上传
// 方便在 daemon 回调和 CLI 中直接调用
func UploadFromCrashEvent(ev CrashEvent, cfgDir, packName string) (*model.CrashReportUploadResponse, error) {
	return CollectAndUpload(
		filepath.Dir(filepath.Dir(ev.FilePath)), // 从崩溃文件路径反推 mcDir
		cfgDir, packName, 0, ev.Reason, nil,
	)
}

// collectLogTail 收集 latest.log 末尾 N 行
func collectLogTail(mcDir string, report *model.CrashReport) {
	logPath := filepath.Join(mcDir, "logs", "latest.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	// 取末尾 50 行
	tailLines := 50
	if len(lines) < tailLines {
		tailLines = len(lines)
	}
	report.LogTail = strings.Join(lines[len(lines)-tailLines:], "\n")
}

// collectCrashReport 读取最新的崩溃报告文件
func collectCrashReport(mcDir string, report *model.CrashReport) {
	crashDir := filepath.Join(mcDir, "crash-reports")
	entries, err := os.ReadDir(crashDir)
	if err != nil {
		return
	}

	// 找最新的 .txt
	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().Unix()
		if modTime > newestTime {
			newestTime = modTime
			newest = filepath.Join(crashDir, e.Name())
		}
	}

	if newest == "" {
		return
	}

	data, err := os.ReadFile(newest)
	if err != nil {
		return
	}

	// 截取前 16KB 避免上传过大
	maxSize := 16 * 1024
	if len(data) > maxSize {
		data = data[:maxSize]
	}
	report.CrashReport = string(data)
}

// collectJVMError 读取 JVM hs_err 日志
func collectJVMError(mcDir string, report *model.CrashReport) {
	matches, err := filepath.Glob(filepath.Join(mcDir, "hs_err_pid*.log"))
	if err != nil {
		return
	}
	if len(matches) == 0 {
		return
	}

	// 最新的
	newest := matches[0]
	newestTime := int64(0)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.ModTime().Unix() > newestTime {
			newestTime = info.ModTime().Unix()
			newest = m
		}
	}

	data, err := os.ReadFile(newest)
	if err != nil {
		return
	}

	maxSize := 16 * 1024
	if len(data) > maxSize {
		data = data[:maxSize]
	}
	report.JVMError = string(data)
}
