package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================
// ServerUpdateInfo 解析测试
// ============================================================

func TestParseServerUpdateInfo(t *testing.T) {
	data := []byte(`{
		"update": {
			"mode": "incremental",
			"version": "v1.2.0",
			"from_version": "v1.1.0",
			"mc_version": "1.20.1",
			"base_url": "https://cdn.example.com/files",
			"incremental": {
				"added": [
					{ "path": "mods/sodium.jar", "hash": "aaaa", "size": 1024, "url": "" }
				],
				"updated": [
					{ "path": "config/sodium.properties", "hash": "bbbb", "size": 512, "url": "" }
				],
				"removed": ["mods/old.jar"]
			}
		}
	}`)

	var raw struct {
		Update *ServerUpdateInfo `json:"update"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	info := raw.Update
	if info == nil {
		t.Fatal("update 字段为空")
	}
	if info.Version != "v1.2.0" {
		t.Errorf("expected v1.2.0, got %s", info.Version)
	}
	if info.FromVersion != "v1.1.0" {
		t.Errorf("expected v1.1.0, got %s", info.FromVersion)
	}
	if info.BaseURL != "https://cdn.example.com/files" {
		t.Errorf("baseURL 错误: %s", info.BaseURL)
	}
	if info.Mode != "incremental" {
		t.Errorf("expected incremental, got %s", info.Mode)
	}

	incr := info.Incremental
	if incr == nil {
		t.Fatal("incremental 字段为空")
	}
	if len(incr.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(incr.Added))
	}
	if incr.Added[0].Path != "mods/sodium.jar" {
		t.Errorf("added path 错误: %s", incr.Added[0].Path)
	}
	if incr.Added[0].Hash != "aaaa" {
		t.Errorf("added hash 错误: %s", incr.Added[0].Hash)
	}
	if len(incr.Updated) != 1 {
		t.Fatalf("expected 1 updated, got %d", len(incr.Updated))
	}
	if len(incr.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(incr.Removed))
	}
	if incr.Removed[0] != "mods/old.jar" {
		t.Errorf("removed path 错误: %s", incr.Removed[0])
	}
}

func TestParseFullPackUpdateInfo(t *testing.T) {
	data := []byte(`{
		"update": {
			"mode": "full",
			"version": "v2.0.0",
			"mc_version": "1.21",
			"full_pack": {
				"url": "https://cdn.example.com/pack-v2.0.0.zip",
				"hash": "abcdef123456",
				"size": 52428800
			}
		}
	}`)

	var raw struct {
		Update *ServerUpdateInfo `json:"update"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	info := raw.Update
	if info.Mode != "full" {
		t.Errorf("expected full mode, got %s", info.Mode)
	}
	if info.FullPack.URL != "https://cdn.example.com/pack-v2.0.0.zip" {
		t.Errorf("full pack URL 错误: %s", info.FullPack.URL)
	}
	if info.FullPack.Size != 52428800 {
		t.Errorf("full pack size 错误: %d", info.FullPack.Size)
	}
}

// ============================================================
// UpdateResult.Summary 测试
// ============================================================

func TestUpdateResultSummary(t *testing.T) {
	tests := []struct {
		name   string
		result *UpdateResult
		want   string
	}{
		{
			name: "最新版本",
			result: &UpdateResult{
				Version:     "v1.2.0",
				FromVersion: "v1.2.0",
				Skipped:     -1,
			},
			want: "已是最新版本 ✓",
		},
		{
			name: "有变更",
			result: &UpdateResult{
				Version:       "v1.2.0",
				FromVersion:   "v1.1.0",
				Added:         2,
				Updated:       1,
				Deleted:       1,
				Downloaded:    3,
				DownloadBytes: 1048576,
			},
			want: "v1.1.0 → v1.2.0: +2, ~1, -1 | 下载 3 个文件, 1.0 MB 下载",
		},
		{
			name: "有缓存命中",
			result: &UpdateResult{
				Version:     "v1.2.0",
				FromVersion: "v1.1.0",
				Added:       2,
				CacheHits:   2,
				Downloaded:  0,
			},
			want: "v1.1.0 → v1.2.0: +2 | 下载 0 个文件 (缓存命中 2 个)",
		},
		{
			name: "全量更新",
			result: &UpdateResult{
				Version:       "v2.0.0",
				FromVersion:   "",
				Downloaded:    1,
				Added:         1,
			},
			want: " → v2.0.0: +1 | 下载 1 个文件",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Summary()
			if got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================================================
// CheckLocalVersion 测试
// ============================================================

func TestCheckLocalVersion(t *testing.T) {
	mcDir := t.TempDir()

	updater := NewUpdater("", mcDir)

	// 新建仓库时无快照
	ver := updater.CheckLocalVersion()
	if ver != "" {
		t.Errorf("新仓库应返回空版本，got %q", ver)
	}

	// 创建快照后再检查
	repo := NewLocalRepo(mcDir)
	repo.Init("1.20.1")
	os.MkdirAll(filepath.Join(mcDir, "mods"), 0755)
	os.WriteFile(filepath.Join(mcDir, "mods", "test.jar"), []byte("test"), 0644)
	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")

	// 新 updater 应能检测到
	updater2 := NewUpdater("", mcDir)
	ver2 := updater2.CheckLocalVersion()
	if ver2 == "" {
		t.Fatal("应检测到已有快照")
	}
	if ver2 != "v1.0" {
		t.Errorf("expected v1.0, got %s", ver2)
	}
}

// ============================================================
// ApplyUpdate dry-run 测试
// ============================================================

func TestApplyUpdateDryRun(t *testing.T) {
	cfgDir := t.TempDir()
	mcDir := t.TempDir()

	updater := NewUpdater(cfgDir, mcDir)
	updater.updateInfo = &ServerUpdateInfo{
		Mode:    "incremental",
		Version: "v1.2.0",
		BaseURL: "https://example.com/files",
		Incremental: &IncrementalInfo{
			Added: []FileChangeEntry{
				{Path: "mods/new.jar", Hash: "aaaa", Size: 1024},
			},
			Updated: []FileChangeEntry{
				{Path: "mods/updated.jar", Hash: "bbbb", Size: 2048},
			},
			Removed: []string{"mods/old.jar"},
		},
	}

	result, err := updater.ApplyUpdate(true)
	if err != nil {
		t.Fatalf("ApplyUpdate(dryRun) 失败: %v", err)
	}

	if result.Skipped <= 0 {
		t.Errorf("dry run 应该返回 skip 计数，got %d", result.Skipped)
	}
	if result.Version != "v1.2.0" {
		t.Errorf("expected v1.2.0, got %s", result.Version)
	}
	if result.FromVersion != "" {
		t.Errorf("expected empty fromVersion, got %s", result.FromVersion)
	}
}

// ============================================================
// BuildServerUpdateInfo 测试
// ============================================================

func TestBuildServerUpdateInfo(t *testing.T) {
	mcDir := t.TempDir()
	repo := NewLocalRepo(mcDir)
	repo.Init("1.20.1")

	// 创建初始快照
	os.MkdirAll(filepath.Join(mcDir, "mods"), 0755)
	os.WriteFile(filepath.Join(mcDir, "mods", "sodium.jar"), []byte("v1"), 0644)
	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")

	// 修改文件创建第二版
	os.WriteFile(filepath.Join(mcDir, "mods", "sodium.jar"), []byte("v2"), 0644)
	os.WriteFile(filepath.Join(mcDir, "mods", "iris.jar"), []byte("new"), 0644)
	repo.CreateFullSnapshot("v2.0", []string{"mods"}, "incremental_update", "v1.0")

	updater := NewUpdater("", mcDir)
	info, err := updater.BuildServerUpdateInfo("v2.0", "v1.0", "v2.0", "https://cdn.example.com/files")
	if err != nil {
		t.Fatalf("BuildServerUpdateInfo 失败: %v", err)
	}

	if info.Version != "v2.0" {
		t.Errorf("expected v2.0, got %s", info.Version)
	}
	if info.Mode != "incremental" {
		t.Errorf("expected incremental, got %s", info.Mode)
	}
	if info.BaseURL != "https://cdn.example.com/files" {
		t.Errorf("expected baseURL, got %s", info.BaseURL)
	}

	// 检查增量清单
	incr := info.Incremental
	if incr == nil {
		t.Fatal("incremental 为空")
	}

	// 应该有 1 个新增 (iris.jar) 和 1 个更新 (sodium.jar, hash 变了)
	addedPaths := make(map[string]bool)
	updatedPaths := make(map[string]bool)
	for _, f := range incr.Added {
		addedPaths[f.Path] = true
	}
	for _, f := range incr.Updated {
		updatedPaths[f.Path] = true
	}

	if !addedPaths["mods/iris.jar"] {
		t.Error("iris.jar 应在 added 中")
	}
	if !updatedPaths["mods/sodium.jar"] {
		t.Error("sodium.jar 应在 updated 中（hash 已变）")
	}
}

// ============================================================
// 已最新版本跳过测试
// ============================================================

func TestAlreadyUpToDate(t *testing.T) {
	cfgDir := t.TempDir()
	mcDir := t.TempDir()

	updater := NewUpdater(cfgDir, mcDir)
	updater.localVersion = "v1.0"
	updater.updateInfo = &ServerUpdateInfo{
		Mode:    "incremental",
		Version: "v1.0",
	}

	result, err := updater.ApplyUpdate(false)
	if err != nil {
		t.Fatalf("ApplyUpdate 失败: %v", err)
	}

	if result.Skipped != -1 {
		t.Errorf("Skipped 应为 -1 (已最新)，got %d", result.Skipped)
	}
}

// ============================================================
// 无增量信息回退测试
// ============================================================

func TestApplyUpdateNoIncremental(t *testing.T) {
	cfgDir := t.TempDir()
	mcDir := t.TempDir()

	updater := NewUpdater(cfgDir, mcDir)
	updater.updateInfo = &ServerUpdateInfo{
		Mode:    "full",
		Version: "v2.0",
		FullPack: &FullPackInfo{
			URL:  "https://example.com/pack.zip",
			Hash: "testhash",
			Size: 1000,
		},
	}

	// full mode + 无增量 → 应报错提示全量下载
	_, err := updater.ApplyUpdate(false)
	if err == nil {
		t.Fatal("应返回错误提示全量更新")
	}
	if !containsUpdateError(err, "全量更新") {
		t.Errorf("错误信息应提示全量，got: %v", err)
	}
}

func containsUpdateError(err error, substr string) bool {
	return len(err.Error()) >= len(substr) && func() bool {
		for i := 0; i <= len(err.Error())-len(substr); i++ {
			if err.Error()[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
