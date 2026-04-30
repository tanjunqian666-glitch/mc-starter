package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================
// semver 比较测试
// ============================================================

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"1.20.4", "1.20.3", 1},
		{"1.20", "1.19.9", 1},
		{"1.20", "1.20.0", 0},
		{"1.2.3-beta", "1.2.3", 0},        // 忽略预发布标签
		{"1.2.3", "1.2.3-alpha.1", 0},     // 忽略预发布标签
		{"1.0.0", "1.0.0+sha.abc", 0},     // 忽略 build metadata
		{"1.0.0", "1.0.0.1", -1},          // 额外段
		{"1.0.0.1", "1.0.0", 1},           // 额外段
	}

	for _, tt := range tests {
		got := compareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
		// 对称性测试
		rev := compareVersions(tt.v2, tt.v1)
		if rev != -tt.want {
			t.Errorf("compareVersions(%q, %q) 对称性 = %d, want %d", tt.v2, tt.v1, rev, -tt.want)
		}
	}
}

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		v    string
		want []int
	}{
		{"1.2.3", []int{1, 2, 3}},
		{"1.20", []int{1, 20}},
		{"0", []int{0}},
		{"0.0.1", []int{0, 0, 1}},
	}

	for _, tt := range tests {
		got := parseVersionParts(tt.v)
		if len(got) != len(tt.want) {
			t.Errorf("parseVersionParts(%q) = %v, want %v", tt.v, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseVersionParts(%q) = %v, want %v", tt.v, got, tt.want)
				break
			}
		}
	}
}

// ============================================================
// 状态文件读写测试
// ============================================================

func TestUpdateStateSaveLoad(t *testing.T) {
	dir := t.TempDir()
	su := NewSelfUpdater(dir, "1.0.0", "http://example.com")

	state, err := su.LoadState()
	if err != nil {
		t.Fatalf("初始加载状态失败: %v", err)
	}
	if state.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", state.CurrentVersion, "1.0.0")
	}

	// 保存状态
	state.LastCheckTime = "2026-05-01T00:00:00+08:00"
	state.PendingUpdate = &PendingUpdate{
		Version:  "1.1.0",
		Downloaded: true,
	}
	if err := su.SaveState(state); err != nil {
		t.Fatalf("保存状态失败: %v", err)
	}

	// 重新加载验证
	loaded, err := su.LoadState()
	if err != nil {
		t.Fatalf("重新加载状态失败: %v", err)
	}
	if loaded.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", loaded.CurrentVersion, "1.0.0")
	}
	if loaded.LastCheckTime != "2026-05-01T00:00:00+08:00" {
		t.Errorf("LastCheckTime = %q", loaded.LastCheckTime)
	}
	if loaded.PendingUpdate == nil || loaded.PendingUpdate.Version != "1.1.0" {
		t.Errorf("PendingUpdate = %+v", loaded.PendingUpdate)
	}
}

func TestUpdateStateAutoInit(t *testing.T) {
	dir := t.TempDir()
	su := NewSelfUpdater(dir, "dev", "http://example.com")

	state, err := su.LoadState()
	if err != nil {
		t.Fatalf("加载新状态失败: %v", err)
	}
	if state.CurrentVersion != "dev" {
		t.Errorf("CurrentVersion = %q, want %q", state.CurrentVersion, "dev")
	}
	if len(state.UpdateHistory) != 0 {
		t.Errorf("UpdateHistory 应初始为空, got %d", len(state.UpdateHistory))
	}
}

func TestUpdateStateAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, UpdateStateFileName)
	su := NewSelfUpdater(dir, "1.0.0", "http://example.com")

	// 保存状态
	state := &UpdateState{CurrentVersion: "1.0.0"}
	if err := su.SaveState(state); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 确保没有 .tmp 残留
	if _, err := os.Stat(statePath + ".tmp"); !os.IsNotExist(err) {
		t.Error("临时文件未清理")
	}

	// 确保文件可读
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("读取状态文件失败: %v", err)
	}
	var loaded UpdateState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
}

// ============================================================
// ValidateChannelSwitch 测试
// ============================================================

func TestValidateChannelSwitch(t *testing.T) {
	tests := []struct {
		from, to string
		wantErr  bool
	}{
		{"stable", "stable", false},
		{"stable", "beta", false},
		{"stable", "dev", false},
		{"beta", "stable", false},
		{"beta", "beta", false},
		{"beta", "dev", false},
		{"dev", "dev", false},
		{"dev", "beta", false},
		{"dev", "stable", true}, // dev → stable 不允许
		{"stable", "invalid", true},
	}

	for _, tt := range tests {
		err := ValidateChannelSwitch(tt.from, tt.to)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateChannelSwitch(%q, %q) 错误 = %v, wantErr=%v", tt.from, tt.to, err, tt.wantErr)
		}
	}
}

// ============================================================
// 通道切换测试
// ============================================================

func TestSetChannelStr(t *testing.T) {
	dir := t.TempDir()
	su := NewSelfUpdater(dir, "1.0.0", "http://example.com")

	if su.Channel != ChannelStable {
		t.Errorf("默认通道 = %s, want stable", su.Channel)
	}

	if err := su.SetChannelStr("beta"); err != nil {
		t.Fatalf("切换到 beta 失败: %v", err)
	}
	if su.Channel != ChannelBeta {
		t.Errorf("通道 = %s, want beta", su.Channel)
	}

	if err := su.SetChannelStr("dev"); err != nil {
		t.Fatalf("切换到 dev 失败: %v", err)
	}
	if su.Channel != ChannelDev {
		t.Errorf("通道 = %s, want dev", su.Channel)
	}

	// 检查状态文件
	state, err := su.LoadState()
	if err != nil {
		t.Fatalf("加载状态失败: %v", err)
	}
	if state.UpdateChannel != "dev" {
		t.Errorf("状态文件中的通道 = %s, want dev", state.UpdateChannel)
	}

	// 无效通道
	if err := su.SetChannelStr("invalid"); err == nil {
		t.Error("设置无效通道应该返回错误")
	}
}

// ============================================================
// 更新历史测试
// ============================================================

func TestGetUpdateHistory(t *testing.T) {
	dir := t.TempDir()
	su := NewSelfUpdater(dir, "1.0.0", "http://example.com")

	// 空历史
	entries, err := su.GetUpdateHistory()
	if err != nil {
		t.Fatalf("获取空历史失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("空历史应返回 0 条, got %d", len(entries))
	}

	// 添加历史
	state, _ := su.LoadState()
	state.UpdateHistory = []UpdateHistoryEntry{
		{From: "1.0.0", To: "1.1.0", Time: "2026-01-01T00:00:00Z", Success: true},
		{From: "1.1.0", To: "1.2.0", Time: "2026-02-01T00:00:00Z", Success: true},
	}
	su.SaveState(state)

	entries, err = su.GetUpdateHistory()
	if err != nil {
		t.Fatalf("获取历史失败: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("期望 2 条历史, got %d", len(entries))
	}
	if entries[0].From != "1.0.0" || entries[0].To != "1.1.0" {
		t.Errorf("第一条历史 = %+v", entries[0])
	}
}

// ============================================================
// FormatUpdateHistory 测试
// ============================================================

func TestFormatUpdateHistory(t *testing.T) {
	entries := []UpdateHistoryEntry{
		{From: "1.0.0", To: "1.1.0", Time: "2026-01-01T00:00:00Z", Success: true},
	}
	output := FormatUpdateHistory(entries)
	if len(output) == 0 {
		t.Error("格式化输出不应为空")
	}
	if len(output) < 10 {
		t.Errorf("输出太短: %q", output)
	}

	empty := FormatUpdateHistory(nil)
	if len(empty) == 0 {
		t.Error("空历史格式化不应为空")
	}
}

// ============================================================
// filterUpdateArgs 测试
// ============================================================

func TestFilterUpdateArgs(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{nil, nil},
		{[]string{"update"}, []string{"update"}},
		{[]string{"self-update", "apply"}, nil},
		{[]string{"update", "--pack", "main"}, []string{"update", "--pack", "main"}},
	}

	for _, tt := range tests {
		got := filterUpdateArgs(tt.args)
		if len(got) != len(tt.want) {
			t.Errorf("filterUpdateArgs(%v) = %v, want %v", tt.args, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("filterUpdateArgs(%v) = %v, want %v", tt.args, got, tt.want)
				break
			}
		}
	}
}

// ============================================================
// verifySHA256File 测试
// ============================================================

func TestVerifySHA256File(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.bin")

	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	// 计算正确的 hash
	correctHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	verified, err := verifySHA256File(testFile, correctHash)
	if err != nil {
		t.Fatalf("校验失败: %v", err)
	}
	if !verified {
		t.Error("正确 hash 应返回 true")
	}

	// 错误 hash
	verified, err = verifySHA256File(testFile, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("校验失败: %v", err)
	}
	if verified {
		t.Error("错误 hash 应返回 false")
	}

	// 不存在的文件
	_, err = verifySHA256File(filepath.Join(dir, "nonexistent"), correctHash)
	if err == nil {
		t.Error("不存在的文件应返回错误")
	}
}
