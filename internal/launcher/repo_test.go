package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTempRepo 创建临时仓库用于测试
func createTempRepo(t *testing.T) (*LocalRepo, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mc-repo-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}

	// 创建一个 .minecraft 结构
	mcDir := filepath.Join(tmpDir, ".minecraft")
	if err := os.MkdirAll(filepath.Join(mcDir, "mods"), 0755); err != nil {
		t.Fatalf("创建 mods 目录失败: %v", err)
	}

	repo := NewLocalRepo(mcDir)
	if err := repo.Init("1.20.4"); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}

	return repo, tmpDir
}

func createTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(path), 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

func createTestFileWithContent(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

// TestRepoInit 测试仓库初始化
func TestRepoInit(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	if !repo.IsInitialized() {
		t.Error("期望仓库已初始化")
	}
	if repo.meta.MCVersion != "1.20.4" {
		t.Errorf("期望 MC 版本 1.20.4, 得到 %s", repo.meta.MCVersion)
	}
	if repo.meta.Version != 1 {
		t.Errorf("期望仓库格式版本 1, 得到 %d", repo.meta.Version)
	}

	// 验证目录结构
	if _, err := os.Stat(repo.snapshotsDir); os.IsNotExist(err) {
		t.Error("snapshots 目录未创建")
	}
	if _, err := os.Stat(repo.filesDir); os.IsNotExist(err) {
		t.Error("files 目录未创建")
	}
	if _, err := os.Stat(filepath.Join(repo.baseDir, "repo.json")); os.IsNotExist(err) {
		t.Error("repo.json 未创建")
	}
}

// TestRepoInitIdempotent 测试重复初始化幂等
func TestRepoInitIdempotent(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	// 重复初始化应该不报错
	if err := repo.Init("1.20.4"); err != nil {
		t.Errorf("重复 Init 应成功: %v", err)
	}
}

// TestCreateFullSnapshot 测试创建全量快照
func TestCreateFullSnapshot(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	// 创建测试文件
	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "sodium-0.5.3.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "lithium-0.11.2.jar"))
	createTestFile(t, filepath.Join(mcDir, "config", "sodium-options.txt"))

	// 扫描 mods 和 config
	manifest, err := repo.CreateFullSnapshot("v1.0", []string{"mods", "config"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	if len(manifest.Files) != 3 {
		t.Errorf("期望 3 个文件，得到 %d", len(manifest.Files))
	}

	// 验证快照目录
	if !repo.HasSnapshot("v1.0") {
		t.Error("快照 v1.0 应存在")
	}

	// 验证文件被缓存
	for _, entry := range manifest.Files {
		if !repo.IsCached(entry.Hash) {
			t.Errorf("文件 %s 应被缓存", entry.Hash[:12])
		}
	}

	// 验证 symlink
	symlink, err := os.Readlink(repo.currentDir)
	if err != nil {
		t.Fatalf("读取 symlink 失败: %v", err)
	}
	if !strings.HasSuffix(symlink, "v1.0") {
		t.Errorf("symlink 应指向 v1.0, 得到 %s", symlink)
	}
}

// TestLoadSnapshotManifest 测试加载快照清单
func TestLoadSnapshotManifest(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "test.jar"))

	original, err := repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	loaded, err := repo.LoadSnapshotManifest("v1.0")
	if err != nil {
		t.Fatalf("LoadSnapshotManifest 失败: %v", err)
	}

	if len(loaded.Files) != len(original.Files) {
		t.Errorf("加载的清单文件数不匹配: %d vs %d", len(loaded.Files), len(original.Files))
	}
}

// TestComputeDiff 测试差异计算
func TestComputeDiff(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)

	// 创建 v1.0: 3 个文件
	createTestFile(t, filepath.Join(mcDir, "mods", "sodium-0.5.3.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "lithium-0.11.2.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "iris-1.6.0.jar"))
	oldManifest, err := repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	// 修改：sodium 有新版本，删了 lithium，新增 fabric-api
	newManifest := &SnapshotManifest{
		Version:   "v1.1",
		CreatedAt: oldManifest.CreatedAt,
		Source:    "incremental_update",
		Files:     make(map[string]RepoFileEntry),
	}

	// 不变的文件
	newManifest.Files["mods/iris-1.6.0.jar"] = oldManifest.Files["mods/iris-1.6.0.jar"]

	// sodium 更新
	sodiumOld := oldManifest.Files["mods/sodium-0.5.3.jar"]
	newManifest.Files["mods/sodium-0.6.0.jar"] = RepoFileEntry{
		Hash:   "new-sodium-hash",
		Size:   sodiumOld.Size + 1000,
		Action: "add",
		Cached: false,
	}

	// fabric-api 新增
	newManifest.Files["mods/fabric-api-0.90.0.jar"] = RepoFileEntry{
		Hash:   "fabric-api-hash",
		Size:   2048,
		Action: "add",
		Cached: false,
	}

	// 计算差异
	diff := repo.ComputeDiff(oldManifest, newManifest)

	if len(diff.Deleted) != 2 {
		t.Errorf("期望 2 个删除(sodium+litium), 得到 %d", len(diff.Deleted))
	}
	if len(diff.Added) != 2 {
		t.Errorf("期望 2 个新增(sodium-0.6 + fabric-api), 得到 %d", len(diff.Added))
	}
	if diff.Unchanged != 1 {
		t.Errorf("期望 1 个未变(iris), 得到 %d", diff.Unchanged)
	}
	if len(diff.Updated) != 0 {
		t.Errorf("期望 0 个更新(文件名变了), 得到 %d", len(diff.Updated))
	}
}

// TestComputeDiffWithRename 测试文件更新的差异计算（同名文件 hash 变化）
func TestComputeDiffWithRename(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)

	createTestFile(t, filepath.Join(mcDir, "mods", "sodium-0.5.3.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "iris-1.6.0.jar"))
	oldManifest, err := repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	// 修改同名文件（sodium.jar 内容变了）
	newManifest := &SnapshotManifest{
		Files: make(map[string]RepoFileEntry),
	}
	newManifest.Files["mods/sodium-0.5.3.jar"] = RepoFileEntry{
		Hash:   "updated-sodium-hash",
		Size:   999999,
		Action: "add",
	}
	newManifest.Files["mods/iris-1.6.0.jar"] = oldManifest.Files["mods/iris-1.6.0.jar"]

	diff := repo.ComputeDiff(oldManifest, newManifest)

	if len(diff.Updated) != 1 {
		t.Errorf("期望 1 个更新(sodium hash 变了), 得到 %d", len(diff.Updated))
	}
	if diff.Unchanged != 1 {
		t.Errorf("期望 1 个未变(iris), 得到 %d", diff.Unchanged)
	}
}

// TestCacheFile 测试文件缓存
func TestCacheFile(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	// 创建测试文件并缓存
	srcFile := filepath.Join(tmpDir, "test-mod.jar")
	createTestFile(t, srcFile)

	ok, err := repo.CacheFile(srcFile, "test-hash-123")
	if err != nil {
		t.Fatalf("CacheFile 失败: %v", err)
	}
	if !ok {
		t.Error("期望返回 true（新缓存）")
	}

	// 再次缓存 — 应返回 false
	ok, err = repo.CacheFile(srcFile, "test-hash-123")
	if err != nil {
		t.Fatalf("重复缓存失败: %v", err)
	}
	if ok {
		t.Error("期望返回 false（已存在）")
	}

	// 验证缓存文件存在
	if !repo.IsCached("test-hash-123") {
		t.Error("缓存文件应存在")
	}
}

// TestCleanCache 测试缓存清理
func TestCleanCache(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	// 创建源文件并缓存
	createTestFile(t, filepath.Join(tmpDir, "src-a"))
	createTestFile(t, filepath.Join(tmpDir, "src-b"))
	createTestFile(t, filepath.Join(tmpDir, "src-c"))
	repo.cacheFile(filepath.Join(tmpDir, "src-a"), "hash-a")
	repo.cacheFile(filepath.Join(tmpDir, "src-b"), "hash-b")
	repo.cacheFile(filepath.Join(tmpDir, "src-c"), "hash-c")

	// 只引用 hash-a
	refs := map[string]bool{
		"hash-a": true,
	}

	deleted, freed, errs := repo.CleanCache(refs, false)
	if len(errs) > 0 {
		t.Fatalf("CleanCache 失败: %v", errs)
	}
	if deleted != 2 {
		t.Errorf("期望删除 2 个，得到 %d", deleted)
	}
	if freed <= 0 {
		t.Errorf("期望释放 >0 字节，得到 %d", freed)
	}

	// hash-a 应保留
	if !repo.IsCached("hash-a") {
		t.Error("hash-a 应保留")
	}
}

// TestCleanCacheDryRun 测试缓存清理的 dry-run 模式
func TestCleanCacheDryRun(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	createTestFile(t, filepath.Join(tmpDir, "src-a"))
	createTestFile(t, filepath.Join(tmpDir, "src-b"))
	repo.cacheFile(filepath.Join(tmpDir, "src-a"), "hash-a")
	repo.cacheFile(filepath.Join(tmpDir, "src-b"), "hash-b")

	deleted, _, _ := repo.CleanCache(map[string]bool{"hash-a": true}, true)
	if deleted != 1 {
		t.Errorf("dry-run: 期望发现 1 个可删除，得到 %d", deleted)
	}
	if !repo.IsCached("hash-b") {
		t.Error("dry-run: hash-b 应仍存在")
	}
}

// TestListSnapshots 测试快照列表
func TestListSnapshots(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "test.jar"))

	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	repo.CreateFullSnapshot("v1.1", []string{"mods"}, "incremental_update", "v1.0")

	snapshots, err := repo.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots 失败: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("期望 2 个快照，得到 %d", len(snapshots))
	}
	if snapshots[0] != "v1.1" {
		t.Errorf("最新快照应在最前: 期望 v1.1, 得到 %s", snapshots[0])
	}
}

// TestDeleteSnapshot 测试删除快照
func TestDeleteSnapshot(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "test.jar"))

	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	repo.CreateFullSnapshot("v1.1", []string{"mods"}, "incremental_update", "v1.0")

	if err := repo.DeleteSnapshot("v1.0"); err != nil {
		t.Fatalf("DeleteSnapshot 失败: %v", err)
	}

	snapshots, _ := repo.ListSnapshots()
	if len(snapshots) != 1 {
		t.Errorf("删除后期望 1 个快照，得到 %d", len(snapshots))
	}
	if repo.LatestSnapshot() != "v1.1" {
		t.Errorf("latest 应为 v1.1, 得到 %s", repo.LatestSnapshot())
	}
}

// TestScanDirectory 测试目录扫描
func TestScanDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mc-scan-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	createTestFile(t, filepath.Join(tmpDir, "file1.txt"))
	createTestFile(t, filepath.Join(tmpDir, "sub", "file2.txt"))

	manifest := &SnapshotManifest{
		Files: make(map[string]RepoFileEntry),
	}

	if err := ScanDirectory(tmpDir, manifest); err != nil {
		t.Fatalf("ScanDirectory 失败: %v", err)
	}

	if len(manifest.Files) != 2 {
		t.Errorf("期望 2 个文件，得到 %d", len(manifest.Files))
	}

	if _, ok := manifest.Files["file1.txt"]; !ok {
		t.Error("file1.txt 应在清单中")
	}
	if _, ok := manifest.Files["sub/file2.txt"]; !ok {
		t.Error("sub/file2.txt 应在清单中")
	}
}

// TestHasSnapshot 测试快照存在检查
func TestHasSnapshot(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	if repo.HasSnapshot("v0.0") {
		t.Error("v0.0 不应存在")
	}

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "test.jar"))
	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")

	if !repo.HasSnapshot("v1.0") {
		t.Error("v1.0 应存在")
	}
}

// TestStats 测试统计
func TestStats(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "test.jar"))
	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")

	stats := repo.Stats()
	if !strings.Contains(stats, "MC=1.20.4") {
		t.Errorf("Stats 应包含 MC 版本信息: %s", stats)
	}
	if !strings.Contains(stats, "快照=1") {
		t.Errorf("Stats 应包含快照计数: %s", stats)
	}
	if !strings.Contains(stats, "缓存=1") {
		t.Errorf("Stats 应包含缓存计数: %s", stats)
	}
}

// TestReferencedHashes 测试引用 hash 收集
func TestReferencedHashes(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	createTestFile(t, filepath.Join(mcDir, "mods", "sodium.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "lithium.jar"))
	repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")

	hashes, err := repo.ReferencedHashes()
	if err != nil {
		t.Fatalf("ReferencedHashes 失败: %v", err)
	}

	if len(hashes) != 2 {
		t.Errorf("期望 2 个引用 hash，得到 %d", len(hashes))
	}
}

// TestApplyDiff 测试应用差异
func TestApplyDiff(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)

	// 创建旧版本
	createTestFile(t, filepath.Join(mcDir, "mods", "old-mod.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "keep-mod.jar"))
	createTestFile(t, filepath.Join(mcDir, "config", "keep-config.txt"))
	oldManifest, err := repo.CreateFullSnapshot("v1.0", []string{"mods", "config"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	// 新建版本：删 old-mod，新增 new-mod
	newManifest := &SnapshotManifest{Files: make(map[string]RepoFileEntry)}
	// 留 keep-mod
	newManifest.Files["mods/keep-mod.jar"] = oldManifest.Files["mods/keep-mod.jar"]
	// 留 keep-config
	newManifest.Files["config/keep-config.txt"] = oldManifest.Files["config/keep-config.txt"]
	// 新增 new-mod — 先缓存它
	newFilePath := filepath.Join(tmpDir, "new-mod.jar")
	createTestFile(t, newFilePath)
	newHash, _ := computeFileHash(newFilePath)
	repo.CacheFile(newFilePath, newHash)
	newManifest.Files["mods/new-mod.jar"] = RepoFileEntry{
		Hash: newHash, Size: 100, Action: "add",
	}

	diff := repo.ComputeDiff(oldManifest, newManifest)

	applied := repo.ApplyDiff(diff, mcDir)
	if applied != 1 {
		t.Errorf("期望应用 1 个文件，得到 %d", applied)
	}

	// 验证
	if _, err := os.Stat(filepath.Join(mcDir, "mods", "old-mod.jar")); !os.IsNotExist(err) {
		t.Error("old-mod.jar 应已被删除")
	}
	if _, err := os.Stat(filepath.Join(mcDir, "mods", "new-mod.jar")); os.IsNotExist(err) {
		t.Error("new-mod.jar 应已添加")
	}
	if _, err := os.Stat(filepath.Join(mcDir, "mods", "keep-mod.jar")); os.IsNotExist(err) {
		t.Error("keep-mod.jar 应保留")
	}
}

// TestRestoreSnapshot 测试快照恢复到 .minecraft
func TestRestoreSnapshot(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	os.MkdirAll(filepath.Join(mcDir, "mods"), 0755)

	// 创建两个文件并建立快照
	createTestFile(t, filepath.Join(mcDir, "mods", "sodium.jar"))
	createTestFile(t, filepath.Join(mcDir, "mods", "iris.jar"))
	createTestFile(t, filepath.Join(mcDir, "config", "sodium-options.txt"))

	manifest, err := repo.CreateFullSnapshot("v1.0", []string{"mods", "config"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}
	_ = manifest

	// 删除 mods 目录的内容（模拟崩溃或意外丢失）
	os.RemoveAll(filepath.Join(mcDir, "mods"))
	os.RemoveAll(filepath.Join(mcDir, "config"))

	// 恢复快照
	if err := repo.RestoreSnapshot("v1.0", mcDir); err != nil {
		t.Fatalf("RestoreSnapshot 失败: %v", err)
	}

	// 验证恢复
	for _, file := range []string{"mods/sodium.jar", "mods/iris.jar", "config/sodium-options.txt"} {
		path := filepath.Join(mcDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("文件 %s 应从快照恢复", file)
		}
	}
}

// TestRestoreSnapshotWithDeleteActions 测试快照恢复时处理 delete 条目
func TestRestoreSnapshotWithDeleteActions(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	os.MkdirAll(filepath.Join(mcDir, "mods"), 0755)

	// 创建文件并建立快照
	createTestFile(t, filepath.Join(mcDir, "mods", "sodium.jar"))
	man, err := repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	// 手动给一个条目加上 delete action（模拟增量回滚）
	sodiumEntry := man.Files["mods/sodium.jar"]
	sodiumEntry.Action = "delete"
	man.Files["mods/sodium.jar"] = sodiumEntry

	// 写入 mock manifest
	snapDir := repo.SnapshotDir("v1.0")
	data, _ := json.MarshalIndent(man, "", "  ")
	os.WriteFile(filepath.Join(snapDir, "manifest.json"), data, 0644)

	// 删除 sodium.jar — RestoreSnapshot 删除 标记的文件
	os.RemoveAll(filepath.Join(mcDir, "mods"))

	// 恢复：sodium.jar 上有 delete action，不应被恢复
	if err := repo.RestoreSnapshot("v1.0", mcDir); err != nil {
		t.Fatalf("RestoreSnapshot 失败: %v", err)
	}

	// delete 的文件不应存在
	if _, err := os.Stat(filepath.Join(mcDir, "mods", "sodium.jar")); !os.IsNotExist(err) {
		t.Error("delete action 的文件不应被恢复")
	}
}

// TestRestoreSnapshotInvalidName 测试恢复不存在的快照
func TestRestoreSnapshotInvalidName(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	err := repo.RestoreSnapshot("nonexistent", tmpDir)
	if err == nil {
		t.Error("恢复不存在的快照应返回错误")
	}
}

// TestRestoreSnapshotOverwrites 测试快照恢复正确覆盖已有文件
func TestRestoreSnapshotOverwrites(t *testing.T) {
	repo, tmpDir := createTempRepo(t)
	defer os.RemoveAll(tmpDir)

	mcDir := filepath.Dir(repo.baseDir)
	os.MkdirAll(filepath.Join(mcDir, "mods"), 0755)

	// 创建快照版本
	originalContent := []byte("original content")
	createTestFileWithContent(t, filepath.Join(mcDir, "mods", "config-override.txt"), originalContent)

	_, err := repo.CreateFullSnapshot("v1.0", []string{"mods"}, "full_download", "")
	if err != nil {
		t.Fatalf("CreateFullSnapshot 失败: %v", err)
	}

	// 用新内容覆盖文件
	newContent := []byte("modified content")
	os.WriteFile(filepath.Join(mcDir, "mods", "config-override.txt"), newContent, 0644)

	// 重新扫描并创建 v2 快照
	os.MkdirAll(filepath.Join(mcDir, "config"), 0755)
	repo.CreateFullSnapshot("v2.0", []string{"mods", "config"}, "incremental_update", "v1.0")

	// 恢复 v1.0 快照
	if err := repo.RestoreSnapshot("v1.0", mcDir); err != nil {
		t.Fatalf("RestoreSnapshot v1.0 失败: %v", err)
	}

	// 验证内容被恢复为 original
	restored, err := os.ReadFile(filepath.Join(mcDir, "mods", "config-override.txt"))
	if err != nil {
		t.Fatalf("读取恢复的文件失败: %v", err)
	}
	if string(restored) != string(originalContent) {
		t.Errorf("内容应被覆盖为原始内容: 期望 %s, 得到 %s", string(originalContent), string(restored))
	}
}
