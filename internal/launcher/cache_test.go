package launcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTempCache 创建临时缓存用于测试
func createTempCache(t *testing.T) (*CacheStore, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mc-cache-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}

	cs := NewCacheStore(tmpDir)
	if err := cs.Init(); err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	return cs, tmpDir
}

func createTestFileForCache(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	data := []byte(content)
	if data == nil {
		data = []byte(path)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

// TestCacheInit 测试缓存初始化
func TestCacheInit(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	if !cs.Has("nonexistent") {
		// 还没加任何东西，Has 应该返回 false
	}

	// 验证目录结构
	if _, err := os.Stat(cs.filesDir); os.IsNotExist(err) {
		t.Error("files 目录应存在")
	}
	if _, err := os.Stat(cs.indexDir); os.IsNotExist(err) {
		t.Error("indexes 目录应存在")
	}
}

// TestCachePutAndHas 测试放入和检查
func TestCachePutAndHas(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "test.dat")
	createTestFileForCache(t, srcFile, "hello cache")

	// Put
	ok, err := cs.Put(srcFile, "sha256:abc123")
	if err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	if !ok {
		t.Error("期望 true（新缓存）")
	}

	if !cs.Has("sha256:abc123") {
		t.Error("缓存应存在")
	}

	// 重复 Put — 应返回 false（已存在）
	ok, err = cs.Put(srcFile, "sha256:abc123")
	if err != nil {
		t.Fatalf("重复 Put 失败: %v", err)
	}
	if ok {
		t.Error("重复 Put 期望 false")
	}
}

// TestCacheGet 测试获取缓存文件
func TestCacheGet(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "test.dat")
	createTestFileForCache(t, srcFile, "get test data")

	cs.Put(srcFile, "sha256:get0001")

	path, err := cs.Get("sha256:get0001")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if path == "" {
		t.Fatal("期望非空路径")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取缓存文件失败: %v", err)
	}
	if string(data) != "get test data" {
		t.Errorf("内容不匹配: 期望 'get test data', 得到 '%s'", string(data))
	}

	// 获取不存在的 key
	_, err = cs.Get("sha256:nonexist")
	if err == nil {
		t.Error("期望获取不存在的 key 返回错误")
	}
}

// TestCachePutData 测试 PutData
func TestCachePutData(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	path, err := cs.PutData([]byte("direct data"), "sha256:direct001")
	if err != nil {
		t.Fatalf("PutData 失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if string(data) != "direct data" {
		t.Errorf("内容不匹配: 期望 'direct data', 得到 '%s'", string(data))
	}
}

// TestCacheIndex 测试索引缓存
func TestCacheIndex(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	err := cs.PutIndex("test_index.json", []byte(`{"key": "value"}`))
	if err != nil {
		t.Fatalf("PutIndex 失败: %v", err)
	}

	if !cs.HasIndex("test_index.json") {
		t.Error("索引应存在")
	}

	data, err := cs.GetIndex("test_index.json")
	if err != nil {
		t.Fatalf("GetIndex 失败: %v", err)
	}
	if string(data) != `{"key": "value"}` {
		t.Errorf("内容不匹配: %s", string(data))
	}
}

// TestCacheClean 测试缓存清理
func TestCacheClean(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	// 放入 3 个文件
	srcA := filepath.Join(tmpDir, "a.dat")
	srcB := filepath.Join(tmpDir, "b.dat")
	srcC := filepath.Join(tmpDir, "c.dat")
	createTestFileForCache(t, srcA, "aaaa")
	createTestFileForCache(t, srcB, "bbbb")
	createTestFileForCache(t, srcC, "cccc")

	cs.Put(srcA, "hash-a")
	cs.Put(srcB, "hash-b")
	cs.Put(srcC, "hash-c")

	// 对 hash-a 调 Get 增加引用计数
	cs.Get("hash-a")

	// 清理：引用计数 < 2 的删除
	deleted, freed, errs := cs.Clean(CleanOptions{MinRefCount: 2})
	if len(errs) > 0 {
		t.Fatalf("Clean 失败: %v", errs)
	}
	if deleted != 2 {
		t.Errorf("期望删除 2 个(hash-b, hash-c), 得到 %d", deleted)
	}
	if freed <= 0 {
		t.Errorf("期望释放 >0 字节, 得到 %d", freed)
	}

	if !cs.Has("hash-a") {
		t.Error("hash-a 应保留(引用计数=2)")
	}
	if cs.Has("hash-b") {
		t.Error("hash-b 应被删除")
	}
}

// TestCacheCleanDryRun 测试 dry-run 模式
func TestCacheCleanDryRun(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcA := filepath.Join(tmpDir, "a.dat")
	srcB := filepath.Join(tmpDir, "b.dat")
	createTestFileForCache(t, srcA, "aaaa")
	createTestFileForCache(t, srcB, "bbbb")
	cs.Put(srcA, "hash-a")
	cs.Put(srcB, "hash-b")

	deleted, _, errs := cs.Clean(CleanOptions{
		DryRun:     true,
		KeepHashes: map[string]bool{"hash-a": true},
	})
	if len(errs) > 0 {
		t.Fatalf("Clean dry-run 失败: %v", errs)
	}
	if deleted != 1 {
		t.Errorf("dry-run: 期望发现 1 个可删除, 得到 %d", deleted)
	}
	if !cs.Has("hash-b") {
		t.Error("dry-run: hash-b 应仍存在")
	}
}

// TestCacheCleanMaxAge 测试过期清理
func TestCacheCleanMaxAge(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "old.dat")
	createTestFileForCache(t, srcFile, "old data")
	cs.Put(srcFile, "hash-old")

	// 清理：最大过期 1ns（即全都过期）
	deleted, _, errs := cs.Clean(CleanOptions{MaxAge: 1 * time.Nanosecond})
	if len(errs) > 0 {
		t.Fatalf("Clean 失败: %v", errs)
	}
	if deleted != 1 {
		t.Errorf("期望删除 1 个, 得到 %d", deleted)
	}
}

// TestCacheRefCount 测试引用计数
func TestCacheRefCount(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "ref.dat")
	createTestFileForCache(t, srcFile, "ref data")

	cs.Put(srcFile, "hash-ref")
	if cs.RefCount("hash-ref") != 1 {
		t.Errorf("Put 后期望引用计数 1, 得到 %d", cs.RefCount("hash-ref"))
	}

	cs.Get("hash-ref")
	if cs.RefCount("hash-ref") != 2 {
		t.Errorf("Get 后期望引用计数 2, 得到 %d", cs.RefCount("hash-ref"))
	}
}

// TestCacheComputeSHA256 测试 SHA256 计算
func TestCacheComputeSHA256(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "hash.dat")
	createTestFileForCache(t, srcFile, "test content")

	hash, err := cs.ComputeSHA256(srcFile)
	if err != nil {
		t.Fatalf("ComputeSHA256 失败: %v", err)
	}
	if hash == "" {
		t.Error("期望非空 hash")
	}

	// 验证一致性
	hash2, _ := cs.ComputeSHA256(srcFile)
	if hash != hash2 {
		t.Error("期望相同输入产生相同 hash")
	}
}

// TestCacheStats 测试统计信息
func TestCacheStats(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(tmpDir, "stat.dat")
	createTestFileForCache(t, srcFile, "stats data")
	cs.Put(srcFile, "hash-stat")

	stats := cs.Stats()
	if stats == "" {
		t.Error("期望非空统计信息")
	}
}

// TestCacheCachedFilePath 测试缓存路径生成
func TestCacheCachedFilePath(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	path := cs.CachedFilePath("sha256:test")
	expected := filepath.Join(tmpDir, "files", "sha256:test")
	if path != expected {
		t.Errorf("路径不匹配: 期望 %s, 得到 %s", expected, path)
	}
}

// TestCacheReferencedHashes 测试引用 hash 收集
func TestCacheReferencedHashes(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	srcA := filepath.Join(tmpDir, "a.dat")
	srcB := filepath.Join(tmpDir, "b.dat")
	createTestFileForCache(t, srcA, "aaaa")
	createTestFileForCache(t, srcB, "bbbb")
	cs.Put(srcA, "hash-a")
	cs.Put(srcB, "hash-b")

	refs := cs.ReferencedHashes()
	if len(refs) != 2 {
		t.Errorf("期望 2 个引用, 得到 %d", len(refs))
	}
	if !refs["hash-a"] {
		t.Error("hash-a 应被引用")
	}
}

// TestCacheEmptyClean 测试空缓存清理
func TestCacheEmptyClean(t *testing.T) {
	cs, tmpDir := createTempCache(t)
	defer os.RemoveAll(tmpDir)

	deleted, freed, errs := cs.Clean(CleanOptions{})
	if len(errs) > 0 {
		t.Fatalf("Clean 失败: %v", errs)
	}
	if deleted != 0 {
		t.Errorf("期望 0 删除, 得到 %d", deleted)
	}
	if freed != 0 {
		t.Errorf("期望 0 释放, 得到 %d", freed)
	}
}
