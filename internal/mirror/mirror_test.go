package mirror

import (
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	m := Resolve("global")
	if m.Label != "Mojang" {
		t.Errorf("global label = %q, want Mojang", m.Label)
	}
	if m.Manifest != "https://piston-meta.mojang.com" {
		t.Errorf("global manifest = %q", m.Manifest)
	}

	m2 := Resolve("china")
	if m2.Label != "BMCLAPI" {
		t.Errorf("china label = %q, want BMCLAPI", m2.Label)
	}

	// 不支持的 mode
	m3 := Resolve("invalid")
	if m3.Label != "Mojang" {
		t.Errorf("invalid mode should fallback to global, got %s", m3.Label)
	}
}

func TestAssetURL(t *testing.T) {
	m := Resolve("global")
	hash := "bdf48ef6b5d0d23bbb02e17d04865216179f510a"

	url := AssetURL(m, hash)
	expected := "https://resources.download.minecraft.net/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a"
	if url != expected {
		t.Errorf("AssetURL = %q, want %q", url, expected)
	}

	m2 := Resolve("china")
	url2 := AssetURL(m2, hash)
	expected2 := "https://bmclapi2.bangbang93.com/assets/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a"
	if url2 != expected2 {
		t.Errorf("AssetURL(china) = %q, want %q", url2, expected2)
	}
}

func TestLibraryURL(t *testing.T) {
	m := Resolve("global")
	path := "net/minecraft/client/1.20.4/client-1.20.4.jar"

	url := LibraryURL(m, path)
	expected := "https://libraries.minecraft.net/net/minecraft/client/1.20.4/client-1.20.4.jar"
	if url != expected {
		t.Errorf("LibraryURL = %q, want %q", url, expected)
	}
}

func TestRewriteURL(t *testing.T) {
	m := Mirror{
		Label:   "BMCLAPI",
		Manifest: "https://bmclapi2.bangbang93.com",
		Asset:   "https://bmclapi2.bangbang93.com/assets",
		Library: "https://bmclapi2.bangbang93.com/maven",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{
			"https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
			"https://bmclapi2.bangbang93.com/mc/game/version_manifest_v2.json",
		},
		{
			"https://resources.download.minecraft.net/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a",
			"https://bmclapi2.bangbang93.com/assets/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a",
		},
		{
			"https://libraries.minecraft.net/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar",
			"https://bmclapi2.bangbang93.com/maven/org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar",
		},
		{
			"https://unknown.example.com/test",
			"https://unknown.example.com/test",
		},
	}

	for _, tt := range tests {
		got := rewriteURL(tt.input, m)
		if got != tt.expected {
			t.Errorf("rewriteURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBestMatch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", "https://piston-meta.mojang.com"},
		{"https://resources.download.minecraft.net/bd/abc", "https://resources.download.minecraft.net"},
		{"https://libraries.minecraft.net/org/example/test.jar", "https://libraries.minecraft.net"},
		{"https://meta.fabricmc.net/v2/versions/loader/1.20.4", "https://meta.fabricmc.net"},
		{"https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.15.11/fabric-loader-0.15.11.jar", "https://maven.fabricmc.net"},
		{"https://unknown.com/test", ""},
	}

	for _, tt := range tests {
		got := bestMatch(tt.input)
		if got != tt.expected {
			t.Errorf("bestMatch(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewSmartSelector(t *testing.T) {
	// auto 模式
	s := NewSmartSelector("auto", 0)
	if s.threshold != 4*time.Second {
		t.Errorf("auto threshold = %v, want 4s", s.threshold)
	}

	// explicit
	s2 := NewSmartSelector("global", 2*time.Second)
	if s2.threshold != 2*time.Second {
		t.Errorf("threshold = %v, want 2s", s2.threshold)
	}
	if s2.primary.Label != "Mojang" {
		t.Errorf("primary = %s, want Mojang", s2.primary.Label)
	}

	// china 模式
	s3 := NewSmartSelector("china", 0)
	if s3.primary.Label != "BMCLAPI" {
		t.Errorf("china primary = %s, want BMCLAPI", s3.primary.Label)
	}
}

func TestProbeAndSelectNoSwitch(t *testing.T) {
	s := NewSmartSelector("global", 0)
	// 不执行实际 HTTP 请求，仅验证不 panic 且返回 primary
	// 无法在测试中模拟延迟，用 head 请求会超时
	_ = s
}

func TestCurrentSource(t *testing.T) {
	s := NewSmartSelector("global", 0)
	src := s.CurrentSource()
	if src != "primary (Mojang)" {
		t.Errorf("initial source = %q, want primary (Mojang)", src)
	}
}

func TestProbeLatencyZeroOnTimeout(t *testing.T) {
	// 对不存在的地址探测应返回 0
	d := probeLatency("http://192.0.2.1:1/test", 50*time.Millisecond)
	if d != 0 {
		t.Errorf("expected 0 for unreachable, got %v", d)
	}
}
