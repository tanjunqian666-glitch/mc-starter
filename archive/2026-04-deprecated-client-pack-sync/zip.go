package pack

import (
	"archive/zip"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gege-tlph/mc-starter/internal/downloader"
	"github.com/gege-tlph/mc-starter/internal/logger"
)

// ============================================================
// P1.12 — zip 整合包下载 + 解压 + hash 校验
//
// 流程:
//   1. 依据 server.json 中的 modpack source 下载 zip
//   2. SHA1/SHA256 完整性校验（可选）
//   3. 解压到临时目录
//   4. 返回解压后的文件清单（含 hash）
// ============================================================

// ZipEntry 解压后的单个文件记录
type ZipEntry struct {
	RelPath    string // 包内相对路径（如 mods/sodium.jar）
	TempPath   string // 解压后的临时路径
	SHA1       string // SHA1 hex
	SHA256     string // SHA256 hex
	Size       int64  // 字节数
}

// ZipResult zip 包处理结果
type ZipResult struct {
	Entries  []ZipEntry // 解压后的文件列表
	TempRoot string     // 解压临时根目录
}

// ZipHandler zip 包处理器
type ZipHandler struct {
	downloader *downloader.Downloader
	hash       string // 期望的 SHA256（可选）
}

// NewZipHandler 创建 zip 处理器
func NewZipHandler() *ZipHandler {
	return &ZipHandler{
		downloader: downloader.New(),
	}
}

// WithHash 设置期望的 SHA256 校验值
func (zh *ZipHandler) WithHash(hash string) *ZipHandler {
	zh.hash = hash
	return zh
}

// DownloadAndExtract 下载 zip 包并解压
//
// 参数:
//   - url:      zip 下载地址
//   - destDir:  解压目标目录（会创建 zip-{name} 子目录）
//   - name:     包标识（用于目录命名和日志）
//
// 返回值:
//   - ZipResult: 包含解压文件清单和临时根目录
//   - error:     错误信息
func (zh *ZipHandler) DownloadAndExtract(url, destDir, name string) (*ZipResult, error) {
	logger.Info("pack: 开始处理 %s (%s)", name, url)

	// 下载 zip
	zipPath, err := zh.downloadZip(url, destDir, name)
	if err != nil {
		return nil, fmt.Errorf("下载 zip 包失败: %w", err)
	}

	// 校验完整性
	if zh.hash != "" {
		logger.Info("pack: 校验 SHA256...")
		if err := verifySHA256(zipPath, zh.hash); err != nil {
			os.Remove(zipPath)
			return nil, fmt.Errorf("SHA256 校验失败: %w", err)
		}
	}

	// 解压
	result, err := zh.extractZip(zipPath, destDir, name)
	if err != nil {
		return nil, fmt.Errorf("解压 zip 包失败: %w", err)
	}

	// 解压完成后删除 zip 文件
	if err := os.Remove(zipPath); err != nil {
		logger.Warn("pack: 删除临时 zip 失败: %v", err)
	}

	logger.Info("pack: %s 处理完成 — %d 个文件, 临时目录 %s", name, len(result.Entries), result.TempRoot)
	return result, nil
}

// ExtractExisting 直接解压已存在的 zip 文件（不下载）
func (zh *ZipHandler) ExtractExisting(zipPath, destDir, name string) (*ZipResult, error) {
	return zh.extractZip(zipPath, destDir, name)
}

// downloadZip 下载 zip 文件
func (zh *ZipHandler) downloadZip(url, destDir, name string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("创建目标目录失败: %w", err)
	}

	zipPath := filepath.Join(destDir, fmt.Sprintf("%s.zip", name))
	if err := zh.downloader.File(url, zipPath, ""); err != nil {
		return "", err
	}
	return zipPath, nil
}

// extractZip 解压 zip 文件到临时目录并构建文件清单
func (zh *ZipHandler) extractZip(zipPath, destDir, name string) (*ZipResult, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("打开 zip 文件失败: %w", err)
	}
	defer reader.Close()

	// 创建解压目录
	tempRoot := filepath.Join(destDir, fmt.Sprintf("zip-%s", name))
	if err := os.MkdirAll(tempRoot, 0755); err != nil {
		return nil, fmt.Errorf("创建解压目录 %s 失败: %w", tempRoot, err)
	}

	var entries []ZipEntry
	for _, f := range reader.File {
		// 跳过目录项
		if f.FileInfo().IsDir() {
			continue
		}

		// 安全处理路径 — 防止 zip slip
		relPath := sanitizePath(f.Name)
		targetPath := filepath.Join(tempRoot, relPath)

		// 创建父目录
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			logger.Warn("pack: 创建目录 %s 失败: %v", filepath.Dir(targetPath), err)
			continue
		}

		// 解压文件
		if err := extractZipFile(f, targetPath); err != nil {
			logger.Warn("pack: 解压 %s 失败: %v", relPath, err)
			continue
		}

		// 计算 hash
		sha1Hash, sha256Hash, err := computeHashes(targetPath)
		if err != nil {
			logger.Warn("pack: 计算 %s 的 hash 失败: %v", relPath, err)
			continue
		}

		entry := ZipEntry{
			RelPath:  relPath,
			TempPath: targetPath,
			SHA1:     sha1Hash,
			SHA256:   sha256Hash,
			Size:     f.FileInfo().Size(),
		}
		entries = append(entries, entry)
	}

	result := &ZipResult{
		Entries:  entries,
		TempRoot: tempRoot,
	}

	return result, nil
}

// Cleanup 清理解压产生的临时文件
func (zr *ZipResult) Cleanup() {
	if zr.TempRoot != "" {
		if err := os.RemoveAll(zr.TempRoot); err != nil {
			logger.Warn("pack: 清理临时目录 %s 失败: %v", zr.TempRoot, err)
		}
	}
}

// ============================================================
// 辅助函数
// ============================================================

// sanitizePath 清理路径，防止 zip slip 攻击
// 移除 .. 、前导 / 等危险路径
func sanitizePath(path string) string {
	// 统一分隔符
	path = strings.ReplaceAll(path, "\\", "/")
	// 清理 . 和 ..
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
	return strings.Join(clean, string(filepath.Separator))
}

// extractZipFile 解压 zip 中的单个文件
func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("打开 zip 条目失败: %w", err)
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("创建文件 %s 失败: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("写入文件 %s 失败: %w", dest, err)
	}
	return nil
}

// computeHashes 计算文件的 SHA1 和 SHA256
func computeHashes(path string) (sha1hex, sha256hex string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	sha1Hasher := sha1.New()
	sha256Hasher := sha256.New()
	w := io.MultiWriter(sha1Hasher, sha256Hasher)

	if _, err := io.Copy(w, f); err != nil {
		return "", "", err
	}

	sha1hex = hex.EncodeToString(sha1Hasher.Sum(nil))
	sha256hex = hex.EncodeToString(sha256Hasher.Sum(nil))
	return sha1hex, sha256hex, nil
}

// verifySHA256 校验文件 SHA256
func verifySHA256(path, expected string) error {
	_, hexStr, err := computeHashes(path)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}
	if hexStr != expected {
		return fmt.Errorf("SHA256 不匹配: 期望 %s, 实际 %s", expected, hexStr)
	}
	return nil
}
