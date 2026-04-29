package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
// Fabric 安装器单元测试
// ──────────────────────────────────────────────

// TestParseFabricProfile 测试 Fabric profile JSON 能否解析为 VersionMeta
//
// Fabric profile JSON 格式和 MC version.json 一致，只是字段更简单：
//   - 有 id / inheritsFrom / mainClass
//   - libraries 使用 name+url 格式（无 downloads.artifact）
//   - 没有 assets / downloads / arguments（部分有 arguments.jvm）
func TestParseFabricProfile(t *testing.T) {
	// 模拟 Fabric profile JSON（基于 1.20.4 + loader 0.19.2 的实际数据简化）
	profileJSON := `{
		"id": "fabric-loader-0.19.2-1.20.4",
		"inheritsFrom": "1.20.4",
		"releaseTime": "2026-04-29T05:07:55+0000",
		"time": "2026-04-29T05:07:55+0000",
		"type": "release",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"arguments": {
			"game": [],
			"jvm": ["-DFabricMcEmu= net.minecraft.client.main.Main "]
		},
		"libraries": [
			{
				"name": "org.ow2.asm:asm:9.9",
				"url": "https://maven.fabricmc.net/"
			},
			{
				"name": "net.fabricmc:fabric-loader:0.19.2",
				"url": "https://maven.fabricmc.net/"
			},
			{
				"name": "net.fabricmc:intermediary:1.20.4",
				"url": "https://maven.fabricmc.net/"
			}
		]
	}`

	var meta VersionMeta
	if err := json.Unmarshal([]byte(profileJSON), &meta); err != nil {
		t.Fatalf("解析 Fabric profile 失败: %v", err)
	}

	if meta.ID != "fabric-loader-0.19.2-1.20.4" {
		t.Errorf("期望 ID fabric-loader-0.19.2-1.20.4, 得到 %s", meta.ID)
	}
	if meta.InheritsFrom != "1.20.4" {
		t.Errorf("期望 inheritsFrom 1.20.4, 得到 %s", meta.InheritsFrom)
	}
	if meta.MainClass != "net.fabricmc.loader.impl.launch.knot.KnotClient" {
		t.Errorf("期望 mainClass KnotClient, 得到 %s", meta.MainClass)
	}
	if len(meta.Libraries) != 3 {
		t.Fatalf("期望 3 个 libraries, 得到 %d", len(meta.Libraries))
	}

	// 验证第一个 library 的 name+url 格式
	lib := meta.Libraries[0]
	if lib.Name != "org.ow2.asm:asm:9.9" {
		t.Errorf("期望 name org.ow2.asm:asm:9.9, 得到 %s", lib.Name)
	}
	if lib.URL != "https://maven.fabricmc.net/" {
		t.Errorf("期望 url https://maven.fabricmc.net/, 得到 %s", lib.URL)
	}

	// 验证 name+url 格式的 library 能被 ResolveToFiles 正确处理
	// 这种格式没有 downloads.artifact，走 lib.URL fallback 路径
	lm := NewLibraryManager("/tmp/libraries", "/tmp/natives")
	files := lm.ResolveToFiles(meta.Libraries)
	if len(files) != 3 {
		t.Fatalf("期望 ResolveToFiles 返回 3 个文件, 得到 %d", len(files))
	}

	// 验证 Maven 路径转换
	asmFile := files[0]
	if !strings.Contains(asmFile.LocalPath, "org/ow2/asm/asm/9.9/asm-9.9.jar") {
		t.Errorf("期望 Maven 路径包含 asm-9.9.jar, 得到 %s", asmFile.LocalPath)
	}
	if !strings.Contains(asmFile.URL, "https://maven.fabricmc.net/") {
		t.Errorf("期望 URL 包含 maven.fabricmc.net, 得到 %s", asmFile.URL)
	}

	// 验证 fabric-loader
	loaderFile := files[1]
	if !strings.Contains(loaderFile.LocalPath, "net/fabricmc/fabric-loader") {
		t.Errorf("期望路径包含 fabric-loader, 得到 %s", loaderFile.LocalPath)
	}

	// 验证 intermediary
	intermediaryFile := files[2]
	if !strings.Contains(intermediaryFile.LocalPath, "net/fabricmc/intermediary") {
		t.Errorf("期望路径包含 intermediary, 得到 %s", intermediaryFile.LocalPath)
	}
}

// TestFabricVersionID 测试版本 ID 生成和判断
func TestFabricVersionID(t *testing.T) {
	tests := []struct {
		loader string
		mc     string
		wantID string
	}{
		{"0.19.2", "1.20.4", "fabric-loader-0.19.2-1.20.4"},
		{"0.15.11", "1.19.4", "fabric-loader-0.15.11-1.19.4"},
		{"0.14.21", "1.20.1", "fabric-loader-0.14.21-1.20.1"},
	}

	for _, tt := range tests {
		id := FabricVersionID(tt.loader, tt.mc)
		if id != tt.wantID {
			t.Errorf("FabricVersionID(%q, %q) = %q, 期望 %q", tt.loader, tt.mc, id, tt.wantID)
		}
		if !IsFabricVersion(id) {
			t.Errorf("IsFabricVersion(%q) = false, 期望 true", id)
		}
	}
}

// TestIsFabricVersion 测试非 Fabric 版本 ID 的判断
func TestIsFabricVersion(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"fabric-loader-0.19.2-1.20.4", true},
		{"1.20.4", false},
		{"1.20.4-fabric", false},
		{"", false},
		{"fabric", false},
		{"fabric-loader-", false}, // 长度 <= 14
	}

	for _, tt := range tests {
		got := IsFabricVersion(tt.id)
		if got != tt.want {
			t.Errorf("IsFabricVersion(%q) = %v, 期望 %v", tt.id, got, tt.want)
		}
	}
}

// TestFabricInstallerProfileWrite 测试 Fabric profile 写入验证
func TestFabricInstallerProfileWrite(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	versionsDir := filepath.Join(tmpDir, "versions")
	_ = NewFabricInstaller("1.20.4", "0.19.2", versionsDir, "/tmp/libraries")

	// 手动写入 profile JSON（模拟 Install 的 Step 3）
	profileData := fmt.Sprintf(`{
		"id": "fabric-loader-0.19.2-1.20.4",
		"inheritsFrom": "1.20.4",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"type": "release",
		"libraries": [
			{"name": "org.ow2.asm:asm:9.9", "url": "https://maven.fabricmc.net/"}
		]
	}`)

	versionDir := filepath.Join(versionsDir, "fabric-loader-0.19.2-1.20.4")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(versionDir, "fabric-loader-0.19.2-1.20.4.json")
	if err := os.WriteFile(profilePath, []byte(profileData), 0644); err != nil {
		t.Fatal(err)
	}

	// 验证文件存在
	gotPath := FabricProfilePath(versionsDir, "fabric-loader-0.19.2-1.20.4")
	if gotPath != profilePath {
		t.Errorf("FabricProfilePath 期望 %s, 得到 %s", profilePath, gotPath)
	}
	if _, err := os.Stat(gotPath); os.IsNotExist(err) {
		t.Errorf("Fabric profile 文件应存在: %s", gotPath)
	}
}

// TestFabricInstallerVerify 测试安装验证
func TestFabricInstallerVerify(t *testing.T) {
	tmpDir := t.TempDir()
	versionsDir := filepath.Join(tmpDir, "versions")
	librariesDir := filepath.Join(tmpDir, "libraries")

	installer := NewFabricInstaller("1.20.4", "0.19.2", versionsDir, librariesDir)

	// 没有 profile JSON 时验证应报缺失
	missing, err := installer.VerifyInstallation("fabric-loader-0.19.2-1.20.4")
	if err != nil {
		t.Fatalf("VerifyInstallation 不应返回 error: %v", err)
	}
	if len(missing) == 0 {
		t.Error("没有 profile JSON 时应返回缺失文件")
	}

	// 写入 profile JSON
	versionDir := filepath.Join(versionsDir, "fabric-loader-0.19.2-1.20.4")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	profileData := `{
		"id": "fabric-loader-0.19.2-1.20.4",
		"inheritsFrom": "1.20.4",
		"mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient",
		"type": "release",
		"libraries": [
			{"name": "org.ow2.asm:asm:9.9", "url": "https://maven.fabricmc.net/"},
			{"name": "net.fabricmc:fabric-loader:0.19.2", "url": "https://maven.fabricmc.net/"}
		]
	}`
	profilePath := filepath.Join(versionDir, "fabric-loader-0.19.2-1.20.4.json")
	if err := os.WriteFile(profilePath, []byte(profileData), 0644); err != nil {
		t.Fatal(err)
	}

	// 无 library 文件时应该报缺失
	missing, err = installer.VerifyInstallation("fabric-loader-0.19.2-1.20.4")
	if err != nil {
		t.Fatalf("VerifyInstallation 不应返回 error: %v", err)
	}
	if len(missing) == 0 {
		t.Error("没有 library 文件时应返回缺失")
	}
	expectedPrefix := []string{"asm", "fabric-loader"}
	for _, prefix := range expectedPrefix {
		found := false
		for _, m := range missing {
			if strings.Contains(m, prefix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("期望缺失文件包含 %s, 但未见: %v", prefix, missing)
		}
	}
}
