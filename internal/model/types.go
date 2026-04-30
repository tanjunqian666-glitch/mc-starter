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

// PackInfo 服务端返回的单个包信息
type PackInfo struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Primary      bool   `json:"primary"`
	LatestVersion string `json:"latest_version"`
	Description  string `json:"description,omitempty"`
	SizeMB       float64 `json:"size_mb,omitempty"`
}

// PacksResponse GET /api/v1/packs 响应
type PacksResponse struct {
	Packs []PackInfo `json:"packs"`
}

// IncrementalUpdate 增量更新响应
type IncrementalUpdate struct {
	Version       string           `json:"version"`
	FromVersion   string           `json:"from_version"`
	Mode          string           `json:"mode"` // "incremental" | "full"
	Added         []FileChangeEntry `json:"added,omitempty"`
	Updated       []FileChangeEntry `json:"updated,omitempty"`
	Removed       []string          `json:"removed,omitempty"`
	TotalDiffBytes int64            `json:"total_diff_bytes,omitempty"`
}

// FileChangeEntry 单个文件变更
type FileChangeEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// PackDetail GET /api/v1/packs/{name} 响应
type PackDetail struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Primary       bool   `json:"primary"`
	LatestVersion string `json:"latest_version"`
	Description   string `json:"description,omitempty"`
	FileCount     int    `json:"file_count"`
	TotalSize     int64  `json:"total_size"`
}

// ============================================================
// 客户端配置
// ============================================================

// PackState 包的本地状态
type PackState struct {
	Enabled      bool   `json:"enabled"`
	Status       string `json:"status"` // "synced" | "updating" | "disabled" | "none"
	LocalVersion string `json:"local_version"`
	Dir          string `json:"dir"` // packs/{name} 或绝对路径
}

// SelfUpdate 自更新配置
type SelfUpdate struct {
	URL     string `json:"url" yaml:"url"`
	Version string `json:"version" yaml:"version"`
}

// LocalConfig 用户本地偏好配置
type LocalConfig struct {
	MinecraftDir string               `json:"minecraft_dir,omitempty"`
	ServerURL    string               `json:"server_url,omitempty"`
	ServerToken  string               `json:"server_token,omitempty"`
	Packs        map[string]PackState `json:"packs,omitempty"`
	Launcher     string               `json:"launcher,omitempty"` // "bare" | "pcl2" | "hmcl"
	JavaHome     string               `json:"java_home,omitempty"`
	Memory       int                  `json:"memory,omitempty"`
	Username     string               `json:"username,omitempty"`
	MirrorMode   string               `json:"mirror_mode,omitempty"` // "auto" | "china" | "global"
}

// ============================================================
// 服务端配置（已废弃，保留兼容）
// 新版通过 REST API 获取，不再使用本地 server.json
// ============================================================

// ServerConfig 服务端下发配置（已废弃）
type ServerConfig struct {
	Version    VersionConfig `json:"version" yaml:"version"`
	Modpacks   []Modpack     `json:"modpacks,omitempty" yaml:"modpacks,omitempty"`
	MirrorURL  string        `json:"mirror_url,omitempty" yaml:"mirror_url,omitempty"`
	SelfUpdate *SelfUpdate   `json:"self_update,omitempty" yaml:"self_update,omitempty"`
}

// VersionConfig Minecraft 版本配置
type VersionConfig struct {
	ID        string `json:"id" yaml:"id"`                             // e.g. "1.20.4"
	Loader    string `json:"loader,omitempty" yaml:"loader,omitempty"` // "fabric" | "forge" | "quilt" | ""
	LoaderVer string `json:"loader_version,omitempty" yaml:"loader_version,omitempty"`
	JavaArgs  string `json:"java_args,omitempty" yaml:"java_args,omitempty"`
	// Deprecated: 使用 Packs 字段
}

// Modpack 模组包定义（已废弃，新版使用 PackInfo）
type Modpack struct {
	Slug   string   `json:"slug" yaml:"slug"`
	Source string   `json:"source" yaml:"source"` // "modrinth" | "curseforge" | "url"
	Files  []string `json:"files,omitempty" yaml:"files,omitempty"`
}
