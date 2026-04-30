package model

// LibraryFile 统一库文件描述（参考 PCL 的 McLibToken）
// 将"解析"和"下载"分离：先解析出完整的 LibraryFile 列表，再批量下载。
type LibraryFile struct {
	LocalPath    string // 本地完整路径（Maven 路径）
	URL          string // 下载 URL
	SHA1         string // SHA1 校验（可选）
	Size         int64  // 文件大小（可选）
	IsNative     bool   // 是否为 natives（需要解压）
	OriginalName string // 原始 Maven 坐标
}

// ============================================================
// 服务端 API 响应类型（REST API v1）
// ============================================================

// ============================================================
// 频道（Channel）类型 — P6 频道体系
// ============================================================

// ChannelInfo 服务端返回的单个频道信息
type ChannelInfo struct {
	Name         string `json:"name"`                     // 频道标识（如 "mods-core"）
	DisplayName  string `json:"display_name"`             // 显示名称（如 "核心模组"）
	Description  string `json:"description,omitempty"`    // 描述
	Required     bool   `json:"required"`                 // 是否必选
	Version      string `json:"version"`                  // 当前版本
	FileCount    int    `json:"file_count,omitempty"`      // 文件数
	TotalSize    int64  `json:"total_size,omitempty"`      // 总大小（字节）
}

// ChannelState 客户端记录的单个频道本地状态
type ChannelState struct {
	Enabled bool   `json:"enabled"`  // 是否启用
	Version string `json:"version"`  // 本地版本号（空=未安装）
}

// PackInfo 服务端返回的单个包信息（P6 扩展：增加 Channels 字段）
type PackInfo struct {
	Name          string         `json:"name"`
	DisplayName   string         `json:"display_name"`
	Primary       bool           `json:"primary"`
	LatestVersion string         `json:"latest_version"`
	Description   string         `json:"description,omitempty"`
	SizeMB        float64        `json:"size_mb,omitempty"`
	Channels      []ChannelInfo  `json:"channels,omitempty"` // P6: 频道列表
}

// PacksResponse GET /api/v1/packs 响应
type PacksResponse struct {
	Packs []PackInfo `json:"packs"`
}

// ChannelsResponse GET /api/v1/packs/{name}/channels 响应
type ChannelsResponse struct {
	Channels []ChannelInfo `json:"channels"`
}

// IncrementalUpdate 增量更新响应（P6 扩展：增加 Channels 字段）
type IncrementalUpdate struct {
	Version        string                    `json:"version"`
	FromVersion    string                    `json:"from_version"`
	Mode           string                    `json:"mode"` // "incremental" | "full"
	MCVersion      string                    `json:"mc_version,omitempty"`  // 所需 MC 版本
	Loader         string                    `json:"loader,omitempty"`       // 所需加载器，格式 "<type>-<ver>" 或空
	Added          []FileChangeEntry         `json:"added,omitempty"`
	Updated        []FileChangeEntry         `json:"updated,omitempty"`
	Removed        []string                  `json:"removed,omitempty"`
	TotalDiffBytes int64                     `json:"total_diff_bytes,omitempty"`
	Channels       map[string]ChannelVersion `json:"channels,omitempty"` // P6: 各频道版本变化
}

// ChannelVersion 增量响应中频道的版本状态
type ChannelVersion struct {
	Version string `json:"version"`
	Changed bool   `json:"changed"`
}

// FileChangeEntry 单个文件变更（P6 扩展：增加 Channel 字段）
type FileChangeEntry struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
	Channel string `json:"channel,omitempty"` // P6: 所属频道名
}

// PackDetail GET /api/v1/packs/{name} 响应（P6 扩展：增加 Channels 字段）
type PackDetail struct {
	Name          string         `json:"name"`
	DisplayName   string         `json:"display_name"`
	Primary       bool           `json:"primary"`
	LatestVersion string         `json:"latest_version"`
	Description   string         `json:"description,omitempty"`
	FileCount     int            `json:"file_count"`
	TotalSize     int64          `json:"total_size"`
	Channels      []ChannelInfo  `json:"channels,omitempty"` // P6: 频道列表
}

// ============================================================
// 客户端配置
// ============================================================

// PackState 包的本地状态（P6 扩展：增加 Channels 字段）
type PackState struct {
	Enabled      bool                    `json:"enabled"`
	Status       string                  `json:"status"` // "synced" | "updating" | "disabled" | "none"
	LocalVersion string                  `json:"local_version"`
	Dir          string                  `json:"dir"` // packs/{name} 或绝对路径
	Channels     map[string]ChannelState `json:"channels,omitempty"` // P6: 频道状态 key=频道名
}

// SelfUpdate 自更新配置
type SelfUpdate struct {
	URL     string `json:"url" yaml:"url"`
	Version string `json:"version" yaml:"version"`
}

// LocalConfig 用户本地偏好配置
type LocalConfig struct {
	MinecraftDir  string               `json:"minecraft_dir,omitempty"`  // 已废弃，读取时自动迁移到 MinecraftDirs["_default"]
	MinecraftDirs map[string]string    `json:"minecraft_dirs,omitempty"` // key=包名, val=.minecraft路径
	ServerURL     string               `json:"server_url,omitempty"`
	ServerToken   string               `json:"server_token,omitempty"`
	Packs         map[string]PackState `json:"packs,omitempty"`
	Launcher      string               `json:"launcher,omitempty"` // "bare" | "pcl2" | "hmcl"
	JavaHome      string               `json:"java_home,omitempty"`
	Username      string               `json:"username,omitempty"`
	MirrorMode    string               `json:"mirror_mode,omitempty"` // "auto" | "china" | "global"
}

// GetMinecraftDir 获取指定包对应的 .minecraft 目录
// 优先返回包专用目录，回退到 _default，再回退到旧 MinecraftDir
func (c *LocalConfig) GetMinecraftDir(packName string) string {
	if c.MinecraftDirs != nil {
		if dir, ok := c.MinecraftDirs[packName]; ok && dir != "" {
			return dir
		}
		if dir, ok := c.MinecraftDirs["_default"]; ok && dir != "" {
			return dir
		}
	}
	if c.MinecraftDir != "" {
		return c.MinecraftDir
	}
	return ""
}

// SetMinecraftDir 设置指定包的 .minecraft 目录
func (c *LocalConfig) SetMinecraftDir(packName, mcDir string) {
	if c.MinecraftDirs == nil {
		c.MinecraftDirs = make(map[string]string)
	}
	c.MinecraftDirs[packName] = mcDir
}

// MigrateMinecraftDir 兼容旧字段：读取时将 MinecraftDir 迁移到 MinecraftDirs["_default"]
func (c *LocalConfig) MigrateMinecraftDir() {
	if c.MinecraftDir != "" && (c.MinecraftDirs == nil || c.MinecraftDirs["_default"] == "") {
		if c.MinecraftDirs == nil {
			c.MinecraftDirs = make(map[string]string)
		}
		c.MinecraftDirs["_default"] = c.MinecraftDir
		// 不清空旧字段，保存时不写旧字段即可
	}
}

// ============================================================
// 服务端配置（已废弃，保留兼容）
// 新版通过 REST API 获取，不再使用本地 server.json
// ============================================================

// MCVersionConfig 本地 server.json 中的 MC 版本配置
// 用于 run/sync 命令确定目标 MC 版本
type MCVersionConfig struct {
	ID string `json:"id"` // e.g. "1.20.4"
}
