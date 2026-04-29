package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestMC(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// 创建测试数据
	mods := filepath.Join(dir, "mods")
	config := filepath.Join(dir, "config")
	saves := filepath.Join(dir, "saves")
	os.MkdirAll(mods, 0755)
	os.MkdirAll(config, 0755)
	os.MkdirAll(saves, 0755)

	// 放几个文件
	os.WriteFile(filepath.Join(mods, "sodium.jar"), []byte("fake sodium"), 0644)
	os.WriteFile(filepath.Join(mods, "lithium.jar"), []byte("fake lithium"), 0644)
	os.WriteFile(filepath.Join(config, "sodium-mixins.properties"), []byte("mixins=true"), 0644)
	os.WriteFile(filepath.Join(saves, "world1", "level.dat"), []byte("fake world"), 0644)
	os.WriteFile(filepath.Join(dir, "options.txt"), []byte("renderDistance:12"), 0644)

	// 空目录也测试
	os.MkdirAll(filepath.Join(dir, "resourcepacks"), 0755)
	os.MkdirAll(filepath.Join(dir, "shaderpacks"), 0755)
	os.MkdirAll(filepath.Join(dir, "screenshots"), 0755)

	return dir
}

func TestCreateBackup(t *testing.T) {
	mcDir := setupTestMC(t)

	opts := BackupOptions{
		Reason:    ReasonRepair,
		MCVersion: "1.20.1",
		MaxKeep:   10,
	}

	result, err := CreateBackup(mcDir, opts)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if result.FileCount == 0 {
		t.Error("expected >0 files backed up")
	}
	if result.SizeBytes <= 0 {
		t.Error("expected >0 bytes backed up")
	}

	// 验证备份目录存在
	backupDir := filepath.Join(mcDir, "starter_backups")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Fatal("backup dir not created")
	}

	// 验证 meta.json
	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	metaPath := filepath.Join(backupDir, entries[0].Name(), "meta.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Fatal("meta.json not created")
	}

	// 验证 options.txt 被备份
	optsBackup := filepath.Join(backupDir, entries[0].Name(), "options.txt")
	if _, err := os.Stat(optsBackup); os.IsNotExist(err) {
		t.Error("options.txt not backed up")
	}
}

func TestCreateBackupEmptyMods(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "options.txt"), []byte("test"), 0644)
	// 没有 mods 目录

	opts := BackupOptions{Reason: ReasonManual}
	result, err := CreateBackup(dir, opts)
	if err != nil {
		t.Fatalf("CreateBackup with missing mods failed: %v", err)
	}
	if result.FileCount <= 0 {
		t.Error("expected at least config and options.txt backed up")
	}
}

func TestListBackups(t *testing.T) {
	mcDir := setupTestMC(t)
	cleanup := filepath.Join(mcDir, "starter_backups")

	// 创建两个备份，用不同的 meta 写不同 reason
	// 第一个备份：手动创建 meta
	backupRoot := filepath.Join(mcDir, BackupDirName)
	os.MkdirAll(backupRoot, 0755)

	d1 := filepath.Join(backupRoot, "backup_20260429_010000")
	os.MkdirAll(d1, 0755)
	meta1 := BackupMeta{ID: "20260429_010000", CreatedAt: time.Now().Add(-2 * time.Hour), Reason: ReasonRepair, FileCount: 3}
	b1, _ := json.MarshalIndent(meta1, "", "  ")
	os.WriteFile(filepath.Join(d1, "meta.json"), b1, 0644)

	d2 := filepath.Join(backupRoot, "backup_20260429_020000")
	os.MkdirAll(d2, 0755)
	meta2 := BackupMeta{ID: "20260429_020000", CreatedAt: time.Now().Add(-1 * time.Hour), Reason: ReasonManual, FileCount: 5}
	b2, _ := json.MarshalIndent(meta2, "", "  ")
	os.WriteFile(filepath.Join(d2, "meta.json"), b2, 0644)

	_ = cleanup
	backups, err := ListBackups(mcDir)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}

	// 最新在前
	if backups[0].Reason != ReasonManual {
		t.Errorf("expected latest reason=manual, got %s", backups[0].Reason)
	}
}

func TestRestoreBackup(t *testing.T) {
	mcDir := setupTestMC(t)

	// 先手动创建 meta 而不是走 CreateBackup，避免恢复前自动备份干扰
	backupRoot := filepath.Join(mcDir, BackupDirName)
	os.MkdirAll(backupRoot, 0755)

	backupID := "20260429_test00"
	backupPath := filepath.Join(backupRoot, "backup_"+backupID)
	os.MkdirAll(filepath.Join(backupPath, "mods"), 0755)
	os.WriteFile(filepath.Join(backupPath, "mods", "sodium.jar"), []byte("fake sodium"), 0644)
	os.WriteFile(filepath.Join(backupPath, "mods", "lithium.jar"), []byte("fake lithium"), 0644)
	os.MkdirAll(filepath.Join(backupPath, "config"), 0755)
	os.WriteFile(filepath.Join(backupPath, "config", "sodium-mixins.properties"), []byte("mixins=true"), 0644)
	os.WriteFile(filepath.Join(backupPath, "options.txt"), []byte("renderDistance:12"), 0644)
	meta := BackupMeta{ID: backupID, CreatedAt: time.Now(), Reason: ReasonRepair, FileCount: 4}
	b, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(backupPath, "meta.json"), b, 0644)

	// 修改原文件（模拟修复前的损坏）
	os.WriteFile(filepath.Join(mcDir, "mods", "sodium.jar"), []byte("corrupted"), 0644)
	os.Remove(filepath.Join(mcDir, "config", "sodium-mixins.properties"))

	// 恢复
	restored, err := RestoreBackup(mcDir, backupID)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if restored <= 0 {
		t.Error("expected >0 files restored")
	}

	// 验证文件已恢复
	data, err := os.ReadFile(filepath.Join(mcDir, "mods", "sodium.jar"))
	if err != nil {
		t.Fatal("sodium.jar not restored")
	}
	if string(data) != "fake sodium" {
		t.Errorf("expected 'fake sodium', got '%s'", string(data))
	}
}

func TestDeleteBackup(t *testing.T) {
	mcDir := setupTestMC(t)

	CreateBackup(mcDir, BackupOptions{Reason: ReasonManual, MaxKeep: 10})

	backups, _ := ListBackups(mcDir)
	if len(backups) != 1 {
		t.Fatal("expected 1 backup")
	}

	err := DeleteBackup(mcDir, backups[0].ID)
	if err != nil {
		t.Fatalf("DeleteBackup failed: %v", err)
	}

	backups, _ = ListBackups(mcDir)
	if len(backups) != 0 {
		t.Error("expected 0 backups after delete")
	}
}

func TestCleanupOldBackups(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(dir, "starter_backups")
	os.MkdirAll(backupRoot, 0755)

	// 创建 7 个备份
	for i := 0; i < 7; i++ {
		name := filepath.Join(backupRoot, fmt.Sprintf("backup_202604%02d_%02d0000", 20+i, 10+i))
		os.MkdirAll(name, 0755)
		os.WriteFile(filepath.Join(name, "meta.json"), []byte("{}"), 0644)
		os.WriteFile(filepath.Join(name, "placeholder.txt"), []byte("x"), 0644)
	}

	removed := cleanupOldBackups(backupRoot, 5)
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// 验证只剩下 5 个
	entries, _ := os.ReadDir(backupRoot)
	backupDirs := 0
	for _, e := range entries {
		if e.IsDir() {
			backupDirs++
		}
	}
	if backupDirs != 5 {
		t.Errorf("expected 5 backup dirs, got %d", backupDirs)
	}
}

func TestGetBackupSizeTotal(t *testing.T) {
	mcDir := setupTestMC(t)

	CreateBackup(mcDir, BackupOptions{Reason: ReasonManual, MaxKeep: 10})

	size := GetBackupSizeTotal(mcDir)
	if size <= 0 {
		t.Error("expected >0 backup total size")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "sub", "dst.txt")

	os.WriteFile(src, []byte("hello world"), 0644)

	err := copyFile(src, dst)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal("dst file not found")
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(data))
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 100), 0644)
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "b.txt"), make([]byte, 200), 0644)

	size := dirSize(dir)
	if size != 300 {
		t.Errorf("expected 300, got %d", size)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	mcDir := setupTestMC(t)

	// 手动创建备份目录（避开 CreateBackup 的时间戳）
	backupRoot := filepath.Join(mcDir, BackupDirName)
	os.MkdirAll(backupRoot, 0755)

	backupID := "roundtrip_0001"
	backupPath := filepath.Join(backupRoot, "backup_"+backupID)
	os.MkdirAll(filepath.Join(backupPath, "mods"), 0755)
	os.WriteFile(filepath.Join(backupPath, "mods", "sodium.jar"), []byte("fake sodium"), 0644)
	os.WriteFile(filepath.Join(backupPath, "mods", "lithium.jar"), []byte("fake lithium"), 0644)
	os.MkdirAll(filepath.Join(backupPath, "config"), 0755)
	os.WriteFile(filepath.Join(backupPath, "config", "sodium-mixins.properties"), []byte("mixins=true"), 0644)
	os.WriteFile(filepath.Join(backupPath, "options.txt"), []byte("renderDistance:12"), 0644)
	meta := BackupMeta{ID: backupID, CreatedAt: time.Now(), Reason: ReasonRepair, FileCount: 4}
	b, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(backupPath, "meta.json"), b, 0644)

	// 修改文件
	os.WriteFile(filepath.Join(mcDir, "mods", "sodium.jar"), []byte("corrupted"), 0644)

	// 恢复
	_, err := RestoreBackup(mcDir, backupID)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// 验证文件已恢复
	data, err := os.ReadFile(filepath.Join(mcDir, "mods", "sodium.jar"))
	if err != nil {
		t.Fatal("sodium.jar not found after restore")
	}
	if string(data) != "fake sodium" {
		t.Errorf("expected 'fake sodium', got '%s'", string(data))
	}
}
