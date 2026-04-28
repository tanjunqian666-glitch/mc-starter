package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tanjunqian666-glitch/mc-starter/internal/model"
)

// Manager 配置管理器
type Manager struct {
	dir string
}

// New 创建配置管理器
func New(dir string) *Manager {
	return &Manager{dir: dir}
}

// Dir 返回配置目录
func (m *Manager) Dir() string { return m.dir }

// LoadServer 加载服务端配置
func (m *Manager) LoadServer() (*model.ServerConfig, error) {
	path := filepath.Join(m.dir, "server.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取服务端配置 %s: %w", path, err)
	}
	var cfg model.ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析服务端配置: %w", err)
	}
	return &cfg, nil
}

// LoadLocal 加载本地配置
func (m *Manager) LoadLocal() (*model.LocalConfig, error) {
	path := filepath.Join(m.dir, "local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.LocalConfig{
				Launcher:   "bare",
				MirrorMode: "auto",
				Memory:     4096,
				Username:   "Player",
			}, nil
		}
		return nil, fmt.Errorf("读取本地配置 %s: %w", path, err)
	}
	var cfg model.LocalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析本地配置: %w", err)
	}
	return &cfg, nil
}

// SaveLocal 保存本地配置
func (m *Manager) SaveLocal(cfg *model.LocalConfig) error {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	path := filepath.Join(m.dir, "local.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化本地配置: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入本地配置: %w", err)
	}
	return nil
}
