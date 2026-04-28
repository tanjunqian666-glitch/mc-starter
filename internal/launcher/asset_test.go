package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sampleAssetIndex 示例 Asset 索引
var sampleAssetIndex = AssetIndex{
	Objects: map[string]AssetObject{
		"icons/icon_16x16.png":         {Hash: "bdf48ef6b5d0d23bbb02e17d04865216179f510a", Size: 3665},
		"minecraft/lang/zh_cn.json":    {Hash: "abc123def456abc123def456abc123def456abc1", Size: 123456},
		"minecraft/sounds/block/stone/hit1.ogg": {Hash: "def789abc123def789abc123def789abc123def7", Size: 5678},
		"minecraft/textures/gui/title/minecraft.png": {Hash: "1111112222223333334444445555556666667777", Size: 89123},
	},
}

func TestObjectURL(t *testing.T) {
	tests := []struct {
		hash string
		want string
	}{
		{"bdf48ef6b5d0d23bbb02e17d04865216179f510a", "https://resources.download.minecraft.net/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a"},
		{"abc", "https://resources.download.minecraft.net/ab/abc"},
		{"a", ""},  // 短于 2 字符
		{"", ""},   // 空字符串
	}

	for _, tt := range tests {
		got := ObjectURL(tt.hash)
		if got != tt.want {
			t.Errorf("ObjectURL(%q) = %q, want %q", tt.hash, got, tt.want)
		}
	}
}

func TestMirrorObjectURL(t *testing.T) {
	got := MirrorObjectURL("bdf48ef6b5d0d23bbb02e17d04865216179f510a")
	want := "https://bmclapi2.bangbang93.com/assets/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a"
	if got != want {
		t.Errorf("MirrorObjectURL = %q, want %q", got, want)
	}
}

func TestAssetObjectPath(t *testing.T) {
	dir := t.TempDir()
	m := NewAssetManager(dir, dir, nil, nil)

	path := m.AssetObjectPath("bdf48ef6b5d0d23bbb02e17d04865216179f510a")
	want := filepath.Join(dir, "objects", "bd", "bdf48ef6b5d0d23bbb02e17d04865216179f510a")
	if path != want {
		t.Errorf("AssetObjectPath = %q, want %q", path, want)
	}

	// 短 hash
	short := m.AssetObjectPath("a")
	if short != "" {
		t.Errorf("短 hash 应返回空字符串")
	}
}

func TestAssetIndexCaching(t *testing.T) {
	dir := t.TempDir()
	m := NewAssetManager(dir, dir, nil, nil)

	cachePath := m.cacheIndexPath("13")

	// 写入缓存
	data, _ := json.Marshal(sampleAssetIndex)
	m.saveCachedIndex(cachePath, data)

	// 检查文件存在
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("缓存文件未创建")
	}

	// 读取缓存
	idx, ok := m.loadCachedIndex(cachePath)
	if !ok {
		t.Fatal("无法加载缓存")
	}

	if len(idx.Objects) != 4 {
		t.Errorf("Objects = %d, want 4", len(idx.Objects))
	}

	if idx.Objects["icons/icon_16x16.png"].Hash != "bdf48ef6b5d0d23bbb02e17d04865216179f510a" {
		t.Errorf("Asset hash 不匹配")
	}
}

func TestListObjects(t *testing.T) {
	dir := t.TempDir()
	m := NewAssetManager(dir, dir, nil, nil)

	objects := m.ListObjects(&sampleAssetIndex)
	if len(objects) != 4 {
		t.Errorf("ListObjects = %d, want 4", len(objects))
	}

	// 验证字段
	found := false
	for _, obj := range objects {
		if obj.VirtualPath == "icons/icon_16x16.png" {
			found = true
			if obj.Hash != "bdf48ef6b5d0d23bbb02e17d04865216179f510a" {
				t.Errorf("Hash 不匹配")
			}
			if obj.Size != 3665 {
				t.Errorf("Size 不匹配")
			}
		}
	}
	if !found {
		t.Error("未找到 icons/icon_16x16.png")
	}
}

func TestStatistics(t *testing.T) {
	dir := t.TempDir()
	m := NewAssetManager(dir, dir, nil, nil)

	stats := m.Statistics(&sampleAssetIndex)
	if stats.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", stats.TotalFiles)
	}
	expectedSize := int64(3665 + 123456 + 5678 + 89123)
	if stats.TotalSize != expectedSize {
		t.Errorf("TotalSize = %d, want %d", stats.TotalSize, expectedSize)
	}
}

func TestIndexID(t *testing.T) {
	dir := t.TempDir()
	mm := NewVersionManifestManager(filepath.Join(dir, "manifest"))
	vm := NewVersionMetaManager(filepath.Join(dir, "versions"), mm)
	m := NewAssetManager(dir, dir, mm, vm)

	// 没有 version meta 时应该报错
	_, err := m.IndexID("nonexistent")
	if err == nil {
		t.Error("不存在的版本应该报错")
	}
}
