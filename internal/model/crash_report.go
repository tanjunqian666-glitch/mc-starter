// Package model — 崩溃报告相关类型
//
// P2.10 崩溃报告上传：检测到崩溃后收集信息上传到服务端

package model

import "time"

// CrashReport 上传到服务端的崩溃报告
type CrashReport struct {
	Version       string `json:"version"`         // 更新器版本
	MCVersion     string `json:"mc_version"`      // MC 版本号
	LoaderType    string `json:"loader_type"`     // fabric / forge / vanilla
	LoaderVersion string `json:"loader_version"`  // Loader 版本
	ModList       []Mod  `json:"mod_list"`        // 模组列表（名称+版本）
	ExitCode      int    `json:"exit_code"`       // 退出码
	ErrorMessage  string `json:"error_message"`   // 崩溃原因
	LogTail       string `json:"log_tail"`        // latest.log 末尾 N 行
	CrashReport   string `json:"crash_report"`    // 完整崩溃报告内容
	JVMError      string `json:"jvm_error"`       // hs_err 文件内容
	ConfigHash    string `json:"config_hash"`     // server.json/last sync hash
	Timestamp     string `json:"timestamp"`       // ISO8601
}

// Mod 简化的模组信息
type Mod struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	File    string `json:"file,omitempty"`
}

// CrashReportUploadRequest 上传请求体
type CrashReportUploadRequest struct {
	PackName string       `json:"pack_name"`
	Report   CrashReport `json:"report"`
}

// CrashReportUploadResponse 上传响应
type CrashReportUploadResponse struct {
	Status  string `json:"status"`
	Advice  string `json:"advice,omitempty"` // 服务端可能返回建议
	Ticket  string `json:"ticket,omitempty"` // 追踪工单号
}

// NewCrashReport 创建一个崩溃报告
func NewCrashReport(errMsg string, exitCode int) CrashReport {
	return CrashReport{
		ErrorMessage: errMsg,
		ExitCode:     exitCode,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
}
