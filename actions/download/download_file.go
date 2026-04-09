// Package download actions/download/download_file.go
package download

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultSegments = 10
	segmentRetries  = 3
	bufSize         = 256 * 1024
)

// File downloads a single model file using multi-segment concurrency where supported.
// Falls back to single-connection resume when the server does not support Range requests.
func File(baseURL, repoID, filePath, saveDir string) error {
	fileURL := fmt.Sprintf("%s/%s/resolve/main/%s", baseURL, repoID, filePath)
	savePath := filepath.Join(saveDir, filePath)

	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		return fmt.Errorf("创建子目录失败: %w", err)
	}

	totalSize, rangeOK, err := probeFile(fileURL)
	if err != nil {
		return fmt.Errorf("探测文件信息失败: %w", err)
	}

	if info, statErr := os.Stat(savePath); statErr == nil && totalSize > 0 && info.Size() == totalSize {
		fmt.Println("✅ 已存在，跳过")
		return nil
	}

	if rangeOK && totalSize >= int64(defaultSegments)*bufSize {
		return downloadMulti(fileURL, savePath, totalSize)
	}
	return downloadSingle(fileURL, savePath, totalSize)
}

func probeFile(url string) (size int64, rangeOK bool, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, url, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("HEAD 请求失败，状态码：%d", resp.StatusCode)
	}
	return resp.ContentLength, resp.Header.Get("Accept-Ranges") == "bytes", nil
}
