package launcher

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMavenCoords(t *testing.T) {
	tests := []struct {
		name     string
		expected *MavenArtifact
	}{
		{"org.lwjgl:lwjgl:3.3.1", &MavenArtifact{Group: "org.lwjgl", Artifact: "lwjgl", Version: "3.3.1", Extension: "jar"}},
		{"com.mojang:patchy:2.2.10", &MavenArtifact{Group: "com.mojang", Artifact: "patchy", Version: "2.2.10", Extension: "jar"}},
		{"org.lwjgl:lwjgl-platform:3.3.1:natives-windows", &MavenArtifact{Group: "org.lwjgl", Artifact: "lwjgl-platform", Version: "3.3.1", Classifier: "natives-windows", Extension: "jar"}},
		{"invalid", nil},
		{"too:few", nil},
	}

	for _, tt := range tests {
		got := ParseMavenCoords(tt.name)
		if tt.expected == nil {
			if got != nil {
				t.Errorf("ParseMavenCoords(%q) = %+v, want nil", tt.name, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("ParseMavenCoords(%q) = nil, want %+v", tt.name, *tt.expected)
			continue
		}
		if got.Group != tt.expected.Group || got.Artifact != tt.expected.Artifact ||
			got.Version != tt.expected.Version || got.Classifier != tt.expected.Classifier {
			t.Errorf("ParseMavenCoords(%q) = %+v, want %+v", tt.name, *got, *tt.expected)
		}
	}
}

func TestMavenURL(t *testing.T) {
	coords := &MavenArtifact{Group: "org.lwjgl", Artifact: "lwjgl", Version: "3.3.1"}
	got := MavenURL("https://libraries.minecraft.net", coords)
	want := "https://libraries.minecraft.net/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar"
	if got != want {
		t.Errorf("MavenURL = %q, want %q", got, want)
	}

	// 带 classifier
	coords2 := &MavenArtifact{Group: "org.lwjgl", Artifact: "lwjgl-platform", Version: "3.3.1", Classifier: "natives-windows"}
	got2 := MavenURL("https://libraries.minecraft.net", coords2)
	want2 := "https://libraries.minecraft.net/org/lwjgl/lwjgl-platform/3.3.1/lwjgl-platform-3.3.1-natives-windows.jar"
	if got2 != want2 {
		t.Errorf("MavenURL(classifier) = %q, want %q", got2, want2)
	}
}

func TestShouldInclude(t *testing.T) {
	tests := []struct {
		name  string
		rules []Rule
		want  bool
	}{
		{"空 rules", nil, true},
		{"空数组", []Rule{}, true},
		{"仅 allow 无 OS", []Rule{{Action: "allow"}}, true},
		{"仅 disallow 无 OS", []Rule{{Action: "disallow"}}, false},
		{"allow windows", []Rule{{Action: "allow", OS: &OSRule{Name: "windows"}}}, true},
		{"disallow windows", []Rule{{Action: "disallow", OS: &OSRule{Name: "windows"}}}, false},
		{"allow linux", []Rule{{Action: "allow", OS: &OSRule{Name: "linux"}}}, false},
		{"allow osx", []Rule{{Action: "allow", OS: &OSRule{Name: "osx"}}}, false},

		// 增强测试：features 标签
		{"allow + is_demo_user=true（仅允许 Demo 用户，我们不是）", []Rule{{Action: "allow", Features: &RuleFeatures{IsDemoUser: boolPtr(true)}}}, false},

		// allow 和 disallow 组合
		{"allow windows + allow osx", []Rule{
			{Action: "allow", OS: &OSRule{Name: "windows"}},
			{Action: "allow", OS: &OSRule{Name: "osx"}},
		}, true},
		{"allow all + disallow linux", []Rule{
			{Action: "allow"},
			{Action: "disallow", OS: &OSRule{Name: "linux"}},
		}, true},
		{"allow all + disallow windows", []Rule{
			{Action: "allow"},
			{Action: "disallow", OS: &OSRule{Name: "windows"}},
		}, false},
		{"allow windows + disallow all", []Rule{
			{Action: "allow", OS: &OSRule{Name: "windows"}},
			{Action: "disallow", OS: &OSRule{Name: "windows"}},
		}, false}, // disallow 优先
	}

	for _, tt := range tests {
		got := ShouldInclude(tt.rules)
		if got != tt.want {
			t.Errorf("ShouldInclude(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestMavenLocalPath(t *testing.T) {
	dir := t.TempDir()
	m := NewLibraryManager(dir, filepath.Join(dir, "natives"))

	coords := &MavenArtifact{Group: "org.lwjgl", Artifact: "lwjgl", Version: "3.3.1"}
	got := m.MavenLocalPath(coords)
	want := filepath.Join(dir, "org", "lwjgl", "lwjgl", "3.3.1", "lwjgl-3.3.1.jar")
	if got != want {
		t.Errorf("MavenLocalPath = %q, want %q", got, want)
	}
}

func TestExtractNatives(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "test.jar")

	// 创建一个测试 JAR（实际 ZIP）
	zipFile, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(zipFile)

	// 添加一个 native dll 文件
	dllContent := []byte("fake dll content")
	dllWriter, err := writer.Create("natives/test.dll")
	if err != nil {
		t.Fatal(err)
	}
	dllWriter.Write(dllContent)

	// 添加一个非 native 文件（应被跳过）
	textWriter, err := writer.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	textWriter.Write([]byte("Manifest-Version: 1.0"))

	writer.Close()
	zipFile.Close()

	// 解压 natives
	nativesDir := filepath.Join(dir, "natives_out")
	if err := extractNatives(jarPath, nativesDir); err != nil {
		t.Fatalf("extractNatives 失败: %v", err)
	}

	// 验证 .dll 被提取
	dllOut := filepath.Join(nativesDir, "test.dll")
	if _, err := os.Stat(dllOut); os.IsNotExist(err) {
		t.Error("natives 中的 .dll 未提取")
	}

	// 验证 META-INF 没有被提取
	metaOut := filepath.Join(nativesDir, "MANIFEST.MF")
	if _, err := os.Stat(metaOut); err == nil {
		t.Error("META-INF 不应该被提取")
	}
}

func TestBuildClasspath(t *testing.T) {
	libs := []string{
		"/libraries/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar",
		"/libraries/com/mojang/patchy/2.2.10/patchy-2.2.10.jar",
	}
	clientJar := "/jars/1.20.4.jar"

	cp := BuildClasspath(libs, clientJar)
	expectedSep := string(filepath.ListSeparator)

	if !stringsContains(cp, "lwjgl-3.3.1.jar") {
		t.Error("classpath 应包含 lwjgl")
	}
	if !stringsContains(cp, "1.20.4.jar") {
		t.Error("classpath 应包含 client.jar")
	}
	if !stringsContains(cp, string(expectedSep)) {
		t.Errorf("classpath 应包含分隔符 %q", expectedSep)
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNormalizeOS(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"windows", "windows"},
		{"Windows", "windows"},
		{"win", "windows"},
		{"osx", "osx"},
		{"mac", "osx"},
		{"macOS", "osx"},
		{"linux", "linux"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := normalizeOS(tt.input)
		if got != tt.want {
			t.Errorf("normalizeOS(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
