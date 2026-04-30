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

// MCVersionConfig 本地 server.json 中的 MC 版本配置
// 用于 run/sync 命令确定目标 MC 版本
type MCVersionConfig struct {
	ID string `json:"id"` // e.g. "1.20.4"
}
