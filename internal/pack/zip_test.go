package pack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTestZip 在指定路径创建一个测试用的 zip 文件
// files: map[包内路径]文件内容
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

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"mods/sodium.jar", "mods/sodium.jar"},
		{"../escape.jar", "escape.jar"},
		{"a/../../b/c.jar", "b/c.jar"},
		{"./mods/./sodium.jar", "mods/sodium.jar"},
		{"foo\\bar.jar", "foo/bar.jar"},
		{"a/b/../c/d.jar", "a/c/d.jar"},
	}

	for _, tt := range tests {
		got := sanitizePath(tt.input)
		expected := strings.ReplaceAll(tt.expected, "/", string(filepath.Separator))
		if got != expected {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, expected)
		}
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	files := map[string]string{
		"mods/sodium.jar":     "binary-content-1",
		"mods/fabric-api.jar": "binary-content-2",
		"config/options.txt":  "key=value",
	}
	createTestZip(t, zipPath, files)

	handler := NewZipHandler()
	result, err := handler.ExtractExisting(zipPath, dir, "test")
	if err != nil {
		t.Fatalf("ExtractExisting failed: %v", err)
	}

	if len(result.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result.Entries))
	}

	// 检查条目内容
	entryMap := make(map[string]ZipEntry)
	for _, e := range result.Entries {
		entryMap[e.RelPath] = e
	}

	expectedFiles := []string{"mods/sodium.jar", "mods/fabric-api.jar", "config/options.txt"}
	for _, name := range expectedFiles {
		entry, ok := entryMap[name]
		if !ok {
			t.Errorf("expected entry %s not found", name)
			continue
		}
		if entry.SHA1 == "" {
			t.Errorf("entry %s has empty SHA1", name)
		}
		if entry.SHA256 == "" {
			t.Errorf("entry %s has empty SHA256", name)
		}
	}

	// 验证临时文件存在
	for _, entry := range result.Entries {
		if _, err := os.Stat(entry.TempPath); os.IsNotExist(err) {
			t.Errorf("temp file %s does not exist", entry.TempPath)
		}
	}

	// 验证 cleanup
	result.Cleanup()
	if _, err := os.Stat(result.TempRoot); !os.IsNotExist(err) {
		t.Errorf("temp root %s should have been removed", result.TempRoot)
	}
}

func TestFlattenSingleDir(t *testing.T) {
	tests := []struct {
		name     string
		entries  []ZipEntry
		expected []string // flattened relPaths
	}{
		{
			name: "single top dir — flatten",
			entries: []ZipEntry{
				{RelPath: "Modpack/mods/a.jar", TempPath: "/tmp/a"},
				{RelPath: "Modpack/config/b.txt", TempPath: "/tmp/b"},
			},
			expected: []string{"mods/a.jar", "config/b.txt"},
		},
		{
			name: "no top dir — no change",
			entries: []ZipEntry{
				{RelPath: "mods/a.jar", TempPath: "/tmp/a"},
				{RelPath: "config/b.txt", TempPath: "/tmp/b"},
			},
			expected: []string{"mods/a.jar", "config/b.txt"},
		},
		{
			name: "multiple top dirs — no flatten",
			entries: []ZipEntry{
				{RelPath: "Modpack1/mods/a.jar", TempPath: "/tmp/a"},
				{RelPath: "Modpack2/config/b.txt", TempPath: "/tmp/b"},
			},
			expected: []string{"Modpack1/mods/a.jar", "Modpack2/config/b.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ZipResult{Entries: tt.entries}
			result = flattenSingleDir(result)
			for i, entry := range result.Entries {
				if entry.RelPath != tt.expected[i] {
					t.Errorf("entry %d: got %q, want %q", i, entry.RelPath, tt.expected[i])
				}
			}
		})
	}
}

func TestComputeDiff(t *testing.T) {
	dir := t.TempDir()

	// 创建目标目录中的现有文件
	os.MkdirAll(filepath.Join(dir, "mods"), 0755)
	os.WriteFile(filepath.Join(dir, "mods", "unchanged.jar"), []byte("same-content"), 0644)
	os.WriteFile(filepath.Join(dir, "mods", "toremove.jar"), []byte("old-content"), 0644)
	os.WriteFile(filepath.Join(dir, "mods", "updated.jar"), []byte("old-updated-content"), 0644)

	// zip 中的文件（unchanged 不变，新增 new.jar，更新 updated.jar）
	zipEntries := []ZipEntry{
		{RelPath: "mods/unchanged.jar", SHA1: mustSHA1(t, []byte("same-content")), Size: 12},
		{RelPath: "mods/new.jar", SHA1: mustSHA1(t, []byte("new-content")), Size: 11},
		{RelPath: "mods/updated.jar", SHA1: mustSHA1(t, []byte("new-updated-content")), Size: 19},
	}

	diff := ComputeDiff(zipEntries, dir, "mods")

	// unchanged 应该匹配（因为本地文件内容就是 same-content）
	if diff.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", diff.Unchanged)
	}
	if len(diff.Added) != 1 || diff.Added[0].RelPath != "mods/new.jar" {
		t.Errorf("expected 1 added (mods/new.jar), got %v", diff.Added)
	}
	if len(diff.Updated) != 1 || diff.Updated[0].RelPath != "mods/updated.jar" {
		t.Errorf("expected 1 updated (mods/updated.jar), got %v", diff.Updated)
	}
	if len(diff.Deleted) != 1 || diff.Deleted[0].RelPath != "mods/toremove.jar" {
		t.Errorf("expected 1 deleted (mods/toremove.jar), got %v", diff.Deleted)
	}
}

func TestComputeDiffIdentical(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mods"), 0755)
	os.WriteFile(filepath.Join(dir, "mods", "a.jar"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(dir, "mods", "b.jar"), []byte("content2"), 0644)

	zipEntries := []ZipEntry{
		{RelPath: "mods/a.jar", SHA1: mustSHA1(t, []byte("content")), Size: 7},
		{RelPath: "mods/b.jar", SHA1: mustSHA1(t, []byte("content2")), Size: 8},
	}

	diff := ComputeDiff(zipEntries, dir, "mods")
	if diff.Unchanged != 2 {
		t.Errorf("expected 2 unchanged, got %d", diff.Unchanged)
	}
	if diff.HasChanges() {
		t.Errorf("expected no changes, got %s", diff.Summary())
	}
}

func TestApplyDiff(t *testing.T) {
	dir := t.TempDir()
	tempDir := t.TempDir()

	// 准备 zip 源文件
	os.WriteFile(filepath.Join(tempDir, "new.jar"), []byte("new-content"), 0644)
	os.WriteFile(filepath.Join(tempDir, "updated.jar"), []byte("updated-content"), 0644)

	// 准备本地文件
	os.MkdirAll(filepath.Join(dir, "mods"), 0755)
	os.WriteFile(filepath.Join(dir, "mods", "old.jar"), []byte("old-content"), 0644)
	os.WriteFile(filepath.Join(dir, "mods", "unchanged.jar"), []byte("same"), 0644)

	zipEntries := []ZipEntry{
		{RelPath: "mods/new.jar", TempPath: filepath.Join(tempDir, "new.jar"), SHA1: "abc", Size: 11},
		{RelPath: "mods/updated.jar", TempPath: filepath.Join(tempDir, "updated.jar"), SHA1: "def", Size: 15},
	}

	diff := &DiffResult{
		Added:   []DiffEntry{{RelPath: "mods/new.jar", Status: "added"}},
		Updated: []DiffEntry{{RelPath: "mods/updated.jar", Status: "updated"}},
		Deleted: []DiffEntry{{RelPath: "mods/old.jar", Status: "deleted"}},
	}

	applied := ApplyDiff(diff, zipEntries, dir, false)
	if applied != 3 {
		t.Errorf("expected 3 applied, got %d", applied)
	}

	// 验证结果
	if _, err := os.Stat(filepath.Join(dir, "mods", "new.jar")); os.IsNotExist(err) {
		t.Error("new.jar should exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "updated.jar")); os.IsNotExist(err) {
		t.Error("updated.jar should exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "old.jar")); !os.IsNotExist(err) {
		t.Error("old.jar should have been deleted")
	}
}

func TestApplyDiffDryRun(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mods"), 0755)
	os.WriteFile(filepath.Join(dir, "mods", "old.jar"), []byte("old"), 0644)

	diff := &DiffResult{
		Deleted: []DiffEntry{{RelPath: "mods/old.jar", Status: "deleted"}},
	}

	applied := ApplyDiff(diff, nil, dir, true)
	if applied != 1 {
		t.Errorf("expected 1 applied (dry), got %d", applied)
	}
	// dry-run 不实际删除
	if _, err := os.Stat(filepath.Join(dir, "mods", "old.jar")); os.IsNotExist(err) {
		t.Error("dry-run should not delete files")
	}
}

// mustSHA1 计算字符串的 SHA1 hex
func mustSHA1(t *testing.T, data []byte) string {
	t.Helper()
	h, _, err := computeHashesFromData(data)
	if err != nil {
		t.Fatalf("compute SHA1: %v", err)
	}
	return h
}

// computeHashesFromData 直接计算字节数据的 hash（用于测试）
func computeHashesFromData(data []byte) (string, string, error) {
	// 用临时文件绕一圈
	dir, err := os.MkdirTemp("", "hash-test-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "tmp")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", "", err
	}
	return computeHashes(path)
}
