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

// ServerConfig 服务端下发配置，自动更新，不要手动改
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
}

// Modpack 模组包定义
type Modpack struct {
	Slug   string   `json:"slug" yaml:"slug"`
	Source string   `json:"source" yaml:"source"` // "modrinth" | "curseforge" | "url"
	Files  []string `json:"files,omitempty" yaml:"files,omitempty"`
}

// SelfUpdate 自更新配置
type SelfUpdate struct {
	URL     string `json:"url" yaml:"url"`
	Version string `json:"version" yaml:"version"`
}

// LocalConfig 用户本地偏好配置
type LocalConfig struct {
	InstallPath      string   `json:"install_path,omitempty" yaml:"install_path,omitempty"`
	Launcher         string   `json:"launcher,omitempty" yaml:"launcher,omitempty"` // "bare" | "pcl2" | "hmcl"
	JavaHome         string   `json:"java_home,omitempty" yaml:"java_home,omitempty"`
	Memory           int      `json:"memory,omitempty" yaml:"memory,omitempty"`
	Username         string   `json:"username,omitempty" yaml:"username,omitempty"`
	MirrorMode       string   `json:"mirror_mode,omitempty" yaml:"mirror_mode,omitempty"` // "auto" | "china" | "global"
	ManagedVersions  []string `json:"managed_versions,omitempty" yaml:"managed_versions,omitempty"` // 当前管理的版本名列表
}
