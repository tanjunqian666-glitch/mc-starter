package mirror

import "strings"

// Mirror 镜像配置
// 国内用户访问 Mojang 官方 S3 (Amazon) 带宽极慢，
// BMCLAPI (bangbang93) 是国内最主流的 Minecraft 文件镜像。
//
// 各端点的功能：
//   Manifest   — 版本清单 (version_manifest_v2.json)
//   Version    — 版本元数据 (version.json)
//   Asset      — 资源文件 (音效/纹理等，通过 SHA1 前 2 位分目录)
//   Library    — Java 库文件 (Maven 仓库镜像)
//   FabricMeta — Fabric Loader 元数据 API
//   FabricMaven — Fabric Maven 仓库
type Mirror struct {
	Manifest    string
	Version     string
	Asset       string
	Library     string
	FabricMeta  string
	FabricMaven string
}

// DefaultMirrors 各模式镜像映射
// global: Mojang 官方 + Fabric 官方
// china:  BMCLAPI (bangbang93) — 国内最核心的 MC 镜像
//         注意: 实测从国外访问 BMCLAPI 偶发 HTTP 525/522，
//         调用方应准备 fallback 到官方源
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
// mode 支持 "global" / "china" / "auto"
// auto: 自动尝试 global → china 兜底
// 不支持的 mode 回退到 global
func Resolve(mode string) Mirror {
	if m, ok := DefaultMirrors[mode]; ok {
		return m
	}
	return DefaultMirrors["global"]
}

// AssetURL 构造 asset 对象下载 URL
// Asset 文件按 SHA1 前 2 位分目录存储
// 路径规则: {mirror_asset}/{hash[:2]}/{full_hash}
// 例: https://resources.download.minecraft.net/bd/bdf48ef6b5d0d23bbb02e17d04865216179f510a
func AssetURL(m Mirror, hash string) string {
	return strings.Join([]string{m.Asset, hash[:2], hash}, "/")
}

// LibraryURL 构造库文件 URL（Maven 坐标转路径）
// Maven 坐标经过路径转换后拼接在镜像地址后面
// 例: https://libraries.minecraft.net/net/minecraft/client/1.20.4/client-1.20.4.jar
func LibraryURL(m Mirror, path string) string {
	return strings.Join([]string{m.Library, path}, "/")
}
