package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/gege-tlph/mc-starter/internal/pack"
)

// PackMeta 整合包元数据（索引条目）
type PackMeta struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Primary      bool   `json:"primary"`
	LatestVersion string `json:"latest_version"`
	Description  string `json:"description,omitempty"`
}

// PackIndex 整合包索引文件结构
type PackIndex struct {
	Packs map[string]*PackMeta `json:"packs"` // key = pack name
}

// PackStore 管理整合包的生命周期和索引
type PackStore struct {
	mu       sync.RWMutex
	config   *ServerConfig
	index    *PackIndex
	indexPath string
}

// NewPackStore 创建 PackStore
func NewPackStore(cfg *ServerConfig) (*PackStore, error) {
	store := &PackStore{
		config:   cfg,
		index:    &PackIndex{Packs: make(map[string]*PackMeta)},
		indexPath: filepath.Join(cfg.Storage.PacksDir, "index.json"),
	}

	// 确保目录存在
	if err := os.MkdirAll(cfg.Storage.PacksDir, 0755); err != nil {
		return nil, fmt.Errorf("创建 packs 目录失败: %w", err)
	}
	if err := os.MkdirAll(cfg.Storage.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建 data 目录失败: %w", err)
	}

	// 加载已有索引
	if err := store.loadIndex(); err != nil {
		return nil, fmt.Errorf("加载索引失败: %w", err)
	}

	return store, nil
}

// loadIndex 从磁盘加载索引
func (s *PackStore) loadIndex() error {
	data, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 新仓库
		}
		return err
	}

	var idx PackIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("解析索引文件失败: %w", err)
	}

	if idx.Packs == nil {
		idx.Packs = make(map[string]*PackMeta)
	}
	s.index = &idx
	return nil
}

// saveIndex 保存索引到磁盘
func (s *PackStore) saveIndex() error {
	data, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化索引失败: %w", err)
	}
	return os.WriteFile(s.indexPath, data, 0644)
}

// CreatePack 创建新整合包
func (s *PackStore) CreatePack(name, displayName, description string, primary bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.index.Packs[name]; exists {
		return fmt.Errorf("整合包 '%s' 已存在", name)
	}

	// 如果设为主包，取消其他主包标记
	if primary {
		for _, m := range s.index.Packs {
			m.Primary = false
		}
	}

	s.index.Packs[name] = &PackMeta{
		Name:         name,
		DisplayName:  displayName,
		Primary:      primary,
		LatestVersion: "",
		Description:  description,
	}

	// 创建包目录
	packDir := filepath.Join(s.config.Storage.PacksDir, name)
	if err := os.MkdirAll(packDir, 0755); err != nil {
		return fmt.Errorf("创建包目录失败: %w", err)
	}
	// 创建 versions 子目录
	if err := os.MkdirAll(filepath.Join(packDir, "versions"), 0755); err != nil {
		return fmt.Errorf("创建 versions 目录失败: %w", err)
	}

	return s.saveIndex()
}

// DeletePack 删除整合包（含所有版本）
func (s *PackStore) DeletePack(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.index.Packs[name]; !exists {
		return fmt.Errorf("整合包 '%s' 不存在", name)
	}

	packDir := filepath.Join(s.config.Storage.PacksDir, name)
	if err := os.RemoveAll(packDir); err != nil {
		return fmt.Errorf("删除包目录失败: %w", err)
	}

	delete(s.index.Packs, name)
	return s.saveIndex()
}

// ListPacks 返回所有包的元数据（P6 扩展：附带频道信息）
func (s *PackStore) ListPacks() []model.PackInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.PackInfo, 0, len(s.index.Packs))
	for _, m := range s.index.Packs {
		pf := loadPackFileInfo(s.config.Storage.PacksDir, m.Name, m.LatestVersion)
		pi := model.PackInfo{
			Name:          m.Name,
			DisplayName:   m.DisplayName,
			Primary:       m.Primary,
			LatestVersion: m.LatestVersion,
			Description:   m.Description,
			SizeMB:        pf.sizeMB,
		}
		// 附带频道信息（不锁定，读不到就算了）
		channels, _ := s.GetChannels(m.Name)
		pi.Channels = channels
		result = append(result, pi)
	}
	return result
}

// GetPack 返回单个包详情（P6 扩展：附带频道信息）
func (s *PackStore) GetPack(name string) (*model.PackDetail, error) {
	s.mu.RLock()
	m, exists := s.index.Packs[name]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("整合包 '%s' 不存在", name)
	}

	pf := loadPackFileInfo(s.config.Storage.PacksDir, name, m.LatestVersion)

	// 加载频道信息
	channels, _ := s.GetChannels(name)

	return &model.PackDetail{
		Name:          m.Name,
		DisplayName:   m.DisplayName,
		Primary:       m.Primary,
		LatestVersion: m.LatestVersion,
		Description:   m.Description,
		FileCount:     pf.fileCount,
		TotalSize:     pf.totalSize,
		Channels:      channels,
	}, nil
}

// UpdateLatestVersion 更新包的最新版本号
func (s *PackStore) UpdateLatestVersion(name, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, exists := s.index.Packs[name]
	if !exists {
		return fmt.Errorf("整合包 '%s' 不存在", name)
	}
	m.LatestVersion = version
	return s.saveIndex()
}

// PackDir 返回包的文件存储目录
func (s *PackStore) PackDir(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(s.config.Storage.PacksDir, name)
}

// FilesDir 返回包的文件存储目录（按 hash 分目录）
func (s *PackStore) FilesDir(name string) string {
	return filepath.Join(s.PackDir(name), "files")
}

// VersionsDir 返回包的版本目录
func (s *PackStore) VersionsDir(name string) string {
	return filepath.Join(s.PackDir(name), "versions")
}

// Config 返回服务端配置
func (s *PackStore) Config() *ServerConfig {
	return s.config
}

// packFileInfo 缓存的文件信息
type packFileInfo struct {
	sizeMB    float64
	fileCount int
	totalSize int64
}

// loadPackFileInfo 加载包文件信息（从最新版本 manifest）
func loadPackFileInfo(packsDir, name, version string) packFileInfo {
	if version == "" {
		return packFileInfo{}
	}

	manifestPath := filepath.Join(packsDir, name, "versions", version, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return packFileInfo{}
	}

	var m pack.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return packFileInfo{}
	}

	return packFileInfo{
		sizeMB:    float64(m.TotalSize) / 1024 / 1024,
		fileCount: m.FileCount,
		totalSize: m.TotalSize,
	}
}

// ============================================================
// 频道管理（P6 频道体系）
// ============================================================

// ChannelMeta 频道元数据（存储为 packages/{pack}/channels/{channel}/meta.json）
type ChannelMeta struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description,omitempty"`
	Dirs        []string `json:"dirs"`       // 关联目录（包内相对路径，如 ["mods/", "config/"]）
	Required    bool     `json:"required"`   // 是否必选
	Priority    int      `json:"priority"`   // 合并优先级
	Version     string   `json:"version"`    // 当前版本
	FileCount   int      `json:"file_count"`
	TotalSize   int64    `json:"total_size"`
}

// channelsDir 返回包的频道目录
func (s *PackStore) channelsDir(name string) string {
	return filepath.Join(s.PackDir(name), "channels")
}

// channelDir 返回指定频道的目录
func (s *PackStore) channelDir(name, channelName string) string {
	return filepath.Join(s.channelsDir(name), channelName)
}

// CreateChannel 创建新频道
func (s *PackStore) CreateChannel(name, channelName, displayName, description string, required bool, dirs []string) error {
	if channelName == "" || channelName == "all" {
		return fmt.Errorf("频道名不能为空或为保留名 'all'")
	}

	chDir := s.channelDir(name, channelName)
	if _, err := os.Stat(chDir); err == nil {
		return fmt.Errorf("频道 '%s' 已存在", channelName)
	}

	if err := os.MkdirAll(chDir, 0755); err != nil {
		return fmt.Errorf("创建频道目录失败: %w", err)
	}

	if displayName == "" {
		displayName = channelName
	}

	meta := ChannelMeta{
		Name:        channelName,
		DisplayName: displayName,
		Description: description,
		Dirs:        dirs,
		Required:    required,
		Priority:    0,
		Version:     "",
	}

	return s.saveChannelMeta(name, channelName, &meta)
}

// DeleteChannel 删除频道
func (s *PackStore) DeleteChannel(name, channelName string) error {
	if channelName == "all" {
		return fmt.Errorf("不能删除基础频道 'all'")
	}

	chDir := s.channelDir(name, channelName)
	if _, err := os.Stat(chDir); os.IsNotExist(err) {
		return fmt.Errorf("频道 '%s' 不存在", channelName)
	}

	return os.RemoveAll(chDir)
}

// GetChannels 返回包的所有频道信息
func (s *PackStore) GetChannels(name string) ([]model.ChannelInfo, error) {
	chDir := s.channelsDir(name)
	if _, err := os.Stat(chDir); os.IsNotExist(err) {
		// 没有频道目录，自动创建 all 频道
		os.MkdirAll(chDir, 0755)
		return s.ensureAllChannel(name)
	}

	entries, err := os.ReadDir(chDir)
	if err != nil {
		return nil, fmt.Errorf("读取频道目录失败: %w", err)
	}

	var channels []model.ChannelInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		meta, err := s.loadChannelMeta(name, e.Name())
		if err != nil {
			// 跳过读不出的条目
			continue
		}
		channels = append(channels, model.ChannelInfo{
			Name:        meta.Name,
			DisplayName: meta.DisplayName,
			Description: meta.Description,
			Required:    meta.Required,
			Version:     meta.Version,
			FileCount:   meta.FileCount,
			TotalSize:   meta.TotalSize,
		})
	}

	if len(channels) == 0 {
		return s.ensureAllChannel(name)
	}

	return channels, nil
}

// ensureAllChannel 确保 all 频道存在（兼容无频道的老包）
func (s *PackStore) ensureAllChannel(name string) ([]model.ChannelInfo, error) {
	allMeta := ChannelMeta{
		Name:        "all",
		DisplayName: "全部文件",
		Required:    true,
		Version:     "",
	}

	allDir := s.channelDir(name, "all")
	if err := os.MkdirAll(allDir, 0755); err != nil {
		return nil, fmt.Errorf("创建 all 频道目录失败: %w", err)
	}

	if err := s.saveChannelMeta(name, "all", &allMeta); err != nil {
		return nil, err
	}

	return []model.ChannelInfo{{
		Name:        "all",
		DisplayName: "全部文件",
		Required:    true,
		Version:     "",
	}}, nil
}

// loadChannelMeta 加载频道元数据
func (s *PackStore) loadChannelMeta(name, channelName string) (*ChannelMeta, error) {
	metaPath := filepath.Join(s.channelDir(name, channelName), "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var meta ChannelMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// saveChannelMeta 保存频道元数据
func (s *PackStore) saveChannelMeta(name, channelName string, meta *ChannelMeta) error {
	chDir := s.channelDir(name, channelName)
	if err := os.MkdirAll(chDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	metaPath := filepath.Join(chDir, "meta.json")
	return os.WriteFile(metaPath, data, 0644)
}

// packDirExists 检查包目录是否存在
func packDirExists(packsDir, name string) bool {
	info, err := os.Stat(filepath.Join(packsDir, name, "versions"))
	return err == nil && info.IsDir()
}
