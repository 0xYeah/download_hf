// Package download actions/download/download_single.go
package download

import (
	"context"
	"errors"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	var bgWg sync.WaitGroup
	bgWg.Add(2)
	go func() { defer bgWg.Done(); runProgressPrinter(ctx, totalSize, &downloaded) }()
	go func() { defer bgWg.Done(); runWatchdog(ctx, cancel, &downloaded) }()

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
			if errors.Is(readErr, context.Canceled) {
				writeErr = fmt.Errorf("下载中断（2分钟无数据），下次运行将自动续传")
			} else {
				writeErr = fmt.Errorf("读取失败: %w", readErr)
			}
			break
		}
	}

	cancel()
	bgWg.Wait()
	return writeErr
}
