// Package pack 提供服务端整合包管理功能。
//
// 核心链路：
//   1. import — 解压整合包 zip，扫描文件，与当前版本对比差异
//   2. publish — 将 draft 版本转为正式发布版本，生成增量清单
//   3. diff — 直接比较两个已发布版本的差异
//
// 这是纯服务端功能，不涉及客户端 zip 下载/解压。
// 客户端只通过 API 拉取版本信息和按 hash 下载单个文件。
package pack

import (
	"archive/zip"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ============================================================
// 核心数据结构
// ============================================================

// FileEntry 文件清单中的单个条目
type FileEntry struct {
	Path string `json:"path"` // 相对路径（如 "mods/sodium.jar"）
	SHA1 string `json:"sha1"` // SHA1 hex
	SHA256 string `json:"sha256"` // SHA256 hex
	Size int64  `json:"size"` // 字节数
}

// Manifest 版本文件清单
type Manifest struct {
	Version   string      `json:"version"`    // 版本号（如 "v1.2.0"）
	CreatedAt time.Time   `json:"created_at"` // 创建时间
	Files     []FileEntry `json:"files"`      // 所有文件
	FileCount int         `json:"file_count"`
	TotalSize int64       `json:"total_size"`
	MCVersion string      `json:"mc_version,omitempty"` // 所需 MC 版本（如 "1.21.1"），客户端用此版本下载本体
	Loader    string      `json:"loader,omitempty"`      // 所需加载器版本，格式 "<type>-<ver>"（如 "fabric-0.16.10"），空=vanilla
}

// Diff 两个版本间的差异
type Diff struct {
	FromVersion string      `json:"from_version"`
	ToVersion   string      `json:"to_version"`
	Added       []FileEntry `json:"added"`     // 新版本有、旧版本无
	Removed     []FileEntry `json:"removed"`   // 旧版本有、新版本无
	Updated     []FileEntry `json:"updated"`   // hash 变了（含 old_sha1 字段）
	Unchanged   int         `json:"unchanged"`
}

// UpdatedEntry 更新条目（带旧 hash 以便对比）
type UpdatedEntry struct {
	Path    string `json:"path"`
	OldSHA1 string `json:"old_sha1"`
	NewSHA1 string `json:"new_sha1"`
	OldSize int64  `json:"old_size"`
	NewSize int64  `json:"new_size"`
}

// ImportResult 导入结果
type ImportResult struct {
	Status      string    `json:"status"` // "draft"
	Version     string    `json:"version"`
	MCVersion   string    `json:"mc_version,omitempty"`
	Loader      string    `json:"loader,omitempty"`
	DisplayName string    `json:"display_name,omitempty"` // 从 zip 中读到的整合包名
	Manifest    *Manifest `json:"manifest"`
	Diff        *Diff     `json:"diff,omitempty"`
	PrevVersion string    `json:"prev_version,omitempty"`
}

// ============================================================
// 核心功能：导入、发布、差异
// ============================================================

// ImportZip 导入整合包 zip，与当前版本对比，生成 draft
//
// 参数:
//   - zipPath:  zip 文件路径
//   - repoDir:  发布仓库根目录（如 /data/mc-starter/repo）
//   - version:  版本号（如 "v1.2.0"，空则自动从文件名推断）
func ImportZip(zipPath, repoDir, version string) (*ImportResult, error) {
	// 解压到临时目录
	tmpDir, err := os.MkdirTemp("", "mc-pack-import-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	manifest, err := extractAndScan(zipPath, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("解包失败: %w", err)
	}

	if version == "" {
		version = inferVersion(zipPath)
	}
	manifest.Version = version

	// 尝试从 modrinth.index.json 读取整合包名
	displayName := extractDisplayNameFromZip(zipPath)

	// 优先从 modrinth.index.json 读取 mc_version + loader（精确版本号）
	// 回退到从文件名推断（只推断类型，不推版本号）
	mcVersion, loader := extractModrinthMeta(zipPath)
	if mcVersion == "" {
		mcVersion, loader = inferFromMods(manifest)
	}

	// 将推断结果写入 Manifest，使 publish 后持久化保留
	manifest.MCVersion = mcVersion
	manifest.Loader = loader

	// 加载上一版本的 manifest
	prevManifest, prevVersion := loadLatestPublished(repoDir)

	result := &ImportResult{
		Status:      "draft",
		Version:     version,
		MCVersion:   mcVersion,
		Loader:      loader,
		DisplayName: displayName,
		Manifest:    manifest,
		PrevVersion: prevVersion,
	}

	if prevManifest != nil {
		result.Diff = ComputeDiff(prevManifest, manifest)
	}

	// 保存 draft 到仓库
	if err := saveDraft(repoDir, version, manifest, result.Diff); err != nil {
		return nil, fmt.Errorf("保存 draft 失败: %w", err)
	}

	return result, nil
}

// PublishDraft 将 draft 版本转为正式发布版本
//
// 参数:
//   - repoDir:  发布仓库根目录
//   - version:  版本号（空 = 发布最新 draft）
//   - message:  发布说明（可选）
func PublishDraft(repoDir, version, message string) error {
	if version == "" {
		var err error
		version, err = findLatestDraft(repoDir)
		if err != nil {
			return fmt.Errorf("未找到 draft 版本: %w", err)
		}
	}

	draftDir := filepath.Join(repoDir, "versions", version+".draft")
	pubDir := filepath.Join(repoDir, "versions", version)
	currentSymlink := filepath.Join(repoDir, "current")

	// 检查 draft 是否存在
	if _, err := os.Stat(draftDir); os.IsNotExist(err) {
		return fmt.Errorf("draft %s 不存在 (%s)", version, draftDir)
	}

	// 确保发布目录不存在（防止覆盖已有版本）
	if _, err := os.Stat(pubDir); err == nil {
		return fmt.Errorf("版本 %s 已发布，不能重复发布", version)
	}

	// 重命名 draft → published
	if err := os.Rename(draftDir, pubDir); err != nil {
		return fmt.Errorf("发布失败 (rename): %w", err)
	}

	// 更新 current symlink（非致命）
	os.Remove(currentSymlink)
	if err := os.Symlink(pubDir, currentSymlink); err != nil {
		// Docker 中可能不支持 symlink，忽略
	}

	// 更新 server.json
	if err := updateServerJSON(repoDir, version, message); err != nil {
		return fmt.Errorf("更新 server.json 失败: %w", err)
	}

	return nil
}

// ComputeDiff 计算两个 Manifest 的差异
func ComputeDiff(prev, curr *Manifest) *Diff {
	diff := &Diff{
		FromVersion: prev.Version,
		ToVersion:   curr.Version,
	}

	prevIdx := make(map[string]FileEntry)
	for _, f := range prev.Files {
		prevIdx[f.Path] = f
	}
	currIdx := make(map[string]FileEntry)
	for _, f := range curr.Files {
		currIdx[f.Path] = f
	}

	// 遍历当前版本
	for path, entry := range currIdx {
		if old, ok := prevIdx[path]; !ok {
			diff.Added = append(diff.Added, entry)
		} else if old.SHA1 != entry.SHA1 {
			diff.Updated = append(diff.Updated, entry)
		} else {
			diff.Unchanged++
		}
	}

	// 遍历旧版本，找出被删除的
	for path, entry := range prevIdx {
		if _, ok := currIdx[path]; !ok {
			diff.Removed = append(diff.Removed, entry)
		}
	}

	// 排序保持稳定输出
	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Path < diff.Added[j].Path })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Path < diff.Removed[j].Path })
	sort.Slice(diff.Updated, func(i, j int) bool { return diff.Updated[i].Path < diff.Updated[j].Path })

	return diff
}

// DiffVersions 直接比较仓库中的两个版本
func DiffVersions(repoDir, fromVer, toVer string) (*Diff, error) {
	packDir := filepath.Join(repoDir, "versions")

	prev, err := loadManifest(filepath.Join(packDir, fromVer, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("加载 %s 清单失败: %w", fromVer, err)
	}
	curr, err := loadManifest(filepath.Join(packDir, toVer, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("加载 %s 清单失败: %w", toVer, err)
	}

	return ComputeDiff(prev, curr), nil
}

// ============================================================
// 差异信息方法
// ============================================================

// Summary 返回差异摘要文本
func (d *Diff) Summary() string {
	var parts []string
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("+%d", len(d.Added)))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("-%d", len(d.Removed)))
	}
	if len(d.Updated) > 0 {
		parts = append(parts, fmt.Sprintf("~%d", len(d.Updated)))
	}
	parts = append(parts, fmt.Sprintf("=%d", d.Unchanged))
	return strings.Join(parts, " ")
}

// HasChanges 判断是否有实际变更
func (d *Diff) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Updated) > 0
}

// TotalDiffBytes 返回增量传输所需的字节数
func (d *Diff) TotalDiffBytes() int64 {
	var total int64
	for _, e := range d.Added {
		total += e.Size
	}
	for _, e := range d.Updated {
		total += e.Size
	}
	return total
}

// ============================================================
// zip 解包与文件扫描
// ============================================================

// extractAndScan 解压 zip 到临时目录并扫描生成 Manifest
func extractAndScan(zipPath, tmpDir string) (*Manifest, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开 zip 文件失败: %w", err)
	}
	defer reader.Close()

	// 解压所有文件
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		relPath := sanitizePath(f.Name)
		targetPath := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			continue
		}
		if err := extractZipFile(f, targetPath); err != nil {
			continue
		}
	}

	// 扫描目录生成清单（自动展平单层顶层目录）
	manifest := &Manifest{
		CreatedAt: time.Now(),
	}
	// 检查是否只有一个顶层目录
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) == 1 && entries[0].IsDir() {
		// 展平：扫描子目录
		subDir := filepath.Join(tmpDir, entries[0].Name())
		scanDir(subDir, "", manifest)
	} else {
		scanDir(tmpDir, "", manifest)
	}

	return manifest, nil
}

// scanDir 递归扫描目录，构建 Manifest
func scanDir(dir, prefix string, manifest *Manifest) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		if prefix != "" {
			relPath = prefix + "/" + relPath
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sha1h := sha1.Sum(data)
		sha256h := sha256.Sum256(data)

		entry := FileEntry{
			Path:   relPath,
			SHA1:   hex.EncodeToString(sha1h[:]),
			SHA256: hex.EncodeToString(sha256h[:]),
			Size:   info.Size(),
		}
		manifest.Files = append(manifest.Files, entry)
		manifest.TotalSize += info.Size()
		manifest.FileCount++
		return nil
	})
}

// ============================================================
// 仓库文件读写
// ============================================================

// saveDraft 将导入结果保存为 draft 版本
func saveDraft(repoDir, version string, manifest *Manifest, diff *Diff) error {
	draftDir := filepath.Join(repoDir, "versions", version+".draft")
	if err := os.MkdirAll(draftDir, 0755); err != nil {
		return err
	}

	// 写入 manifest.json
	if err := writeJSON(filepath.Join(draftDir, "manifest.json"), manifest); err != nil {
		return err
	}

	// 写入 diff.json（如果有上一版本）
	if diff != nil {
		if err := writeJSON(filepath.Join(draftDir, "diff.json"), diff); err != nil {
			return err
		}
	}

	// 写入 meta.json
	meta := map[string]interface{}{
		"version":  version,
		"status":   "draft",
		"created":  time.Now(),
		"files":    manifest.FileCount,
		"total_mb": float64(manifest.TotalSize) / 1024 / 1024,
	}
	if diff != nil && diff.HasChanges() {
		meta["diff_summary"] = diff.Summary()
	}
	return writeJSON(filepath.Join(draftDir, "meta.json"), meta)
}

// loadLatestPublished 加载最新已发布版本的 manifest
func loadLatestPublished(repoDir string) (*Manifest, string) {
	packDir := filepath.Join(repoDir, "versions")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return nil, ""
	}

	// 找到最新的已发布版本（不含 .draft 后缀）
	var versions []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".draft") {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return nil, ""
	}

	sort.Sort(sort.Reverse(sort.StringSlice(versions)))
	latest := versions[0]

	manifest, err := loadManifest(filepath.Join(packDir, latest, "manifest.json"))
	if err != nil {
		return nil, ""
	}
	manifest.Version = latest
	return manifest, latest
}

// loadManifest 从文件加载 Manifest
func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// findLatestDraft 找到最新的 draft 版本
func findLatestDraft(repoDir string) (string, error) {
	packDir := filepath.Join(repoDir, "versions")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return "", err
	}

	var drafts []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".draft") {
			drafts = append(drafts, strings.TrimSuffix(e.Name(), ".draft"))
		}
	}
	if len(drafts) == 0 {
		return "", fmt.Errorf("没有 draft 版本")
	}

	sort.Sort(sort.Reverse(sort.StringSlice(drafts)))
	return drafts[0], nil
}

// updateServerJSON 更新仓库根目录的 server.json
func updateServerJSON(repoDir, version, message string) error {
	// 加载当前 manifest 获取文件信息
	packDir := filepath.Join(repoDir, "versions", version)
	manifest, err := loadManifest(filepath.Join(packDir, "manifest.json"))
	if err != nil {
		return err
	}

	// 加载上一版本的 diff（若有）
	diffData, _ := os.ReadFile(filepath.Join(packDir, "diff.json"))
	hasDiff := diffData != nil

	cfg := map[string]interface{}{
		"version":         version,
		"published_at":    time.Now(),
		"file_count":      manifest.FileCount,
		"total_size":      manifest.TotalSize,
		"has_incremental": hasDiff,
	}
	if message != "" {
		cfg["message"] = message
	}

	path := filepath.Join(repoDir, "server.json")
	return writeJSON(path, cfg)
}

// ============================================================
// 辅助函数
// ============================================================

func sanitizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	var clean []string
	for _, p := range parts {
		switch p {
		case "", ".":
			continue
		case "..":
			if len(clean) > 0 {
				clean = clean[:len(clean)-1]
			}
		default:
			clean = append(clean, p)
		}
	}
	return filepath.Join(clean...)
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func inferVersion(zipPath string) string {
	base := filepath.Base(zipPath)
	// 去掉 .zip 后缀
	base = strings.TrimSuffix(base, ".zip")
	return base
}

func inferFromMods(manifest *Manifest) (mcVersion, loader string) {
	// 从 mods/ 目录的文件名推断 MC 版本和 Loader 类型
	// ⚠️ 注意：loader 只推断类型（如 "fabric"），版本号无法从文件名获得
	// 管理员需通过服务端 API 手动补全为完整格式（如 "fabric-0.15.0"）
	for _, f := range manifest.Files {
		if !strings.HasPrefix(f.Path, "mods/") {
			continue
		}
		name := strings.ToLower(filepath.Base(f.Path))

		// 检测 fabric
		if strings.Contains(name, "fabric") || strings.Contains(name, "fabricloader") {
			loader = "fabric"
		}
		if strings.Contains(name, "forge") {
			loader = "forge"
		}
		if strings.Contains(name, "quilt") {
			loader = "quilt"
		}

		// 从文件名提取 MC 版本：mod-1.20.1.jar 或 mod+mc1.20.1.jar
		if mcVersion == "" {
			if idx := strings.Index(name, "mc"); idx >= 0 {
				v := name[idx+2:]
				v = strings.TrimLeft(v, "0123456789")
				// 取第一个片段
				parts := strings.Split(v, "-")
				if len(parts) > 0 && len(parts[0]) > 3 {
					mcVersion = parts[0]
				}
			}
		}
	}
	return
}

// writeJSON 写入 JSON 文件（原子写入，临时文件 + rename）
func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EnsureRepo 确保发布仓库结构存在
func EnsureRepo(repoDir string) error {
	dirs := []string{
		repoDir,
		filepath.Join(repoDir, "versions"),
	}
	if err := os.MkdirAll(dirs[0], 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(dirs[1], 0755); err != nil {
		return err
	}
	return nil
}

// ListVersions 列出当前发布仓库中的所有版本
func ListVersions(repoDir string) (drafts, published []string, err error) {
	packDir := filepath.Join(repoDir, "versions")
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".draft") {
			drafts = append(drafts, strings.TrimSuffix(e.Name(), ".draft"))
		} else {
			published = append(published, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(published)))
	sort.Sort(sort.Reverse(sort.StringSlice(drafts)))
	return drafts, published, nil
}

// ============================================================
// 从整合包 zip 提取信息
// ============================================================

// modrinthIndex 对应 modrinth.index.json 的结构
type modrinthIndex struct {
	Name       string            `json:"name"`
	VersionID  string            `json:"versionId"`
	Game       string            `json:"game"`
	Dependencies map[string]string `json:"dependencies"` // "minecraft": "1.20.1", "fabric-loader": "0.15.11"
}

// extractDisplayNameFromZip 尝试从 zip 中读取整合包名
// 目前支持: modrinth.index.json 的 name 字段
func extractDisplayNameFromZip(zipPath string) string {
	idx := parseModrinthIndex(zipPath)
	if idx != nil {
		return idx.Name
	}
	return ""
}

// extractModrinthMeta 从 .mrpack/.zip 中读取 modrinth.index.json 的元数据
// 返回 mc_version 和 loader 完整规格（如 "fabric-0.15.11"）
// 读取失败时返回 ("", "")
func extractModrinthMeta(zipPath string) (mcVersion, loader string) {
	idx := parseModrinthIndex(zipPath)
	if idx == nil {
		return "", ""
	}
	mcVersion = idx.Dependencies["minecraft"]

	// dependencies 里已知的 loader key
	loaders := []string{"fabric-loader", "forge", "neoforge", "quilt"}
	for _, l := range loaders {
		if ver, ok := idx.Dependencies[l]; ok && ver != "" {
			// 转成我们的格式：把 "fabric-loader" 映射为 "fabric"
			typeName := strings.TrimSuffix(l, "-loader")
			if typeName == l {
				typeName = l
			}
			return mcVersion, typeName + "-" + ver
		}
	}
	return mcVersion, ""
}

// parseModrinthIndex 通用解析 modrinth.index.json
func parseModrinthIndex(zipPath string) *modrinthIndex {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil
	}
	defer reader.Close()

	for _, f := range reader.File {
		if f.Name == "modrinth.index.json" {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil
			}

			var idx modrinthIndex
			if err := json.Unmarshal(data, &idx); err != nil {
				return nil
			}
			return &idx
		}
	}
	return nil
}
