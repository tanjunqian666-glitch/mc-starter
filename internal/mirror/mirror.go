package mirror

import "strings"

// Mirror 镜像配置
type Mirror struct {
	Manifest   string // 版本清单
	Version    string // 版本 JSON
	Asset      string // Asset 资源
	Library    string // 库文件
	FabricMeta string // Fabric 元数据
	FabricMaven string // Fabric Maven
}

// DefaultMirrors 各模式镜像映射
var DefaultMirrors = map[string]Mirror{
	"global": {
		Manifest:    "https://piston-meta.mojang.com",
		Version:     "https://piston-meta.mojang.com",
		Asset:       "https://resources.download.minecraft.net",
		Library:     "https://libraries.minecraft.net",
		FabricMeta:  "https://meta.fabricmc.net",
		FabricMaven: "https://maven.fabricmc.net",
	},
	"china": {
		Manifest:    "https://bmclapi2.bangbang93.com",
		Version:     "https://bmclapi2.bangbang93.com",
		Asset:       "https://bmclapi2.bangbang93.com/assets",
		Library:     "https://bmclapi2.bangbang93.com/maven",
		FabricMeta:  "https://bmclapi2.bangbang93.com/fabric-meta",
		FabricMaven: "https://bmclapi2.bangbang93.com/maven",
	},
}

// Resolve 根据 mode 获取镜像集
// auto: 自动尝试 global → china 兜底
func Resolve(mode string) Mirror {
	if m, ok := DefaultMirrors[mode]; ok {
		return m
	}
	return DefaultMirrors["global"]
}

// AssetURL 构造 asset 对象下载 URL
func AssetURL(m Mirror, hash string) string {
	return strings.Join([]string{m.Asset, hash[:2], hash}, "/")
}

// LibraryPath 构造库文件 URL（Maven 坐标转路径）
func LibraryURL(m Mirror, path string) string {
	return strings.Join([]string{m.Library, path}, "/")
}
