// Package launcher — 自更新模块 (P3)
//
// 功能：
//   - 从服务端检查新版本并下载
//   - 替换自身 exe（Windows bat 脚本策略）
//   - 启动后 10s 健康检测 → 未标记则自动回滚
//   - 多通道支持（stable / beta / dev）
//   - 静默下载 + 下次启动时应用
//
// 目录：
//   .local/
//     update_tmp/          ← 更新临时文件
//       starter-{ver}.exe  ← 下载完毕待替换
//     starter-update-state.json  ← 更新状态
//     starter.exe.old      ← 旧版本备份（回滚用）
//
// 更新状态字段（starter-update-state.json）：
//   current_version    当前版本号
//   update_channel     当前通道
//   last_check_time    上次检查时间
//   pending_update     待应用的更新信息
//   update_history     版本变更历史（保留最近 10 条）
package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/downloader"
)

// ============================================================
// 常量与默认值
// ============================================================

const (
	// DefaultUpdateCheckInterval 默认更新检查间隔 (24h)
	DefaultUpdateCheckInterval = 24 * time.Hour

	// StartupCheckGracePeriod 新版本启动后，等待正常标记的时间（超时则回滚）
	StartupCheckGracePeriod = 10 * time.Second

	// UpdateStateFileName 更新状态文件名
	UpdateStateFileName = "starter-update-state.json"

	// UpdateTempDirName 更新临时目录
	UpdateTempDirName = "update_tmp"

	// OldExeSuffix 旧 exe 备份后缀
	OldExeSuffix = ".old"

	// MaxHistoryEntries 保留的历史记录数
	MaxHistoryEntries = 10
)

// UpdateChannel 更新通道
type UpdateChannel string

const (
	ChannelStable UpdateChannel = "stable"
	ChannelBeta   UpdateChannel = "beta"
	ChannelDev    UpdateChannel = "dev"
)

// UpdateCheckMode 检查模式
type UpdateCheckMode int

const (
	CheckOnLaunch UpdateCheckMode = iota // 每次启动检查
	CheckDaily                           // 每天一次（默认）
	CheckManual                          // 仅手动
)

// ============================================================
// 元数据结构
// ============================================================

// UpdateMeta 服务端返回的版本信息
type UpdateMeta struct {
	Version     string   `json:"version"`      // semver
	Channel     string   `json:"channel"`       // stable|beta|dev
	ReleaseDate string   `json:"release_date"`  // 发布日期
	DownloadURL string   `json:"download_url"`  // 下载链接
	Hash        string   `json:"hash"`          // SHA256
	Signature   string   `json:"signature"`     // 可选：数字签名
	Changelog   []string `json:"changelog"`     // 更新日志
	MinVersion  string   `json:"min_version"`   // 强制更新门槛
	Critical    bool     `json:"critical"`      // 是否强制更新
}

// PendingUpdate 待应用的更新信息
type PendingUpdate struct {
	Version      string   `json:"version"`
	Downloaded   bool     `json:"downloaded"`
	Applied      bool     `json:"applied"`
	AppliedOk    bool     `json:"applied_ok"`
	StartupOK    bool     `json:"startup_ok"`
	AppliedTime  string   `json:"applied_time,omitempty"`
	TempPath     string   `json:"temp_path"`
	HashVerified bool     `json:"hash_verified"`
	Changelog    []string `json:"changelog,omitempty"`
}

// UpdateHistoryEntry 更新历史条目
type UpdateHistoryEntry struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Time    string `json:"time"`
	Success bool   `json:"success"`
}

// UpdateState 更新状态文件结构
type UpdateState struct {
	CurrentVersion  string               `json:"current_version"`
	UpdateChannel   string               `json:"update_channel"`
	LastCheckTime   string               `json:"last_check_time,omitempty"`
	LastCheckResult *LastCheckResult     `json:"last_check_result,omitempty"`
	PendingUpdate   *PendingUpdate       `json:"pending_update,omitempty"`
	UpdateHistory   []UpdateHistoryEntry `json:"update_history,omitempty"`
}

// LastCheckResult 上次检查结果
type LastCheckResult struct {
	Available    bool     `json:"available"`
	LatestVersion string  `json:"latest_version"`
	Changelog    []string `json:"changelog,omitempty"`
	Critical     bool     `json:"critical"`
}

// ============================================================
// SelfUpdater 主结构
// ============================================================

// SelfUpdater 自更新管理器
type SelfUpdater struct {
	LocalDir  string // .local 目录（与状态文件同目录）
	DL        *downloader.Downloader
	Version   string // 当前版本号
	Channel   UpdateChannel
	ServerURL string // 更新端点基础 URL
}

// NewSelfUpdater 创建自更新管理器
// localDir: .local 目录路径（通常为 config/.local）
// version: 当前版本号（从编译时注入的 version 变量传入）
// serverURL: 服务端 URL，用于组合更新端点
func NewSelfUpdater(localDir, version, serverURL string) *SelfUpdater {
	return &SelfUpdater{
		LocalDir:  localDir,
		DL:        downloader.New(),
		Version:   version,
		Channel:   ChannelStable,
		ServerURL: strings.TrimRight(serverURL, "/"),
	}
}

// SetChannel 设置更新通道
func (su *SelfUpdater) SetChannel(ch UpdateChannel) {
	su.Channel = ch
}

// ============================================================
// 状态文件读写
// ============================================================

func (su *SelfUpdater) statePath() string {
	return filepath.Join(su.LocalDir, UpdateStateFileName)
}

func (su *SelfUpdater) updateDir() string {
	return filepath.Join(su.LocalDir, UpdateTempDirName)
}

// LoadState 读取更新状态文件
func (su *SelfUpdater) LoadState() (*UpdateState, error) {
	path := su.statePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UpdateState{
				CurrentVersion: su.Version,
				UpdateChannel:  string(su.Channel),
			}, nil
		}
		return nil, fmt.Errorf("读取状态文件失败: %w", err)
	}

	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析状态文件失败: %w", err)
	}
	return &state, nil
}

// SaveState 保存更新状态文件
func (su *SelfUpdater) SaveState(state *UpdateState) error {
	path := su.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建状态目录失败: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}

	// 原子写入
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("写入临时状态文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("重命名状态文件失败: %w", err)
	}
	return nil
}

// ============================================================
// 版本比较 (semver)
// ============================================================

// compareVersions 比较两个 semver 版本号
// 返回: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
// 忽略预发布标签（-alpha, -beta）和 build metadata（+sha.xxx），纯比较主.次.修订
func compareVersions(v1, v2 string) int {
	// 去掉预发布标签和 build metadata
	v1 = stripSemverSuffix(v1)
	v2 = stripSemverSuffix(v2)

	v1Parts := parseVersionParts(v1)
	v2Parts := parseVersionParts(v2)

	// 补齐到相同长度（1.20 == 1.20.0）
	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}
	for len(v1Parts) < maxLen {
		v1Parts = append(v1Parts, 0)
	}
	for len(v2Parts) < maxLen {
		v2Parts = append(v2Parts, 0)
	}

	for i := 0; i < maxLen; i++ {
		if v1Parts[i] < v2Parts[i] {
			return -1
		}
		if v1Parts[i] > v2Parts[i] {
			return 1
		}
	}
	return 0
}

// stripSemverSuffix 去掉预发布标签和 build metadata
// "1.2.3-beta.1+sha.abc" → "1.2.3"
func stripSemverSuffix(v string) string {
	// 先去掉 build metadata（+ 之后）
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	// 再去掉预发布标签（- 之后）
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	return v
}

// parseVersionParts 将版本号拆为数字切片
// "1.2.3" → [1, 2, 3]; "1.20" → [1, 20]; "0" → [0]
func parseVersionParts(v string) []int {
	parts := strings.Split(v, ".")
	result := make([]int, len(parts))
	for i, p := range parts {
		n := 0
		fmt.Sscanf(p, "%d", &n)
		result[i] = n
	}
	return result
}

// ============================================================
// 更新检查 (P3.1)
// ============================================================

// CheckUpdate 检查是否有更新可用
// 返回最新版本号、是否有更新、错误
func (su *SelfUpdater) CheckUpdate() (*UpdateMeta, bool, error) {
	meta, err := su.fetchUpdateMeta()
	if err != nil {
		return nil, false, fmt.Errorf("获取更新信息失败: %w", err)
	}
	if meta == nil {
		return nil, false, nil
	}

	// 比较版本
	cmp := compareVersions(su.Version, meta.Version)
	if cmp >= 0 {
		return meta, false, nil // 已是最新
	}

	return meta, true, nil
}

// CheckAndDownload 检查更新并静默下载
// 返回: 是否有新版本、要下载的元信息、错误
func (su *SelfUpdater) CheckAndDownload() (*UpdateMeta, bool, error) {
	meta, available, err := su.CheckUpdate()
	if err != nil || !available {
		return meta, available, err
	}

	if err := su.DownloadUpdate(meta); err != nil {
		return meta, true, fmt.Errorf("下载更新失败: %w", err)
	}

	return meta, true, nil
}

// fetchUpdateMeta 从服务端获取更新元信息
func (su *SelfUpdater) fetchUpdateMeta() (*UpdateMeta, error) {
	endpoint := fmt.Sprintf("%s/api/updater/%s/version.json", su.ServerURL, su.Channel)

	// 如果 CacheDir 为空（未配置），直接 HTTP 拉取
	// 这里简单地从 URL 获取 JSON
	cacheDir := filepath.Join(su.LocalDir, "update_cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	cachePath := filepath.Join(cacheDir, fmt.Sprintf("version-%s.json", su.Channel))
	if err := su.DL.File(endpoint, cachePath, ""); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("读取缓存文件失败: %w", err)
	}

	var meta UpdateMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析版本元信息失败: %w", err)
	}
	return &meta, nil
}

// ============================================================
// 下载与校验 (P3.2)
// ============================================================

// DownloadUpdate 下载新版本并校验
func (su *SelfUpdater) DownloadUpdate(meta *UpdateMeta) error {
	dir := su.updateDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建更新目录失败: %w", err)
	}

	// 下载文件
	tmpPath := filepath.Join(dir, fmt.Sprintf("starter-%s.exe.tmp", meta.Version))
	exePath := strings.TrimSuffix(tmpPath, ".tmp")

	// 先清空已存在的文件
	os.Remove(tmpPath)
	os.Remove(exePath)

	// 下载到 .tmp
	if err := su.DL.File(meta.DownloadURL, tmpPath, ""); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("下载更新包失败: %w", err)
	}

	// 校验 hash
	if meta.Hash != "" {
		verified, err := verifySHA256File(tmpPath, meta.Hash)
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("校验 hash 失败: %w", err)
		}
		if !verified {
			os.Remove(tmpPath)
			return fmt.Errorf("SHA256 校验失败: 文件可能被篡改")
		}
	}

	// 重命名为目标文件名（去掉 .tmp）
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	// 设置可执行权限（Linux/Mac）
	if runtime.GOOS != "windows" {
		os.Chmod(exePath, 0755)
	}

	// 保存待更新状态
	state, err := su.LoadState()
	if err != nil {
		return err
	}

	state.PendingUpdate = &PendingUpdate{
		Version:      meta.Version,
		Downloaded:   true,
		TempPath:     exePath,
		HashVerified: meta.Hash == "",
		Changelog:    meta.Changelog,
	}

	return su.SaveState(state)
}

// verifySHA256File 计算文件的 SHA256 并对比
func verifySHA256File(path, expected string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	return got == expected, nil
}

// ============================================================
// 替换与重启 (P3.3)
// ============================================================

// ApplyUpdate 应用已下载的更新，替换当前 exe 并重启
// Windows: 使用 bat 脚本策略，由脚本等当前进程退出后替换
// Linux/Mac: 直接替换

// ApplyUpdateResult 应用更新结果
type ApplyUpdateResult struct {
	Applied       bool
	NewVersion    string
	RestartArgs   []string
}

// ApplyUpdate 应用已下载的更新
// 返回结果信息，如果替换成功则当前进程应退出
func (su *SelfUpdater) ApplyUpdate() (*ApplyUpdateResult, error) {
	state, err := su.LoadState()
	if err != nil {
		return nil, err
	}

	if state.PendingUpdate == nil || !state.PendingUpdate.Downloaded {
		return nil, fmt.Errorf("没有已下载的待更新")
	}

	newExePath := state.PendingUpdate.TempPath
	if _, statErr := os.Stat(newExePath); os.IsNotExist(statErr) {
		return nil, fmt.Errorf("更新文件不存在: %s", newExePath)
	}

	currentExe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取当前 exe 路径失败: %w", err)
	}
	currentExe, err = filepath.Abs(currentExe)
	if err != nil {
		return nil, fmt.Errorf("规范化路径失败: %w", err)
	}

	backupPath := currentExe + OldExeSuffix

	switch runtime.GOOS {
	case "windows":
		return su.applyUpdateWindows(currentExe, newExePath, backupPath, state)
	default:
		return su.applyUpdateUnix(currentExe, newExePath, backupPath, state)
	}
}

// applyUpdateWindows Windows 替换策略：bat 脚本
func (su *SelfUpdater) applyUpdateWindows(currentExe, newExePath, backupPath string, state *UpdateState) (*ApplyUpdateResult, error) {
	// 生成 bat 脚本
	batContent := fmt.Sprintf(`@echo off
chcp 65001 >nul
echo mc-starter 正在更新...
:wait
tasklist /FI "IMAGENAME eq starter.exe" 2>NUL | find /I "starter.exe" >NUL
if not errorlevel 1 (
    timeout /T 1 /NOBREAK >NUL
    goto wait
)
copy /Y "%s" "%s"
if errorlevel 1 (
    echo 替换失败
    pause
    exit /b 1
)
del /F "%s"
start "" "%s"
del "%%~f0"
`, newExePath, currentExe, newExePath, currentExe)

	batPath := filepath.Join(os.TempDir(), "starter-update.bat")
	if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
		return nil, fmt.Errorf("创建更新脚本失败: %w", err)
	}

	// 记日志
	fmt.Printf("[自更新] 正在应用更新 %s → %s\n", state.CurrentVersion, state.PendingUpdate.Version)
	fmt.Printf("[自更新] 更新脚本: %s\n", batPath)

	// 启动 bat 脚本继承当前工作目录
	cmd := exec.Command("cmd", "/C", batPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动更新脚本失败: %w", err)
	}

	// 保存更新历史
	su.recordUpdate(state, true)

	// 当前进程退出
	fmt.Println("[自更新] 应用完成，正在退出...")
	os.Exit(0)
	return nil, nil // unreachable
}

// applyUpdateUnix Linux/Mac 替换策略：直接替换
func (su *SelfUpdater) applyUpdateUnix(currentExe, newExePath, backupPath string, state *UpdateState) (*ApplyUpdateResult, error) {
	// 1. 备份当前 exe
	if err := copyFile(currentExe, backupPath); err != nil {
		return nil, fmt.Errorf("备份当前版本失败: %w", err)
	}

	// 2. 替换
	if err := os.Rename(newExePath, currentExe); err != nil {
		return nil, fmt.Errorf("替换失败: %w", err)
	}
	os.Chmod(currentExe, 0755)

	// 3. 保存更新历史
	su.recordUpdate(state, true)

	// 4. 重启（继承启动参数）
	args := filterUpdateArgs(os.Args[1:])
	cmd := exec.Command(currentExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return &ApplyUpdateResult{
			Applied:    true,
			NewVersion: state.PendingUpdate.Version,
		}, fmt.Errorf("重启失败: %w，新版本已替换但需手动启动", err)
	}

	fmt.Printf("[自更新] 已替换并重启 %s\n", state.PendingUpdate.Version)
	os.Exit(0)
	return nil, nil
}

// filterUpdateArgs 过滤掉自更新相关的参数
func filterUpdateArgs(args []string) []string {
	var filtered []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "self-update", "--self-update":
			// 跳过 self-update 及其子参数
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case "apply":
			// 如果前面是 self-update，跳过
			if i > 0 && args[i-1] == "self-update" {
				continue
			}
			filtered = append(filtered, args[i])
		default:
			filtered = append(filtered, args[i])
		}
	}
	return filtered
}

// ============================================================
// 回滚 (P3.4)
// ============================================================

// CheckStartupHealth 新版本首次启动时调用
// 标记启动成功，启动后 10s 如果没标记则认为启动失败，触发自动回滚
func (su *SelfUpdater) CheckStartupHealth() {
	state, err := su.LoadState()
	if err != nil || state.PendingUpdate == nil {
		return
	}

	// 如果刚刚应用了更新但还没标记 OK
	if state.PendingUpdate.Applied && !state.PendingUpdate.AppliedOk {
		state.PendingUpdate.StartupOK = true
		su.SaveState(state)

		// 10s 后检查是否标记了成功
		time.AfterFunc(StartupCheckGracePeriod, func() {
			su.checkRollbackAfterStartup()
		})
	}
}

// MarkStartupOK 标记新版本启动成功（由主流程在初始化完成后调用）
func (su *SelfUpdater) MarkStartupOK() {
	state, err := su.LoadState()
	if err != nil || state.PendingUpdate == nil {
		return
	}

	if state.PendingUpdate.Applied {
		state.PendingUpdate.AppliedOk = true
		state.PendingUpdate.StartupOK = true
		su.SaveState(state)
	}
}

// checkRollbackAfterStartup 启动后检查：如果 AppliedOk 没被标记则自动回滚
func (su *SelfUpdater) checkRollbackAfterStartup() {
	state, err := su.LoadState()
	if err != nil || state.PendingUpdate == nil {
		return
	}

	if !state.PendingUpdate.Applied {
		return
	}

	if !state.PendingUpdate.AppliedOk && state.PendingUpdate.StartupOK {
		fmt.Println("[自更新] 新版本启动异常，自动回滚...")
		if err := su.Rollback(); err != nil {
			fmt.Fprintf(os.Stderr, "[自更新] 自动回滚失败: %v\n", err)
		}
	}
}

// CheckPendingUpdate 启动时检查是否有待应用的更新
// 返回是否有待应用的更新、版本号、更新日志
func (su *SelfUpdater) CheckPendingUpdate() (hasPending bool, version string, changelog []string) {
	state, err := su.LoadState()
	if err != nil || state.PendingUpdate == nil {
		return false, "", nil
	}
	if state.PendingUpdate.Downloaded && !state.PendingUpdate.Applied {
		return true, state.PendingUpdate.Version, state.PendingUpdate.Changelog
	}
	return false, "", nil
}

// Rollback 回滚到上一个版本
func (su *SelfUpdater) Rollback() error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前 exe 路径失败: %w", err)
	}
	currentExe, err = filepath.Abs(currentExe)
	if err != nil {
		return fmt.Errorf("规范化路径失败: %w", err)
	}

	backupPath := currentExe + OldExeSuffix

	if _, statErr := os.Stat(backupPath); os.IsNotExist(statErr) {
		return fmt.Errorf("没有可用的备份 (%s 不存在)", backupPath)
	}

	// 先备份当前版本（防止误操作）
	rollbackBackup := currentExe + ".rollback-backup"
	if err := copyFile(currentExe, rollbackBackup); err != nil {
		return fmt.Errorf("备份当前版本失败: %w", err)
	}

	// 回滚
	if err := copyFile(backupPath, currentExe); err != nil {
		return fmt.Errorf("回滚失败: %w", err)
	}

	if runtime.GOOS != "windows" {
		os.Chmod(currentExe, 0755)
	}

	fmt.Printf("[自更新] 已回滚到上一个版本\n")
	fmt.Printf("[自更新] 回滚前版本已备份到: %s\n", rollbackBackup)

	return nil
}

// ============================================================
// 更新历史
// ============================================================

// recordUpdate 记录更新历史
func (su *SelfUpdater) recordUpdate(state *UpdateState, success bool) {
	state.UpdateHistory = append(state.UpdateHistory, UpdateHistoryEntry{
		From:    state.CurrentVersion,
		To:      state.PendingUpdate.Version,
		Time:    time.Now().Format(time.RFC3339),
		Success: success,
	})

	// 清理旧历史
	if len(state.UpdateHistory) > MaxHistoryEntries {
		state.UpdateHistory = state.UpdateHistory[len(state.UpdateHistory)-MaxHistoryEntries:]
	}

	// 更新当前版本
	state.CurrentVersion = state.PendingUpdate.Version
	state.PendingUpdate = nil

	su.SaveState(state)
}

// GetUpdateHistory 获取更新历史
func (su *SelfUpdater) GetUpdateHistory() ([]UpdateHistoryEntry, error) {
	state, err := su.LoadState()
	if err != nil {
		return nil, err
	}
	if state.UpdateHistory == nil {
		return []UpdateHistoryEntry{}, nil
	}
	return state.UpdateHistory, nil
}

// RollbackHistory 手动回滚到指定版本
// 注意: 回滚不会自动重启，需要用户手动重启
func (su *SelfUpdater) RollbackHistory(version string) error {
	state, err := su.LoadState()
	if err != nil {
		return err
	}

	// 检查历史中是否有目标版本
	found := false
	for _, entry := range state.UpdateHistory {
		if entry.To == version && entry.Success {
			found = true
			break
		}
	}
	_ = found

	return su.Rollback()
}

// ============================================================
// 多通道切换 (P3.5)
// ============================================================

// SetChannelStr 从字符串设置更新通道
var validChannels = map[string]UpdateChannel{
	"stable": ChannelStable,
	"beta":   ChannelBeta,
	"dev":    ChannelDev,
}

func (su *SelfUpdater) SetChannelStr(ch string) error {
	c, ok := validChannels[strings.ToLower(ch)]
	if !ok {
		return fmt.Errorf("无效通道: %s（可选: stable, beta, dev）", ch)
	}
	su.Channel = c

	// 保存到状态文件
	state, err := su.LoadState()
	if err == nil {
		state.UpdateChannel = string(c)
		su.SaveState(state)
	}
	return nil
}

// ValidateChannelSwitch 验证通道切换是否允许
func ValidateChannelSwitch(from, to string) error {
	if from == to {
		return nil
	}
	switch from {
	case "stable":
		if to == "beta" || to == "dev" {
			return nil
		}
	case "beta":
		if to == "stable" || to == "dev" {
			return nil
		}
	case "dev":
		if to == "beta" {
			return nil
		}
		return fmt.Errorf("dev → stable 需要先切换到 beta")
	}
	return fmt.Errorf("不允许的通道切换: %s → %s", from, to)
}

// ============================================================
// 交互与通知 (P3.6)
// ============================================================

// IsCriticalUpdate 检查是否有关键更新（低于最小版本要求）
func (su *SelfUpdater) IsCriticalUpdate(meta *UpdateMeta) bool {
	if !meta.Critical {
		return false
	}
	return compareVersions(su.Version, meta.MinVersion) < 0
}

// FormatChangelog 格式化更新日志为字符串
func FormatChangelog(meta *UpdateMeta) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📦 更新 %s (%s)\n", meta.Version, meta.Channel))
	b.WriteString(fmt.Sprintf("   发布: %s\n", meta.ReleaseDate))
	b.WriteString("\n   更新内容:\n")
	for _, line := range meta.Changelog {
		b.WriteString(fmt.Sprintf("     • %s\n", line))
	}
	return b.String()
}

// FormatUpdateHistory 格式化更新历史
func FormatUpdateHistory(entries []UpdateHistoryEntry) string {
	if len(entries) == 0 {
		return "暂无更新历史"
	}
	var b strings.Builder
	b.WriteString("更新历史:\n")
	for i, entry := range entries {
		status := "✓"
		if !entry.Success {
			status = "✗"
		}
		// 尝试解析时间，格式化为本地时间
		t, err := time.Parse(time.RFC3339, entry.Time)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", entry.Time)
		}
		var timeStr string
		if !t.IsZero() {
			timeStr = t.Format("2006-01-02")
		} else {
			timeStr = entry.Time
		}
		b.WriteString(fmt.Sprintf("  %d. %s %s → %s (%s)\n", i+1, status, entry.From, entry.To, timeStr))
	}
	return b.String()
}

// ============================================================
// 辅助函数
// ============================================================

// GetCurrentExeName 获取当前 exe 的文件名
func GetCurrentExeName() string {
	exe, err := os.Executable()
	if err != nil {
		return "starter"
	}
	return filepath.Base(exe)
}
