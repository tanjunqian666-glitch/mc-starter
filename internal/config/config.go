package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/model"
)

// Manager 配置管理器
type Manager struct {
	dir    string
	client *http.Client
}

// New 创建配置管理器
func New(dir string) *Manager {
	return &Manager{
		dir: dir,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dir 返回配置目录
func (m *Manager) Dir() string { return m.dir }

// ============================================================
// 服务端 API 客户端
// ============================================================

// FetchPacks 从服务端拉取包列表
func (m *Manager) FetchPacks(serverURL string) (*model.PacksResponse, error) {
	url := strings.TrimRight(serverURL, "/") + "/api/v1/packs"
	data, err := m.httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("拉取包列表失败: %w", err)
	}
	var resp model.PacksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析包列表失败: %w", err)
	}
	return &resp, nil
}

// FetchUpdate 从服务端拉取增量更新信息（P6 扩展：支持 channels 参数）
func (m *Manager) FetchUpdate(serverURL, packName, fromVersion string, channels []string) (*model.IncrementalUpdate, error) {
	baseURL := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/update", baseURL, packName)
	params := []string{}
	if fromVersion != "" {
		params = append(params, "from="+fromVersion)
	}
	if len(channels) > 0 {
		params = append(params, "channels="+strings.Join(channels, ","))
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}
	data, err := m.httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("拉取增量更新失败: %w", err)
	}
	var update model.IncrementalUpdate
	if err := json.Unmarshal(data, &update); err != nil {
		return nil, fmt.Errorf("解析增量更新失败: %w", err)
	}
	return &update, nil
}

// DownloadFile 从服务端下载文件到本地路径
func (m *Manager) DownloadFile(serverURL, packName, fileHash, destPath string) error {
	baseURL := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/files/%s", baseURL, packName, fileHash)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	resp, err := m.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return err
	}

	return os.Rename(tmpPath, destPath)
}

// Ping 检查服务端是否可达
func (m *Manager) Ping(serverURL string) error {
	url := strings.TrimRight(serverURL, "/") + "/api/v1/ping"
	data, err := m.httpGet(url)
	if err != nil {
		return err
	}
	// 任何非空响应即表示正常
	if len(data) == 0 {
		return fmt.Errorf("服务端返回空响应")
	}
	return nil
}

// PostCrashReport 上传崩溃报告到服务端
func (m *Manager) PostCrashReport(serverURL, packName string, report model.CrashReport) (*model.CrashReportUploadResponse, error) {
	baseURL := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/crash-report", baseURL, packName)

	body := model.CrashReportUploadRequest{
		PackName: packName,
		Report:   report,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化崩溃报告失败: %w", err)
	}

	resp, err := m.client.Post(url, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("上传崩溃报告失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var uploadResp model.CrashReportUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		// 即使解析失败，上传本身可能成功
		return &model.CrashReportUploadResponse{Status: "accepted"}, nil
	}
	return &uploadResp, nil
}

// HTTPGet 返回原始 HTTP 响应（用于外部文件下载）
func (m *Manager) HTTPGet(url string) (*http.Response, error) {
	return m.client.Get(url)
}

// ============================================================
// P6 频道 API
// ============================================================

// FetchPackDetail 拉取单个包详情（含频道信息）
func (m *Manager) FetchPackDetail(serverURL, packName string) (*model.PackDetail, error) {
	baseURL := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s", baseURL, packName)
	data, err := m.httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("拉取包详情失败: %w", err)
	}
	var detail model.PackDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("解析包详情失败: %w", err)
	}
	return &detail, nil
}

// FetchChannels 拉取包的频道列表
func (m *Manager) FetchChannels(serverURL, packName string) (*model.ChannelsResponse, error) {
	baseURL := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/api/v1/packs/%s/channels", baseURL, packName)
	data, err := m.httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("拉取频道列表失败: %w", err)
	}
	var resp model.ChannelsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("解析频道列表失败: %w", err)
	}
	return &resp, nil
}

// ============================================================
// 本地配置读写
// ============================================================

// LoadLocalServerConfig 加载本地 server.json（MC 版本配置）
// 用于 run/sync 命令获取目标 MC 版本
func (m *Manager) LoadLocalServerConfig() (*model.MCVersionConfig, error) {
	path := filepath.Join(m.dir, "server.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", path, err)
	}
	var cfg struct {
		Version model.MCVersionConfig `json:"version"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	return &cfg.Version, nil
}

// LoadLocal 加载本地配置
func (m *Manager) LoadLocal() (*model.LocalConfig, error) {
	path := filepath.Join(m.dir, "local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.LocalConfig{
				Launcher:     "bare",
				MirrorMode:   "auto",
				Username:     "Player",
				Packs:        make(map[string]model.PackState),
				MinecraftDirs: make(map[string]string),
			}, nil
		}
		return nil, fmt.Errorf("读取本地配置 %s: %w", path, err)
	}
	var cfg model.LocalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析本地配置: %w", err)
	}
	if cfg.Packs == nil {
		cfg.Packs = make(map[string]model.PackState)
	}
	if cfg.MinecraftDirs == nil {
		cfg.MinecraftDirs = make(map[string]string)
	}
	// 迁移旧 MinecraftDir 到 MinecraftDirs["_default"]
	cfg.MigrateMinecraftDir()
	return &cfg, nil
}

// SaveLocal 保存本地配置
func (m *Manager) SaveLocal(cfg *model.LocalConfig) error {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	if cfg.Packs == nil {
		cfg.Packs = make(map[string]model.PackState)
	}
	if cfg.MinecraftDirs == nil {
		cfg.MinecraftDirs = make(map[string]string)
	}
	// 保存时清空旧字段，用新版 MinecraftDirs
	cfg.MinecraftDir = ""
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

// GetMinecraftDir 获取 .minecraft 目录（指定包的目录，回退到 _default）
func (m *Manager) GetMinecraftDir(localCfg *model.LocalConfig) string {
	return localCfg.GetMinecraftDir("")
}

// GetPackWorkDir 获取指定包的工作目录

// GetPackWorkDir 获取指定包的工作目录
func (m *Manager) GetPackWorkDir(mcDir, packName string) string {
	return filepath.Join(mcDir, "packs", packName)
}

// ============================================================
// 内部
// ============================================================

func (m *Manager) httpGet(url string) ([]byte, error) {
	// 支持本地文件路径（开发测试）
	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "file://") {
		filePath := strings.TrimPrefix(url, "file://")
		return os.ReadFile(filePath)
	}

	resp, err := m.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}
