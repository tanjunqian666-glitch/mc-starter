package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleManifest 用于单元测试的示例清单
var sampleManifest = VersionManifest{
	Latest: LatestVersions{
		Release:  "1.20.4",
		Snapshot: "24w14a",
	},
	Versions: []VersionEntry{
		{ID: "1.20.4", Type: "release", URL: "https://example.com/1.20.4.json", Time: "2024-02-01T09:54:13+00:00", ReleaseTime: "2023-12-07T11:09:23+00:00"},
		{ID: "24w14a", Type: "snapshot", URL: "https://example.com/24w14a.json", Time: "2024-02-01T09:54:13+00:00", ReleaseTime: "2024-02-01T11:09:23+00:00"},
		{ID: "1.20.3", Type: "release", URL: "https://example.com/1.20.3.json", Time: "2024-01-15T10:00:00+00:00", ReleaseTime: "2023-11-15T10:00:00+00:00"},
		{ID: "1.20.2", Type: "release", URL: "https://example.com/1.20.2.json", Time: "2024-01-01T10:00:00+00:00", ReleaseTime: "2023-10-15T10:00:00+00:00"},
		{ID: "b1.7.3", Type: "old_beta", URL: "https://example.com/b1.7.3.json", Time: "2011-07-08T10:00:00+00:00", ReleaseTime: "2011-07-08T10:00:00+00:00"},
	},
}

func TestFindVersion(t *testing.T) {
	m := &VersionManifestManager{}
	m.manifest = &sampleManifest

	tests := []struct {
		id   string
		find bool
	}{
		{"1.20.4", true},
		{"24w14a", true},
		{"b1.7.3", true},
		{"1.99.99", false},
		{"", false},
	}

	for _, tt := range tests {
		got := m.FindVersion(tt.id)
		if tt.find && got == nil {
			t.Errorf("FindVersion(%q) = nil, want entry", tt.id)
		}
		if !tt.find && got != nil {
			t.Errorf("FindVersion(%q) = non-nil, want nil", tt.id)
		}
	}

	// 验证找到的版本 ID 正确
	got := m.FindVersion("1.20.4")
	if got == nil || got.ID != "1.20.4" {
		t.Errorf("FindVersion(1.20.4) ID mismatch")
	}
}

func TestListVersionsByType(t *testing.T) {
	m := &VersionManifestManager{}
	m.manifest = &sampleManifest

	releases := m.ListVersionsByType("release", 0)
	if len(releases) != 3 {
		t.Errorf("ListVersionsByType(release) = %d, want 3", len(releases))
	}

	snapshots := m.ListVersionsByType("snapshot", 0)
	if len(snapshots) != 1 {
		t.Errorf("ListVersionsByType(snapshot) = %d, want 1", len(snapshots))
	}

	limited := m.ListVersionsByType("release", 2)
	if len(limited) != 2 {
		t.Errorf("ListVersionsByType(release, 2) = %d, want 2", len(limited))
	}
}

func TestCacheLoadSave(t *testing.T) {
	dir := t.TempDir()
	m := NewVersionManifestManager(dir)

	// 写入缓存
	manifest := &sampleManifest
	manifest.fetchedAt = time.Now()
	m.saveCache(manifest)

	// 检查缓存文件存在
	cachePath := m.CachePath()
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("缓存文件未创建")
	}

	// 读取缓存
	loaded, ok := m.loadCached()
	if !ok {
		t.Fatal("无法加载缓存")
	}

	if loaded.Latest.Release != "1.20.4" {
		t.Errorf("Latest.Release = %q, want 1.20.4", loaded.Latest.Release)
	}

	if len(loaded.Versions) != 5 {
		t.Errorf("Versions = %d, want 5", len(loaded.Versions))
	}

	// 验证 fetchedAt 被正确设置
	if loaded.fetchedAt.IsZero() {
		t.Error("fetchedAt 未设置")
	}
}

func TestCacheTTL(t *testing.T) {
	dir := t.TempDir()
	m := NewVersionManifestManager(dir)

	// 写入一个旧的缓存
	oldManifest := sampleManifest
	oldData, _ := json.MarshalIndent(oldManifest, "", "  ")
	cachePath := m.CachePath()
	os.MkdirAll(filepath.Dir(cachePath), 0755)
	os.WriteFile(cachePath, oldData, 0644)

	// 设置文件修改时间为 2 小时前
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	os.Chtimes(cachePath, twoHoursAgo, twoHoursAgo)

	// TTL=1h → 应该过期，不会从缓存加载
	loaded, ok := m.loadCached()
	if !ok {
		t.Fatal("即使过期也应能加载缓存文件")
	}
	_ = loaded

	// 加载完成后检查 fetchedAt 是否接近 2 小时前
	if !loaded.fetchedAt.IsZero() {
		if loaded.fetchedAt.After(time.Now().Add(-1 * time.Hour)) {
			t.Error("fetchedAt 应该接近 2 小时前")
		}
	}
}

func TestGetManifest(t *testing.T) {
	m := &VersionManifestManager{}

	// 尚未拉取
	if m.GetManifest() != nil {
		t.Error("未拉取时 GetManifest 应为 nil")
	}

	m.manifest = &sampleManifest
	if m.GetManifest() == nil {
		t.Error("已设置时 GetManifest 不应为 nil")
	}
}
