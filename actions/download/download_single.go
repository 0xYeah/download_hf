// Package download actions/download/download_single.go
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
)

func downloadSingle(url, dest string, totalSize int64) error {
	var startPos int64
	if info, err := os.Stat(dest); err == nil {
		startPos = info.Size()
		fmt.Printf("🔁 断点续传：已下载 %d 字节\n", startPos)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	if startPos > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startPos))
	}

	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败，状态码：%d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if startPos > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flags, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	downloaded := startPos
	ctx, cancel := context.WithCancel(context.Background())

	var printerWg sync.WaitGroup
	printerWg.Add(1)
	go func() {
		defer printerWg.Done()
		runProgressPrinter(ctx, totalSize, &downloaded)
	}()

	buf := make([]byte, bufSize)
	var writeErr error
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, we := f.Write(buf[:n]); we != nil {
				writeErr = fmt.Errorf("写入失败: %w", we)
				break
			}
			atomic.AddInt64(&downloaded, int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeErr = fmt.Errorf("读取失败: %w", readErr)
			break
		}
	}

	cancel()
	printerWg.Wait()
	return writeErr
}
