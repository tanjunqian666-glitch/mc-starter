package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// createPCL2Mock 创建一个模拟 PCL2 exe 文件
// content 中包含 PCL2 特征字符串
func createPCL2Mock(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)

	// PE 文件必须 MZ 头，然后包含 PCL2 特征字符串
	content := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00" +
		"\xb8\x00\x00\x00\x00\x00\x00\x00\x40\x00\x00\x00\x00\x00\x00\x00" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
		"\x00\x00\x00\x00\x80\x00\x00\x00\x0e\x1f\xba\x0e\x00\xb4\x09\xcd" +
		"\x21\xb8\x01\x4c\xcd\x21\x54\x68\x69\x73\x20\x70\x72\x6f\x67\x72" +
		"\x61\x6d\x20\x63\x61\x6e\x6e\x6f\x74\x20\x62\x65\x20\x72\x75\x6e" +
		"\x20\x69\x6e\x20\x44\x4f\x53\x20\x6d\x6f\x64\x65\x2e\x0d\x0d\x0a" +
		"\x24\x00\x00\x00\x00\x00\x00\x00" +
		"Plain Craft Launcher 2 by szszss" +
		"PCL2 internal strings\x00")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("写入 mock exe 失败: %v", err)
	}
	return path
}

// createNonPCLExe 创建一个普通的 exe（不含 PCL2 特征）
func createNonPCLExe(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := []byte("MZ\x90\x00" + "Some other application\x00")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("写入 mock exe 失败: %v", err)
	}
	return path
}

// TestPCLDetectFileName 测试文件名匹配
func TestPCLDetectFileName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pcl-detect-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	createPCL2Mock(t, tmpDir, "Plain Craft Launcher 2.exe")
	createNonPCLExe(t, tmpDir, "notepad.exe")

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)

	if result == nil {
		t.Fatal("应检测到 PCL2")
	}
	if result.Level < 1 {
		t.Errorf("检测层级应 >= 1, 得到 %d", result.Level)
	}
}

// TestPCLDetectAllNames 测试各种命名
func TestPCLDetectAllNames(t *testing.T) {
	names := []string{
		"Plain Craft Launcher 2.exe",
		"PCL2.exe",
		"pcl2.exe",
		"PCL.exe",
		"PCL Community Edition.exe",
		"plaincraftlauncher.exe",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			createPCL2Mock(t, tmpDir, name)

			detector := NewPCLDetector()
			result := detector.detectInDir(tmpDir)
			if result == nil {
				t.Errorf("应检测到 %s", name)
			}
		})
	}
}

// TestPCLDetectSkipNonPCL 测试非 PCL exe 跳过
func TestPCLDetectSkipNonPCL(t *testing.T) {
	tmpDir := t.TempDir()

	createNonPCLExe(t, tmpDir, "notepad.exe")
	createNonPCLExe(t, tmpDir, "java.exe")
	createNonPCLExe(t, tmpDir, "PCL2_Backup.exe") // 含 PCL 但不匹配模式

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)
	if result != nil {
		t.Errorf("非 PCL exe 应跳过, 但检测到: %s", result.Path)
	}
}

// TestPCLDetectWithIni 测试 PCL.ini 检测
func TestPCLDetectWithIni(t *testing.T) {
	tmpDir := t.TempDir()

	createPCL2Mock(t, tmpDir, "PCL2.exe")

	// 创建 PCL.ini
	iniContent := "[Main]\nVersion=2.8.6.0\nLastCheckUpdate=\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "PCL.ini"), []byte(iniContent), 0644); err != nil {
		t.Fatalf("写入 PCL.ini 失败: %v", err)
	}

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)
	if result == nil {
		t.Fatal("应检测到 PCL2")
	}
	if result.Version != "2.8.6.0" {
		t.Errorf("版本应为 2.8.6.0, 得到 %s", result.Version)
	}
	if result.Level < 3 {
		t.Errorf("PCL.ini 存在时检测层级应 >= 3, 得到 %d", result.Level)
	}
	if !result.PCLIniExists() {
		t.Error("PCLIniExists 应返回 true")
	}
}

// TestPCLDetectPortable 测试便携版
func TestPCLDetectPortable(t *testing.T) {
	tmpDir := t.TempDir()

	createPCL2Mock(t, tmpDir, "PCL2.exe")

	// 创建 PCLPortable.ini
	iniContent := "[Main]\nVersion=2.8.6.0\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "PCLPortable.ini"), []byte(iniContent), 0644); err != nil {
		t.Fatalf("写入 PCLPortable.ini 失败: %v", err)
	}

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)
	if result == nil {
		t.Fatal("应检测到 PCL2")
	}
	if !result.IsPortable {
		t.Error("便携版应标记为 portable")
	}
}

// TestPCLDetectHashWhitelist 测试 hash 白名单
func TestPCLDetectHashWhitelist(t *testing.T) {
	tmpDir := t.TempDir()

	mockPath := createPCL2Mock(t, tmpDir, "PCL2.exe")

	detector := NewPCLDetector()

	// 计算 mock 文件的 hash 并加入白名单
	data, _ := os.ReadFile(mockPath)
	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])
	detector.AddKnownHash(hash, "2.8.6.0")

	result := detector.detectInDir(tmpDir)
	if result == nil {
		t.Fatal("应检测到 PCL2")
	}
	if result.Hash != hash {
		t.Errorf("hash 不匹配: 期望 %s, 得到 %s", hash, result.Hash)
	}
	if result.Level < 4 {
		t.Errorf("hash 匹配时检测层级应 >= 4, 得到 %d", result.Level)
	}
}

// TestPCLDetectAll 测试检测全部
func TestPCLDetectAll(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	createPCL2Mock(t, dir1, "PCL2.exe")
	createPCL2Mock(t, dir2, "Plain Craft Launcher 2.exe")

	detector := NewPCLDetector()
	results := detector.DetectAll([]string{dir1, dir2})

	if len(results) != 2 {
		t.Errorf("期望 2 个结果, 得到 %d", len(results))
	}
}

// TestPCLDetectNotFound 测试未找到
func TestPCLDetectNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)
	if result != nil {
		t.Errorf("空目录应返回 nil, 得到 %v", result)
	}
}

// TestPCLDetectNonExistentDir 测试不存在的目录
func TestPCLDetectNonExistentDir(t *testing.T) {
	detector := NewPCLDetector()
	result := detector.detectInDir("/nonexistent/path/for/sure")
	if result != nil {
		t.Errorf("不存在的目录应返回 nil, 得到 %v", result)
	}
}

// TestPCLDetectValidate 测试 Validate 方法
func TestPCLDetectValidate(t *testing.T) {
	tmpDir := t.TempDir()

	pclPath := createPCL2Mock(t, tmpDir, "PCL2.exe")
	nonPCLPath := createNonPCLExe(t, tmpDir, "something.exe")

	detector := NewPCLDetector()
	if !detector.Validate(pclPath) {
		t.Error("PCL exe 应验证通过")
	}
	if detector.Validate(nonPCLPath) {
		t.Error("非 PCL exe 应验证失败")
	}
}

// TestPCLDetectionString 测试 String 方法
func TestPCLDetectionString(t *testing.T) {
	tmpDir := t.TempDir()

	createPCL2Mock(t, tmpDir, "PCL2.exe")
	iniContent := "[Main]\nVersion=2.8.6.0\n"
	os.WriteFile(filepath.Join(tmpDir, "PCL.ini"), []byte(iniContent), 0644)

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)

	if result == nil {
		t.Fatal("应检测到 PCL2")
	}

	summary := result.String()
	if summary == "" {
		t.Error("String 应返回非空字符串")
	}
}

// TestPCLDetectExeWithoutMZ 测试不含 MZ 头的文件
func TestPCLDetectExeWithoutMZ(t *testing.T) {
	tmpDir := t.TempDir()

	// 文件名匹配但不含 MZ 头
	path := filepath.Join(tmpDir, "PCL2.exe")
	content := []byte("not a PE file but named PCL2.exe")
	os.WriteFile(path, content, 0644)

	detector := NewPCLDetector()
	result := detector.detectInDir(tmpDir)
	if result == nil {
		t.Fatal("文件名匹配就应返回结果")
	}
	// 但 level 应该是 1（仅文件名）
	if result.Level != 1 {
		t.Errorf("无 PE 特征时 level 应为 1, 得到 %d", result.Level)
	}
}

// TestPCLFindPCLIni 测试 findPCLIni 方法
func TestPCLFindPCLIni(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 PCL.ini
	iniPath := filepath.Join(tmpDir, "PCL.ini")
	os.WriteFile(iniPath, []byte("[Main]"), 0644)

	detector := NewPCLDetector()
	found := detector.findPCLIni(tmpDir)
	if found == "" {
		t.Error("应找到 PCL.ini")
	}
}

// TestPCLReadPCLIni 测试 readPCLIni 方法
func TestPCLReadPCLIni(t *testing.T) {
	detector := NewPCLDetector()

	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "PCL.ini")
	content := "[Main]\nVersion=2.8.6.0\nLastCheckUpdate=\n"
	os.WriteFile(iniPath, []byte(content), 0644)

	version, portable := detector.readPCLIni(iniPath)
	if version != "2.8.6.0" {
		t.Errorf("版本应为 2.8.6.0, 得到 %s", version)
	}
	if portable {
		t.Error("PCL.ini 不应标记 portable")
	}

	// 测试便携版
	portableIni := filepath.Join(tmpDir, "PCLPortable.ini")
	os.WriteFile(portableIni, []byte("[Main]\nVersion=2.8.6.0\n"), 0644)
	version, portable = detector.readPCLIni(portableIni)
	if version != "2.8.6.0" {
		t.Errorf("版本应为 2.8.6.0, 得到 %s", version)
	}
	if !portable {
		t.Error("PCLPortable.ini 应标记 portable")
	}
}
