package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gege-tlph/mc-starter/internal/model"
)

// createTempIncrSync 创建临时目录和 IncrementalSync 用于测试
func createTempIncrSync(t *testing.T) (*IncrementalSync, string, string) {
	t.Helper()

	// 创建临时 cfgDir 和 mcDir
	baseDir, err := os.MkdirTemp("", "mc-incr-sync-*")
	if err != nil {
		t.Fatalf("创建基础目录失败: %v", err)
	}

	cfgDir := filepath.Join(baseDir, "config")
	mcDir := filepath.Join(baseDir, ".minecraft")

	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("创建 cfgDir 失败: %v", err)
	}
	if err := os.MkdirAll(mcDir, 0755); err != nil {
		t.Fatalf("创建 mcDir 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mcDir, "mods"), 0755); err != nil {
		t.Fatalf("创建 mods 目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mcDir, "config"), 0755); err != nil {
		t.Fatalf("创建 config 目录失败: %v", err)
	}

	is := NewIncrementalSync(cfgDir, mcDir)

	return is, cfgDir, mcDir
}

// TestNewIncrementalSync 测试创建增量同步管理器
func TestNewIncrementalSync(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	if is == nil {
		t.Fatal("IncrementalSync 不应为 nil")
	}

	if is.CacheStore() == nil {
		t.Error("CacheStore 不应为 nil")
	}

	if is.LocalRepo() == nil {
		t.Error("LocalRepo 不应为 nil")
	}
}

// TestCacheVersionMetaData 测试 version meta 缓存
func TestCacheVersionMetaData(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	// 缓存 version meta
	data := []byte(`{"id": "1.20.4", "type": "release"}`)
	is.CacheVersionMetaData("1.20.4", data)

	// 检查缓存是否存在
	key := "version_meta:1.20.4"
	if !is.CacheStore().Has(key) {
		t.Error("version meta 应已缓存")
	}

	path, err := is.CacheStore().Get(key)
	if err != nil {
		t.Fatalf("获取缓存失败: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取缓存文件失败: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("内容不匹配: 期望 %s, 得到 %s", string(data), string(got))
	}
}

// TestTryCacheVersionMeta 测试获取缓存 version meta
func TestTryCacheVersionMeta(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	// 未缓存时应返回空
	path := is.TryCacheVersionMeta("nonexistent")
	if path != "" {
		t.Error("未缓存的版本应返回空串")
	}

	// 缓存后应命中
	is.CacheVersionMetaData("1.20.4", []byte(`{"id": "1.20.4"}`))
	path = is.TryCacheVersionMeta("1.20.4")
	if path == "" {
		t.Error("缓存后应命中")
	}
}

// TestTryCacheAsset 测试 Asset 缓存
func TestTryCacheAsset(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	hash := "abc123def4567890abcdef1234567890abcdef12"

	// 未缓存时应返回空
	path := is.TryCacheAsset(hash)
	if path != "" {
		t.Error("未缓存的 Asset 应返回空串")
	}

	// 放一个进去
	tmpFile := filepath.Join(cfgDir, "test_asset.dat")
	if err := os.WriteFile(tmpFile, []byte("asset data"), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	is.StoreAsset(hash, tmpFile)

	path = is.TryCacheAsset(hash)
	if path == "" {
		t.Error("缓存后应命中")
	}
}

// TestTryCacheLibrary 测试 Library 缓存
func TestTryCacheLibrary(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	sha1 := "abcdef1234567890abcdef1234567890abcdef12"

	// 未缓存时应返回空
	path := is.TryCacheLibrary(sha1)
	if path != "" {
		t.Error("未缓存的 Library 应返回空串")
	}

	// 空 SHA1 也应返回空
	path = is.TryCacheLibrary("")
	if path != "" {
		t.Error("空 SHA1 应返回空串")
	}

	// 放一个进去
	tmpFile := filepath.Join(cfgDir, "test_lib.jar")
	if err := os.WriteFile(tmpFile, []byte("library data"), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	is.StoreLibrary(sha1, tmpFile)

	path = is.TryCacheLibrary(sha1)
	if path == "" {
		t.Error("缓存后应命中")
	}
}

// TestAssetFromCache 测试从缓存复制 Asset
func TestAssetFromCache(t *testing.T) {
	is, cfgDir, mcDir := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	hash := "asset0011234567890abcdef1234567890abcdef"
	content := "asset file content"

	// 先缓存一个 Asset
	tmpFile := filepath.Join(cfgDir, "test_asset_src.dat")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	is.StoreAsset(hash, tmpFile)

	// 从缓存复制到 objects 目录
	destPath := filepath.Join(mcDir, "assets", "objects", hash[:2], hash)
	ok := is.AssetFromCache(hash, destPath)
	if !ok {
		t.Fatal("AssetFromCache 应成功")
	}

	// 验证内容
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(got) != content {
		t.Errorf("内容不匹配: 期望 %s, 得到 %s", content, string(got))
	}
}

// TestEnsureRepoAndCreateSnapshot 测试 repo 初始化和快照创建
func TestEnsureRepoAndCreateSnapshot(t *testing.T) {
	is, cfgDir, mcDir := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	// 初始化 repo
	if err := is.EnsureRepo("1.20.4"); err != nil {
		t.Fatalf("EnsureRepo 失败: %v", err)
	}

	// 在 mods 下创建一些文件
	modFile := filepath.Join(mcDir, "mods", "sodium.jar")
	if err := os.WriteFile(modFile, []byte("mod content"), 0644); err != nil {
		t.Fatalf("创建 mod 文件失败: %v", err)
	}
	configFile := filepath.Join(mcDir, "config", "options.txt")
	if err := os.WriteFile(configFile, []byte("options"), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	// 创建快照
	if err := is.CreateSyncSnapshot("v1.0", []string{"mods", "config"}); err != nil {
		t.Fatalf("CreateSyncSnapshot 失败: %v", err)
	}

	// 验证快照存在
	if !is.LocalRepo().HasSnapshot("v1.0") {
		t.Error("快照 v1.0 应存在")
	}

	// 验证 latest 指向正确
	if is.LocalRepo().LatestSnapshot() != "v1.0" {
		t.Errorf("LatestSnapshot 应为 v1.0, 得到 %s", is.LocalRepo().LatestSnapshot())
	}
}

// TestDiffSinceSnapshot 测试快照间差异计算
func TestDiffSinceSnapshot(t *testing.T) {
	is, cfgDir, mcDir := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	if err := is.EnsureRepo("1.20.4"); err != nil {
		t.Fatalf("EnsureRepo 失败: %v", err)
	}

	// 创建初始 mod 文件
	modFile := filepath.Join(mcDir, "mods", "sodium.jar")
	if err := os.WriteFile(modFile, []byte("v1 content"), 0644); err != nil {
		t.Fatalf("创建 mod 文件失败: %v", err)
	}
	configFile := filepath.Join(mcDir, "config", "options.txt")
	if err := os.WriteFile(configFile, []byte("options v1"), 0644); err != nil {
		t.Fatalf("创建配置文件失败: %v", err)
	}

	// 创建 v1 快照
	if err := is.CreateSyncSnapshot("v1.0", []string{"mods", "config"}); err != nil {
		t.Fatalf("创建 v1.0 快照失败: %v", err)
	}

	// 修改内容
	if err := os.WriteFile(modFile, []byte("v2 content"), 0644); err != nil {
		t.Fatalf("更新 mod 文件失败: %v", err)
	}
	// 新增一个 mod
	newModFile := filepath.Join(mcDir, "mods", "fabric-api.jar")
	if err := os.WriteFile(newModFile, []byte("fabric api"), 0644); err != nil {
		t.Fatalf("创建新 mod 文件失败: %v", err)
	}
	// 删除 config 文件
	if err := os.Remove(configFile); err != nil {
		t.Fatalf("删除配置文件失败: %v", err)
	}

	// 计算差异
	diff, err := is.DiffSinceSnapshot("v1.0", []string{"mods", "config"})
	if err != nil {
		t.Fatalf("DiffSinceSnapshot 失败: %v", err)
	}

	if diff == nil {
		t.Fatal("diff 不应为 nil")
	}

	// sodium.jar 应被标记为 Updated
	foundUpdate := false
	foundAdd := false
	for _, f := range diff.Updated {
		if f.RelPath == "mods/sodium.jar" {
			foundUpdate = true
			break
		}
	}
	if !foundUpdate {
		t.Error("sodium.jar 应被标记为 Updated")
	}

	// fabric-api.jar 应被标记为 Added
	for _, f := range diff.Added {
		if f.RelPath == "mods/fabric-api.jar" {
			foundAdd = true
			break
		}
	}
	if !foundAdd {
		t.Error("fabric-api.jar 应被标记为 Added")
	}

	// options.txt 应被标记为 Deleted
	var deleted bool
	for _, f := range diff.Deleted {
		if f.RelPath == "config/options.txt" {
			deleted = true
			break
		}
	}
	if !deleted {
		t.Error("options.txt 应被标记为 Deleted")
	}
}

// TestConsumeLibraryFiles 测试库文件消费
func TestConsumeLibraryFiles(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	sha1 := "libsha100001234567890abcdef1234567890ab"

	// 先缓存一个文件
	cachedContent := []byte("cached library")
	tmpFile := filepath.Join(cfgDir, "cached_lib.jar")
	if err := os.WriteFile(tmpFile, cachedContent, 0644); err != nil {
		t.Fatalf("写入缓存源文件失败: %v", err)
	}
	is.StoreLibrary(sha1, tmpFile)

	// 创建 LibraryFile 列表
	files := []model.LibraryFile{
		// 有缓存的文件
		{
			LocalPath:    filepath.Join(cfgDir, "libraries", "cached.jar"),
			URL:          "http://example.com/cached.jar",
			SHA1:         sha1,
			IsNative:     false,
			OriginalName: "test:lib:1.0",
		},
		// 无 SHA1 的文件（应加入下载列表）
		{
			LocalPath:    filepath.Join(cfgDir, "libraries", "nohash.jar"),
			URL:          "http://example.com/nohash.jar",
			SHA1:         "",
			IsNative:     false,
			OriginalName: "nohash:lib:1.0",
		},
		// 未缓存的文件（应加入下载列表）
		{
			LocalPath:    filepath.Join(cfgDir, "libraries", "uncached.jar"),
			URL:          "http://example.com/uncached.jar",
			SHA1:         "sha1nonexistent9876543210abcdef12345678",
			IsNative:     false,
			OriginalName: "uncached:lib:1.0",
		},
	}

	toDownload, fromCache := is.ConsumeLibraryFiles(files)

	if fromCache != 1 {
		t.Errorf("期望从缓存复制 1 个文件, 得到 %d", fromCache)
	}

	if len(toDownload) != 2 {
		t.Errorf("期望 2 个文件需要下载, 得到 %d: %v", len(toDownload), toDownload)
	}

	// 验证从缓存复制的文件内容正确
	cachedDest := filepath.Join(cfgDir, "libraries", "cached.jar")
	got, err := os.ReadFile(cachedDest)
	if err != nil {
		t.Fatalf("读取从缓存复制的文件失败: %v", err)
	}
	if string(got) != string(cachedContent) {
		t.Errorf("缓存内容不匹配: 期望 %s, 得到 %s", string(cachedContent), string(got))
	}
}

// TestSyncStats 测试统计信息
func TestSyncStats(t *testing.T) {
	is, cfgDir, _ := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	stats := is.SyncStats()
	if stats == "" {
		t.Error("统计信息不应为空")
	}
}

// TestCleanOrphaned 测试清理孤立的缓存文件
func TestCleanOrphaned(t *testing.T) {
	is, cfgDir, mcDir := createTempIncrSync(t)
	defer os.RemoveAll(filepath.Dir(cfgDir))

	if err := is.EnsureRepo("1.20.4"); err != nil {
		t.Fatalf("EnsureRepo 失败: %v", err)
	}

	// 创建 mod 和快照
	modFile := filepath.Join(mcDir, "mods", "sodium.jar")
	if err := os.WriteFile(modFile, []byte("mod data"), 0644); err != nil {
		t.Fatalf("创建 mod 文件失败: %v", err)
	}
	if err := is.CreateSyncSnapshot("v1.0", []string{"mods"}); err != nil {
		t.Fatalf("创建快照失败: %v", err)
	}

	// 手动放一个未引用的文件到缓存
	unusedContent := []byte("unused")
	is.CacheStore().PutData(unusedContent, "unused_hash")

	// dry-run 清理
	deleted, _, errs := is.CleanOrphaned(true)
	if len(errs) > 0 {
		t.Fatalf("CleanOrphaned dry-run 失败: %v", errs)
	}
	if deleted != 1 {
		t.Errorf("dry-run 期望发现 1 个可删除, 得到 %d", deleted)
	}

	// 正常清理（应只删除 unused_hash，因为 mod 的 hash 被快照引用）
	deleted, _, errs = is.CleanOrphaned(false)
	if len(errs) > 0 {
		t.Fatalf("CleanOrphaned 失败: %v", errs)
	}
	if deleted != 1 {
		t.Errorf("期望删除 1 个, 得到 %d", deleted)
	}

	// unused_hash 应已被删除
	if is.CacheStore().Has("unused_hash") {
		t.Error("unused_hash 应已被清理")
	}
}
