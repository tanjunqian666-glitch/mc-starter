// Package server 实现 mc-starter-server REST API 服务端。
package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServerConfig 服务端配置
type ServerConfig struct {
	Server  ServerSection  `yaml:"server"`
	Auth    AuthSection    `yaml:"auth"`
	Storage StorageSection `yaml:"storage"`
	Packs   PacksSection   `yaml:"packs"`
}

// ServerSection HTTP 服务配置
type ServerSection struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	TLSEnabled bool   `yaml:"tls_enabled"`
	TLSCert    string `yaml:"tls_cert"`
	TLSKey     string `yaml:"tls_key"`
}

// AuthSection 认证配置
type AuthSection struct {
	Enabled              bool   `yaml:"enabled"`
	AdminToken           string `yaml:"admin_token"`
	ClientRequireToken   bool   `yaml:"client_require_token"`
}

// StorageSection 存储配置
type StorageSection struct {
	DataDir       string `yaml:"data_dir"`
	PacksDir      string `yaml:"packs_dir"`
	FileStorage   string `yaml:"file_storage"` // "local" (暂只支持)
	MaxPackSizeMB int    `yaml:"max_pack_size_mb"`
}

// PacksSection 包默认配置
type PacksSection struct {
	DefaultPrimary string `yaml:"default_primary"`
}

// DefaultConfig 返回带默认值的配置
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Server: ServerSection{
			Host:       "0.0.0.0",
			Port:       8443,
			TLSEnabled: false,
		},
		Auth: AuthSection{
			Enabled:              true,
			AdminToken:           "change-me-please",
			ClientRequireToken:   false,
		},
		Storage: StorageSection{
			DataDir:       "./data",
			PacksDir:      "./packs",
			FileStorage:   "local",
			MaxPackSizeMB: 1024,
		},
		Packs: PacksSection{},
	}
}

// LoadConfig 从文件加载配置，文件不存在则返回默认
// 加载后从环境变量覆盖（方便 Docker 部署）
func LoadConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return applyEnvOverrides(DefaultConfig()), nil
		}
		return nil, fmt.Errorf("读取配置 %s 失败: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}

	return applyEnvOverrides(cfg), nil
}

// SaveConfig 保存配置到文件
func SaveConfig(path string, cfg *ServerConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ListenAddr 返回监听地址字符串
func (s *ServerConfig) ListenAddr() string {
	return fmt.Sprintf("%s:%d", s.Server.Host, s.Server.Port)
}

// applyEnvOverrides 从环境变量覆盖配置（遵循 MC_前缀惯例）
// 支持的环境变量:
//   MC_SERVER_HOST, MC_SERVER_PORT, MC_SERVER_TLS_ENABLED
//   MC_AUTH_ENABLED, MC_AUTH_ADMIN_TOKEN, MC_AUTH_CLIENT_REQUIRE_TOKEN
//   MC_STORAGE_DATA_DIR, MC_STORAGE_PACKS_DIR
//   MC_PACKS_DEFAULT_PRIMARY
func applyEnvOverrides(cfg *ServerConfig) *ServerConfig {
	if v := os.Getenv("MC_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("MC_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("MC_SERVER_TLS_ENABLED"); v != "" {
		cfg.Server.TLSEnabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("MC_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("MC_AUTH_ADMIN_TOKEN"); v != "" {
		cfg.Auth.AdminToken = v
	}
	if v := os.Getenv("MC_AUTH_CLIENT_REQUIRE_TOKEN"); v != "" {
		cfg.Auth.ClientRequireToken = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("MC_STORAGE_DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv("MC_STORAGE_PACKS_DIR"); v != "" {
		cfg.Storage.PacksDir = v
	}
	if v := os.Getenv("MC_PACKS_DEFAULT_PRIMARY"); v != "" {
		cfg.Packs.DefaultPrimary = v
	}
	// 支持文件存储方式覆盖
	if v := os.Getenv("MC_STORAGE_FILE_STORAGE"); v != "" {
		cfg.Storage.FileStorage = v
	}
	// 支持上传大小限制覆盖
	if v := os.Getenv("MC_STORAGE_MAX_PACK_SIZE_MB"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Storage.MaxPackSizeMB = p
		}
	}
	return cfg
}
