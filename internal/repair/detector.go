package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P2.8 崩溃检测器
//
// 设计原则：
//   - 不依赖进程关系（我们不启动 MC，只做检测）
//   - 主方案：fsnotify 文件系统事件监听（0 CPU 开销）
//   - 降级方案：30s 定时轮询（fsnotify 不可用时）
//   - 已读标记机制防止重复报警
//
// 检测目标：
//   1. crash-reports/*.txt — MC 客户端崩溃报告
//   2. hs_err_pid*.log — JVM 致命错误日志
//   3. 其他 *.txt 崩溃输出（可选扩展）
// ============================================================

// DetectorState 检测器持久化状态
type DetectorState struct {
	LastCheck    time.Time           `json:"last_check"`     // 最后检测时间
	LastCrash    time.Time           `json:"last_crash"`     // 最后崩溃时间
	LastCrashID  string              `json:"last_crash_id"`  // 最后崩溃文件标识（hash 或时间戳）
	SeenFiles    map[string]string   `json:"seen_files"`     // 已处理文件 → 标识
}

// CrashEvent 崩溃事件
type CrashEvent struct {
	Time     time.Time `json:"time"`      // 检测到崩溃的时间
	Type     string    `json:"type"`      // "crash_report" / "hs_err" / "unknown"
	FilePath string    `json:"file_path"` // 触发崩溃的文件路径
	Reason   string    `json:"reason"`    // 提取的崩溃原因（短文本）
}

// CrashHandler 崩溃回调类型
type CrashHandler func(event CrashEvent)

// Detector 崩溃检测器
type Detector struct {
	mu           sync.Mutex
	mcDir        string
	statePath    string
	state        DetectorState
	watcher      *fsnotify.Watcher
	pollTicker   *time.Ticker
	pollOnly     bool
	onCrash      CrashHandler
	started      bool
	stopCh       chan struct{}
	debounceDur  time.Duration // 防抖等待时间
}

// DetectorOptions 检测器配置
type DetectorOptions struct {
	MCVersion   string      // 关联的 MC 版本（可选，用于状态记录）
	OnCrash     CrashHandler // 崩溃回调
	PollOnly    bool         // 强制使用轮询模式（调试用）
	DebounceDur time.Duration // 防抖时间（默认 2s）
}

// NewDetector 创建崩溃检测器
// mcDir: .minecraft 根目录
// opts: 可选配置（OnCrash 等），传递 nil 使用默认值
func NewDetector(mcDir string, opts *DetectorOptions) *Detector {
	d := &Detector{
		mcDir:       mcDir,
		statePath:   filepath.Join(mcDir, "starter_backups", "detector_state.json"),
		state:       DetectorState{SeenFiles: make(map[string]string)},
		stopCh:      make(chan struct{}),
		debounceDur: 2 * time.Second,
	}

	if opts != nil {
		d.onCrash = opts.OnCrash
		if opts.PollOnly {
			d.pollOnly = true
		}
		if opts.DebounceDur > 0 {
			d.debounceDur = opts.DebounceDur
		}
	}

	// 加载持久化状态
	d.loadState()

	return d
}

// Start 启动检测器
func (d *Detector) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return nil
	}
	d.started = true

	// 重新创建 stopCh（防止 Stop 后复用）
	d.stopCh = make(chan struct{})

	// 尝试初始化 fsnotify
	if !d.pollOnly {
		w, err := fsnotify.NewWatcher()
		if err == nil {
			d.watcher = w
			logger.Info("崩溃检测器: 使用 fsnotify 事件监听模式")

			// 监听 crash-reports/ 目录
			crashDir := filepath.Join(d.mcDir, "crash-reports")
			if watchErr := d.ensureAndWatch(crashDir); watchErr != nil {
				logger.Warn("崩溃检测器: 无法监听 crash-reports/: %v", watchErr)
			}

			// 监听 .minecraft 根目录（hs_err_pid*.log）
			if watchErr := d.watcher.Add(d.mcDir); watchErr != nil {
				logger.Warn("崩溃检测器: 无法监听 .minecraft 根目录: %v", watchErr)
			}

			go d.eventLoop()
		} else {
			logger.Warn("崩溃检测器: fsnotify 初始化失败 (%v), 降级到轮询模式", err)
			d.pollOnly = true
		}
	}

	if d.pollOnly {
		logger.Info("崩溃检测器: 使用轮询模式 (30s 间隔)")
		d.pollTicker = time.NewTicker(30 * time.Second)
		go d.pollLoop()
	}

	return nil
}

// Stop 停止检测器
func (d *Detector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started {
		return
	}
	d.started = false

	// 关闭旧 channel，通知所有 goroutine 退出
	close(d.stopCh)

	if d.watcher != nil {
		d.watcher.Close()
		d.watcher = nil
	}
	if d.pollTicker != nil {
		d.pollTicker.Stop()
		d.pollTicker = nil
	}

	d.saveState()
}

// PollNow 手动触发一次检测（供外部调用）
// 如果设置了 OnCrash 回调，检测到的崩溃事件会触发回调
func (d *Detector) PollNow() []CrashEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	events := d.scanCrashFiles()
	for _, ev := range events {
		d.recordCrash(ev)
		d.fireCrash(ev)
	}
	return events
}

// LastCrashTime 返回最后检测到的崩溃时间
func (d *Detector) LastCrashTime() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state.LastCrash
}

// LastCrashID 返回最后崩溃文件标识
func (d *Detector) LastCrashID() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state.LastCrashID
}

// setOnCrash 设置崩溃回调（运行时动态调整）
func (d *Detector) SetOnCrash(handler CrashHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onCrash = handler
}

// ============================================================
// 事件驱动循环（fsnotify）
// ============================================================

func (d *Detector) eventLoop() {
	// 防抖用的聚合 map：文件名 → 触发时间
	debounce := make(map[string]time.Time)
	var debounceMu sync.Mutex

	// 防抖协程
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			d.mu.Lock()
			debounceMu.Lock()
			now := time.Now()
			for name, firstSeen := range debounce {
				if now.Sub(firstSeen) >= d.debounceDur {
					delete(debounce, name)
					d.mu.Unlock()
					// 释放锁再扫描，避免长时间持有
					events := d.scanCrashFilesFiltered(name)
					d.mu.Lock()
					for _, ev := range events {
						d.recordCrash(ev)
						d.fireCrash(ev)
					}
				}
			}
			debounceMu.Unlock()
			d.mu.Unlock()
		}
	}()

	for {
		select {
		case <-d.stopCh:
			return

		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}

			// 只关心新建和写入事件
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			name := filepath.Base(event.Name)
			if !isCrashFileName(name) {
				continue
			}

			// 加入防抖队列
			debounceMu.Lock()
			if _, exists := debounce[name]; !exists {
				debounce[name] = time.Now()
			}
			debounceMu.Unlock()

		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			logger.Warn("崩溃检测器: fsnotify 错误: %v", err)
		}
	}
}

// ============================================================
// 轮询模式
// ============================================================

func (d *Detector) pollLoop() {
	// 等待 pollTicker 初始化（在 Start 中设置）
	ticker := d.pollTicker
	for ticker == nil {
		select {
		case <-d.stopCh:
			return
		default:
			time.Sleep(10 * time.Millisecond)
			ticker = d.pollTicker
		}
	}

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.mu.Lock()
			events := d.scanCrashFiles()
			for _, ev := range events {
				d.recordCrash(ev)
				d.fireCrash(ev)
			}
			d.mu.Unlock()
		}
	}
}

// ============================================================
// 崩溃文件扫描（核心逻辑）
// ============================================================

// scanCrashFiles 扫描全部崩溃文件，返回新事件
// 调用方需持有 d.mu 锁
func (d *Detector) scanCrashFiles() []CrashEvent {
	var events []CrashEvent

	// 1. 扫描 crash-reports/
	crashDir := filepath.Join(d.mcDir, "crash-reports")
	if dirEntries, err := os.ReadDir(crashDir); err == nil {
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isCrashReportFile(name) {
				continue
			}
			if d.isSeen(name) {
				continue
			}
			fullPath := filepath.Join(crashDir, name)
			if !isRecentlyModified(fullPath, 24*time.Hour) {
				continue // 超过 24h 的忽略
			}
			events = append(events, d.buildCrashEvent("crash_report", fullPath, name))
		}
	}

	// 2. 扫描 hs_err_pid*.log
	hsErrGlob := filepath.Join(d.mcDir, "hs_err_pid*.log")
	if matches, err := filepath.Glob(hsErrGlob); err == nil {
		for _, fullPath := range matches {
			name := filepath.Base(fullPath)
			if d.isSeen(name) {
				continue
			}
			if !isRecentlyModified(fullPath, 24*time.Hour) {
				continue
			}
			events = append(events, d.buildCrashEvent("hs_err", fullPath, name))
		}
	}

	return events
}

// scanCrashFilesFiltered 扫描指定文件名的崩溃文件
// 用于防抖完成后精确扫描某文件
func (d *Detector) scanCrashFilesFiltered(filename string) []CrashEvent {
	var events []CrashEvent

	// 在 crash-reports/ 找
	crashPath := filepath.Join(d.mcDir, "crash-reports", filename)
	if _, err := os.Stat(crashPath); err == nil {
		if !d.isSeen(filename) {
			if isRecentlyModified(crashPath, 24*time.Hour) {
				events = append(events, d.buildCrashEvent("crash_report", crashPath, filename))
			}
		}
	}

	// 在根目录找（hs_err 或其它 *.txt）
	rootPath := filepath.Join(d.mcDir, filename)
	if _, err := os.Stat(rootPath); err == nil {
		if rootPath != crashPath && !d.isSeen(filename) {
			if isRecentlyModified(rootPath, 24*time.Hour) {
				eventType := "crash_report"
				if strings.HasPrefix(filename, "hs_err_pid") {
					eventType = "hs_err"
				}
				events = append(events, d.buildCrashEvent(eventType, rootPath, filename))
			}
		}
	}

	return events
}

// buildCrashEvent 从文件路径构建崩溃事件
func (d *Detector) buildCrashEvent(eventType, fullPath, name string) CrashEvent {
	ev := CrashEvent{
		Time:     time.Now(),
		Type:     eventType,
		FilePath: fullPath,
		Reason:   extractCrashReason(fullPath, eventType),
	}
	return ev
}

// recordCrash 记录崩溃到状态
func (d *Detector) recordCrash(ev CrashEvent) {
	fileID := fileIDFromPath(ev.FilePath)
	d.state.LastCheck = time.Now()
	d.state.LastCrash = ev.Time
	d.state.LastCrashID = fileID
	d.state.SeenFiles[filepath.Base(ev.FilePath)] = fileID
	d.saveState()

	logger.Info("检测到崩溃: %s (%s)", ev.Reason, ev.Type)
}

// fireCrash 触发崩溃回调（goroutine 中执行）
func (d *Detector) fireCrash(ev CrashEvent) {
	if d.onCrash != nil {
		go d.onCrash(ev)
	}
}

// ============================================================
// 辅助函数
// ============================================================

// isSeen 检查文件是否已处理
func (d *Detector) isSeen(name string) bool {
	_, exists := d.state.SeenFiles[name]
	return exists
}

// ensureAndWatch 确保目录存在并加入监听
func (d *Detector) ensureAndWatch(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return d.watcher.Add(dir)
}

// isCrashFileName 判断文件名是否是崩溃相关
func isCrashFileName(name string) bool {
	// JVM hs_err 日志
	if strings.HasPrefix(name, "hs_err_pid") && strings.HasSuffix(name, ".log") {
		return true
	}
	// Crash report 文件名格式: crash-YYYY-MM-DD_HH.MM.SS[-client/server].txt
	if strings.HasPrefix(name, "crash-") && strings.HasSuffix(name, ".txt") {
		return true
	}
	return false
}

// isRecentlyModified 检查文件是否在指定时间内被修改
func isRecentlyModified(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < maxAge
}

// fileIDFromPath 从文件路径生成唯一标识
func fileIDFromPath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("%s-%d", path, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%d", path, info.Size(), info.ModTime().UnixNano())
}

// isCrashReportFile 判断是否为崩溃报告文件名（仅在 crash-reports/ 目录内使用）
func isCrashReportFile(name string) bool {
	return strings.HasSuffix(name, ".txt")
}

// extractCrashReason 从文件中提取崩溃原因摘要
func extractCrashReason(path string, eventType string) string {
	// hs_err 文件用 JVM 错误摘要
	if eventType == "hs_err" {
		return "JVM 致命错误，请检查 hs_err_pid*.log"
	}

	// 崩溃报告读取前几行
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "未知原因"
	}

	// 尝试在崩溃报告中找关键信息
	content := string(data)
	lines := strings.Split(content, "\n")

	// 常见崩溃原因关键词
	reasons := []string{
		"OutOfMemoryError",
		"StackOverflowError",
		"NullPointerException",
		"UnsupportedClassVersionError",
		"NoClassDefFoundError",
		"UnsatisfiedLinkError",
		"ClassNotFoundException",
		"Exception in thread",
	}

	for _, line := range lines {
		upper := strings.ToUpper(line)
		for _, reason := range reasons {
			if strings.Contains(upper, strings.ToUpper(reason)) {
				if len(reason) > 40 {
					return reason[:40]
				}
				return reason
			}
		}
	}

	// 没有匹配到常见原因，取第一行非空内容
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if len(trimmed) > 60 {
				return trimmed[:60]
			}
			return trimmed
		}
	}

	return "Minecraft 崩溃"
}

// ============================================================
// 状态持久化
// ============================================================

func (d *Detector) loadState() {
	data, err := os.ReadFile(d.statePath)
	if err != nil {
		return // 新安装，无历史状态
	}
	if err := json.Unmarshal(data, &d.state); err != nil {
		logger.Warn("崩溃检测器: 解析状态文件失败, 重置: %v", err)
		d.state = DetectorState{SeenFiles: make(map[string]string)}
		return
	}
	if d.state.SeenFiles == nil {
		d.state.SeenFiles = make(map[string]string)
	}
	logger.Debug("崩溃检测器: 已加载状态, 上次检测 %s, 上次崩溃 %s",
		d.state.LastCheck.Format(time.RFC3339),
		d.state.LastCrash.Format(time.RFC3339))
}

func (d *Detector) saveState() {
	dir := filepath.Dir(d.statePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(d.state, "", "  ")
	if err != nil {
		return
	}
	// 原子写入：先写 tmp 再 rename
	tmpPath := d.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}
	os.Rename(tmpPath, d.statePath)
}

// ResetState 重置检测器状态（相当于清空"已读"标记）
// 通常不需要调用，仅用于调试或强制重新检测
func (d *Detector) ResetState() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = DetectorState{SeenFiles: make(map[string]string)}
	d.saveState()
}

// PollDetect 单次检测（不启动后台循环，适合 CLI 场景）
// 返回检测到的崩溃事件列表
// 注意：每次调用都会更新持久化状态，重复调用不会返回相同的事件
func PollDetect(mcDir string) []CrashEvent {
	d := NewDetector(mcDir, nil)
	d.mu.Lock()
	defer d.mu.Unlock()
	events := d.scanCrashFiles()
	for _, ev := range events {
		d.recordCrash(ev)
	}
	d.saveState()
	return events
}
