package repair

import (
	"os"
	"path/filepath"
	"testing"
)

func setupRepairTestMC(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mods := filepath.Join(dir, "mods")
	config := filepath.Join(dir, "config")
	saves := filepath.Join(dir, "saves")
	os.MkdirAll(mods, 0755)
	os.MkdirAll(config, 0755)
	os.MkdirAll(saves, 0755)

	os.WriteFile(filepath.Join(mods, "sodium.jar"), []byte("fake sodium"), 0644)
	os.WriteFile(filepath.Join(mods, "lithium.jar"), []byte("fake lithium"), 0644)
	os.WriteFile(filepath.Join(config, "sodium-mixins.properties"), []byte("mixins=true"), 0644)
	os.WriteFile(filepath.Join(dir, "options.txt"), []byte("renderDistance:12"), 0644)

	return dir
}

func TestRepairCleanAll(t *testing.T) {
	mcDir := setupRepairTestMC(t)

	cfg := RepairConfig{Action: ActionCleanAll}
	result, err := Repair(mcDir, cfg)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	if result.Action != ActionCleanAll {
		t.Errorf("expected ActionCleanAll, got %s", result.Action)
	}
	if result.BackupDir == "" {
		t.Error("expected backup dir to be set")
	}

	// 验证 mods 和 config 已被清空
	entries, _ := os.ReadDir(filepath.Join(mcDir, "mods"))
	if len(entries) != 0 {
		t.Errorf("expected mods/ to be empty, got %d entries", len(entries))
	}
	entries, _ = os.ReadDir(filepath.Join(mcDir, "config"))
	if len(entries) != 0 {
		t.Errorf("expected config/ to be empty, got %d entries", len(entries))
	}

	// 验证 cleaned dirs
	cleaned := make(map[string]bool)
	for _, d := range result.CleanedDirs {
		cleaned[d] = true
	}
	if !cleaned["mods"] || !cleaned["config"] || !cleaned["resourcepacks"] {
		t.Error("expected mods, config, resourcepacks in cleaned dirs")
	}
}

func TestRepairModsOnly(t *testing.T) {
	mcDir := setupRepairTestMC(t)

	_, err := Repair(mcDir, RepairConfig{Action: ActionModsOnly})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	// mods 被清空
	entries, _ := os.ReadDir(filepath.Join(mcDir, "mods"))
	if len(entries) != 0 {
		t.Errorf("expected mods/ to be empty, got %d entries", len(entries))
	}

	// config 没有被清理
	entries, _ = os.ReadDir(filepath.Join(mcDir, "config"))
	if len(entries) == 0 {
		t.Error("expected config/ to not be cleaned")
	}
}

func TestRepairConfigOnly(t *testing.T) {
	mcDir := setupRepairTestMC(t)

	_, err := Repair(mcDir, RepairConfig{Action: ActionConfigOnly})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	// config 被清空
	entries, _ := os.ReadDir(filepath.Join(mcDir, "config"))
	if len(entries) != 0 {
		t.Errorf("expected config/ to be empty, got %d entries", len(entries))
	}

	// mods 没有被清理
	entries, _ = os.ReadDir(filepath.Join(mcDir, "mods"))
	if len(entries) == 0 {
		t.Error("expected mods/ to not be cleaned")
	}
}

func TestRepairRollback(t *testing.T) {
	mcDir := setupRepairTestMC(t)

	// 先创建一个备份
	CreateBackup(mcDir, BackupOptions{Reason: ReasonRepair, MaxKeep: 10})

	// 修改文件
	os.WriteFile(filepath.Join(mcDir, "mods", "sodium.jar"), []byte("corrupted"), 0644)

	// 回滚
	result, err := Repair(mcDir, RepairConfig{Action: ActionRollback})
	if err != nil {
		t.Fatalf("Repair rollback failed: %v", err)
	}

	if result.Action != ActionRollback {
		t.Errorf("expected ActionRollback, got %s", result.Action)
	}
	if result.Restored <= 0 {
		t.Error("expected >0 files restored")
	}

	// 验证文件被恢复
	data, err := os.ReadFile(filepath.Join(mcDir, "mods", "sodium.jar"))
	if err != nil {
		t.Fatal("sodium.jar not found after rollback")
	}
	if string(data) != "fake sodium" {
		t.Errorf("expected 'fake sodium', got '%s'", string(data))
	}
}

func TestRepairRollbackEmpty(t *testing.T) {
	_, err := Repair(t.TempDir(), RepairConfig{Action: ActionRollback})
	if err != nil {
		t.Fatalf("Repair rollback on empty dir failed: %v", err)
	}
}

func TestIsRepairDir(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"mods", true},
		{"config", true},
		{"resourcepacks", true},
		{"shaderpacks", true},
		{"saves", false},
		{"versions", false},
		{"assets", false},
		{"libraries", false},
		{"natives", false},
	}
	for _, tt := range tests {
		got := IsRepairDir(tt.name)
		if got != tt.want {
			t.Errorf("IsRepairDir(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCleanDir(t *testing.T) {
	dir := t.TempDir()
	testDir := filepath.Join(dir, "testdata")
	os.MkdirAll(testDir, 0755)
	os.WriteFile(filepath.Join(testDir, "a.txt"), []byte("a"), 0644)

	if err := cleanDir(testDir); err != nil {
		t.Fatalf("cleanDir failed: %v", err)
	}

	entries, _ := os.ReadDir(testDir)
	if len(entries) != 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}

	// 清理不存在的目录应成功
	if err := cleanDir(filepath.Join(dir, "notexist")); err != nil {
		t.Fatalf("cleanDir on non-existent failed: %v", err)
	}
}

func TestRepairListBackups(t *testing.T) {
	mcDir := setupRepairTestMC(t)

	CreateBackup(mcDir, BackupOptions{Reason: ReasonRepair, MaxKeep: 10})

	result, err := Repair(mcDir, RepairConfig{Action: ActionListBackups})
	if err != nil {
		t.Fatalf("Repair list-backups failed: %v", err)
	}
	if result.Action != ActionListBackups {
		t.Errorf("expected ActionListBackups, got %s", result.Action)
	}
}

func TestRepairEmptyMCNoCrash(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mods"), 0755)

	result, err := Repair(dir, RepairConfig{Action: ActionCleanAll})
	if err != nil {
		t.Fatalf("Repair on minimal mc dir failed: %v", err)
	}
	if result.BackupDir == "" {
		t.Error("expected backup to be created even with minimal content")
	}
}
