package server

import (
	"fmt"

	"github.com/gege-tlph/mc-starter/internal/model"
)

// ============================================================
// PackStore 接口 — 整合包存储抽象层
//
// 当前实现: PackStore（文件系统 JSON）
// 未来实现: SQLiteStore
//
// 切换方式：
//   1. 实现此接口
//   2. 在 NewStore 中加 case
//   3. Server 不感知存储后端
// ============================================================

// PackStoreIface 整合包存储接口
type PackStoreIface interface {
	// 包管理
	CreatePack(name, displayName, description string, primary bool) error
	DeletePack(name string) error
	ListPacks() []model.PackInfo
	GetPack(name string) (*model.PackDetail, error)
	UpdateLatestVersion(name, version string) error
	UpdateDisplayName(name, displayName string) error

	// 频道管理（P6）
	CreateChannel(name, channelName, displayName, description string, required bool, dirs []string) error
	DeleteChannel(name, channelName string) error
	GetChannels(name string) ([]model.ChannelInfo, error)

	// 目录查询（用于文件读写）
	PackDir(name string) string
	FilesDir(name string) string
	VersionsDir(name string) string

	// 配置访问
	Config() *ServerConfig
}

// NewStore 创建存储后端实例
// storageType: "json"（文件系统 JSON，当前）| "sqlite"（未来）
func NewStore(cfg *ServerConfig, storageType string) (PackStoreIface, error) {
	switch storageType {
	case "json", "":
		return NewPackStore(cfg)
	case "sqlite":
		return nil, fmt.Errorf("SQLite 存储尚未实现")
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", storageType)
	}
}
