package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gege-tlph/mc-starter/internal/logger"
)

// Downloader 文件下载器
// 支持重试 + SHA256 校验。
// 注意: Minecraft Asset index 中的 hash 是 SHA1，不是 SHA256。
//
//	调用者应根据资源类型自行决定是否传 hash 参数：
//	- version_manifest / version.json / client.jar → 用 downloader 的 SHA256? 错，这些也是 SHA1
//	实际上 File() 的 expectedHash 是 SHA256 参数，调用方传入空字符串 "" 后自行校验即可。
type Downloader struct {
	client  *http.Client
	retries int
}

// New 创建下载器
func New() *Downloader {
	return &Downloader{
		client:  &http.Client{},
		retries: 3,
	}
}

// File 下载单个文件到目标路径，可选校验 SHA256
//
// expectedHash 参数存在设计陷阱：它做的是 SHA256 校验，
// 但 Minecraft 的大多数文件（version.json, client.jar, assets 等）使用 SHA1。
// 建议传 "" 由调用方自行做对应算法的校验。
//
// 下载流程：
//  1. 如果 expectedHash 非空且本地已有匹配 → 跳过（缓存命中）
//  2. 下载到 .tmp 临时文件 → 校验 hash（如有） → 原子重命名
//  3. 失败自动重试，最多 retries 次
func (d *Downloader) File(url, dest string, expectedHash string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if expectedHash != "" {
		if ok, _ := verifySHA256(dest, expectedHash); ok {
			logger.Debug("缓存命中: %s", dest)
			return nil
		}
	}

	logger.Info("下载: %s", url)
	var lastErr error
	for i := 0; i < d.retries; i++ {
		if err := d.download(url, dest, expectedHash); err != nil {
			lastErr = err
			logger.Warn("下载失败(第%d次): %v", i+1, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("下载失败(重试%d次): %w", d.retries, lastErr)
}

// download 执行单次下载
// 使用 MultiWriter 同时写入文件和计算 SHA256，避免读两次网络流
func (d *Downloader) download(url, dest string, hash string) error {
	resp, err := d.client.Get(url)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	// 边写文件边计算 SHA256（通过 MultiWriter + io.Copy）
	h := sha256.New()
	w := io.MultiWriter(f, h)
	if _, err := io.Copy(w, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("写入失败: %w", err)
	}
	f.Close()

	if hash != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != hash {
			os.Remove(tmp)
			return fmt.Errorf("hash 不匹配: 期望 %s, 实际 %s", hash, got)
		}
	}

	// 原子重命名，防止部分写入
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	return nil
}

// verifySHA256 校验已有文件的 SHA256（用于跳过已下载）
func verifySHA256(path, expected string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == expected, nil
}
