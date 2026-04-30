package launcher

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gege-tlph/mc-starter/internal/config"
	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// IncrementalUpdate 解析测试
// ============================================================

func TestParseIncrementalUpdate(t *testing.T) {
	data := []byte(`{
		"version": "v1.2.0",
		"from_version": "v1.1.0",
		"mode": "incremental",
		"added": [
			{ "path": "mods/sodium.jar", "hash": "aaaa", "size": 1024 }
		],
		"updated": [
			{ "path": "config/sodium.properties", "hash": "bbbb", "size": 512 }
		],
		"removed": ["mods/old.jar"]
	}`)

	var info model.IncrementalUpdate
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.Version != "v1.2.0" {
		t.Errorf("expected v1.2.0, got %s", info.Version)
	}
	if info.FromVersion != "v1.1.0" {
		t.Errorf("expected v1.1.0, got %s", info.FromVersion)
	}
	if info.Mode != "incremental" {
		t.Errorf("expected incremental, got %s", info.Mode)
	}
	if len(info.Added) != 1 {
		t.Fatalf("added 数量错误: %d", len(info.Added))
	}
	if info.Added[0].Path != "mods/sodium.jar" {
		t.Errorf("added path 错误: %s", info.Added[0].Path)
	}
	if info.Added[0].Hash != "aaaa" {
		t.Errorf("added hash 错误: %s", info.Added[0].Hash)
	}
	if len(info.Updated) != 1 {
		t.Fatalf("updated 数量错误: %d", len(info.Updated))
	}
	if info.Updated[0].Path != "config/sodium.properties" {
		t.Errorf("updated path 错误: %s", info.Updated[0].Path)
	}
	if len(info.Removed) != 1 {
		t.Fatalf("removed 数量错误: %d", len(info.Removed))
	}
	if info.Removed[0] != "mods/old.jar" {
		t.Errorf("removed 错误: %s", info.Removed[0])
	}
}

// ============================================================
// Updater 构造测试
// ============================================================

func TestNewUpdater(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "config")
	mcDir := filepath.Join(tmpDir, ".minecraft")

	u := NewUpdater(cfgDir, mcDir, config.New(cfgDir))
	if u == nil {
		t.Fatal("NewUpdater returned nil")
	}

	if u.CacheStore() == nil {
		t.Error("CacheStore() returned nil")
	}

	if u.LocalRepo() == nil {
		t.Error("LocalRepo() returned nil")
	}
}

// ============================================================
// UpdateResult Summary 测试
// ============================================================

func TestUpdateResultSummary(t *testing.T) {
	tests := []struct {
		result UpdateResult
		want   string
	}{
		{
			UpdateResult{PackName: "test", Version: "v1", FromVersion: "v0", Added: 2, Updated: 1, Deleted: 1, Skipped: 10, Downloaded: 3, DownloadBytes: 1048576},
			"[test] v0 → v1: +2, ~1, -1, =10 | 下载 3 个文件, 1.0 MB 下载",
		},
		{
			UpdateResult{PackName: "test", Version: "v1", FromVersion: "v0", Skipped: -1},
			"[test] 已是最新版本 ✓",
		},
		{
			UpdateResult{PackName: "test", Version: "v1", FromVersion: "v0", CacheHits: 5, Downloaded: 2, Added: 2},
			"[test] v0 → v1: +2 | 下载 2 个文件 (缓存命中 5 个)",
		},
	}

	for _, tt := range tests {
		got := tt.result.Summary()
		if got != tt.want {
			t.Errorf("Summary() = %q, want %q", got, tt.want)
		}
	}
}

// ============================================================
// HTTPGet 方法测试
// ============================================================

func TestManagerHTTPGet(t *testing.T) {
	mg := config.New("/tmp/test-config")
	if mg == nil {
		t.Fatal("config.New returned nil")
	}
}
