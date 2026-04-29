package repair

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// setupDetectorTestMC 创建带崩溃文件的 .minecraft 结构
func setupDetectorTestMC(t *testing.T, withCrash bool) string {
	t.Helper()
	dir := t.TempDir()

	if withCrash {
		crashDir := filepath.Join(dir, "crash-reports")
		os.MkdirAll(crashDir, 0755)

		// 模拟崩溃报告
		crashContent := `---- Minecraft Crash Report ----
// Oops!

Time: 2026-04-29 15:30:00
Description: Unexpected error

java.lang.OutOfMemoryError: Java heap space
	at net.minecraft.client.Minecraft.run(Minecraft.java:1234)`

		os.WriteFile(filepath.Join(crashDir, "crash-2026-04-29_15.30.00-client.txt"),
			[]byte(crashContent), 0644)

		// 模拟 JVM hs_err
		hsErrContent := `#
# A fatal error has been detected by the Java Runtime Environment:
#
#  SIGSEGV (0xb) at pc=0x00007f1234567890, pid=12345, tid=67890
#`
		os.WriteFile(filepath.Join(dir, "hs_err_pid12345.log"),
			[]byte(hsErrContent), 0644)
	}

	return dir
}

func TestNewDetector(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(dir, nil)

	if d.mcDir != dir {
		t.Errorf("expected mcDir=%s, got %s", dir, d.mcDir)
	}
	if d.debounceDur != 2*time.Second {
		t.Errorf("expected default debounce 2s, got %v", d.debounceDur)
	}
}

func TestDetectorStartStop(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(dir, &DetectorOptions{PollOnly: true})

	if err := d.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !d.started {
		t.Error("expected started=true")
	}

	d.Stop()
	if d.started {
		t.Error("expected started=false after Stop")
	}
}

func TestDetectorStartTwice(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(dir, &DetectorOptions{PollOnly: true})

	if err := d.Start(); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	if err := d.Start(); err != nil {
		t.Fatalf("second Start should be noop: %v", err)
	}
	d.Stop()
}

func TestPollDetectNoCrash(t *testing.T) {
	dir := setupDetectorTestMC(t, false)
	events := PollDetect(dir)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestPollDetectWithCrash(t *testing.T) {
	dir := setupDetectorTestMC(t, true)
	events := PollDetect(dir)

	// 应该有 crash-report + hs_err 两个文件
	if len(events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(events))
	}

	// 验证类型
	types := make(map[string]bool)
	for _, ev := range events {
		types[ev.Type] = true
		if ev.FilePath == "" {
			t.Error("expected non-empty FilePath")
		}
		if ev.Reason == "" {
			t.Error("expected non-empty Reason")
		}
	}
	if !types["crash_report"] {
		t.Error("expected crash_report event")
	}
	if !types["hs_err"] {
		t.Error("expected hs_err event")
	}
}

func TestPollDetectIdempotent(t *testing.T) {
	dir := setupDetectorTestMC(t, true)

	// 第一次检测到 2 个事件
	events1 := PollDetect(dir)
	if len(events1) < 2 {
		t.Fatalf("expected >=2 events on first poll, got %d", len(events1))
	}

	// 第二次检测不应重复
	events2 := PollDetect(dir)
	if len(events2) != 0 {
		t.Errorf("expected 0 events on second poll (already seen), got %d", len(events2))
	}
}

func TestDetectorCrashCallback(t *testing.T) {
	dir := setupDetectorTestMC(t, true)

	var mu sync.Mutex
	received := make([]CrashEvent, 0)

	d := NewDetector(dir, &DetectorOptions{
		PollOnly: true,
		OnCrash: func(ev CrashEvent) {
			mu.Lock()
			received = append(received, ev)
			mu.Unlock()
		},
	})

	if err := d.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer d.Stop()

	// 手动触发一次检测
	d.PollNow()

	// 给回调一点时间
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected >=2 crash callbacks, got %d", count)
	}
}

func TestExtractCrashReason(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		content   string
		expected  string
	}{
		{
			name:      "oom",
			eventType: "crash_report",
			content:   "java.lang.OutOfMemoryError: Java heap space",
			expected:  "OutOfMemoryError",
		},
		{
			name:      "npe",
			eventType: "crash_report",
			content:   "java.lang.NullPointerException: Cannot invoke",
			expected:  "NullPointerException",
		},
		{
			name:      "hs_err",
			eventType: "hs_err",
			content:   "# SIGSEGV at pc=0x00007f...",
			expected:  "JVM 致命错误，请检查 hs_err_pid*.log",
		},
		{
			name:      "empty",
			eventType: "crash_report",
			content:   "",
			expected:  "未知原因",
		},
		{
			name:      "exception_in_thread",
			eventType: "crash_report",
			content:   "Exception in thread \"Render thread\"\njava.lang.RuntimeException: Something went wrong",
			expected:  "Exception in thread",
		},
		{
			name:      "first_line",
			eventType: "crash_report",
			content:   "This is exactly sixty chars long content for test purpose",
			expected:  "This is exactly sixty chars long content for test purpose", // 57 chars, no truncation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 写入临时文件
			dir := t.TempDir()
			path := filepath.Join(dir, "crash.txt")
			os.WriteFile(path, []byte(tt.content), 0644)

			reason := extractCrashReason(path, tt.eventType)
			if reason != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, reason)
			}
		})
	}
}

func TestIsCrashFileName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"crash-2026-04-29_15.30.00-client.txt", true},
		{"crash-2026-04-29_15.30.00-server.txt", true},
		{"hs_err_pid12345.log", true},
		{"hs_err_pid12345.log.old", false},
		{"hs_err_pid.log", true},
		{"options.txt", false},
		{"sodium.jar", false},
		{"level.dat", false},
		{"README.txt", false},
		{"crash-report.txt", true},  // 虽然不标准但也接受
	}
	for _, tt := range tests {
		got := isCrashFileName(tt.name)
		if got != tt.want {
			t.Errorf("isCrashFileName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsRecentlyModified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("test"), 0644)

	if !isRecentlyModified(path, 1*time.Hour) {
		t.Error("expected recently modified (just created)")
	}
}

func TestDetectorResetState(t *testing.T) {
	dir := setupDetectorTestMC(t, true)

	d := NewDetector(dir, nil)
	d.PollNow() // 消耗初始事件

	if d.LastCrashTime().IsZero() {
		t.Error("expected non-zero LastCrashTime after detection")
	}

	d.ResetState()

	if len(d.state.SeenFiles) != 0 {
		t.Errorf("expected empty SeenFiles after reset, got %d", len(d.state.SeenFiles))
	}
}

func TestDetectorStatePersistence(t *testing.T) {
	dir := setupDetectorTestMC(t, true)

	// 创建检测器并检测
	d1 := NewDetector(dir, &DetectorOptions{PollOnly: true})
	d1.PollNow()

	lastCrash := d1.LastCrashTime()
	lastID := d1.LastCrashID()

	// 创建新检测器（模拟重启），应加载持久化状态
	d2 := NewDetector(dir, &DetectorOptions{PollOnly: true})

	if d2.LastCrashTime().IsZero() {
		t.Error("expected non-zero LastCrashTime after loading state")
	}
	if d2.LastCrashID() == "" {
		t.Error("expected non-empty LastCrashID after loading state")
	}

	_ = lastCrash
	_ = lastID
}

func TestDetectorOnlyNewFiles(t *testing.T) {
	dir := setupDetectorTestMC(t, true)

	events := PollDetect(dir)
	if len(events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(events))
	}

	// 加一个新崩溃文件
	crashDir := filepath.Join(dir, "crash-reports")
	os.WriteFile(filepath.Join(crashDir, "crash-2026-04-29_16.00.00-client.txt"),
		[]byte("java.lang.NullPointerException"), 0644)

	// 应该只检测到新文件
	events2 := PollDetect(dir)
	if len(events2) != 1 {
		t.Errorf("expected 1 new event, got %d", len(events2))
	}
	if len(events2) > 0 && !strings.Contains(events2[0].FilePath, "16.00.00") {
		t.Error("expected the new file to be detected")
	}
}
