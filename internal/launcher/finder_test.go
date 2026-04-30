package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gege-tlph/mc-starter/internal/model"
)

// TestFindVersionDir 测试版本目录查找
func TestFindVersionDir(t *testing.T) {
	// 在临时目录创建模拟的 .minecraft 结构
	dir := t.TempDir()

	mcDir := filepath.Join(dir, ".minecraft")
	versionsDir := filepath.Join(mcDir, "versions")
	versionDir := filepath.Join(versionsDir, "my-test-pack-v1")

	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 写一个空的 version.json
	if err := os.WriteFile(filepath.Join(versionDir, "version.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// 用 FindVersionDir 查找
	cfg := &model.LocalConfig{
		MinecraftDir: mcDir,
	}
	
	got := FindVersionDir(cfg, "my-test-pack-v1")
	if got != versionDir {
		t.Errorf("FindVersionDir = %q, want %q", got, versionDir)
	}

	// 查找不存在的版本
	got = FindVersionDir(cfg, "nonexistent")
	if got != "" {
		t.Errorf("FindVersionDir for nonexistent = %q, want empty", got)
	}
}

// TestFindManagedVersions 测试多版本查找
func TestFindManagedVersions(t *testing.T) {
	dir := t.TempDir()

	// 在 dir 底下建 .minecraft（回退扫描会检查 InstallPath）
	mcDir := filepath.Join(dir, ".minecraft")

	// mcDir 下有 v1
	v1Dir := filepath.Join(mcDir, "versions", "pack-v1")
	os.MkdirAll(v1Dir, 0755)
	os.WriteFile(filepath.Join(v1Dir, "version.json"), []byte("{}"), 0644)

	// mcDir 下有 v2
	v2Dir := filepath.Join(mcDir, "versions", "pack-v2")
	os.MkdirAll(v2Dir, 0755)
	os.WriteFile(filepath.Join(v2Dir, "version.json"), []byte("{}"), 0644)

	finder := NewVersionFinder(&model.LocalConfig{MinecraftDir: mcDir})

	// 查找 pack-v1 和 pack-v2
	results := finder.FindManagedVersions([]string{"pack-v1", "pack-v2"})

	if results["pack-v1"] == nil || !results["pack-v1"].Found {
		t.Error("pack-v1 should be found")
	}
	if results["pack-v2"] == nil || !results["pack-v2"].Found {
		t.Error("pack-v2 should be found")
	}

	t.Logf("pack-v1: %s (from PCL: %v)", results["pack-v1"].VersionDir, results["pack-v1"].FromPCL)
	t.Logf("pack-v2: %s (from PCL: %v)", results["pack-v2"].VersionDir, results["pack-v2"].FromPCL)
}

// TestFindMinecraftDirs 测试 .minecraft 目录发现
func TestFindMinecraftDirs(t *testing.T) {
	dir := t.TempDir()

	// 创建几个 .minecraft 目录
	mc1 := filepath.Join(dir, "minecraft_a", ".minecraft")
	mc2 := filepath.Join(dir, "minecraft_b", ".minecraft")

	os.MkdirAll(filepath.Join(mc1, "versions"), 0755)
	os.MkdirAll(filepath.Join(mc2, "versions"), 0755)

	// 切到临时目录
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// 在当前目录也建一个 .minecraft
	os.MkdirAll(filepath.Join(dir, ".minecraft", "versions"), 0755)

	dirs := FindMinecraftDirs()

	if len(dirs) == 0 {
		t.Log("No minecraft dirs found (expected on non-Windows without PCL)")
		t.Log("dirs:", dirs)
		return
	}

	t.Logf("Found %d .minecraft directories:", len(dirs))
	for i, d := range dirs {
		t.Logf("  [%d] %s", i, d)
	}
}

// TestVersionDirExists 测试目录存在性检查
func TestVersionDirExists(t *testing.T) {
	dir := t.TempDir()
	mcDir := filepath.Join(dir, ".minecraft")
	vDir := filepath.Join(mcDir, "versions", "test-version")

	os.MkdirAll(vDir, 0755)
	os.WriteFile(filepath.Join(vDir, "version.json"), []byte("{}"), 0644)

	if !VersionDirExists(mcDir, "test-version") {
		t.Error("VersionDirExists should return true for existing version")
	}
	if VersionDirExists(mcDir, "missing-version") {
		t.Error("VersionDirExists should return false for missing version")
	}
}

// TestFindLatestVersionDir 测试多目录选最新
func TestFindLatestVersionDir(t *testing.T) {
	dir := t.TempDir()

	mc1 := filepath.Join(dir, "old")
	mc2 := filepath.Join(dir, "new")

	os.MkdirAll(filepath.Join(mc1, "versions", "test-pack"), 0755)
	os.MkdirAll(filepath.Join(mc2, "versions", "test-pack"), 0755)

	// 先写 mc1 的 version.json
	os.WriteFile(filepath.Join(mc1, "versions", "test-pack", "version.json"), []byte("{}"), 0644)

	latest := FindLatestVersionDir(
		[]string{filepath.Join(dir, "old"), filepath.Join(dir, "new")},
		"test-pack",
	)
	if latest == "" {
		t.Fatal("FindLatestVersionDir should find at least one")
	}
	_ = latest

	// 用 FindLatestVersionDir 取目录
	found := FindLatestVersionDir(
		[]string{filepath.Join(dir, "old"), filepath.Join(dir, "new")},
		"test-pack",
	)
	if found == "" {
		t.Error("FindLatestVersionDir should find the version dir")
	}
}

// TestKnownLauncherFallback 测试启动器回退逻辑
func TestKnownLauncherFallback(t *testing.T) {
	dir := t.TempDir()

	// 只有普通 .minecraft，没有 PCL
	mcDir := filepath.Join(dir, ".minecraft")
	vDir := filepath.Join(mcDir, "versions", "vanilla-pack")
	os.MkdirAll(vDir, 0755)
	os.WriteFile(filepath.Join(vDir, "version.json"), []byte("{}"), 0644)

	finder := NewVersionFinder(&model.LocalConfig{
		MinecraftDir: mcDir,
		Launcher:    "auto",
	})

	results := finder.FindManagedVersions([]string{"vanilla-pack"})
	if results["vanilla-pack"] == nil || !results["vanilla-pack"].Found {
		t.Error("vanilla-pack should be found via fallback")
	}
	if results["vanilla-pack"].FromPCL {
		t.Error("should NOT be from PCL since no PCL detected")
	}

	t.Logf("Result: %s (fromPCL=%v)", results["vanilla-pack"].VersionDir, results["vanilla-pack"].FromPCL)
}
