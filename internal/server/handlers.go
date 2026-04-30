package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gege-tlph/mc-starter/internal/model"
	"github.com/gege-tlph/mc-starter/internal/pack"
)

// ============================================================
// 客户端端点
// ============================================================

// handlePing GET /api/v1/ping
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"version":  "1.0.0",
		"time":     time.Now().UTC().Format(time.RFC3339),
	})
}

// handleListPacks GET /api/v1/packs
func (s *Server) handleListPacks(w http.ResponseWriter, r *http.Request) {
	packs := s.store.ListPacks()
	writeJSON(w, http.StatusOK, map[string]any{
		"packs": packs,
	})
}

// handleGetPack GET /api/v1/packs/{name}
func (s *Server) handleGetPack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	detail, err := s.store.GetPack(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleGetUpdate GET /api/v1/packs/{name}/update?from={version}
func (s *Server) handleGetUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fromVersion := r.URL.Query().Get("from")

	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	versionsDir := s.store.VersionsDir(name)

	// 读取最新发布的 manifest
	latestVersion := ""
	latestManifest := loadManifestFromDir(versionsDir, "")

	if latestManifest != nil {
		latestVersion = latestManifest.Version
	}

	if latestManifest == nil {
		// 没有已发布的版本
		writeError(w, http.StatusNotFound, "VERSION_NOT_FOUND", "整合包 '"+name+"' 没有已发布版本")
		return
	}

	if fromVersion == "" {
		// 没有 from 参数，返回全量信息
		writeJSON(w, http.StatusOK, map[string]any{
			"version":      latestVersion,
			"from_version": "",
			"mode":         "full",
			"file_count":   latestManifest.FileCount,
			"total_size":   latestManifest.TotalSize,
			"mc_version":   latestManifest.MCVersion,
			"loader":       latestManifest.Loader,
		})
		return
	}

	if fromVersion == latestVersion {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":      latestVersion,
			"from_version": fromVersion,
			"mode":         "incremental",
			"mc_version":   latestManifest.MCVersion,
			"loader":       latestManifest.Loader,
			"added":        []any{},
			"updated":      []any{},
			"removed":      []string{},
			"total_diff_bytes": 0,
		})
		return
	}

	// 加载上一版本的 manifest
	fromManifest := loadManifestForVersion(versionsDir, fromVersion)
	if fromManifest == nil {
		writeError(w, http.StatusNotFound, "VERSION_NOT_FOUND", "版本 '"+fromVersion+"' 不存在")
		return
	}

	// 计算差异
	diff := pack.ComputeDiff(fromManifest, latestManifest)

	// 转换格式
	added := toFileChangeEntries(diff.Added, "sha256")
	updated := toFileChangeEntriesFromDiff(diff.Updated)

	removed := make([]string, len(diff.Removed))
	for i, f := range diff.Removed {
		removed[i] = f.Path
	}

	var totalDiffBytes int64
	for _, f := range diff.Added {
		totalDiffBytes += f.Size
	}
	for _, f := range diff.Updated {
		totalDiffBytes += f.Size
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":         latestVersion,
		"from_version":    fromVersion,
		"mode":            "incremental",
		"mc_version":      latestManifest.MCVersion,
		"loader":          latestManifest.Loader,
		"added":           added,
		"updated":         updated,
		"removed":         removed,
		"total_diff_bytes": totalDiffBytes,
	})
}

// handleFileDownload GET /api/v1/packs/{name}/files/{hash}
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	hash := r.PathValue("hash")

	if name == "" || hash == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名或 hash")
		return
	}

	// files/{hash[:2]}/{hash}
	filePath := filepath.Join(s.store.PackDir(name), "files", hash[:2], hash)

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "读取文件失败")
		return
	}
	defer f.Close()

	// 获取文件信息
	info, err := f.Stat()
	if err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	http.ServeContent(w, r, hash, time.Time{}, f)
}

// ============================================================
// 管理端端点
// ============================================================

// handleCreatePack POST /api/v1/admin/packs
func (s *Server) handleCreatePack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description,omitempty"`
		Primary     bool   `json:"primary"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "无效的 JSON 请求体")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "包名不能为空")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}

	if err := s.store.CreatePack(req.Name, req.DisplayName, req.Description, req.Primary); err != nil {
		if strings.Contains(err.Error(), "已存在") {
			writeError(w, http.StatusConflict, "PACK_EXISTS", err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "created",
		"name":   req.Name,
	})
}

// handleDeletePack DELETE /api/v1/admin/packs/{name}
func (s *Server) handleDeletePack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	if err := s.store.DeletePack(name); err != nil {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"name":   name,
	})
}

// handleGetPackConfig GET /api/v1/admin/packs/{name}/config
func (s *Server) handleGetPackConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	detail, err := s.store.GetPack(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleUpdatePackConfig PUT /api/v1/admin/packs/{name}/config
func (s *Server) handleUpdatePackConfig(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req struct {
		DisplayName string `json:"display_name,omitempty"`
		Description string `json:"description,omitempty"`
		MCVersion   string `json:"mc_version,omitempty"`  // MC 版本（如 "1.21.1"），空=不修改
		Loader      string `json:"loader,omitempty"`       // Loader 完整规格（如 "fabric-0.15.0"），空=不修改
		Primary     *bool  `json:"primary,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "无效的 JSON 请求体")
		return
	}

	// 更新 Manifest 配置（publish 后的版本）
	store := s.store
	if err := store.UpdatePackConfig(name, req.MCVersion, req.Loader); err != nil {
		writeError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleImportPack POST /api/v1/admin/packs/{name}/import
func (s *Server) handleImportPack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	// 检查包是否存在
	if !packDirExists(s.config.Storage.PacksDir, name) {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", "整合包 '"+name+"' 不存在")
		return
	}

	// 读取 multipart 上传的文件
	r.ParseMultipartForm(int64(s.config.Storage.MaxPackSizeMB) * 1024 * 1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "IMPORT_FAILED", "缺少上传文件: "+err.Error())
		return
	}
	defer file.Close()

	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "mc-pack-upload-*.zip")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "创建临时文件失败")
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, file); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "写入临时文件失败")
		return
	}
	tmpFile.Close()

	// 版本号：优先从 form field 取，其次从文件名推断
	version := r.FormValue("version")
	if version == "" {
		version = strings.TrimSuffix(header.Filename, ".zip")
	}

	// 调用 pack.ImportZip
	result, err := pack.ImportZip(tmpFile.Name(), s.store.PackDir(name), version)
	if err != nil {
		writeError(w, http.StatusBadRequest, "IMPORT_FAILED", "导入失败: "+err.Error())
		return
	}

	// display_name：优先用 zip 内解析到的，其次 form field，最后文件名
	displayName := result.DisplayName
	if displayName == "" {
		displayName = r.FormValue("display_name")
	}
	if displayName == "" {
		displayName = strings.TrimSuffix(header.Filename, ".zip")
	}
	// 更新包展示名
	if err := s.store.UpdateDisplayName(name, displayName); err != nil {
		fmt.Printf("WARN: 更新展示名失败: %v\n", err)
	}

	// 把文件复制到 files/ 目录
	if err := copyFilesToStore(tmpFile.Name(), s.store.FilesDir(name), result.Manifest); err != nil {
		// 失败不阻断，只是文件存储出错
		fmt.Printf("WARN: 文件存储失败: %v\n", err)
	}

	// 保存 zip 副本到版本目录（用于全量下载）
	versionsDir := s.store.VersionsDir(name)
	zipDest := filepath.Join(versionsDir, result.Version+".draft", "pack.zip")
	if err := os.MkdirAll(filepath.Dir(zipDest), 0755); err == nil {
		srcData, err := os.ReadFile(tmpFile.Name())
		if err == nil {
			os.WriteFile(zipDest, srcData, 0644)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// handlePublishPack POST /api/v1/admin/packs/{name}/publish
func (s *Server) handlePublishPack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	var req struct {
		Message string `json:"message,omitempty"`
		Primary *bool  `json:"primary,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	packDir := s.store.PackDir(name)
	repoDir := filepath.Join(packDir, "versions")

	// 查找最新的 draft
	latestDraft, err := findLatestDraftInDir(repoDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "DRAFT_NOT_FOUND", "没有未发布的 draft")
		return
	}

	// 调用 pack.PublishDraft
	if err := pack.PublishDraft(packDir, latestDraft, req.Message); err != nil {
		writeError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	// 更新索引
	s.store.UpdateLatestVersion(name, latestDraft)

	// 发布时如果设为主包
	if req.Primary != nil && *req.Primary {
		// 简化：caller 应在创建包时设置
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "published",
		"version": latestDraft,
	})
}

// handleListVersions GET /api/v1/admin/packs/{name}/versions
func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versionsDir := s.store.VersionsDir(name)

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", "整合包 '"+name+"' 不存在")
		return
	}

	type versionInfo struct {
		Version   string `json:"version"`
		Published bool   `json:"published"`
		CreatedAt string `json:"created_at,omitempty"`
	}

	var versions []versionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vi := versionInfo{Version: e.Name()}
		if !strings.HasSuffix(e.Name(), ".draft") {
			vi.Published = true
		} else {
			vi.Version = strings.TrimSuffix(e.Name(), ".draft")
		}

		// 读取 manifest 获取创建时间
		manifestPath := filepath.Join(versionsDir, e.Name(), "manifest.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			var m pack.Manifest
			if json.Unmarshal(data, &m) == nil {
				vi.CreatedAt = m.CreatedAt.Format(time.RFC3339)
			}
		}

		versions = append(versions, vi)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"versions": versions,
	})
}

// handleDeleteVersion DELETE /api/v1/admin/packs/{name}/versions/{ver}
func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ver := r.PathValue("ver")

	versionDir := filepath.Join(s.store.VersionsDir(name), ver)
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		// 也尝试 .draft 后缀
		versionDir = filepath.Join(s.store.VersionsDir(name), ver+".draft")
		if _, err := os.Stat(versionDir); os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "VERSION_NOT_FOUND", "版本 '"+ver+"' 不存在")
			return
		}
	}

	if err := os.RemoveAll(versionDir); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "删除版本失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "deleted",
		"version": ver,
	})
}

// ============================================================
// 内部辅助
// ============================================================

// loadManifestFromDir 从版本目录加载最新发布的 manifest
func loadManifestFromDir(versionsDir, exclude string) *pack.Manifest {
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil
	}

	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".draft") {
			continue
		}
		if name == exclude {
			continue
		}
		if name > latest {
			latest = name
		}
	}

	if latest == "" {
		return nil
	}

	return loadManifestForVersion(versionsDir, latest)
}

// loadManifestForVersion 加载指定版本的 manifest
func loadManifestForVersion(versionsDir, version string) *pack.Manifest {
	manifestPath := filepath.Join(versionsDir, version, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}

	var m pack.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// toFileChangeEntries 转换 FileEntry 为 FileChangeEntry
func toFileChangeEntries(entries []pack.FileEntry, hashField string) []any {
	result := make([]any, len(entries))
	for i, f := range entries {
		h := f.SHA256
		if hashField == "sha1" {
			h = f.SHA1
		}
		result[i] = map[string]any{
			"path": f.Path,
			"hash": h,
			"size": f.Size,
		}
	}
	return result
}

// toFileChangeEntriesFromDiff 转换 Diff.Updated ([]FileEntry) 为 JSON
func toFileChangeEntriesFromDiff(entries []pack.FileEntry) []any {
	result := make([]any, len(entries))
	for i, f := range entries {
		result[i] = map[string]any{
			"path": f.Path,
			"hash": f.SHA1,
			"size": f.Size,
		}
	}
	return result
}

// copyFilesToStore 将 zip 中的文件按 hash 复制到文件存储
func copyFilesToStore(zipPath, filesDir string, manifest *pack.Manifest) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 文件失败: %w", err)
	}
	defer reader.Close()

	// 构建 zip 文件路径 → zip.File 映射
	zipFileMap := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		zipFileMap[f.Name] = f
	}

	for _, entry := range manifest.Files {
		hashDir := filepath.Join(filesDir, entry.SHA256[:2])
		destPath := filepath.Join(hashDir, entry.SHA256)

		// 已存在则跳过
		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		if err := os.MkdirAll(hashDir, 0755); err != nil {
			return fmt.Errorf("创建 hash 目录 %s 失败: %w", hashDir, err)
		}

		// 从 zip 中提取文件
		zf, ok := zipFileMap[entry.Path]
		if !ok {
			// 如果 entry.Path 带 overrides/ 前缀，尝试 strip
			stripped := strings.TrimPrefix(entry.Path, "overrides/")
			zf, ok = zipFileMap[stripped]
			if !ok {
				return fmt.Errorf("zip 中未找到文件: %s", entry.Path)
			}
		}

		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("打开 zip 内文件 %s 失败: %w", entry.Path, err)
		}

		out, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("创建文件 %s 失败: %w", destPath, err)
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", destPath, err)
		}
	}

	return nil
}

// findLatestDraftInDir 在版本目录中查找最新的 draft
func findLatestDraftInDir(versionsDir string) (string, error) {
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return "", fmt.Errorf("读取版本目录失败: %w", err)
	}

	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".draft") {
			continue
		}
		ver := strings.TrimSuffix(e.Name(), ".draft")
		if ver > latest {
			latest = ver
		}
	}

	if latest == "" {
		return "", fmt.Errorf("没有找到 draft 版本")
	}
	return latest, nil
}

// ============================================================
// 频道端点（P6 频道体系）
// ============================================================

// handleListChannels GET /api/v1/packs/{name}/channels
func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	channels, err := s.store.GetChannels(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "PACK_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.ChannelsResponse{Channels: channels})
}

// handleCreateChannel POST /api/v1/admin/packs/{name}/channels
func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	var req struct {
		ChannelName string   `json:"channel_name"`
		DisplayName string   `json:"display_name"`
		Description string   `json:"description,omitempty"`
		Required    bool     `json:"required"`
		Dirs        []string `json:"dirs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "无效的 JSON 请求体")
		return
	}

	if req.ChannelName == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "频道名不能为空")
		return
	}

	if err := s.store.CreateChannel(name, req.ChannelName, req.DisplayName, req.Description, req.Required, req.Dirs); err != nil {
		writeError(w, http.StatusBadRequest, "CREATE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":  "created",
		"channel": req.ChannelName,
		"pack":    name,
	})
}

// handleDeleteChannel DELETE /api/v1/admin/packs/{name}/channels/{channel}
func (s *Server) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	channel := r.PathValue("channel")
	if name == "" || channel == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名或频道名")
		return
	}

	if err := s.store.DeleteChannel(name, channel); err != nil {
		writeError(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "deleted",
		"channel": channel,
		"pack":    name,
	})
}

// ============================================================
// 崩溃报告端点
// ============================================================

// handleCrashReport POST /api/v1/packs/{name}/crash-report
// 接收客户端上传的崩溃报告，记录到服务端供分析
func (s *Server) handleCrashReport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少包名")
		return
	}

	var req model.CrashReportUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "无效的 JSON 请求体: "+err.Error())
		return
	}

	// 记录到服务端日志
	report := req.Report
	fmt.Printf("[CRASH] pack=%s, error=%s, exitCode=%d, ts=%s\n",
		name, report.ErrorMessage, report.ExitCode, report.Timestamp)

	if report.LogTail != "" {
		logTail := report.LogTail
		if len(logTail) > 200 {
			logTail = logTail[:200] + "..."
		}
		fmt.Printf("[CRASH] log_tail=%s\n", logTail)
	}

	// 存储到文件系统（按包名+日期归档）
	crashDir := filepath.Join(s.store.PackDir(name), "crash-reports")
	if err := os.MkdirAll(crashDir, 0755); err == nil {
		filename := fmt.Sprintf("crash-%s.json", time.Now().UTC().Format("20060102T150405Z"))
		data, _ := json.MarshalIndent(req, "", "  ")
		os.WriteFile(filepath.Join(crashDir, filename), data, 0644)
	}

	// 返回响应（含建议工单号）
	ticket := fmt.Sprintf("CRASH-%s-%s", name, time.Now().UTC().Format("20060102-150405"))
	writeJSON(w, http.StatusOK, model.CrashReportUploadResponse{
		Status: "accepted",
		Ticket: ticket,
		Advice: "崩溃报告已接收。你可以尝试运行 `starter repair` 自动修复环境。",
	})
}

// handleFullDownload GET /api/v1/packs/{name}/download
// 返回全量整合包 zip（增量不可用时的兜底下载）
func (s *Server) handleFullDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	versionsDir := s.store.VersionsDir(name)

	// 用 loadManifestFromDir 找出最新版本
	from := ""
	for {
		manifest := loadManifestFromDir(versionsDir, from)
		if manifest == nil {
			break
		}
		from = manifest.Version
	}
	latest := ""
	if m := loadManifestFromDir(versionsDir, ""); m != nil {
		latest = m.Version
	}

	if latest == "" {
		writeError(w, http.StatusNotFound, "VERSION_NOT_FOUND", "没有已发布版本")
		return
	}

	zipPath := filepath.Join(versionsDir, latest, "pack.zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "ZIP_NOT_FOUND", "全量包 zip 不存在，请重新导入")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, name, latest))
	http.ServeFile(w, r, zipPath)
}

// Because go 1.22 uses the new routing pattern, we need PathValue
// This works with Go 1.22+ which is already required by go.mod
