package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P2.9 静默守护 — 后台进程监控 + 崩溃检测
//
// 设计借鉴自 PCL2（Plain Craft Launcher 2）的 ModWatcher + CrashAnalyzer：
//
// PCL2 思路       →  我们实现
// ─────────────────────────────────────────────────
// Process 输出流   →  进程捕获暂缺（跨平台限制）
// 日志关键词实时    →  latest.log 增量扫描 + 关键词
// 退出码双重判断   →  ExitCode != 0 + 窗口/时间判定
// 窗口检测         →  Windows 专用（暂缺）
// 加载进度跟踪     →  日志进度阶段标记
// 崩溃报告目录扫描  →  Detector 复用
// 异步崩溃分析     →  Daemon → CrashHandler 回调
//
// 与 P2.13 托盘的关系：
//   - Daemon = 后台逻辑引擎（纯扫描+检测）
//   - Tray = 前端展示（托盘图标+右键菜单）
//   - Daemon 可独立运行（headless 模式不依赖托盘）
//
// 设计原则：
//   - 跨平台：Linux/macOS 用 /proc/{pid}/status，Windows 用 tasklist（桩）
//   - 无新增外部依赖
// ============================================================

// DaemonEvent 守护进程事件类型
type DaemonEvent string

const (
	EventProcessExited DaemonEvent = "process_exited" // 监控的进程退出了
	EventCrashDetected DaemonEvent = "crash_detected"  // 检测到新崩溃
	EventLogError      DaemonEvent = "log_error"       // 日志中检测到异常
	EventMCStarted     DaemonEvent = "mc_started"      // MC 进程启动
	EventDaemonStopped DaemonEvent = "daemon_stopped"  // 守护进程停止
)

// DaemonEventHandler 守护事件回调
type DaemonEventHandler func(event DaemonEvent, data interface{})

// WatchedProcess 被监控的进程
type WatchedProcess struct {
	Name string // 进程名（Windows: "javaw.exe" / Linux: "java"）
	PID  int    // 进程 PID（0 表示自动检测）
}

// DaemonConfig 守护配置
type DaemonConfig struct {
	MinecraftDir string            // .minecraft 目录
	WatchedProcs []WatchedProcess  // 待监控的进程列表（空=自动检测）
	PollInterval time.Duration     // 轮询间隔（默认 5s）
	StartTime    time.Time         // MC 启动时间（用于过滤历史崩溃）
	OnEvent      DaemonEventHandler // 事件回调（nil 则只日志）
}

// LogProgressStage 日志加载进度阶段
// 借鉴 PCL2 ModWatcher 的 5 阶段进度标记
type LogProgressStage int

const (
	LogStageNone       LogProgressStage = 0
	LogStageOutput     LogProgressStage = 1 // 有日志输出
	LogStageUserSet    LogProgressStage = 2 // Setting user:
	LogStageLWJGL      LogProgressStage = 3 // LWJGL 版本确认
	LogStageSound      LogProgressStage = 4 // OpenAL initialized
	LogStageTextures   LogProgressStage = 5 // 材质加载完成（MC 窗口已出现）
)

// crashSignal 一次潜在的崩溃信号，等待验证
type crashSignal struct {
	key    string    // 去重 key
	source string    // "log_keyword" / "log_error" / "exit_code"
	detail string    // 日志行或错误描述
	time   time.Time // 记录时间
}

// Daemon 后台守护结构
type Daemon struct {
	mu       sync.Mutex
	cfg      DaemonConfig
	detector *Detector
	stopCh   chan struct{}
	started  bool

	// 运行时状态
	logPos              int64             // latest.log 读取位置
	logStage            LogProgressStage  // 加载进度阶段
	knownProcesses      map[string]bool   // 已知进程（key -> alive）
	windowShown         bool              // 窗口是否已出现
	processStarted      bool              // 是否观察到了进程启动
	pendingCrashSignals []crashSignal     // 待验证的崩溃信号
	lastVerifiedCrash   time.Time         // 上次已验证的崩溃时间
}

// NewDaemon 创建守护进程
func NewDaemon(cfg DaemonConfig) *Daemon {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now()
	}

	if len(cfg.WatchedProcs) == 0 {
		cfg.WatchedProcs = defaultWatchedProcs()
	}

	d := &Daemon{
		cfg:            cfg,
		stopCh:         make(chan struct{}),
		knownProcesses: make(map[string]bool),
	}

	// 创建崩溃检测器（PollOnly 模式，由 Daemon 统一驱动轮询）
	d.detector = NewDetector(cfg.MinecraftDir, &DetectorOptions{
		PollOnly:    true,
		DebounceDur: 2 * time.Second,
		OnCrash: func(ev CrashEvent) {
			d.mu.Lock()
			handler := d.cfg.OnEvent
			d.mu.Unlock()
			if handler != nil {
				handler(EventCrashDetected, ev)
			}
		},
	})

	return d
}

// Start 启动守护
func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return nil
	}
	d.started = true
	d.stopCh = make(chan struct{})

	// 启动崩溃检测器
	if err := d.detector.Start(); err != nil {
		logger.Warn("崩溃检测器启动失败: %v", err)
	}

	// 初始进程扫描，记录初始状态
	d.scanProcesses()

	go d.loop()

	logger.Info("守护: 已启动 (轮询间隔 %v, %d 个监控进程, MC 目录 %s)",
		d.cfg.PollInterval, len(d.cfg.WatchedProcs), d.cfg.MinecraftDir)
	return nil
}

// Stop 停止守护
func (d *Daemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started {
		return
	}
	d.started = false
	close(d.stopCh)

	if d.detector != nil {
		d.detector.Stop()
	}

	d.fire(EventDaemonStopped, nil)
	logger.Info("守护: 已停止")
}

// PollNow 手动触发一轮检测
func (d *Daemon) PollNow() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.poll()
}

// ProcessStates 返回当前各进程的存活状态
func (d *Daemon) ProcessStates() map[string]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.scanProcesses()
	result := make(map[string]bool, len(d.knownProcesses))
	for k, v := range d.knownProcesses {
		result[k] = v
	}
	return result
}

// LogStage 返回当前加载进度
func (d *Daemon) LogStage() LogProgressStage {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.logStage
}

// ============================================================
// 主循环
// ============================================================

func (d *Daemon) loop() {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.mu.Lock()
			d.poll()
			d.mu.Unlock()
		}
	}
}

// poll 执行一轮检测
// 调用方需持有 d.mu 锁
func (d *Daemon) poll() {
	// 1. 检测进程存活性
	allExited := d.scanProcesses()

	// 2. 如果进程存活，检测最新日志（实时崩溃关键字 + 进度跟踪）
	if !allExited {
		d.checkLogFile()
	}

	// 3. 验证待处理的崩溃信号：只有该目录下真的生成了崩溃报告才触发回调
	d.verifyAndFireCrashes()

	// 4. 如果所有监控进程都退出了，触发退出事件
	if allExited && len(d.cfg.WatchedProcs) > 0 {
		logger.Info("守护: 所有监控进程已退出")
		d.mu.Unlock()
		d.fire(EventProcessExited, d.knownProcesses)
		d.mu.Lock()

		// 自动停止
		d.started = false
		close(d.stopCh)
	}
}

// ============================================================
// 进程检测
// ============================================================

// scanProcesses 扫描所有监控进程的存活状态
// 返回 true = 全部已退出
func (d *Daemon) scanProcesses() bool {
	allExited := true

	for i, proc := range d.cfg.WatchedProcs {
		alive := isProcessAlive(proc)
		key := procKey(proc)

		if alive {
			allExited = false
		}

		// 记录已知进程（即使 PID=0 也记录）
		prevAlive, known := d.knownProcesses[key]
		d.knownProcesses[key] = alive

		if proc.PID > 0 {
			if known && prevAlive != alive {
				if alive {
					logger.Info("守护: 进程 %s (PID=%d) 已启动", proc.Name, proc.PID)
					d.processStarted = true
					d.mu.Unlock()
					d.fire(EventMCStarted, proc)
					d.mu.Lock()
				} else {
					logger.Info("守护: 进程 %s (PID=%d) 已退出, ExitCode=%d", proc.Name, proc.PID, getProcessExitCode(proc))
				}
			} else if !known && alive {
				logger.Debug("守护: 发现进程 %s (PID=%d)", proc.Name, proc.PID)
				d.processStarted = true
			}
		}

		// 进程退出后清空 PID（避免二次触发退出事件）
		if known && !alive && proc.PID > 0 {
			d.cfg.WatchedProcs[i].PID = 0
		}

		_ = i
	}

	return allExited && len(d.cfg.WatchedProcs) > 0 && d.allKnownExited()
}

// allKnownExited 检查所有已知进程是否都已标记退出
func (d *Daemon) allKnownExited() bool {
	for _, alive := range d.knownProcesses {
		if alive {
			return false
		}
	}
	return true
}

// ============================================================
// 日志检测（借鉴 PCL2 ModWatcher）
// ============================================================

// crashKeywords 实时崩溃检测关键词（从 PCL2 GameLog 方法提取）
var crashKeywords = []string{
	"Crash report saved to",
	"This crash report has been saved to:",
	"Minecraft ran into a problem!",
	"Could not save crash report to",
	"An exception was thrown, the game will display an error screen and halt.",
	"Someone is closing me!",
	"Restarting Minecraft with command",
}

// verifyAndFireCrashes 验证待处理的崩溃信号
// 规则（鸽鸽要求）：不管用什么方式检测到崩溃，都必须确认该目录下有
// 新的崩溃日志文件才触发回调。如果这个目录没生成崩溃报告 → 不弹窗。
func (d *Daemon) verifyAndFireCrashes() {
	if len(d.pendingCrashSignals) == 0 {
		return
	}

	// 检查目录下是否有新的崩溃文件
	hasNewCrash := hasNewCrashReport(d.cfg.MinecraftDir, d.lastVerifiedCrash)

	// 不管有没有崩溃文件，超过 10s 的待处理信号都过期清除
	now := time.Now()
	var remaining []crashSignal
	for _, sig := range d.pendingCrashSignals {
		if now.Sub(sig.time) > 10*time.Second {
			continue // 过期
		}

		if hasNewCrash {
			// 确认是"我们的"崩溃 → 触发回调
			d.lastVerifiedCrash = now
			d.mu.Unlock()
			d.fire(EventCrashDetected, sig)
			d.mu.Lock()
			// 一个信号就够了，清空队列
			d.pendingCrashSignals = nil
			return
		}
		remaining = append(remaining, sig)
	}

	d.pendingCrashSignals = remaining
}

// hasNewCrashReport 检查指定的 .minecraft 目录下是否有新的崩溃报告
// after: 只检查在此时间之后生成的文件
func hasNewCrashReport(mcDir string, after time.Time) bool {
	// 1. 检查 crash-reports/*.txt
	crashDir := filepath.Join(mcDir, "crash-reports")
	if entries, err := os.ReadDir(crashDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(after) && strings.HasSuffix(e.Name(), ".txt") {
				return true
			}
		}
	}

	// 2. 检查 hs_err_pid*.log（JVM 崩溃日志在 .minecraft 根目录）
	matches, err := filepath.Glob(filepath.Join(mcDir, "hs_err_pid*.log"))
	if err == nil {
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			if info.ModTime().After(after) {
				return true
			}
		}
	}

	return false
}

// checkLogFile 检测 latest.log 新增内容
// 功能：
//   1. 实时崩溃关键字检测（借鉴 PCL2 GameLog 逻辑）
//   2. 加载进度跟踪（5 阶段，从 PCL2 ProgressUpdate 提取）
//   3. 异常关键词扫描（FATAL/OOM/NPE 等）
//
// 规则：所有崩溃/异常信号只做记录，实际触发回调前会经过 verifyCrash 确认
// 该目录下确有崩溃日志文件。避免日志中出现"Crash report saved to"但文件
// 实际上在其他整合包目录的情况。
func (d *Daemon) checkLogFile() {
	logPath := filepath.Join(d.cfg.MinecraftDir, "logs", "latest.log")
	f, err := os.Open(logPath)
	if err != nil {
		return // 日志还没生成
	}
	defer f.Close()

	// 定位到上次读取位置
	if d.logPos > 0 {
		_, err = f.Seek(d.logPos, 0)
		if err != nil {
			d.logPos = 0
			return
		}
	}

	// 读取新内容（最多 128KB 防止爆内存）
	buf := make([]byte, 131072)
	n, err := f.Read(buf)
	if err != nil && n <= 0 {
		return
	}
	d.logPos += int64(n)

	content := string(buf[:n])
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// 1. 实时崩溃检测（PCL2 风格：Crash report saved to → 立即标记崩溃）
		if d.detectCrashKeyword(trimmed) {
			logger.Warn("守护: 日志检测到潜在崩溃: %s", trimLine(trimmed, 80))
			d.signalCrash("log_keyword", trimmed)
			continue
		}

		// 2. 加载进度跟踪（PCL2 风格 5 阶段）
		d.updateLogStage(trimmed)

		// 3. 异常关键词扫描
		if d.isErrorKeyword(trimmed) {
			logger.Warn("守护: 日志检测到异常: %s", trimLine(trimmed, 80))
			d.signalCrash("log_error", trimmed)
		}
	}
}

// signalCrash 记录一次潜在的崩溃信号
// 不会直接触发回调，等待下一轮 poll 验证
func (d *Daemon) signalCrash(source string, detail string) {
	// 不重复记录相同的信号
	key := fmt.Sprintf("%s:%s", source, detail)
	for _, s := range d.pendingCrashSignals {
		if s.key == key {
			return
		}
	}

	d.pendingCrashSignals = append(d.pendingCrashSignals, crashSignal{
		key:    key,
		source: source,
		detail: detail,
		time:   time.Now(),
	})
}


// detectCrashKeyword 实时崩溃关键字检测
// 对应 PCL2 GameLog 中的明确崩溃标记
func (d *Daemon) detectCrashKeyword(line string) bool {
	for _, kw := range crashKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// updateLogStage 更新加载进度阶段
// 对应 PCL2 GameLog 中的 5 阶段进度跟踪
func (d *Daemon) updateLogStage(line string) {
	upper := strings.ToUpper(line)

	switch {
	case d.logStage < LogStageOutput:
		d.logStage = LogStageOutput
		fallthrough
	case d.logStage < LogStageUserSet && strings.Contains(line, "Setting user:"):
		d.logStage = LogStageUserSet
		fallthrough
	case d.logStage < LogStageLWJGL && (strings.Contains(upper, "LWJGL VERSION") || strings.Contains(upper, "LWJGL RELEASE")):
		d.logStage = LogStageLWJGL
		fallthrough
	case d.logStage < LogStageSound && (strings.Contains(line, "OpenAL initialized") || strings.Contains(line, "Starting up SoundSystem")):
		d.logStage = LogStageSound
		fallthrough
	case d.logStage < LogStageTextures && (strings.Contains(line, "Created") && strings.Contains(line, "textures") || strings.Contains(line, "Found animation info")):
		d.logStage = LogStageTextures
		d.windowShown = true
	}
}

// isErrorKeyword 检查是否为异常关键词
var errorKeywords = []string{
	"FATAL",
	"NullPointerException",
	"OutOfMemoryError",
	"StackOverflowError",
	"UnsupportedClassVersionError",
	"NoClassDefFoundError",
	"UnsatisfiedLinkError",
	"Exception in thread",
	"ClassNotFoundException",
}

func (d *Daemon) isErrorKeyword(line string) bool {
	upper := strings.ToUpper(line)
	for _, kw := range errorKeywords {
		if strings.Contains(upper, strings.ToUpper(kw)) {
			return true
		}
	}
	return false
}

// ============================================================
// 退出码崩溃辅助检测（借鉴 PCL2 TimerLog）
// ============================================================

// checkExitCodeCrash 检查进程是否因崩溃退出
// 逻辑（PCL2 风格）：
//   1. 进程退出码 != 0
//   2. 窗口已经出现过（避免加载阶段就崩了）
//   3. 进程确实曾经启动过
func (d *Daemon) checkExitCodeCrash(allExited bool) {
	if !allExited || !d.processStarted {
		return
	}

	// 退出码检测需要跨平台实现，目前只在有 /proc 的系统工作
	for key, alive := range d.knownProcesses {
		if alive {
			continue
		}
		// 解析 key 获取旧 PID
		parts := strings.SplitN(key, ":", 2)
		if len(parts) < 2 {
			continue
		}
		_ = parts[0] // name
		// 退出码在进程退出后不可获取（os.ProcessState.ExitCode() 需要 Wait）
		// 因此在 Linux 上我们在 scanProcesses 时已通过 /proc/{pid}/status 获取
		// 但 Wait 只能调一次，所以此处仅作日志
		logger.Debug("守护: 进程 %s 已退出，窗口状态=%v", key, d.windowShown)
		_ = d.windowShown
	}
}

// ============================================================
// 事件触发
// ============================================================

func (d *Daemon) fire(event DaemonEvent, data interface{}) {
	if d.cfg.OnEvent != nil {
		go d.cfg.OnEvent(event, data)
	}
}

// ============================================================
// 跨平台进程检测
// ============================================================

// isProcessAlive 检查进程是否存活
// Linux/macOS: 通过 /proc/{pid}/status
// Windows: 暂用 os.FindProcess + Signal(0)（Windows 不工作）
func isProcessAlive(proc WatchedProcess) bool {
	if proc.PID <= 0 {
		return false
	}

	// 方案 A: os.FindProcess
	p, err := os.FindProcess(proc.PID)
	if err != nil {
		return false
	}

	// 方案 B: Signal(0) — Unix only
	if err := p.Signal(os.Signal(nil)); err != nil {
		return false
	}

	// 方案 C: 验证 /proc/{pid}/status 进程名匹配（Unix 兜底）
	if statPath := fmt.Sprintf("/proc/%d/status", proc.PID); fileExists(statPath) {
		data, err := os.ReadFile(statPath)
		if err != nil {
			return false
		}
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "Name:") {
				actualName := strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
				if strings.EqualFold(actualName, proc.Name) {
					return true
				}
				return false // PID 被重用
			}
		}
	}

	return true
}

// getProcessExitCode 获取进程退出码（仅在 /proc 可用时）
func getProcessExitCode(proc WatchedProcess) int {
	statPath := fmt.Sprintf("/proc/%d/status", proc.PID)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return -1
	}
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "State:") {
			// State: Z (zombie) — 僵尸进程
			// State: X (dead) — 已死
			if strings.Contains(line, "Z") || strings.Contains(line, "X") {
				return -1
			}
		}
	}
	// 进程还活着就没有退出码
	return 0
}

// ============================================================
// 辅助函数
// ============================================================

// defaultWatchedProcs 返回默认监控进程列表
func defaultWatchedProcs() []WatchedProcess {
	return []WatchedProcess{
		{Name: "java"},  // MC 本体
		{Name: "javaw"}, // Windows 无控制台 Java
	}
}

func procKey(proc WatchedProcess) string {
	if proc.PID > 0 {
		return fmt.Sprintf("%s:%d", proc.Name, proc.PID)
	}
	return proc.Name
}

func trimLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
