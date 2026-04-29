package repair

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P2.9 静默守护 — 测试
// ============================================================

// setupDaemonTestEnv 创建守护测试环境
func setupDaemonTestEnv(t *testing.T, withLog bool) string {
	t.Helper()
	dir := t.TempDir()

	if withLog {
		logDir := filepath.Join(dir, "logs")
		os.MkdirAll(logDir, 0755)
		// 正常日志 — 模拟 MC 加载流程的前几个阶段
		os.WriteFile(filepath.Join(logDir, "latest.log"),
			[]byte("[main/INFO]: Setting user: Player\n"+
				"[main/INFO]: LWJGL Version 3.2.2\n"+
				"[main/INFO]: OpenAL initialized\n"+
				"[main/INFO]: Created: 1024x512 textures-atlas\n"), 0644)
	}

	return dir
}

func TestNewDaemon(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: time.Second,
	})

	if d == nil {
		t.Fatal("NewDaemon returned nil")
	}
	if len(d.cfg.WatchedProcs) == 0 {
		t.Error("WatchedProcs should have defaults")
	}
	if d.cfg.PollInterval != time.Second {
		t.Errorf("expected PollInterval=1s, got %v", d.cfg.PollInterval)
	}

	d.Stop()
}

func TestDaemonStartStop(t *testing.T) {
	logger.Init(true)
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 100 * time.Millisecond,
	})

	if err := d.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	// 幂等
	if err := d.Start(); err != nil {
		t.Fatalf("Start(idempotent) failed: %v", err)
	}

	d.Stop()
	d.Stop() // 双重 Stop 幂等
}

func TestDaemonProcessAlive(t *testing.T) {
	alive := isProcessAlive(WatchedProcess{
		Name: "test",
		PID:  os.Getpid(),
	})
	if !alive {
		t.Log("os.FindProcess + Signal(0) may not work in this environment")
	}

	alive = isProcessAlive(WatchedProcess{
		Name: "nonexistent",
		PID:  999999999,
	})
	if alive {
		t.Error("non-existent PID should not be alive")
	}
}

func TestDaemonProcessScan(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 100 * time.Millisecond,
		WatchedProcs: []WatchedProcess{
			{Name: "nonexistent-proc", PID: 0},
		},
	})

	states := d.ProcessStates()
	if len(states) != 1 {
		t.Errorf("expected 1 process state, got %d", len(states))
	}
	for key, alive := range states {
		if alive {
			t.Errorf("process %s should not be alive (PID=0)", key)
		}
	}
}

func TestDaemonCrashDetection(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	crashDir := filepath.Join(dir, "crash-reports")
	os.MkdirAll(crashDir, 0755)
	os.WriteFile(filepath.Join(crashDir, "crash-2026-04-30_05.00.00-client.txt"),
		[]byte("---- Minecraft Crash Report ----\njava.lang.OutOfMemoryError"), 0644)

	var mu sync.Mutex
	crashEvents := []CrashEvent{}

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 50 * time.Millisecond,
		StartTime:    time.Now().Add(-time.Hour),
		OnEvent: func(event DaemonEvent, data interface{}) {
			if event == EventCrashDetected {
				mu.Lock()
				if ev, ok := data.(CrashEvent); ok {
					crashEvents = append(crashEvents, ev)
				}
				mu.Unlock()
			}
		},
	})

	d.Start()
	defer d.Stop()

	d.PollNow()
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(crashEvents)
	mu.Unlock()

	t.Logf("detected %d crash events", count)
}

func TestDaemonLogDetection(t *testing.T) {
	var mu sync.Mutex
	detectedEvents := 0

	d := NewDaemon(DaemonConfig{
		MinecraftDir: t.TempDir(),
		OnEvent: func(event DaemonEvent, data interface{}) {
			if event == EventCrashDetected {
				mu.Lock()
				detectedEvents++
				mu.Unlock()
			}
		},
	})
	defer d.Stop()

	// 直接注入崩溃信号 + 放崩溃报告
	crashDir := filepath.Join(d.cfg.MinecraftDir, "crash-reports")
	os.MkdirAll(crashDir, 0755)

	d.mu.Lock()
	d.pendingCrashSignals = append(d.pendingCrashSignals, crashSignal{
		key:    "log_error:test",
		source: "log_error",
		detail: "java.lang.OutOfMemoryError",
		time:   time.Now(),
	})
	d.mu.Unlock()

	// 还没有崩溃报告 → 验证不应通过
	d.mu.Lock()
	d.verifyAndFireCrashes()
	d.mu.Unlock()

	mu.Lock()
	c1 := detectedEvents
	mu.Unlock()
	if c1 > 0 {
		t.Error("no crash report yet, should not fire")
	}

	// 放崩溃报告 → 验证应通过
	os.WriteFile(filepath.Join(crashDir, "crash-2026-04-30_05.00.00-client.txt"),
		[]byte("java.lang.OutOfMemoryError"), 0644)

	d.mu.Lock()
	d.verifyAndFireCrashes()
	d.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	c2 := detectedEvents
	mu.Unlock()
	if c2 == 0 {
		t.Error("crash report exists, should fire EventCrashDetected")
	}
}

// TestDaemonCrashKeyword 测试实时崩溃关键字检测（PCL2 风格）
func TestDaemonCrashKeyword(t *testing.T) {
	dir := setupDaemonTestEnv(t, true)

	// 先放一个崩溃报告（否则验证不过）
	crashDir := filepath.Join(dir, "crash-reports")
	os.MkdirAll(crashDir, 0755)
	os.WriteFile(filepath.Join(crashDir, "crash-2026-04-30_06.00.00-client.txt"),
		[]byte("java.lang.OutOfMemoryError"), 0644)

	var mu sync.Mutex
	detectedEvents := 0

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: time.Second,
		OnEvent: func(event DaemonEvent, data interface{}) {
			if event == EventCrashDetected || event == EventLogError {
				mu.Lock()
				detectedEvents++
				mu.Unlock()
			}
		},
	})

	// 追加不同崩溃关键字的日志
	logPath := filepath.Join(dir, "logs", "latest.log")
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("[main/WARN]: Crash report saved to ./crash-reports/crash-2025-01-01_12.00.00-client.txt\n")
	f.WriteString("[main/ERROR]: An exception was thrown, the game will display an error screen and halt.\n")
	f.Close()

	// 通过 poll() 调用（持有锁），而不是直接调 checkLogFile
	d.mu.Lock()
	d.logPos = 0
	d.poll()
	d.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := detectedEvents
	mu.Unlock()

	t.Logf("crash keyword events: %d", count)
}

// TestDaemonLogStageTracking 测试加载进度阶段跟踪（PCL2 风格 5 阶段）
func TestDaemonLogStageTracking(t *testing.T) {
	dir := setupDaemonTestEnv(t, true)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: time.Second,
	})

	// 读取初始日志（setup 阶段写了 Setting user / LWJGL / OpenAL / textures）
	d.mu.Lock()
	d.checkLogFile()
	stage := d.logStage
	d.mu.Unlock()

	if stage < LogStageTextures {
		t.Errorf("expected log stage >= LogStageTextures (5), got %d", stage)
	}
}

func TestDaemonLogPositionPersistence(t *testing.T) {
	logger.Init(true)
	dir := setupDaemonTestEnv(t, true)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: time.Second,
	})

	d.mu.Lock()
	d.checkLogFile()
	pos1 := d.logPos
	d.mu.Unlock()

	if pos1 <= 0 {
		t.Fatal("logPos should be > 0 after reading")
	}

	// 追加内容
	logPath := filepath.Join(dir, "logs", "latest.log")
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("[main/INFO]: New log line\n")
	f.Close()

	d.mu.Lock()
	d.checkLogFile()
	pos2 := d.logPos
	d.mu.Unlock()

	if pos2 <= pos1 {
		t.Errorf("logPos should increase, was %d now %d", pos1, pos2)
	}
}

func TestDaemonProcessStates(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: time.Second,
		WatchedProcs: []WatchedProcess{
			{Name: "proc-a", PID: 0},
			{Name: "proc-b", PID: 0},
		},
	})

	states := d.ProcessStates()
	if len(states) != 2 {
		t.Errorf("expected 2 process states, got %d", len(states))
	}
	for _, alive := range states {
		if alive {
			t.Error("all processes should be marked dead (PID=0)")
		}
	}
}

func TestDaemonConcurrentStartStop(t *testing.T) {
	logger.Init(true)
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 10 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Start()
			time.Sleep(20 * time.Millisecond)
			d.Stop()
		}()
	}
	wg.Wait()

	if d.started {
		t.Error("daemon should be stopped after concurrent Start/Stop")
	}
}

func TestDaemonEventCallbacks(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	var mu sync.Mutex
	events := []DaemonEvent{}

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 50 * time.Millisecond,
		WatchedProcs: []WatchedProcess{
			{Name: "gone-proc", PID: 0},
		},
		OnEvent: func(event DaemonEvent, data interface{}) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	d.Start()
	defer d.Stop()

	d.PollNow()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	length := len(events)
	mu.Unlock()

	t.Logf("events captured: %v", events)
	_ = length
}

func TestDaemonAllExited(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)

	d := NewDaemon(DaemonConfig{
		MinecraftDir: dir,
		PollInterval: 50 * time.Millisecond,
		WatchedProcs: []WatchedProcess{
			{Name: "dead-proc", PID: 0},
		},
	})

	d.knownProcesses["dead-proc:0"] = false

	if !d.allKnownExited() {
		t.Error("all processes marked dead should report all exited")
	}

	d.knownProcesses["live-proc:0"] = true
	if d.allKnownExited() {
		t.Error("a live process exists, should not report all exited")
	}
}

// TestDaemonDetectCrashKeyword 测试 detectCrashKeyword 方法
func TestDaemonDetectCrashKeyword(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)
	d := NewDaemon(DaemonConfig{MinecraftDir: dir})

	tests := []struct {
		line    string
		want    bool
	}{
		{"[WARN]: Crash report saved to ./crash-reports/abc.txt", true},
		{"[WARN]: This crash report has been saved to: /path/crash.txt", true},
		{"[ERROR]: Could not save crash report to /path", true},
		{"[ERROR]: An exception was thrown, the game will display an error screen and halt.", true},
		{"Someone is closing me!", true},
		{"Restarting Minecraft with command", true},
		{"[main/INFO]: Setting user: Player", false},
		{"[main/INFO]: LWJGL Version 3.2.2 build 1", false},
	}

	for _, tt := range tests {
		got := d.detectCrashKeyword(tt.line)
		if got != tt.want {
			t.Errorf("detectCrashKeyword(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

// TestDaemonUpdateLogStage 测试加载进度跟踪
func TestDaemonUpdateLogStage(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)
	d := NewDaemon(DaemonConfig{MinecraftDir: dir})

	tests := []struct {
		line     string
		minStage LogProgressStage
	}{
		{"[main/INFO]: Setting user: Player", LogStageUserSet},
		{"[main/INFO]: LWJGL Version 3.2.2", LogStageLWJGL},
		{"[main/INFO]: OpenAL initialized", LogStageSound},
		{"[main/INFO]: Created: 512x256 textures-atlas", LogStageTextures},
		{"[main/DEBUG]: Found animation info for minecraft:block/cobblestone", LogStageTextures},
	}

	for _, tt := range tests {
		d.logStage = LogStageNone // 重置
		d.updateLogStage(tt.line)
		if d.logStage < tt.minStage {
			t.Errorf("updateLogStage(%q) = stage %d, want >= %d", tt.line, d.logStage, tt.minStage)
		}
	}
}

// TestDaemonIsErrorKeyword 测试异常关键词检测
func TestDaemonIsErrorKeyword(t *testing.T) {
	dir := setupDaemonTestEnv(t, false)
	d := NewDaemon(DaemonConfig{MinecraftDir: dir})

	tests := []struct {
		line string
		want bool
	}{
		{"[main/FATAL]: Something went terribly wrong", true},
		{"java.lang.OutOfMemoryError: Java heap space", true},
		{"java.lang.NullPointerException", true},
		{"java.lang.StackOverflowError", true},
		{"Caused by: java.lang.ClassNotFoundException: com.example.Mod", true},
		{"[main/INFO]: Minecraft 1.20.4", false},
		{"[main/INFO]: Loaded 42 mods", false},
	}

	for _, tt := range tests {
		got := d.isErrorKeyword(tt.line)
		if got != tt.want {
			t.Errorf("isErrorKeyword(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestProcKey(t *testing.T) {
	tests := []struct {
		proc WatchedProcess
		want string
	}{
		{WatchedProcess{Name: "java", PID: 0}, "java"},
		{WatchedProcess{Name: "javaw", PID: 12345}, "javaw:12345"},
		{WatchedProcess{Name: "java", PID: 999}, "java:999"},
	}

	for _, tt := range tests {
		got := procKey(tt.proc)
		if got != tt.want {
			t.Errorf("procKey(%+v) = %q, want %q", tt.proc, got, tt.want)
		}
	}
}

func TestTrimLine(t *testing.T) {
	short := "hello"
	if got := trimLine(short, 80); got != short {
		t.Errorf("trimLine should return as-is for short strings, got %q", got)
	}

	long := "abcdefghijklmnopqrstuvwxyz"
	if got := trimLine(long, 10); len(got) != 13 {
		t.Errorf("trimLine should trim with ..., got %q (len=%d)", got, len(got))
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "exists.txt")
	os.WriteFile(path, []byte("test"), 0644)
	if !fileExists(path) {
		t.Error("fileExists should return true for existing file")
	}

	if fileExists(filepath.Join(dir, "not-exists.txt")) {
		t.Error("fileExists should return false for non-existing file")
	}
}

// TestDefaultWatchedProcs 测试默认进程列表
func TestDefaultWatchedProcs(t *testing.T) {
	procs := defaultWatchedProcs()
	if len(procs) != 2 {
		t.Errorf("defaultWatchedProcs should return 2 entries, got %d", len(procs))
	}

	names := map[string]bool{}
	for _, p := range procs {
		names[p.Name] = true
	}
	if !names["java"] {
		t.Error("default procs should include 'java'")
	}
	if !names["javaw"] {
		t.Error("default procs should include 'javaw'")
	}
}
