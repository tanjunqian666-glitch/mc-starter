package pack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		t.Fatalf("创建 zip 目录失败: %v", err)
	}
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建 zip 文件失败: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目 %s 失败: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("写入 zip 条目 %s 失败: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}
}

func TestImportZip(t *testing.T) {
	repoDir := t.TempDir()
	// 创建打包名为 pack 的子目录
	os.MkdirAll(filepath.Join(repoDir, "versions"), 0755)
	zipPath := filepath.Join(repoDir, "cjc-pack-v1.0.0.zip")

	// 创建测试 zip
	createTestZip(t, zipPath, map[string]string{
		"mods/sodium.jar":        "sodium-binary",
		"mods/lithium.jar":       "lithium-binary",
		"config/options.txt":     "key=value",
	})

	// 导入
	result, err := ImportZip(zipPath, repoDir, "")
	if err != nil {
		t.Fatalf("ImportZip failed: %v", err)
	}

	if result.Status != "draft" {
		t.Errorf("expected status draft, got %s", result.Status)
	}
	if result.Version == "" {
		t.Error("expected non-empty version")
	}
	if result.Manifest.FileCount != 3 {
		t.Errorf("expected 3 files, got %d", result.Manifest.FileCount)
	}

	// 检查清单路径
	paths := make(map[string]bool)
	for _, f := range result.Manifest.Files {
		paths[f.Path] = true
		if f.SHA1 == "" {
			t.Errorf("file %s has empty SHA1", f.Path)
		}
	}
	if !paths["mods/sodium.jar"] {
		t.Error("expected mods/sodium.jar in manifest")
	}
	if !paths["config/options.txt"] {
		t.Error("expected config/options.txt in manifest")
	}

	// 这是第一版，应该没有 diff
	if result.Diff != nil {
		t.Error("expected nil diff for first import")
	}

	// 检查 draft 目录是否有文件
	draftDir := filepath.Join(repoDir, "versions", result.Version+".draft")
	if _, err := os.Stat(draftDir); os.IsNotExist(err) {
		t.Errorf("draft directory %s should exist", draftDir)
	}
	if _, err := os.Stat(filepath.Join(draftDir, "manifest.json")); os.IsNotExist(err) {
		t.Error("manifest.json should exist in draft")
	}
	if _, err := os.Stat(filepath.Join(draftDir, "meta.json")); os.IsNotExist(err) {
		t.Error("meta.json should exist in draft")
	}
}

func TestImportZipWithFlatten(t *testing.T) {
	repoDir := t.TempDir()
	zipPath := filepath.Join(repoDir, "pack-v2.zip")

	// 单层顶层目录的 zip
	createTestZip(t, zipPath, map[string]string{
		"Modpack/mods/sodium.jar":  "sodium",
		"Modpack/config/test.toml": "config",
	})

	result, err := ImportZip(zipPath, repoDir, "v2.0.0")
	if err != nil {
		t.Fatalf("ImportZip failed: %v", err)
	}

	// 验证展平
	paths := make(map[string]bool)
	for _, f := range result.Manifest.Files {
		paths[f.Path] = true
	}
	if paths["Modpack/mods/sodium.jar"] {
		t.Error("expected flattened path, got Modpack/mods/sodium.jar")
	}
	if !paths["mods/sodium.jar"] {
		t.Error("expected mods/sodium.jar after flatten")
	}
	if !paths["config/test.toml"] {
		t.Error("expected config/test.toml after flatten")
	}
}

func TestDiffVersions(t *testing.T) {
	repoDir := t.TempDir()

	// 先导入 v1，再导入 v2，对比差异
	v1zip := filepath.Join(repoDir, "v1.zip")
	createTestZip(t, v1zip, map[string]string{
		"mods/a.jar": "content-a",
		"mods/b.jar": "content-b",
		"config/x":   "config-x",
	})

	v1, err := ImportZip(v1zip, repoDir, "v1.0.0")
	if err != nil {
		t.Fatalf("import v1: %v", err)
	}
	if v1.Diff != nil {
		t.Error("v1 should have no diff")
	}

	// publish v1
	if err := PublishDraft(repoDir, "v1.0.0", ""); err != nil {
		t.Fatalf("publish v1: %v", err)
	}

	// 导入 v2：改 b.jar，新增 c.jar，删除 a.jar，config/x 不变
	v2zip := filepath.Join(repoDir, "v2.zip")
	createTestZip(t, v2zip, map[string]string{
		"mods/b.jar": "content-b-updated",
		"mods/c.jar": "content-c",
		"config/x":   "config-x",
	})

	v2, err := ImportZip(v2zip, repoDir, "v2.0.0")
	if err != nil {
		t.Fatalf("import v2: %v", err)
	}

	if v2.Diff == nil {
		t.Fatal("v2 should have diff")
	}
	if !v2.Diff.HasChanges() {
		t.Fatal("v2 diff should have changes")
	}

	// 验证差异
	if len(v2.Diff.Added) != 1 || v2.Diff.Added[0].Path != "mods/c.jar" {
		t.Errorf("expected 1 added (mods/c.jar), got %v", v2.Diff.Added)
	}
	if len(v2.Diff.Removed) != 1 || v2.Diff.Removed[0].Path != "mods/a.jar" {
		t.Errorf("expected 1 removed (mods/a.jar), got %v", v2.Diff.Removed)
	}
	if len(v2.Diff.Updated) != 1 || v2.Diff.Updated[0].Path != "mods/b.jar" {
		t.Errorf("expected 1 updated (mods/b.jar), got %v", v2.Diff.Updated)
	}
	if v2.Diff.Unchanged != 1 {
		t.Errorf("expected 1 unchanged (config/x), got %d", v2.Diff.Unchanged)
	}

	// 验证 prevVersion
	if v2.PrevVersion != "v1.0.0" {
		t.Errorf("expected prevVersion v1.0.0, got %s", v2.PrevVersion)
	}
}

func TestPublishDraft(t *testing.T) {
	repoDir := t.TempDir()

	zipPath := filepath.Join(repoDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"mods/test.jar": "test",
	})

	_, err := ImportZip(zipPath, repoDir, "v1.0.0")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// 发布
	if err := PublishDraft(repoDir, "", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 检查 published 目录
	pubDir := filepath.Join(repoDir, "versions", "v1.0.0")
	if _, err := os.Stat(pubDir); os.IsNotExist(err) {
		t.Errorf("published dir %s should exist", pubDir)
	}
	if _, err := os.Stat(filepath.Join(pubDir, "manifest.json")); os.IsNotExist(err) {
		t.Error("manifest.json should exist in published")
	}

	// draft 应该不存在了
	draftDir := filepath.Join(repoDir, "versions", "v1.0.0.draft")
	if _, err := os.Stat(draftDir); !os.IsNotExist(err) {
		t.Error("draft dir should have been removed")
	}

	// 检查 current symlink（Wine/Windows 下可能不支持）
	current := filepath.Join(repoDir, "current")
	if link, err := os.Readlink(current); err == nil {
		if filepath.Base(link) != "v1.0.0" {
			t.Errorf("expected current -> v1.0.0, got %s", link)
		}
	}

	// 检查 server.json
	if _, err := os.Stat(filepath.Join(repoDir, "server.json")); os.IsNotExist(err) {
		t.Error("server.json should exist after publish")
	}
}

func TestDuplicatePublish(t *testing.T) {
	repoDir := t.TempDir()

	zipPath := filepath.Join(repoDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"a": "1"})

	ImportZip(zipPath, repoDir, "v1")
	if err := PublishDraft(repoDir, "v1", ""); err != nil {
		t.Fatalf("first publish: %v", err)
	}

	// 重复发布应该报错
	if err := PublishDraft(repoDir, "v1", ""); err == nil {
		t.Error("expected error for duplicate publish")
	}
}

func TestListVersions(t *testing.T) {
	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "versions"), 0755)

	drafts, published, err := ListVersions(repoDir)
	if err != nil {
		t.Fatalf("ListVersions on empty repo: %v", err)
	}
	if len(drafts) != 0 || len(published) != 0 {
		t.Errorf("expected empty, got drafts=%d published=%d", len(drafts), len(published))
	}

	// 导入两个版本
	createTestZip(t, filepath.Join(repoDir, "v1.zip"), map[string]string{"a": "1"})
	ImportZip(zipPath(repoDir, "v1"), repoDir, "v1")

	createTestZip(t, filepath.Join(repoDir, "v2.zip"), map[string]string{"b": "2"})
	ImportZip(zipPath(repoDir, "v2"), repoDir, "v2")

	drafts, published, _ = ListVersions(repoDir)
	if len(drafts) != 2 {
		t.Errorf("expected 2 drafts, got %d", len(drafts))
	}
	if len(published) != 0 {
		t.Errorf("expected 0 published, got %d", len(published))
	}

	// 发布第一个
	PublishDraft(repoDir, "v1", "")
	drafts, published, _ = ListVersions(repoDir)
	if len(drafts) != 1 {
		t.Errorf("expected 1 draft, got %d", len(drafts))
	}
	if len(published) != 1 {
		t.Errorf("expected 1 published, got %d", len(published))
	}
}

func zipPath(dir, name string) string {
	return filepath.Join(dir, name+".zip")
}

func TestComputeDiffIdentical(t *testing.T) {
	prev := &Manifest{
		Version: "v1",
		Files: []FileEntry{
			{Path: "mods/a.jar", SHA1: "abc"},
			{Path: "mods/b.jar", SHA1: "def"},
		},
	}
	curr := &Manifest{
		Version: "v2",
		Files: []FileEntry{
			{Path: "mods/a.jar", SHA1: "abc"},
			{Path: "mods/b.jar", SHA1: "def"},
		},
	}

	diff := ComputeDiff(prev, curr)
	if diff.HasChanges() {
		t.Errorf("expected no changes for identical manifests")
	}
	if diff.Unchanged != 2 {
		t.Errorf("expected 2 unchanged, got %d", diff.Unchanged)
	}
}

func TestComputeDiffAllChanged(t *testing.T) {
	prev := &Manifest{
		Version: "v1",
		Files:   []FileEntry{},
	}
	curr := &Manifest{
		Version: "v2",
		Files: []FileEntry{
			{Path: "mods/new.jar", SHA1: "abc", Size: 1000},
		},
	}

	diff := ComputeDiff(prev, curr)
	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(diff.Added))
	}
	if diff.TotalDiffBytes() != 1000 {
		t.Errorf("expected 1000 diff bytes, got %d", diff.TotalDiffBytes())
	}
}
