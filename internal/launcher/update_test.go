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

// ============================================================
// P6 频道 JSON 解析测试
// ============================================================

func TestParseIncrementalUpdateWithChannels(t *testing.T) {
	data := []byte(`{
		"version": "v1.2.0",
		"from_version": "v1.1.0",
		"mode": "incremental",
		"channels": {
			"all": {"version": "v1.2.0", "changed": true},
			"mods-core": {"version": "v1.2.0", "changed": true},
			"shaderpacks": {"version": "v1.0.0", "changed": false}
		},
		"added": [
			{ "path": "mods/sodium.jar", "hash": "aaaa", "size": 1024, "channel": "mods-core" }
		],
		"updated": [
			{ "path": "config/sodium.properties", "hash": "bbbb", "size": 512, "channel": "mods-core" }
		],
		"removed": ["mods/old.jar"]
	}`)

	var info model.IncrementalUpdate
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// Verify challenge channels field
	if info.Channels == nil {
		t.Fatal("channels 字段为空")
	}
	if info.Channels["all"].Version != "v1.2.0" {
		t.Errorf("all channel version = %s, want v1.2.0", info.Channels["all"].Version)
	}
	if !info.Channels["mods-core"].Changed {
		t.Error("mods-core 应标记为 changed")
	}
	if info.Channels["shaderpacks"].Changed {
		t.Error("shaderpacks 不应标记为 changed")
	}

	// Verify file entry has channel field
	if len(info.Added) != 1 {
		t.Fatalf("added 数量错误: %d", len(info.Added))
	}
	if info.Added[0].Channel != "mods-core" {
		t.Errorf("added channel 错误: %s, want mods-core", info.Added[0].Channel)
	}
	if info.Updated[0].Channel != "mods-core" {
		t.Errorf("updated channel 错误: %s, want mods-core", info.Updated[0].Channel)
	}
}

func TestChannelInfoJSON(t *testing.T) {
	data := []byte(`{
		"name": "shaderpacks",
		"display_name": "光影包",
		"description": "高性能需求光影",
		"required": false,
		"version": "v1.0.0",
		"file_count": 2,
		"total_size": 52428800
	}`)

	var info model.ChannelInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if info.Name != "shaderpacks" {
		t.Errorf("name = %s", info.Name)
	}
	if info.DisplayName != "光影包" {
		t.Errorf("display_name = %s", info.DisplayName)
	}
	if info.Required {
		t.Error("required 应为 false")
	}
	if info.FileCount != 2 {
		t.Errorf("file_count = %d", info.FileCount)
	}
	if info.TotalSize != 52428800 {
		t.Errorf("total_size = %d", info.TotalSize)
	}
}

func TestPackInfoWithChannelsJSON(t *testing.T) {
	data := []byte(`{
		"name": "main-pack",
		"display_name": "主服整合包",
		"primary": true,
		"latest_version": "v1.2.0",
		"channels": [
			{
				"name": "mods-core",
				"display_name": "核心模组",
				"required": true,
				"version": "v1.2.0",
				"file_count": 12,
				"total_size": 8388608
			},
			{
				"name": "shaderpacks",
				"display_name": "光影包",
				"required": false,
				"version": "v1.0.0"
			}
		]
	}`)

	var info model.PackInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(info.Channels) != 2 {
		t.Fatalf("channels 数量 = %d, want 2", len(info.Channels))
	}
	if info.Channels[0].Name != "mods-core" {
		t.Errorf("第一个频道名 = %s", info.Channels[0].Name)
	}
	if info.Channels[1].Name != "shaderpacks" {
		t.Errorf("第二个频道名 = %s", info.Channels[1].Name)
	}
	if info.Channels[1].Required {
		t.Errorf("shaderpacks.Required 应为 false")
	}
}

func TestChannelsResponseJSON(t *testing.T) {
	data := []byte(`{
		"channels": [
			{
				"name": "mods-core",
				"display_name": "核心模组",
				"required": true,
				"version": "v1.2.0"
			}
		]
	}`)

	var resp model.ChannelsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(resp.Channels) != 1 {
		t.Fatalf("channels 数量 = %d", len(resp.Channels))
	}
	if resp.Channels[0].Name != "mods-core" {
		t.Errorf("name = %s", resp.Channels[0].Name)
	}
}

func TestPackStateWithChannels(t *testing.T) {
	data := []byte(`{
		"enabled": true,
		"status": "synced",
		"local_version": "v1.2.0",
		"dir": "packs/main-pack",
		"channels": {
			"mods-core": {
				"enabled": true,
				"version": "v1.2.0"
			},
			"shaderpacks": {
				"enabled": false,
				"version": ""
			}
		}
	}`)

	var state model.PackState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if state.Channels == nil {
		t.Fatal("channels 字段为空")
	}
	if !state.Channels["mods-core"].Enabled {
		t.Error("mods-core 应已启用")
	}
	if state.Channels["mods-core"].Version != "v1.2.0" {
		t.Errorf("mods-core 版本 = %s", state.Channels["mods-core"].Version)
	}
	if state.Channels["shaderpacks"].Enabled {
		t.Error("shaderpacks 不应启用")
	}
	if state.Channels["shaderpacks"].Version != "" {
		t.Errorf("shaderpacks 版本 = %s", state.Channels["shaderpacks"].Version)
	}
}
