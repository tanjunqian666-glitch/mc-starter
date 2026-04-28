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
func (d *Downloader) File(url, dest string, expectedHash string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if expectedHash != "" {
		if ok, _ := verifyHash(dest, expectedHash); ok {
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

	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	return nil
}

func verifyHash(path, expected string) (bool, error) {
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
