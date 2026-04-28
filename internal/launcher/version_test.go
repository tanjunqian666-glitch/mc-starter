package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sampleVersionJSON 用于测试的示例 version.json
var sampleVersionJSON = VersionMeta{
	ID:        "1.20.4",
	Type:      "release",
	MainClass: "net.minecraft.client.main.Main",
	Assets:    "13",
	AssetIndex: &AssetIndexRef{
		ID:        "13",
		Sha1:      "a32b31c2d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9",
		Size:      423456,
		TotalSize: 123456789,
		URL:       "https://example.com/13.json",
	},
	Downloads: &Downloads{
		Client: &DownloadEntry{
			Sha1: "abc123def456abc123def456abc123def456abc1",
			Size: 28512345,
			URL:  "https://example.com/client.jar",
		},
	},
	MinimumLauncherVer: 21,
	ReleaseTime:        "2023-12-07T11:09:23+00:00",
}

func TestParseVersionMeta(t *testing.T) {
	data, err := json.Marshal(sampleVersionJSON)
	if err != nil {
		t.Fatal(err)
	}

	meta, err := parseVersionMeta(data)
	if err != nil {
		t.Fatalf("parseVersionMeta 失败: %v", err)
	}

	if meta.ID != "1.20.4" {
		t.Errorf("ID = %q, want 1.20.4", meta.ID)
	}
	if meta.MainClass != "net.minecraft.client.main.Main" {
		t.Errorf("MainClass = %q", meta.MainClass)
	}
	if meta.Downloads.Client.Sha1 != "abc123def456abc123def456abc123def456abc1" {
		t.Errorf("Client SHA1 不匹配")
	}
}

func TestParseVersionMetaEmpty(t *testing.T) {
	_, err := parseVersionMeta([]byte("{}"))
	if err == nil {
		t.Error("空 JSON 应该报错")
	}
}

func TestParseVersionMetaInvalid(t *testing.T) {
	_, err := parseVersionMeta([]byte("not json"))
	if err == nil {
		t.Error("无效 JSON 应该报错")
	}
}

func TestVersionMetaCaching(t *testing.T) {
	dir := t.TempDir()
	mm := NewVersionManifestManager(filepath.Join(dir, "manifest"))
	vm := NewVersionMetaManager(filepath.Join(dir, "versions"), mm)

	cachePath := vm.cachePath("1.20.4")

	// 写入缓存
	data, _ := json.Marshal(sampleVersionJSON)
	vm.saveVersionMeta(cachePath, data)

	// 验证文件存在
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("缓存文件未创建")
	}

	// 读取缓存
	meta, err := vm.loadVersionMeta(cachePath)
	if err != nil {
		t.Fatalf("loadVersionMeta 失败: %v", err)
	}
	if meta.ID != "1.20.4" {
		t.Errorf("缓存加载后 ID = %q", meta.ID)
	}
}

func TestDownloadClientJarNoDownloads(t *testing.T) {
	dir := t.TempDir()
	mm := NewVersionManifestManager(filepath.Join(dir, "manifest"))
	vm := NewVersionMetaManager(filepath.Join(dir, "versions"), mm)

	meta := &VersionMeta{ID: "1.20.4"}
	_, err := vm.DownloadClientJar(meta, dir)
	if err == nil {
		t.Error("没有 Downloads 时应该报错")
	}
}

func TestVerifySHA1(t *testing.T) {
	// 创建测试文件
	f := filepath.Join(t.TempDir(), "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(f, content, 0644); err != nil {
		t.Fatal(err)
	}

	// SHA1 of "hello world"
	expected := "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed"

	ok, err := verifySHA1(f, expected)
	if err != nil {
		t.Fatalf("verifySHA1 失败: %v", err)
	}
	if !ok {
		t.Error("SHA1 应该匹配")
	}

	// 错误 hash
	ok, err = verifySHA1(f, "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("verifySHA1 失败: %v", err)
	}
	if ok {
		t.Error("错误 hash 应该不匹配")
	}

	// 空 hash（跳过校验）
	ok, err = verifySHA1(f, "")
	if err != nil {
		t.Fatalf("verifySHA1('') 失败: %v", err)
	}
	if !ok {
		t.Error("空 hash 应该跳过")
	}
}
