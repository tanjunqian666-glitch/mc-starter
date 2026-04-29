package launcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// SyncPhase sync 流程的阶段标记
type SyncPhase string

const (
	PhaseVersionManifest SyncPhase = "version_manifest" // 版本清单已同步
	PhaseVersionMeta     SyncPhase = "version_meta"     // version.json 已获取
	PhaseClientJar       SyncPhase = "client_jar"       // client.jar 已下载
	PhaseAssetIndex      SyncPhase = "asset_index"      // Asset 索引已拉取
	PhaseAssetFiles      SyncPhase = "asset_files"      // Asset 文件已下载
	PhaseLibraries       SyncPhase = "libraries"        // Libraries 已下载
	PhaseNatives         SyncPhase = "natives"          // Natives 已解压
	PhaseComplete        SyncPhase = "complete"         // sync 全部完成
)

// SyncState 记录 sync 流程的阶段性完成状态
// 用于断点恢复：下次运行 sync 时读取此文件，跳过已完成阶段
//
// 设计参考 PCL 的 LoaderTask 链式进度追踪，但简化：
// PCL 用 LoaderTask 的 ProgressWeight 做细粒度进度报告，
// 我们只需要阶段级的完成标记 + 文件级别的跳过。
//
// 文件存储位置: {cacheDir}/sync_state.json
type SyncState struct {
	mu sync.RWMutex

	VersionID     string    `json:"version_id"`      // 目标 MC 版本 ID
	Completed     []string  `json:"completed"`       // 已完成的阶段列表
	AssetCount    int       `json:"asset_count"`     // 已下载的 Asset 文件数
	LibraryCount  int       `json:"library_count"`   // 已下载的 Library 文件数
	FailedAssets  []string  `json:"failed_assets"`   // 上次失败的 Asset hash
	FailedLibs    []string  `json:"failed_libs"`     // 上次失败的 Library 名
	StartedAt     time.Time `json:"started_at"`      // 本次 sync 开始时间
	LastUpdatedAt time.Time `json:"last_updated_at"` // 最后更新时间

	cacheDir string // 缓存目录
	filePath string // 状态文件路径
}

// NewSyncState 创建 sync 状态追踪器
// cacheDir: 状态文件存储目录（通常是 config/.cache/）
// versionID: 当前 sync 的目标 MC 版本
func NewSyncState(cacheDir, versionID string) *SyncState {
	return &SyncState{
		VersionID:     versionID,
		Completed:     make([]string, 0),
		FailedAssets:  make([]string, 0),
		FailedLibs:    make([]string, 0),
		StartedAt:     time.Now(),
		LastUpdatedAt: time.Now(),
		cacheDir:      cacheDir,
		filePath:      filepath.Join(cacheDir, "sync_state.json"),
	}
}

// LoadSyncState 从磁盘读取 sync 状态
// 如果文件不存在或版本不匹配，返回 nil 表示无有效断点
func LoadSyncState(cacheDir, versionID string) *SyncState {
	filePath := filepath.Join(cacheDir, "sync_state.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Debug("sync 状态文件损坏，从头开始: %v", err)
		return nil
	}

	// 版本不匹配 → 无效断点
	if state.VersionID != versionID {
		logger.Debug("sync 状态版本不匹配(期望=%s, 上次=%s)，从头开始", versionID, state.VersionID)
		return nil
	}

	// 如果已经 complete，前面的 sync 完成了，不是断点
	if state.HasCompleted(PhaseComplete) {
		return nil
	}

	state.mu.Lock()
	state.cacheDir = cacheDir
	state.filePath = filePath
	state.mu.Unlock()

	logger.Info("发现 sync 断点: version=%s, 已完成 %d 个阶段",
		versionID, len(state.Completed))
	for _, p := range state.Completed {
		logger.Debug("  已完成: %s", p)
	}
	if len(state.FailedAssets) > 0 {
		logger.Info("  上次 %d 个 Asset 失败，将重试", len(state.FailedAssets))
	}
	if len(state.FailedLibs) > 0 {
		logger.Info("  上次 %d 个 Library 失败，将重试", len(state.FailedLibs))
	}

	return &state
}

// MarkCompleted 标记一个阶段已完成并持久化
func (s *SyncState) MarkCompleted(phase SyncPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.Completed {
		if p == string(phase) {
			return // 已标记
		}
	}

	s.Completed = append(s.Completed, string(phase))
	s.LastUpdatedAt = time.Now()
	s.save()
	logger.Debug("sync 阶段完成: %s", phase)
}

// HasCompleted 检查某个阶段是否已完成
func (s *SyncState) HasCompleted(phase SyncPhase) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.Completed {
		if p == string(phase) {
			return true
		}
	}
	return false
}

// AddFailedAsset 记录失败的 Asset（下次重试）
func (s *SyncState) AddFailedAsset(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 去重
	for _, h := range s.FailedAssets {
		if h == hash {
			return
		}
	}
	s.FailedAssets = append(s.FailedAssets, hash)
	s.LastUpdatedAt = time.Now()
	s.save()
}

// AddFailedLib 记录失败的 Library（下次重试）
func (s *SyncState) AddFailedLib(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range s.FailedLibs {
		if n == name {
			return
		}
	}
	s.FailedLibs = append(s.FailedLibs, name)
	s.LastUpdatedAt = time.Now()
	s.save()
}

// SetAssetCount 设置已下载的 Asset 文件数
func (s *SyncState) SetAssetCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AssetCount = count
	s.LastUpdatedAt = time.Now()
	s.save()
}

// SetLibraryCount 设置已下载的 Library 文件数
func (s *SyncState) SetLibraryCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LibraryCount = count
	s.LastUpdatedAt = time.Now()
	s.save()
}

// FailedAssetHashes 返回上次失败的 Asset hash 列表
func (s *SyncState) FailedAssetHashes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.FailedAssets))
	copy(result, s.FailedAssets)
	return result
}

// FailedLibraryNames 返回上次失败的 Library 名称列表
func (s *SyncState) FailedLibraryNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.FailedLibs))
	copy(result, s.FailedLibs)
	return result
}

// ClearFailures 清理所有失败记录
func (s *SyncState) ClearFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FailedAssets = make([]string, 0)
	s.FailedLibs = make([]string, 0)
	s.LastUpdatedAt = time.Now()
	s.save()
}

// Reset 清空所有状态（从头开始）
func (s *SyncState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Completed = make([]string, 0)
	s.FailedAssets = make([]string, 0)
	s.FailedLibs = make([]string, 0)
	s.AssetCount = 0
	s.LibraryCount = 0
	s.LastUpdatedAt = time.Now()
	s.save()
}

// Remove 删除状态文件（sync 完成后清理）
func (s *SyncState) Remove() {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.Remove(s.filePath)
	s.Completed = make([]string, 0)
}

// IsStale 检查状态是否过期（超过 1 小时未更新视为过期，防止未完成的 session 遗留）
func (s *SyncState) IsStale() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastUpdatedAt) > 1*time.Hour
}

// Summary 返回当前状态摘要（用于日志/进度输出）
func (s *SyncState) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf("version=%s, completed=%d, assets=%d, libs=%d, failed=%d+%d",
		s.VersionID, len(s.Completed), s.AssetCount, s.LibraryCount,
		len(s.FailedAssets), len(s.FailedLibs))
}

// save 写入磁盘
func (s *SyncState) save() {
	if s.filePath == "" {
		return
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		logger.Warn("序列化 sync 状态失败: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		logger.Warn("创建 sync 状态目录失败: %v", err)
		return
	}

	// 原子写入：先写 .tmp 再重命名
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.Warn("写入 sync 状态临时文件失败: %v", err)
		return
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		logger.Warn("原子重命名 sync 状态文件失败: %v", err)
	}
}

// IsStaleEx 检查特定 age 是否过期（用于自定义超时）
func (s *SyncState) IsStaleEx(maxAge time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastUpdatedAt) > maxAge
}
