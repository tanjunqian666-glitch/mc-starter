package server

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestPackStore 创建测试用的 PackStore（临时目录）
func newTestPackStore(t *testing.T) *PackStore {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &ServerConfig{
		Server: ServerSection{
			Host: "127.0.0.1",
			Port: 0,
		},
		Storage: StorageSection{
			PacksDir: filepath.Join(tmpDir, "packs"),
			DataDir:  filepath.Join(tmpDir, "data"),
		},
	}
	store, err := NewPackStore(cfg)
	if err != nil {
		t.Fatalf("NewPackStore failed: %v", err)
	}
	return store
}

func TestCreatePack(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "测试描述", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	// 获取包详情
	detail, err := s.GetPack("test-pack")
	if err != nil {
		t.Fatalf("GetPack failed: %v", err)
	}
	if detail.Name != "test-pack" {
		t.Errorf("name = %s", detail.Name)
	}
	if !detail.Primary {
		t.Error("primary 应为 true")
	}
}

func TestCreateChannel(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	// 创建频道
	if err := s.CreateChannel("test-pack", "shaderpacks", "光影包", "高性能需求光影", false, []string{"shaderpacks/"}); err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	// 获取频道列表
	channels, err := s.GetChannels("test-pack")
	if err != nil {
		t.Fatalf("GetChannels failed: %v", err)
	}

	// 应该包含 all + shaderpacks
	found := false
	for _, ch := range channels {
		if ch.Name == "shaderpacks" {
			found = true
			if ch.DisplayName != "光影包" {
				t.Errorf("display_name = %s", ch.DisplayName)
			}
			if ch.Required {
				t.Error("required 应为 false")
			}
		}
	}
	if !found {
		t.Error("未找到 shaderpacks 频道")
	}
}

func TestDeleteChannel(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	if err := s.CreateChannel("test-pack", "shaderpacks", "光影包", "", false, []string{"shaderpacks/"}); err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	if err := s.DeleteChannel("test-pack", "shaderpacks"); err != nil {
		t.Fatalf("DeleteChannel failed: %v", err)
	}

	// 验证已删除
	chDir := s.channelDir("test-pack", "shaderpacks")
	if _, err := os.Stat(chDir); !os.IsNotExist(err) {
		t.Error("频道目录应已被删除")
	}
}

func TestGetChannelsCreatesAllChannel(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	// 没有显式创建频道，应该自动创建 all
	channels, err := s.GetChannels("test-pack")
	if err != nil {
		t.Fatalf("GetChannels failed: %v", err)
	}

	foundAll := false
	for _, ch := range channels {
		if ch.Name == "all" {
			foundAll = true
			if !ch.Required {
				t.Error("all 频道应为 required")
			}
		}
	}
	if !foundAll {
		t.Error("未自动创建 all 频道")
	}
}

func TestCantDeleteAllChannel(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	s.GetChannels("test-pack") // 触发 all 频道创建

	if err := s.DeleteChannel("test-pack", "all"); err == nil {
		t.Error("删除 all 频道应报错")
	}
}

func TestListPacksWithChannels(t *testing.T) {
	s := newTestPackStore(t)

	if err := s.CreatePack("test-pack", "测试包", "", true); err != nil {
		t.Fatalf("CreatePack failed: %v", err)
	}

	if err := s.CreateChannel("test-pack", "shaderpacks", "光影包", "", false, []string{"shaderpacks/"}); err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	packs := s.ListPacks()
	if len(packs) != 1 {
		t.Fatalf("pack count = %d", len(packs))
	}

	found := false
	for _, ch := range packs[0].Channels {
		if ch.Name == "shaderpacks" {
			found = true
		}
	}
	if !found {
		t.Error("ListPacks 应包含频道信息")
	}
}
